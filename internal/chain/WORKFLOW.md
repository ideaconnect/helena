# internal/chain — workflows

## Lifecycle of a chained Send

`MainUI.send` builds the per-Send `chainExecutor` and snapshots the active
collection as a `*session.ChainFinderSnapshot` (via `Session.SnapshotChainFinder`,
which satisfies `chain.RequestFinder`; a `nilFinder` is used when no collection
is loaded), launches the worker goroutine, and the first thing the goroutine
does is call `chain.Resolve(ctx, leaf, finder, exec, progress)`. The story from
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
   full trace. **Just before this call**, if a `ProgressFunc` was
   supplied to `Resolve`, it is invoked as
   `progress(*stepCount, total, step.Alias, sub.Name)` so the UI
   can update its status line to "step N/total" mid-chain. **After
   the call**, when the HTTP actually went out (`view.Request.URL`
   non-empty), the runner appends one auto-trace line —
   `→ chain[<alias>] <METHOD> <URL>` — to the shared console so the
   user can see the post-pre-script wire URL of each chain step in
   the Console panel after the Send completes.
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

## Overlay rollback on failure

The Send goroutine snapshots the overlay BEFORE invoking
`chain.Resolve` and calls `Session.RestoreEnvOverlay(preOverlay)` on
any chain error (including panics caught by the top-level recover).
This means a chain that runs steps 1, 2, 3 successfully — each
writing TOKEN, CSRF, USER — and then errors at step 4, leaves the
overlay in its pre-Send state. The user sees the error and presses
Send on something legitimate; that next request's `{{TOKEN}}`
template doesn't pick up the half-applied attacker-controlled value.

The rollback is intentionally **at the chain boundary**, not per
step: succeeding chain steps that the leaf eventually needs (e.g.
TOKEN written by Login so Profile can read `{{TOKEN}}`) survive,
because the leaf executes only AFTER `chain.Resolve` returns with no
error. The overlay snapshot is only restored on the error path.

## Diamond pattern: shared predecessor runs once per branch

For `A → [B, C]` where both `B.Chain` and `C.Chain` reference `D`,
the runner executes `D` twice — once while resolving `B`'s chain
(landing in `B`'s alias map under whatever name `B` gave it) and
once while resolving `C`'s chain. The `visiting` set deletes the
entry on the way out of each subtree, so `D` is not flagged as a
cycle when re-entered through a different branch.

There is **no cross-branch dedupe** by design: each request's chain
map is per-request scope, and a single shared `View` would have to
land under different aliases in `B`'s and `C`'s maps. If you want
"run D once and let both branches see it", hoist `D` up to `A`'s
own chain (where it has a single alias) and reference
`chain.<aliasInA>` from inside `B`/`C`'s scripts.

Users who put a heavy `Bootstrap` step into multiple branches will
see that endpoint hit `N` times per Send — the depth + step caps
keep the worst case bounded but the per-branch repetition is real.
Document this when adding chain to a shared "before all" request.

## Caps the runner enforces

| Limit | Constant | What happens on exceed |
| ----- | -------- | --------------------- |
| Nesting depth | `MaxChainDepth` (8) | `chain: depth exceeds limit 8 …` |
| Total step count per Resolve | `MaxChainSteps` (32) | `chain: step count exceeds limit 32 …` |
| Cumulative console lines | `MaxChainConsoleLines` (1024) | Drops further lines; appends `[chain console truncated]` once. |

Each cap is a per-Resolve counter — distinct Sends start fresh. The
caps are conservative for legitimate use (no realistic user-authored
chain exceeds depth 8 or 32 total steps) and tight against
imported-collection abuse.

## Auth on chain steps

`Session.SnapshotChainFinder` is called once on the UI thread before
the worker goroutine launches. It clones the active collection's
folders + requests, and as it does so it **pre-flattens each
request's `Auth` via `auth.Resolve(own, ancestors)`** — the same
walk Send applies to the leaf via `EffectiveAuth`. So a chain step
that was stored with `Auth.Type == AuthInherit` (the common case
when the user didn't set per-request auth) carries the parent
folder's or collection's Auth into the chained Send.

This guarantee was added after the 7.4 review found that bare
`FindRequestByPath` returned a Request whose `Auth` was the raw
loaded value, so chained requests fired without authentication. If
you change the chain-step execution path, preserve the snapshot
finder's auth-flattening — without it, the most common chain shape
(Login inherits OAuth2 from Auth folder, Profile chains Login)
silently breaks.

## Per-step progress callback

`Resolve` accepts a `ProgressFunc` (may be `nil`) that fires once
before each chain step's `ExecuteOnce`. Two timing details:

- **`total` is pre-walked.** When `progress != nil`, `Resolve` does a
  single cheap walk of the same tree the executor will follow,
  applying the same visiting-set cycle skip and depth/step caps so
  the reported total matches what will actually run. The walk does
  not call `ExecuteOnce` and tolerates unresolvable paths by
  skipping them (the real Resolve will surface those as errors).
- **Callback runs on the worker goroutine.** The UI's wiring in
  `internal/ui/shell.go` wraps the callback in `fyne.Do` so the
  status-line update lands on the UI thread (invariant #4).

The UI Send pipeline uses this to show `Chain step 2/3: Login`
mid-chain and falls back to `Sending: <leafName>` once the chain
finishes and the leaf is in flight. Sends with no chain pass
through unchanged (the callback never fires).

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
