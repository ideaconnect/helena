package session

import (
	"path/filepath"
	"testing"
)

func TestAddRenameWorkspace(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	s, _ := New(cfgPath)
	if got := s.WorkspaceNames(); len(got) != 1 || got[0] != "Default" {
		t.Fatalf("initial workspaces = %v", got)
	}
	if err := s.AddWorkspace("Work"); err != nil {
		t.Fatalf("AddWorkspace: %v", err)
	}
	if got := s.WorkspaceNames(); len(got) != 2 || got[1] != "Work" {
		t.Errorf("after add = %v", got)
	}
	if err := s.RenameWorkspace(1, "Office"); err != nil {
		t.Fatalf("RenameWorkspace: %v", err)
	}
	if got := s.WorkspaceNames()[1]; got != "Office" {
		t.Errorf("after rename = %q", got)
	}

	// Persistence: a fresh session sees the changes.
	s2, _ := New(cfgPath)
	if got := s2.WorkspaceNames(); len(got) != 2 || got[1] != "Office" {
		t.Errorf("after reopen = %v", got)
	}
}

func TestDeleteWorkspace(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	s, _ := New(cfgPath)
	_ = s.AddWorkspace("Work")
	_ = s.AddWorkspace("Personal")
	if err := s.DeleteWorkspace(0); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	if got := s.WorkspaceNames(); len(got) != 2 || got[0] != "Work" || got[1] != "Personal" {
		t.Errorf("after delete = %v", got)
	}
	// Can't delete last two? we have two; delete one more.
	_ = s.DeleteWorkspace(1)
	if got := s.WorkspaceNames(); len(got) != 1 {
		t.Errorf("after second delete = %v", got)
	}
	if err := s.DeleteWorkspace(0); err == nil {
		t.Errorf("deleting last workspace should error")
	}
}
