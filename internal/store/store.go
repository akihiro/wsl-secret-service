// Package store manages persistent metadata for Secret Service collections and items.
// Only metadata (labels, attributes, timestamps, content type) is stored here.
// The actual secret values are stored in the Windows Credential Manager via the backend.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ItemMeta holds the metadata for a single secret item.
type ItemMeta struct {
	Label       string            `json:"label"`
	Attributes  map[string]string `json:"attributes"`
	Created     uint64            `json:"created"`
	Modified    uint64            `json:"modified"`
	ContentType string            `json:"content_type"`
}

// CollectionMeta holds the metadata for a collection of items.
type CollectionMeta struct {
	Label    string              `json:"label"`
	Created  uint64              `json:"created"`
	Modified uint64              `json:"modified"`
	Items    map[string]ItemMeta `json:"items"`
}

// storeData is the top-level JSON structure persisted to disk.
type storeData struct {
	Version     int                       `json:"version"`
	Collections map[string]CollectionMeta `json:"collections"`
	Aliases     map[string]string         `json:"aliases"`
}

// ItemRef identifies an item by collection name and UUID.
type ItemRef struct {
	Collection string
	UUID       string
}

// Store provides thread-safe access to Secret Service metadata.
type Store struct {
	path string
	mu   sync.RWMutex
	data storeData
}

// New creates (or loads) the metadata store at configDir/metadata.json.
// If the store is new, it creates a default "login" collection with the "default" alias.
func New(configDir string) (*Store, error) {
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}

	s := &Store{
		path: filepath.Join(configDir, "metadata.json"),
		data: storeData{
			Version:     1,
			Collections: make(map[string]CollectionMeta),
			Aliases:     make(map[string]string),
		},
	}

	if err := s.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load metadata: %w", err)
	}

	// Ensure the "login" collection and "default" alias always exist.
	if _, ok := s.data.Collections["login"]; !ok {
		now := uint64(time.Now().Unix())
		s.data.Collections["login"] = CollectionMeta{
			Label:    "Login",
			Created:  now,
			Modified: now,
			Items:    make(map[string]ItemMeta),
		}
		s.data.Aliases["default"] = "login"
		if err := s.save(); err != nil {
			return nil, fmt.Errorf("save initial metadata: %w", err)
		}
	}

	return s, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, &s.data)
}

// save writes metadata.json atomically via a temp file + rename.
// Caller must hold s.mu (write lock).
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write tmp metadata: %w", err)
	}
	return os.Rename(tmp, s.path)
}

// Save persists current state to disk.
func (s *Store) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.save()
}

// --- Collections ---

// GetCollection returns a copy of the collection metadata for name.
func (s *Store) GetCollection(name string) (CollectionMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.data.Collections[name]
	return c, ok
}

// ListCollections returns all collection names.
func (s *Store) ListCollections() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	names := make([]string, 0, len(s.data.Collections))
	for name := range s.data.Collections {
		names = append(names, name)
	}
	return names
}

// CreateCollection adds a new collection. Returns error if it already exists.
func (s *Store) CreateCollection(name, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Collections[name]; ok {
		return fmt.Errorf("collection %q already exists", name)
	}
	now := uint64(time.Now().Unix())
	s.data.Collections[name] = CollectionMeta{
		Label:    label,
		Created:  now,
		Modified: now,
		Items:    make(map[string]ItemMeta),
	}
	return s.save()
}

// UpdateCollectionLabel updates the label of an existing collection.
func (s *Store) UpdateCollectionLabel(name, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.data.Collections[name]
	if !ok {
		return fmt.Errorf("collection %q not found", name)
	}
	c.Label = label
	c.Modified = uint64(time.Now().Unix())
	s.data.Collections[name] = c
	return s.save()
}

// DeleteCollection removes a collection and all its items.
func (s *Store) DeleteCollection(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.data.Collections[name]; !ok {
		return fmt.Errorf("collection %q not found", name)
	}
	delete(s.data.Collections, name)
	// Remove any aliases pointing to this collection.
	for alias, target := range s.data.Aliases {
		if target == name {
			delete(s.data.Aliases, alias)
		}
	}
	return s.save()
}

// --- Items ---

// GetItem returns a copy of the item metadata.
func (s *Store) GetItem(collection, uuid string) (ItemMeta, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.data.Collections[collection]
	if !ok {
		return ItemMeta{}, false
	}
	item, ok := c.Items[uuid]
	return item, ok
}

// ListItems returns all item UUIDs in a collection.
func (s *Store) ListItems(collection string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c, ok := s.data.Collections[collection]
	if !ok {
		return nil
	}
	uuids := make([]string, 0, len(c.Items))
	for uuid := range c.Items {
		uuids = append(uuids, uuid)
	}
	return uuids
}

// CreateItem adds a new item to a collection.
func (s *Store) CreateItem(collection, uuid string, meta ItemMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.data.Collections[collection]
	if !ok {
		return fmt.Errorf("collection %q not found", collection)
	}
	if meta.Attributes == nil {
		meta.Attributes = make(map[string]string)
	}
	now := uint64(time.Now().Unix())
	if meta.Created == 0 {
		meta.Created = now
	}
	meta.Modified = now
	c.Items[uuid] = meta
	c.Modified = now
	s.data.Collections[collection] = c
	return s.save()
}

// UpdateItem replaces the metadata for an existing item.
func (s *Store) UpdateItem(collection, uuid string, meta ItemMeta) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.data.Collections[collection]
	if !ok {
		return fmt.Errorf("collection %q not found", collection)
	}
	if _, ok := c.Items[uuid]; !ok {
		return fmt.Errorf("item %q not found in collection %q", uuid, collection)
	}
	meta.Modified = uint64(time.Now().Unix())
	c.Items[uuid] = meta
	c.Modified = meta.Modified
	s.data.Collections[collection] = c
	return s.save()
}

// DeleteItem removes an item from a collection.
func (s *Store) DeleteItem(collection, uuid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.data.Collections[collection]
	if !ok {
		return fmt.Errorf("collection %q not found", collection)
	}
	if _, ok := c.Items[uuid]; !ok {
		return fmt.Errorf("item %q not found in collection %q", uuid, collection)
	}
	delete(c.Items, uuid)
	c.Modified = uint64(time.Now().Unix())
	s.data.Collections[collection] = c
	return s.save()
}

// SearchItems finds all items whose attributes are a superset of attrs.
// An empty attrs map matches all items.
func (s *Store) SearchItems(attrs map[string]string) []ItemRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var results []ItemRef
	for colName, col := range s.data.Collections {
		for uuid, item := range col.Items {
			if matchesAll(item.Attributes, attrs) {
				results = append(results, ItemRef{Collection: colName, UUID: uuid})
			}
		}
	}
	return results
}

// SearchItemsInCollection finds items within a specific collection matching attrs.
func (s *Store) SearchItemsInCollection(collection string, attrs map[string]string) []ItemRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	col, ok := s.data.Collections[collection]
	if !ok {
		return nil
	}
	var results []ItemRef
	for uuid, item := range col.Items {
		if matchesAll(item.Attributes, attrs) {
			results = append(results, ItemRef{Collection: collection, UUID: uuid})
		}
	}
	return results
}

// matchesAll returns true if itemAttrs contains all key/value pairs in want.
func matchesAll(itemAttrs, want map[string]string) bool {
	for k, v := range want {
		if itemAttrs[k] != v {
			return false
		}
	}
	return true
}

// --- Aliases ---

// GetAlias resolves an alias to a collection name, or "" if not found.
func (s *Store) GetAlias(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data.Aliases[name]
}

// SetAlias maps an alias name to a collection name.
// Pass collection="" to remove the alias.
func (s *Store) SetAlias(name, collection string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if collection == "" {
		delete(s.data.Aliases, name)
	} else {
		if _, ok := s.data.Collections[collection]; !ok {
			return fmt.Errorf("collection %q not found", collection)
		}
		s.data.Aliases[name] = collection
	}
	return s.save()
}
