# exporter — Structure

## Files

| File | Responsibility |
| --- | --- |
| [doc.go](doc.go) | Package-level doc comment. |
| [exporter.go](exporter.go) | The `ToCurl` / `ToWget` renderers (`renderCurl`, `renderWget`), the body re-reader (`readBodyBytes`), the header sorter, and the three shell-quoting helpers. |
| [codegen.go](codegen.go) | The language-snippet renderers `ToFetch` / `ToPython` / `ToGo` (#95) and helpers `exportHeaders` (ordered headers incl. a synthesized Host) and `jsLit` (JSON/JS/Python string literal). All reuse `httpclient.Build` + `readBodyBytes` like the shell renderers. |
| [exporter_test.go](exporter_test.go) | End-to-end tests for both shell renderers — simple GET, JSON POST with headers, var/param resolution, settings-flag mapping, wget-specific behavior, error propagation, and shell-quoting edge cases. |
| [codegen_test.go](codegen_test.go) | Exact-output tests pinning each language generator (fetch, python, go simple / custom-client) plus a shared var-resolution check. |

## Public renderers

- `ToCurl` ([exporter.go:23](exporter.go#L23)) — builds the `*http.Request` via `httpclient.Build`, then delegates to `renderCurl`.
- `ToWget` ([exporter.go:32](exporter.go#L32)) — same flow, but emits a `wget` invocation. The Build call is identical to `ToCurl` and to `(*httpclient.Client).Do`'s first step.

Both return the same error shape as `httpclient.Build`: variable resolution failures and URL parse failures surface as `error` values; the caller decides whether to display them.

## Internal renderers

- `renderCurl` ([exporter.go:41](exporter.go#L41)) emits:
  - `curl -X METHOD <url>`
  - settings flags: `-k` (insecure), `-L` (follow redirects), `--max-time N` (timeout).
  - per-header `-H 'Key: Value'` lines in sorted-key order.
  - `--data-raw <body>` when a body is present.
  Lines are joined with ` \` + newline + two-space indent.

- `renderWget` ([exporter.go:69](exporter.go#L69)) emits:
  - `wget --method=METHOD`
  - settings flags: `--no-check-certificate`, `--max-redirect=0` (note: only added when `FollowRedirects=false` — wget *follows* redirects by default), `--timeout=N`.
  - per-header `--header='Key: Value'` lines, sorted.
  - `--body-data=<body>` when a body is present.
  - `-qO-` to write the response to stdout.
  - the URL as a trailing argument.

The headers are sorted ([exporter.go:99](exporter.go#L99)) so output is deterministic across runs.

## Body re-reading

`readBodyBytes` ([exporter.go:108](exporter.go#L108)) uses `req.GetBody()` (set by `httpclient.Build`) to obtain a fresh `io.ReadCloser` over the body bytes. `req.Body` itself is not consumed — this is what makes it safe for the renderer to walk the request without disturbing it. The `GetBody` contract from `net/http` matches exactly what the exporter needs.

## Shell-quoting strategy

Three helpers split the responsibility:

- `needsShellQuote` ([exporter.go:137](exporter.go#L137)) — fast check that returns `true` for empty strings or strings containing any character outside the shell-safe whitelist `[A-Za-z0-9-_./:+%,]`.
- `shellQuote` ([exporter.go:123](exporter.go#L123)) — the **default** quoter for header values and body content. Returns the input bare when `needsShellQuote(s)` is `false`; otherwise calls `alwaysQuote`. This keeps simple cases (`Content-Type`, `application/json`, `X-Token`) un-cluttered.
- `alwaysQuote` ([exporter.go:133](exporter.go#L133)) — unconditional POSIX single-quoting with the `'\''` escape for embedded apostrophes. Used for **URLs** ([exporter.go:42](exporter.go#L42), [exporter.go:96](exporter.go#L96)) so every exported command shows the URL the same way, regardless of whether the URL happens to contain shell-safe characters only.

The split exists because URLs always look more readable when wrapped in quotes — even when not strictly required — while bare values look cleaner for short header tokens.
