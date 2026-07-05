# httpclient — Structure

## Files

| File | Responsibility |
| --- | --- |
| [doc.go](doc.go) | Package-level doc comment. |
| [httpclient.go](httpclient.go) | All implementation: `Client`/`Response`/`New`, the pure `Build`, the per-body-type `buildBody` switch, the Digest/NTLM challenge handshakes (`ntlmHandshake`/`drainBody`), the SSE `Stream`/`StreamMeta` (#74), and the `corsAdvisory` helper. |
| [httpclient_test.go](httpclient_test.go) | Black-box behavior tests using `net/http/httptest`: param merging, JSON body + var resolution, unresolved-var errors, redirect policy, form-body fallback, CORS advisory matrix. |

## Type catalog

### `Client` ([httpclient.go:31](httpclient.go#L31))

Wraps a configured `*http.Client` and the `model.Settings` it was built from. The settings are retained so `Do` knows whether to compute the CORS advisory after a successful round-trip; configuration that maps onto the transport (insecure TLS, timeout, redirect policy) is materialized at construction time inside `New`.

### `Response` ([httpclient.go:19](httpclient.go#L19))

Plain result struct returned by `Do`. The body is read into memory (no streaming) so callers can inspect it repeatedly, but bounded by the effective cap — `Settings.MaxResponseBytes`, or the `MaxResponseBytes` default (100 MiB) when unset (<=0) — via `(*Client).maxResponseBytes` + `readCapped`, so a huge/hostile response can't OOM the app; `Truncated` is set when the cap clipped the body. `Size` is the byte length of `Body`; `Duration` measures the wall-clock time of the `http.Do` call plus the body read. `CORSWarning` is `""` when no advisory applies — including when `Settings.CORSWarning` is disabled.

### `Settings` interactions

`model.Settings` is read in exactly two places:

1. **`NewTransport` / `New` / `NewWithTransport`** — `NewTransport` reads `InsecureSkipVerify` to flip `tls.Config.InsecureSkipVerify` on the transport (which owns the connection pool) and sets `IdleConnTimeout` (90 s) so idle keep-alive sockets expire instead of pinning goroutines for the session; `(*Client).CloseIdleConnections` releases the pool early for short-lived owners. `New` pairs a fresh transport with a Client; `NewWithTransport` reuses a caller-cached transport so its pool survives across sends (#52). Both apply `TimeoutSeconds > 0` to `http.Client.Timeout` and install a `CheckRedirect`: it returns `http.ErrUseLastResponse` when `FollowRedirects=false`, otherwise follows (capped at 10 hops) and, once the chain leaves the originally-targeted host, drops the caller-flagged credential headers set via `SetCrossHostStripHeaders` (e.g. an API key in a header — Go already strips `Authorization`/`Cookie` cross-domain). An `https`→`http` downgrade (even to the **same** host) is treated the same way and additionally drops `Authorization`, since Go's own stripping keys only on a host change and would otherwise forward credentials over cleartext.
2. **`(*Client).Do`** — `CORSWarning` gates the call to `corsAdvisory`, which also flags credentialed (cookie-bearing) requests against a wildcard / un-credentialed `Access-Control-Allow-Origin`. Query params are appended to the URL in table order (not `url.Values.Encode`'s sorted order).

`Build` is settings-free by design: it produces the same wire request regardless of TLS/timeout/redirect choices, which is what makes it safely shareable with the exporter package.

### Helpers

- `buildBody` ([httpclient.go:174](httpclient.go#L174)) — dispatches on `model.BodyType`. JSON/XML/Text use raw `Content` plus the `ContentType()` from the model; Form uses structured `Form` pairs when present and falls back to `Content` otherwise; **Multipart** encodes the enabled `Body.Form` pairs as `multipart/form-data` via `mime/multipart` and returns the writer's `FormDataContentType()` (with the generated boundary); text fields only. **File** (#24) reads the bytes of `Body.FilePath` via `os.ReadFile` and returns them with `Body.ContentType` (default `application/octet-stream`); an empty path yields no body and a missing file is a build error. **NTLM** (#78): on a 401 inviting NTLM, `ntlmHandshake` runs the NEGOTIATE→CHALLENGE→AUTHENTICATE exchange over one keep-alive connection (`drainBody` returns each intermediate connection to the pool) and re-sends with the type-3 header. A handshake that fails at any step restores the initial 401's body (captured by `drainBodyBytes` before its connection is freed) so `Do` surfaces the original 401 instead of a read on a closed body; the Digest retry likewise closes the challenge response only once the authenticated retry supersedes it. **GraphQL** (#70) serializes `Body.Content` (the query) and `Body.GraphQLVariables` (raw JSON) into a `{"query":…,"variables":…}` envelope as `application/json`; blank variables omit the key and invalid-JSON variables are a build error.
- `corsAdvisory` ([httpclient.go:201](httpclient.go#L201)) — returns a human-readable warning when a browser would block the response: no `Access-Control-Allow-Origin`, or an `Access-Control-Allow-Origin` that is neither `*` nor an exact case-insensitive match for `Origin`. With no `Origin` header on the request, no warning is ever produced.
- `dedupe` ([httpclient.go:216](httpclient.go#L216)) — order-preserving string-set used to flatten the list of unresolved variable names before formatting the error message.
