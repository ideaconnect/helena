package integration

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestScriptOverlayReachesNextSend verifies the full overlay
// pipeline: a post-script on request A writes TOKEN via
// helena.env.set; a subsequent Send of request B uses {{TOKEN}}
// in its URL — httpclient.Build resolves it via the per-Send
// snapshot of the session overlay.
//
// This is the canonical "log in once, use the token everywhere"
// workflow Helena's scripting + env overlay machinery enables.
func TestScriptOverlayReachesNextSend(t *testing.T) {
	var (
		mu          sync.Mutex
		seenQueries []url.Values
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seenQueries = append(seenQueries, r.URL.Query())
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"token":"from-server"}`))
	}))
	p := NewPipelineWithServer(t, srv)

	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{
			{
				Name: "Login", Method: model.POST, URL: srv.URL + "/login",
				Body: model.Body{Type: model.BodyNone},
				Auth: model.Auth{Type: model.AuthInherit},
				Scripts: model.Scripts{
					PostResponse: `helena.env.set("TOKEN", response.json.token);`,
				},
			},
			{
				Name: "Probe", Method: model.GET, URL: srv.URL + "/probe?t={{TOKEN}}",
				Body: model.Body{Type: model.BodyNone},
				Auth: model.Auth{Type: model.AuthInherit},
			},
		},
	}
	if err := p.SaveAndOpen(c); err != nil {
		t.Fatalf("SaveAndOpen: %v", err)
	}
	if _, _, err := p.Send("Login"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, _, err := p.Send("Probe"); err != nil {
		t.Fatalf("Probe: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seenQueries) != 2 {
		t.Fatalf("seenQueries = %d, want 2", len(seenQueries))
	}
	if got := seenQueries[1].Get("t"); got != "from-server" {
		t.Errorf("Probe query t = %q, want from-server (overlay didn't reach next Send)", got)
	}
}

// TestOverlayClearedOnReopen verifies invariant 9: env overlay is
// session-lifetime only and does NOT persist across a session
// reopen. The first session's TOKEN write must not survive into a
// fresh session backed by the same on-disk config.
func TestOverlayClearedOnReopen(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	p := NewPipelineWithServer(t, srv)

	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			Name: "X", Method: model.GET, URL: srv.URL + "/x",
			Body: model.Body{Type: model.BodyNone},
			Auth: model.Auth{Type: model.AuthInherit},
		}},
	}
	if err := p.SaveAndOpen(c); err != nil {
		t.Fatalf("SaveAndOpen: %v", err)
	}
	p.Sess.SetEnvOverlay("TOKEN", "set-in-session-1")
	if v, ok := p.Sess.EnvOverlay("TOKEN"); !ok || v != "set-in-session-1" {
		t.Fatalf("pre-reopen overlay = (%q,%v), want (\"set-in-session-1\",true)", v, ok)
	}
	if err := p.Reopen(); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	if _, ok := p.Sess.EnvOverlay("TOKEN"); ok {
		t.Error("overlay survived reopen; should be session-lifetime only (invariant 9)")
	}
}

// TestChainStepOverlayWriteIsRolledBackOnLeafFailure verifies the
// Phase 7.4 overlay-rollback semantics from the integration angle: a
// chain step writes TOKEN, the leaf then fails (forced via a
// post-script throw with no successful HTTP). The overlay must
// revert so the next Send doesn't see TOKEN.
func TestChainStepOverlayWriteIsRolledBackOnLeafFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Force a pre-script failure on Leaf so the chain returns an error.
		w.WriteHeader(http.StatusOK)
	}))
	p := NewPipelineWithServer(t, srv)

	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{
			{
				Name: "Boot", Method: model.GET, URL: srv.URL + "/boot",
				Body: model.Body{Type: model.BodyNone},
				Auth: model.Auth{Type: model.AuthInherit},
				Scripts: model.Scripts{
					PostResponse: `helena.env.set("TOKEN", "chain-wrote-me");`,
				},
			},
			{
				Name: "Leaf", Method: model.GET, URL: srv.URL + "/leaf",
				Body:  model.Body{Type: model.BodyNone},
				Auth:  model.Auth{Type: model.AuthInherit},
				Chain: []model.ChainStep{{Alias: "boot", Request: "Boot"}},
				Scripts: model.Scripts{
					// A chain-step failure rolls back. We need the chain
					// itself to error, not just the leaf. Easiest: have
					// Boot's POST-script set TOKEN, then a SECOND chain
					// step that fails with cycle / unresolved-ref to
					// trigger the rollback path.
					PreRequest: `throw new Error("force chain failure");`,
				},
			},
		},
	}
	// Adjust: add a second chain step on Leaf pointing at a non-existent
	// path so chain.Resolve errors out AFTER Boot wrote TOKEN.
	c.Requests[1].Chain = append(c.Requests[1].Chain,
		model.ChainStep{Alias: "missing", Request: "Nope"})

	if err := p.SaveAndOpen(c); err != nil {
		t.Fatalf("SaveAndOpen: %v", err)
	}
	// First, a quick Send of Boot alone to verify TOKEN flows when
	// nothing fails — sanity check the wiring.
	if _, _, err := p.Send("Boot"); err != nil {
		t.Fatalf("Boot solo: %v", err)
	}
	if v, _ := p.Sess.EnvOverlay("TOKEN"); v != "chain-wrote-me" {
		t.Fatalf("Boot solo did not set TOKEN: got %q", v)
	}
	// Reset overlay manually for the rollback test.
	p.Sess.RestoreEnvOverlay(map[string]string{})

	// Now Send Leaf: Boot's chain step writes TOKEN, then the
	// "missing" step fails → rollback should revert.
	if _, _, err := p.Send("Leaf"); err == nil {
		t.Fatal("expected chain error from Send(Leaf), got nil")
	}
	if v, ok := p.Sess.EnvOverlay("TOKEN"); ok {
		t.Errorf("TOKEN survived rollback: got %q (overlay should be reset)", v)
	}
}
