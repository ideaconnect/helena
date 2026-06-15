package session

import (
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestMoveClampsOutOfRangeIndex covers clampIndex's i>n and i<0 branches.
func TestMoveClampsOutOfRangeIndex(t *testing.T) {
	s := openSessionWithCollection(t)
	if _, err := s.MoveNode("0/r0", "0/f0", 99); err != nil { // past the end → append
		t.Fatalf("MoveNode high index: %v", err)
	}
	if got := reqName(s, "0/f0/r1"); got != "Health" {
		t.Errorf("0/f0/r1 = %q, want Health (clamped to append)", got)
	}

	s2 := openSessionWithCollection(t)
	if _, err := s2.MoveNode("0/r0", "0/f0", -5); err != nil { // negative → front
		t.Fatalf("MoveNode negative index: %v", err)
	}
	if got := reqName(s2, "0/f0/r0"); got != "Health" {
		t.Errorf("0/f0/r0 = %q, want Health (clamped to front)", got)
	}
}

// TestMoveCascadesChainRefs covers the MoveNode path that rewrites chain
// references whose target moved (cascadeChainRefs / rewriteRequests / hasPrefix).
func TestMoveCascadesChainRefs(t *testing.T) {
	s := openSessionWithCollection(t)                    // folder Users (0/f0) with Create User (0/f0/r0)
	if _, err := s.AddFolder("0", "Group"); err != nil { // 0/f1
		t.Fatal(err)
	}
	if _, err := s.AddRequest("0", "Caller"); err != nil { // 0/r1
		t.Fatal(err)
	}
	// Caller chains to the request inside Users by name-path.
	s.cols[0].Requests[1].Chain = []model.ChainStep{{Alias: "step", Request: "Users/Create User"}}

	if _, err := s.MoveNode("0/f0", "0/f1", 0); err != nil { // Users -> Group/Users
		t.Fatalf("MoveNode: %v", err)
	}
	if got := s.cols[0].Requests[1].Chain[0].Request; got != "Group/Users/Create User" {
		t.Errorf("chain ref = %q, want Group/Users/Create User (cascaded)", got)
	}
}

// TestCascadeChainRefsKindAware verifies the cascade rewrites only refs that
// genuinely target the moved node (#100): a folder rename touches descendant
// refs but leaves a same-named request ref and a same-prefix sibling alone; a
// request rename touches only its exact ref, not a descendant of a same-named
// folder.
func TestCascadeChainRefsKindAware(t *testing.T) {
	mkCol := func() *model.Collection {
		return &model.Collection{Requests: []model.Request{
			{Name: "child", Chain: []model.ChainStep{{Alias: "a", Request: "Auth/Login"}}},   // descendant of folder Auth
			{Name: "sibling", Chain: []model.ChainStep{{Alias: "a", Request: "AuthBackup"}}}, // same prefix, different node
			{Name: "reqref", Chain: []model.ChainStep{{Alias: "a", Request: "Auth"}}},        // targets a request named Auth
		}}
	}
	// Folder "Auth" -> "Sec": only the descendant ref changes.
	col := mkCol()
	cascadeChainRefs(col, []string{"Auth"}, []string{"Sec"}, true)
	if got := col.Requests[0].Chain[0].Request; got != "Sec/Login" {
		t.Errorf("folder rename: descendant ref = %q, want Sec/Login", got)
	}
	if got := col.Requests[1].Chain[0].Request; got != "AuthBackup" {
		t.Errorf("folder rename: same-prefix sibling wrongly rewritten: %q", got)
	}
	if got := col.Requests[2].Chain[0].Request; got != "Auth" {
		t.Errorf("folder rename: same-named request ref wrongly rewritten: %q", got)
	}
	// Request "Auth" -> "Sec": only the exact ref changes.
	col = mkCol()
	cascadeChainRefs(col, []string{"Auth"}, []string{"Sec"}, false)
	if got := col.Requests[2].Chain[0].Request; got != "Sec" {
		t.Errorf("request rename: exact ref = %q, want Sec", got)
	}
	if got := col.Requests[0].Chain[0].Request; got != "Auth/Login" {
		t.Errorf("request rename: same-named folder's descendant wrongly rewritten: %q", got)
	}
}

// TestMoveErrorPaths covers MoveNode's rejection branches.
func TestMoveErrorPaths(t *testing.T) {
	s := openSessionWithCollection(t)
	cases := []struct{ name, src, dst string }{
		{"malformed collection segment", "x/f0", "x"},
		{"non-movable collection node", "0", "0"},
		{"invalid drop target", "0/r0", "0/f9"},
		{"folder into itself", "0/f0", "0/f0"},
		{"folder into its descendant", "0/f0", "0/f0/f9"},
	}
	for _, c := range cases {
		if _, err := s.MoveNode(c.src, c.dst, 0); err == nil {
			t.Errorf("%s: expected error", c.name)
		}
	}
}

// TestDuplicateItemErrorPaths covers DuplicateItem's invalid-node and
// not-duplicable branches.
func TestDuplicateItemErrorPaths(t *testing.T) {
	s := openSessionWithCollection(t)
	for _, id := range []string{"0", "0/r9", "0/f9", "bogus"} {
		if _, err := s.DuplicateItem(id); err == nil {
			t.Errorf("DuplicateItem(%q): expected error", id)
		}
	}
}
