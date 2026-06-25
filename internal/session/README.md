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
- `Session.CollectionDir(i int) string` — on-disk directory of the loaded
  collection at index `i` (a stable key for open tabs across reordering).
- `Session.LoadErrors() []LoadError` — collections of the active workspace that
  failed to load on the most recent reload (a copy; `nil` when all loaded). The
  UI surfaces these so a corrupt/moved collection is not silently dropped.
- `Session.AddWorkspace(name string) error`
- `Session.RenameWorkspace(i int, name string) error`
- `Session.DeleteWorkspace(i int) error`

### Tree navigation

- `Session.Tree() *Tree`
- `Session.EffectiveAuth(nodeID string) model.Auth` — flatten Inherit for the
  request at `nodeID` by walking the folder → collection chain via
  [internal/auth](../auth/).
- `Session.LocateRequest(dir, requestID string) (string, *model.Request, bool)`
  — find a request by its persistent `Request.ID` within the collection at
  `dir`, returning its current node ID + a live pointer. Scoped to the owning
  collection so a forked-and-reopened collection's duplicate IDs never cross.
  Used by the UI tab strip to re-derive node IDs after tree mutations.
- `Session.ContainerPaths() []ContainerRef` — every container (collection roots
  + folders) across loaded collections, as `{Label, NodeID}` destinations for
  the scratch-tab "Save As" picker.
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
- `Session.AddRequestValue(parentID string, r model.Request) (string, error)` —
  the populated sibling of `AddRequest`: inserts a fully-formed request (mints a
  fresh ID), used when saving a scratch tab into a collection.
- `Session.AddFolder(parentID, name string) (string, error)`
- `Session.RenameItem(nodeID, name string) error`
- `Session.DeleteItem(nodeID string) error`
- `Session.DuplicateItem(nodeID string) (string, error)`

### Environments & resolution

- `Session.CollectionEnvironmentNames() []string`
- `Session.ActiveEnvName() string` / `Session.SetActiveEnv(name string)`
- `Session.ActiveEnvironment() *model.Environment`
- `Session.AddEnvironment(name string) error` — append a uniquely-named
  environment and persist (errors on empty/duplicate name or no active collection).
- `Session.RenameEnvironment(oldName, newName string) error` — rename + persist;
  the active selection follows a renamed active env.
- `Session.DeleteEnvironment(name string) error` — remove + persist; the active
  selection moves to the first remaining env (or clears) when the active one goes.
- `Session.SetActiveEnvironmentVariables(variables []model.Variable)`
- `Session.Resolver() *vars.Resolver` — ordered scopes (global < .env < collection variables < active environment < script overlay) plus the dynamic-variable fallback (`{{$guid}}` etc.). `SnapshotGlobalVars` / `SnapshotActiveDotEnvVars` / `SnapshotActiveCollectionVars` / `SnapshotActiveEnvVars` capture the lower scopes for the Send worker.
- `Session.ResolverForRequest(r *model.Request) *vars.Resolver` — `Resolver` plus the request's own variables (#82) layered as the highest static scope (global < .env < collection < environment < request < script overlay). A nil request behaves like `Resolver`. Used by the URL preview and exporter; the Send worker layers the request scope itself in `execution.go`.
- `Session.GlobalVariables() []model.Variable` / `SetGlobalVariables([]model.Variable) error` — read / persist the app-wide global variables (#83), the lowest static scope shared across every collection; stored in the config. `SnapshotGlobalVars()` copies the enabled set for the Send worker.
- `Session.SnapshotActiveDotEnvVars() map[string]string` — a copy of the active collection's `.env` variables (#84), parsed from `<collection>/.env` and cached (the cache is dropped on reload).
- `ParseEnvVars(text string) []model.Variable` — `"key = value"` line text
  to variables; `#`-prefixed lines are disabled.
- `FormatEnvVars(vs []model.Variable) string` — inverse of `ParseEnvVars`.

### Settings & UI state

- `Session.Settings() model.Settings` / `Session.SetSettings(st model.Settings)`
- `Session.SetOpenRequest(nodeID string)` / `Session.OpenRequest() string` —
  legacy single-open-request state; superseded by the tab set below but kept as
  a restore fallback.
- `Session.SetOpenTabs(tabs []config.UIOpenTab, active int)` /
  `Session.OpenTabs() ([]config.UIOpenTab, int)` — the open editor tabs (by
  collection dir + `Request.ID`) and the active index. `SetOpenTabs` clears the
  legacy `OpenRequest`.
- `Session.SetWindowSize(w, h int)` / `Session.WindowSize() (int, int)`

### OAuth2 tokens

- `Session.TokenCache() *auth.TokenCache` — process-lifetime cache used by
  the UI Send path to namespace OAuth2 tokens per session.
- `Session.ActiveCollectionDir() string` — the active collection's on-disk
  directory, used as the cache namespace prefix.

### Cookie jar

- `Session.CookieJar() *cookiejar.Jar` — the session-scoped cookie jar (#91),
  installed on every per-send Client so `Set-Cookie` responses persist and
  matching cookies are replayed on later sends. In-memory and process-lifetime
  only (never persisted), so it survives workspace/collection switches but not a
  restart. See [`internal/cookiejar`](../cookiejar).

### Types

- `Session` — the in-memory application state for the active workspace.
- `Tree` — navigable view over the loaded collections, see
  [STRUCTURE.md](STRUCTURE.md).
- `ContainerRef` — a `{Label, NodeID}` destination container returned by
  `ContainerPaths` for the scratch-tab "Save As" picker.
- `LoadError` — a `{Dir, Err}` record for a collection that failed to load
  during reload, returned by `LoadErrors`.

## Dependencies

- [`internal/auth`](../auth/) — `auth.Resolve` powers `Session.EffectiveAuth`.
- [`internal/config`](../config/) — load/save the persisted YAML config.
- [`internal/model`](../model/) — domain types.
- [`internal/storage`](../storage/) — disk I/O for collections.
- [`internal/vars`](../vars/) — `{{name}}` template resolver.

Standard library only otherwise (`fmt`, `slices`, `strconv`, `strings`).
