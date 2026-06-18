package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestCollectionVarsResolveBelowEnvironment pins #80: collection-level variables
// resolve, but an environment variable of the same name overrides them (and a
// collection-only name still resolves).
func TestCollectionVarsResolveBelowEnvironment(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c")
	col := model.Collection{
		Name: "C",
		Variables: []model.Variable{
			{Enabled: true, Key: "k", Value: "from-collection"},
			{Enabled: true, Key: "conly", Value: "col-only"},
			{Enabled: false, Key: "off", Value: "should-not-apply"},
		},
		Environments: []model.Environment{{
			Name: "Dev",
			Variables: []model.Variable{
				{Enabled: true, Key: "k", Value: "from-env"},
				{Enabled: true, Key: "eonly", Value: "env-only"},
			},
		}},
	}
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}

	s, _ := New(filepath.Join(t.TempDir(), "config.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)
	s.SetActiveEnv("Dev")

	out, missing := s.Resolver().Resolve("{{k}}|{{conly}}|{{eonly}}")
	if len(missing) != 0 {
		t.Fatalf("unexpected missing names: %v", missing)
	}
	if want := "from-env|col-only|env-only"; out != want {
		t.Errorf("resolve = %q; want %q (environment overrides collection)", out, want)
	}

	// A disabled collection variable does not resolve.
	if _, missing := s.Resolver().Resolve("{{off}}"); len(missing) != 1 || missing[0] != "off" {
		t.Errorf("disabled collection var should be unresolved; missing=%v", missing)
	}
}
