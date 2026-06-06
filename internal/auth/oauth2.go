package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/idct/helena/internal/model"
)

// OAuth2Resolver returns an access token to use as a Bearer credential for
// the supplied OAuth2 configuration. Implementations are typically caching
// (so the same config doesn't trigger repeated token fetches) and own a
// small *http.Client for the token-endpoint calls.
//
// Apply receives an OAuth2Resolver; passing nil keeps the legacy
// "OAuth2 not implemented" behaviour.
type OAuth2Resolver interface {
	Token(ctx context.Context, a model.OAuth2Auth) (string, error)
}

// TokenEntry is a cached OAuth2 token plus the wall-clock instant at which
// it expires. Helena treats ExpiresAt as an inclusive upper bound and
// re-fetches once Time.Until(ExpiresAt) drops below a small safety skew.
type TokenEntry struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

// TokenCache is a goroutine-safe in-memory map keyed by an opaque string —
// typically derived from CacheKey(collectionDir, OAuth2Auth). It carries no
// persistence; the session loses its tokens when the process exits, which
// is the intended v0.2 scope (persisted tokens need an encryption story
// and live in the backlog).
type TokenCache struct {
	mu     sync.Mutex
	tokens map[string]TokenEntry
	// keyLocks serializes token acquisition per key across all resolvers that
	// share this cache. The Send path mints a fresh resolver per request, so
	// the serialization point has to live on the shared cache — otherwise two
	// concurrent sends for the same OAuth2 config would each run the full
	// (possibly interactive) grant and, for authorization_code, open two tabs.
	keyMu    sync.Mutex
	keyLocks map[string]*sync.Mutex
}

// NewTokenCache returns an empty cache ready for concurrent use.
func NewTokenCache() *TokenCache {
	return &TokenCache{tokens: map[string]TokenEntry{}, keyLocks: map[string]*sync.Mutex{}}
}

// LockKey acquires the per-key lock and returns its unlock func. Callers wrap
// the cache-miss → fetch → store sequence in it (with a re-check after locking)
// so the same key is fetched once even under concurrent Token calls. A nil
// cache yields a no-op unlock.
func (c *TokenCache) LockKey(key string) func() {
	if c == nil {
		return func() {}
	}
	c.keyMu.Lock()
	if c.keyLocks == nil {
		c.keyLocks = map[string]*sync.Mutex{}
	}
	mu := c.keyLocks[key]
	if mu == nil {
		mu = &sync.Mutex{}
		c.keyLocks[key] = mu
	}
	c.keyMu.Unlock()
	mu.Lock()
	return mu.Unlock
}

// Get returns the entry stored under key, if any.
func (c *TokenCache) Get(key string) (TokenEntry, bool) {
	if c == nil {
		return TokenEntry{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tokens[key]
	return t, ok
}

// Set stores entry under key, overwriting any prior value.
func (c *TokenCache) Set(key string, entry TokenEntry) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens[key] = entry
}

// Clear removes the entry stored under key.
func (c *TokenCache) Clear(key string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.tokens, key)
}

// ClearAll drops every cached entry. Used by the UI "Clear cached tokens"
// button on the OAuth2 panel and on logout-like operations.
func (c *TokenCache) ClearAll() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tokens = map[string]TokenEntry{}
}

// CacheKey returns a stable string identifying an OAuth2 config under the
// given namespace (typically the collection directory). The namespace
// prevents two collections that happen to share the same token URL from
// sharing tokens — a workspace-level concern.
func CacheKey(namespace string, a model.OAuth2Auth) string {
	pkce := "0"
	if a.UsePKCE {
		pkce = "1"
	}
	// Every input that changes which token you'd receive must be in the key,
	// or a rotated secret / changed redirect / toggled PKCE would silently
	// reuse a stale token.
	return strings.Join([]string{
		namespace, string(a.Grant), a.TokenURL, a.AuthURL,
		a.ClientID, a.ClientSecret, a.RedirectURI, a.Scope, a.Audience, pkce,
	}, "|")
}

// cachingResolver is the default OAuth2Resolver implementation: looks up
// the cache, falls back to fetching via the matching grant flow, and
// stores the result. client_credentials always works; authorization_code
// is enabled only when a non-nil starter is supplied (so headless / test
// contexts can still construct a resolver). Unsupported grants return
// ErrOAuth2NotImplemented so a configured-but-unwired grant fails loudly
// instead of silently sending no auth.
type cachingResolver struct {
	cache      *TokenCache
	httpClient *http.Client
	namespace  string
	starter    AuthCodeStarter
	skew       time.Duration
}

// NewOAuth2Resolver returns a resolver that handles client_credentials and,
// when starter is non-nil, authorization_code (+ PKCE). httpClient is used
// for token-endpoint POSTs; pass nil for http.DefaultClient. namespace
// scopes the cache so two collections sharing a token URL don't share
// tokens.
func NewOAuth2Resolver(cache *TokenCache, httpClient *http.Client, namespace string, starter AuthCodeStarter) OAuth2Resolver {
	if httpClient == nil {
		// A sane default timeout (not the per-request setting) so a hung token
		// endpoint can't wedge a Send forever.
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &cachingResolver{
		cache:      cache,
		httpClient: httpClient,
		namespace:  namespace,
		starter:    starter,
		skew:       30 * time.Second,
	}
}

// NewClientCredentialsResolver is a convenience shorthand for callers that
// only need client_credentials. Equivalent to NewOAuth2Resolver with a
// nil AuthCodeStarter.
func NewClientCredentialsResolver(cache *TokenCache, httpClient *http.Client, namespace string) OAuth2Resolver {
	return NewOAuth2Resolver(cache, httpClient, namespace, nil)
}

func (r *cachingResolver) Token(ctx context.Context, a model.OAuth2Auth) (string, error) {
	switch a.Grant {
	case model.OAuth2ClientCredentials:
		return r.clientCredentialsToken(ctx, a)
	case model.OAuth2AuthorizationCode:
		return r.authorizationCodeToken(ctx, a)
	default:
		return "", ErrOAuth2NotImplemented
	}
}

func (r *cachingResolver) clientCredentialsToken(ctx context.Context, a model.OAuth2Auth) (string, error) {
	if a.TokenURL == "" {
		return "", fmt.Errorf("oauth2 client_credentials: token URL is empty")
	}
	if a.ClientID == "" {
		return "", fmt.Errorf("oauth2 client_credentials: client id is empty")
	}
	key := CacheKey(r.namespace, a)
	if t, ok := r.cache.Get(key); ok && time.Until(t.ExpiresAt) > r.skew {
		return t.AccessToken, nil
	}
	unlock := r.cache.LockKey(key)
	defer unlock()
	// Re-check: a concurrent caller may have populated the cache while we waited
	// for the per-key lock, so we don't fetch a second time.
	if t, ok := r.cache.Get(key); ok && time.Until(t.ExpiresAt) > r.skew {
		return t.AccessToken, nil
	}
	entry, err := FetchClientCredentialsToken(ctx, r.httpClient, a)
	if err != nil {
		return "", err
	}
	r.cache.Set(key, entry)
	return entry.AccessToken, nil
}

// refreshToken exchanges a stored refresh_token for a fresh access token at
// a.TokenURL (RFC 6749 §6), avoiding a re-run of the interactive grant. The
// response may omit a new refresh_token, in which case the caller keeps the old.
func (r *cachingResolver) refreshToken(ctx context.Context, a model.OAuth2Auth, refresh string) (TokenEntry, error) {
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refresh)
	if a.ClientID != "" {
		form.Set("client_id", a.ClientID)
	}
	if a.ClientSecret != "" {
		form.Set("client_secret", a.ClientSecret)
	}
	if a.Scope != "" {
		form.Set("scope", a.Scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenEntry{}, fmt.Errorf("oauth2 refresh_token: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := r.httpClient.Do(req)
	if err != nil {
		return TokenEntry{}, fmt.Errorf("oauth2 refresh_token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenEntry{}, fmt.Errorf("oauth2 refresh_token: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenEntry{}, fmt.Errorf("oauth2 refresh_token: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return parseTokenResponse(body)
}

// FetchClientCredentialsToken POSTs to a.TokenURL with
// grant_type=client_credentials and the supplied client_id / client_secret
// / scope / audience in the body (application/x-www-form-urlencoded). The
// response is parsed for access_token, token_type, expires_in,
// refresh_token (optional). Non-2xx responses surface the body as the
// error message so the user sees the token endpoint's complaint.
func FetchClientCredentialsToken(ctx context.Context, client *http.Client, a model.OAuth2Auth) (TokenEntry, error) {
	if client == nil {
		client = http.DefaultClient
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", a.ClientID)
	if a.ClientSecret != "" {
		form.Set("client_secret", a.ClientSecret)
	}
	if a.Scope != "" {
		form.Set("scope", a.Scope)
	}
	if a.Audience != "" {
		form.Set("audience", a.Audience)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenEntry{}, fmt.Errorf("oauth2 client_credentials: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return TokenEntry{}, fmt.Errorf("oauth2 client_credentials: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TokenEntry{}, fmt.Errorf("oauth2 client_credentials: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TokenEntry{}, fmt.Errorf("oauth2 client_credentials: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return parseTokenResponse(body)
}

// tokenResponse mirrors the RFC 6749 §5.1 successful response shape.
// expires_in is documented as a number but some providers (notably
// older Auth0) emit it as a string, so we accept both via json.Number.
type tokenResponse struct {
	AccessToken  string      `json:"access_token"`
	TokenType    string      `json:"token_type"`
	ExpiresIn    json.Number `json:"expires_in"`
	RefreshToken string      `json:"refresh_token"`
	Scope        string      `json:"scope"`
}

// parseTokenResponse decodes an RFC 6749 token response body into a
// TokenEntry. expires_in is interpreted as seconds from "now". Missing
// expires_in defaults to one hour, matching most providers' implicit
// behaviour.
func parseTokenResponse(body []byte) (TokenEntry, error) {
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.UseNumber()
	var tr tokenResponse
	if err := dec.Decode(&tr); err != nil {
		return TokenEntry{}, fmt.Errorf("oauth2 client_credentials: decode response: %w", err)
	}
	if tr.AccessToken == "" {
		return TokenEntry{}, fmt.Errorf("oauth2 client_credentials: response missing access_token: %s", strings.TrimSpace(string(body)))
	}
	expiresIn := int64(3600)
	if tr.ExpiresIn != "" {
		// A present value wins even when non-positive: a provider returning 0 /
		// negative means the token is already dead, so honour that (it'll be
		// treated as expired and re-fetched) rather than pretending it lasts 1h.
		if n, err := strconv.ParseInt(tr.ExpiresIn.String(), 10, 64); err == nil {
			expiresIn = n
		}
	}
	return TokenEntry{
		AccessToken:  tr.AccessToken,
		RefreshToken: tr.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(expiresIn) * time.Second),
	}, nil
}
