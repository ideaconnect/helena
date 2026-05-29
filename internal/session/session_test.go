package session

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// writeSampleCollection saves a small collection (one root request "Health",
// one folder "Users" with "Create User") to a fresh temp dir and returns it.
func writeSampleCollection(t *testing.T) string {
	t.Helper()
	c := model.Collection{
		Name:     "Demo API",
		Requests: []model.Request{{Name: "Health", Method: model.GET, URL: "https://{{base}}/health"}},
		Folders: []model.Folder{{
			Name:     "Users",
			Requests: []model.Request{{Name: "Create User", Method: model.POST, URL: "https://{{base}}/users"}},
		}},
	}
	dir := filepath.Join(t.TempDir(), "demo-api")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save sample: %v", err)
	}
	return dir
}

// TestOpenCollectionPersistsAndReloads verifies that OpenCollection records the
// directory in the active workspace and a fresh Session at the same config path
// re-loads the same collection.
func TestOpenCollectionPersistsAndReloads(t *testing.T) {
	colDir := writeSampleCollection(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yml")

	s, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(s.Collections()) != 0 {
		t.Fatalf("fresh session has %d collections, want 0", len(s.Collections()))
	}
	if err := s.OpenCollection(colDir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	if len(s.Collections()) != 1 || s.Collections()[0].Name != "Demo API" {
		t.Fatalf("after open: %+v", s.Collections())
	}

	// A new session from the same config path must remember the collection.
	s2, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	if len(s2.Collections()) != 1 || s2.Collections()[0].Name != "Demo API" {
		t.Errorf("reloaded session lost the collection: %+v", s2.Collections())
	}
}

// TestTreeNavigation verifies the Tree node ID layout: root produces "0",
// folders produce "0/f0", requests produce "0/r0", IsBranch/Label/Request
// behave consistently with these IDs.
func TestTreeNavigation(t *testing.T) {
	colDir := writeSampleCollection(t)
	s, _ := New(filepath.Join(t.TempDir(), "config.yml"))
	if err := s.OpenCollection(colDir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	tr := s.Tree()

	if got := tr.ChildIDs(""); !reflect.DeepEqual(got, []string{"0"}) {
		t.Errorf("root children = %v, want [0]", got)
	}
	if got := tr.ChildIDs("0"); !reflect.DeepEqual(got, []string{"0/f0", "0/r0"}) {
		t.Errorf("collection children = %v, want [0/f0 0/r0]", got)
	}
	if !tr.IsBranch("0/f0") || tr.IsBranch("0/r0") {
		t.Errorf("branch detection wrong for folder/request")
	}
	if got := tr.Label("0/r0"); got != "GET  Health" {
		t.Errorf("request label = %q, want %q", got, "GET  Health")
	}
	if got := tr.Label("0/f0"); got != "Users" {
		t.Errorf("folder label = %q, want Users", got)
	}

	if got := tr.ChildIDs("0/f0"); !reflect.DeepEqual(got, []string{"0/f0/r0"}) {
		t.Errorf("folder children = %v, want [0/f0/r0]", got)
	}
	r, ok := tr.Request("0/f0/r0")
	if !ok || r.Name != "Create User" || r.Method != model.POST {
		t.Errorf("nested request lookup = %+v ok=%v", r, ok)
	}
	if _, ok := tr.Request("0/f0"); ok {
		t.Errorf("folder id should not resolve to a request")
	}
}
