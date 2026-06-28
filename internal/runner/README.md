# runner

`runner` is Helena's headless collection runner — the engine behind the
`helena run <collection-dir>` CLI subcommand (#90). It loads no GUI: given an
open `session.Session`, it executes every request in the active collection and
returns a structured `Report` of statuses and pass/fail checks, suitable for CI.

Each request is sent independently with its **own** chain, scripts, variable
resolution, auth, and assertions — exactly the pipeline a GUI Send runs — so a
CLI run reproduces what the app would send. The env overlay is rolled back
after each request so script-set values don't leak between them.

> **Duplication note.** The per-request execution here intentionally duplicates
> the UI's `chainExecutor` (and the test-only one in `internal/integration`).
> Extracting one shared execution engine used by the UI, integration, and this
> runner is tracked as follow-up cleanup; this package is purely additive and
> does not touch the UI send path.

## Public API

- `Run(ctx context.Context, sess *session.Session) Report` — execute every request in `sess`'s active collection (depth-first, tree order) and return the `Report`.
- `Report` — `{Results []RequestResult}` with `Failed() bool` (any request error or failed check) and `Totals() (requests, checksPassed, checksFailed int)`.
- `RequestResult` — `{Path, Method, URL string; StatusCode int; Duration time.Duration; Err string; Checks []Check; Skipped bool}` with `OK() bool` (no error and all checks passed). `Skipped` is set when a pre-request script calls `helena.runner.skip()` (#92); such a request is not sent and does not fail the run.
- `Check` — `{Name string; Passed bool; Error string}`: one assertion (#88) or `test()`/`expect()` (#87) outcome.

## Dependencies

- [`internal/session`](../session), [`internal/httpclient`](../httpclient), [`internal/chain`](../chain), [`internal/scripting`](../scripting), [`internal/assertion`](../assertion), [`internal/auth`](../auth), [`internal/vars`](../vars), [`internal/model`](../model).
- Standard library: `context`, `time`, `strings`.
