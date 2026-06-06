# internal/chain — structure

## Files

| File | Purpose |
| ---- | ------- |
| [chain.go](chain.go) | The whole package: types (`View`, `RequestView`, `ResponseView`), interfaces (`Executor`, `RequestFinder`), and the recursive `Resolve` + `resolveSteps` runner with cycle detection. |
| [chainvars.go](chainvars.go) | `VarLookup` — a `vars.Resolver` fallback that resolves `{{chain.<alias>.…}}` template names against a chain-result map (JSON path / headers / status / body / request fields). Lets chain results be used as ordinary `{{variables}}` anywhere, including auth fields. |
| [chain_test.go](chain_test.go) | Behavioural suite — empty chain no-op, single-step, recursive ordering A→B→C, direct + indirect cycle detection, unresolved ref, duplicate alias, missing alias, executor failure propagation. |
| [chainvars_test.go](chainvars_test.go) | `VarLookup` across the JSON / headers / status / body / request surfaces, array indices, null/scalar-only rules, and the not-found cases (unknown alias / path, malformed JSON, missing prefix). |

## Types

### `View` ([chain.go](chain.go))

```go
type View struct {
    Request  RequestView
    Response ResponseView
}
```

A snapshot of one executed chain step — the value that lands under an
alias in the parent request's chain map and, transitively, in the
leaf's `chain.<alias>` global. View is a value type; mutating it
inside a script doesn't propagate to the next step or the next Send.

### `RequestView` ([chain.go](chain.go))

```go
type RequestView struct {
    Method string
    URL    string
    Body   []byte
}
```

The wire-level request snapshot. URL is the resolved URL (vars
substituted, query params merged) and Body is the encoded body bytes
(URL-encoded form / multipart envelope / raw bytes), both lifted
from `httpclient.Response.RequestURL` / `Response.RequestBody`.
Method reflects any pre-script mutation. Together these let leaf
scripts see what each chain step actually sent, not the
pre-resolution template.

### `ResponseView` ([chain.go](chain.go))

```go
type ResponseView struct {
    StatusCode  int
    Status      string
    Headers     http.Header
    Body        []byte
    Size        int64
    Duration    time.Duration
    CORSWarning string
}
```

Carries the full response shape used both by scripts (`StatusCode`,
`Status`, `Headers`, `Body`) and by the UI display path of the leaf
(`Size`, `Duration`, `CORSWarning`). The two display fields are
empty/zero for chain-step Views in practice but the leaf needs them,
so the type carries them uniformly.

### `Executor` ([chain.go](chain.go))

```go
type Executor interface {
    ExecuteOnce(
        ctx context.Context,
        r model.Request,
        chainMap map[string]View,
    ) (View, []string, error)
}
```

The single execution path for a request: pre-script → HTTP → post-script.
Returns the captured View, any console lines emitted during pre/post
(so the UI can render the full trace), and the first phase error if
any. The production implementation lives in
[internal/ui/shell.go](../ui/shell.go) as `chainExecutor`.

### `RequestFinder` ([chain.go](chain.go))

```go
type RequestFinder interface {
    FindRequestByPath(ref string) (model.Request, bool)
    FindRequestByID(id string) (model.Request, bool)
}
```

Resolves a `ChainStep` target. `FindRequestByID` is consulted first
when the step carries a non-empty `RequestID` (the stable Request.ID
of the target, persisted to YAML at `info.id`); `FindRequestByPath`
is the slash-separated-name fallback when the ID is empty or
doesn't match anything in the snapshot. The production implementation
is `session.ChainFinderSnapshot`, which owns both an O(1) id map and
the cloned tree for path walks. Chain tests use a small map-backed
fake with a linear by-ID scan.

### `ProgressFunc` ([chain.go](chain.go))

```go
type ProgressFunc func(step, total int, alias, name string)
```

Optional callback `Resolve` invokes once before each step's
`ExecuteOnce`. `step` is 1-based across the whole Resolve; `total`
is pre-walked upfront so callers can render `step N/total`. Runs on
the calling goroutine — UI callers must marshal to the UI thread
via `fyne.Do`.

## Functions

| Function | Role |
| -------- | --- |
| `Resolve` | Public entry. Initializes the visiting set with the leaf's ID, optionally pre-walks the chain to compute the progress total, delegates to `resolveSteps` for the leaf's own Chain, and returns (chainMap, console, err). |
| `resolveSteps` | Walks one request's chain. For each step: validates alias / ref, looks up the predecessor via `resolveTarget` (ID-first, path-fallback), recursively resolves its own chain, fires `progress` (if any), executes the step via the supplied executor, and accumulates the result. Cycle detection is keyed by `Request.ID` in the shared `visiting` map. |
| `resolveTarget` | Per-step target lookup. Prefers `step.RequestID` via `finder.FindRequestByID`, falls back to `step.Request` via `finder.FindRequestByPath`. Returning false leaves the caller to surface the standard "cannot resolve" error against the path field. |
| `countSteps` | Mirrors `resolveSteps` without executing — used by `Resolve` to compute the `total` passed to `ProgressFunc`. Same visiting / depth / step-cap semantics so the count matches the runtime walk. |

## What is NOT here

- No HTTP execution. `Executor` is an interface; the caller (UI Send
  pipeline) wires it to `httpclient`.
- No script binding. The chain package never imports
  `internal/scripting`; the UI adapter converts `chain.View` to
  `scripting.ChainView` before invoking `Run*`.
- No storage. The on-disk schema lives in
  [internal/storage/opencollection.go](../storage/opencollection.go)
  under the `chain:` block; ChainStep round-trips through `ocChainStep`.
- No UI. The Chain tab editor lives in [internal/ui/chain.go](../ui/chain.go);
  the `chainExecutor` (Executor) is in [internal/ui/shell.go](../ui/shell.go),
  and the `RequestFinder` is `session.ChainFinderSnapshot` (built by
  `session.SnapshotChainFinder`, with `nilFinder` as the no-collection fallback).
