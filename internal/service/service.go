// SPDX-License-Identifier: Apache-2.0

// Package service implements the org.freedesktop.Secret.Service D-Bus interface
// and all sub-objects (Collection, Item, Session, Prompt) required by the
// Freedesktop.org Secret Service specification version 0.2.
package service

import (
	"fmt"
	"log"
	"math/big"
	"strings"

	"github.com/akihiro/wsl-secret-service/internal/backend"
	"github.com/akihiro/wsl-secret-service/internal/store"
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/prop"
	"github.com/google/uuid"
)

// Service is the root D-Bus object at /org/freedesktop/secrets.
// It implements org.freedesktop.Secret.Service.
type Service struct {
	conn        *dbus.Conn
	store       *store.Store
	backend     backend.Backend
	sessions    *sessionRegistry
	collections map[string]*Collection // keyed by collection name
	svcProps    *prop.Properties
}

// New creates and fully initialises the Secret Service:
//   - exports all D-Bus objects (Service, existing Collections, their Items, the stub Prompt)
//   - subscribes to NameOwnerChanged to clean up orphaned sessions
//
// The caller is responsible for requesting the well-known bus name before
// calling New, or passing replaceExisting=true to RequestName.
func New(conn *dbus.Conn, st *store.Store, be backend.Backend) (*Service, error) {
	svc := &Service{
		conn:        conn,
		store:       st,
		backend:     be,
		sessions:    newSessionRegistry(),
		collections: make(map[string]*Collection),
	}

	// Export Service methods.
	if err := conn.Export(svc, dbus.ObjectPath(ServicePath), ServiceIface); err != nil {
		return nil, fmt.Errorf("export service: %w", err)
	}

	// Export Service properties.
	if err := svc.exportServiceProps(); err != nil {
		return nil, fmt.Errorf("export service props: %w", err)
	}

	// Export the stub Prompt object.
	prompt := &Prompt{path: PromptStubObjPath, conn: conn}
	if err := conn.Export(prompt, PromptStubObjPath, PromptIface); err != nil {
		return nil, fmt.Errorf("export prompt: %w", err)
	}

	// Export all persisted collections and their items.
	for _, colName := range st.ListCollections() {
		if err := svc.loadCollection(colName); err != nil {
			log.Printf("warning: could not load collection %q: %v", colName, err)
		}
	}

	// Subscribe to NameOwnerChanged to clean up sessions when clients disconnect.
	conn.BusObject().AddMatchSignal("org.freedesktop.DBus", "NameOwnerChanged")
	go svc.watchNameOwnerChanged()

	return svc, nil
}

// exportServiceProps registers the Properties interface for the Service object.
func (svc *Service) exportServiceProps() error {
	colNames := svc.store.ListCollections()
	colPaths := make([]dbus.ObjectPath, len(colNames))
	for i, name := range colNames {
		colPaths[i] = CollectionPath(name)
	}

	propsSpec := prop.Map{
		ServiceIface: {
			"Collections": {
				Value:    colPaths,
				Writable: false,
				Emit:     prop.EmitTrue,
			},
		},
	}
	p, err := prop.Export(svc.conn, dbus.ObjectPath(ServicePath), propsSpec)
	if err != nil {
		return err
	}
	svc.svcProps = p
	return nil
}

// loadCollection exports an existing collection and all its items from the store.
func (svc *Service) loadCollection(name string) error {
	col := &Collection{name: name, svc: svc}
	if err := svc.exportCollection(col); err != nil {
		return err
	}
	svc.collections[name] = col

	// Export each item in the collection.
	for _, itemUUID := range svc.store.ListItems(name) {
		item := &Item{collectionName: name, uuid: itemUUID, svc: svc}
		if err := svc.exportItem(item); err != nil {
			log.Printf("warning: could not export item %s/%s: %v", name, itemUUID, err)
		}
	}
	return nil
}

// updateCollectionsProp refreshes the Collections property on the Service object.
func (svc *Service) updateCollectionsProp() {
	if svc.svcProps == nil {
		return
	}
	names := svc.store.ListCollections()
	paths := make([]dbus.ObjectPath, len(names))
	for i, n := range names {
		paths[i] = CollectionPath(n)
	}
	svc.svcProps.SetMust(ServiceIface, "Collections", paths)
}

// watchNameOwnerChanged listens for D-Bus client disconnections and removes
// any sessions owned by the disconnected client.
func (svc *Service) watchNameOwnerChanged() {
	ch := make(chan *dbus.Signal, 16)
	svc.conn.Signal(ch)
	for sig := range ch {
		if sig.Name != "org.freedesktop.DBus.NameOwnerChanged" {
			continue
		}
		if len(sig.Body) < 3 {
			continue
		}
		// Body: [name, oldOwner, newOwner]
		newOwner, _ := sig.Body[2].(string)
		if newOwner != "" {
			continue // name gained a new owner — not a disconnect
		}
		// A client disconnected; remove all sessions in memory whose path
		// ends with that client's unique name (we don't currently track
		// per-sender sessions, so this is a best-effort cleanup for future
		// sender-tagged sessions).
		// For now, just let sessions GC naturally on Close().
	}
}

// --- org.freedesktop.Secret.Service methods ---

// OpenSession implements Service.OpenSession(algorithm, input).
// Supports "plain" and "dh-ietf1024-sha256-aes128-cbc-pkcs7".
func (svc *Service) OpenSession(algorithm string, input dbus.Variant) (dbus.Variant, dbus.ObjectPath, *dbus.Error) {
	var sess *Session
	var output dbus.Variant

	switch algorithm {
	case "plain":
		sess = &Session{path: SessionPath(uuid.New().String()), conn: svc.conn, svc: svc}
		output = dbus.MakeVariant("")

	case "dh-ietf1024-sha256-aes128-cbc-pkcs7":
		clientPubBytes, ok := input.Value().([]byte)
		if !ok || len(clientPubBytes) == 0 {
			return dbus.MakeVariant(""), "/",
				dbusError("org.freedesktop.DBus.Error.InvalidArgs", "expected client DH public key as byte array")
		}
		clientPubKey := new(big.Int).SetBytes(clientPubBytes)

		privKey, pubKey, err := dhGenerateKeyPair()
		if err != nil {
			return dbus.MakeVariant(""), "/",
				dbusError("org.freedesktop.DBus.Error.Failed", fmt.Sprintf("generate DH key pair: %v", err))
		}

		aesKey := dhDeriveAESKey(privKey, clientPubKey)
		serverPubBytes := bigIntToGroupBytes(pubKey)

		sess = &Session{
			path:   SessionPath(uuid.New().String()),
			conn:   svc.conn,
			svc:    svc,
			aesKey: aesKey,
		}
		output = dbus.MakeVariant(serverPubBytes)

	default:
		return dbus.MakeVariant(""), "/",
			&dbus.Error{
				Name: "org.freedesktop.Secret.Error.NotSupported",
				Body: []any{fmt.Sprintf("unsupported session algorithm %q", algorithm)},
			}
	}

	if err := svc.conn.Export(sess, sess.path, SessionIface); err != nil {
		return dbus.MakeVariant(""), "/",
			dbusError("org.freedesktop.DBus.Error.Failed", fmt.Sprintf("export session: %v", err))
	}
	svc.sessions.add(sess)
	return output, sess.path, nil
}

// CreateCollection implements Service.CreateCollection(properties, alias).
// If alias already maps to an existing collection, that collection is returned.
// Returns (collectionPath, "/") — no prompt is ever needed.
func (svc *Service) CreateCollection(
	properties map[string]dbus.Variant,
	alias string,
) (dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	// If the alias already resolves, return that collection.
	if alias != "" {
		if existing := svc.store.GetAlias(alias); existing != "" {
			return CollectionPath(existing), StubPromptPath, nil
		}
	}

	// Extract label from properties.
	label := "Secrets"
	if v, ok := properties[CollectionIface+".Label"]; ok {
		if s, ok := v.Value().(string); ok && s != "" {
			label = s
		}
	}

	// Derive a slug from the label for the collection name.
	name := collectionSlug(label)
	// Ensure uniqueness.
	base := name
	for i := 2; ; i++ {
		if _, exists := svc.store.GetCollection(name); !exists {
			break
		}
		name = fmt.Sprintf("%s%d", base, i)
	}

	// Persist.
	if err := svc.store.CreateCollection(name, label); err != nil {
		return "/", StubPromptPath, dbusError("org.freedesktop.DBus.Error.Failed", err.Error())
	}

	// Set alias if requested.
	if alias != "" {
		_ = svc.store.SetAlias(alias, name)
	}

	// Export.
	col := &Collection{name: name, svc: svc}
	if err := svc.exportCollection(col); err != nil {
		return "/", StubPromptPath, dbusError("org.freedesktop.DBus.Error.Failed", err.Error())
	}
	svc.collections[name] = col

	colPath := CollectionPath(name)
	_ = svc.conn.Emit(dbus.ObjectPath(ServicePath), ServiceIface+".CollectionCreated", colPath)
	svc.updateCollectionsProp()

	return colPath, StubPromptPath, nil
}

// SearchItems implements Service.SearchItems(attributes).
// Returns (unlocked, locked) — all items are always unlocked.
func (svc *Service) SearchItems(attributes map[string]string) ([]dbus.ObjectPath, []dbus.ObjectPath, *dbus.Error) {
	refs := svc.store.SearchItems(attributes)
	paths := make([]dbus.ObjectPath, len(refs))
	for i, ref := range refs {
		paths[i] = ItemPath(ref.Collection, ref.UUID)
	}
	return paths, []dbus.ObjectPath{}, nil
}

// Unlock implements Service.Unlock(objects).
// All objects are always unlocked. Returns (objects, "/").
func (svc *Service) Unlock(objects []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	return objects, StubPromptPath, nil
}

// Lock implements Service.Lock(objects).
// Locking is not supported; returns ([], "/").
func (svc *Service) Lock(objects []dbus.ObjectPath) ([]dbus.ObjectPath, dbus.ObjectPath, *dbus.Error) {
	return []dbus.ObjectPath{}, StubPromptPath, nil
}

// GetSecrets implements Service.GetSecrets(items, session).
// Returns a map of item path → Secret for each requested item.
func (svc *Service) GetSecrets(
	items []dbus.ObjectPath,
	session dbus.ObjectPath,
) (map[dbus.ObjectPath]Secret, *dbus.Error) {
	sess, ok := svc.sessions.get(session)
	if !ok {
		return nil, dbusError("org.freedesktop.Secret.Error.NoSession",
			fmt.Sprintf("session %s is not open", session))
	}

	result := make(map[dbus.ObjectPath]Secret, len(items))
	for _, itemPath := range items {
		colName, itemUUID := ItemUUIDFromPath(itemPath)
		if colName == "" || itemUUID == "" {
			continue
		}
		meta, ok := svc.store.GetItem(colName, itemUUID)
		if !ok {
			continue
		}
		target := fmt.Sprintf("wsl-ss/%s/%s", colName, itemUUID)
		secretBytes, err := svc.backend.Get(target)
		if err != nil {
			continue // Skip items whose secrets can't be retrieved.
		}
		ct := meta.ContentType
		if ct == "" {
			ct = "text/plain; charset=utf8"
		}
		params, value, err := sess.encryptSecret(secretBytes)
		if err != nil {
			log.Printf("warning: could not encrypt secret for %s: %v", itemPath, err)
			continue
		}
		result[itemPath] = Secret{
			Session:     session,
			Parameters:  params,
			Value:       value,
			ContentType: ct,
		}
	}
	return result, nil
}

// ReadAlias implements Service.ReadAlias(name).
// Returns the collection path for the given alias, or "/" if not found.
func (svc *Service) ReadAlias(name string) (dbus.ObjectPath, *dbus.Error) {
	colName := svc.store.GetAlias(name)
	if colName == "" {
		return "/", nil
	}
	return CollectionPath(colName), nil
}

// SetAlias implements Service.SetAlias(name, collection).
// Passing "/" or "" as collection removes the alias.
func (svc *Service) SetAlias(name string, collection dbus.ObjectPath) *dbus.Error {
	colStr := string(collection)
	if colStr == "/" || colStr == "" {
		if err := svc.store.SetAlias(name, ""); err != nil {
			return dbusError("org.freedesktop.DBus.Error.Failed", err.Error())
		}
		return nil
	}
	colName := CollectionNameFromPath(collection)
	if colName == "" {
		return dbusError("org.freedesktop.DBus.Error.InvalidArgs",
			fmt.Sprintf("invalid collection path: %s", collection))
	}
	if err := svc.store.SetAlias(name, colName); err != nil {
		return dbusError("org.freedesktop.DBus.Error.Failed", err.Error())
	}
	return nil
}

// collectionSlug converts a human-readable label into a valid D-Bus path component.
// e.g., "My Secrets" → "mysecrets"
func collectionSlug(label string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(label) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "collection"
	}
	return b.String()
}
