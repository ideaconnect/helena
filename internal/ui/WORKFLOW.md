# internal/ui — workflows

This file walks through the most important runtime flows in the UI module.
Where a flag or guard matters (the `loading` write-back suppression, the
nil-`win` guard, the off-UI-goroutine send), it is called out explicitly.

## Application startup (main.go → NewMainUI → SetWindow)

[cmd/helena/main.go](../../cmd/helena/main.go) drives this sequence:

1. `app.NewWithID("tech.idct.helena")` — create the Fyne app.
2. `a.SetIcon(...)` from `assets.AppIcon`.
3. `config.DefaultPath()` resolves the persistent config path; on failure the
   app runs in-memory.
4. `sess, _ := session.New(cfgPath)` — load workspaces, last-open requests,
   settings, recent windows.
5. `ui.ApplyTheme(a, sess.Settings().Theme)` switches Fyne's theme **before**
   widget construction so initial layout uses the right palette.
6. `w := a.NewWindow("Helena")` and `w.Resize(...)` honour the session's
   remembered window size (default 1100x720).
7. `mainUI := ui.NewMainUI(sess)` —
   ([shell.go:67](shell.go#L67)) builds every widget, wires `OnChanged`
   callbacks, assembles the toolbar / address bar / sidebar / split layout,
   calls `refreshEnvironments`, and if `sess.OpenRequest()` is non-empty
   selects the previously open node.
8. `mainUI.SetWindow(w)` — ([shell.go:216](shell.go#L216)) records `m.win` and
   calls `registerShortcuts`. **Before this call `m.win` is nil and dialog
   actions short-circuit.** Keyboard shortcuts are not yet active either.
9. `w.SetContent(mainUI.Root())` — installs the root canvas object.
10. `a.Lifecycle().SetOnStopped(...)` registers a callback that persists the
    final window size into the session.
11. `w.ShowAndRun()` — Fyne main loop.

## Selecting a tree node and loading a request

Each row in the collections tree carries an ID. The handler is registered in
`buildTree` at [shell.go:221](shell.go#L221).

1. The tree fires `OnSelected(id)`.
2. `m.lastSelectedNodeID = id` (used later by `parentForNew`, rename, delete,
   duplicate).
3. If the ID resolves to a collection (`sess.Tree().CollectionIndex(id) >= 0`),
   `sess.SetActiveCollection(...)` updates the active collection and
   `refreshEnvironments` reseeds the Environment dropdown for that collection.
4. `sess.Tree().Request(id)` returns the `*model.Request` if the node is a
   request. Otherwise the handler returns and the editor remains as it was.
5. `m.loadRequest(req, id)` — ([shell.go:249](shell.go#L249)) **sets
   `m.loading = true` for the duration**, then:
   - Stores `currentRequest` / `currentRequestID`.
   - Enables the Save button.
   - Pushes `req.Method`, `req.URL`, `req.Body.Type`, `req.Body.Content`,
     `req.Docs` into widgets.
   - Calls `rebuildParamsRows` and `rebuildHeadersRows` to drop the prior
     request's KV row widgets and create new ones bound to the new slice.
   - Calls `refreshDocsPreview` and `updateURLPreview`.
   The deferred `m.loading = false` re-arms widget write-back.
6. Status line shows `"Loaded: <name>"` and `sess.SetOpenRequest(id)` records
   the selection for next launch.

### Why the loading flag matters

Every `SetText` / `SetSelected` call in step 5 triggers the widget's
`OnChanged`. Those callbacks normally write into `m.currentRequest`:

```go
m.URL.OnChanged = func(s string) {
    if !m.loading && m.currentRequest != nil {
        m.currentRequest.URL = s
    }
    m.updateURLPreview()
}
```

Without the `!m.loading` guard, the call to `m.URL.SetText(req.URL)` would
write the loaded URL back into `currentRequest` (harmless), and then the next
SetText on `BodyContent` would do the same — but inside more complex
programmatic updates (KV rows, body type changes that reset content) the
write-back loop can clobber freshly loaded data with stale values. The flag
makes `loadRequest` a single atomic UI write.

## Sending a request (UI → httpclient → fyne.Do)

`send` lives at [shell.go:490](shell.go#L490). It is bound to the Send button
(`m.Send.OnTapped = m.send`), to the URL entry's `OnSubmitted`, and to the
Enter keyboard shortcut.

1. Trim-check the URL; bail early if empty.
2. Snapshot the request: a value copy of `*m.currentRequest`, or a fresh
   `model.Request{Method, URL}` if nothing is loaded.
3. **Flatten Inherit auth on the copy.** When a request is loaded, overwrite
   `req.Auth` with `m.sess.EffectiveAuth(m.currentRequestID)` so the
   downstream `httpclient.Build` → `auth.Apply` chain sees the concrete
   auth (Basic / Bearer / API-Key / etc.) instead of the literal `Inherit`
   sentinel. Done on a *copy*, so the in-memory request the user is editing
   keeps its `Inherit` value.
4. Build a fresh `httpclient.New(m.sess.Settings())`, install an
   OAuth2 resolver via `client.SetOAuth2Resolver(auth.NewOAuth2Resolver(sess.TokenCache(), nil, sess.ActiveCollectionDir(), newAuthCodeStarter()))`,
   and grab `m.sess.Resolver()` — variable substitution uses the active
   env. The OAuth2 resolver uses `http.DefaultClient` for the token
   endpoint (the user's TLS / timeout settings deliberately don't apply
   to the auth server — those settings are for the API under test).
   `newAuthCodeStarter` returns the Fyne adapter from
   [oauth2.go](oauth2.go), which opens the browser via
   `fyne.CurrentApp().OpenURL` when an authorization_code flow needs
   the user to approve scopes.
5. Build a `scripting.Runtime` via
   `scripting.New(sessionEnvBridge{s: m.sess})`. The bridge's `Get`
   reads through `Session.Resolver().Lookup` (overlay over active env)
   and `Set` calls `Session.SetEnvOverlay`. The runtime is cheap to
   construct; we make one per Send so any session changes between Sends
   are picked up automatically.
6. Status -> "Sending…", Send button disabled, CORS banner hidden — all on
   the UI goroutine.
7. **Spawn a goroutine.** Inside, first run the pre-request hook:
   - If `req.Scripts.IsEmpty()` is true, skip the runtime entirely.
   - Otherwise `rt.RunPreRequest(ctx, req.Scripts.PreRequest, &req)`.
     A non-nil error short-circuits the Send: post the error to the
     Raw response panel, dump any captured console lines into the
     Script Console panel, re-enable Send, and return. **The request
     is never sent when the pre-script fails.**
   - On success, build the resolver from the captured `envSnap` plus
     a fresh `m.sess.SnapshotEnvOverlay()` so any `helena.env.set`
     calls the pre-script made become visible to `httpclient.Build`
     without the worker having to call `m.sess.Resolver()` (which
     would race against UI-thread env mutations).
8. Call `client.Do(ctx, req, resolver)`. This blocks on the network
   and must not run on the UI goroutine.
9. If Do returned a response and `PostResponse` is non-empty, run
   `rt.RunPostResponse(ctx, req.Scripts.PostResponse, req,
   scripting.ResponseInput{...})`. Errors from the post-script don't
   block the response from being shown — they're appended to the
   status line ("200 OK · … · post-script: …"). Mutations on the
   request inside the post-script are ignored — the request has
   already gone over the wire.
10. Inside the goroutine, marshal results back via `fyne.Do(func() { ... })`.
   `fyne.Do` is the only safe way to touch widgets from a non-UI goroutine;
   it schedules the callback onto Fyne's event loop.
11. The `fyne.Do` body:
    - Re-enables Send and renders the pre + post console lines into
      `scriptConsole`.
    - On error: selects the Raw tab, sets the status line, dumps the error text
      into the Raw entry, clears Pretty and Headers.
    - On success: fills `responseRaw`, `headersText`; runs
      `responsefmt.PrettyJSON` / `PrettyXML` based on Content-Type to fill
      Pretty when applicable; selects Pretty or Raw depending on whether
      pretty-printing succeeded.
    - Sets status to `"<Status> · <size> · <duration>"`, possibly
      suffixed with `· post-script: <err>`.
    - If `resp.CORSWarning != ""`, shows the orange `corsBanner`.

The Send button is the lifecycle marker: disabled when the goroutine is in
flight, re-enabled in the `fyne.Do` block. Pressing Enter twice in a row
doesn't fire a second request mid-flight because Fyne's shortcut handler
calls `m.send` which still trips on the disabled-button state implicitly
(the second send simply queues another HTTP request but the first one was
already in flight). The simpler invariant is: keyboard repeats are rare and
the worst case is two concurrent sends, both safely routed through `fyne.Do`.

## Saving a request to disk

`saveRequest` at [shell.go:296](shell.go#L296), wired to the Save button and
the `Mod+S` shortcut.

1. If no request is loaded, set status and bail.
2. `currentRequest.Params = pruneEmptyKV(...)` and same for Headers — strips
   blank "+ Add" rows so they don't end up in YAML.
3. `rebuildParamsRows` / `rebuildHeadersRows` re-draw the editor after the
   slice shrink so deleted blank rows actually disappear.
4. `m.sess.SaveActiveCollection()` writes the collection's YAML files via
   `internal/storage`. On error: status update + dialog (only if `m.win` is
   set).
5. Status -> `"Saved: <name>"` on success.

## Opening / importing / creating a collection

These three flows all funnel into `session.OpenCollection(dir)` once the
target directory is known.

### Open (existing folder)

`openCollection` at [shell.go:469](shell.go#L469). Folder picker -> on
selection, `sess.OpenCollection(path)` -> `m.Tree.Refresh()` + status update.

### New collection

`actionNewCollection` at [collections.go:17](collections.go#L17):

1. `promptName` -> user types a name.
2. `dialog.ShowFolderOpen` -> user picks a parent directory.
3. `uniqueCollectionDir(parent, name)` returns a slug with a numeric suffix
   if needed so two new collections in the same parent don't collide.
4. `os.MkdirAll`, `appstorage.Save(model.Collection{Name: name}, dir)` —
   writes an empty `opencollection.yml`.
5. `sess.OpenCollection(dir)`, refresh tree, refresh envs.

### Import (OpenAPI / Swagger / WSDL)

`actionImport` at [import.go:26](import.go#L26) shows a dialog with two
options: a URL entry + Fetch button, or a local file picker.

- **URL** -> `importFromURL` runs `importer.FromURL(url, settings)` in a
  goroutine (TLS / timeout honour `model.Settings`) and returns to the UI
  via `fyne.Do`.
- **File** -> `importFromFile` reads the bytes, calls `importer.From(data)`.

Both paths land in `chooseImportDestination(c)`:

1. Folder picker for the parent directory.
2. `uniqueCollectionDir`, `os.MkdirAll`, `appstorage.Save(c, dir)`.
3. `sess.OpenCollection(dir)` + tree/env refresh + status update.

## Switching workspaces

User picks a workspace in the toolbar Select:

1. `onWorkspaceChanged(name)` finds the index and calls `sess.SetActive(i)`.
2. `m.Tree.Refresh()` re-reads `sess.Tree()` so the sidebar shows that
   workspace's collections.

The Workspaces… button opens `editWorkspaces`
([workspaces.go:14](workspaces.go#L14)), a list-style dialog with Add /
Rename / Delete buttons. After mutation, `refreshWorkspaceDropdown` reseeds
the toolbar Select. Delete also calls `loadRequest(nil, "")` to clear the
editor (the previously open request may have been in the deleted workspace).

## Switching environments

User picks an environment in the toolbar Select:

1. `onEnvChanged(name)` -> `sess.SetActiveEnv("")` for `noEnv`, otherwise
   `sess.SetActiveEnv(name)`.
2. `updateURLPreview()` re-runs the resolver against the new env so the
   italic preview label reflects the change instantly.

The Environments… button opens `editEnvironments` at
[shell.go:580](shell.go#L580):

1. Bails if no collection is open.
2. If the active env name is empty, takes the first env or creates a
   "Default" one.
3. Shows a multi-line `key = value` entry pre-filled with
   `session.FormatEnvVars(env.Variables)`.
4. On Save: `session.ParseEnvVars` -> preserves the `Secret` flag for
   pre-existing secret keys -> `SetActiveEnvironmentVariables` ->
   `SaveActiveCollection` -> `refreshEnvironments` + `updateURLPreview`.

## Changing theme

The Settings… button opens `editSettings` at [shell.go:643](shell.go#L643).
The Theme row is a Select pre-populated via `themeName(s.Theme)`. On Save:

1. `themeFromName(themeSelect.Selected)` maps the label back to a
   `model.Theme`.
2. `sess.SetSettings(...)` persists the new value.
3. `ApplyTheme(fyne.CurrentApp(), newTheme)` switches the live Fyne theme so
   the change takes effect without restart.

## Keyboard shortcut dispatch

Registered by `registerShortcuts` at [shortcuts.go:29](shortcuts.go#L29),
called from `SetWindow`.

1. Build `m.shortcuts` — a slice of `shortcutSpec` with key, optional extra
   modifier (`fyne.KeyModifierShift` for Mod+Shift+N), human label, action
   label, and a closure.
2. For each spec, `c.AddShortcut(&desktop.CustomShortcut{KeyName,
   Modifier: fyne.KeyModifierShortcutDefault | s.extraMod}, ...)`. The
   default modifier maps to Ctrl on Linux/Windows and Command on macOS.
3. `c.SetOnTypedKey(...)` watches for `F1` to open the help dialog
   (`showShortcuts`).
4. `showShortcuts` builds the help dialog from the same `m.shortcuts` slice
   so the displayed bindings stay in sync with what was registered.

Because `registerShortcuts` short-circuits when `m.win == nil`, calling
`NewMainUI` followed by `Root()` without `SetWindow` (the unit-test path)
leaves `m.shortcuts == nil` and the canvas has no bindings.

## Docs tab: Edit → Preview rendering

`buildDocsTab` at [docs.go:14](docs.go#L14) creates a nested `AppTabs` with
Edit and Preview subtabs.

1. `docsEditor` is a monospace multi-line entry. Its `OnChanged` writes back
   into `currentRequest.Docs` (gated by `!m.loading`).
2. `docsPreview` is a `widget.RichTextFromMarkdown` with word wrap.
3. The outer `docsTabs.OnSelected` callback fires whenever the user switches
   subtabs. When the Preview subtab is selected, `refreshDocsPreview()` calls
   `docsPreview.ParseMarkdown(docsEditor.Text)` to re-render.
4. `refreshDocsPreview` is also called from `loadRequest` so opening a
   request pre-populates the preview without needing the user to switch
   tabs.

Re-parsing on every tab switch is intentional: live preview would flicker
through partial Markdown as the user types, and the per-tab re-parse is
cheap enough not to matter.

## Auth tab: type-driven form swap

`buildAuthTab` at [auth.go:53](auth.go#L53) builds a fixed set of widgets
once and toggles visibility based on the selected Type. The tab lives
between Params and Headers in `m.Request`.

1. The Type dropdown (`m.authType`) lists six labels — None, Inherit from
   parent, Basic Auth, Bearer Token, API Key, OAuth 2.0 — mapped to
   `model.AuthType` via `authTypeByLabel`.
2. Six form panels (`authNonePanel`, `authInheritPanel`,
   `authBasicPanel`, `authBearerPanel`, `authAPIKeyPanel`,
   `authOAuth2Panel`) are stacked inside `authFormsStack` (a Fyne
   stack container). `refreshAuthVisibility` hides every panel and then
   shows only the one matching the active Type.
3. Each editable widget on every panel has an `OnChanged` that calls one
   of the `ensure*` helpers (e.g. `ensureBasic()`) to lazily allocate the
   matching sub-struct on the current request and then writes the field.
   All callbacks honour the `m.loading` guard.
4. The Inherit panel surfaces what would actually be applied by walking
   the ancestor chain via `m.sess.EffectiveAuth(m.currentRequestID)` and
   formatting the resolved Type in `m.authInheritLabel`.
   `refreshAuthInheritLabel` runs both after the user changes the
   dropdown and at the end of `loadAuthTab`.
5. `loadAuthTab(req)` is called from `loadRequest` under the `m.loading`
   flag. It pushes the request's Auth fields into the matching widgets,
   sets the Type dropdown, clears all the unused sub-form widgets so
   stale data from a previous request never appears, and refreshes
   visibility + the Inherit label.

Switching Type does NOT clear the other sub-structs in memory — the user
can flip between Basic and Bearer without losing partial fills. They
only stop being persisted when `authToFile` drops everything except the
sub-struct matching the active Type. The next load will see a clean
file and rebuild the form accordingly.

The OAuth 2.0 panel includes a **Clear cached tokens** button. It calls
`m.sess.TokenCache().ClearAll()` and sets a status message. Use it when
rotating a client secret or recovering from a stale token without
restarting the app; the next Send re-fetches.
