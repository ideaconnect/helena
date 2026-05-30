package integration

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/idct/helena/internal/importer"
	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/storage"
)

const importedSpec = `openapi: 3.0.0
info:
  title: Imported API
servers:
  - url: SERVER_URL_PLACEHOLDER
paths:
  /health:
    get:
      summary: Health
  /me:
    get:
      summary: Whoami
`

// TestImportPersistReloadAndSend verifies the full importer → storage
// → session → httpclient wire: an OpenAPI spec imports into a
// collection, lands on disk, survives a reopen, and the imported
// requests are sendable end-to-end (with the imported {{base_url}}
// env variable resolving to the test server).
func TestImportPersistReloadAndSend(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		case "/me":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"user":"alice"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	p := NewPipelineWithServer(t, srv)

	// Import the spec with the test server's URL substituted into
	// the OpenAPI `servers` block so {{base_url}} resolves correctly.
	spec := strings.ReplaceAll(importedSpec, "SERVER_URL_PLACEHOLDER", srv.URL)
	c, err := importer.FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	if err := storage.Save(c, p.CollDir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	// Reopen via a fresh session — verifies the disk round-trip
	// preserved everything Send needs (URLs, env vars, tree shape).
	if err := p.Reopen(); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	// The importer creates a "Default" env with base_url; select it
	// so URL templates referencing {{base_url}} resolve.
	p.Sess.SetActiveEnv("Default")

	view, _, err := p.Send("Health")
	if err != nil {
		t.Fatalf("Send Health: %v", err)
	}
	if view.Response.StatusCode != 200 || string(view.Response.Body) != "ok" {
		t.Errorf("Health response = %d %q, want 200 \"ok\"", view.Response.StatusCode, view.Response.Body)
	}

	view, _, err = p.Send("Whoami")
	if err != nil {
		t.Fatalf("Send Whoami: %v", err)
	}
	if !strings.Contains(string(view.Response.Body), "alice") {
		t.Errorf("Whoami body = %q, want substring 'alice'", view.Response.Body)
	}
}

// TestImportThenChainFromExternalRequest verifies the wire between
// the importer and chain.Resolve: a user can import an OpenAPI spec
// and then add a NEW request that chains to one of the imported
// requests, getting that response in the leaf's pre-script.
//
// This is the realistic "import the API spec, then write tests that
// reuse the imported endpoints" workflow.
func TestImportThenChainFromExternalRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/me":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"user":"alice","id":42}`))
		case "/audit":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(string(r.Header.Get("X-User-Id"))))
		}
	}))
	p := NewPipelineWithServer(t, srv)

	spec := strings.ReplaceAll(importedSpec, "SERVER_URL_PLACEHOLDER", srv.URL)
	c, err := importer.FromOpenAPI([]byte(spec))
	if err != nil {
		t.Fatalf("FromOpenAPI: %v", err)
	}
	// Add a non-imported "Audit" request that chains to the imported
	// "Whoami" and forwards the user-id as a header.
	c.Requests = append(c.Requests, model.Request{
		Name: "Audit", Method: model.GET, URL: "{{base_url}}/audit",
		Body: model.Body{Type: model.BodyNone}, Auth: model.Auth{Type: model.AuthInherit},
		Chain: []model.ChainStep{{Alias: "me", Request: "Whoami"}},
		Scripts: model.Scripts{
			PreRequest: `request.headers["X-User-Id"] = String(chain.me.response.json.id);`,
		},
	})
	if err := storage.Save(c, p.CollDir); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := p.Reopen(); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	// The importer creates a "Default" env with base_url; select it
	// so URL templates referencing {{base_url}} resolve.
	p.Sess.SetActiveEnv("Default")

	view, _, err := p.Send("Audit")
	if err != nil {
		t.Fatalf("Send Audit: %v", err)
	}
	if view.Response.StatusCode != 200 {
		t.Errorf("Audit status = %d, want 200", view.Response.StatusCode)
	}
	if got := string(view.Response.Body); got != "42" {
		t.Errorf("Audit body = %q, want \"42\" (chain → script wiring)", got)
	}
}
