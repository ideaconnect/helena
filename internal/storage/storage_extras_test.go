package storage

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestUnknownFieldsPreserved writes a YAML file with fields we don't model
// (auth, runtime scripts, docs, a custom collection-level field), loads it,
// modifies a known field, saves, and confirms the unknown fields are still
// present.
func TestUnknownFieldsPreserved(t *testing.T) {
	dir := t.TempDir()

	rootYAML := `info:
  name: Demo
  type: collection
shared:
  notes: "this is a custom collection-level field"
`
	if err := os.WriteFile(filepath.Join(dir, collectionFile), []byte(rootYAML), 0o644); err != nil {
		t.Fatalf("write root: %v", err)
	}

	reqYAML := `info:
  name: GetThing
  type: http
  seq: 1
http:
  method: GET
  url: https://example.test/x
  auth:
    type: bearer
    token: secret-abc
runtime:
  scripts:
    - type: pre-request
      code: console.log("hi")
docs: |
  Some markdown documentation.
`
	if err := os.WriteFile(filepath.Join(dir, "getthing.yml"), []byte(reqYAML), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Modify known fields.
	c.Name = "Demo (renamed)"
	if len(c.Requests) != 1 {
		t.Fatalf("got %d requests, want 1", len(c.Requests))
	}
	c.Requests[0].URL = "https://example.test/y"

	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	rootData, _ := os.ReadFile(filepath.Join(dir, collectionFile))
	if !bytes.Contains(rootData, []byte("notes:")) || !bytes.Contains(rootData, []byte("custom collection-level")) {
		t.Errorf("root file lost the custom 'shared' field:\n%s", rootData)
	}
	if !bytes.Contains(rootData, []byte("Demo (renamed)")) {
		t.Errorf("rename did not persist:\n%s", rootData)
	}

	// Find the request file (slug may differ).
	entries, _ := os.ReadDir(dir)
	var reqPath string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ymlExt) && e.Name() != collectionFile {
			reqPath = filepath.Join(dir, e.Name())
		}
	}
	if reqPath == "" {
		t.Fatalf("no request file found in %s", dir)
	}
	reqData, _ := os.ReadFile(reqPath)
	for _, want := range []string{"auth:", "bearer", "secret-abc", "runtime:", `console.log`, "docs:", "https://example.test/y"} {
		if !bytes.Contains(reqData, []byte(want)) {
			t.Errorf("request file lost %q after save:\n%s", want, reqData)
		}
	}
}

// TestSaveSweepsOrphans verifies that removing items from the in-memory
// collection causes their on-disk artifacts to disappear on the next Save.
func TestSaveSweepsOrphans(t *testing.T) {
	dir := t.TempDir()

	c := model.Collection{
		Name:     "C",
		Requests: []model.Request{{Name: "R1", Method: model.GET, URL: "u"}},
		Folders: []model.Folder{{
			Name:     "F1",
			Requests: []model.Request{{Name: "R2", Method: model.GET, URL: "u"}},
		}},
		Environments: []model.Environment{{
			Name:      "Local",
			Variables: []model.Variable{{Enabled: true, Key: "k", Value: "v"}},
		}},
	}
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save initial: %v", err)
	}

	// Remove everything and re-save.
	c.Folders = nil
	c.Requests = nil
	c.Environments = nil
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save empty: %v", err)
	}

	// No request .yml files (other than opencollection.yml).
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() {
			if e.Name() == environmentsDir {
				continue
			}
			if _, err := os.Stat(filepath.Join(dir, e.Name(), folderFile)); err == nil {
				t.Errorf("folder dir %q should have been swept", e.Name())
			}
		} else if strings.HasSuffix(e.Name(), ymlExt) && e.Name() != collectionFile {
			t.Errorf("orphan request file %q should have been swept", e.Name())
		}
	}

	// Environments dir, if it still exists, should be empty of .yml.
	envEntries, _ := os.ReadDir(filepath.Join(dir, environmentsDir))
	for _, e := range envEntries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ymlExt) {
			t.Errorf("orphan environment file %q should have been swept", e.Name())
		}
	}
}
