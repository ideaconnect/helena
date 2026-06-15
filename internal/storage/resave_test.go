package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestSaveResaveMergesPriorFiles re-saves a collection that has environments and
// a nested folder. The second Save reads back the prior env/folder/request
// files to merge non-format fields, exercising readEnvFile / readFolderFile, the
// env write+sweep, and the body-merge branch that a single save never touches.
func TestSaveResaveMergesPriorFiles(t *testing.T) {
	dir := t.TempDir()
	c := sampleCollection()
	c.Environments = []model.Environment{{
		ID:   model.NewID(),
		Name: "Dev",
		Variables: []model.Variable{
			{Enabled: true, Key: "base", Value: "api.test"},
			{Enabled: false, Key: "debug", Value: "1"},
		},
	}}

	if err := Save(c, dir); err != nil {
		t.Fatalf("Save 1: %v", err)
	}
	// Re-save over the existing tree: triggers the prior-file read-back/merge.
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save 2: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Environments) != 1 || got.Environments[0].Name != "Dev" {
		t.Errorf("environments = %+v, want one named Dev", got.Environments)
	}
	if len(got.Folders) != 1 || len(got.Folders[0].Requests) != 1 {
		t.Errorf("folders round-trip = %+v", got.Folders)
	}
}

// TestLoadIgnoresNonCollectionSubdir verifies a stray subdirectory without a
// folder.yml is skipped (the "not an OpenCollection folder" continue branch in
// loadItems) rather than failing the load.
func TestLoadIgnoresNonCollectionSubdir(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, collectionFile), []byte("info:\n  name: T\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "not-a-folder"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "not-a-folder", "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load with stray subdir: %v", err)
	}
	if len(c.Folders) != 0 {
		t.Errorf("stray subdir was treated as a folder: %+v", c.Folders)
	}
}
