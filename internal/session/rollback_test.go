package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestMutatorRollsBackOnSaveFailure verifies that when a tree mutator's save
// fails, the in-memory model is rolled back to match disk rather than keeping
// the un-persisted change (#109). A regular file planted at the slug of the
// folder being added forces storage.Save to fail mid-tree; because Save is
// atomic, disk is untouched, and the session reloads to that last-good state.
func TestMutatorRollsBackOnSaveFailure(t *testing.T) {
	c := model.Collection{
		Name:     "Demo",
		Requests: []model.Request{{Name: "Health", Method: model.GET, URL: "https://x/health"}},
	}
	dir := filepath.Join(t.TempDir(), "col")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	s, _ := New(filepath.Join(t.TempDir(), "config.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}

	// Plant a regular file at "reports" so creating the "Reports" folder dir
	// fails when the mutator tries to persist.
	if err := os.WriteFile(filepath.Join(dir, "reports"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := s.AddFolder("0", "Reports"); err == nil {
		t.Fatal("expected AddFolder to fail when save cannot create the folder dir")
	}

	// In-memory must match disk: the un-persisted folder is gone.
	if cols := s.Collections(); len(cols) != 1 || len(cols[0].Folders) != 0 {
		t.Errorf("in-memory state diverged from disk after failed save: %+v", cols)
	}
	if len(s.Collections()[0].Requests) != 1 {
		t.Errorf("rollback lost the original request: %+v", s.Collections()[0].Requests)
	}

	// Disk must still load cleanly as the original single-request collection.
	got, err := storage.Load(dir)
	if err != nil {
		t.Fatalf("Load after failed save: %v", err)
	}
	if len(got.Folders) != 0 || len(got.Requests) != 1 {
		t.Errorf("on-disk collection mutated by a failed save: %+v", got)
	}
}
