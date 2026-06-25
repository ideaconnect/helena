# session — structure

## Files

| File | Responsibility |
| --- | --- |
| [session.go](session.go) | `Session` type, constructor `New`, workspace switching, active collection / environment (incl. `MoveCollection` — reorders the top-level collection list with final-index semantics, keeping `cols`/`dirs`/workspace in lockstep and the active pointer on its collection), UI state persistence (open request, open tabs, window size, settings), `Resolver()` / `ResolverForNode()` / `ResolverForRequest()` (folder-scoped #81 + request-scoped #82 variables) and the `enabledVars` helper, folder variables (#81: `SnapshotAncestorVars` / `FolderVariables` / `SetFolderVariables`), global variables (#83: `GlobalVariables` / `SetGlobalVariables` / `SnapshotGlobalVars`, the lowest scope, persisted in config), env overlay (`SetEnvOverlay` / `EnvOverlay` / `ClearEnvOverlay` / `SnapshotEnvOverlay` / `SnapshotActiveEnvVars`), request location for the UI tab strip (`LocateRequest`, `ContainerPaths` / `ContainerRef`, `CollectionDir`), open-tab persistence (`SetOpenTabs` / `OpenTabs`), `SaveActiveCollection` / `saveCollection`, and `FindRequestByPath` used by [internal/chain](../chain/). |
| [dotenv.go](dotenv.go) | Collection-root `.env` support (#84): `parseDotEnv` (KEY=VALUE, comments, `export`, quotes), `activeDotEnvVars` (lazy per-dir cache, lowest static scope), and `SnapshotActiveDotEnvVars` for the Send worker. |
| [tree.go](tree.go) | `Tree` navigation model used by the Fyne `widget.Tree`. Defines the node ID format and the lookups that drive `widget.Tree` callbacks, plus `Search` (#67 — the pure cross-collection filter) and `AncestorVars` / `containerFolder` (#81 — the merged folder-scoped variables on a node's ancestors, inner folders winning). |
| [items.go](items.go) | Tree mutation: `AddRequest`, `AddRequestValue`, `AddFolder`, `RenameItem`, `DeleteItem`, `DuplicateItem`, `MoveNode`, plus one-step delete undo (#68): `DeleteItem` snapshots the removed node into `deletedNode`, and `RestoreLastDeleted` / `CanUndoDelete` / `LastDeletedName` re-insert it at its original (clamped) position. `cloneRequestKeepID` / `cloneFolderKeepID` deep-copy while preserving IDs (unlike the duplication clones), so chain refs survive a delete→restore. Each mutator persists. `MoveNode` relocates a folder/request within the same collection (drag-and-drop): it removes the item, re-resolves the destination container by a stable folder `model.ID` (the removal shifts positional node IDs), inserts, cascades chain refs like a rename, and saves. Helpers: `folderIDForContainer`, `containerByFolderID`, `nodeIDOfModel`, `clampIndex`. |
| [workspace.go](workspace.go) | Workspace CRUD: `AddWorkspace`, `RenameWorkspace`, `DeleteWorkspace`. |
| [env.go](env.go) | Plain-text environment-variable parsing and formatting: `ParseEnvVars` / `FormatEnvVars`. |
| [session_test.go](session_test.go) | Open-collection persistence + tree navigation. |
| [move_test.go](move_test.go) | `MoveNode` (request into folder, sibling reorder with index adjustment, folder-into-folder, reject cross-collection + folder-into-self/descendant) and `MoveCollection` (reorder + active-pointer follow + persistence). |
| [session_auth_test.go](session_auth_test.go) | `EffectiveAuth` resolution covering own-wins, folder→collection inheritance, fallback to `AuthNone`, and unknown-id safety. |
| [session_env_test.go](session_env_test.go) | Resolver, env editing persistence, parse/format env vars. |
| [session_overlay_test.go](session_overlay_test.go) | Script-set env overlay: basic round-trip, clear, Resolver layering (overlay > active env), non-persistence invariant, and concurrent-safety with `-race`. |
| [session_chain_test.go](session_chain_test.go) | `FindRequestByPath` resolution: top-level requests, nested folders, leading-slash tolerance, unknown paths, empty paths, and the no-active-collection short-circuit. |
| [session_save_test.go](session_save_test.go) | Round-trip of request edits through Tree pointer + save. |
| [session_settings_test.go](session_settings_test.go) | Settings persistence. |
| [session_uistate_test.go](session_uistate_test.go) | UI state persistence (active collection, active env, open request, window size) and stability across collection reordering. |
| [session_tabs_test.go](session_tabs_test.go) | `LocateRequest` (root / nested / not-found / re-derive-after-delete / collection-scoped duplicate IDs), `AddRequestValue`, `ContainerPaths`, `CollectionDir`, and `SetOpenTabs` / `OpenTabs` round-trip + stability across request reordering. |
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
| `jar` | `*cookiejar.Jar` | Session-lifetime cookie jar (#91) returned by `CookieJar()` and installed on every per-send Client. **In-memory only** — never persisted. Constructed in `New`, untouched by `reload`. |
| `dotEnv` | `map[string]map[string]string` | Per-collection-dir cache of parsed `.env` variables (#84), the lowest static resolver scope. Lazy-populated on the UI goroutine; dropped by `reload` so reopened collections re-read from disk. |

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
  collection path, open-request reference, open tabs + active tab, window size.
- `config.UIOpenRequest` — open-request reference: `{Collection path, NodePath}`
  where `NodePath` is the slash-separated node ID minus the collection index
  segment (e.g. `f1/r0`).
- `config.UIOpenTab` — open-tab reference: `{Collection path, RequestID}`.
  Anchored by `Request.ID` (not node path) so a restored tab survives request
  reordering within the collection.
