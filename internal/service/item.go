// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"

	"github.com/akihiro/wsl-secret-service/internal/store"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
)

// Item implements the org.freedesktop.Secret.Item D-Bus interface.
// Each item is registered at /org/freedesktop/secrets/collection/{col}/{uuid}.
type Item struct {
	collectionName string
	uuid           string
	svc            *Service
	props          *prop.Properties
}

// itemTarget returns the Windows Credential Manager TargetName for this item.
func (i *Item) itemTarget() string {
	return fmt.Sprintf("wsl-ss/%s/%s", i.collectionName, i.uuid)
}

// Delete implements org.freedesktop.Secret.Item.Delete().
// Removes the item from the metadata store and backend, then unexports the D-Bus object.
// Returns "/" (no prompt needed).
func (i *Item) Delete() (dbus.ObjectPath, *dbus.Error) {
	i.svc.recordActivity()

	target := i.itemTarget()
	path := ItemPath(i.collectionName, i.uuid)

	// Remove from backend (ignore not-found since metadata may exist without a secret).
	_ = i.svc.backend.Delete(target)

	// Remove from metadata store.
	if err := i.svc.store.DeleteItem(i.collectionName, i.uuid); err != nil {
		return StubPromptPath, dbusError("org.freedesktop.Secret.Error.NoSuchObject", err.Error())
	}

	// Unexport D-Bus object.
	_ = i.svc.conn.Export(nil, path, ItemIface)
	_ = i.svc.conn.Export(nil, path, "org.freedesktop.DBus.Properties")

	// Notify the collection that an item was deleted and update its Items property.
	i.svc.notifyItemDeleted(i.collectionName, path)

	return StubPromptPath, nil
}

// GetSecret implements org.freedesktop.Secret.Item.GetSecret(session).
func (i *Item) GetSecret(session dbus.ObjectPath) (dbus.Variant, *dbus.Error) {
	i.svc.recordActivity()

	sess, ok := i.svc.sessions.get(session)
	if !ok {
		return dbus.Variant{}, dbusError("org.freedesktop.Secret.Error.NoSession",
			fmt.Sprintf("session %s is not open", session))
	}

	meta, ok := i.svc.store.GetItem(i.collectionName, i.uuid)
	if !ok {
		return dbus.Variant{}, dbusError("org.freedesktop.Secret.Error.NoSuchObject",
			fmt.Sprintf("item %s/%s not found", i.collectionName, i.uuid))
	}

	secretBytes, err := i.svc.backend.Get(i.itemTarget())
	if err != nil {
		return dbus.Variant{}, dbusError("org.freedesktop.Secret.Error.IsLocked",
			fmt.Sprintf("retrieve secret: %v", err))
	}

	ct := meta.ContentType
	if ct == "" {
		ct = "text/plain; charset=utf8"
	}

	params, value, err := sess.encryptSecret(secretBytes)
	if err != nil {
		return dbus.Variant{}, dbusError("org.freedesktop.DBus.Error.Failed",
			fmt.Sprintf("encrypt secret: %v", err))
	}

	secret := Secret{
		Session:     session,
		Parameters:  params,
		Value:       value,
		ContentType: ct,
	}
	return dbus.MakeVariant(secret), nil
}

// SetSecret implements org.freedesktop.Secret.Item.SetSecret(secret).
// Stores the new secret value and updates the Modified timestamp.
func (i *Item) SetSecret(secret dbus.Variant) *dbus.Error {
	i.svc.recordActivity()

	// Unmarshal the secret variant into the Secret struct.
	var sec Secret
	if err := secret.Store(&sec); err != nil {
		return dbusError("org.freedesktop.DBus.Error.InvalidArgs",
			fmt.Sprintf("invalid secret variant: %v", err))
	}

	sess, ok := i.svc.sessions.get(sec.Session)
	if !ok {
		return dbusError("org.freedesktop.Secret.Error.NoSession",
			fmt.Sprintf("session %s is not open", sec.Session))
	}

	plaintext, err := sess.decryptSecret(sec.Parameters, sec.Value)
	if err != nil {
		return dbusError("org.freedesktop.DBus.Error.Failed",
			fmt.Sprintf("decrypt secret: %v", err))
	}

	if err := i.svc.backend.Set(i.itemTarget(), plaintext); err != nil {
		return dbusError("org.freedesktop.DBus.Error.Failed",
			fmt.Sprintf("store secret: %v", err))
	}

	// Update content type and modified timestamp in the store.
	meta, ok := i.svc.store.GetItem(i.collectionName, i.uuid)
	if ok {
		meta.ContentType = sec.ContentType
		_ = i.svc.store.UpdateItem(i.collectionName, i.uuid, meta)
	}

	i.svc.notifyItemChanged(i.collectionName, ItemPath(i.collectionName, i.uuid))
	return nil
}

// exportItem exports all D-Bus interfaces for this item onto the connection.
// Called once when the item is first created or loaded from the store.
func (svc *Service) exportItem(item *Item) error {
	path := ItemPath(item.collectionName, item.uuid)

	// Export the Item interface (methods).
	if err := svc.conn.Export(item, path, ItemIface); err != nil {
		return fmt.Errorf("export item methods at %s: %w", path, err)
	}

	// Export properties.
	meta, _ := svc.store.GetItem(item.collectionName, item.uuid)
	propsSpec := prop.Map{
		ItemIface: {
			"Locked": {
				Value:    false,
				Writable: false,
				Emit:     prop.EmitFalse,
			},
			"Attributes": {
				Value:    attrsOrEmpty(meta.Attributes),
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: func(c *prop.Change) *dbus.Error {
					if newAttrs, ok := c.Value.(map[string]string); ok {
						m, exists := svc.store.GetItem(item.collectionName, item.uuid)
						if exists {
							m.Attributes = newAttrs
							_ = svc.store.UpdateItem(item.collectionName, item.uuid, m)
						}
					}
					return nil
				},
			},
			"Label": {
				Value:    meta.Label,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: func(c *prop.Change) *dbus.Error {
					if label, ok := c.Value.(string); ok {
						m, exists := svc.store.GetItem(item.collectionName, item.uuid)
						if exists {
							m.Label = label
							_ = svc.store.UpdateItem(item.collectionName, item.uuid, m)
						}
					}
					return nil
				},
			},
			"Created": {
				Value:    meta.Created,
				Writable: false,
				Emit:     prop.EmitFalse,
			},
			"Modified": {
				Value:    meta.Modified,
				Writable: false,
				Emit:     prop.EmitFalse,
			},
		},
	}

	props, err := prop.Export(svc.conn, path, propsSpec)
	if err != nil {
		return fmt.Errorf("export item properties at %s: %w", path, err)
	}
	item.props = props

	// Explicitly export the standard D-Bus Properties interface for proper introspection.
	// This ensures clients can discover that the object implements org.freedesktop.DBus.Properties.
	if err := svc.conn.Export(item, path, "org.freedesktop.DBus.Properties"); err != nil {
		return fmt.Errorf("export item properties interface at %s: %w", path, err)
	}

	return nil
}

func attrsOrEmpty(a map[string]string) map[string]string {
	if a == nil {
		return map[string]string{}
	}
	return a
}

// notifyItemDeleted emits Collection.ItemDeleted and updates the Items property.
func (svc *Service) notifyItemDeleted(collectionName string, itemPath dbus.ObjectPath) {
	colPath := CollectionPath(collectionName)
	_ = svc.conn.Emit(colPath, CollectionIface+".ItemDeleted", itemPath)
	svc.updateCollectionItemsProp(collectionName)
}

// notifyItemChanged emits Collection.ItemChanged.
func (svc *Service) notifyItemChanged(collectionName string, itemPath dbus.ObjectPath) {
	colPath := CollectionPath(collectionName)
	_ = svc.conn.Emit(colPath, CollectionIface+".ItemChanged", itemPath)
}

// dbusError creates a D-Bus error with the given name and message.
func dbusError(name, msg string) *dbus.Error {
	return &dbus.Error{Name: name, Body: []interface{}{msg}}
}

// updateCollectionItemsProp refreshes the Items property of a collection.
func (svc *Service) updateCollectionItemsProp(collectionName string) {
	col, ok := svc.collections[collectionName]
	if !ok {
		return
	}
	uuids := svc.store.ListItems(collectionName)
	paths := make([]dbus.ObjectPath, len(uuids))
	for idx, u := range uuids {
		paths[idx] = ItemPath(collectionName, u)
	}
	if col.props != nil {
		col.props.SetMust(CollectionIface, "Items", paths)
	}
}

// itemMetaFromProperties parses item properties from a CreateItem call.
func itemMetaFromProperties(properties map[string]dbus.Variant) store.ItemMeta {
	meta := store.ItemMeta{
		Attributes:  make(map[string]string),
		ContentType: "text/plain; charset=utf8",
	}
	if v, ok := properties[CollectionIface+".Label"]; ok {
		if s, ok := v.Value().(string); ok {
			meta.Label = s
		}
	}
	if v, ok := properties[ItemIface+".Label"]; ok {
		if s, ok := v.Value().(string); ok {
			meta.Label = s
		}
	}
	if v, ok := properties[ItemIface+".Attributes"]; ok {
		if attrs, ok := v.Value().(map[string]string); ok {
			meta.Attributes = attrs
		}
	}
	return meta
}
