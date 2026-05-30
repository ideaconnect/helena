package integration

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/idct/helena/internal/model"
	"github.com/idct/helena/internal/session"
)

// TestOAuth2TokenCacheReusedWithinCollection verifies that a second
// Send of the same OAuth2 client_credentials request reuses the
// cached token rather than re-fetching from the token endpoint.
// This is the documented invariant of the per-Session TokenCache.
func TestOAuth2TokenCacheReusedWithinCollection(t *testing.T) {
	var tokenHits atomic.Int64
	srv := newOAuth2Server(t, &tokenHits, "cached-token")
	p := NewPipelineWithServer(t, srv)

	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			Name: "Profile", Method: model.GET, URL: srv.URL + "/profile",
			Body: model.Body{Type: model.BodyNone},
			Auth: model.Auth{
				Type: model.AuthOAuth2,
				OAuth2: &model.OAuth2Auth{
					Grant:        model.OAuth2ClientCredentials,
					TokenURL:     srv.URL + "/oauth/token",
					ClientID:     "ci",
					ClientSecret: "cs",
				},
			},
		}},
	}
	if err := p.SaveAndOpen(c); err != nil {
		t.Fatalf("SaveAndOpen: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, _, err := p.Send("Profile"); err != nil {
			t.Fatalf("Send #%d: %v", i+1, err)
		}
	}
	if got := tokenHits.Load(); got != 1 {
		t.Errorf("token endpoint hit %d times, want 1 (cache should serve after first)", got)
	}
}

// TestOAuth2TokenCacheNamespacedAcrossCollections verifies that two
// collections (different on-disk dirs) sharing the same token URL +
// client config do NOT share cached tokens — the namespace argument
// to NewClientCredentialsResolver keys each collection separately.
//
// Without this, opening a second collection with the same auth
// config would reuse the first's token even though the user might
// have rotated credentials between them.
func TestOAuth2TokenCacheNamespacedAcrossCollections(t *testing.T) {
	var tokenHits atomic.Int64
	srv := newOAuth2Server(t, &tokenHits, "shared-token")

	// Two pipelines, each with its own session + on-disk collection
	// dir. Same OAuth2 config in both. Each Send should hit the
	// token endpoint because the cache key includes the
	// collection's ActiveCollectionDir.
	auth := model.Auth{
		Type: model.AuthOAuth2,
		OAuth2: &model.OAuth2Auth{
			Grant:        model.OAuth2ClientCredentials,
			TokenURL:     srv.URL + "/oauth/token",
			ClientID:     "ci",
			ClientSecret: "cs",
		},
	}
	makeColl := func(name string) model.Collection {
		return model.Collection{
			Name: name, Auth: model.Auth{Type: model.AuthNone},
			Requests: []model.Request{{
				Name: "Profile", Method: model.GET, URL: srv.URL + "/profile",
				Body: model.Body{Type: model.BodyNone},
				Auth: auth,
			}},
		}
	}
	p1 := NewPipelineWithServer(t, srv)
	if err := p1.SaveAndOpen(makeColl("A")); err != nil {
		t.Fatalf("p1 SaveAndOpen: %v", err)
	}
	if _, _, err := p1.Send("Profile"); err != nil {
		t.Fatalf("p1 Send: %v", err)
	}

	// Second pipeline = second collection dir = different cache namespace.
	// Build it manually since the test server is shared.
	tmp := t.TempDir()
	p2 := &Pipeline{
		TmpDir:  tmp,
		CfgPath: filepath.Join(tmp, "cfg.yml"),
		CollDir: filepath.Join(tmp, "collection"),
	}
	if err := p2.SaveAndOpen(makeColl("B")); err != nil {
		t.Fatalf("p2 SaveAndOpen: %v", err)
	}
	if _, _, err := p2.Send("Profile"); err != nil {
		t.Fatalf("p2 Send: %v", err)
	}

	if got := tokenHits.Load(); got != 2 {
		t.Errorf("token endpoint hit %d times, want 2 (cache keys must be namespaced per collection)", got)
	}
}

// TestOAuth2TokenCacheClearAllForcesRefetch verifies that after
// Session.TokenCache().ClearAll() — what the UI "Clear cached tokens"
// button invokes — the next Send re-fetches.
func TestOAuth2TokenCacheClearAllForcesRefetch(t *testing.T) {
	var tokenHits atomic.Int64
	srv := newOAuth2Server(t, &tokenHits, "tok")
	p := NewPipelineWithServer(t, srv)

	c := model.Collection{
		Name: "C", Auth: model.Auth{Type: model.AuthNone},
		Requests: []model.Request{{
			Name: "X", Method: model.GET, URL: srv.URL + "/x",
			Body: model.Body{Type: model.BodyNone},
			Auth: model.Auth{
				Type: model.AuthOAuth2,
				OAuth2: &model.OAuth2Auth{
					Grant: model.OAuth2ClientCredentials, TokenURL: srv.URL + "/oauth/token",
					ClientID: "ci", ClientSecret: "cs",
				},
			},
		}},
	}
	if err := p.SaveAndOpen(c); err != nil {
		t.Fatalf("SaveAndOpen: %v", err)
	}

	for i := 0; i < 2; i++ {
		if _, _, err := p.Send("X"); err != nil {
			t.Fatalf("Send #%d: %v", i+1, err)
		}
	}
	if got := tokenHits.Load(); got != 1 {
		t.Fatalf("pre-clear: token hits = %d, want 1", got)
	}

	p.Sess.TokenCache().ClearAll()

	if _, _, err := p.Send("X"); err != nil {
		t.Fatalf("post-clear Send: %v", err)
	}
	if got := tokenHits.Load(); got != 2 {
		t.Errorf("post-clear token hits = %d, want 2 (ClearAll should force a re-fetch)", got)
	}
}

// newOAuth2Server returns an httptest.Server that responds to
// /oauth/token with an RFC 6749 client_credentials token body and
// counts hits via the supplied atomic. Other paths return 200
// "hello" so leaf requests don't 404.
func newOAuth2Server(t *testing.T, hits *atomic.Int64, accessToken string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			hits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":3600}`, accessToken)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
}

// Compile-time check that *Pipeline keeps its Sess field accessible
// for the cross-pipeline test above. If session.Session ever moves
// to an unexported type this assertion will fail at build time.
var _ = (*session.Session)(nil)
