package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestRequestChainRoundTrip verifies Chain entries survive Save → Load
// and that the on-disk YAML carries the documented key names.
func TestRequestChainRoundTrip(t *testing.T) {
	orig := model.Collection{
		Name: "Chain sample",
		Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			Name:   "Profile",
			Method: model.GET,
			URL:    "https://api/profile",
			Body:   model.Body{Type: model.BodyNone},
			Auth:   model.Auth{Type: model.AuthInherit},
			Chain: []model.ChainStep{
				{Alias: "login", Request: "Auth/Login"},
				{Alias: "csrf", Request: "Bootstrap"},
			},
		}},
	}
	dir := t.TempDir()
	if err := Save(orig, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "profile.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	for _, want := range []string{"chain:", "alias: login", "request: Auth/Login", "alias: csrf"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("expected %q in YAML:\n%s", want, body)
		}
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(got.Requests))
	}
	gc := got.Requests[0].Chain
	if len(gc) != 2 || gc[0].Alias != "login" || gc[0].Request != "Auth/Login" ||
		gc[1].Alias != "csrf" || gc[1].Request != "Bootstrap" {
		t.Errorf("chain = %+v", gc)
	}
}

// TestRequestChainEmptyOmittedFromYAML verifies an empty Chain doesn't
// emit a stray `chain:` block in the YAML.
func TestRequestChainEmptyOmittedFromYAML(t *testing.T) {
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
	if strings.Contains(string(body), "chain:") {
		t.Errorf("empty Chain produced a chain: block:\n%s", body)
	}
}

// TestRequestChainExtraPreserved verifies that unknown keys nested on
// a chain entry (e.g. an alias `description` from another tool)
// survive a load → save cycle, satisfying invariant 1.
func TestRequestChainExtraPreserved(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, collectionFile),
		[]byte("info:\n  name: ChainExtra\n  type: collection\n"), 0o644); err != nil {
		t.Fatalf("write root: %v", err)
	}
	reqYAML := `info:
  name: Profile
  type: http
  seq: 1
http:
  method: GET
  url: https://api/profile
chain:
  - alias: login
    request: Auth/Login
    description: runs the login first
`
	if err := os.WriteFile(filepath.Join(dir, "profile.yml"), []byte(reqYAML), 0o644); err != nil {
		t.Fatalf("write request: %v", err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Edit a known field to force the Save merge path.
	c.Requests[0].URL = "https://api/v2/profile"
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := os.ReadFile(filepath.Join(dir, "profile.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(out), "description:") {
		t.Errorf("chain entry description was lost:\n%s", out)
	}
}

// TestRequestIDPersistsViaInfoBlock verifies that a request's ID is
// written to info.id on Save and read back on Load — so chain refs
// pinned by RequestID stay valid across reloads. Files written by
// older Helena (no info.id) still get a fresh ID on Load, which the
// next Save then persists.
func TestRequestIDPersistsViaInfoBlock(t *testing.T) {
	orig := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			ID: "fixed-id-0123456789abcdef", Name: "R", Method: model.GET,
			URL: "https://x/", Body: model.Body{Type: model.BodyNone},
			Auth: model.Auth{Type: model.AuthInherit},
		}},
	}
	dir := t.TempDir()
	if err := Save(orig, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "r.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(body), "id: fixed-id-0123456789abcdef") {
		t.Errorf("expected info.id in YAML:\n%s", body)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Requests[0].ID != "fixed-id-0123456789abcdef" {
		t.Errorf("Load lost id: got %q", got.Requests[0].ID)
	}
}

// TestRequestMissingIDIsGeneratedOnLoad verifies that a YAML file
// without info.id (e.g. authored by older Helena or by Bruno) still
// loads with a generated ID, and the next Save persists it.
func TestRequestMissingIDIsGeneratedOnLoad(t *testing.T) {
	dir := t.TempDir()
	yml := "info:\n  name: R\n  type: http\nhttp:\n  method: GET\n  url: https://x/\n"
	if err := os.WriteFile(filepath.Join(dir, "r.yml"), []byte(yml), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	colYAML := "info:\n  name: C\n  type: collection\n"
	if err := os.WriteFile(filepath.Join(dir, "opencollection.yml"), []byte(colYAML), 0o644); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(c.Requests) != 1 || c.Requests[0].ID == "" {
		t.Fatalf("expected request with generated ID, got %+v", c.Requests)
	}
	gen := c.Requests[0].ID
	if err := Save(c, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, _ := os.ReadFile(filepath.Join(dir, "r.yml"))
	if !strings.Contains(string(out), "id: "+gen) {
		t.Errorf("next Save did not persist id %q:\n%s", gen, out)
	}
}

// TestChainStepRequestIDRoundTrip verifies that ChainStep.RequestID
// (the stable-ID pin) survives Save → Load and lands under requestId
// in the YAML.
func TestChainStepRequestIDRoundTrip(t *testing.T) {
	orig := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			Name: "Profile", Method: model.GET, URL: "https://x/profile",
			Body: model.Body{Type: model.BodyNone},
			Auth: model.Auth{Type: model.AuthInherit},
			Chain: []model.ChainStep{
				{Alias: "login", Request: "Auth/Login", RequestID: "login-pin"},
			},
		}},
	}
	dir := t.TempDir()
	if err := Save(orig, dir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "profile.yml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(body), "requestId: login-pin") {
		t.Errorf("expected requestId in YAML:\n%s", body)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Requests[0].Chain[0].RequestID != "login-pin" {
		t.Errorf("RequestID lost on Load: %+v", c.Requests[0].Chain[0])
	}
}
