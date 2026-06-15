package session

import (
	"path/filepath"
	"testing"
)

// TestOpenCollectionDeduplicates verifies opening an already-open directory
// re-activates the existing entry instead of loading a second racing copy (#106).
func TestOpenCollectionDeduplicates(t *testing.T) {
	dir := writeSampleCollection(t)
	s, err := New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection 1: %v", err)
	}
	n := len(s.dirs)
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection 2: %v", err)
	}
	if len(s.dirs) != n {
		t.Errorf("re-open duplicated the collection: dirs = %v", s.dirs)
	}
	if s.activeCol < 0 || s.dirs[s.activeCol] != dir {
		t.Errorf("re-open did not re-activate the existing entry (activeCol=%d, dirs=%v)", s.activeCol, s.dirs)
	}
}

// TestWorkspaceNameUniqueness verifies duplicate workspace names are rejected
// on add and rename (the dropdown selects by name), while renaming to the same
// name is allowed (#98).
func TestWorkspaceNameUniqueness(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "config.yml"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.AddWorkspace("Prod"); err != nil {
		t.Fatalf("AddWorkspace Prod: %v", err)
	}
	if err := s.AddWorkspace("Prod"); err == nil {
		t.Error("AddWorkspace allowed a duplicate name")
	}
	if err := s.AddWorkspace("  Prod  "); err == nil {
		t.Error("AddWorkspace allowed a whitespace-only-different duplicate")
	}
	if err := s.AddWorkspace("Dev"); err != nil {
		t.Fatalf("AddWorkspace Dev: %v", err)
	}
	// Find Dev's index and try to rename it onto Prod (reject) and onto itself (allow).
	names := s.WorkspaceNames()
	devIdx := -1
	for i, n := range names {
		if n == "Dev" {
			devIdx = i
		}
	}
	if devIdx < 0 {
		t.Fatalf("Dev not found in %v", names)
	}
	if err := s.RenameWorkspace(devIdx, "Prod"); err == nil {
		t.Error("RenameWorkspace allowed colliding with an existing name")
	}
	if err := s.RenameWorkspace(devIdx, "Dev"); err != nil {
		t.Errorf("RenameWorkspace to its own name was rejected: %v", err)
	}
}

// TestSetActiveEnvNoActiveCollection verifies SetActiveEnv with no active
// collection (activeCol == -1) does not insert a spurious -1 key into the
// int-keyed env map (regression for #19).
func TestSetActiveEnvNoActiveCollection(t *testing.T) {
	s, err := New("")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.activeCol != -1 {
		t.Fatalf("precondition: activeCol = %d, want -1", s.activeCol)
	}
	s.SetActiveEnv("dev")
	if _, ok := s.activeEnv[-1]; ok {
		t.Errorf("SetActiveEnv inserted a -1 key: %v", s.activeEnv)
	}
	if len(s.activeEnv) != 0 {
		t.Errorf("activeEnv = %v, want empty", s.activeEnv)
	}
}

// TestParseLeafRejectsUnknownKind verifies parseLeaf returns ok=false for an id
// whose leaf kind byte is not c/f/r, while still accepting valid kinds
// (regression for #20).
func TestParseLeafRejectsUnknownKind(t *testing.T) {
	for _, id := range []string{"0/x5", "0/z0", "1/2/q3", "0/ 1"} {
		if _, _, _, ok := parseLeaf(id); ok {
			t.Errorf("parseLeaf(%q) ok=true, want false (unknown kind)", id)
		}
	}
	for _, id := range []string{"0", "0/c1", "0/f2", "0/r3", "1/2/r0"} {
		if _, _, _, ok := parseLeaf(id); !ok {
			t.Errorf("parseLeaf(%q) ok=false, want true (valid kind)", id)
		}
	}
}
