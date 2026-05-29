# internal/ui — structure

## Files

| File | Purpose |
| ---- | ------- |
| [doc.go](doc.go) | Package doc comment. |
| [shell.go](shell.go) | `MainUI` struct, `NewMainUI`, the main layout, send/save/loadRequest, params/headers row machinery, environment + settings dialogs, body validate/format. Also defines `sessionEnvBridge` (the `scripting.EnvBridge` adapter), `sessionRequestFinder` (the `chain.RequestFinder` adapter), and `chainExecutor` — the single execution path that runs pre-script → `client.Do` → post-script for both chain steps and the leaf. |
| [items.go](items.go) | Tree CRUD actions (new request, new folder, rename, duplicate, delete) plus `parentForNew`, `promptName`, `nameOfNode`, `isAncestor` helpers. |
| [workspaces.go](workspaces.go) | `editWorkspaces` dialog and `refreshWorkspaceDropdown`. |
| [collections.go](collections.go) | `actionNewCollection` — prompt + folder picker + empty YAML write. |
| [import.go](import.go) | `actionImport` chooser plus URL / file paths and `slugify` / `uniqueCollectionDir` helpers. |
| [export.go](export.go) | `actionExport` — cURL / wget snippet dialog plus `newSnippetEntry`. |
| [docs.go](docs.go) | `buildDocsTab` and `refreshDocsPreview` — per-request Markdown editor with rendered preview subtab. |
| [scripts.go](scripts.go) | `buildScriptsTab` — the Pre-request / Post-response code editors and the read-only Console panel below. `loadScriptsTab` populates the editors during `loadRequest`; `setScriptConsole` renders the captured console output after each Send. |
| [chain.go](chain.go) | `buildChainTab` — the list of (Alias, Request path) rows for declaring before-hooks. `loadChainTab` / `rebuildChainRows` / `addChainStep` / `buildChainRow` follow the same patterns as the Params and Headers tabs. |
| [auth.go](auth.go) | `buildAuthTab`, `loadAuthTab`, `refreshAuthVisibility`, `refreshAuthInheritLabel`, and the `ensureBasic`/`ensureBearer`/`ensureAPIKey`/`ensureOAuth2` lazy allocators for the per-type sub-structs. |
| [oauth2.go](oauth2.go) | `fyneAuthCodeStarter` — adapter that hands the authorization URL to `fyne.CurrentApp().OpenURL`. The `newAuthCodeStarter` package-level var lets tests swap in a fake. |
| [theme.go](theme.go) | `ApplyTheme` plus the `themeName` / `themeFromName` string mapping used by the picker. |
| [shortcuts.go](shortcuts.go) | `shortcutSpec`, `registerShortcuts`, `showShortcuts`, `shortcutModifierName`, and `shortcutRowLayout`. |
| [shell_test.go](shell_test.go) | `NewMainUI` construction + headless layout smoke test. |
| [docs_test.go](docs_test.go) | Docs editor load + preview + write-back, plus clear-on-nil behaviour. |
| [scripts_test.go](scripts_test.go) | Scripts tab load + write-back, loading-flag suppression across request swaps, clear-on-nil for both editors and the console, console rendering (incl. truncation), and the `sessionEnvBridge` adapter. |
| [chain_test.go](chain_test.go) | Chain tab load + add/delete, loading-flag suppression across request swaps, and `pruneEmptyChain` save-time filter. |
| [auth_test.go](auth_test.go) | Auth tab load + write-back for Bearer / Basic / API-Key, type-change → Auth.Type, and m.loading suppression. |
| [shortcuts_test.go](shortcuts_test.go) | Shortcut registration, modifier label, dialog open, and nil-window short-circuit. |
| [theme_test.go](theme_test.go) | `themeName` / `themeFromName` round-trip and `ApplyTheme` panic-safety. |

## MainUI struct

`MainUI` lives in [shell.go](shell.go#L27). It is the single owner of every
long-lived widget plus the bookkeeping required to load and save a request
without infinite write-back loops.

### Fields

| Field | Type | Role |
| ----- | ---- | ---- |
| `sess` | `*session.Session` | All persisted state — workspaces, collections, environments, settings, and tree access. |
| `win` | `fyne.Window` | Parent for dialogs; **set late** by `SetWindow`, so dialog-opening actions guard for nil — see "Late `m.win`" below. |
| `Workspace` | `*widget.Select` | Toolbar workspace dropdown. |
| `Environment` | `*widget.Select` | Toolbar environment dropdown (with `noEnv` first option). |
| `Method` | `*widget.Select` | HTTP method picker on the address bar. |
| `URL` | `*widget.Entry` | URL entry; Enter triggers `send`. |
| `urlPreview` | `*widget.Label` | Italic label under the URL showing the resolved form (or unresolved-vars warning). Hidden when nothing to show. |
| `Save` | `*widget.Button` | Disabled until a request is loaded. |
| `Send` | `*widget.Button` | "Send" by default (high importance) / "Abort" while a Send is in flight (warning importance). Tap routes through `sendOrAbort` which dispatches based on `sendCancel`. |
| `Tree` | `*widget.Tree` | Collections sidebar tree. |
| `Request` | `*container.AppTabs` | Request editor tabs: Params, Auth, Headers, Body, Docs. |
| `Response` | `*container.AppTabs` | Response tabs: Pretty, Raw, Headers. |
| `Status` | `*widget.Label` | Footer status line. |
| `paramsRows` | `*fyne.Container` | VBox of KV rows for query params. |
| `headersRows` | `*fyne.Container` | VBox of KV rows for headers. |
| `BodyType` | `*widget.Select` | Body type select (none / json / xml / text / form / multipart). |
| `BodyContent` | `*widget.Entry` | Body text area. |
| `docsEditor` | `*widget.Entry` | Markdown source editor in the Docs tab. |
| `docsPreview` | `*widget.RichText` | Rendered Markdown shown in the Docs > Preview subtab. |
| `preScriptEditor` | `*widget.Entry` | Monospace editor for `request.Scripts.PreRequest` source. |
| `postScriptEditor` | `*widget.Entry` | Monospace editor for `request.Scripts.PostResponse` source. |
| `scriptConsole` | `*widget.Entry` | Read-only console panel below the script editors. Filled by `setScriptConsole` with the joined console lines from the last Send's chain steps + leaf pre+post results. Capped at `scriptConsoleMaxLines`. |
| `chainRows` | `*fyne.Container` | VBox of (Alias, Request path) rows for the Chain tab. Rebuilt by `rebuildChainRows` after add/delete or loadRequest. |
| `authType` | `*widget.Select` | Auth Type dropdown (None / Inherit / Basic / Bearer / API Key / OAuth 2.0). Drives `refreshAuthVisibility`. |
| `authBasic*` / `authBearer*` / `authAPIKey*` / `authOAuth2*` | various entries / selects / check | Per-type form widgets. Each `OnChanged` calls the matching `ensure*` allocator if the sub-struct is nil. |
| `authInheritLabel` | `*widget.Label` | Live preview text showing what `session.EffectiveAuth(currentRequestID)` would resolve to, refreshed on load and type change. |
| `authNonePanel` / `authInheritPanel` / `authBasicPanel` / `authBearerPanel` / `authAPIKeyPanel` / `authOAuth2Panel` | containers / `*widget.Form` | The six stacked form panels; only the one matching the selected Type is shown. |
| `authFormsStack` | `*fyne.Container` (Stack) | Stack container holding all six panels — `refreshAuthVisibility` hides every panel then shows the active one. |
| `authOAuth2ClearTokens` | `*widget.Button` | "Clear cached tokens" button on the OAuth2 panel — calls `Session.TokenCache().ClearAll()` so a rotated client secret forces the next Send to refetch. |
| `responseRaw` | `*widget.Entry` | Raw response body view. |
| `prettyText` | `*widget.Entry` | Pretty-printed JSON/XML view. |
| `headersText` | `*widget.Entry` | Response headers view. |
| `corsBanner` | `*canvas.Text` | Orange banner above the response panel surfacing CORS warnings. |
| `currentRequest` | `*model.Request` | Pointer to the request currently bound to the editor widgets. Direct writes happen via `OnChanged` callbacks. |
| `currentRequestID` | `string` | Tree node ID for `currentRequest`; cleared when the selected node is deleted. |
| `lastSelectedNodeID` | `string` | Last node the user selected; the basis for `parentForNew`, rename, delete, and duplicate targets. |
| `loading` | `bool` | **Write-back suppression flag.** Set true by `loadRequest` while it pushes values into widgets so the `OnChanged` callbacks (which would write back into `currentRequest`) become no-ops. Without this, programmatic SetText/SetSelected calls would clobber the model with the previous request's data. |
| `sendCancel` | `context.CancelFunc` | Non-nil while a Send goroutine is in flight; lets `sendOrAbort` route a button tap into context cancellation. Set on the UI thread when `send` launches the goroutine, cleared by `resetSendButton` in every teardown path. |
| `shortcuts` | `[]shortcutSpec` | Cached shortcut table used both for canvas registration and for rendering the help dialog. |
| `root` | `fyne.CanvasObject` | The fully assembled top-level container returned by `Root()`. |

### The `loading` flag

Every editable widget owned by `MainUI` writes back through an `OnChanged` (or
the `Select` callback) into `currentRequest`. When the user clicks a tree
node, `loadRequest` needs to push the new request's data into those same
widgets. Without protection, each `SetText` / `SetSelected` would fire
`OnChanged` and immediately overwrite the freshly loaded request with the
values left over from the previous one — a write-back loop.

`loadRequest` therefore sets `m.loading = true` for the duration of the update
(deferred reset). Each widget callback checks `if !m.loading && m.currentRequest != nil`
before writing. The same flag is checked from the Docs editor and the Body
content callbacks. This is the only synchronisation needed because Fyne
callbacks run on the UI goroutine.

### Late `m.win`

`NewMainUI` runs before the parent window exists — `cmd/helena` constructs the
UI, calls `SetWindow`, then `ShowAndRun`. Any method that opens a dialog
(`editEnvironments`, `editSettings`, `editWorkspaces`, `openCollection`,
`actionImport`, `actionExport`, `actionNewCollection`, every `action*` in
items.go, `showShortcuts`) starts with `if m.win == nil { return }` so that
shortcuts firing before window assignment, or unit tests that skip
`SetWindow`, can't crash the program. `registerShortcuts` short-circuits the
same way.

## Other notable types

### `shortcutSpec` (shortcuts.go)

```go
type shortcutSpec struct {
    keyName  fyne.KeyName
    extraMod fyne.KeyModifier
    label    string
    action   string
    do       func()
}
```

`extraMod` is OR'd with `fyne.KeyModifierShortcutDefault` (Ctrl on Linux/Windows,
Command on macOS) when registering the binding. Leave it zero for plain
Mod+key bindings; set it to `fyne.KeyModifierShift` to make a Mod+Shift+key
binding that can coexist with the plain Mod+key version. That is how
`Mod+Shift+N` (New collection) sits alongside `Mod+N` (New request) without
either swallowing the other.

### `shortcutRowLayout` (shortcuts.go)

A two-column layout used by the shortcuts help dialog: a 110-px-minimum key
column on the left and a flexible action label on the right. Simpler than a
grid because only one column has a known minimum width.

## Startup, in brief

See [WORKFLOW.md](WORKFLOW.md) for the full lifecycle. The short version:

1. `cmd/helena` creates the `fyne.App` and `*session.Session`.
2. `ui.ApplyTheme` runs before any window exists.
3. `mainUI := ui.NewMainUI(sess)` builds every widget and assigns them to the
   exported fields.
4. The window is created, then `mainUI.SetWindow(w)` records the dialog parent
   and registers shortcuts against `w.Canvas()`.
5. `w.SetContent(mainUI.Root())` shows the UI.
