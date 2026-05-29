# internal/scripting — structure

## Files

| File | Purpose |
| ---- | ------- |
| [scripting.go](scripting.go) | Package doc, public types (`Runtime`, `Result`, `EnvBridge`, `ResponseInput`), constructor `New`, and the two entry points `RunPreRequest` / `RunPostResponse`. |
| [bindings.go](bindings.go) | All internal binding helpers: `bindHelena`, `bindConsole`, `stringify`, `runWithTimeout`, `requestToObject` / `writeBackRequest` / `mergeKVFromObject`, `responseToObject`, `tryParseJSON`. |
| [xml.go](xml.go) | `tryParseXML` and the recursive `readXMLElement` helper that converts response XML bodies into a JS-friendly nested map. |
| [scripting_test.go](scripting_test.go) | The full behavioural suite — 18 tests covering both phases, env bridge writes, console capture, JSON / XML parsing, timeout, cancellation, error propagation. |

## Public types

### `Runtime` ([scripting.go](scripting.go))

Owns the shared `EnvBridge`. Safe to reuse across requests because each
`Run*` call constructs its own `goja.Runtime` — there is no per-runtime
script state, no module cache, no globals carried between calls.

### `EnvBridge` ([scripting.go](scripting.go))

```go
type EnvBridge interface {
    Get(name string) (string, bool)
    Set(name, value string)
}
```

The session adapter (`sessionEnvBridge` in [internal/ui/shell.go](../ui/shell.go)) is the
production implementation. The test fixture `fakeBridge` in
[scripting_test.go](scripting_test.go) is a hash-map double.

A `nil` bridge passed to `New` is replaced by `nopBridge` so callers
that don't care about env (tests, dry-runs) don't have to construct a
stub.

### `Result` ([scripting.go](scripting.go))

```go
type Result struct {
    Console []string
}
```

One slice entry per `console.log` / `info` / `warn` / `error` call.
Errors thrown inside the script are returned as the `error` return
from `Run*`; the partial `Console` accumulated before the throw still
comes back so the UI can show users how far the script got.

### `ResponseInput` ([scripting.go](scripting.go))

```go
type ResponseInput struct {
    StatusCode int
    Status     string      // "200 OK"
    Headers    http.Header
    Body       []byte
}
```

A small input struct rather than `*http.Response` so the package stays
free of net/http server-side semantics and can be driven directly from
tests.

## Internal helpers

| Helper | What it does |
| ------ | ------------ |
| `bindHelena` | Attaches `helena.env.{get,set}` and `helena.vars.get` to the VM. All three flow through `Runtime.env`. |
| `bindConsole` | Attaches `console.{log,info,warn,error}`. Each emits one space-joined line into `Result.Console`. |
| `stringify` | Turns a `goja.Value` into a console line: strings pass through, `null` / `undefined` become their names, everything else is JSON-encoded so `console.log({a:1})` shows useful structure. |
| `runWithTimeout` | Wraps `vm.RunString` with a `ScriptTimeout` watchdog and a ctx-cancel watcher. The watcher goroutine calls `vm.Interrupt` and stores the reason behind a mutex so the error returned upward names the cause (timeout vs cancel vs script-thrown). |
| `requestToObject` | Builds a fresh `goja.Object` mirroring the model's Method, URL, Body content, enabled headers (as flat `{name: value}`), and enabled params. |
| `writeBackRequest` | Reads the (possibly mutated) JS request object back into `*model.Request`. Scalars are direct writes; headers and params go through `mergeKVFromObject`. |
| `mergeKVFromObject` | Reconciles the existing `[]KeyValue` slice with the post-script JS object: disabled rows pass through unchanged, enabled rows present in the object are updated, enabled rows missing from the object are dropped (the user called `delete`), and new object keys append as enabled rows. Case-insensitive matching keeps HTTP header semantics. |
| `responseToObject` | Builds the read-only response object — scalars, headers, body / text, plus `json` (when body parses) and `xml` (when body parses). |
| `tryParseJSON` | Returns parsed value + true when the body starts with `{` or `[` and decodes cleanly. Top-level scalars are rejected so `response.json` stays undefined for non-API bodies. |
| `tryParseXML` | Calls `readXMLElement` on the first `StartElement` token; returns nil + false on any decoder error. |
| `readXMLElement` | Recursively consumes XML tokens until the matching end tag, building a map with `$` for attributes, `_` for trimmed text content, and child element names mapped to either a single object or an array (when the same name appears more than once). |

## Constants

| Name | Value | Why |
| ---- | ----- | --- |
| `ScriptTimeout` | `5 * time.Second` | Long enough for slow JSON-parsing scripts under load; short enough that a hung script doesn't make the UI feel frozen. The same cap is enforced on pre- and post-response hooks. |

## What is NOT here

- No script storage. Helena keeps the source on the model's `Scripts`
  field; storage round-trips it via [internal/storage/opencollection.go](../storage/opencollection.go).
- No UI. The script editors and console panel live in
  [internal/ui/scripts.go](../ui/scripts.go).
- No Fyne dependency. The Runtime is GUI-free so it stays unit-testable
  without a Fyne app.
- No CGO. goja is pure Go.
