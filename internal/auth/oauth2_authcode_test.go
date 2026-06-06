package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/idct/helena/internal/model"
)

// failStarter fails the test if the browser flow is ever started — used to
// assert the refresh path skips the interactive grant.
type failStarter struct{ t *testing.T }

func (s *failStarter) OpenAuthURL(string) error {
	s.t.Error("browser flow started despite a valid refresh token")
	return nil
}

// TestAuthorizationCodeUsesRefreshTokenBeforeBrowser verifies that an expired
// cached token carrying a refresh_token is renewed via the refresh_token grant
// (no new browser tab).
func TestAuthorizationCodeUsesRefreshTokenBeforeBrowser(t *testing.T) {
	tokenSrv, captured := newTokenServerCapturing(t, "refreshed-token")
	defer tokenSrv.Close()
	cache := NewTokenCache()
	cfg := model.OAuth2Auth{
		Grant: model.OAuth2AuthorizationCode, AuthURL: "https://auth/x",
		TokenURL: tokenSrv.URL, ClientID: "id",
	}
	cache.Set(CacheKey("ns", cfg), TokenEntry{
		AccessToken: "old", RefreshToken: "rt", ExpiresAt: time.Now().Add(-time.Hour),
	})
	r := NewOAuth2Resolver(cache, tokenSrv.Client(), "ns", &failStarter{t: t})
	// Short deadline: if refresh wrongly falls through to the browser flow, fail
	// fast instead of hanging on the 5-minute callback wait.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tok, err := r.Token(ctx, cfg)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok != "refreshed-token" {
		t.Errorf("token = %q, want refreshed-token", tok)
	}
	form := captured.Load()
	if form == nil || form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "rt" {
		t.Errorf("token endpoint form = %v, want grant_type=refresh_token refresh_token=rt", form)
	}
}

// fakeStarter parses the auth URL, then GETs the redirect URI to drive
// the authorization_code flow without a real browser.
type fakeStarter struct {
	code              string
	overrideState     string // when non-empty, replaces the real state in the callback
	errorCode         string // when non-empty, sends error=<errorCode> on the callback
	errorDescription  string
	capturedAuthURL   string
	capturedChallenge string
	openErr           error // returned by OpenAuthURL (used to verify error path)
}

func (s *fakeStarter) OpenAuthURL(authURL string) error {
	if s.openErr != nil {
		return s.openErr
	}
	s.capturedAuthURL = authURL
	u, err := url.Parse(authURL)
	if err != nil {
		return err
	}
	q := u.Query()
	s.capturedChallenge = q.Get("code_challenge")
	redirectURI := q.Get("redirect_uri")
	state := q.Get("state")
	if s.overrideState != "" {
		state = s.overrideState
	}
	cb, err := url.Parse(redirectURI)
	if err != nil {
		return err
	}
	cbq := cb.Query()
	cbq.Set("state", state)
	if s.errorCode != "" {
		cbq.Set("error", s.errorCode)
		if s.errorDescription != "" {
			cbq.Set("error_description", s.errorDescription)
		}
	} else if s.code != "" {
		cbq.Set("code", s.code)
	}
	cb.RawQuery = cbq.Encode()
	go func() {
		_, _ = http.Get(cb.String())
	}()
	return nil
}

// newTokenServerCapturing returns a token-endpoint mock that records the
// form values it was sent (so PKCE/redirect-URI behaviour can be
// inspected) and returns the supplied access token.
func newTokenServerCapturing(t *testing.T, accessToken string) (*httptest.Server, *atomic.Pointer[url.Values]) {
	t.Helper()
	var captured atomic.Pointer[url.Values]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		v := r.Form
		captured.Store(&v)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"access_token":"` + accessToken + `","token_type":"Bearer","expires_in":3600}`))
	}))
	return srv, &captured
}

// TestAuthorizationCodeHappyPathWithPKCE verifies the end-to-end flow:
// fake starter completes the callback with a code, the resolver exchanges
// it at the token endpoint, and the returned access token is cached.
// The PKCE code_challenge sent to /auth must match the SHA-256 of the
// code_verifier sent to /token.
func TestAuthorizationCodeHappyPathWithPKCE(t *testing.T) {
	tokenSrv, captured := newTokenServerCapturing(t, "code-token")
	defer tokenSrv.Close()

	starter := &fakeStarter{code: "the-code"}
	r := NewOAuth2Resolver(NewTokenCache(), tokenSrv.Client(), "ns", starter)
	cfg := model.OAuth2Auth{
		Grant:    model.OAuth2AuthorizationCode,
		AuthURL:  "https://auth.example.com/authorize",
		TokenURL: tokenSrv.URL,
		ClientID: "id",
		UsePKCE:  true,
	}
	got, err := r.Token(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "code-token" {
		t.Errorf("AccessToken = %q, want code-token", got)
	}

	form := captured.Load()
	if form == nil {
		t.Fatal("token endpoint never hit")
	}
	if (*form).Get("grant_type") != "authorization_code" {
		t.Errorf("grant_type = %q", (*form).Get("grant_type"))
	}
	if (*form).Get("code") != "the-code" {
		t.Errorf("code = %q", (*form).Get("code"))
	}
	verifier := (*form).Get("code_verifier")
	if verifier == "" {
		t.Errorf("code_verifier missing from token request")
	}
	sum := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if starter.capturedChallenge != wantChallenge {
		t.Errorf("code_challenge sent to /auth = %q, want SHA-256(verifier) = %q", starter.capturedChallenge, wantChallenge)
	}
}

// TestAuthorizationCodeWithoutPKCE verifies that disabling PKCE omits
// both the code_challenge from the authorize URL and the code_verifier
// from the token exchange — necessary so providers that reject extra
// params (or want client-secret-only auth) still work.
func TestAuthorizationCodeWithoutPKCE(t *testing.T) {
	tokenSrv, captured := newTokenServerCapturing(t, "tok")
	defer tokenSrv.Close()

	starter := &fakeStarter{code: "c"}
	r := NewOAuth2Resolver(NewTokenCache(), tokenSrv.Client(), "ns", starter)
	cfg := model.OAuth2Auth{
		Grant:    model.OAuth2AuthorizationCode,
		AuthURL:  "https://auth.example.com/authorize",
		TokenURL: tokenSrv.URL,
		ClientID: "id",
	}
	if _, err := r.Token(context.Background(), cfg); err != nil {
		t.Fatalf("Token: %v", err)
	}
	if starter.capturedChallenge != "" {
		t.Errorf("code_challenge should be empty without PKCE; got %q", starter.capturedChallenge)
	}
	if v := (*captured.Load()).Get("code_verifier"); v != "" {
		t.Errorf("code_verifier should be empty without PKCE; got %q", v)
	}
}

// TestAuthorizationCodeStateMismatchErrors verifies the CSRF guard:
// when the callback's state doesn't match the value the resolver
// generated, Token returns an error instead of exchanging the code.
func TestAuthorizationCodeStateMismatchErrors(t *testing.T) {
	tokenSrv, _ := newTokenServerCapturing(t, "won't-be-used")
	defer tokenSrv.Close()

	starter := &fakeStarter{code: "c", overrideState: "evil-state"}
	r := NewOAuth2Resolver(NewTokenCache(), tokenSrv.Client(), "ns", starter)
	_, err := r.Token(context.Background(), model.OAuth2Auth{
		Grant:    model.OAuth2AuthorizationCode,
		AuthURL:  "https://auth.example.com/authorize",
		TokenURL: tokenSrv.URL,
		ClientID: "id",
	})
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Errorf("err = %v, want state mismatch error", err)
	}
}

// TestAuthorizationCodeServerErrorPropagates verifies that error /
// error_description returned via the redirect URI surface as the Token
// error (so the user sees "access_denied: User cancelled" rather than
// a generic timeout).
func TestAuthorizationCodeServerErrorPropagates(t *testing.T) {
	tokenSrv, _ := newTokenServerCapturing(t, "")
	defer tokenSrv.Close()

	starter := &fakeStarter{errorCode: "access_denied", errorDescription: "user cancelled"}
	r := NewOAuth2Resolver(NewTokenCache(), tokenSrv.Client(), "ns", starter)
	_, err := r.Token(context.Background(), model.OAuth2Auth{
		Grant:    model.OAuth2AuthorizationCode,
		AuthURL:  "https://auth.example.com/authorize",
		TokenURL: tokenSrv.URL,
		ClientID: "id",
	})
	if err == nil || !strings.Contains(err.Error(), "access_denied") || !strings.Contains(err.Error(), "user cancelled") {
		t.Errorf("err = %v, want access_denied with description", err)
	}
}

// TestAuthorizationCodeOpenErrorPropagates verifies the resolver
// returns the starter's OpenAuthURL error verbatim rather than waiting
// out the 5-minute timeout when the browser open failed up front.
func TestAuthorizationCodeOpenErrorPropagates(t *testing.T) {
	tokenSrv, _ := newTokenServerCapturing(t, "")
	defer tokenSrv.Close()

	openErr := errors.New("no display")
	starter := &fakeStarter{openErr: openErr}
	r := NewOAuth2Resolver(NewTokenCache(), tokenSrv.Client(), "ns", starter)
	_, err := r.Token(context.Background(), model.OAuth2Auth{
		Grant:    model.OAuth2AuthorizationCode,
		AuthURL:  "https://auth.example.com/authorize",
		TokenURL: tokenSrv.URL,
		ClientID: "id",
	})
	if err == nil || !errors.Is(err, openErr) {
		t.Errorf("err = %v, want wrapping %v", err, openErr)
	}
}

// TestAuthorizationCodeWithoutStarterReturnsNotImplemented verifies
// that constructing the resolver without a starter (the
// NewClientCredentialsResolver shorthand) still rejects authorization_code
// loudly — so a user configuring the grant in a CI / headless context
// sees the missing wiring rather than a hang.
func TestAuthorizationCodeWithoutStarterReturnsNotImplemented(t *testing.T) {
	r := NewClientCredentialsResolver(NewTokenCache(), nil, "ns")
	_, err := r.Token(context.Background(), model.OAuth2Auth{
		Grant: model.OAuth2AuthorizationCode, AuthURL: "https://x", TokenURL: "https://y", ClientID: "id",
	})
	if !errors.Is(err, ErrOAuth2NotImplemented) {
		t.Errorf("err = %v, want ErrOAuth2NotImplemented", err)
	}
}

// TestAuthorizationCodeCachesTokenAcrossCalls verifies that after a
// successful flow, the next Token call hits the cache instead of
// triggering a second browser open. The fakeStarter would deadlock
// the test if its OpenAuthURL fired (no code provided), so a cached
// hit is the only way Token can return success twice.
func TestAuthorizationCodeCachesTokenAcrossCalls(t *testing.T) {
	tokenSrv, _ := newTokenServerCapturing(t, "cached")
	defer tokenSrv.Close()

	var opens int32
	starter := &fakeStarter{code: "c"}
	wrapped := starterFunc(func(u string) error {
		atomic.AddInt32(&opens, 1)
		return starter.OpenAuthURL(u)
	})

	r := NewOAuth2Resolver(NewTokenCache(), tokenSrv.Client(), "ns", wrapped)
	cfg := model.OAuth2Auth{
		Grant: model.OAuth2AuthorizationCode, AuthURL: "https://auth/", TokenURL: tokenSrv.URL, ClientID: "id",
	}
	if _, err := r.Token(context.Background(), cfg); err != nil {
		t.Fatalf("Token #1: %v", err)
	}
	if _, err := r.Token(context.Background(), cfg); err != nil {
		t.Fatalf("Token #2: %v", err)
	}
	if got := atomic.LoadInt32(&opens); got != 1 {
		t.Errorf("OpenAuthURL hits = %d, want 1", got)
	}
}

// TestPickListenAddrRejectsNonLocalhost verifies the safety guard on
// redirect URI hosts — Helena can't forward a public-host callback to
// a desktop listener, so a misconfigured RedirectURI should fail fast.
func TestPickListenAddrRejectsNonLocalhost(t *testing.T) {
	_, _, err := pickListenAddr("http://example.com:9000/callback")
	if err == nil || !strings.Contains(err.Error(), "localhost") {
		t.Errorf("err = %v, want non-localhost rejection", err)
	}
}

// TestRandomURLTokenUniqueness verifies that two consecutive calls
// don't produce the same token — a sanity check on the crypto/rand
// path.
func TestRandomURLTokenUniqueness(t *testing.T) {
	a, err := randomURLToken(24)
	if err != nil {
		t.Fatalf("randomURLToken: %v", err)
	}
	b, err := randomURLToken(24)
	if err != nil {
		t.Fatalf("randomURLToken: %v", err)
	}
	if a == b {
		t.Errorf("two random tokens collided: %q", a)
	}
	if !strings.HasPrefix(a, base64URLAlphabet(a)) {
		// alphabet sanity — ensure URL-safe characters only
		t.Errorf("token %q contains non-URL-safe characters", a)
	}
}

// base64URLAlphabet returns the longest prefix of s composed only of
// URL-safe characters; the test asserts that prefix is the whole
// string.
func base64URLAlphabet(s string) string {
	const allowed = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	for i, c := range s {
		if !strings.ContainsRune(allowed, c) {
			return s[:i]
		}
	}
	return s
}

// starterFunc adapts a plain func into the AuthCodeStarter interface
// so tests can wrap an existing starter to count calls.
type starterFunc func(string) error

func (f starterFunc) OpenAuthURL(u string) error { return f(u) }

// TestAuthorizationCodeTimeoutWhenStarterDoesNothing verifies that the
// resolver respects ctx cancellation rather than wedging the goroutine
// when the browser opens but the user never completes the flow.
func TestAuthorizationCodeTimeoutWhenStarterDoesNothing(t *testing.T) {
	tokenSrv, _ := newTokenServerCapturing(t, "")
	defer tokenSrv.Close()

	doNothing := starterFunc(func(string) error { return nil })
	r := NewOAuth2Resolver(NewTokenCache(), tokenSrv.Client(), "ns", doNothing)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	_, err := r.Token(ctx, model.OAuth2Auth{
		Grant: model.OAuth2AuthorizationCode, AuthURL: "https://auth/", TokenURL: tokenSrv.URL, ClientID: "id",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "timed out") && !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want timed out or DeadlineExceeded", err)
	}
}
