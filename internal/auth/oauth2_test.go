package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
)

// newTokenServer returns a fake OAuth2 token endpoint that responds with
// the supplied JSON body and HTTP status. The returned *int32 counts how
// many times the endpoint was hit so cache behaviour can be verified.
func newTokenServer(t *testing.T, body string, status int) (*httptest.Server, *int32) {
	t.Helper()
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Method != http.MethodPost {
			t.Errorf("token server: method = %s, want POST", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/x-www-form-urlencoded" {
			t.Errorf("token server: content-type = %q, want application/x-www-form-urlencoded", ct)
		}
		_ = r.ParseForm()
		if g := r.Form.Get("grant_type"); g != "client_credentials" {
			t.Errorf("token server: grant_type = %q, want client_credentials", g)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	return srv, &hits
}

// TestFetchClientCredentialsTokenHappyPath verifies that a successful
// token response is parsed into a TokenEntry with the expected fields
// and that ExpiresAt is in the future by roughly the advertised
// expires_in seconds.
func TestFetchClientCredentialsTokenHappyPath(t *testing.T) {
	srv, _ := newTokenServer(t, `{"access_token":"abc","token_type":"Bearer","expires_in":120,"refresh_token":"rt"}`, http.StatusOK)
	defer srv.Close()

	before := time.Now()
	got, err := FetchClientCredentialsToken(context.Background(), srv.Client(),
		model.OAuth2Auth{Grant: model.OAuth2ClientCredentials, TokenURL: srv.URL, ClientID: "id", ClientSecret: "secret", Scope: "read"})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got.AccessToken != "abc" {
		t.Errorf("AccessToken = %q", got.AccessToken)
	}
	if got.RefreshToken != "rt" {
		t.Errorf("RefreshToken = %q", got.RefreshToken)
	}
	wantExpires := before.Add(120 * time.Second)
	if got.ExpiresAt.Before(wantExpires.Add(-2*time.Second)) || got.ExpiresAt.After(wantExpires.Add(2*time.Second)) {
		t.Errorf("ExpiresAt = %v, want around %v", got.ExpiresAt, wantExpires)
	}
}

// TestFetchClientCredentialsTokenSurfacesErrorBody verifies that the
// token endpoint's error response body is included in the returned
// error, so users see what the server complained about.
func TestFetchClientCredentialsTokenSurfacesErrorBody(t *testing.T) {
	srv, _ := newTokenServer(t, `{"error":"invalid_client","error_description":"bad secret"}`, http.StatusUnauthorized)
	defer srv.Close()

	_, err := FetchClientCredentialsToken(context.Background(), srv.Client(),
		model.OAuth2Auth{Grant: model.OAuth2ClientCredentials, TokenURL: srv.URL, ClientID: "id"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid_client") {
		t.Errorf("error %q does not include server body", err)
	}
}

// TestResolverCachesAndReusesToken verifies that two Token() calls
// for the same OAuth2 config trigger exactly one token-endpoint hit
// — the second is served from the cache.
func TestResolverCachesAndReusesToken(t *testing.T) {
	srv, hits := newTokenServer(t, `{"access_token":"abc","expires_in":3600}`, http.StatusOK)
	defer srv.Close()

	cache := NewTokenCache()
	r := NewClientCredentialsResolver(cache, srv.Client(), "ns")
	cfg := model.OAuth2Auth{Grant: model.OAuth2ClientCredentials, TokenURL: srv.URL, ClientID: "id"}
	if _, err := r.Token(context.Background(), cfg); err != nil {
		t.Fatalf("Token #1: %v", err)
	}
	if _, err := r.Token(context.Background(), cfg); err != nil {
		t.Fatalf("Token #2: %v", err)
	}
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Errorf("token endpoint hits = %d, want 1", got)
	}
}

// TestResolverRefetchesAfterExpiry verifies that when a cached token
// is within the safety skew of expiry, the resolver re-fetches rather
// than returning the about-to-expire credential.
func TestResolverRefetchesAfterExpiry(t *testing.T) {
	srv, hits := newTokenServer(t, `{"access_token":"abc","expires_in":1}`, http.StatusOK)
	defer srv.Close()

	cache := NewTokenCache()
	r := NewClientCredentialsResolver(cache, srv.Client(), "ns")
	cfg := model.OAuth2Auth{Grant: model.OAuth2ClientCredentials, TokenURL: srv.URL, ClientID: "id"}
	if _, err := r.Token(context.Background(), cfg); err != nil {
		t.Fatalf("Token #1: %v", err)
	}
	// expires_in is 1s, skew is 30s — second call must refetch.
	if _, err := r.Token(context.Background(), cfg); err != nil {
		t.Fatalf("Token #2: %v", err)
	}
	if got := atomic.LoadInt32(hits); got != 2 {
		t.Errorf("token endpoint hits = %d, want 2", got)
	}
}

// TestResolverDifferentNamespacesDoNotShareTokens verifies that the
// CacheKey namespace prefix isolates two collections that happen to
// hit the same token URL with the same client.
func TestResolverDifferentNamespacesDoNotShareTokens(t *testing.T) {
	srv, hits := newTokenServer(t, `{"access_token":"abc","expires_in":3600}`, http.StatusOK)
	defer srv.Close()

	cache := NewTokenCache()
	cfg := model.OAuth2Auth{Grant: model.OAuth2ClientCredentials, TokenURL: srv.URL, ClientID: "id"}
	rA := NewClientCredentialsResolver(cache, srv.Client(), "/dir/a")
	rB := NewClientCredentialsResolver(cache, srv.Client(), "/dir/b")
	if _, err := rA.Token(context.Background(), cfg); err != nil {
		t.Fatalf("rA.Token: %v", err)
	}
	if _, err := rB.Token(context.Background(), cfg); err != nil {
		t.Fatalf("rB.Token: %v", err)
	}
	if got := atomic.LoadInt32(hits); got != 2 {
		t.Errorf("token endpoint hits = %d, want 2", got)
	}
}

// TestResolverRejectsUnimplementedGrant verifies that authorization_code
// (not yet wired in 7.1c) surfaces ErrOAuth2NotImplemented so the user
// learns the grant is missing rather than silently sending no auth.
func TestResolverRejectsUnimplementedGrant(t *testing.T) {
	r := NewClientCredentialsResolver(NewTokenCache(), nil, "")
	_, err := r.Token(context.Background(), model.OAuth2Auth{
		Grant: model.OAuth2AuthorizationCode, TokenURL: "https://nope",
	})
	if !errors.Is(err, ErrOAuth2NotImplemented) {
		t.Errorf("authorization_code err = %v, want ErrOAuth2NotImplemented", err)
	}
}

// TestApplyOAuth2WithResolverSetsBearer verifies that Apply, when given
// a working resolver, sets Authorization: Bearer <token> on the outgoing
// request — the end-to-end happy path.
func TestApplyOAuth2WithResolverSetsBearer(t *testing.T) {
	srv, _ := newTokenServer(t, `{"access_token":"resolved","expires_in":3600}`, http.StatusOK)
	defer srv.Close()

	cache := NewTokenCache()
	r := NewClientCredentialsResolver(cache, srv.Client(), "ns")
	req, _ := http.NewRequest(http.MethodGet, "https://api/x", nil)
	err := Apply(context.Background(), req,
		model.Auth{Type: model.AuthOAuth2, OAuth2: &model.OAuth2Auth{
			Grant: model.OAuth2ClientCredentials, TokenURL: srv.URL, ClientID: "id",
		}}, r)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got := req.Header.Get("Authorization"); got != "Bearer resolved" {
		t.Errorf("Authorization = %q, want Bearer resolved", got)
	}
}

// TestTokenCacheClearAllEmptiesEverything verifies the UI "Clear cached
// tokens" path actually drops the stored entries.
func TestTokenCacheClearAllEmptiesEverything(t *testing.T) {
	c := NewTokenCache()
	c.Set("k1", TokenEntry{AccessToken: "v1", ExpiresAt: time.Now().Add(time.Hour)})
	c.Set("k2", TokenEntry{AccessToken: "v2", ExpiresAt: time.Now().Add(time.Hour)})
	c.ClearAll()
	if _, ok := c.Get("k1"); ok {
		t.Errorf("k1 still present after ClearAll")
	}
	if _, ok := c.Get("k2"); ok {
		t.Errorf("k2 still present after ClearAll")
	}
}
