# session — structure

## Files

| File | Responsibility |
| --- | --- |
| [session.go](session.go) | `Session` type, constructor `New`, workspace switching, active collection / environment, UI state persistence (open request, window size, settings), `Resolver()`, env overlay (`SetEnvOverlay` / `EnvOverlay` / `ClearEnvOverlay`). |
| [tree.go](tree.go) | `Tree` navigation model used by the Fyne `widget.Tree`. Defines the node ID format and the lookups that drive `widget.Tree` callbacks. |
| [items.go](items.go) | Tree mutation: `AddRequest`, `AddFolder`, `RenameItem`, `DeleteItem`, `DuplicateItem`. Each one mutates the in-memory collection through pointer access and calls `SaveActiveCollection`. |
| [workspace.go](workspace.go) | Workspace CRUD: `AddWorkspace`, `RenameWorkspace`, `DeleteWorkspace`. |
| [env.go](env.go) | Plain-text environment-variable parsing and formatting: `ParseEnvVars` / `FormatEnvVars`. |
| [session_test.go](session_test.go) | Open-collection persistence + tree navigation. |
| [session_auth_test.go](session_auth_test.go) | `EffectiveAuth` resolution covering own-wins, folder→collection inheritance, fallback to `AuthNone`, and unknown-id safety. |
| [session_env_test.go](session_env_test.go) | Resolver, env editing persistence, parse/format env vars. |
| [session_overlay_test.go](session_overlay_test.go) | Script-set env overlay: basic round-trip, clear, Resolver layering (overlay > active env), non-persistence invariant, and concurrent-safety with `-race`. |
| [session_save_test.go](session_save_test.go) | Round-trip of request edits through Tree pointer + save. |
| [session_settings_test.go](session_settings_test.go) | Settings persistence. |
| [session_uistate_test.go](session_uistate_test.go) | UI state persistence (active collection, active env, open request, window size) and stability across collection reordering. |
| [workspace_test.go](workspace_test.go) | Workspace add/rename/delete, including the "cannot delete last" rule. |
| [items_test.go](items_test.go) | Add/Rename/Delete/Duplicate tree items. |

## Session struct

Defined in [session.go](session.go):

| Field | Type | Meaning |
| --- | --- | --- |
| `cfgPath` | `string` | Path to the YAML config file. Empty path disables persistence. |
| `cfg` | `config.Config` | In-memory copy of persisted state (workspaces, active index, settings, UI). |
| `cols` | `[]model.Collection` | Collections loaded for the active workspace, in workspace order. |
| `dirs` | `[]string` | Source directory of each loaded collection, aligned with `cols`. Used as a stable identity for the UI state map (so reordering collections doesn't break "open request" restoration). |
| `activeCol` | `int` | Index into `cols`, or `-1` when none. |
| `tokens` | `*auth.TokenCache` | Process-lifetime OAuth2 token cache. Keyed by `CacheKey(collectionDir, OAuth2Auth)` so two collections that share a token URL never share tokens. Constructed in `New`. |
| `activeEnv` | `map[int]string` | Active environment name per collection (keyed by index). Populated from `cfg.UI.ActiveEnv` (path-keyed) on reload. |
| `overlayMu` | `sync.RWMutex` | Guards `overlay` against concurrent script-thread writes and UI-thread reads. |
| `overlay` | `map[string]string` | Script-set env overlay. Highest-precedence resolver scope. **In-memory only** — never persisted (AGENTS invariant 9). |

The pair `(cols, dirs)` is the bridge between the index-keyed in-memory model
and the path-keyed persisted UI state. The map is rebuilt on every `reload`
to keep them in sync.

## Tree navigation model

Defined in [tree.go](tree.go).

```
type Tree struct {
    cols []model.Collection
}
```

`Tree` is a thin read-only view over the loaded collections, built on demand
by `Session.Tree()`. It exposes exactly the four callbacks Fyne's `widget.Tree`
needs (`ChildIDs`, `IsBranch`, `Label`, plus the leaf data accessor `Request`)
and one convenience for the UI (`CollectionIndex`).

### Node ID format

Node IDs are slash-separated. The first segment is always a collection index;
subsequent segments are either folder references (`f<i>`) or — only as the
last segment — a request reference (`r<i>`).

```
""           -> root (the implicit container holding all collections)
"0"          -> collection 0
"0/f1"       -> folder 1 of collection 0
"0/f1/f0"    -> folder 0 nested inside folder 1 of collection 0
"0/f1/r0"    -> request 0 of folder 1 of collection 0
"1/r3"       -> request 3 at the root of collection 1
```

So `"0/f1/r0"` reads as: collection 0 → folder 1 → request 0.

The format is exploited two ways:

- A "leaf" prefix (`c`/`f`/`r`) and index can be parsed off the last segment
  by `parseLeaf` ([items.go](items.go)) to identify what kind of node a given
  ID addresses.
- The parent container can be recovered by chopping the last segment with
  `parentID` ([tree.go](tree.go)).

### Tree helpers

| Function | Role |
| --- | --- |
| `Tree.ChildIDs(id)` | List child IDs (folders before requests) of the node at `id`. |
| `Tree.IsBranch(id)` | Folder/collection IDs are branches; `r<i>` leaves are not. |
| `Tree.Label(id)` | Display text (`"METHOD  Name"` for requests, plain name for folders/collections). |
| `Tree.Request(id)` | Pointer to the model request at `id`, or `(nil, false)` if `id` doesn't point to a request. The pointer aliases the in-memory collection — edits through it are seen by the session until reload. |
| `Tree.containerAt(id)` | Internal: walk parts, return the `(folders, requests)` at that container. |
| `Tree.containerAuth(id)` | Internal: return the `Auth` value on the collection or folder addressed by `id`. False for request nodes and out-of-range indices. |
| `Tree.AncestorAuths(id)` | Walks the parents of `id` (immediate parent first, collection root last) and returns each container's `Auth`. Consumed by `Session.EffectiveAuth` and the [internal/auth](../auth/) `Resolve` walk. |
| `Tree.nameAt(id)` | Internal: display name for a collection or folder ID. |
| `Tree.CollectionIndex(id)` | Convenience for the UI: which top-level collection does this node belong to. |

The mutating side lives on `Session` ([items.go](items.go)) and uses a pointer
variant `containerAtPtr` so changes write through to the loaded collections.

## Config types (re-exposed)

The `Session` constructor wraps these types from [`internal/config`](../config/):

- `config.Config` — workspaces, active index, settings, UI state.
- `config.Workspace` — workspace name + list of collection directories.
- `config.UIState` — active collection path, active environment per
  collection path, open-request reference, window size.
- `config.UIOpenRequest` — open-request reference: `{Collection path, NodePath}`
  where `NodePath` is the slash-separated node ID minus the collection index
  segment (e.g. `f1/r0`).
