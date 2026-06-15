package session

import "testing"

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
