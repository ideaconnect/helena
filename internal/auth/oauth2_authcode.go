package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/idct/helena/internal/model"
)

// AuthCodeStarter opens a browser tab at the supplied authorization URL.
// The OAuth2 authorization_code grant is interactive — Helena depends on
// the user logging in at the authorization server's UI and approving the
// requested scopes — so the starter encapsulates the only side effect the
// resolver can't perform itself (or test).
//
// UI implementations use fyne.CurrentApp().OpenURL; tests use a fake that
// hits the redirect URI directly to drive the flow.
type AuthCodeStarter interface {
	OpenAuthURL(authURL string) error
}

// authCodeFlowTimeout caps how long we'll wait for the user to complete
// the browser flow. 5 minutes covers slow logins / 2FA / approval pages
// while still failing rather than leaking a goroutine forever.
const authCodeFlowTimeout = 5 * time.Minute

// authorizationCodeToken runs the authorization_code grant end-to-end:
// generates state + (optionally) PKCE, binds a local listener on the
// redirect URI, opens the browser via the starter, waits for the callback,
// and exchanges the resulting code for a token. Cached entries skip the
// whole dance.
func (r *cachingResolver) authorizationCodeToken(ctx context.Context, a model.OAuth2Auth) (string, error) {
	if r.starter == nil {
		return "", ErrOAuth2NotImplemented
	}
	if a.AuthURL == "" {
		return "", fmt.Errorf("oauth2 authorization_code: auth URL is empty")
	}
	if a.TokenURL == "" {
		return "", fmt.Errorf("oauth2 authorization_code: token URL is empty")
	}
	if a.ClientID == "" {
		return "", fmt.Errorf("oauth2 authorization_code: client id is empty")
	}

	key := CacheKey(r.namespace, a)
	if t, ok := r.cache.Get(key); ok && time.Until(t.ExpiresAt) > r.skew {
		return t.AccessToken, nil
	}

	state, err := randomURLToken(24)
	if err != nil {
		return "", fmt.Errorf("oauth2 authorization_code: generate state: %w", err)
	}
	var codeVerifier, codeChallenge string
	if a.UsePKCE {
		codeVerifier, err = randomURLToken(48)
		if err != nil {
			return "", fmt.Errorf("oauth2 authorization_code: generate code_verifier: %w", err)
		}
		sum := sha256.Sum256([]byte(codeVerifier))
		codeChallenge = base64.RawURLEncoding.EncodeToString(sum[:])
	}

	listenAddr, redirectURI, err := pickListenAddr(a.RedirectURI)
	if err != nil {
		return "", fmt.Errorf("oauth2 authorization_code: %w", err)
	}
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return "", fmt.Errorf("oauth2 authorization_code: bind callback listener on %s: %w", listenAddr, err)
	}
	if redirectURI == "" {
		redirectURI = fmt.Sprintf("http://%s/callback", ln.Addr().String())
	}

	codeCh := make(chan string, 1)
	flowErrCh := make(chan error, 1)
	server := &http.Server{
		ReadHeaderTimeout: 10 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			q := req.URL.Query()
			if e := q.Get("error"); e != "" {
				desc := strings.TrimSpace(q.Get("error_description"))
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, "Authorization failed. You can close this tab.\n")
				select {
				case flowErrCh <- fmt.Errorf("oauth2 authorization_code: %s: %s", e, desc):
				default:
				}
				return
			}
			if q.Get("state") != state {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, "State mismatch. You can close this tab.\n")
				select {
				case flowErrCh <- fmt.Errorf("oauth2 authorization_code: state mismatch"):
				default:
				}
				return
			}
			code := q.Get("code")
			if code == "" {
				w.Header().Set("Content-Type", "text/plain; charset=utf-8")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, "Missing code. You can close this tab.\n")
				select {
				case flowErrCh <- fmt.Errorf("oauth2 authorization_code: callback missing code"):
				default:
				}
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = io.WriteString(w, "Authorization complete. You can close this tab and return to Helena.\n")
			select {
			case codeCh <- code:
			default:
			}
		}),
	}
	defer func() { _ = server.Close() }()
	go func() { _ = server.Serve(ln) }()

	authURL := buildAuthCodeURL(a, redirectURI, state, codeChallenge)
	if err := r.starter.OpenAuthURL(authURL); err != nil {
		return "", fmt.Errorf("oauth2 authorization_code: open browser: %w", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, authCodeFlowTimeout)
	defer cancel()
	var code string
	select {
	case code = <-codeCh:
	case e := <-flowErrCh:
		return "", e
	case <-waitCtx.Done():
		return "", fmt.Errorf("oauth2 authorization_code: timed out waiting for callback: %w", waitCtx.Err())
	}

	entry, err := r.exchangeAuthorizationCode(ctx, a, code, redirectURI, codeVerifier)
	if err != nil {
		return "", err
	}
	r.cache.Set(key, entry)
	return entry.AccessToken, nil
}

// pickListenAddr returns the local listener bind address and the
// redirect URI Helena will hand the authorization server. If the user
// didn't set RedirectURI we bind on a random localhost port and
// generate the URI from the listener address. If they did, we honor
// the port but refuse non-loopback hosts — Helena can't proxy traffic
// from a real public host back to the desktop.
func pickListenAddr(userSet string) (addr, redirectURI string, err error) {
	if userSet == "" {
		return "127.0.0.1:0", "", nil
	}
	u, err := url.Parse(userSet)
	if err != nil {
		return "", "", fmt.Errorf("invalid redirect URI %q: %w", userSet, err)
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return "", "", fmt.Errorf("redirect URI host must be localhost / 127.0.0.1; got %q", host)
	}
	port := u.Port()
	if port == "" {
		return "", "", fmt.Errorf("redirect URI %q must include an explicit port", userSet)
	}
	return "127.0.0.1:" + port, userSet, nil
}

// buildAuthCodeURL composes the RFC 6749 §4.1.1 authorization endpoint
// URL, preserving any query string the user already encoded into
// a.AuthURL.
func buildAuthCodeURL(a model.OAuth2Auth, redirectURI, state, codeChallenge string) string {
	u, err := url.Parse(a.AuthURL)
	if err != nil {
		return a.AuthURL // best effort; the server will reject the malformed call
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", a.ClientID)
	q.Set("redirect_uri", redirectURI)
	if a.Scope != "" {
		q.Set("scope", a.Scope)
	}
	if a.Audience != "" {
		q.Set("audience", a.Audience)
	}
	q.Set("state", state)
	if codeChallenge != "" {
		q.Set("code_challenge", codeChallenge)
		q.Set("code_challenge_method", "S256")
	}
	u.RawQuery = q.Encode()
	return u.String()
}

// exchangeAuthorizationCode trades a one-time code for an access token at
// a.TokenURL. The redirect_uri must match the one used in the initial
// authorization request. code_verifier is sent only when PKCE was used.
func (r *cachingResolver) exchangeAuthorizationCode(ctx context.Context, a model.OAuth2Auth, code, redirectURI, codeVerifier string) (TokenEntry, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", a.ClientID)
	if a.ClientSecret != "" {
		form.Set("client_secret", a.ClientSecret)
	}
	if codeVerifier != "" {
		form.Set("code_verifier", codeVerifier)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenEntry{}, fmt.Errorf("oauth2 authorization_code: build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return TokenEntry{}, fmt.Errorf("oauth2 authorization_code: token exchange: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenEntry{}, fmt.Errorf("oauth2 authorization_code: read token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenEntry{}, fmt.Errorf("oauth2 authorization_code: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return parseTokenResponse(body)
}

// randomURLToken returns a URL-safe random string with byteLen bytes of
// entropy, base64-url-encoded (so PKCE / state values are RFC 7636 §4.1
// compliant without further escaping).
func randomURLToken(byteLen int) (string, error) {
	if byteLen <= 0 {
		return "", errors.New("randomURLToken: byteLen must be positive")
	}
	b := make([]byte, byteLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
