# exporter — Workflow

## Rendering a curl command

`ToCurl` ([exporter.go:23](exporter.go#L23)) follows the same ordering whether the input has variables, params, or a body:

1. Call `httpclient.Build(context.Background(), r, res)`. This is the same Build that `(*httpclient.Client).Do` uses.
2. On error (unresolved `{{var}}`, invalid URL), return immediately — the caller surfaces the message.
3. Call `renderCurl(req, s)` ([exporter.go:41](exporter.go#L41)).

`renderCurl` builds a `[]string` of command fragments, then joins them with ` \` + newline + two-space indent. Order:

1. `curl -X <Method> <quoted URL>` — URL goes through `alwaysQuote`.
2. Settings flags, in declaration order:
   - `-k` when `InsecureSkipVerify`.
   - `-L` when `FollowRedirects`.
   - `--max-time N` when `TimeoutSeconds > 0`.
3. Headers, sorted by key. Each value pair: `-H <shellQuote("Key: Value")>`. Multi-valued headers emit one `-H` per value.
4. Body. `readBodyBytes(req)` calls `req.GetBody()` to get a fresh reader over the body bytes. If non-empty, append `--data-raw <shellQuote(body)>`.

Wget rendering ([exporter.go:69](exporter.go#L69)) follows the same five-step shape but with wget flag names. The two non-obvious differences:

- `--max-redirect=0` is emitted only when `FollowRedirects` is **false** — wget follows redirects by default, so the flag suppresses them instead of enabling them.
- `-qO-` and the URL are appended last, in that order; the URL is the trailing positional argument.

## Reusing `httpclient.Build` for fidelity

This is the deliberate design choice that distinguishes Helena's exporter from a naive "loop over r.Headers and r.Params" implementation. By calling `httpclient.Build`, the exporter inherits:

- **`{{var}}` resolution** — done once, eagerly, before anything else.
- **Disabled-row filtering** — `EnabledPairs` is run on headers, params and form rows.
- **URL query merge** — existing `?` parameters on `r.URL` survive; enabled `Params` are appended via `url.Values.Encode`.
- **Auto Content-Type** — `BodyJSON` -> `application/json`, etc., unless the user supplied an explicit `Content-Type` header.
- **Body construction by type** — JSON/XML/Text use resolved `Content`; structured `BodyForm` URL-encodes its pairs; the fallback path on `BodyForm` uses raw `Content`; multipart returns an error.
- **`req.GetBody`** — `httpclient.Build` sets this so the exporter can re-read the body without consuming the original.

The fidelity guarantee in one sentence: the bytes that would have gone on the wire if you pressed *Send* are the bytes that appear in the exported command's `--data-raw` / `--body-data` value, and the URL string that would have been dialed is the URL the command points at.

If a `Settings` flag changes wire behavior (insecure TLS, redirects, timeout), the exporter emits the corresponding command flag too. Settings that affect only Helena's UI (e.g. `CORSWarning`) are intentionally not rendered — there is no equivalent flag in curl/wget and the warning belongs in Helena, not in the user's shell.

## Quoting rules

Three helpers in [exporter.go](exporter.go) implement POSIX shell quoting:

- **`needsShellQuote(s)`** ([exporter.go:137](exporter.go#L137)) — character whitelist: `[A-Za-z0-9-_./:+%,]`. Empty strings return `true` (they need quoting to be visible as an argument).
- **`shellQuote(s)`** ([exporter.go:123](exporter.go#L123)) — returns `s` bare when `needsShellQuote(s)` is `false`; otherwise calls `alwaysQuote`. Used for **header values** and **body content** so simple tokens (`application/json`, an opaque API key) appear unquoted and copy-paste cleanly.
- **`alwaysQuote(s)`** ([exporter.go:133](exporter.go#L133)) — `'` + `strings.ReplaceAll(s, "'", "'\\''")` + `'`. The `'\''` trick is the canonical POSIX way to embed a single quote inside a single-quoted string: end the quote, escape the apostrophe, start a new quote. Used for **URLs** (every URL is unconditionally quoted) so that exported commands look visually consistent regardless of whether the URL happens to contain shell-safe characters only.

Example behavior (from the test matrix in [exporter_test.go](exporter_test.go)):

| Input | `shellQuote` |
| --- | --- |
| `""` | `''` |
| `simple` | `simple` |
| `with space` | `'with space'` |
| `single'quote` | `'single'\''quote'` |
| `back\slash` | `'back\slash'` |
| `https://x?y=1` | `'https://x?y=1'` |
| `path/with/slashes` | `path/with/slashes` |

URLs always emit via `alwaysQuote`, so `https://example.com/x` renders as `'https://example.com/x'` even though it would survive bare.
