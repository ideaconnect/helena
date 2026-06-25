# session — workflows

## Opening a collection

The user picks a directory from the file dialog. The UI calls
`Session.OpenCollection(dir)`. The session:

1. Calls `storage.Load(dir)` to parse the OpenCollection YAML tree into a
   `model.Collection`.
2. Appends `dir` to the active workspace's `Collections` slice (so it is
   remembered on next start).
3. Appends the loaded collection to `cols` and `dirs` (kept index-aligned).
4. Sets `activeCol` to the new index and records the directory in
   `cfg.UI.ActiveCollection`.
5. Calls `persist()`, which writes the entire config back to `cfgPath`.

On next launch, `New` will replay this through `reload`: every directory in
the active workspace is loaded; a failed load is dropped from `cols`/`dirs`
but its `{Dir, Err}` is recorded in `loadErrs` and exposed via `LoadErrors()`,
which the UI surfaces as a non-transient diagnostic (#108) instead of letting
the collection silently disappear.

## Switching the active environment

The active environment determines which variables `Resolver()` exposes for
`{{name}}` template expansion.

1. The UI calls `Session.SetActiveEnv(name)` after the user picks one from the
   environment dropdown (or the empty string for "no environment").
2. The session updates `activeEnv[activeCol]` (index-keyed in-memory map) and
   the persisted `cfg.UI.ActiveEnv[dir]` (path-keyed) so reordering
   collections later doesn't lose the choice. Empty name removes the entry.
3. `persist()` writes the config.
4. The next `Resolver()` call layers ordered scopes — the global `Variables`
   (#83), then the collection-root `.env` (#84), then the active collection's
   `Variables` (#80), then `ActiveEnvironment().Variables`, then the env overlay
   (highest precedence) — each filtered to `Enabled` entries, and attaches the
   `vars.Dynamic` fallback for `{{$guid}}`/`{{$timestamp}}`/… (#85). So `.env`
   overrides a global of the same name, a collection value overrides `.env`, an
   environment value overrides the collection, and the overlay overrides all
   (see "Env overlay" below).
   `ResolverForRequest(r)` inserts the request's own `Variables` (#82) between
   the environment and the overlay, making them the highest **static** scope:
   global < .env < collection < environment < request < overlay. The Send worker
   layers the same scopes directly in `execution.go`; the URL preview and
   exporter call `ResolverForRequest(currentRequest)`.

The `.env` file is read from the active collection's directory and cached per
collection (the cache is dropped on `reload()`, so reopening a collection
re-reads it); it is never written by Helena.

`ActiveEnvironment()` resolves by name (not index) into the active
collection's `Environments`, so editing an environment's name through the
UI keeps the dropdown consistent without a separate rebuild step.

## Env overlay (script-set variables)

`overlay` is a `map[string]string` of variables a per-request script set
via `helena.env.set(name, value)` (see
[internal/scripting](../scripting/)). It is the highest-precedence
scope on the resolver — overrides the active environment — but it
**never** reaches `storage.Save`.

Public API:

| Method | Use |
| ------ | --- |
| `SetEnvOverlay(name, value)` | Called by `sessionEnvBridge.Set` whenever a script runs `helena.env.set`. Empty name is silently ignored. |
| `EnvOverlay(name) (string, bool)` | Direct read of the overlay only — useful in tests to assert a script wrote a value. The Resolver path is what production code should use. |
| `ClearEnvOverlay()` | Drops every entry. The UI doesn't call this today; a future "Reset scripted env" button would. |

`Resolver()` adds `snapshotOverlay()` as the last (highest precedence)
scope passed to `vars.New`. The snapshot is a copy so the goroutine
that evaluates `{{var}}` substitution doesn't race against a script
goroutine still writing entries. The pre-request hook can therefore
call `helena.env.set` and the same Send's URL/body resolution will pick
up the new value — the resolver is rebuilt after the pre-script runs
in `MainUI.send`.

Concurrency: writes go through `overlayMu.Lock()`, reads through
`overlayMu.RLock()`. `TestEnvOverlayConcurrentSafe` hammers both and
runs under `go test -race`.

## Restoring UI state on startup

`New(cfgPath)` calls `config.Load`, which yields a populated `config.Config`
(or `config.Default()` if the file is missing). `reload` then:

1. Walks the active workspace's `Collections` and tries `storage.Load` on
   each. Successful loads land in `cols` and `dirs`. A failure (renamed
   directory, deleted folder, broken YAML) is dropped from `cols`/`dirs` so
   the app still starts, but its `{Dir, Err}` is appended to `loadErrs`
   (reset at the top of every `reload`) and exposed via `LoadErrors()` for the
   UI to surface (#108).
2. Picks `activeCol`: if `cfg.UI.ActiveCollection` matches a `dirs` entry,
   that becomes active; otherwise `0` if any collections loaded, `-1`
   otherwise.
3. Rebuilds `activeEnv` (`map[int]string`) by translating the persisted
   path-keyed `cfg.UI.ActiveEnv` into the current index space.

The open request and window size live in the config and are restored
lazily — the shell asks `Session.OpenRequest()` / `Session.WindowSize()`
when it builds the UI:

- `OpenRequest()` reads `cfg.UI.OpenRequest`, finds the matching collection
  directory in `dirs`, and rebuilds the full node ID as
  `<index> + "/" + <NodePath>`. If the directory is no longer loaded it
  returns `""`.
- `WindowSize()` returns the persisted `(w, h)` pair, or `(0, 0)` when unset.

## Tree node ID format

See [STRUCTURE.md](STRUCTURE.md#node-id-format) for the full table. In short:

```
0/f1/r0
| |  |
| |  +- request 0
| +---- folder 1
+------ collection 0
```

The first segment is the collection index in `cols`. Subsequent segments are
`f<i>` for folder children. The final segment may be either `f<i>` (an
addressable folder) or `r<i>` (a request leaf). The empty string `""` is the
synthetic root used by `widget.Tree`.

The persisted form in `config.UIOpenRequest.NodePath` drops the collection
segment — the collection is recorded separately by directory path so the
restoration is stable across collection reordering.

## Save-back after edits

Two distinct edit surfaces feed into the same save path:

- Tree mutation helpers ([items.go](items.go)) — `AddRequest`, `AddFolder`,
  `RenameItem`, `DeleteItem`, `DuplicateItem`, `MoveNode`. Each one walks the
  tree via `containerAtPtr` (which returns pointers into the in-memory model),
  applies the mutation, and persists. **Rollback guard (#109):** all save sites
  funnel through `persistCollection(ci)`, which on a `storage.Save` failure
  calls `reload()` so the in-memory model snaps back to disk. Because
  `storage.Save` is atomic (a failed save leaves disk unchanged), reload
  restores the exact pre-mutation state — memory never stays ahead of disk. The
  caller still surfaces the error; any node IDs it computed are invalid on
  failure.
- Direct edits — the request editor in the UI gets a pointer from
  `Tree.Request(id)` and mutates fields in place (URL, headers, body, …).
  When the user triggers save (e.g. closes the tab, hits the save shortcut),
  the shell calls `Session.SaveActiveCollection()`.

`SaveActiveCollection()` and `saveCollection(ci)` both delegate to
`persistCollection(ci)` (the rollback guard above), which wraps
`storage.Save(cols[ci], dirs[ci])`. The interesting work
(preserving unknown YAML fields, sweeping orphans) is in
[`internal/storage`](../storage/) — see its
[WORKFLOW.md](../storage/WORKFLOW.md).

When an entire folder is duplicated, `DuplicateItem` does a deep copy through
`deepCopyFolder` / `deepCopyRequest` so the copy's slices (Headers, Params,
Form, nested Folders and Requests) are independent of the original. New IDs
are assigned at every level so the duplicate is treated as a fresh item.

`MoveNode` (the drag-and-drop relocation, same collection only) is the one
mutation where positional node IDs can't be trusted across the change: removing
the source shifts the indices of later siblings, which can invalidate a
positional pointer to the destination. So it captures the destination by a
stable folder `model.ID` (minting one only if a legacy item lacks it — an
existing request ID is never rewritten, since open tabs track requests by it),
removes the source, re-resolves the destination by that ID, inserts, then
cascades chain references whose name-path prefix changed (mirroring
`RenameItem`). `MoveCollection` reorders `cols`/`dirs` and the workspace's dir
list in lockstep and keeps the active-collection pointer on its collection.

## Resolving effective auth

`Session.EffectiveAuth(nodeID)` is the bridge between the on-disk
auth-inheritance tree and the concrete `model.Auth` that
[`internal/auth`](../auth/) `Apply` writes onto the outgoing request.

1. `Tree()` builds a fresh `Tree` over the loaded collections.
2. `tree.Request(nodeID)` returns a pointer to the addressed request, or
   `(nil, false)` for a malformed / unknown ID. The request's own `Auth`
   is used as the starting point (zero value when the request can't be
   resolved at all — `auth.Resolve` then falls back to `AuthNone`).
3. `tree.AncestorAuths(nodeID)` walks the parent chain. For
   `0/f1/r0`, that walk yields:
   - parent `0/f1` → the folder's `Auth`
   - parent `0` → the collection's `Auth`
   - parent `` (root) → stops; the empty root has no auth.
4. `auth.Resolve(reqAuth, ancestors)` short-circuits if `reqAuth` is
   non-Inherit; otherwise scans the chain and returns the first
   non-Inherit value. When everything in the chain inherits (or the
   collection root is `None`, which is the load-time default for roots
   that never configured auth), the result is `AuthNone`.

UI Send calls `EffectiveAuth(m.currentRequestID)` on a copy of the
request right before handing it to `httpclient.Do`, so the engine sees
the flattened auth and never has to know the tree.

## Plain-text environment encoding

`ParseEnvVars` / `FormatEnvVars` ([env.go](env.go)) convert between
`[]model.Variable` and a one-variable-per-line text form:

```
key = value          # enabled
# disabled = value   # disabled (#-prefix)
```

Blank lines and lines without `=` are skipped; the Secret flag has no text form.
These backed the old multi-line text-area environment editor; the UI now uses a
structured key/value list instead (`editEnvironments` in
[internal/ui/envedit.go](../ui/envedit.go), #123), so they are no longer wired
into the UI — they remain the canonical text encoding and keep their own tests.
