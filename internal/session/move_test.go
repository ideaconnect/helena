package session

import (
	"path/filepath"
	"testing"
)

// reqName returns the request name at nodeID or "" — a terse test helper.
func reqName(s *Session, nodeID string) string {
	if r, ok := s.Tree().Request(nodeID); ok {
		return r.Name
	}
	return ""
}

// TestMoveRequestIntoFolder moves a root request into a folder and checks it
// lands at the requested slot and leaves the root behind.
func TestMoveRequestIntoFolder(t *testing.T) {
	s := openSessionWithCollection(t) // root req Health (0/r0); folder Users (0/f0) with Create User (0/f0/r0)

	newID, err := s.MoveNode("0/r0", "0/f0", 0)
	if err != nil {
		t.Fatalf("MoveNode: %v", err)
	}
	if newID != "0/f0/r0" {
		t.Errorf("new id = %q, want 0/f0/r0", newID)
	}
	if got := reqName(s, "0/f0/r0"); got != "Health" {
		t.Errorf("0/f0/r0 = %q, want Health", got)
	}
	if got := reqName(s, "0/f0/r1"); got != "Create User" {
		t.Errorf("0/f0/r1 = %q, want Create User (shifted down)", got)
	}
	if _, ok := s.Tree().Request("0/r0"); ok {
		t.Error("root request should be gone after move")
	}
}

// TestMoveRequestReorder reorders requests within the same container, covering
// the same-container index adjustment.
func TestMoveRequestReorder(t *testing.T) {
	s := openSessionWithCollection(t)
	// Root requests: Health(r0), A(r1), B(r2).
	if _, err := s.AddRequest("0", "A"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddRequest("0", "B"); err != nil {
		t.Fatal(err)
	}

	// Move Health to the end (index == len).
	newID, err := s.MoveNode("0/r0", "0", 3)
	if err != nil {
		t.Fatalf("MoveNode: %v", err)
	}
	if newID != "0/r2" {
		t.Errorf("new id = %q, want 0/r2", newID)
	}
	for idx, want := range map[string]string{"0/r0": "A", "0/r1": "B", "0/r2": "Health"} {
		if got := reqName(s, idx); got != want {
			t.Errorf("%s = %q, want %q", idx, got, want)
		}
	}
}

// TestMoveFolderIntoFolder moves a whole folder subtree into another folder.
func TestMoveFolderIntoFolder(t *testing.T) {
	s := openSessionWithCollection(t)
	fid, err := s.AddFolder("0", "Auth") // 0/f1
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddRequest(fid, "Login"); err != nil { // 0/f1/r0
		t.Fatal(err)
	}

	newID, err := s.MoveNode("0/f1", "0/f0", 0) // Auth into Users
	if err != nil {
		t.Fatalf("MoveNode: %v", err)
	}
	if newID != "0/f0/f0" {
		t.Errorf("new id = %q, want 0/f0/f0", newID)
	}
	if got := reqName(s, "0/f0/f0/r0"); got != "Login" {
		t.Errorf("nested request after folder move = %q, want Login", got)
	}
	if s.Tree().IsBranch("0/f1") && s.Tree().Label("0/f1") == "Auth" {
		t.Error("Auth should no longer be a root folder")
	}
}

// TestMoveRejectsCrossCollection ensures a move across collections fails.
func TestMoveRejectsCrossCollection(t *testing.T) {
	s := openSessionWithCollection(t)
	if err := s.OpenCollection(writeSampleCollection(t)); err != nil { // collection 1
		t.Fatal(err)
	}
	if _, err := s.MoveNode("1/r0", "0", 0); err == nil {
		t.Error("expected cross-collection move to be rejected")
	}
}

// TestMoveRejectsFolderIntoItself guards the self/descendant drop.
func TestMoveRejectsFolderIntoItself(t *testing.T) {
	s := openSessionWithCollection(t)
	if _, err := s.MoveNode("0/f0", "0/f0", 0); err == nil {
		t.Error("expected folder-into-itself to be rejected")
	}
	if _, err := s.AddFolder("0/f0", "Sub"); err != nil { // 0/f0/f0
		t.Fatal(err)
	}
	if _, err := s.MoveNode("0/f0", "0/f0/f0", 0); err == nil {
		t.Error("expected folder-into-descendant to be rejected")
	}
}

// TestMoveCollectionReorder reorders the top-level collection list and checks
// the active-collection pointer follows its collection.
func TestMoveCollectionReorder(t *testing.T) {
	s := openSessionWithCollection(t) // collection 0
	d0 := s.CollectionDir(0)
	if err := s.OpenCollection(writeSampleCollection(t)); err != nil { // 1
		t.Fatal(err)
	}
	d1 := s.CollectionDir(1)
	if err := s.OpenCollection(writeSampleCollection(t)); err != nil { // 2
		t.Fatal(err)
	}
	d2 := s.CollectionDir(2)

	s.SetActiveCollection(0) // active = d0
	if err := s.MoveCollection(0, 2); err != nil {
		t.Fatalf("MoveCollection: %v", err)
	}
	// from=0, final index 2: remove d0 → [d1,d2], insert at 2 → [d1, d2, d0].
	if s.CollectionDir(0) != d1 || s.CollectionDir(1) != d2 || s.CollectionDir(2) != d0 {
		t.Errorf("order = [%s %s %s]", s.CollectionDir(0), s.CollectionDir(1), s.CollectionDir(2))
	}
	if s.ActiveCollection() != 2 {
		t.Errorf("active collection = %d, want 2 (followed d0)", s.ActiveCollection())
	}
}

// TestMoveCollectionPersists verifies the reorder survives a reload from the
// same config path.
func TestMoveCollectionPersists(t *testing.T) {
	cfg := filepath.Join(t.TempDir(), "config.yml")
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.OpenCollection(writeSampleCollection(t)); err != nil {
		t.Fatal(err)
	}
	if err := s.OpenCollection(writeSampleCollection(t)); err != nil {
		t.Fatal(err)
	}
	d0, d1 := s.CollectionDir(0), s.CollectionDir(1)
	if err := s.MoveCollection(0, 1); err != nil {
		t.Fatalf("MoveCollection: %v", err)
	}

	s2, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if s2.CollectionDir(0) != d1 || s2.CollectionDir(1) != d0 {
		t.Errorf("reloaded order = [%s %s], want [%s %s]", s2.CollectionDir(0), s2.CollectionDir(1), d1, d0)
	}
}
