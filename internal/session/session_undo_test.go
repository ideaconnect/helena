package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// openUndoSession saves col to a temp dir and returns an active session over it.
func openUndoSession(t *testing.T, col model.Collection) (*Session, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "c")
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}
	s, err := New("")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)
	return s, dir
}

// TestUndoRestoresRequestAndFile pins #68: deleting a request removes it from
// the model and disk, and RestoreLastDeleted brings it back at its original
// position — with its persistent ID intact — both in memory and on disk.
func TestUndoRestoresRequestAndFile(t *testing.T) {
	s, dir := openUndoSession(t, model.Collection{
		Name: "C",
		Requests: []model.Request{
			{ID: "id-alpha", Name: "Alpha", Method: model.GET, URL: "https://a/"},
			{ID: "id-beta", Name: "Beta", Method: model.POST, URL: "https://b/"},
		},
	})

	if s.CanUndoDelete() {
		t.Fatal("CanUndoDelete true before any delete")
	}

	if err := s.DeleteItem("0/r0"); err != nil {
		t.Fatalf("DeleteItem: %v", err)
	}
	if !s.CanUndoDelete() || s.LastDeletedName() != "Alpha" {
		t.Errorf("after delete: CanUndo=%v name=%q", s.CanUndoDelete(), s.LastDeletedName())
	}
	if got, _ := storage.Load(dir); len(got.Requests) != 1 || got.Requests[0].Name != "Beta" {
		t.Fatalf("on-disk after delete = %+v, want only Beta", reqNames(got.Requests))
	}

	newID, err := s.RestoreLastDeleted()
	if err != nil {
		t.Fatalf("RestoreLastDeleted: %v", err)
	}
	if newID != "0/r0" {
		t.Errorf("restored node id = %q, want 0/r0", newID)
	}
	got, _ := storage.Load(dir)
	if len(got.Requests) != 2 || got.Requests[0].Name != "Alpha" || got.Requests[1].Name != "Beta" {
		t.Fatalf("on-disk after restore = %v, want [Alpha Beta]", reqNames(got.Requests))
	}
	if got.Requests[0].ID != "id-alpha" {
		t.Errorf("restored request ID = %q, want id-alpha preserved (chain-ref safety)", got.Requests[0].ID)
	}
	if s.CanUndoDelete() {
		t.Error("undo state not cleared after a successful restore")
	}
	if _, err := s.RestoreLastDeleted(); err == nil {
		t.Error("second RestoreLastDeleted should error (nothing to undo)")
	}
}

// TestUndoRestoresFolderSubtree verifies a deleted folder (with nested content)
// is restored whole.
func TestUndoRestoresFolderSubtree(t *testing.T) {
	s, dir := openUndoSession(t, model.Collection{
		Name: "C",
		Folders: []model.Folder{{
			Name:     "Admin",
			Requests: []model.Request{{ID: "id-ban", Name: "Ban", Method: model.POST, URL: "https://x/"}},
		}},
	})

	if err := s.DeleteItem("0/f0"); err != nil {
		t.Fatalf("DeleteItem folder: %v", err)
	}
	if got, _ := storage.Load(dir); len(got.Folders) != 0 {
		t.Fatalf("folder not deleted on disk: %+v", got.Folders)
	}

	if _, err := s.RestoreLastDeleted(); err != nil {
		t.Fatalf("RestoreLastDeleted: %v", err)
	}
	got, _ := storage.Load(dir)
	if len(got.Folders) != 1 || got.Folders[0].Name != "Admin" ||
		len(got.Folders[0].Requests) != 1 || got.Folders[0].Requests[0].Name != "Ban" {
		t.Fatalf("folder subtree not restored: %+v", got.Folders)
	}
}

func reqNames(rs []model.Request) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.Name
	}
	return out
}
