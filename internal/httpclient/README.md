# httpclient

`httpclient` is Helena's HTTP execution layer. It turns a `model.Request` into an `*http.Request` (resolving `{{vars}}`, expanding query/header/body fields, picking a Content-Type) and dispatches it through a `net/http` client whose transport honors the user's `model.Settings` (insecure TLS, redirect policy, timeout, optional proxy from environment).

Because Helena runs as a native app and not in a browser, the CORS preflight contract does not apply. The package therefore computes an advisory string (`Response.CORSWarning`) explaining why a browser **would have** blocked the response, leaving enforcement decisions to the user.

`Build` is exported and side-effect-free: the exporter package reuses it so the rendered `curl` / `wget` commands are byte-for-byte equivalent to what `Do` would have sent.

## Public API

- `Client` — executes `model.Request` instances with behavior derived from settings.
- `New(s model.Settings) *Client` — constructs a `Client` honoring insecure TLS, redirect policy, timeout, and the response-body cap (`Settings.MaxResponseBytes`, falling back to the 100 MiB default when unset).
- `(*Client).SetOAuth2Resolver(r auth.OAuth2Resolver)` — install the resolver consulted when a request's resolved auth is OAuth2. Nil leaves OAuth2 surfacing as `ErrOAuth2NotImplemented`.
- `(*Client).Do(ctx, r, res) (*Response, error)` — builds, sends, fully reads the response, optionally attaches a CORS advisory.
- `Build(ctx, r, res, oauth2) (*http.Request, error)` — pure assembler: resolves variables, applies auth (including OAuth2 via the supplied resolver), and returns an `*http.Request`; errors name every unresolved `{{var}}`. Pass nil `oauth2` for callers like the exporter that don't want a live token fetch.
- `Response` — captured outcome of an executed request: status, headers, body bytes, size, duration, optional `CORSWarning`.

## Dependencies

- `net/http`, `net/url`, `crypto/tls` — request/response plumbing and transport configuration.
- `bytes`, `io`, `strings`, `time`, `context`, `fmt` — standard library helpers.
- [`github.com/idct/helena/internal/model`](../model) — `Request`, `Settings`, `Body`, `KeyValue`, `Method`, `BodyType.ContentType()`, `EnabledPairs`.
- [`github.com/idct/helena/internal/vars`](../vars) — `*vars.Resolver` for `{{var}}` substitution.
- [`github.com/idct/helena/internal/auth`](../auth) — `auth.ResolveValues` and `auth.Apply` substitute credential `{{vars}}` and write the resulting Authorization / API-Key onto the outgoing request.
