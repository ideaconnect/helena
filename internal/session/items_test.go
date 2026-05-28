package session

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

func openSessionWithCollection(t *testing.T) *Session {
	t.Helper()
	dir := writeSampleCollection(t) // Has one root request "Health" and folder "Users" with "Create User"
	s, err := New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	return s
}

func TestAddRequest(t *testing.T) {
	s := openSessionWithCollection(t)
	id, err := s.AddRequest("0", "Probe")
	if err != nil {
		t.Fatalf("AddRequest: %v", err)
	}
	if id != "0/r1" {
		t.Errorf("new id = %q, want 0/r1", id)
	}
	r, ok := s.Tree().Request(id)
	if !ok || r.Name != "Probe" || r.Method != model.GET {
		t.Errorf("new request = %+v ok=%v", r, ok)
	}
}

func TestAddFolderAndNestedRequest(t *testing.T) {
	s := openSessionWithCollection(t)

	fid, err := s.AddFolder("0", "Auth")
	if err != nil {
		t.Fatalf("AddFolder: %v", err)
	}
	if fid != "0/f1" {
		t.Errorf("folder id = %q, want 0/f1", fid)
	}
	rid, err := s.AddRequest(fid, "Login")
	if err != nil {
		t.Fatalf("AddRequest (nested): %v", err)
	}
	if rid != "0/f1/r0" {
		t.Errorf("nested request id = %q, want 0/f1/r0", rid)
	}
	r, ok := s.Tree().Request(rid)
	if !ok || r.Name != "Login" {
		t.Errorf("nested request = %+v ok=%v", r, ok)
	}
}

func TestRenameItem(t *testing.T) {
	s := openSessionWithCollection(t)
	if err := s.RenameItem("0/r0", "Pong"); err != nil {
		t.Fatalf("RenameItem request: %v", err)
	}
	if r, _ := s.Tree().Request("0/r0"); r.Name != "Pong" {
		t.Errorf("after rename request name = %q", r.Name)
	}
	if err := s.RenameItem("0/f0", "Accounts"); err != nil {
		t.Fatalf("RenameItem folder: %v", err)
	}
	if got := s.Tree().Label("0/f0"); got != "Accounts" {
		t.Errorf("after folder rename label = %q", got)
	}
}

func TestDeleteItem(t *testing.T) {
	s := openSessionWithCollection(t)
	if err := s.DeleteItem("0/f0"); err != nil {
		t.Fatalf("DeleteItem folder: %v", err)
	}
	// After deleting the folder, only the root request remains.
	if got := s.Tree().ChildIDs("0"); len(got) != 1 || got[0] != "0/r0" {
		t.Errorf("after delete folder, children = %v, want [0/r0]", got)
	}
	if err := s.DeleteItem("0/r0"); err != nil {
		t.Fatalf("DeleteItem request: %v", err)
	}
	if got := s.Tree().ChildIDs("0"); len(got) != 0 {
		t.Errorf("after delete request, children = %v, want []", got)
	}
}

func TestDuplicateRequest(t *testing.T) {
	s := openSessionWithCollection(t)
	newID, err := s.DuplicateItem("0/r0")
	if err != nil {
		t.Fatalf("DuplicateItem: %v", err)
	}
	if newID != "0/r1" {
		t.Errorf("dup id = %q, want 0/r1", newID)
	}
	r, _ := s.Tree().Request(newID)
	if r == nil || !strings.Contains(r.Name, "(copy)") {
		t.Errorf("dup name = %+v, want suffix '(copy)'", r)
	}
}

func TestDuplicateFolderDeepCopies(t *testing.T) {
	s := openSessionWithCollection(t)
	newID, err := s.DuplicateItem("0/f0")
	if err != nil {
		t.Fatalf("DuplicateItem: %v", err)
	}
	if newID != "0/f1" {
		t.Errorf("dup folder id = %q, want 0/f1", newID)
	}
	// Edit the original folder's request name; the copy must be unaffected.
	if err := s.RenameItem("0/f0/r0", "Original changed"); err != nil {
		t.Fatalf("RenameItem on original: %v", err)
	}
	cp, _ := s.Tree().Request("0/f1/r0")
	if cp == nil || cp.Name == "Original changed" {
		t.Errorf("duplicate aliased original; copy.Name = %q", cp.Name)
	}
}
