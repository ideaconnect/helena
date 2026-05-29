# internal/chain

Per-request before-hooks. A request can declare a list of
`ChainStep`s; each step names another request in the same collection
by slash-separated path and gives the result a script-visible alias.
The chain runner executes those predecessors in order, recursively,
before the leaf request runs.

## Purpose

Lets users compose multi-step flows that the existing Send pipeline
treats as a single user action: "send Profile" implicitly runs Login
first, and Login's response is available to Profile's scripts as
`chain.login.response.json.token`. The Helena UI's address bar still
shows Profile; pressing Send executes the whole graph.

## Example

```yaml
# requests/profile.yml
info:
  name: Profile
  type: http
http:
  method: GET
  url: https://api/profile
chain:
  - alias: login
    request: Auth/Login
scripts:
  preRequest: |
    request.headers["Authorization"] = "Bearer " + chain.login.response.json.token;
```

```yaml
# requests/auth-login.yml
info:
  name: Login
  type: http
http:
  method: POST
  url: https://api/login
  body:
    type: json
    data: '{"username":"{{USER}}","password":"{{PASS}}"}'
```

Pressing Send on `Profile` runs `Login` first, captures its JSON
response under `chain.login`, then runs `Profile` with the bearer
token already plumbed through.

## Resolution rules

- **Recursive.** A step's own `Chain` expands first. `A → [B] → [C]`
  executes as `C`, `B`, `A`.
- **Per-request alias scope.** When `B` runs, its scripts see only
  `chain.<B's own aliases>`. `A` sees only `chain.<A's own aliases>`.
  Aliases never bleed across levels.
- **Order matches the YAML.** Steps execute in the order they appear
  under `chain:`.
- **Cycle detection.** A request that (transitively) lists itself
  produces a clear `chain: cycle detected through "<name>"` error
  rather than a stack overflow. The check uses a visiting set keyed
  by `Request.ID` (assigned fresh on load).
- **Failure aborts the chain.** Any pre-script, HTTP, or post-script
  error at any step stops execution; the leaf is not sent. The error
  names the offending step's alias so the user can fix it.

## Public API

```go
func Resolve(
    ctx context.Context,
    leaf model.Request,
    finder RequestFinder,
    exec Executor,
) (map[string]View, []string, error)

type View struct {
    Request  RequestView
    Response ResponseView
}

type RequestView struct {
    Method string
    URL    string
    Body   []byte
}

type ResponseView struct {
    StatusCode  int
    Status      string
    Headers     http.Header
    Body        []byte
    Size        int64
    Duration    time.Duration
    CORSWarning string
}

type Executor interface {
    ExecuteOnce(
        ctx context.Context,
        r model.Request,
        chainMap map[string]View,
    ) (View, []string, error)
}

type RequestFinder interface {
    FindRequestByPath(ref string) (model.Request, bool)
}
```

`Resolve` returns the alias→View map the leaf's own scripts should
see (built from the leaf's own `Chain`, with all nested chains
already executed), plus accumulated console output from every step
the executor ran along the way. The leaf itself is run by the
caller — the UI Send pipeline reuses the same `Executor` with the
returned chain map.

## Dependencies

- `github.com/idct/helena/internal/model` — `Request`, `ChainStep`.

That's it. The package is deliberately free of `internal/scripting`,
`internal/httpclient`, and `internal/session`; the `Executor` and
`RequestFinder` interfaces are the only seams. UI tests can drive
the runner with map-backed fakes, and the production wiring sits in
[internal/ui/shell.go](../ui/shell.go) (`chainExecutor`,
`sessionRequestFinder`).

## Keep the docs in sync

If you change the public API, the error-message shape (cycle error,
unresolved-ref error), or the alias scoping rules, update
[STRUCTURE.md](STRUCTURE.md) and [WORKFLOW.md](WORKFLOW.md) in the
same change. The scoping rules are part of the user-facing contract —
scripts written today rely on `chain.<alias>` being local to the
declaring request.
