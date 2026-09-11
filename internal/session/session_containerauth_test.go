package session

import (
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestInheritedAuthIgnoresOwnAuth verifies InheritedAuth reports what the
// ancestors would supply even when the node has concrete auth of its own —
// the distinction that makes it the right source for the Inherit preview
// (EffectiveAuth would return the node's own API-Key here).
func TestInheritedAuthIgnoresOwnAuth(t *testing.T) {
	s := newSessionWith(t, writeAuthSampleCollection(t))
	got := s.InheritedAuth("0/f0/r1") // Users/Delete User (own: API key)
	if got.Type != model.AuthBasic || got.Basic == nil || got.Basic.Username != "alice" {
		t.Errorf("InheritedAuth = %+v, want the folder's Basic", got)
	}
	if eff := s.EffectiveAuth("0/f0/r1"); eff.Type != model.AuthAPIKey {
		t.Errorf("EffectiveAuth = %+v, want the request's own API key", eff)
	}
}

// TestInheritedAuthForFolderAndRoot covers the container cases: a folder
// inherits from the collection root, a root request too, and the collection
// root itself (no parent) plus an unknown node fall back to None.
func TestInheritedAuthForFolderAndRoot(t *testing.T) {
	s := newSessionWith(t, writeAuthSampleCollection(t))
	if got := s.InheritedAuth("0/f0"); got.Type != model.AuthBearer || got.Bearer == nil || got.Bearer.Token != "root-token" {
		t.Errorf("InheritedAuth(folder) = %+v, want collection Bearer", got)
	}
	if got := s.InheritedAuth("0/r0"); got.Type != model.AuthBearer {
		t.Errorf("InheritedAuth(root request) = %+v, want collection Bearer", got)
	}
	if got := s.InheritedAuth("0"); got.Type != model.AuthNone {
		t.Errorf("InheritedAuth(collection root) = %+v, want None", got)
	}
	if got := s.InheritedAuth("9/f9"); got.Type != model.AuthNone {
		t.Errorf("InheritedAuth(unknown) = %+v, want None", got)
	}
}

// TestFolderAuthReadsOwnValue verifies FolderAuth returns the folder's own
// (unresolved) auth and rejects non-folder node IDs.
func TestFolderAuthReadsOwnValue(t *testing.T) {
	s := newSessionWith(t, writeAuthSampleCollection(t))
	a, ok := s.FolderAuth("0/f0")
	if !ok || a.Type != model.AuthBasic {
		t.Errorf("FolderAuth(0/f0) = %+v, %v; want Basic, true", a, ok)
	}
	if _, ok := s.FolderAuth("0/f0/r0"); ok {
		t.Error("FolderAuth on a request node should be false")
	}
	if _, ok := s.FolderAuth("0"); ok {
		t.Error("FolderAuth on the collection root should be false")
	}
	if _, ok := s.FolderAuth(""); ok {
		t.Error("FolderAuth on an empty id should be false")
	}
}

// TestUpdateFolderPersistsAuthAndVariables verifies one UpdateFolder call
// changes several fields, writes them to disk, and that descendants see the
// new auth through the inheritance walk on a fresh load.
func TestUpdateFolderPersistsAuthAndVariables(t *testing.T) {
	dir := writeAuthSampleCollection(t)
	s := newSessionWith(t, dir)
	err := s.UpdateFolder("0/f0", func(f *model.Folder) {
		f.Auth = model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "folder-token"}}
		f.Variables = []model.Variable{{Enabled: true, Key: "fv", Value: "1"}}
	})
	if err != nil {
		t.Fatalf("UpdateFolder: %v", err)
	}
	if got := s.InheritedAuth("0/f0/r0"); got.Type != model.AuthBearer || got.Bearer.Token != "folder-token" {
		t.Errorf("in-memory InheritedAuth after update = %+v", got)
	}

	c, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := c.Folders[0]
	if f.Auth.Type != model.AuthBearer || f.Auth.Bearer == nil || f.Auth.Bearer.Token != "folder-token" {
		t.Errorf("folder auth not persisted: %+v", f.Auth)
	}
	if len(f.Variables) != 1 || f.Variables[0].Key != "fv" {
		t.Errorf("folder variables not persisted: %+v", f.Variables)
	}
	fresh := newSessionWith(t, dir)
	if got := fresh.EffectiveAuth("0/f0/r0"); got.Type != model.AuthBearer || got.Bearer.Token != "folder-token" {
		t.Errorf("reloaded EffectiveAuth = %+v, want the new folder Bearer", got)
	}
}

// TestUpdateFolderRejectsNonFolder verifies the mutate callback never runs for
// a request node, the collection root, or garbage.
func TestUpdateFolderRejectsNonFolder(t *testing.T) {
	s := newSessionWith(t, writeAuthSampleCollection(t))
	for _, id := range []string{"0/f0/r0", "0", "", "nope"} {
		called := false
		if err := s.UpdateFolder(id, func(*model.Folder) { called = true }); err == nil {
			t.Errorf("UpdateFolder(%q) = nil error, want failure", id)
		}
		if called {
			t.Errorf("UpdateFolder(%q) invoked mutate", id)
		}
	}
}

// TestUpdateCollectionPersistsRootAuth verifies the collection-root
// counterpart writes the root auth (and variables) and that top-level
// requests inherit it after a reload.
func TestUpdateCollectionPersistsRootAuth(t *testing.T) {
	dir := writeAuthSampleCollection(t)
	s := newSessionWith(t, dir)
	err := s.UpdateCollection(0, func(c *model.Collection) {
		c.Auth = model.Auth{Type: model.AuthAPIKey, APIKey: &model.APIKeyAuth{Name: "X-K", Value: "v", Placement: model.APIKeyHeader}}
		c.Variables = []model.Variable{{Enabled: true, Key: "cv", Value: "1"}}
	})
	if err != nil {
		t.Fatalf("UpdateCollection: %v", err)
	}
	c, err := storage.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if c.Auth.Type != model.AuthAPIKey || c.Auth.APIKey == nil || c.Auth.APIKey.Name != "X-K" {
		t.Errorf("root auth not persisted: %+v", c.Auth)
	}
	if len(c.Variables) != 1 || c.Variables[0].Key != "cv" {
		t.Errorf("collection variables not persisted: %+v", c.Variables)
	}
	fresh := newSessionWith(t, dir)
	if got := fresh.EffectiveAuth("0/r0"); got.Type != model.AuthAPIKey {
		t.Errorf("reloaded EffectiveAuth(root request) = %+v, want API key", got)
	}
}

// TestUpdateCollectionRejectsBadIndex verifies out-of-range indices error
// without invoking mutate.
func TestUpdateCollectionRejectsBadIndex(t *testing.T) {
	s := newSessionWith(t, writeAuthSampleCollection(t))
	for _, ci := range []int{-1, 1, 42} {
		called := false
		if err := s.UpdateCollection(ci, func(*model.Collection) { called = true }); err == nil {
			t.Errorf("UpdateCollection(%d) = nil error, want failure", ci)
		}
		if called {
			t.Errorf("UpdateCollection(%d) invoked mutate", ci)
		}
	}
}
