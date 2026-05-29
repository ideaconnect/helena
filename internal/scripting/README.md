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

func (rt *Runtime) RunPreRequest(ctx context.Context, script string, r *model.Request) (Result, error)
func (rt *Runtime) RunPostResponse(ctx context.Context, script string, r model.Request, in ResponseInput) (Result, error)

type EnvBridge interface {
    Get(name string) (string, bool)
    Set(name, value string)
}

type Result struct {
    Console []string
}

type ResponseInput struct {
    StatusCode int
    Status     string
    Headers    http.Header
    Body       []byte
}

const ScriptTimeout = 5 * time.Second
```

## Script surface

Bound globals in both phases:

| Name | What it does |
| ---- | ------------ |
| `helena.env.get(name)` | Returns the resolved value of `name` (overlay over active env). Empty string when missing. |
| `helena.env.set(name, value)` | Writes to the in-memory overlay. Never persisted. |
| `helena.vars.get(name)` | Alias for `helena.env.get`. |
| `console.log(...args)` | Appends one line (space-joined args) to `Result.Console`. |
| `console.info(...)` | Same as `log`. |
| `console.warn(...)` | Prefixes the line with `WARN: `. |
| `console.error(...)` | Prefixes the line with `ERROR: `. |

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
| `response.headers` | object | First value per header, case-insensitive (HTTP semantics). |
| `response.body` | string | Raw body. |
| `response.text` | string | Alias for `body`. |
| `response.json` | parsed | JSON object/array when the body parses; otherwise `undefined`. |
| `response.xml` | parsed | xml2js-style nested object with `$` for attrs and `_` for text. Duplicate child names become arrays. `undefined` when the body isn't XML. |

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
  send the request.

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
