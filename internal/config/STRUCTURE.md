# config — Structure

## Files

| File | Responsibility |
| --- | --- |
| [doc.go](doc.go) | Package-level godoc only. |
| [config.go](config.go) | All persisted types (`Config`, `Workspace`, `UIState`, `UIOpenRequest`, `UIOpenTab`), defaults, path lookup, `Load`/`Save`, and the schema-version machinery (`CurrentSchemaVersion`, `migrations`, `migrate`, `migrateTo1`). |
| [migrate_test.go](migrate_test.go) | Schema-migration tests: legacy (unversioned) config migrates forward, future-version config warns without data loss, current-version no-op. |
| [config_test.go](config_test.go) | Round-trip and edge-case tests: missing file, empty path, save/load fidelity, clamping of `Active`. |
| [config_ui_test.go](config_ui_test.go) | Round-trip test for `UIState` (active collection, env map, open request pointer, open tabs + active index, window size) and the empty-tabs `omitempty` check. |

## Type catalog

### `Config` — [config.go:39](config.go#L39)
The root persisted document.
- `Workspaces` — ordered list; the UI shows them in a switcher.
- `Active` — index into `Workspaces`; `Load` clamps it to `0` if out of range so a hand-edited file can't crash the app.
- `Settings` — embedded `model.Settings`.
- `UI` — last-known session state (see `UIState`).
- `Variables` — global variables (#83): app-wide `[]model.Variable`, the lowest-precedence resolver scope, shared across every collection.

### `Workspace` — [config.go:15](config.go#L15)
A workspace entry inside the config.
- `Collections` — directory paths of OpenCollection folders the workspace currently includes. Collection contents are read by `internal/storage`, not by this package.

### `UIState` — [config.go:40](config.go#L40)
Restorable session state.
- `ActiveCollection` — directory path of the collection that was selected.
- `ActiveEnv` — map of collection-dir -> environment name, so each collection remembers its own active environment.
- `OpenRequest` — pointer to the request that had focus, or `nil` if none. Legacy single-request state; superseded by `OpenTabs` but kept as a restore fallback for configs written before tabs existed.
- `OpenTabs` — the open editor tabs (`omitempty`, so a tab-less session keeps a clean file).
- `ActiveTab` — index into `OpenTabs` of the focused tab (`omitempty`).
- `WindowWidth` / `WindowHeight` — last window size; restored at startup so the app reopens at its previous size.
- `ResponseWrap` — the response viewer's soft-wrap toggle (`omitempty`; off is the default, so the key appears only once a user turns wrapping on). Restored at startup so the viewer reopens in the mode it was left in.

### `UIOpenRequest` — [config.go:23](config.go#L23)
Locates an open request without using slice indices.
- `Collection` — directory path of the owning collection.
- `NodePath` — in-collection node path like `"f0/r1"` (the OpenCollection layout uses these directory-style paths so restoration is stable across reorders/renames of siblings).

### `UIOpenTab` — [config.go:33](config.go#L33)
Locates one open editor tab.
- `Collection` — directory path of the owning collection.
- `RequestID` — the target's persistent `Request.ID`. Anchoring by ID (not node path) keeps a restored tab pointing at the right request even after siblings are reordered, inserted, or deleted. Scratch (unsaved) tabs are not persistable and never produce a `UIOpenTab`.
