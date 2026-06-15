package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
)

// TestClearNamespaceScopesToCollection verifies a scoped clear drops only the
// target namespace's tokens and leaves other collections' tokens intact (#96).
func TestClearNamespaceScopesToCollection(t *testing.T) {
	c := NewTokenCache()
	cfg := model.OAuth2Auth{Grant: model.OAuth2ClientCredentials, TokenURL: "https://t", ClientID: "id"}
	kA := CacheKey("/coll/A", cfg)
	kB := CacheKey("/coll/B", cfg)
	c.Set(kA, TokenEntry{AccessToken: "a"})
	c.Set(kB, TokenEntry{AccessToken: "b"})

	c.ClearNamespace("/coll/A")
	if _, ok := c.Get(kA); ok {
		t.Error("ClearNamespace left the target collection's token")
	}
	if _, ok := c.Get(kB); !ok {
		t.Error("ClearNamespace wrongly dropped another collection's token")
	}

	// A namespace with no matching keys is a no-op (B survives).
	c.ClearNamespace("/coll/does-not-exist")
	if _, ok := c.Get(kB); !ok {
		t.Error("ClearNamespace with no match dropped an unrelated token")
	}
	// Nil cache is a safe no-op across the mutators / lock.
	var nilCache *TokenCache
	nilCache.ClearNamespace("/coll/A")
	nilCache.ClearAll()
	nilCache.Clear(kA)
	nilCache.Set(kA, TokenEntry{})
	if _, ok := nilCache.Get(kA); ok {
		t.Error("nil cache Get returned ok=true")
	}
	nilCache.LockKey("x")() // returns a no-op unlock; calling it must not panic
}

// TestCacheKeyDistinguishesSensitiveInputs verifies the cache key changes when
// any token-determining input changes, so a rotated secret / changed redirect /
// toggled PKCE can't silently reuse a stale token.
func TestCacheKeyDistinguishesSensitiveInputs(t *testing.T) {
	base := model.OAuth2Auth{Grant: model.OAuth2ClientCredentials, TokenURL: "https://t", ClientID: "id", ClientSecret: "s1"}
	k := CacheKey("ns", base)
	if CacheKey("ns", base) != k {
		t.Error("CacheKey not stable for identical config")
	}
	for name, mut := range map[string]func(*model.OAuth2Auth){
		"ClientSecret": func(a *model.OAuth2Auth) { a.ClientSecret = "s2" },
		"UsePKCE":      func(a *model.OAuth2Auth) { a.UsePKCE = true },
		"RedirectURI":  func(a *model.OAuth2Auth) { a.RedirectURI = "http://localhost:9999/cb" },
		"AuthURL":      func(a *model.OAuth2Auth) { a.AuthURL = "https://auth/other" },
	} {
		c := base
		mut(&c)
		if CacheKey("ns", c) == k {
			t.Errorf("CacheKey ignores %s — change would reuse a stale token", name)
		}
	}
}

// TestResolverConcurrentTokenFetchesOnce verifies the per-key lock: many
// concurrent Token calls for the same config trigger exactly one token fetch.
func TestResolverConcurrentTokenFetchesOnce(t *testing.T) {
	srv, hits := newTokenServer(t, `{"access_token":"abc","expires_in":3600}`, http.StatusOK)
	defer srv.Close()
	r := NewClientCredentialsResolver(NewTokenCache(), srv.Client(), "ns")
	cfg := model.OAuth2Auth{Grant: model.OAuth2ClientCredentials, TokenURL: srv.URL, ClientID: "id"}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = r.Token(context.Background(), cfg) }()
	}
	wg.Wait()
	if got := atomic.LoadInt32(hits); got != 1 {
		t.Errorf("token endpoint hits = %d, want 1 (concurrent fetches must serialize)", got)
	}
}

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

// TestTokenCacheClearDropsSingleEntry verifies the per-key Clear
// removes only the named entry, leaving siblings intact.
func TestTokenCacheClearDropsSingleEntry(t *testing.T) {
	c := NewTokenCache()
	c.Set("k1", TokenEntry{AccessToken: "v1", ExpiresAt: time.Now().Add(time.Hour)})
	c.Set("k2", TokenEntry{AccessToken: "v2", ExpiresAt: time.Now().Add(time.Hour)})
	c.Clear("k1")
	if _, ok := c.Get("k1"); ok {
		t.Errorf("k1 still present after Clear")
	}
	if _, ok := c.Get("k2"); !ok {
		t.Errorf("k2 dropped by Clear(k1)")
	}
}

// TestTokenCacheNilReceiverSafe verifies the cache's nil-receiver
// guards on every method so callers can pass nil for "no cache" without
// nil-deref panics. Matches the comments on each method.
func TestTokenCacheNilReceiverSafe(t *testing.T) {
	var c *TokenCache
	if _, ok := c.Get("k"); ok {
		t.Error("nil Get returned ok=true")
	}
	c.Set("k", TokenEntry{})
	c.Clear("k")
	c.ClearAll()
}

// TestResolverUnsupportedGrantReturnsNotImplemented verifies that a
// grant the resolver doesn't know (here: a custom string) surfaces
// ErrOAuth2NotImplemented rather than silently sending no auth.
func TestResolverUnsupportedGrantReturnsNotImplemented(t *testing.T) {
	r := NewClientCredentialsResolver(NewTokenCache(), nil, "ns")
	_, err := r.(*cachingResolver).Token(context.Background(),
		model.OAuth2Auth{Grant: model.OAuth2Grant("password")})
	if !errors.Is(err, ErrOAuth2NotImplemented) {
		t.Errorf("err = %v, want ErrOAuth2NotImplemented", err)
	}
}

// TestClientCredentialsTokenEmptyURLOrClientIDErrors verifies the
// pre-flight validation: a missing TokenURL or ClientID surfaces a
// clear error before hitting the network.
func TestClientCredentialsTokenEmptyURLOrClientIDErrors(t *testing.T) {
	r := NewClientCredentialsResolver(NewTokenCache(), nil, "ns").(*cachingResolver)
	_, err := r.clientCredentialsToken(context.Background(),
		model.OAuth2Auth{Grant: model.OAuth2ClientCredentials, ClientID: "ci"})
	if err == nil || !strings.Contains(err.Error(), "token URL is empty") {
		t.Errorf("missing URL err = %v", err)
	}
	_, err = r.clientCredentialsToken(context.Background(),
		model.OAuth2Auth{Grant: model.OAuth2ClientCredentials, TokenURL: "https://x"})
	if err == nil || !strings.Contains(err.Error(), "client id is empty") {
		t.Errorf("missing ClientID err = %v", err)
	}
}

// TestParseTokenResponseErrorPaths verifies the documented failure
// modes: malformed JSON, missing access_token, and (positive case)
// missing expires_in defaulting to one hour.
func TestParseTokenResponseErrorPaths(t *testing.T) {
	if _, err := parseTokenResponse([]byte("not json")); err == nil {
		t.Error("expected error for malformed JSON")
	}
	if _, err := parseTokenResponse([]byte(`{"token_type":"Bearer"}`)); err == nil {
		t.Error("expected error for missing access_token")
	}
	te, err := parseTokenResponse([]byte(`{"access_token":"a","token_type":"Bearer"}`))
	if err != nil {
		t.Fatalf("missing expires_in: %v", err)
	}
	d := time.Until(te.ExpiresAt)
	if d < 50*time.Minute || d > 70*time.Minute {
		t.Errorf("default expiry = %v, want ~1h", d)
	}
}

// TestParseTokenResponseAcceptsStringExpiresIn verifies the
// quirks-compatibility note in parseTokenResponse: some providers
// (older Auth0) emit expires_in as a JSON string instead of number.
func TestParseTokenResponseAcceptsStringExpiresIn(t *testing.T) {
	te, err := parseTokenResponse([]byte(`{"access_token":"a","token_type":"Bearer","expires_in":"42"}`))
	if err != nil {
		t.Fatalf("string expires_in: %v", err)
	}
	d := time.Until(te.ExpiresAt)
	if d < 30*time.Second || d > 50*time.Second {
		t.Errorf("string expires_in -> %v, want ~42s", d)
	}
}

// TestPickListenAddrBranches verifies every documented behaviour of
// the redirect-URI parser: empty input picks a random port; explicit
// loopback host + port preserves the URI; non-loopback host is
// rejected; missing port is rejected; invalid URL is rejected.
func TestPickListenAddrBranches(t *testing.T) {
	addr, uri, err := pickListenAddr("")
	if err != nil || addr != "127.0.0.1:0" || uri != "" {
		t.Errorf("empty: addr=%q uri=%q err=%v", addr, uri, err)
	}

	addr, uri, err = pickListenAddr("http://localhost:9876/callback")
	if err != nil || addr != "127.0.0.1:9876" || uri != "http://localhost:9876/callback" {
		t.Errorf("localhost: addr=%q uri=%q err=%v", addr, uri, err)
	}

	if _, _, err = pickListenAddr("http://attacker.example/cb"); err == nil {
		t.Error("non-loopback should be rejected")
	}
	if _, _, err = pickListenAddr("http://localhost/cb"); err == nil {
		t.Error("missing port should be rejected")
	}
	if _, _, err = pickListenAddr("ht!tp://%%"); err == nil {
		t.Error("invalid URL should be rejected")
	}
}

// TestBuildAuthCodeURLIncludesPKCEAndAudience verifies that PKCE and
// Audience parameters land on the query string when set, and that an
// unparseable AuthURL falls back to returning the input unchanged.
func TestBuildAuthCodeURLIncludesPKCEAndAudience(t *testing.T) {
	a := model.OAuth2Auth{AuthURL: "https://idp.example/authorize", ClientID: "ci",
		Scope: "openid", Audience: "api.example"}
	got := buildAuthCodeURL(a, "http://localhost:1/cb", "state-x", "challenge-y")
	for _, want := range []string{
		"response_type=code", "client_id=ci", "redirect_uri=",
		"scope=openid", "audience=api.example", "state=state-x",
		"code_challenge=challenge-y", "code_challenge_method=S256",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("buildAuthCodeURL missing %q in %q", want, got)
		}
	}

	// Malformed AuthURL: returned unchanged (best-effort).
	bad := buildAuthCodeURL(model.OAuth2Auth{AuthURL: "::not a url::"}, "rc", "s", "c")
	if bad != "::not a url::" {
		t.Errorf("malformed url got rewritten: %q", bad)
	}
}

// TestRandomURLTokenNonPositiveByteLenErrors verifies the explicit
// guard against zero/negative byteLen rather than producing a
// zero-length token that would defeat the PKCE / state purpose.
func TestRandomURLTokenNonPositiveByteLenErrors(t *testing.T) {
	if _, err := randomURLToken(0); err == nil {
		t.Error("randomURLToken(0) should error")
	}
	if _, err := randomURLToken(-1); err == nil {
		t.Error("randomURLToken(-1) should error")
	}
	// Positive case: returns base64-url-encoded bytes.
	tok, err := randomURLToken(8)
	if err != nil {
		t.Fatalf("randomURLToken(8): %v", err)
	}
	if tok == "" {
		t.Error("randomURLToken(8) returned empty string")
	}
}

// TestExchangeAuthorizationCodeBuildErrorSurfaces verifies an invalid
// TokenURL surfaces from the http.NewRequest step rather than
// panicking later.
func TestExchangeAuthorizationCodeBuildErrorSurfaces(t *testing.T) {
	r := &cachingResolver{httpClient: http.DefaultClient}
	_, err := r.exchangeAuthorizationCode(context.Background(),
		model.OAuth2Auth{TokenURL: "ht!tp://%%"}, "code", "http://localhost/cb", "")
	if err == nil {
		t.Error("expected build error for malformed TokenURL")
	}
}

// TestExchangeAuthorizationCodeNon2xxErrors verifies a 4xx token
// response from the IdP surfaces as a clear error including the body.
func TestExchangeAuthorizationCodeNon2xxErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("invalid_grant"))
	}))
	defer srv.Close()
	r := &cachingResolver{httpClient: srv.Client()}
	_, err := r.exchangeAuthorizationCode(context.Background(),
		model.OAuth2Auth{TokenURL: srv.URL}, "c", "http://localhost/cb", "")
	if err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("err = %v, want invalid_grant in message", err)
	}
}
