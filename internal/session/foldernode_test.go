package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestFolderNodeID checks name-path → folder-node-id resolution, the inverse of
// nodeNamePath for folders (#89).
func TestFolderNodeID(t *testing.T) {
	col := model.Collection{
		Name: "API",
		Folders: []model.Folder{
			{Name: "Auth", Folders: []model.Folder{{Name: "OAuth"}}},
			{Name: "Users"},
		},
	}
	dir := filepath.Join(t.TempDir(), "api")
	if err := storage.Save(col, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}

	cases := []struct {
		path   string
		wantID string
		wantOK bool
	}{
		{"", "0", true},                 // empty → collection root
		{"Auth", "0/f0", true},          // top-level folder
		{"Auth/OAuth", "0/f0/f0", true}, // nested folder
		{"Users", "0/f1", true},         // second sibling
		{"/Auth/", "0/f0", true},        // leading/trailing slashes ignored
		{"Nope", "", false},             // no such folder
		{"Auth/Nope", "", false},        // no such nested folder
	}
	for _, c := range cases {
		id, ok := s.FolderNodeID(0, c.path)
		if id != c.wantID || ok != c.wantOK {
			t.Errorf("FolderNodeID(0, %q) = %q,%v; want %q,%v", c.path, id, ok, c.wantID, c.wantOK)
		}
	}

	// Out-of-range collection index.
	if id, ok := s.FolderNodeID(5, "Auth"); ok || id != "" {
		t.Errorf("FolderNodeID(5, Auth) = %q,%v; want \"\",false", id, ok)
	}
}
