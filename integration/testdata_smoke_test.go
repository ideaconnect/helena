package integration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/importer"
	"github.com/idct/helena/internal/storage"
)

// repoRoot resolves the absolute path to the repository root from
// any test in the integration package. Walks up from the current
// working dir until it finds the go.mod file.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found walking up from %q", dir)
		}
		dir = parent
	}
}

// TestTestdataOpenAPIFixturesParse verifies every shared OpenAPI
// fixture under testdata/openapi/ either parses cleanly (the two
// well-formed specs) or errors clearly (the intentionally broken
// one). Catches drift between the fixture files and the importer
// contract before downstream tests rely on either.
func TestTestdataOpenAPIFixturesParse(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "openapi")
	cases := map[string]bool{
		"minimal.yaml": true,  // expect parse success
		"complex.yaml": true,  // expect parse success
		"broken.yaml":  false, // expect parse error
	}
	for name, wantOK := range cases {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		_, err = importer.FromOpenAPI(body)
		if wantOK && err != nil {
			t.Errorf("%s: expected to parse, got %v", name, err)
		}
		if !wantOK && err == nil {
			t.Errorf("%s: expected parse error, got nil", name)
		}
	}
}

// TestTestdataSwaggerFixturesParse runs the same parse-or-error
// matrix on the Swagger 2 fixtures.
func TestTestdataSwaggerFixturesParse(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "swagger")
	for _, name := range []string{"basic.yaml", "parameters.yaml"} {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if _, err := importer.FromOpenAPI(body); err != nil {
			t.Errorf("%s: parse: %v", name, err)
		}
	}
}

// TestTestdataCollectionsLoad verifies every shared collection
// fixture loads via storage.Load — catches a missing field, a
// misspelled YAML key, or a stale fixture before downstream tests
// trip on it.
func TestTestdataCollectionsLoad(t *testing.T) {
	root := filepath.Join(repoRoot(t), "testdata", "collections")
	for _, name := range []string{"minimal", "complex", "extras"} {
		c, err := storage.Load(filepath.Join(root, name))
		if err != nil {
			t.Errorf("%s: Load: %v", name, err)
			continue
		}
		if c.Name == "" {
			t.Errorf("%s: loaded collection has empty Name", name)
		}
	}
}

// TestTestdataExtrasFixtureSurvivesInPlaceRoundTrip verifies the
// hand-authored extras fixture round-trips through storage.Load →
// storage.Save in place (the production semantics: the UI Save
// button writes back to the same dir the collection was loaded
// from) with every documented `helena-x-*` marker still present.
// This is the canonical positive case for AGENTS invariant 1.
//
// We copy the fixture into a tempdir first so the smoke test never
// writes to the repo-tracked file.
func TestTestdataExtrasFixtureSurvivesInPlaceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(repoRoot(t), "testdata", "collections", "extras")
	entries, err := os.ReadDir(srcDir)
	if err != nil {
		t.Fatalf("read extras dir: %v", err)
	}
	for _, e := range entries {
		body, err := os.ReadFile(filepath.Join(srcDir, e.Name()))
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), body, 0o644); err != nil {
			t.Fatalf("copy %s: %v", e.Name(), err)
		}
	}
	c, err := storage.Load(dir)
	if err != nil {
		t.Fatalf("Load extras: %v", err)
	}
	// In-place re-save — same directory the load read from. This is
	// the production round-trip path (UI Save back to source dir).
	if err := storage.Save(c, dir); err != nil {
		t.Fatalf("Save round-trip: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "opencollection.yml"))
	if err != nil {
		t.Fatalf("read saved: %v", err)
	}
	for _, marker := range []string{
		"helena-x-collection-marker",
		"customCollectionSetting",
		"helena-x-bearer-marker",
	} {
		if !strings.Contains(string(got), marker) {
			t.Errorf("marker %q lost on in-place round-trip:\n%s", marker, got)
		}
	}
	probe, err := os.ReadFile(filepath.Join(dir, "probe.yml"))
	if err != nil {
		t.Fatalf("read probe: %v", err)
	}
	for _, marker := range []string{
		"helena-x-info-marker",
		"helena-x-header-marker",
		"helena-x-param-marker",
		"helena-x-chain-marker",
		"framework: external",
	} {
		if !strings.Contains(string(probe), marker) {
			t.Errorf("probe marker %q lost on in-place round-trip:\n%s", marker, probe)
		}
	}
}
