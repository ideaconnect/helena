package session

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

func writeCollectionWithEnv(t *testing.T) string {
	t.Helper()
	c := model.Collection{
		Name:     "Env Demo",
		Requests: []model.Request{{Name: "Ping", Method: model.GET, URL: "{{base}}/ping"}},
		Environments: []model.Environment{{
			Name: "Local",
			Variables: []model.Variable{
				{Enabled: true, Key: "base", Value: "http://localhost:9000"},
				{Enabled: false, Key: "token", Value: "xyz"},
			},
		}},
	}
	dir := filepath.Join(t.TempDir(), "env-demo")
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return dir
}

func TestResolverFromActiveEnv(t *testing.T) {
	dir := writeCollectionWithEnv(t)
	s, _ := New(filepath.Join(t.TempDir(), "config.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	if got := s.CollectionEnvironmentNames(); !reflect.DeepEqual(got, []string{"Local"}) {
		t.Fatalf("env names = %v, want [Local]", got)
	}
	// With no environment selected, nothing resolves.
	if out, missing := s.Resolver().Resolve("{{base}}/x"); out != "{{base}}/x" || len(missing) == 0 {
		t.Errorf("no-env resolve = %q missing=%v", out, missing)
	}
	s.SetActiveEnv("Local")
	out, missing := s.Resolver().Resolve("{{base}}/x")
	if out != "http://localhost:9000/x" || len(missing) != 0 {
		t.Errorf("resolved = %q missing=%v", out, missing)
	}
	// A disabled variable must not resolve.
	if _, missing := s.Resolver().Resolve("{{token}}"); len(missing) == 0 {
		t.Errorf("disabled var should remain unresolved")
	}
}

func TestEnvEditPersists(t *testing.T) {
	dir := writeCollectionWithEnv(t)
	cfg := filepath.Join(t.TempDir(), "config.yml")

	s, _ := New(cfg)
	if err := s.OpenCollection(dir); err != nil {
		t.Fatalf("OpenCollection: %v", err)
	}
	s.SetActiveEnv("Local")
	s.SetActiveEnvironmentVariables([]model.Variable{{Enabled: true, Key: "base", Value: "https://prod.example"}})
	if err := s.SaveActiveCollection(); err != nil {
		t.Fatalf("SaveActiveCollection: %v", err)
	}

	// A new session (same config) must load the edited environment.
	s2, _ := New(cfg)
	s2.SetActiveEnv("Local")
	if out, _ := s2.Resolver().Resolve("{{base}}"); out != "https://prod.example" {
		t.Errorf("after reload resolved = %q, want https://prod.example", out)
	}
}

func TestParseFormatEnvVars(t *testing.T) {
	got := ParseEnvVars("base = http://x\n# token = secret\nfoo=bar\n\n   \nmalformed")
	want := []model.Variable{
		{Enabled: true, Key: "base", Value: "http://x"},
		{Enabled: false, Key: "token", Value: "secret"},
		{Enabled: true, Key: "foo", Value: "bar"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ParseEnvVars = %#v\n want %#v", got, want)
	}
	if back := ParseEnvVars(FormatEnvVars(want)); !reflect.DeepEqual(back, want) {
		t.Errorf("Format->Parse round-trip mismatch: %#v", back)
	}
}
