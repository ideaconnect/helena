# config — Structure

## Files

| File | Responsibility |
| --- | --- |
| [doc.go](doc.go) | Package-level godoc only. |
| [config.go](config.go) | All persisted types (`Config`, `Workspace`, `UIState`, `UIOpenRequest`), defaults, path lookup, and `Load`/`Save`. |
| [config_test.go](config_test.go) | Round-trip and edge-case tests: missing file, empty path, save/load fidelity, clamping of `Active`. |
| [config_ui_test.go](config_ui_test.go) | Round-trip test for `UIState` (active collection, env map, open request pointer, window size). |

## Type catalog

### `Config` — [config.go:39](config.go#L39)
The root persisted document.
- `Workspaces` — ordered list; the UI shows them in a switcher.
- `Active` — index into `Workspaces`; `Load` clamps it to `0` if out of range so a hand-edited file can't crash the app.
- `Settings` — embedded `model.Settings`.
- `UI` — last-known session state (see `UIState`).

### `Workspace` — [config.go:15](config.go#L15)
A workspace entry inside the config.
- `Collections` — directory paths of OpenCollection folders the workspace currently includes. Collection contents are read by `internal/storage`, not by this package.

### `UIState` — [config.go:30](config.go#L30)
Restorable session state.
- `ActiveCollection` — directory path of the collection that was selected.
- `ActiveEnv` — map of collection-dir -> environment name, so each collection remembers its own active environment.
- `OpenRequest` — pointer to the request that had focus, or `nil` if none.
- `WindowWidth` / `WindowHeight` — last window size; restored at startup so the app reopens at its previous size.

### `UIOpenRequest` — [config.go:23](config.go#L23)
Locates an open request without using slice indices.
- `Collection` — directory path of the owning collection.
- `NodePath` — in-collection node path like `"f0/r1"` (the OpenCollection layout uses these directory-style paths so restoration is stable across reorders/renames of siblings).
