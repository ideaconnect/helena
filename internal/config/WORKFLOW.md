# config — Workflow

## Loading config at startup
1. `main` calls `config.DefaultPath()`, which joins `os.UserConfigDir()` with `helena/config.yml`.
2. `main` calls `config.Load(path)`.
3. If the path is `""` or the file is missing, `Load` returns `Default()` (single empty workspace, default settings) with no error.
4. Otherwise the YAML is unmarshaled into `Config`.
5. `Load` validates: an empty `Workspaces` slice is replaced with the default; an out-of-range `Active` is reset to `0`.
6. The returned `Config` seeds the session — workspaces become the switcher entries, `Settings` initialize the HTTP client and theme, `UIState` triggers restoration of the previously open collection/request/window.

## Saving config after a state change
1. Any state mutation (workspace added/removed, active changed, setting toggled, window resized, request opened) calls `config.Save(path, c)`.
2. `Save` runs `os.MkdirAll(filepath.Dir(path), 0o755)` so a first-run write succeeds even when `~/.config/helena/` does not yet exist.
3. The struct is marshaled to YAML and written via `os.WriteFile` with mode `0o644`.

## Restoring UI state from `UIState`
1. After `Load`, the session reads `cfg.UI`.
2. If `ActiveCollection` matches a known collection path, that collection is selected in the tree.
3. For each known collection, `ActiveEnv[collectionDir]` selects the named environment (falling back to "no env" if absent).
4. If `OpenRequest` is non-nil and its `Collection`/`NodePath` resolve to an existing request, the request opens in the editor.
5. `WindowWidth`/`WindowHeight`, when non-zero, are applied to the main window.

## Clamping a corrupted `Active` index
1. A hand-edited or stale config sets `Active: 9` while `Workspaces` has length 1.
2. `Load` parses the YAML, then checks `c.Active < 0 || c.Active >= len(c.Workspaces)`.
3. The condition is true, so `c.Active` is set to `0`.
4. The session starts on the first workspace; the next `Save` overwrites the bad value.
