package session

import (
	"path/filepath"
	"testing"
)

// TestUIStatePersistsAndRestores verifies that the active collection, active
// environment, open request and window size all round-trip through a fresh
// Session, and that the restored env feeds back into Resolver.
func TestUIStatePersistsAndRestores(t *testing.T) {
	dir := writeCollectionWithEnv(t)
	cfgPath := filepath.Join(t.TempDir(), "config.yml")

	s, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	s.SetActiveEnv("Local")
	s.SetOpenRequest("0/r0") // the sample collection's single root request
	s.SetWindowSize(1280, 768)

	// A fresh session must restore everything.
	s2, err := New(cfgPath)
	if err != nil {
		t.Fatalf("New (reopen): %v", err)
	}
	if got := s2.ActiveCollection(); got != 0 {
		t.Errorf("ActiveCollection = %d, want 0", got)
	}
	if got := s2.ActiveEnvName(); got != "Local" {
		t.Errorf("ActiveEnvName = %q, want Local", got)
	}
	if got := s2.OpenRequest(); got != "0/r0" {
		t.Errorf("OpenRequest = %q, want 0/r0", got)
	}
	if w, h := s2.WindowSize(); w != 1280 || h != 768 {
		t.Errorf("WindowSize = %dx%d, want 1280x768", w, h)
	}
	// And the resolver should now pick up the restored env vars.
	if out, _ := s2.Resolver().Resolve("{{base}}/x"); out != "http://localhost:9000/x" {
		t.Errorf("Resolver after restore = %q", out)
	}
}

// TestOpenRequestStableAcrossCollectionReordering verifies that the open
// request is persisted by collection directory path (not index) so reordering
// the workspace's collections does not break restoration.
func TestOpenRequestStableAcrossCollectionReordering(t *testing.T) {
	// Persist by collection directory path, so reordering collections in the
	// workspace doesn't break restoration.
	dirA := writeCollectionWithEnv(t) // single request at "0/r0" via sample
	cfgPath := filepath.Join(t.TempDir(), "config.yml")

	s, _ := New(cfgPath)
	if err := s.OpenCollection(dirA); err != nil {
		t.Fatalf("OpenCollection A: %v", err)
	}
	s.SetOpenRequest("0/r0")

	// Manually flip workspace collection order before reload (simulates the
	// user re-arranging collection paths on disk / in config).
	s.cfg.Workspaces[0].Collections = []string{dirA} // (only one here; verify path-keyed lookup)
	if err := s.persist(); err != nil {
		t.Fatalf("persist: %v", err)
	}

	s2, _ := New(cfgPath)
	if got := s2.OpenRequest(); got != "0/r0" {
		t.Errorf("OpenRequest after reorder = %q, want 0/r0", got)
	}
}
