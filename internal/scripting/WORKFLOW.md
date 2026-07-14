# internal/scripting — workflows

## Lifecycle of a Send with scripts

This is the end-to-end story for a request whose `Scripts` field has a
non-empty `PreRequest` or `PostResponse`. See
[internal/ui/WORKFLOW.md](../ui/WORKFLOW.md) for the surrounding UI
plumbing.

1. `MainUI.send` constructs a `scripting.Runtime` via
   `scripting.New(sessionEnvBridge{s: m.sess, base: envSnap})`, where `envSnap`
   is a snapshot of the active environment's variables taken on the UI thread
   at send entry. The bridge is a thin adapter: `Get` returns a script-set
   overlay value (`Session.EnvOverlay`) if present, else the frozen `base`
   snapshot — so the worker goroutine never races UI-thread env edits; `Set`
   calls `Session.SetEnvOverlay`.
2. `send` spawns the off-UI goroutine. Inside:
   - If `req.Scripts.IsEmpty()` is false and `PreRequest` is non-empty,
     it calls `rt.RunPreRequest(ctx, req.Scripts.PreRequest, &req)`.
     - `runWithTimeout` starts: a watcher goroutine waits on
       `time.After(ScriptTimeout)`, `ctx.Done()`, and the local `done`
       channel. Whichever fires first calls `vm.Interrupt(reason)`.
     - `requestToObject` mirrors the request into a fresh `goja.Object`.
     - `bindHelena` and `bindConsole` attach the helena.* and console.*
       surfaces.
     - `vm.RunProgram(compileCached(script))` evaluates the user source —
       the compiled program is cached process-wide per distinct source, so a
       re-sent request skips recompiling its scripts.
     - On normal return, `writeBackRequest` reads the JS object back
       into the model: scalars become direct writes; headers and params
       merge through `mergeKVFromObject` (see the merge rules below).
     - The watcher's `done` channel is closed so it exits cleanly.
     - On interrupt, the captured reason is returned as a plain Go
       error ("script execution timed out", "script execution
       cancelled: …", or "script interrupted: …" for unattributed
       interrupts).
   - If pre-script failed, `send` short-circuits: it shows the error in
     the Raw response tab, dumps any pre-emitted console lines into the
     Console panel, re-enables the Send button, and returns. **The
     request is never sent.**
3. Otherwise `client.Do(ctx, req, resolver)` runs as usual. **The
   resolver is built fresh after the pre-script — from the env
   snapshot captured at goroutine entry plus a fresh overlay
   snapshot** — so any `helena.env.set(...)` calls from the pre-script
   are visible to `httpclient.Build` when it expands `{{vars}}`. The
   resolver deliberately doesn't go through `Session.Resolver()` from
   the worker; using the captured snapshots avoids racing against
   UI-thread env edits that may happen during a long-running script.
4. After Do returns, if there's no Do-error and `PostResponse` is
   non-empty, `send` calls
   `rt.RunPostResponse(ctx, req.Scripts.PostResponse, req,
   scripting.ResponseInput{...})`.
   - Same setup as pre, plus `responseToObject` binds the read-only
     `response` global. `tryParseJSON` and `tryParseXML` attempt to
     parse the body regardless of `Content-Type`; on failure the matching
     property stays `undefined`.
   - The post-script's mutations on the request are ignored — the
     request has already gone over the wire. The post-script's main
     job is calling `helena.env.set` for values it wants to forward.
5. `send` posts the final UI update via `fyne.Do`:
   - Pre + post `Console` lines are joined with `\n` into the Console
     panel.
   - If `postErr` is non-nil, it's appended to the status line so the
     user sees "200 OK · 1.2 KB · 47 ms · post-script: …".
   - The successful response body is shown the same way it would be
     without scripts.

## Header / param write-back semantics

`mergeKVFromObject` is the single point where script mutations on
`request.headers` and `request.params` reconcile with the model. The
rules, in order:

1. Disabled rows in the existing model pass through unchanged. Scripts
   never see them and can't enable them by accident.
2. Enabled rows whose key is still a property on the JS object have
   their `Value` updated to the JS value (`obj.Get(matchedKey).String()`).
   Matching is **case-insensitive** so HTTP header semantics hold —
   `request.headers["content-type"] = "x"` updates an existing
   `Content-Type` row in place rather than producing a duplicate.
3. Enabled rows whose key is **absent** from the JS object are dropped.
   The user called `delete request.headers["X-Old"]` and we honor it.
4. JS-object keys not consumed by any existing row are appended as new
   `KeyValue` rows with `Enabled: true`.

The same function handles both `Headers` and `Params`; the merging logic
doesn't care which it is.

## Console output

`bindConsole` installs four functions on the JS `console` global. Each
takes any number of args, runs them through `stringify`, and appends
one space-joined entry to `Result.Console`:

| Function | Prefix |
| -------- | ------ |
| `console.log` | (none) |
| `console.info` | (none) |
| `console.warn` | `WARN: ` |
| `console.error` | `ERROR: ` |

`stringify` short-circuits for `null` / `undefined` / strings, and
falls back to `json.Marshal(value.Export())` for everything else so
`console.log({a: 1})` shows `{"a":1}` instead of the JS engine's
`[object Object]`.

Capture is bounded: once `Result.Console` reaches `maxConsoleLines`
(1000) or `maxConsoleBytes` (256 KiB), `bindConsole` appends a single
`"… console output truncated"` marker and drops the rest. Without this a
`while (true) console.log(x)` loop — which runs for the entire
`ScriptTimeout` — would grow the slice to hundreds of MB and freeze the
UI rendering millions of lines. The test recorder is capped the same way
(`maxTestResults`, dropped silently so pass/fail tallies aren't skewed).

## Interrupt handling

goja exposes asynchronous interruption via `vm.Interrupt(reason)`,
which causes the next bytecode instruction to throw a
`*goja.InterruptedError` with that reason as its `Value()`. We use it
two ways:

1. A 5-second timer per call (`ScriptTimeout`).
2. A `ctx.Done()` watcher so the UI can cancel scripts on tab close (or
   tests can drive timeouts).

The watcher goroutine signs off via a local `done` channel that the
main caller closes after the run returns. There's no leak: even if
both the timer and the script finish at the exact same instant, the
goroutine selects on the `done` close and exits.

The interrupt reason is captured behind a mutex so the caller can
return it as the error message even when goja returns the bare
`InterruptedError`. Without this the user would see
`goja.InterruptedError(Object)` instead of "script execution timed out".

## What scripts CANNOT do

- Write to disk. There is no `helena.fs` surface.
- Read or write the underlying environment file. `helena.env.set` only
  touches the in-memory overlay (invariant 9 in [AGENTS.md](../../AGENTS.md)).
- Send additional HTTP requests. There is no `helena.http` or `fetch`
  binding. (Chained requests ship as a separate feature — their results are
  exposed read-only via the `chain.<alias>` global, not by scripts issuing
  their own HTTP.)
- Persist arbitrary state across Helena restarts. The overlay dies with
  the process.
- Modify the request in the post-response phase — those mutations are
  read but never applied.

## Adding a binding

If you're extending the surface (say adding `helena.crypto.sha256`):

1. Write the binding helper in [bindings.go](bindings.go).
2. Wire it into `bindHelena` so both phases get it.
3. Add a test in [scripting_test.go](scripting_test.go) that
   demonstrates the new binding from a real script.
4. Update [README.md](README.md)'s "Script surface" table and
   [STRUCTURE.md](STRUCTURE.md)'s helpers table.
5. If the binding crosses an invariant boundary (e.g. lets scripts
   write to disk), update [AGENTS.md](../../AGENTS.md) in the same
   change.
