# internal/auth — Structure

## Files

| File | Responsibility |
| --- | --- |
| [auth.go](auth.go) | `Resolve`, `ResolveValues`, `Apply`, `ErrOAuth2NotImplemented`. The synchronous core. |
| [oauth2.go](oauth2.go) | `OAuth2Resolver` interface, `TokenCache`, `TokenEntry`, `CacheKey`, `NewOAuth2Resolver`, `NewClientCredentialsResolver`, `FetchClientCredentialsToken`, and the unexported `cachingResolver` (with grant-dispatch in `Token`), `tokenResponse`, `parseTokenResponse`. |
| [oauth2_authcode.go](oauth2_authcode.go) | `AuthCodeStarter` interface, `cachingResolver.authorizationCodeToken` (listener + state + PKCE + callback waiting), `exchangeAuthorizationCode`, `buildAuthCodeURL`, `pickListenAddr`, `randomURLToken`, and the `authCodeFlowTimeout` constant. |
| [auth_test.go](auth_test.go) | Unit tests covering inheritance resolution, var substitution, each Apply branch, header-conflict short-circuits, and the OAuth2-without-resolver path. |
| [oauth2_test.go](oauth2_test.go) | OAuth2 happy-path fetch + error-body surfacing + resolver caching + skew-aware refetch + namespace isolation + unimplemented-grant rejection + end-to-end Apply-with-resolver + `ClearAll`. |
| [oauth2_authcode_test.go](oauth2_authcode_test.go) | authorization_code flow tests: happy-path-with-PKCE (verifies SHA-256 match between sent challenge and exchanged verifier), no-PKCE, state mismatch, server error propagation, browser-open error, no-starter → ErrOAuth2NotImplemented, cache reuse, non-localhost rejection, random-token uniqueness, context timeout. |

## Public functions

### `Resolve(reqAuth model.Auth, ancestors []model.Auth) model.Auth`

- Short-circuits when `reqAuth` is anything but `Inherit` or the zero
  value — returns it as-is.
- Otherwise scans `ancestors` in order (caller passes innermost folder
  first, collection root last) and returns the first non-Inherit entry.
- When the chain is exhausted, returns `model.Auth{Type: AuthNone}` so
  callers never have to special-case "still Inherit".

### `ResolveValues(a model.Auth, resolve func(string) string) model.Auth`

- Returns a deep copy of `a` with every string field on the active
  sub-struct passed through `resolve`. The input is untouched, so the
  on-disk credential template (`{{TOKEN}}`) is preserved.
- A nil `resolve` is a no-op — returns `a` directly so callers can use
  the function in code paths that may or may not have a resolver yet.

### `Apply(ctx context.Context, req *http.Request, a model.Auth, resolver OAuth2Resolver) error`

- `Type == "" | AuthNone | AuthInherit` → no-op, returns nil.
- `AuthBasic` → `Authorization: Basic <base64(user:pass)>` unless the
  request already carries an explicit `Authorization` header.
- `AuthBearer` → `Authorization: Bearer <token>` with the same
  user-header-wins guard.
- `AuthAPIKey`:
  - `Placement: header` (or zero) → `req.Header.Set(name, value)`,
    skipping when the named header is already set by the user.
  - `Placement: query` → appends the key/value to the URL's query
    string via `req.URL.Query()` / `RawQuery`.
- `AuthOAuth2`:
  - `resolver == nil` → `ErrOAuth2NotImplemented`. Lets the exporter
    render the request without firing a real token fetch.
  - resolver set → `resolver.Token(ctx, *a.OAuth2)` is called; the
    returned token becomes `Authorization: Bearer <token>` (subject to
    the same user-header-wins guard).
- Any other `Type` → returns a descriptive error so unknown values fail
  loudly rather than silently sending without auth.

## Type catalog

Most types this package operates on (`model.Auth` and its sub-structs)
live in [`internal/model/auth.go`](../model/auth.go); see
[`internal/model/STRUCTURE.md`](../model/STRUCTURE.md) for those.

The OAuth2 plumbing is defined here in [oauth2.go](oauth2.go):

### `OAuth2Resolver` (interface)
One-method contract — `Token(ctx context.Context, a model.OAuth2Auth) (string, error)`. Implementations decide caching, grant support, and how to talk to the token endpoint. `Apply` calls this exactly once when it encounters an `AuthOAuth2`.

### `TokenEntry`
- `AccessToken` — the Bearer string that goes on the wire.
- `RefreshToken` — set by grants that return one (authorization_code; rare for client_credentials). Currently captured but not yet used.
- `ExpiresAt` — wall-clock instant after which the entry is stale; the resolver re-fetches once we're within ~30s of it.

### `TokenCache`
- Backed by a `sync.Mutex` + `map[string]TokenEntry`. Safe for concurrent `Get` / `Set` / `Clear` / `ClearAll`.
- Nil-safe: every method is a no-op (or zero-value) on a nil receiver, so callers can pass a single optional cache without nil guards.

### `cachingResolver` (unexported)
Default `OAuth2Resolver`. Holds a cache, an `*http.Client` for token fetches, a namespace string (for `CacheKey`), an optional `AuthCodeStarter`, and a 30-second skew. `Token` dispatches on `a.Grant`:
- `client_credentials` → `clientCredentialsToken` (cache lookup + `FetchClientCredentialsToken` on miss).
- `authorization_code` → `authorizationCodeToken` (cache lookup; on miss, generates state + PKCE, binds a localhost listener, calls `starter.OpenAuthURL`, waits for the redirect, exchanges code at the token endpoint).
- Anything else (or `authorization_code` with `starter == nil`) → `ErrOAuth2NotImplemented`.

### `AuthCodeStarter` (interface)
One-method contract: `OpenAuthURL(url string) error`. Opens the user's browser. Production wiring uses [`internal/ui/oauth2.go`](../ui/oauth2.go)'s `fyneAuthCodeStarter`; tests use a fake that GETs the redirect URI directly to drive the callback.

### `tokenResponse` (unexported)
RFC 6749 §5.1 shape: `access_token`, `token_type`, `expires_in` (`json.Number` so the parser tolerates both numeric and stringified seconds), `refresh_token`, `scope`. `parseTokenResponse` defaults missing `expires_in` to 3600 seconds.
