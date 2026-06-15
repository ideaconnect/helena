# internal/auth — Workflow

## How auth threads through a send

The end-to-end flow for an authenticated request, from button-press to
wire-bytes:

1. User presses **Send** in the UI. The UI hands the `model.Request` plus
   the active `*vars.Resolver` to `httpclient.New(...).Do(ctx, req, res)`.
2. `httpclient.Build` runs variable substitution on URL, body, headers,
   and params. It then calls `auth.ResolveValues(r.Auth, resolve)` so any
   `{{vars}}` inside auth credential fields participate in the same
   missing-name accumulator. Unresolved names from auth surface alongside
   any from URL/headers/body in a single `unresolved variables: ...`
   error.
3. Build constructs the `*http.Request`, applies headers / params / body,
   and then calls `auth.Apply(req, resolvedAuth)`. Apply is the *last*
   mutator before Do executes the request — so explicit user-set
   `Authorization` or matching API-Key headers always win.
4. Once Apply returns, the request is on the wire as normal.

Resolve is **not** called inside Build. The session has the
folder/collection chain, so the caller is expected to flatten `Inherit`
into a concrete `model.Auth` before constructing or sending the request.
Task 7.1b will add the `session.EffectiveAuth(req)` helper that wires
this in for UI sends; until then, requests can use a literal `model.Auth`
(no inheritance) or set `Auth.Type = AuthNone`.

## Resolving inheritance

`Resolve(reqAuth, ancestors)`:

1. If `reqAuth.Type` is anything other than `Inherit` (or the zero
   value), it wins outright. The ancestor chain is ignored entirely.
2. Otherwise walk `ancestors` in the supplied order. Callers pass the
   innermost folder first, the collection root last.
3. The first ancestor with a non-Inherit `Type` is returned wholesale —
   its sub-struct (Basic / Bearer / API-Key / OAuth2) carries along.
4. When the chain is exhausted with no concrete value, return
   `model.Auth{Type: AuthNone}`. The fallback exists so a downstream
   `Apply` call is always meaningful even if the request never had auth
   configured anywhere on the tree.

## Substituting `{{vars}}`

`ResolveValues(a, resolve)` returns a deep copy with every string field
on the active sub-struct run through `resolve`. The switch covers exactly
the fields documented in `internal/model/STRUCTURE.md` for each Type, so
adding a new field to (say) `OAuth2Auth` means revisiting `ResolveValues`
to include it in the substitution pass.

Why a copy rather than mutating in place: the original `a` is typically
the value read off the model, which the UI is also holding. Mutating in
place during Send would write the resolved values into the user's saved
template, replacing `{{TOKEN}}` with the literal token text on disk.

## Applying to the request

`Apply(ctx, req, a, resolver)` is a single switch on `a.Type`:

| Type | Action | User-header guard? |
| --- | --- | --- |
| `""` / `None` / `Inherit` | no-op | n/a |
| `Basic` | `Authorization: Basic <base64>` | yes — skip if `Authorization` already set |
| `Bearer` | `Authorization: Bearer <token>` | yes — skip if `Authorization` already set |
| `APIKey` (header) | `req.Header.Set(name, value)` | yes — skip if `name` header already set |
| `APIKey` (query) | append to `req.URL.RawQuery` | no — query keys can repeat |
| `OAuth2` (resolver nil) | return `ErrOAuth2NotImplemented` | n/a |
| `OAuth2` (resolver set) | call `resolver.Token(ctx, *a.OAuth2)` → `Authorization: Bearer <token>` | yes — skip if `Authorization` already set |
| anything else | return an error naming the unknown type | n/a |

The user-header guard exists because a power-user might want to
hand-craft an `Authorization` value (signed JWT, AWS V4 signature when
that lands) and Apply needs to step aside in that case.

The query path uses `req.URL.Query()` / `RawQuery` rather than appending
to a string, so encoding stays correct for values containing `&` or `=`.

## OAuth2 client_credentials token lifecycle

This is what happens when a request has `Auth.Type == AuthOAuth2`,
`Auth.OAuth2.Grant == OAuth2ClientCredentials`, and `Apply` is called
with the default `cachingResolver`:

1. `Apply` calls `resolver.Token(ctx, *a.OAuth2)`.
2. `cachingResolver.Token` short-circuits if the grant isn't
   `client_credentials` — returning `ErrOAuth2NotImplemented` so
   misconfigured grants fail loudly.
3. The cache key is `CacheKey(namespace, a)` where `namespace` is
   typically the active collection directory. Sharing tokens across
   collections that happen to point at the same token URL would be a
   workspace-level mistake, so namespacing is mandatory.
4. If a cached `TokenEntry` exists and `time.Until(ExpiresAt) > 30s`,
   its `AccessToken` is returned directly. The 30-second skew avoids
   sending a token that the server is about to refuse.
5. Otherwise `FetchClientCredentialsToken(ctx, client, a)` runs:
   - Builds an `application/x-www-form-urlencoded` body with
     `grant_type=client_credentials` + `client_id`, optional
     `client_secret`, `scope`, `audience`.
   - POSTs to `a.TokenURL` via the supplied `*http.Client`.
   - On non-2xx, returns an error including the response body so the
     user sees what the token endpoint complained about
     (`invalid_client: bad secret`).
   - On 2xx, decodes `tokenResponse` (tolerating `expires_in` as
     either number or stringified seconds via `json.Number`), defaults
     missing `expires_in` to 3600s, and returns a `TokenEntry`.
6. The fresh entry goes into the cache under the namespaced key, and
   its `AccessToken` is returned.
7. `Apply` writes `Authorization: Bearer <AccessToken>` on the
   outgoing request.

The "Clear cached tokens" button on the OAuth2 panel of the UI Auth
tab calls `Session.TokenCache().ClearNamespace(ActiveCollectionDir())`
so a user who rotated a client secret can force the next Send to refetch
— scoped to the active collection's namespace so other collections'
cached tokens are untouched. `ClearAll` remains for a deliberate global
logout but is not wired to the per-request button.

## OAuth2 authorization_code lifecycle (+ optional PKCE)

When the request's resolved auth is OAuth2 with grant
`authorization_code` and the resolver was built with a non-nil
`AuthCodeStarter`:

1. `Apply` calls `resolver.Token(ctx, *a.OAuth2)`.
2. `cachingResolver.Token` dispatches to `authorizationCodeToken`.
3. Cache lookup keyed by `CacheKey(namespace, a)`. A live token (more
   than the 30-second skew from expiry) is returned directly — the
   user is not bothered with a browser tab.
4. Cache miss: generate a 24-byte random `state` for CSRF protection.
   If `a.UsePKCE`, also generate a 48-byte `code_verifier` and its
   SHA-256 `code_challenge` (base64-url, no padding — RFC 7636 §4.2).
5. `pickListenAddr(a.RedirectURI)` chooses the bind address:
   - Empty RedirectURI → `127.0.0.1:0` and a generated callback URI.
   - User-set URI → host must be `localhost` / `127.0.0.1` / `::1`,
     port must be explicit. Non-loopback hosts fail fast (Helena
     can't forward callbacks to a public host).
6. `net.Listen("tcp", addr)` binds the listener; an HTTP server is
   started on it in a goroutine. The handler validates `state`,
   surfaces `error` / `error_description` from the query, captures
   `code`, and renders a "Authorization complete." page so the user
   knows to come back to Helena.
7. `buildAuthCodeURL` assembles the auth URL with `response_type=code`,
   `client_id`, `redirect_uri`, `scope`, `audience`, `state`, and
   (when PKCE is on) `code_challenge` + `code_challenge_method=S256`.
8. `starter.OpenAuthURL(authURL)` launches the browser. UI plugs in
   `fyne.CurrentApp().OpenURL`; tests plug in a fake that GETs the
   redirect URI directly.
9. The resolver blocks on `codeCh` / `flowErrCh` / a 5-minute
   `context.WithTimeout`. Any error reaching the listener handler
   (state mismatch, `error=` callback, missing code) is forwarded
   via `flowErrCh`.
10. On `code` receipt, `exchangeAuthorizationCode` POSTs to
    `a.TokenURL` with `grant_type=authorization_code` + the same
    `redirect_uri` + `client_id` / optional `client_secret` + (when
    PKCE was used) the `code_verifier`. Same `parseTokenResponse`
    used by client_credentials handles the response.
11. The fresh `TokenEntry` goes into the cache and the access token
    returns to `Apply`, which sets `Authorization: Bearer <token>`.
12. The HTTP listener is closed by the resolver's `defer`. Any
    in-flight handler had already finished writing the "Authorization
    complete." page in step 6.

If the `AuthCodeStarter` is nil (the `NewClientCredentialsResolver`
shorthand), step 2 short-circuits with `ErrOAuth2NotImplemented`. If
`OpenAuthURL` fails, the resolver returns the underlying error
immediately rather than waiting out the 5-minute timeout.

PKCE is recommended whenever the OAuth2 server supports it; client
secrets baked into a desktop binary leak. The UI checkbox flips the
`UsePKCE` field on `model.OAuth2Auth`; the resolver does the rest.
