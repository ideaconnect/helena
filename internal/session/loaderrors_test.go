package session

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReloadCapturesLoadErrors verifies that a collection which fails to load
// is dropped from the in-memory set but its error is captured in LoadErrors,
// so the UI can surface a diagnostic instead of the collection silently
// vanishing from the sidebar (#108).
func TestReloadCapturesLoadErrors(t *testing.T) {
	colDir := writeSampleCollection(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yml")

	s, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.OpenCollection(colDir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	if errs := s.LoadErrors(); len(errs) != 0 {
		t.Fatalf("clean load reported errors: %+v", errs)
	}

	// Make the persisted collection unloadable, then reopen at the same config
	// path so reload re-reads the workspace's recorded dir and fails on it.
	if err := os.RemoveAll(colDir); err != nil {
		t.Fatal(err)
	}

	s2, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	if len(s2.Collections()) != 0 {
		t.Errorf("unloadable collection was not dropped: %+v", s2.Collections())
	}
	errs := s2.LoadErrors()
	if len(errs) != 1 || errs[0].Dir != colDir {
		t.Fatalf("LoadErrors = %+v, want exactly one for %s", errs, colDir)
	}
	if errs[0].Err == nil {
		t.Error("captured LoadError has a nil Err")
	}
}

// TestLoadErrorsCopyIsIndependent verifies LoadErrors returns a copy callers
// can retain across reloads without aliasing session state.
func TestLoadErrorsCopyIsIndependent(t *testing.T) {
	colDir := writeSampleCollection(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	s, _ := New(cfgPath)
	if err := s.OpenCollection(colDir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	os.RemoveAll(colDir)
	s2, _ := New(cfgPath) // reload of the workspace now fails on the missing dir

	first := s2.LoadErrors()
	if len(first) != 1 {
		t.Fatalf("LoadErrors = %+v, want one", first)
	}
	first[0].Dir = "mutated"
	if s2.LoadErrors()[0].Dir == "mutated" {
		t.Error("LoadErrors handed out an aliased slice; mutation leaked into session state")
	}
}
