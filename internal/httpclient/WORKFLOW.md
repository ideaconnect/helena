# httpclient — Workflow

This document describes how a request travels from a `model.Request` to a `*Response`. The ordering is load-bearing: every flow that follows must keep the same sequence.

## Sending a request end-to-end (Build → Do → corsAdvisory)

Scripts (per-request JS pre / post hooks) are NOT executed by this package — the UI wraps `Do` with `scripting.RunPreRequest` and `scripting.RunPostResponse` around it. See [internal/scripting/WORKFLOW.md](../scripting/WORKFLOW.md) for that pipeline. The pre-hook mutates the `model.Request` *before* it reaches `Build`, and the post-hook reads the `*Response` *after* `Do` returns; this package stays unaware of either.

`(*Client).Do` ([httpclient.go:140](httpclient.go#L140)) is the single entry point for executing a request. The lifetime, in order, is:

1. **Variable resolution.** `Build` ([httpclient.go:57](httpclient.go#L57)) installs a closure `resolve` that wraps the supplied `*vars.Resolver`. Every string that gets used downstream — `r.URL`, body content, key/value pairs in headers, params, form fields, and auth credentials — runs through `resolve`, which also appends unresolved names to a `missing` slice.
2. **Body construction.** `buildBody(r, resolve)` produces `(body []byte, contentType string, err error)`. See "Body construction by type" below.
3. **Header / param fan-out.** `model.EnabledPairs` filters out disabled rows, then each surviving key and value is resolved into a local `kv` slice. Params are kept separate from headers because they merge into the URL's query string.
4. **Auth value substitution.** `auth.ResolveValues(r.Auth, resolve)` returns a deep copy of the request's auth with every credential string substituted. Unresolved names inside auth fields contribute to the same `missing` slice as URL/headers/body so the caller sees them all in one error.
5. **Missing-variable check.** If `dedupe(missing)` is non-empty, `Build` returns `fmt.Errorf("unresolved variables: ...")` *before* parsing the URL — the caller sees the variable names, not a URL-parse error.
6. **URL parse + query merge.** `url.Parse(rawURL)` is followed by appending each enabled `Params` entry to `u.Query()`. Pre-existing query strings on `r.URL` are preserved.
7. **`http.NewRequestWithContext`.** Method defaults to `GET` when blank. The body, if any, is wrapped in `bytes.NewReader`; `req.ContentLength` and `req.GetBody` are set so the request is safely retryable / readable by the exporter.
8. **Header application.** Each resolved header is added. `Host` is special-cased onto `req.Host`. `Content-Type` is auto-set from `buildBody`'s second return value only when the user did not provide one.
9. **Auth application.** `auth.Apply(ctx, req, resolvedAuth, oauth2)` runs *after* every user-provided header lands, so its Basic / Bearer / API-Key fall-back logic can detect existing `Authorization` (or matching API-Key) headers and step aside. OAuth2 grants delegate to the supplied `OAuth2Resolver`; a nil resolver (the exporter's case) surfaces as `auth.ErrOAuth2NotImplemented`. Errors bubble up wrapped as `auth: …`.
10. **Dispatch.** `(*Client).Do` records `start := time.Now()`, calls `c.http.Do(req)`, and defers `Body.Close()`. A transport error is passed through `sanitizeDoError` first: Go's `*url.Error` carries the resolved request URL verbatim, which may include userinfo (`user:pass@host`) or a `{{token}}`-interpolated query, so `redactURL` masks the userinfo and the query string (host + path stay visible) before the error reaches the UI status line (#112).
11. **Response capture.** `readCapped` drains the body into memory bounded by the effective cap (`Settings.MaxResponseBytes`, or the 100 MiB default when unset — see `(*Client).maxResponseBytes`), setting `Truncated` when the cap clips it; `Duration` is `time.Since(start)`. Failures during read return `"reading response body: %w"`.
12. **CORS advisory.** If `c.settings.CORSWarning` is true, `corsAdvisory(req.Header.Get("Origin"), resp.Header)` populates `out.CORSWarning`. This is the *last* step before returning.
13. **Return.** `*Response` is handed back to the caller; the HTTP body bytes, headers and timing are all already in memory.

## Variable resolution timing

Variable resolution happens **once**, up front, inside `Build`. The `resolve` closure is called eagerly on every templated string before any URL parsing or `http.Request` construction. This ordering matters for two reasons:

- The `missing` accumulator collects errors across the whole request in one pass, so a request with five unresolved vars reports all five at once rather than one-at-a-time.
- Downstream code (URL parsing, query encoding, `http.NewRequest`) sees fully concrete strings. It never inspects `{{...}}` itself.

`(*Client).Do` does not re-resolve. The body bytes captured by `Build` are stored on the request via `req.GetBody`; the exporter relies on this to round-trip the body when rendering shell commands.

## Body construction by type

`buildBody` ([httpclient.go:174](httpclient.go#L174)) is a single switch on `model.BodyType`:

| Type | Source | Content-Type |
| --- | --- | --- |
| `""` / `BodyNone` | none | none |
| `BodyJSON` | `resolve(r.Body.Content)` | `application/json` |
| `BodyXML` | `resolve(r.Body.Content)` | `application/xml` |
| `BodyText` | `resolve(r.Body.Content)` | `text/plain` |
| `BodyForm` (structured) | `url.Values{}` populated from `EnabledPairs(r.Body.Form)`, each side resolved | `application/x-www-form-urlencoded` |
| `BodyForm` (fallback) | `resolve(r.Body.Content)` | `application/x-www-form-urlencoded` |
| `BodyMultipart` | — | returns `"multipart bodies are not supported yet"` |
| anything else | `resolve(r.Body.Content)` | `""` (no auto Content-Type) |

The fallback branch for `BodyForm` exists so the raw-text body editor remains useful even when the user has not filled out structured form rows.

The returned `contentType` is advisory: it is only written into the request when the user has not already set an explicit `Content-Type` header. An explicit user header always wins.

## Settings application

`New` ([httpclient.go:38](httpclient.go#L38)) translates `model.Settings` into a single configured `*http.Client`:

- `InsecureSkipVerify=true` → custom transport with `tls.Config{InsecureSkipVerify: true}`.
- `TimeoutSeconds > 0` → `http.Client.Timeout = N * time.Second`. A value of `0` means "no timeout", matching the rest of the app's conventions.
- `FollowRedirects=false` → `CheckRedirect` returns `http.ErrUseLastResponse`, so `Do` resolves with the 3xx response intact.
- Proxy is always taken from `http.ProxyFromEnvironment`.

Settings that affect rendering only (e.g. `CORSWarning`) are kept on the `Client` and consulted inside `Do`.

## CORS advisory

`corsAdvisory` ([httpclient.go:201](httpclient.go#L201)) is intentionally non-enforcing — Helena will still return the body. It runs only when (a) `Settings.CORSWarning` is on and (b) the request had an `Origin` header. The match is:

- No `Access-Control-Allow-Origin` on the response → warn.
- `Access-Control-Allow-Origin: *` → no warn.
- `Access-Control-Allow-Origin` equals `Origin` case-insensitively → no warn.
- Anything else → warn with both values quoted for clarity.

The warning is surfaced through `Response.CORSWarning`; UI layers decide how prominently to display it.
