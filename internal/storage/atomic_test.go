package storage

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
)

// readTree walks dir and returns a map of relative-path -> file content for
// every regular file, so a before/after snapshot can prove a failed Save left
// the directory untouched.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(dir, path)
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatalf("readTree(%s): %v", dir, err)
	}
	return out
}

// TestSaveAtomicLeavesDirUntouchedOnFailure verifies that when a re-save fails
// part-way through writing the tree, the original on-disk collection is left
// exactly as it was — no half-written state (#109). The failure is induced
// deterministically by planting a regular file where the next save needs to
// create a folder directory.
func TestSaveAtomicLeavesDirUntouchedOnFailure(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "col")
	v1 := model.Collection{
		Name:     "Demo",
		Requests: []model.Request{{Name: "Health", Method: model.GET, URL: "https://x/health"}},
		Folders:  []model.Folder{{Name: "Users", Requests: []model.Request{{Name: "Create", Method: model.POST, URL: "https://x/u"}}}},
	}
	if err := Save(v1, dir); err != nil {
		t.Fatalf("Save v1: %v", err)
	}

	// Plant a regular file named "reports" — the slug of a folder the next
	// save will try to create as a directory, forcing a mid-tree MkdirAll error.
	if err := os.WriteFile(filepath.Join(dir, "reports"), []byte("blocker"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := readTree(t, dir)

	v2 := v1
	v2.Folders = append(append([]model.Folder{}, v1.Folders...), model.Folder{Name: "Reports"})
	v2.Requests = append(append([]model.Request{}, v1.Requests...), model.Request{Name: "New", Method: model.GET, URL: "https://x/new"})
	if err := Save(v2, dir); err == nil {
		t.Fatal("expected Save v2 to fail (folder dir collides with a regular file)")
	}

	after := readTree(t, dir)
	if len(after) != len(before) {
		t.Errorf("dir file count changed: before=%d after=%d", len(before), len(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Errorf("file %q changed by a failed save", k)
		}
	}
	// The new request must not have leaked onto disk.
	if _, err := os.Stat(filepath.Join(dir, "new.yml")); err == nil {
		t.Error("a failed save leaked the new request file onto disk")
	}
	// No staging artifacts must survive a failed save.
	for _, suffix := range []string{".helena-save", ".helena-old"} {
		if _, err := os.Stat(dir + suffix); err == nil {
			t.Errorf("staging artifact %q survived a failed save", dir+suffix)
		}
	}

	// The original collection must still load cleanly.
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load after failed save: %v", err)
	}
	if len(got.Folders) != 1 || len(got.Requests) != 1 {
		t.Errorf("collection mutated by a failed save: %+v", got)
	}
}

// TestSaveStagingFailureSurfaces verifies a failure seeding the staging dir
// (here: the parent path is a regular file, so the staging dir cannot be
// created) surfaces as an error and writes nothing.
func TestSaveStagingFailureSurfaces(t *testing.T) {
	root := t.TempDir()
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Save(model.Collection{Name: "X"}, filepath.Join(blocker, "col")); err == nil {
		t.Error("expected error when the staging parent is a regular file")
	}
}

// TestCopyTreeErrorOnUncreatableTarget covers copyTree's error path when the
// destination cannot be created (its parent is a regular file).
func TestCopyTreeErrorOnUncreatableTarget(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "a.yml"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	blocker := filepath.Join(root, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := copyTree(src, filepath.Join(blocker, "dst")); err == nil {
		t.Error("expected copyTree to fail when the destination parent is a file")
	}
}
