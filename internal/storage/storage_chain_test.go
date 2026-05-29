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
