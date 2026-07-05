package session

import (
	"os"
	"path/filepath"
	"runtime"
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

// TestUndoRestoresIntoCorrectContainerAfterSiblingShift pins the stable-parent
// fix: after deleting a request inside folder B, an intervening op that shifts
// B's positional index (duplicating folder A) must not make undo restore the
// request into the folder that now occupies B's old slot.
func TestUndoRestoresIntoCorrectContainerAfterSiblingShift(t *testing.T) {
	s, _ := openUndoSession(t, model.Collection{
		Name: "C",
		Folders: []model.Folder{
			{ID: "fa", Name: "A"},
			{ID: "fb", Name: "B", Requests: []model.Request{
				{ID: "rr", Name: "R", Method: model.GET, URL: "https://r/"},
			}},
		},
	})

	if err := s.DeleteItem("0/f1/r0"); err != nil { // delete R from folder B
		t.Fatalf("DeleteItem: %v", err)
	}
	// Duplicate folder A → inserts a copy at 0/f1, shifting B to 0/f2.
	if _, err := s.DuplicateItem("0/f0"); err != nil {
		t.Fatalf("DuplicateItem: %v", err)
	}

	newID, err := s.RestoreLastDeleted()
	if err != nil {
		t.Fatalf("RestoreLastDeleted: %v", err)
	}
	if newID != "0/f2/r0" {
		t.Errorf("restored node id = %q, want 0/f2/r0 (folder B's new position)", newID)
	}
	folders := s.Collections()[0].Folders
	if len(folders) != 3 {
		t.Fatalf("folders = %d, want 3 (A, A-copy, B)", len(folders))
	}
	if len(folders[1].Requests) != 0 {
		t.Errorf("the A-copy at 0/f1 wrongly received the restored request: %+v", folders[1].Requests)
	}
	if len(folders[2].Requests) != 1 || folders[2].Requests[0].ID != "rr" {
		t.Errorf("folder B (0/f2) is missing the restored request: %+v", folders[2].Requests)
	}
}

// TestDeleteSaveFailureDisarmsUndo pins that a delete whose save fails (and thus
// reload()s the item back into memory) leaves no armed undo — otherwise a
// following restore would re-insert a duplicate with the same Request.ID.
func TestDeleteSaveFailureDisarmsUndo(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("a read-only directory does not block writes inside it on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("read-only-dir write failure does not apply when running as root")
	}
	parent := t.TempDir()
	dir := filepath.Join(parent, "c")
	if err := storage.Save(model.Collection{Name: "C", Requests: []model.Request{
		{ID: "id-a", Name: "A", Method: model.GET, URL: "https://a/"},
	}}, dir); err != nil {
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

	// Make the parent read-only so storage.Save's stage+swap (which writes a
	// sibling of dir) fails; persistCollection then reload()s the item back.
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(parent, 0o755) })

	if err := s.DeleteItem("0/r0"); err == nil {
		t.Fatal("expected DeleteItem to fail when its save cannot write")
	}
	if s.CanUndoDelete() {
		t.Error("undo left armed after a failed delete-save — a restore would duplicate the node ID")
	}
}
