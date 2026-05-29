# session

Package `session` is the runtime state for the active workspace. It ties the
persisted [`internal/config`](../config/) (workspaces, settings, UI state) to
the collections loaded from [`internal/storage`](../storage/), and exposes a
flat surface the UI can drive without knowing anything about disk layout.

A `Session` owns the loaded collections, an index of the active collection,
the active environment per collection, and the persisted UI state (open
request, window size). It also provides a [`Tree`](tree.go) navigation model
used by the Fyne `widget.Tree` to render collections, and item-mutation
helpers ([items.go](items.go)) that edit the in-memory collections and write
them back through the storage layer.

The active environment is the input to `Resolver()`, which produces a
[`vars.Resolver`](../vars/) that the UI uses to expand `{{name}}` templates
when displaying or sending requests.

## Public API

### Session lifecycle

- `New(cfgPath string) (*Session, error)` — load config and active workspace's
  collections; empty `cfgPath` uses defaults and skips persistence.

### Collections & workspace

- `Session.WorkspaceNames() []string`
- `Session.ActiveIndex() int`
- `Session.SetActive(i int)` — switch active workspace and reload.
- `Session.Collections() []model.Collection`
- `Session.OpenCollection(dir string) error` — add a collection on disk to the
  active workspace.
- `Session.SaveActiveCollection() error` — write the active collection back to
  its source directory.
- `Session.ActiveCollection() int` / `Session.SetActiveCollection(i int)`
- `Session.AddWorkspace(name string) error`
- `Session.RenameWorkspace(i int, name string) error`
- `Session.DeleteWorkspace(i int) error`

### Tree navigation

- `Session.Tree() *Tree`
- `Session.EffectiveAuth(nodeID string) model.Auth` — flatten Inherit for the
  request at `nodeID` by walking the folder → collection chain via
  [internal/auth](../auth/).
- `Tree.ChildIDs(id string) []string`
- `Tree.IsBranch(id string) bool`
- `Tree.Label(id string) string`
- `Tree.Request(id string) (*model.Request, bool)` — pointer into the loaded
  collections; edits through it are visible until reload.
- `Tree.AncestorAuths(id string) []model.Auth` — `Auth` from each container
  above `id`, immediate parent first, collection root last.
- `Tree.CollectionIndex(id string) int`

### Tree item mutation

- `Session.AddRequest(parentID, name string) (string, error)`
- `Session.AddFolder(parentID, name string) (string, error)`
- `Session.RenameItem(nodeID, name string) error`
- `Session.DeleteItem(nodeID string) error`
- `Session.DuplicateItem(nodeID string) (string, error)`

### Environments & resolution

- `Session.CollectionEnvironmentNames() []string`
- `Session.ActiveEnvName() string` / `Session.SetActiveEnv(name string)`
- `Session.ActiveEnvironment() *model.Environment`
- `Session.AddEnvironment(name string)`
- `Session.SetActiveEnvironmentVariables(variables []model.Variable)`
- `Session.Resolver() *vars.Resolver` — enabled env variables only.
- `ParseEnvVars(text string) []model.Variable` — `"key = value"` line text
  to variables; `#`-prefixed lines are disabled.
- `FormatEnvVars(vs []model.Variable) string` — inverse of `ParseEnvVars`.

### Settings & UI state

- `Session.Settings() model.Settings` / `Session.SetSettings(st model.Settings)`
- `Session.SetOpenRequest(nodeID string)` / `Session.OpenRequest() string`
- `Session.SetWindowSize(w, h int)` / `Session.WindowSize() (int, int)`

### OAuth2 tokens

- `Session.TokenCache() *auth.TokenCache` — process-lifetime cache used by
  the UI Send path to namespace OAuth2 tokens per session.
- `Session.ActiveCollectionDir() string` — the active collection's on-disk
  directory, used as the cache namespace prefix.

### Types

- `Session` — the in-memory application state for the active workspace.
- `Tree` — navigable view over the loaded collections, see
  [STRUCTURE.md](STRUCTURE.md).

## Dependencies

- [`internal/auth`](../auth/) — `auth.Resolve` powers `Session.EffectiveAuth`.
- [`internal/config`](../config/) — load/save the persisted YAML config.
- [`internal/model`](../model/) — domain types.
- [`internal/storage`](../storage/) — disk I/O for collections.
- [`internal/vars`](../vars/) — `{{name}}` template resolver.

Standard library only otherwise (`fmt`, `slices`, `strconv`, `strings`).
