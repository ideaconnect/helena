# config

Persists Helena's application-level state to a single YAML file in the user's OS config directory (typically `~/.config/helena/config.yml` on Linux, equivalent locations on macOS/Windows).

What lives here: the list of workspaces and which one is active, the user's `Settings` (TLS, redirects, timeout, theme), global variables (#83 — the app-wide lowest-precedence resolver scope), and `UIState` (which collection/environment/request was last open, the set of open editor tabs + the active one, last window size). What does NOT live here: the contents of collections — those are written by the `storage` package to a separate OpenCollection directory; `config` only stores their on-disk paths. Open *scratch* tabs (unsaved, not in any collection) are likewise not persisted.

`Load` is forgiving: a missing file or an empty path yields `Default()` instead of an error, and an out-of-range `Active` index is clamped to 0. `Save` creates any missing parent directories.

### Schema versioning

`Config` carries a `Version int` (`version:` in YAML), written by `Save` and read by `Load`. The current schema is `CurrentSchemaVersion`. On load:

- A file with **no `version:` key** is treated as version 0 (pre-versioning) and migrated forward.
- A file at a **lower version** runs the ordered migration chain (`migrations[v]`, each a `func(*Config)`) up to the current version and is stamped to it. The v0→v1 step normalizes the legacy unsafe `TimeoutSeconds=0` to the default.
- A file at a **higher (future) version** is loaded best-effort — known fields are preserved and the version is kept — and a warning is emitted (`warnf`) instead of silently downgrading it. Note that keys newer than the current schema are dropped by the struct unmarshal and would be lost if the app re-saves, which is why the warning surfaces rather than swallowing the mismatch.

To evolve the schema: bump `CurrentSchemaVersion`, add a `migrations[N]` step that upgrades from `N-1`, and the rest is automatic. The same versioned-migration pattern can extend to the OpenCollection storage layer when its schema changes.

## Public API

### Types
- `Config` — top-level persisted state (schema `Version`, workspaces, active index, settings, UI state).
- `Workspace` — a named workspace plus the directory paths of its referenced collections.
- `UIState` — restorable UI state (active collection, active env per collection, open request, open tabs + active tab index, window size).
- `UIOpenRequest` — pointer to the currently open request by collection path + in-collection node path.
- `UIOpenTab` — pointer to one open editor tab by collection path + persistent `Request.ID` (ID-anchored so restoration survives request reordering).

### Functions
- `Default() Config` — returns a `Config` with a single empty `Default` workspace and `model.DefaultSettings()`.
- `DefaultPath() (string, error)` — returns the standard config file path (`<os user config dir>/helena/config.yml`).
- `Load(path string) (Config, error)` — reads and validates the config; missing file or empty path returns `Default()`.
- `Save(path string, c Config) error` — writes the config as YAML, creating parents as needed.

## Dependencies

### Internal
- [`internal/model`](../model) — embeds `model.Settings` and uses `model.DefaultSettings()`.

### External
- `gopkg.in/yaml.v3` — YAML (de)serialization.
- `os`, `path/filepath` (stdlib) — file I/O and path resolution via `os.UserConfigDir`.
