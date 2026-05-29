# internal/chain — structure

## Files

| File | Purpose |
| ---- | ------- |
| [chain.go](chain.go) | The whole package: types (`View`, `RequestView`, `ResponseView`), interfaces (`Executor`, `RequestFinder`), and the recursive `Resolve` + `resolveSteps` runner with cycle detection. |
| [chain_test.go](chain_test.go) | Behavioural suite — empty chain no-op, single-step, recursive ordering A→B→C, direct + indirect cycle detection, unresolved ref, duplicate alias, missing alias, executor failure propagation. |

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

The post-resolution request snapshot. Method and URL reflect any
mutations the pre-script made before the request went out.

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
}
```

Resolves a `ChainStep.Request` field — a slash-separated name path
relative to the active collection — into the model. The production
implementation is `Session.FindRequestByPath`; chain tests use a
small map-backed fake.

## Functions

| Function | Role |
| -------- | --- |
| `Resolve` | Public entry. Initializes the visiting set with the leaf's ID, delegates to `resolveSteps` for the leaf's own Chain, and returns (chainMap, console, err). |
| `resolveSteps` | Walks one request's chain. For each step: validates alias / ref, looks up the predecessor via the finder, recursively resolves its own chain, executes it via the supplied executor, and accumulates the result. Cycle detection is keyed by `Request.ID` in the shared `visiting` map. |

## What is NOT here

- No HTTP execution. `Executor` is an interface; the caller (UI Send
  pipeline) wires it to `httpclient`.
- No script binding. The chain package never imports
  `internal/scripting`; the UI adapter converts `chain.View` to
  `scripting.ChainView` before invoking `Run*`.
- No storage. The on-disk schema lives in
  [internal/storage/opencollection.go](../storage/opencollection.go)
  under the `chain:` block; ChainStep round-trips through `ocChainStep`.
- No UI. The Chain tab editor and the `chainExecutor` /
  `sessionRequestFinder` adapters live in
  [internal/ui/chain.go](../ui/chain.go) and
  [internal/ui/shell.go](../ui/shell.go).
