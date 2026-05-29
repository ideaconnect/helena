# internal/chain — workflows

## Lifecycle of a chained Send

`MainUI.send` builds the per-Send `chainExecutor` + `sessionRequestFinder`,
launches the worker goroutine, and the first thing the goroutine does
is call `chain.Resolve(ctx, leaf, finder, exec)`. The story from
there:

1. **Initialise the visiting set.** `Resolve` puts the leaf's
   `Request.ID` into a `map[string]bool` so any chain step that
   transitively references the leaf produces a clean cycle error
   instead of recursing forever.
2. **Recurse into the leaf's `Chain`.** `resolveSteps` walks each
   `model.ChainStep` in order. For every step it:
   - Validates that `Alias` is non-empty and unique within this
     request's chain.
   - Validates that `Request` is non-empty.
   - Looks up the predecessor via `finder.FindRequestByPath(step.Request)`.
     Failure → `chain: cannot resolve request "X" (alias "a")`.
   - Checks the visiting set — if the resolved predecessor's ID is
     already in there, a cycle exists; bail with
     `chain: cycle detected through "<Name>" (alias "<alias>")`.
   - Marks the predecessor as visiting, recursively resolves ITS
     `Chain` (producing the sub-chain map private to that
     predecessor's own scripts), then unmarks before returning.
3. **Execute the predecessor.** `exec.ExecuteOnce(ctx, sub,
   subChainMap)` runs the predecessor's pre-script (with its own
   `chain.<alias>` global), `client.Do`, and post-script. The
   returned `View` lands in this level's accumulating chain map
   under `step.Alias`. Console lines from the predecessor are
   appended to the shared `*console` slice so the UI can show the
   full trace.
4. **Stop on any error.** Pre-script, HTTP, or post-script failure
   anywhere in the recursion aborts `Resolve`. The error is wrapped
   with the failing step's alias and name so the user can fix the
   right step.
5. **Return.** When every step of the leaf's chain has executed
   successfully, `Resolve` returns `(chainMap, console, nil)` —
   chainMap is the leaf's own `{alias → View}` and console holds
   every line every chain step printed.

The leaf itself is then executed by the caller via the same
`exec.ExecuteOnce` with the returned chainMap as its `chain` global.
This keeps the leaf and the chain steps on identical code paths;
there is no "leaf-only" or "chain-step-only" execution branch.

## Alias scope semantics

The map returned by `resolveSteps(req)` is private to `req`'s own
scripts. When `A → [B] → [C]` runs:

- `C`'s `chain` global is empty (C has no Chain).
- `B`'s `chain` global has only `{csrf: View(C)}` (B's declared alias
  for C).
- `A`'s `chain` global has only `{login: View(B)}` (A's declared
  alias for B). A never sees `csrf` — it belongs to B's scope.

This matches user intent: each request reasons about its own chain
declarations, not its predecessors'.

## Cycle detection by ID

The visiting set is keyed by `model.Request.ID`. IDs are assigned
fresh by `storage.Load` on every collection load, so they are stable
within a session but not across reloads — which is exactly the
window the cycle check operates in.

Direct cycle (A's chain references A):
```
visiting = {A: true}
walk A.Chain → encounter A → cycle.
```

Indirect cycle (A → B → A):
```
visiting = {A: true}
walk A.Chain → encounter B → push B → visiting = {A, B}
walk B.Chain → encounter A → A is in visiting → cycle.
```

`Request.ID` being empty is tolerated for tests — the cycle check
just skips visiting-set bookkeeping for that step. Production
requests always carry a non-empty ID assigned by
[internal/storage](../storage/) on load.

## How a chain step's `helena.env.set` reaches the leaf

The session env overlay is process-lifetime; every `helena.env.set`
call hits the live overlay map under `overlayMu` immediately. The
leaf's resolver is built inside `chainExecutor.ExecuteOnce` from the
captured env snapshot plus a *fresh* `SnapshotEnvOverlay()` —
so chain steps that ran before the leaf have already had their
overlay writes flushed, and the leaf's `httpclient.Build` sees them
during `{{var}}` expansion. The leaf's own pre-script runs against
the same overlay too, so a chain step's `helena.env.set("TOKEN", …)`
is visible to the leaf's pre-script under the same name.

## Failure shapes the user sees

The UI surfaces chain failures via `m.Status.SetText("Chain error: …")`
and dumps the error message into the Raw response panel. Sample
messages users may encounter:

- `chain: step is missing an alias (request "Auth/Login")`
- `chain: alias "login" has no request reference`
- `chain: duplicate alias "login" in the same request's chain`
- `chain: cannot resolve request "Auth/Login" (alias "login")`
- `chain: cycle detected through "Login" (alias "login")`
- `chain step "login" (Login): pre-script: <inner script error>`
- `chain step "login" (Login): <http error>`

All consistently prefix with `chain:` (or `chain step`) so the user
can tell at a glance the failure happened during chain resolution,
not during the leaf's own Send.
