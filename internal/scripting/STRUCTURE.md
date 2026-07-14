# internal/scripting — structure

## Files

| File | Purpose |
| ---- | ------- |
| [scripting.go](scripting.go) | Package doc, public types (`Runtime`, `Result`, `EnvBridge`, `ResponseInput`), constructor `New`, the two entry points `RunPreRequest` / `RunPostResponse` (both accept `...RunOption`), `RunOption` / `WithInterpolator` / `WithRequester` / `WithCookies` / `WithRunner` (#92), and the `SendSpec` (with `ToRequest`) + `Cookie` + `RunnerControl` types. |
| [bindings.go](bindings.go) | All internal binding helpers: `bindHelena`, `bindConsole`, `stringify`, `runWithTimeout`, `requestToObject` / `writeBackRequest` / `mergeKVFromObject`, `responseToObject`, `tryParseJSON`, `parseSendSpec` (#92). |
| [programs.go](programs.go) | `compileCached` — a process-wide compiled-`*goja.Program` cache keyed by script source. Runtimes stay per-run (isolation), but identical pre/post scripts compile once and run via `vm.RunProgram` on every fresh VM; bounded at `maxCachedPrograms` (reset-on-overflow), syntax errors are never cached. |
| [helpers.go](helpers.go) | `bindHelpers` — the curated, pure-compute `helena.uuid` / `helena.hash.*` / `helena.date.*` / `helena.base64.*` surface plus `helena.sleep` (#92) — and the local `scriptUUID` formatter. |
| [assert.go](assert.go) | `bindTest` (#87) — the `__helenaRecordTest` collector that appends to `Result.Tests`, plus the JS `testPrelude` that defines the global `test()` runner and the `expect()` matcher chain. |
| [assert_test.go](assert_test.go) | Pass/fail recording, the matcher subset + `.not`, throw-in-test, and pre-request availability. |
| [xml.go](xml.go) | `tryParseXML` and the recursive `readXMLElement` helper that converts response XML bodies into a JS-friendly nested map. |
| [helpers_test.go](helpers_test.go) | Known-answer tests for the hash/HMAC digests, UUID v4 shape + randomness, RFC 3339 date / Unix timestamp, and helper availability in the post-response phase. |
| [scripting_test.go](scripting_test.go) | The full behavioural suite — 18 tests covering both phases, env bridge writes, console capture, JSON / XML parsing, timeout, cancellation, error propagation. |
| [capture_test.go](capture_test.go) | The output-capture caps: console line/byte truncation and the test-result cap that keep a runaway script from OOMing the app. |

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
    Tests   []TestResult // test()/expect() outcomes (#87)
}

type TestResult struct {
    Name   string
    Passed bool
    Error  string
}
```

`Console` holds one entry per `console.log` / `info` / `warn` / `error`
call; `Tests` holds one entry per `test(name, fn)` call (#87), with
`Passed` false and `Error` set when an `expect()` matcher or any throw
fired inside the body. Errors thrown at the top level (outside a
`test`) are returned as the `error` return from `Run*`; the partial
`Console` + `Tests` accumulated before the throw still come back so the
UI can show users how far the script got. The UI renders `Tests` as
`PASS` / `FAIL` lines plus a summary in the Scripts console.

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
| `bindHelena` | Attaches `helena.env.{get,set}`, `helena.vars.get`, `helena.interpolate` (#92 — backed by the per-call `WithInterpolator`, identity when none is supplied), `helena.sendRequest` (#92 — backed by `WithRequester`; throws when unwired; `parseSendSpec` reads the arg object), `helena.cookies.{get,getAll}` (#92 — backed by `WithCookies`; empty when unwired), and `helena.runner.{stop,skip}` (#92 — backed by `WithRunner`; no-op when unwired) to the VM (env flows through `Runtime.env`), then calls `bindHelpers` to add the curated helper surface to the same `helena` object. |
| `bindHelpers` | Attaches `helena.uuid()`, `helena.hash.{md5,sha1,sha256,sha512,hmacSha1,hmacSha256}`, `helena.date.{now,timestamp}`, `helena.base64.{encode,decode}` (#92), and `helena.sleep(ms)` (#92). Pure-compute (crypto/hash, `crypto/rand`, clock); `sleep` only delays the calling script (clamped to `ScriptTimeout`, ctx-aware) and adds no I/O, so the sandbox boundary is unchanged. Takes the run `ctx` so `sleep` aborts on cancel/timeout. |
| `bindConsole` | Attaches `console.{log,info,warn,error}`. Each emits one space-joined line into `Result.Console`, capped at `maxConsoleLines`/`maxConsoleBytes` — past the cap one truncation marker is emitted and further lines dropped, so a runaway log loop can't OOM/freeze the app. |
| `bindTest` | Attaches `test()` / `expect()` (#87): binds the Go `__helenaRecordTest` collector (appends to `Result.Tests`, capped at `maxTestResults`) and runs `testPrelude`, the JS that defines the runner + matcher chain. |
| `stringify` | Turns a `goja.Value` into a console line: strings pass through, `null` / `undefined` become their names, everything else is JSON-encoded so `console.log({a:1})` shows useful structure. |
| `runWithTimeout` | Compiles via `compileCached` (surfacing syntax errors synchronously, exactly as `vm.RunString` formatted them) then wraps `vm.RunProgram` with a `ScriptTimeout` watchdog and a ctx-cancel watcher. The watcher goroutine calls `vm.Interrupt` and stores the reason behind a mutex so the error returned upward names the cause (timeout vs cancel vs script-thrown). |
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
