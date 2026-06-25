# runner — Structure

## Files

| File | Responsibility |
| --- | --- |
| [runner.go](runner.go) | The public `Report` / `RequestResult` / `Check` types and their helpers (`OK`, `Failed`, `Totals`), the `Run` entry point, and `runOne` (per-request orchestration: auth flatten → snapshots → chain → execute → assertions). |
| [exec.go](exec.go) | The `headlessExecutor` (pre-script → http → post-script, with the full resolver scope chain), the `envBridge`, the request-tree walk (`collectRequests` / `walk`), and the `enabledVars` / `chainViewToScripting` / `nilFinder` helpers. |
| [runner_test.go](runner_test.go) | End-to-end runs against an `httptest` server: mixed pass/fail assertions + a `test()` script, all-pass, a connection error, a chain step with env-overlay scripts and request variables, a throwing post-script, and `{{var}}` resolution from the active environment. |

## Type catalog

| Type | Role |
| --- | --- |
| `Report` | The result of a full run: `Results []RequestResult`. `Failed` drives the CLI exit code; `Totals` feeds the summary line. |
| `RequestResult` | One request's outcome — path, method, resolved URL, status, duration, an `Err` for a pre/http/post/chain failure, and the `Checks`. `OK` = no error and every check passed. |
| `Check` | One assertion or scripted-test outcome (`Name`, `Passed`, `Error`), unifying #88 declarative assertions with #87 `test()` results. |

## Non-trivial internals

### `headlessExecutor.executeOnce` — [exec.go:38](exec.go#L38)
Mirrors the UI `chainExecutor` but builds the resolver with the full scope
chain (global < .env < collection < env < request vars < script overlay) so a
CLI run resolves variables identically to a GUI Send. There is no prompt scope
(`{{?Name}}`, #86) — a headless run can't ask, so prompt vars stay unresolved.
`ExecuteOnce` (the `chain.Executor` interface) wraps it, discarding the leaf's
test results for chain steps; the leaf path keeps them via `executeOnce`.

### Run order — [exec.go:130](exec.go#L130)
`collectRequests` walks the tree via `session.Tree().ChildIDs`, which yields
folders before requests, so the run order matches the sidebar's display order.
A request referenced as a chain step runs both on its own turn and as its
dependents' predecessor (v1 behavior).
