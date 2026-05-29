package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestRequestScriptsRoundTrip verifies that PreRequest and PostResponse
// bodies survive Save → Load → Save unchanged, and that the on-disk
// YAML carries the `scripts:` block with the documented key names.
func TestRequestScriptsRoundTrip(t *testing.T) {
	orig := model.Collection{
		Name: "Scripts sample",
		Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			Name:   "Login",
			Method: model.POST,
			URL:    "https://api/login",
			Body:   model.Body{Type: model.BodyNone},
			Auth:   model.Auth{Type: model.AuthInherit},
			Scripts: model.Scripts{
				PreRequest:   `console.log("pre");`,
				PostResponse: `helena.env.set("TOKEN", response.json.token);`,
			},
		}},
	}
	dir := t.TempDir()
	if err := Save(orig, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Inspect the on-disk YAML for the documented keys.
	body, err := os.ReadFile(filepath.Join(dir, "login.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	s := string(body)
	for _, want := range []string{"scripts:", "preRequest:", "postResponse:"} {
		if !strings.Contains(s, want) {
			t.Errorf("expected on-disk YAML to contain %q:\n%s", want, s)
		}
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(got.Requests))
	}
	gs := got.Requests[0].Scripts
	if gs.PreRequest != `console.log("pre");` {
		t.Errorf("PreRequest = %q", gs.PreRequest)
	}
	if gs.PostResponse != `helena.env.set("TOKEN", response.json.token);` {
		t.Errorf("PostResponse = %q", gs.PostResponse)
	}
}

// TestRequestScriptsEmptyOmittedFromYAML verifies that a request whose
// Scripts is the zero value writes no `scripts:` block — keeps clean
// YAML for the majority of requests that don't use scripting.
func TestRequestScriptsEmptyOmittedFromYAML(t *testing.T) {
	c := model.Collection{
		Name: "Plain",
		Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			Name: "Get", Method: model.GET, URL: "https://x/",
			Body: model.Body{Type: model.BodyNone},
			Auth: model.Auth{Type: model.AuthInherit},
		}},
	}
	dir := t.TempDir()
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "get.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(string(body), "scripts:") {
		t.Errorf("zero Scripts produced a scripts: block:\n%s", body)
	}
}

// TestRequestScriptsExtraSurvivesClear verifies that even when the user
// clears both PreRequest and PostResponse, unknown sibling keys nested
// under the on-disk `scripts:` block (e.g. a `tests:` block another tool
// authored) survive — invariant 1 (Extra round-trip) demands it.
func TestRequestScriptsExtraSurvivesClear(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, collectionFile),
		[]byte("info:\n  name: ClearDemo\n  type: collection\n"), 0o644); err != nil {
		t.Fatalf("write root: %v", err)
	}
	reqYAML := `info:
  name: Login
  type: http
  seq: 1
http:
  method: POST
  url: https://api/login
scripts:
  preRequest: 'console.log("pre");'
  postResponse: ''
  tests: |
    pm.test("status is 200", function() { pm.response.to.have.status(200); });
`
	if err := os.WriteFile(filepath.Join(dir, "login.yml"), []byte(reqYAML), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// User clears the script in the UI.
	c.Requests[0].Scripts = model.Scripts{}
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "login.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(out), "tests:") {
		t.Errorf("scripts.tests lost after user cleared hooks:\n%s", out)
	}
}

// TestRequestScriptsExtraSurvivesEdit verifies that unknown keys nested
// inside the on-disk scripts block (e.g. a `tests:` block another tool
// authored) round-trip through a load → save cycle.
func TestRequestScriptsExtraSurvivesEdit(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, collectionFile),
		[]byte("info:\n  name: ExtraDemo\n  type: collection\n"), 0o644); err != nil {
		t.Fatalf("write root: %v", err)
	}

	reqYAML := `info:
  name: Login
  type: http
  seq: 1
http:
  method: POST
  url: https://api/login
scripts:
  preRequest: 'console.log("pre");'
  postResponse: ''
  tests: |
    pm.test("status is 200", function() { pm.response.to.have.status(200); });
`
	if err := os.WriteFile(filepath.Join(dir, "login.yml"), []byte(reqYAML), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Edit a known field so Save runs the Extra-preserving merge.
	c.Requests[0].URL = "https://api/v2/login"
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}

	out, err := os.ReadFile(filepath.Join(dir, "login.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(out), "tests:") {
		t.Errorf("scripts.tests block lost after save:\n%s", out)
	}
	if !strings.Contains(string(out), "https://api/v2/login") {
		t.Errorf("URL edit not saved:\n%s", out)
	}
}
