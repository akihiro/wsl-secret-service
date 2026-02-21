// SPDX-License-Identifier: Apache-2.0

package service

import (
	"fmt"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/google/uuid"
)

// Collection implements the org.freedesktop.Secret.Collection D-Bus interface.
// Each collection is registered at /org/freedesktop/secrets/collection/{name}.
type Collection struct {
	name  string
	svc   *Service
	props *prop.Properties
}

// Delete implements org.freedesktop.Secret.Collection.Delete().
// Removes all items from the backend and metadata store, then unexports the object.
// Returns "/" (no prompt needed).
func (c *Collection) Delete() (dbus.ObjectPath, *dbus.Error) {
	path := CollectionPath(c.name)

	// Delete all items from backend and store.
	for _, itemUUID := range c.svc.store.ListItems(c.name) {
		target := fmt.Sprintf("wsl-ss/%s/%s", c.name, itemUUID)
		_ = c.svc.backend.Delete(target)
		itemPath := ItemPath(c.name, itemUUID)
		c.svc.conn.Export(nil, itemPath, ItemIface)
		c.svc.conn.Export(nil, itemPath, "org.freedesktop.DBus.Properties")
	}

	// Delete from store (removes collection + all items).
	if err := c.svc.store.DeleteCollection(c.name); err != nil {
		return StubPromptPath, dbusError("org.freedesktop.Secret.Error.NoSuchObject", err.Error())
	}

	// Unexport collection D-Bus objects.
	c.svc.conn.Export(nil, path, CollectionIface)
	c.svc.conn.Export(nil, path, "org.freedesktop.DBus.Properties")

	// Remove from in-memory map.
	delete(c.svc.collections, c.name)

	// Emit signal and update Service.Collections property.
	_ = c.svc.conn.Emit(
		dbus.ObjectPath(ServicePath),
		ServiceIface+".CollectionDeleted",
		path,
	)
	c.svc.updateCollectionsProp()

	return StubPromptPath, nil
}

// SearchItems implements org.freedesktop.Secret.Collection.SearchItems(attributes).
// Returns all item paths in this collection whose attributes are a superset of attrs.
func (c *Collection) SearchItems(attributes map[string]string) ([]dbus.ObjectPath, *dbus.Error) {
	refs := c.svc.store.SearchItemsInCollection(c.name, attributes)
	paths := make([]dbus.ObjectPath, len(refs))
	for i, ref := range refs {
		paths[i] = ItemPath(ref.Collection, ref.UUID)
	}
	return paths, nil
}

// CreateItem implements org.freedesktop.Secret.Collection.CreateItem(properties, secret, replace).
// Creates a new item (or replaces an existing one if replace=true and attributes match).
// Returns (itemPath, "/") — no prompt is ever needed.
func (c *Collection) CreateItem(
	properties map[string]dbus.Variant,
	secret Secret,
	replace bool,
) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	// Validate session.
	if _, ok := c.svc.sessions.get(secret.Session); !ok {
		return "/", StubPromptPath, dbusError("org.freedesktop.Secret.Error.NoSession",
			fmt.Sprintf("session %s is not open", secret.Session))
	}

	meta := itemMetaFromProperties(properties)
	if meta.ContentType == "" && secret.ContentType != "" {
		meta.ContentType = secret.ContentType
	}

	// Check for replace: look for an existing item with identical attributes.
	var targetUUID string
	if replace && len(meta.Attributes) > 0 {
		refs := c.svc.store.SearchItemsInCollection(c.name, meta.Attributes)
		if len(refs) > 0 {
			targetUUID = refs[0].UUID
		}
	}

	if targetUUID == "" {
		// Generate a new UUID for this item.
		targetUUID = uuid.New().String()
	}

	target := fmt.Sprintf("wsl-ss/%s/%s", c.name, targetUUID)

	// Store the secret in the backend.
	if err := c.svc.backend.Set(target, secret.Value); err != nil {
		return "/", StubPromptPath, dbusError("org.freedesktop.DBus.Error.Failed",
			fmt.Sprintf("store secret: %v", err))
	}

	// Persist metadata.
	if _, exists := c.svc.store.GetItem(c.name, targetUUID); exists {
		if err := c.svc.store.UpdateItem(c.name, targetUUID, meta); err != nil {
			return "/", StubPromptPath, dbusError("org.freedesktop.DBus.Error.Failed", err.Error())
		}
	} else {
		if err := c.svc.store.CreateItem(c.name, targetUUID, meta); err != nil {
			return "/", StubPromptPath, dbusError("org.freedesktop.DBus.Error.Failed", err.Error())
		}
	}

	// Export the Item D-Bus object.
	item := &Item{
		collectionName: c.name,
		uuid:           targetUUID,
		svc:            c.svc,
	}
	if err := c.svc.exportItem(item); err != nil {
		return "/", StubPromptPath, dbusError("org.freedesktop.DBus.Error.Failed", err.Error())
	}

	itemPath := ItemPath(c.name, targetUUID)

	// Update the Items property and emit signal.
	c.svc.updateCollectionItemsProp(c.name)
	_ = c.svc.conn.Emit(CollectionPath(c.name), CollectionIface+".ItemCreated", itemPath)

	return itemPath, StubPromptPath, nil
}

// exportCollection exports all D-Bus interfaces for a collection onto the connection.
func (svc *Service) exportCollection(col *Collection) error {
	path := CollectionPath(col.name)

	// Export the Collection interface (methods).
	if err := svc.conn.Export(col, path, CollectionIface); err != nil {
		return fmt.Errorf("export collection methods at %s: %w", path, err)
	}

	// Build initial Items list.
	uuids := svc.store.ListItems(col.name)
	itemPaths := make([]dbus.ObjectPath, len(uuids))
	for i, u := range uuids {
		itemPaths[i] = ItemPath(col.name, u)
	}

	// Get collection metadata for properties.
	meta, _ := svc.store.GetCollection(col.name)

	// Export properties.
	propsSpec := prop.Map{
		CollectionIface: {
			"Items": {
				Value:    itemPaths,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
			"Label": {
				Value:    meta.Label,
				Writable: true,
				Emit:     prop.EmitTrue,
				Callback: func(c *prop.Change) *dbus.Error {
					if label, ok := c.Value.(string); ok {
						_ = svc.store.UpdateCollectionLabel(col.name, label)
					}
					return nil
				},
			},
			"Locked": {
				Value:    false,
				Writable: false,
				Emit:     prop.EmitFalse,
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
		return fmt.Errorf("export collection properties at %s: %w", path, err)
	}
	col.props = props
	return nil
}
