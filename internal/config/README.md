# config

Persists Helena's application-level state to a single YAML file in the user's OS config directory (typically `~/.config/helena/config.yml` on Linux, equivalent locations on macOS/Windows).

What lives here: the list of workspaces and which one is active, the user's `Settings` (TLS, redirects, timeout, theme), and `UIState` (which collection/environment/request was last open, last window size). What does NOT live here: the contents of collections — those are written by the `storage` package to a separate OpenCollection directory; `config` only stores their on-disk paths.

`Load` is forgiving: a missing file or an empty path yields `Default()` instead of an error, and an out-of-range `Active` index is clamped to 0. `Save` creates any missing parent directories.

## Public API

### Types
- `Config` — top-level persisted state (workspaces, active index, settings, UI state).
- `Workspace` — a named workspace plus the directory paths of its referenced collections.
- `UIState` — restorable UI state (active collection, active env per collection, open request, window size).
- `UIOpenRequest` — pointer to the currently open request by collection path + in-collection node path.

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
