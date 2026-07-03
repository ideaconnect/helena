# runner — Workflow

`runner.Run` has two entry points: the `helena run` CLI (below) and the in-app
"Run collection" button (#89), which calls `runner.Run` off the UI goroutine and
renders the `Report` as a per-request + aggregate dialog (`actionRunCollection`
/ `showRunReport` in [internal/ui/runcollection.go](../ui/runcollection.go)).

## `helena run <collection-dir> [--env NAME]`
1. `cmd/helena` dispatches the `run` subcommand before `flag.Parse`, so the GUI never starts.
2. It builds an ephemeral `session.New("")` (no config persistence), `OpenCollection(dir)`, `SetActiveCollection(0)`, and `SetActiveEnv(name)` when `--env` is given.
3. `runner.Run(ctx, sess)` executes the collection; `printReport` writes the result; the process exits `1` if `Report.Failed()`, `2` on a usage/setup error, else `0`.

## Running the collection
`Run` calls `collectRequests` to flatten the active collection's tree into a
depth-first list of `(nodeID, path, request)`, then `runOne` for each:

1. **Auth flatten.** `leaf.Auth = sess.EffectiveAuth(nodeID)` resolves Inherit via the folder→collection walk (same as a UI Send).
2. **Wiring.** The run-wide `httpclient.Client` (built once per `Run`, with the session cookie jar and a client-credentials OAuth2 resolver — authorization_code is interactive and unavailable headless; its idle sockets are released when the run ends), a `scripting.Runtime` over an `envBridge`, and the variable-scope snapshots (`global` / `.env` / `collection` / `env`).
3. **Overlay isolation.** The env overlay is snapshotted and restored (deferred) so a request's `helena.env.set` writes don't leak into the next.
4. **Chain.** `chain.Resolve` runs the leaf's before-hooks via `headlessExecutor.ExecuteOnce`; a chain error fails the request without sending the leaf.
5. **Execute.** `executeOnce` runs pre-script → `client.Do` → post-script, building the response view; the leaf's `test()`/`expect()` results come back too. A pre-script `helena.runner.skip()` short-circuits the send and marks the result `Skipped` (#92); a `helena.runner.stop()` (any script) sets the run-level signal so the loop stops after this request.
6. **Assertions.** `assertion.Evaluate(leaf.Assertions, status, headers, body)` adds the declarative checks (#88).
7. Test results and assertion results are merged into `RequestResult.Checks`; an execution failure sets `RequestResult.Err`.

## Report → exit code
`Report.Failed()` is true if any `RequestResult` is not `OK()` — i.e. any
request errored or any check failed — which becomes the CLI's exit code 1. This
is what lets a CI job gate on `helena run`.
