# internal/scripting

JavaScript runtime for per-request hooks. Each Helena request can carry a
`PreRequest` and/or `PostResponse` script; the UI's Send pipeline runs the
pre script before the request is built, the post script after the response
body is read.

## Purpose

Lets users (and other tooling) extract values from one response and feed
them into the next request without leaving Helena. The canonical example
is logging in once and pulling a token out of the JSON body into the
session-scoped environment overlay:

```js
// Post-response hook on a Login request
helena.env.set("TOKEN", response.json.token);
```

The `TOKEN` variable is then available in any subsequent request's
`{{TOKEN}}` template — for the lifetime of the Helena process. The
overlay never touches disk (see [AGENTS.md](../../AGENTS.md) invariant
9).

## Engine

[goja](https://github.com/dop251/goja) — pure-Go ECMAScript 5.1+. No CGO,
no V8, no Node API. The implementation is the same engine Bruno uses for
scripting and the de-facto standard JS runtime for Go.

Each `Run*` call constructs a fresh `goja.Runtime` so state never leaks
between requests. The only shared state is the `EnvBridge`.

## Public API

```go
func New(env EnvBridge) *Runtime

func (rt *Runtime) RunPreRequest(
    ctx context.Context,
    script string,
    r *model.Request,
    chain map[string]ChainView,
    opts ...RunOption,
) (Result, error)

func (rt *Runtime) RunPostResponse(
    ctx context.Context,
    script string,
    r model.Request,
    in ResponseInput,
    chain map[string]ChainView,
    opts ...RunOption,
) (Result, error)

// RunOptions wire host capabilities into a single run (#92). All are
// optional — with none supplied, helena.interpolate is identity,
// helena.sendRequest throws, and helena.cookies reads empty.
func WithInterpolator(fn func(string) string) RunOption
func WithRequester(fn func(SendSpec) (ResponseInput, error)) RunOption
func WithCookies(fn func(rawURL string) []Cookie) RunOption
func WithRunner(ctrl RunnerControl) RunOption // helena.runner (headless runs)

type EnvBridge interface {
    Get(name string) (string, bool)
    Set(name, value string)
}

type Result struct {
    Console []string
    Tests   []TestResult // test()/expect() outcomes (#87)
}

type ResponseInput struct {
    StatusCode int
    Status     string
    Headers    http.Header
    Body       []byte
}

type ChainView struct {
    Request  ChainRequestView
    Response ResponseInput
}

type ChainRequestView struct {
    Method string
    URL    string
    Body   []byte
}

const ScriptTimeout = 5 * time.Second
```

A nil `chain` argument binds an empty `chain` global so scripts can
safely `Object.keys(chain).length` without a type check.

## Script surface

Bound globals in both phases:

| Name | What it does |
| ---- | ------------ |
| `helena.env.get(name)` | Returns the resolved value of `name` (overlay over active env). Empty string when missing. |
| `helena.env.set(name, value)` | Writes to the in-memory overlay. Never persisted. |
| `helena.vars.get(name)` | Alias for `helena.env.get`. |
| `helena.interpolate(template)` | Resolves `{{var}}` references in `template` with the same scope chain a request send uses (global < .env < collection < env < folder/request vars < overlay < chain < dynamic) (#92). Reflects `helena.env.set` writes made earlier in the same script. Unresolved names are left best-effort by the resolver. Identity when run outside a Send (no resolver wired). |
| `helena.sendRequest({url, method, headers, body})` | Performs an ad-hoc HTTP request through the host client (#92) — same cookie jar, TLS settings, and Send context; `{{vars}}` in `url`/`headers`/`body` resolve like a normal request — and returns a response object identical to the post-response `response` global (`status`, `statusText`, `body`, `text`, `json`, `xml`, `headers`). `method` defaults to GET; set a `Content-Type` header for non-text bodies. Throws on a transport error, an invalid spec, or when called outside a Send. |
| `helena.cookies.get(url, name)` | Value of cookie `name` the host jar would send to `url`, or `undefined` (#92). |
| `helena.cookies.getAll(url)` | `{ name: value }` of every cookie the host jar would send to `url` (#92). Empty `{}` when none / outside a Send. |
| `helena.runner.stop()` | In a headless run (`helena run`), halts the run after the current request (#92). No-op in a UI Send. |
| `helena.runner.skip()` | In a headless run, skips the current request's send (#92) — pre-request only. No-op in a UI Send. |
| `helena.uuid()` | Returns a random RFC 4122 v4 UUID string. |
| `helena.hash.md5/sha1/sha256/sha512(text)` | Hex digest of `text`. |
| `helena.hash.hmacSha1/hmacSha256(key, text)` | Hex HMAC digest of `text` keyed by `key`. |
| `helena.date.now()` | Current UTC time as an ISO-8601 (RFC 3339) string. |
| `helena.date.timestamp()` | Current Unix time in seconds (number). |
| `helena.base64.encode/decode(s)` | Standard-base64 encode / decode of `s` (#92). `decode` throws on invalid input. |
| `helena.sleep(ms)` | Blocks the script for up to `ms` milliseconds (#92), clamped to the per-script `ScriptTimeout` and aborting early if the Send is cancelled. Non-positive → no-op. |
| `console.log(...args)` | Appends one line (space-joined args) to `Result.Console`. |
| `console.info(...)` | Same as `log`. |
| `console.warn(...)` | Prefixes the line with `WARN: `. |
| `console.error(...)` | Prefixes the line with `ERROR: `. |
| `test(name, fn)` | Runs `fn` and records a pass/fail into `Result.Tests` (#87). A thrown matcher error (or any throw) → fail with its message; otherwise pass. |
| `expect(actual)` | Returns a Chai-like matcher chain (each negatable via `.not`): `toBe`, `toEqual`, `toBeTruthy`, `toBeFalsy`, `toBeNull`, `toBeDefined`, `toBeUndefined`, `toContain`, `toHaveLength`, `toBeGreaterThan`, `toBeLessThan`. A failed matcher throws, so it must be called inside a `test()` to be recorded. |

Pre-request additionally binds a **mutable** `request`:

| Property | Type | Notes |
| -------- | ---- | ----- |
| `request.method` | string | Writing replaces the model's Method. |
| `request.url` | string | Vars in the URL are NOT yet resolved — write the same template form. |
| `request.body` | string | Raw body content; matches whichever BodyType is selected. |
| `request.headers` | object | Plain JS `{name: value}`. Add, modify, or `delete` keys; merges back via case-insensitive name match. |
| `request.params` | object | Same shape as headers, for query params. |
| `request.form` | object \| undefined | Bound only for `form-urlencoded` / `multipart` bodies. Same shape as headers; mutations land on `r.Body.Form` which `httpclient` prefers over `request.body` for those types. |
| `request.bodyType` | string | Read-only hint — one of `none` / `json` / `xml` / `text` / `form-urlencoded` / `multipart-form`. Useful for body-shape-aware scripts. |

Disabled rows in the model never reach the script object and pass
through unchanged on write-back.

Post-response additionally binds a read-only `request` (same shape) and a
`response`:

| Property | Type | Notes |
| -------- | ---- | ----- |
| `response.status` | int | e.g. `200` |
| `response.statusText` | string | e.g. `"200 OK"` |
| `response.headers` | object | First value per header, keys canonicalized to MIME form (`Content-Type`, `Location`); read with the canonical name. |
| `response.body` | string | Raw body. |
| `response.text` | string | Alias for `body`. |
| `response.json` | parsed | JSON object/array when the body parses; otherwise `undefined`. |
| `response.xml` | parsed | xml2js-style nested object with `$` for attrs and `_` for text. Duplicate child names become arrays. `undefined` when the body isn't XML or exceeds the safety nesting cap (256 levels). |

## The `chain` global

When a request declares `Chain []ChainStep`, the
[internal/chain](../chain/) runner executes the named predecessors
in order before the leaf runs and binds each result as
`chain.<alias>`:

```js
// In the leaf's pre-request script
request.headers["Authorization"] = "Bearer " + chain.login.response.json.token;
```

Each entry has two sub-objects:

| Property | Type | Notes |
| -------- | ---- | ----- |
| `chain.<alias>.request.method` | string | The method the predecessor was sent with (post-pre-script mutations). |
| `chain.<alias>.request.url` | string | Resolved URL the predecessor was sent to — `{{vars}}` substituted, query params merged. Matches what `httpclient.Response.RequestURL` recorded for the request. |
| `chain.<alias>.request.body` | string | Encoded wire body of the predecessor. For `form-urlencoded` bodies the URL-encoded form; for `multipart` bodies the multipart envelope; for JSON / XML / text the raw bytes the server received. Matches `httpclient.Response.RequestBody`. Empty for bodyless requests. |
| `chain.<alias>.response.*` | object | Identical shape to the top-level `response` global (status, statusText, headers, body, text, json, xml). **`json` and `xml` are lazy** — parsed on first access and cached, so a leaf script that only reads `chain.login.response.body` doesn't pay the parse cost. |

Alias scope is **per request** — when request B runs as part of A's
chain, B's own scripts see only B's declared aliases, never A's. A
never sees B's predecessors' aliases either. The `chain` global is
also bound in the post-response phase so post-scripts can correlate
the leaf's response with a predecessor's. A request with no Chain
sees `chain` as `{}` — safe to iterate with `Object.keys(chain)`.

## Timeout

Each `Run*` call is capped at `ScriptTimeout` (5s) wall-clock. The
runtime interrupts execution via `goja.Runtime.Interrupt` and surfaces
the reason as a plain Go error so the UI can show "script execution
timed out" without users decoding `goja.InterruptedError`. Context
cancellation propagates the same way.

## Security model — trust the script source

Scripts run with the **user's network identity**. The pre-request hook
can rewrite `request.url` to any host, the post-response hook can read
every response header (including `Set-Cookie`), and `helena.env.get`
can read every enabled environment variable (including ones flagged
`Secret`, which is a UI-mask hint only). This is the same trust model
as Postman, Bruno, and Insomnia — a script that ships with a
collection runs in the same authority the user has.

What this means in practice:

- **Imported collections are executable.** A pre-request script in a
  collection downloaded from the internet can exfiltrate secrets
  through `request.url` + `request.headers`, or pin-extract
  `Set-Cookie` values in a post-response hook.
- **There is no scheme / host allowlist.** Scripts can reach
  link-local metadata IPs (`169.254.169.254`), localhost services,
  and other private endpoints reachable from the user's machine.
- **The sandbox boundary stops at the network.** goja blocks
  filesystem, process-spawn, and arbitrary native calls — but it
  cannot stop a script from telling Helena's own HTTP client where to
  send the request. `helena.sendRequest` (#92) makes this explicit: it
  performs a host-mediated request through the same client + cookie jar,
  so it does not widen the boundary beyond what `request.url` rewriting
  already allowed — it just exposes it directly to scripts.
- **The curated helpers add no I/O surface.** `helena.uuid`,
  `helena.hash.*`, `helena.date.*`, `helena.base64.*`, and `helena.sleep`
  (which only delays the calling script, clamped + ctx-aware) are pure-compute (crypto/hash,
  `crypto/rand`, and the clock) — they read no files, open no sockets,
  and spawn no processes, so they don't widen the boundary above.

If you're importing a collection from an untrusted source, **read its
script bodies before pressing Send**. Helena makes them visible in the
Scripts tab precisely so you can.

## Dependencies

- `github.com/dop251/goja` — ECMAScript engine.
- `github.com/idct/helena/internal/model` — `Request`, `Scripts`,
  `KeyValue`, `Body` types.

Nothing else. Specifically the package does NOT import
`internal/session` or `internal/httpclient`; both layers consume the
runtime via the `EnvBridge` / `ResponseInput` boundaries so this package
can be tested independently of the rest of Helena.

## Keep the docs in sync

If you add an exported identifier, change the script surface, or
introduce a new runtime flow, update [STRUCTURE.md](STRUCTURE.md) and
[WORKFLOW.md](WORKFLOW.md) in the same change. The script surface table
above is the single source of truth users (and other agents) consult
when authoring hooks — it is part of the API.
