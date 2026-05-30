package integration

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/idct/helena/internal/model"
)

// TestRoundTripFullPipeline verifies the storage ↔ session ↔ chain
// ↔ httpclient pipeline survives a save → reload → send round-trip
// with a non-trivial collection: a folder Bearer that an Inherit
// request picks up, a chain ref pinned by ID, and a leaf pre-script
// that reads chain.<alias>.response.json.
func TestRoundTripFullPipeline(t *testing.T) {
	mu := sync.Mutex{}
	var loginAuth, profileAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		switch r.URL.Path {
		case "/login":
			loginAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"chain-token"}`))
		case "/profile":
			profileAuth = r.Header.Get("Authorization")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"user":"alice"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
		mu.Unlock()
	}))
	p := NewPipelineWithServer(t, srv)

	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Folders: []model.Folder{{
			Name: "Auth",
			Auth: model.Auth{Type: model.AuthBearer, Bearer: &model.BearerAuth{Token: "folder-token"}},
			Requests: []model.Request{{
				Name: "Login", Method: model.POST, URL: srv.URL + "/login",
				Body: model.Body{Type: model.BodyNone},
				Auth: model.Auth{Type: model.AuthInherit},
			}},
		}},
		Requests: []model.Request{{
			Name: "Profile", Method: model.GET, URL: srv.URL + "/profile",
			Body: model.Body{Type: model.BodyNone},
			Auth: model.Auth{Type: model.AuthInherit},
			Chain: []model.ChainStep{{
				Alias: "login", Request: "Auth/Login",
			}},
			Scripts: model.Scripts{
				PreRequest: `request.headers["Authorization"] = "Bearer " + chain.login.response.json.token;`,
			},
		}},
	}
	if err := p.SaveAndOpen(c); err != nil {
		t.Fatalf("SaveAndOpen: %v", err)
	}
	// Force a reload so we exercise the disk round-trip (info.id +
	// chain refs + folder auth all rehydrate from YAML).
	if err := p.Reopen(); err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	view, _, err := p.Send("Profile")
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if view.Response.StatusCode != 200 {
		t.Errorf("leaf status = %d, want 200", view.Response.StatusCode)
	}
	mu.Lock()
	defer mu.Unlock()
	if loginAuth != "Bearer folder-token" {
		t.Errorf("/login Authorization = %q, want Bearer folder-token (folder inheritance broken)", loginAuth)
	}
	if profileAuth != "Bearer chain-token" {
		t.Errorf("/profile Authorization = %q, want Bearer chain-token (chain → script wiring broken)", profileAuth)
	}
}

// TestConcurrentSendsAreRaceClean exercises N goroutines hitting the
// pipeline at once, each Send chaining + scripting + writing to the
// shared env overlay. The race detector catches any cross-goroutine
// state mutation outside the documented overlayMu. Run with
// `go test -race ./integration/...`.
func TestConcurrentSendsAreRaceClean(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping under -short")
	}
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	p := NewPipelineWithServer(t, srv)

	// 5 distinct leaves, each chaining to a shared "Bootstrap". Each
	// Send drives its own pre-script that writes a per-Send env var.
	bootstrap := model.Request{
		Name: "Bootstrap", Method: model.POST, URL: srv.URL + "/bootstrap",
		Body: model.Body{Type: model.BodyNone}, Auth: model.Auth{Type: model.AuthInherit},
	}
	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone}, Requests: []model.Request{bootstrap},
	}
	for i := 0; i < 5; i++ {
		c.Requests = append(c.Requests, model.Request{
			Name: leafName(i), Method: model.GET, URL: srv.URL + "/leaf",
			Body: model.Body{Type: model.BodyNone}, Auth: model.Auth{Type: model.AuthInherit},
			Chain: []model.ChainStep{{Alias: "boot", Request: "Bootstrap"}},
		})
	}
	if err := p.SaveAndOpen(c); err != nil {
		t.Fatalf("SaveAndOpen: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			view, _, err := p.Send(leafName(idx))
			if err != nil {
				t.Errorf("Send %d: %v", idx, err)
				return
			}
			if view.Response.StatusCode != 200 {
				t.Errorf("Send %d: status = %d", idx, view.Response.StatusCode)
			}
		}(i)
	}
	wg.Wait()
	// Each leaf triggers its own chain (Bootstrap) + the leaf itself
	// = 2 hits per leaf × 5 leaves = 10 total.
	if got := hits.Load(); got != 10 {
		t.Errorf("hits = %d, want 10 (5 leaves × 2 hits each)", got)
	}
}

func leafName(i int) string {
	return "Leaf" + string(rune('A'+i))
}
