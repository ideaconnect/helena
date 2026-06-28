# runner — Structure

## Files

| File | Responsibility |
| --- | --- |
| [runner.go](runner.go) | The public `Report` / `RequestResult` / `Check` types and their helpers (`OK`, `Failed`, `Totals`), the `Run` entry point (which threads a `stopSignal` so `helena.runner.stop()` halts the loop, #92), and `runOne` (per-request orchestration: auth flatten → snapshots → chain → execute → assertions; honours `helena.runner.skip()` by marking the result `Skipped` and not sending). |
| [exec.go](exec.go) | The `headlessExecutor` (pre-script → http → post-script, with the full resolver scope chain), the `envBridge`, the request-tree walk (`collectRequests` / `walk`), and the `enabledVars` / `chainViewToScripting` / `nilFinder` helpers. |
| [runner_test.go](runner_test.go) | End-to-end runs against an `httptest` server: mixed pass/fail assertions + a `test()` script, all-pass, a connection error, a chain step with env-overlay scripts and request variables, a throwing post-script, and `{{var}}` resolution from the active environment. |

## Type catalog

| Type | Role |
| --- | --- |
| `Report` | The result of a full run: `Results []RequestResult`. `Failed` drives the CLI exit code; `Totals` feeds the summary line. |
| `RequestResult` | One request's outcome — path, method, resolved URL, status, duration, an `Err` for a pre/http/post/chain failure, the `Checks`, and `Skipped` (the pre-script called `helena.runner.skip()`, #92 — no send). `OK` = no error and every check passed (a skipped request is OK). |
| `Check` | One assertion or scripted-test outcome (`Name`, `Passed`, `Error`), unifying #88 declarative assertions with #87 `test()` results. |

## Non-trivial internals

### `headlessExecutor.executeOnce` — [exec.go:89](exec.go#L89)
Mirrors the UI `chainExecutor` but builds the resolver with the full scope
chain (global < .env < collection < env < request vars < script overlay) so a
CLI run resolves variables identically to a GUI Send. There is no prompt scope
(`{{?Name}}`, #86) — a headless run can't ask, so prompt vars stay unresolved.
`ExecuteOnce` (the `chain.Executor` interface) wraps it, discarding the leaf's
test results for chain steps; the leaf path keeps them via `executeOnce`. `executeOnce` takes an `honorSkip` flag (true only for the leaf) and wires `helena.runner` via a per-request `runControl` (`Stop` sets the shared `stopSignal`; `Skip` flags this request); chain predecessors share the control so their `stop()` works but never honour `skip()`.

### Run order — [exec.go:130](exec.go#L130)
`collectRequests` walks the tree via `session.Tree().ChildIDs`, which yields
folders before requests, so the run order matches the sidebar's display order.
A request referenced as a chain step runs both on its own turn and as its
dependents' predecessor (v1 behavior).
