package session

import (
	"path/filepath"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestRequestVarsResolveAboveEnvironment pins #82: a request's own variables
// are the highest-precedence static scope — they override environment and
// collection variables of the same name — but the runtime script overlay still
// wins. Disabled request vars don't resolve.
func TestRequestVarsResolveAboveEnvironment(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c")
	req := model.Request{
		Name: "R", Method: model.GET, URL: "https://x/",
		Variables: []model.Variable{
			{Enabled: true, Key: "k", Value: "from-request"},
			{Enabled: true, Key: "ronly", Value: "req-only"},
			{Enabled: false, Key: "off", Value: "should-not-apply"},
		},
	}
	col := model.Collection{
		Name:     "C",
		Requests: []model.Request{req},
		Variables: []model.Variable{
			{Enabled: true, Key: "k", Value: "from-collection"},
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

	loaded := &s.Collections()[0].Requests[0]

	out, missing := s.ResolverForRequest(loaded).Resolve("{{k}}|{{ronly}}|{{eonly}}")
	if len(missing) != 0 {
		t.Fatalf("unexpected missing names: %v", missing)
	}
	if want := "from-request|req-only|env-only"; out != want {
		t.Errorf("resolve = %q; want %q (request overrides env + collection)", out, want)
	}

	// A disabled request variable does not resolve.
	if _, missing := s.ResolverForRequest(loaded).Resolve("{{off}}"); len(missing) != 1 || missing[0] != "off" {
		t.Errorf("disabled request var should be unresolved; missing=%v", missing)
	}

	// The runtime script overlay still outranks a request variable.
	s.SetEnvOverlay("k", "from-overlay")
	if out, _ := s.ResolverForRequest(loaded).Resolve("{{k}}"); out != "from-overlay" {
		t.Errorf("overlay should outrank request var; got %q", out)
	}

	// A nil request behaves exactly like the plain Resolver (env wins).
	if out, _ := s.ResolverForRequest(nil).Resolve("{{k}}"); out != "from-overlay" {
		t.Errorf("nil-request resolver = %q; want overlay value", out)
	}
}
