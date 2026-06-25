package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestGlobalVarsResolveBelowEverything pins #83: global variables are the
// lowest-precedence scope (a collection variable of the same name overrides
// them, a global-only name still resolves), disabled globals don't resolve, and
// SetGlobalVariables persists across a session reload.
func TestGlobalVarsResolveBelowEverything(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "config.yml")
	dir := filepath.Join(t.TempDir(), "c")
	col := model.Collection{
		Name:      "C",
		Variables: []model.Variable{{Enabled: true, Key: "k", Value: "from-collection"}},
	}
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}

	s, _ := New(cfgPath)
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)
	if err := s.SetGlobalVariables([]model.Variable{
		{Enabled: true, Key: "k", Value: "from-global"},
		{Enabled: true, Key: "gonly", Value: "global-only"},
		{Enabled: false, Key: "off", Value: "should-not-apply"},
	}); err != nil {
		t.Fatal(err)
	}

	out, missing := s.Resolver().Resolve("{{k}}|{{gonly}}")
	if len(missing) != 0 {
		t.Fatalf("unexpected missing: %v", missing)
	}
	if want := "from-collection|global-only"; out != want {
		t.Errorf("resolve = %q; want %q (collection overrides global; global-only resolves)", out, want)
	}
	if _, missing := s.Resolver().Resolve("{{off}}"); len(missing) != 1 || missing[0] != "off" {
		t.Errorf("disabled global var should be unresolved; missing=%v", missing)
	}

	// Persistence: a fresh session over the same config file sees the globals.
	s2, err := New(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	got := s2.GlobalVariables()
	if len(got) != 3 || got[0].Key != "k" || got[0].Value != "from-global" {
		t.Errorf("global variables did not persist: %+v", got)
	}
}
