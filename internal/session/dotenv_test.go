package session

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

// TestParseDotEnv verifies KEY=VALUE parsing: comments, blank lines, an export
// prefix, quoted values, an '=' inside a value, and skipped malformed lines.
func TestParseDotEnv(t *testing.T) {
	in := []byte(`
# a comment
BASE=https://api.test

export TOKEN = "s3cret"
QUOTED='hello world'
EQ=a=b=c
  SPACED  =  trimmed
NOEQ
=novalue
`)
	got := parseDotEnv(in)
	want := map[string]string{
		"BASE":   "https://api.test",
		"TOKEN":  "s3cret",
		"QUOTED": "hello world",
		"EQ":     "a=b=c",
		"SPACED": "trimmed",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseDotEnv =\n %#v\nwant\n %#v", got, want)
	}
}

// TestDotEnvResolvesBelowCollection pins #84: collection-root .env variables
// resolve at the lowest static precedence — a collection variable of the same
// name overrides them, while a .env-only name still resolves.
func TestDotEnvResolvesBelowCollection(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c")
	col := model.Collection{
		Name: "C",
		Variables: []model.Variable{
			{Enabled: true, Key: "BASE", Value: "from-collection"},
		},
	}
	if err := storage.Save(col, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("BASE=from-dotenv\nDONLY=denv-only\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, _ := New(filepath.Join(t.TempDir(), "config.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)

	out, missing := s.Resolver().Resolve("{{BASE}}|{{DONLY}}")
	if len(missing) != 0 {
		t.Fatalf("unexpected missing: %v", missing)
	}
	if want := "from-collection|denv-only"; out != want {
		t.Errorf("resolve = %q; want %q (collection overrides .env; .env-only resolves)", out, want)
	}
}

// TestDotEnvSnapshotIsCopy verifies SnapshotActiveDotEnvVars returns an
// independent map (mutating it doesn't affect the cached values).
func TestDotEnvSnapshotIsCopy(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c")
	if err := storage.Save(model.Collection{Name: "C"}, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("K=v\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _ := New(filepath.Join(t.TempDir(), "config.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)

	snap := s.SnapshotActiveDotEnvVars()
	if snap["K"] != "v" {
		t.Fatalf("snapshot = %v, want K=v", snap)
	}
	snap["K"] = "mutated"
	if again := s.SnapshotActiveDotEnvVars(); again["K"] != "v" {
		t.Errorf("snapshot mutation leaked into cache: %v", again)
	}
}

// TestDotEnvMissingFileIsEmpty verifies a collection with no .env yields no
// extra variables and doesn't error.
func TestDotEnvMissingFileIsEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "c")
	if err := storage.Save(model.Collection{Name: "C"}, dir); err != nil {
		t.Fatal(err)
	}
	s, _ := New(filepath.Join(t.TempDir(), "config.yml"))
	if err := s.OpenCollection(dir); err != nil {
		t.Fatal(err)
	}
	s.SetActiveCollection(0)
	if got := s.SnapshotActiveDotEnvVars(); len(got) != 0 {
		t.Errorf("expected empty .env map, got %v", got)
	}
}
