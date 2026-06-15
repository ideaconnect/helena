# internal/auth

Resolves and applies authentication on outgoing requests. Sits between the
domain types in [internal/model](../model/) — which carry the auth
configuration — and [internal/httpclient](../httpclient/), which executes
the request.

This package does four things:

1. **Resolve inheritance.** Walks the folder → collection ancestor chain
   for any request whose own auth is `Inherit`, picking the nearest
   non-Inherit value and falling back to `None` when nothing concrete is
   set anywhere up the tree.
2. **Substitute `{{vars}}` inside credentials.** A Bearer token of
   `{{TOKEN}}` is replaced before the header is written, so users can
   reuse env vars exactly as they do in URLs and bodies.
3. **Apply the resolved auth to an `*http.Request`.** Basic and Bearer
   set the `Authorization` header; API-Key writes to a request header or
   query parameter depending on placement; OAuth2 delegates to a
   user-supplied resolver (see below).
4. **Fetch and cache OAuth2 tokens.** A built-in `cachingResolver`
   implements both `client_credentials` and `authorization_code` (with
   optional PKCE) grants. Tokens are cached keyed by collection + auth
   config and reused until a small safety skew before expiry. The
   authorization_code flow binds an ephemeral localhost listener,
   opens the browser via an `AuthCodeStarter` (the UI plugs in
   `fyne.CurrentApp().OpenURL`), waits up to 5 minutes for the
   redirect, verifies the CSRF state, and exchanges the code for a
   token.

A user-set `Authorization` header (or matching API-Key header) always
wins over the auth-derived value, so manual escape hatches keep working.

## Public API

| Symbol | Purpose |
| --- | --- |
| `Resolve(reqAuth, ancestors) model.Auth` | Flatten Inherit by walking the ancestor chain. |
| `ResolveValues(a, resolve) model.Auth` | Return a deep copy with every credential string substituted through `resolve`. |
| `Apply(ctx, req, a, resolver) error` | Mutate the outgoing request based on the resolved auth. `resolver` is consulted only for OAuth2 grants; nil keeps the legacy `ErrOAuth2NotImplemented` behaviour. |
| `OAuth2Resolver` (interface) | One-method interface: `Token(ctx, model.OAuth2Auth) (string, error)`. |
| `TokenCache` | Goroutine-safe in-memory cache with `Get` / `Set` / `Clear` / `ClearNamespace` (scoped to a CacheKey namespace, e.g. one collection) / `ClearAll` (global). |
| `NewTokenCache() *TokenCache` | Empty cache constructor. |
| `CacheKey(namespace, a) string` | Stable cache key combining namespace (typically collection dir) with the OAuth2 config. |
| `NewOAuth2Resolver(cache, httpClient, namespace, starter) OAuth2Resolver` | Default resolver implementing both client_credentials and (when `starter != nil`) authorization_code on top of a `TokenCache`. |
| `NewClientCredentialsResolver(cache, httpClient, namespace) OAuth2Resolver` | Shorthand for `NewOAuth2Resolver(..., nil)` — pure client_credentials, no browser. |
| `AuthCodeStarter` (interface) | Single method `OpenAuthURL(url string) error`. Opens the user's browser at the authorization URL. UI plugs in a Fyne adapter; tests plug in a fake that hits the redirect URI directly. |
| `FetchClientCredentialsToken(ctx, httpClient, a) (TokenEntry, error)` | Pure token-endpoint POST; useful when callers want to bypass the cache. |
| `TokenEntry` | `{AccessToken, RefreshToken, ExpiresAt}` cached value. |
| `ErrOAuth2NotImplemented` | Returned by `Apply` when OAuth2 is configured but no resolver supports the active grant. |

## Dependencies

- Internal: [`internal/model`](../model/) — domain types only.
- External: standard library only (`context`, `encoding/base64`,
  `encoding/json`, `errors`, `fmt`, `io`, `net/http`, `net/url`,
  `strconv`, `strings`, `sync`, `time`). No third-party deps.
