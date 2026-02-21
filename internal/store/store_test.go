// SPDX-License-Identifier: Apache-2.0

package store

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return s
}

func TestNewCreatesLoginCollection(t *testing.T) {
	s := newTestStore(t)

	col, ok := s.GetCollection("login")
	if !ok {
		t.Fatal("expected 'login' collection to exist after New")
	}
	if col.Label != "Login" {
		t.Errorf("label = %q, want %q", col.Label, "Login")
	}
	if s.GetAlias("default") != "login" {
		t.Errorf("alias 'default' should map to 'login'")
	}
}

func TestPersistenceAcrossReloads(t *testing.T) {
	dir := t.TempDir()
	s1, _ := New(dir)
	_ = s1.CreateCollection("work", "Work Secrets")

	// Reload from the same directory.
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := s2.GetCollection("work"); !ok {
		t.Error("collection 'work' not found after reload")
	}
}

func TestCreateAndDeleteCollection(t *testing.T) {
	s := newTestStore(t)

	if err := s.CreateCollection("test", "Test"); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if _, ok := s.GetCollection("test"); !ok {
		t.Fatal("collection not found after create")
	}

	if err := s.DeleteCollection("test"); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}
	if _, ok := s.GetCollection("test"); ok {
		t.Fatal("collection still exists after delete")
	}
}

func TestCreateDuplicateCollectionErrors(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateCollection("dup", "Dup")
	if err := s.CreateCollection("dup", "Dup2"); err == nil {
		t.Fatal("expected error creating duplicate collection")
	}
}

func TestItemCRUD(t *testing.T) {
	s := newTestStore(t)

	meta := ItemMeta{
		Label:       "GitHub Token",
		Attributes:  map[string]string{"service": "github.com", "username": "alice"},
		ContentType: "text/plain; charset=utf8",
	}
	if err := s.CreateItem("login", "uuid-1", meta); err != nil {
		t.Fatalf("CreateItem: %v", err)
	}

	got, ok := s.GetItem("login", "uuid-1")
	if !ok {
		t.Fatal("item not found after create")
	}
	if got.Label != "GitHub Token" {
		t.Errorf("label = %q", got.Label)
	}
	if got.Attributes["service"] != "github.com" {
		t.Errorf("attribute service = %q", got.Attributes["service"])
	}
	if got.Created == 0 {
		t.Error("Created should be set")
	}

	// Update.
	meta.Label = "Updated"
	if err := s.UpdateItem("login", "uuid-1", meta); err != nil {
		t.Fatalf("UpdateItem: %v", err)
	}
	got, _ = s.GetItem("login", "uuid-1")
	if got.Label != "Updated" {
		t.Errorf("updated label = %q, want %q", got.Label, "Updated")
	}

	// Delete.
	if err := s.DeleteItem("login", "uuid-1"); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	if _, ok := s.GetItem("login", "uuid-1"); ok {
		t.Fatal("item still exists after delete")
	}
}

func TestSearchItems(t *testing.T) {
	s := newTestStore(t)

	items := []struct {
		uuid  string
		attrs map[string]string
	}{
		{"u1", map[string]string{"service": "example.com", "user": "alice"}},
		{"u2", map[string]string{"service": "example.com", "user": "bob"}},
		{"u3", map[string]string{"service": "other.com", "user": "alice"}},
	}
	for _, it := range items {
		_ = s.CreateItem("login", it.uuid, ItemMeta{Attributes: it.attrs})
	}

	// Search by single attribute.
	refs := s.SearchItems(map[string]string{"service": "example.com"})
	if len(refs) != 2 {
		t.Errorf("search by service: got %d results, want 2", len(refs))
	}

	// Search by two attributes.
	refs = s.SearchItems(map[string]string{"service": "example.com", "user": "alice"})
	if len(refs) != 1 || refs[0].UUID != "u1" {
		t.Errorf("search by service+user: got %v, want [{login u1}]", refs)
	}

	// Empty search matches all.
	refs = s.SearchItems(map[string]string{})
	if len(refs) != 3 {
		t.Errorf("empty search: got %d results, want 3", len(refs))
	}
}

func TestSearchItemsInCollection(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateCollection("other", "Other")

	_ = s.CreateItem("login", "u1", ItemMeta{Attributes: map[string]string{"svc": "a"}})
	_ = s.CreateItem("other", "u2", ItemMeta{Attributes: map[string]string{"svc": "a"}})

	refs := s.SearchItemsInCollection("login", map[string]string{"svc": "a"})
	if len(refs) != 1 || refs[0].UUID != "u1" {
		t.Errorf("got %v, want login/u1 only", refs)
	}
}

func TestAliases(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateCollection("work", "Work")

	if err := s.SetAlias("primary", "work"); err != nil {
		t.Fatalf("SetAlias: %v", err)
	}
	if got := s.GetAlias("primary"); got != "work" {
		t.Errorf("alias = %q, want %q", got, "work")
	}

	// Remove alias.
	if err := s.SetAlias("primary", ""); err != nil {
		t.Fatalf("remove alias: %v", err)
	}
	if got := s.GetAlias("primary"); got != "" {
		t.Errorf("alias after removal = %q, want empty", got)
	}
}

func TestAtomicSave(t *testing.T) {
	s := newTestStore(t)
	_ = s.CreateCollection("col", "Col")

	// No .tmp file should remain after save.
	tmpPath := filepath.Join(filepath.Dir(s.path), "metadata.json.tmp")
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error(".tmp file was left behind after atomic save")
	}
}
