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
   ([shell.go:67](shell.go#L67)) builds every widget (including the editor tab
   strip above the address bar), wires `OnChanged` callbacks, assembles the
   toolbar / address bar / sidebar / split layout, calls `refreshEnvironments`,
   and then `restoreTabs` — which reopens the persisted `OpenTabs` (resolving
   each by collection dir + `Request.ID`) and activates the previously active
   one, or falls back to the legacy single `OpenRequest` when no tab set is
   stored. See "Editor tabs" below.
8. `mainUI.SetWindow(w)` — ([shell.go:216](shell.go#L216)) records `m.win` and
   calls `registerShortcuts`. **Before this call `m.win` is nil and dialog
   actions short-circuit.** Keyboard shortcuts are not yet active either.
9. `w.SetContent(mainUI.Root())` — installs the root canvas object.
10. `a.Lifecycle().SetOnStopped(...)` registers a callback that persists the
    final window size into the session.
11. `w.ShowAndRun()` — Fyne main loop.

## Sidebar node actions

Tree rows ([treerow.go](treerow.go)) are pure display widgets (method chip +
name). They implement neither Tappable nor Hoverable, so the enclosing tree node
handles selection on a full-width tap and paints the full-width hover +
selection backgrounds itself — a clean, uncluttered highlight.

Node actions live on a toolbar above the tree (built in `NewMainUI`): icon
buttons for **add request, add folder, rename, clone, delete**, wired to
`actionNewRequest` / `actionNewFolder` / `actionRename` / `actionDuplicate` /
`actionDelete` ([items.go](items.go)) — the same entry points the keyboard
shortcuts use. Add request/folder target `parentForNew` (selected
folder/collection, a selected request's parent, or the active collection root);
rename/clone/delete operate on `lastSelectedNodeID`.

`refreshSidebarActions` reconciles the toolbar's enable state with the
selection: rename/delete need a selection, clone needs a selected *request*, and
add request/folder stay enabled. It is called from the tree's `OnSelected` and
after a deletion that clears the selection.

The tree is wrapped in a `container.ThemeOverride` (`sidebarTheme`,
[helena_theme.go](helena_theme.go)) that shrinks the inline-icon + padding sizes
so per-level indentation is shallower and icons denser — scoped to the sidebar.

## Drag-and-drop reordering

Tree rows are `fyne.Draggable` (drag source) but not Tappable, so a click still
selects via the tree node while a drag reorders. The flow ([treedrag.go](treedrag.go)):

1. Each bound row registers itself in `m.treeRows[row] = id` (the `buildTree`
   update callback). This is the hit-test registry.
2. On `Dragged`, `dragTreeNode` records `dragSrcID` + the pointer's
   `AbsolutePosition`, then `computeDrop` resolves a `dropPlan`: `rowAt` finds
   the row under the pointer (only visible rows inside the tree viewport, so
   pooled off-screen rows don't false-match), and the top/bottom half of that
   row plus the source/target kinds decide the plan —
   - dragging a **collection** → `planCollectionDrop` (reorder among
     collections, `Session.MoveCollection`);
   - dragging a **folder/request** → `planNodeDrop`: drop INTO a folder/
     collection (bottom half of a branch), a sibling before/after a same-kind
     row, or append. Cross-collection drops and folder-into-self/descendant are
     rejected (plan invalid).
   `showDropIndicator` draws the insert line or the into-outline for the live plan.
3. On `DragEnd`, `dropTreeNode` re-resolves the plan at the last position and
   `applyDrop` calls `Session.MoveNode` / `MoveCollection`, then `Tree.Refresh`
   + `reconcileTabs` (node IDs shifted) and re-selects the moved node.

The row also implements `desktop.Cursorable`: while `dragActive` is set it
returns a grab (pointer) cursor. Fyne recomputes the cursor from the object
under the pointer on every mouse move — and that path isn't gated by the drag —
so the cursor flips to "grab" as soon as the drag crosses the move threshold and
back on release.

## Selecting a tree node and loading a request

Each row in the collections tree carries an ID. The handler is registered in
`buildTree` at [shell.go:221](shell.go#L221).

1. The tree fires `OnSelected(id)`.
2. `m.lastSelectedNodeID = id` (used later by `parentForNew`, rename, delete,
   duplicate).
3. If the ID is a **request** node, `m.openOrActivate(id)` opens (or focuses) its
   tab — see "Editor tabs" below — and the handler returns. Activation handles
   making the owning collection active + refreshing environments.
4. Otherwise (a folder / collection row), if the ID resolves to a collection
   (`sess.Tree().CollectionIndex(id) >= 0`), `sess.SetActiveCollection(...)`
   updates the active collection and `refreshEnvironments` reseeds the
   Environment dropdown.

`loadRequest` is the shared editor-binding step that tab activation calls (it is
no longer invoked directly from the tree handler):

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

   The status line (`"Loaded: <name>"`), the cached-response restore, and tab
   persistence are done by `activateTab` around this `loadRequest` call — not by
   `loadRequest` itself.

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
write the loaded URL back into `currentRequest` (harmless), but inside more
complex programmatic updates (KV rows, body type changes that reset content)
the write-back loop can clobber freshly loaded data with stale values. The flag
makes `loadRequest` a single atomic UI write.

The request **body editor** (`BodyContent`, an editable `go-fyne-pretty-view`
widget) is loaded with `SetData`, which deliberately does **not** fire its
`OnChanged` — so the debounced write-back never races a load. Its `OnChanged`
*is* debounced for live edits, so the four consumers that need the body bytes
synchronously (`saveRequest`, `send`, `validateBody`, `formatBody`) call
`syncBodyFromEditor`, which pulls the editor's live `Source()` into
`currentRequest.Body.Content` before reading it. `loadRequest` chooses the
editor's syntax format via `formatForBodyType`, and a `BodyType` change
`Reparse`s the buffer at the new format.

### Body Type → Content-Type sync

The body **Type** select (`BodyType`) keeps the request's `Content-Type` header
in step with the chosen encoding, but only while the user hasn't set that header
explicitly. On a user-driven change (the `!m.loading` guard again — the bulk
`SetSelected` in `loadRequest` is skipped), the callback records the previous
type, applies the new one, then calls the pure helper
`applyImpliedContentType(headers, oldType, newType)`:

- No `Content-Type` header → one is added when the new type implies a value
  (json/xml/text/form). `none`/`multipart` imply none — multipart's header,
  with its boundary, is written when the request is sent.
- A `Content-Type` header exists → it is updated (or removed, for none/multipart)
  **only** when its current value still equals the *previous* type's implied
  value, i.e. it was auto-managed. A custom value (anything else) is left alone.

When the helper reports a change the new slice is stored and `rebuildHeadersRows`
re-renders the Headers tab so the edit is visible there.

### Query ↔ URL two-way sync

The **Query** tab and the URL field are two views of one thing. `currentRequest`
holds the bare base in `URL` and the structured pairs (including disabled ones)
in `Params`; `Params` stays the source of truth the send path merges, so the
backend is untouched. The field just shows base + query. Pure helpers live in
[query.go](query.go).

- **Editing the URL field** → `OnChanged` (guarded by `!m.loading && !m.syncing`)
  calls `applyURLEdit`: split off the query, store the base in
  `currentRequest.URL`, reparse the query into `Params` via `mergeQueryFromURL`
  (which keeps disabled rows the URL can't express). It then tries
  `syncParamsRowsInPlace` — when the param count is unchanged (the common case
  while typing) the existing `kvRow` widgets are updated in place (under
  `m.syncing`) instead of being torn down, avoiding a full `RemoveAll`+rebuild
  per keystroke (#53); only a genuine add/remove falls back to
  `rebuildParamsRows`. The field text is left alone so typing isn't disturbed.
- **Editing a Query row** (key/value/enable/delete) → the row's `onChange`
  (`buildKVRow`'s new last arg; nil for the Headers tab) calls
  `syncURLFieldFromParams`, which rewrites the field to `displayURL(base, params)`.
  It sets `m.syncing` around the `SetText` so the field's own `OnChanged` doesn't
  reparse the value straight back.
- **On load**, `loadRequest` folds any query already in the stored URL into
  `Params`, keeps `URL` as the bare base, and sets the field to the merged
  display. (Row callbacks are wired after `SetText`/`SetChecked`, so rebuilds
  don't fire sync.)

## Editor tabs

The editor keeps many requests open at once as a tab strip above the address
bar (an HScroll of `requestTab` widgets + a trailing `+`). All tabs share the
**one** editor + response panel; switching a tab re-points the binding via
`loadRequest`, exactly like the old single-request flow. The logic lives in
[tabs.go](tabs.go); the widget in [tabstrip.go](tabstrip.go).

**Identity vs. node ID — the load-bearing rule.** A tab's identity is the
target's persistent `Request.ID`. A tree node ID (`0/f1/r0`) is *not* identity:
it shifts when siblings are inserted/deleted. So `activateTab` and
`reconcileTabs` always **re-derive** the live node ID from the `Request.ID` via
`session.LocateRequest(dir, requestID)`, and `m.currentRequestID` is kept as
that node ID (or `""` for a scratch tab) because `EffectiveAuth`,
`refreshAuthInheritLabel`, and the delete guard all parse it as a path.

- **Open / activate.** `openOrActivate(nodeID)`: read the request's ID; if a tab
  already has it, `activateTab` it; else append a new tab and activate. Activate
  re-derives the node ID, makes the owning collection active + refreshes
  environments, runs `loadRequest`, restores the tab's cached response via
  `applyResponse(tab.resp)`, rebuilds the strip, and `persistTabs`.
- **Reorder + overflow.** `requestTab` is `fyne.Draggable`; dragging live-reorders
  the strip. `rebuildTabBar` pools one widget per `*openTab` (`tabWidgets`) and
  only re-arranges the container's objects, so the dragged widget instance — and
  therefore the gesture — survives each reorder. `dragTab` reads the other tabs'
  on-screen centers (`otherTabCenters`), maps the pointer X to an insertion index
  (`dropIndex`), and `applyDragTarget` re-inserts the tab (`moveTabModel`) while
  keeping the active tab active; `dragEnd` persists the new order. Fyne
  distinguishes tap from drag, so dragging never also selects. The `⋮`
  overflow button right of the strip opens a menu of every open tab
  (`tabMenuItems`, the active one checked) for jumping to one scrolled out of
  view.
- **Per-tab response.** Each tab caches a `tabResponse` (raw body, header dump,
  status line, CORS text, console, error flag). `send` captures the initiating
  tab; the worker builds the `tabResponse` off-thread and the `fyne.Do` block
  calls `deliverResponse(initTab, resp)`, which **always** stores it on `initTab`
  and repaints the shared panel **only if that tab is still active** (the user
  may switch tabs mid-Send). `applyResponse` feeds the body to the PrettyView
  (`pv.SetData`, auto-detecting format) and is also the restore path on tab
  switch. Only one Send runs at a time (`sendCancel`), so there are no
  concurrent per-tab sends.
- **Reconcile.** Every tree mutation (add / rename / duplicate / delete, in
  [items.go](items.go); also `RemoveCollection`) is followed by `reconcileTabs`:
  re-derive each tree-backed tab's node ID by `Request.ID`, drop tabs whose
  request vanished, re-point the active tab's `currentRequest` / `currentRequestID`
  in place (so in-flight edits survive a slice relocation), and rebuild the strip.
- **Scratch `+` and Save As.** `newScratchTab` opens a blank request the tab
  *owns* (`scratchReq`, `currentRequestID == ""`). Saving a scratch tab routes to
  `saveScratchTabAs` → a name + destination-container dialog (`ContainerPaths`) →
  `commitScratchTab`: make the chosen collection active, `AddRequestValue` (mints
  a fresh ID, one save), then convert the tab to tree-backed and re-activate.
  Closing a scratch tab with content confirms first (its edits would be lost);
  closing a tree-backed tab is silent (edits live in the tree until the app
  exits, as before).
- **Persistence.** `persistTabs` writes the tree-backed tabs (by collection dir +
  `Request.ID`) and the active index in one `SetOpenTabs` call; scratch tabs are
  never persisted. `restoreTabs` reopens them on launch.
- **Workspace switch.** `onWorkspaceChanged` reloads the collections — every
  cached node ID / pointer is invalidated — so it `closeAllTabs` + clears the
  editor (also fixing a pre-existing stale-`currentRequest` window across
  `reload`). v1 tabs are global; switching workspaces closes the strip and per-
  workspace tab memory is not kept.

## Sending a request (UI → httpclient → fyne.Do)

`send` lives in [send.go](send.go). The Send button is bound
to `m.sendOrAbort` (which dispatches between starting a new Send and
cancelling an in-flight one); the URL entry's `OnSubmitted` and the
Enter keyboard shortcut also route through `sendOrAbort` so they
share the same dispatch.

1. Trim-check the URL; bail early if empty.
2. Snapshot the request: a value copy of `*m.currentRequest`, or a fresh
   `model.Request{Method, URL}` if nothing is loaded.
3. **Flatten Inherit auth on the copy.** When a request is loaded, overwrite
   `req.Auth` with `m.sess.EffectiveAuth(m.currentRequestID)` so the
   downstream `httpclient.Build` → `auth.Apply` chain sees the concrete
   auth (Basic / Bearer / API-Key / etc.) instead of the literal `Inherit`
   sentinel. Done on a *copy*, so the in-memory request the user is editing
   keeps its `Inherit` value.
4. Build a per-send `httpclient.NewWithTransport(m.sess.Settings(), m.sessionTransport())` — the throwaway Client carries per-send state (cross-host strip, OAuth2 resolver) but reuses the cached `m.httpTransport` connection pool so repeated sends to one host skip TCP+TLS re-handshakes (#52); the transport is rebuilt only when `InsecureSkipVerify` changes. Install an
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
   `scripting.New(sessionEnvBridge{s: m.sess, base: envSnap})`. The
   bridge's `Get` reads through the captured env snapshot + live
   overlay; `Set` calls `Session.SetEnvOverlay`. Wrap it in a
   `chainExecutor{rt, client, envSnap, sess}` and a
   `sessionRequestFinder{m.sess}` — these are the two adapters the
   chain runner needs.
6. Status -> "Sending…", Send button text swapped to "Abort"
   (warning importance), CORS banner hidden — all on the UI
   goroutine. The `context.WithCancel` is built here and the
   resulting `CancelFunc` stashed on `m.sendCancel`; the button
   click handler (`sendOrAbort`) reads that field and calls
   `cancel()` to abort an in-flight Send.
7. **Spawn a goroutine.** Inside, `defer cancel()` releases the
   context resources whatever the exit path. The chain runner
   executes any declared before-hooks first:
   - Call `chain.Resolve(ctx, req, finder, exec, progress)`. The
     runner walks `req.Chain` in order, recursively resolves each
     predecessor's own `Chain`, executes them via
     `exec.ExecuteOnce` (which is the same single execution path
     used for the leaf — see step 8), and accumulates an alias→View
     map plus the console output of every step. Returns the leaf's
     chainMap (the aliases the leaf's own scripts can read), the
     accumulated console, and the first error. See
     [internal/chain/WORKFLOW.md](../chain/WORKFLOW.md).
   - On error, surface `"Chain error: …"` (or `"Aborted"` when
     `ctx.Err() == context.Canceled`) in the status + Raw panel,
     reset Send via `resetSendButton`, and return. **The leaf is
     never executed when the chain fails.**
8. Run the leaf via the SAME `exec.ExecuteOnce(ctx, req, chainMap)`.
   Inside one ExecuteOnce call:
   - Deep-copy Headers / Params / Body.Form so the worker can't race
     UI-thread edits to the live currentRequest slices.
   - Run the pre-script with the supplied chainMap bound as
     `chain.<alias>`. Pre-script failure returns a fatal error (no
     HTTP).
   - Build the resolver from `envSnap` + a fresh
     `Session.SnapshotEnvOverlay()` so any `helena.env.set` from
     chain steps or this pre-script is visible to `httpclient.Build`,
     and attach `.WithFallback(chain.VarLookup(chainMap))` so this
     request's `{{chain.<alias>.response.json.token}}`-style templates
     (URL, params, headers, body, auth) resolve against its chain
     results.
   - Call `client.Do(ctx, req, resolver)`. HTTP failure returns a
     fatal error.
   - Build the chain.View from the successful response (carries
     Size, Duration, CORSWarning for the leaf-display path).
   - Run the post-script with the chainMap. Post-script failure
     returns a *non-fatal* error — the View is fully populated, so
     the UI can still render the response.
9. Inside the goroutine, marshal results back via `fyne.Do(func() { ... })`.
   `fyne.Do` is the only safe way to touch widgets from a non-UI
   goroutine; it schedules the callback onto Fyne's event loop.
10. The worker builds a `tabResponse` **off the UI goroutine** (pure
    formatting — no widget access):
    - On `view.Response.StatusCode == 0` (pre-script or HTTP failure,
      or abort): an error `tabResponse` carrying the error text + a
      `"Error: …"` / `"Aborted"` status.
    - Otherwise a success `tabResponse` with the raw body, the
      `FormatHeaders` dump, a `"<Status> · <size> · <duration>"`
      status (suffixed `· sent <METHOD> <URL>` if the pre-script
      mutated method/URL, and the post-script error if any), the CORS
      banner text, and the console lines.
    The `fyne.Do` body then just calls `resetSendButton` and
    `deliverResponse(initTab, resp)`. `deliverResponse` stores `resp`
    on the tab that started the Send and, only if that tab is still
    active, calls `applyResponse` — which selects the Body tab, feeds
    the body to the PrettyView (`m.pv.SetData([]byte(rawBody),
    prettyview.FormatAuto)` on success, `FormatRaw` for an error
    string so it can't be mis-rendered as structured), pushes the
    headers into `headersText`, sets the status + console, and
    shows/hides the `corsBanner`. PrettyView auto-detects JSON / XML /
    HTML (raw fallback) and renders it virtualized with fold +
    highlighting; there is no separate Structured/Raw tab any more.
    The chain-error path (step 7) builds and delivers its error
    `tabResponse` the same way. (Per-tab restore on a tab switch goes
    through the same `applyResponse(tab.resp)`.)

### Error-surfacing contract (#51)

There are two surfaces, with different lifetimes:

- **`m.Status`** (the footer label) is **transient**: it carries
  informational progress ("Sending…", "Chain step 2/3", "Settings saved")
  and is freely overwritten by the next update. It must not be the *only*
  place a failure is shown.
- **The error banner** (`errorBanner`, above the response tabs) and the
  response **Body pane** are **non-transient**: on an `isError` response
  `applyResponse` raises the dismissible banner with the failure text and
  renders the error in the Body pane (raw). The banner stays until the user
  dismisses it, the next send starts (`send` hides it), or a subsequent
  successful response clears it (`applyResponse` / `clearResponsePanel`). It is
  visible whichever response sub-tab is active, so a failure can't be missed by
  switching tabs or by a later status update.

The Send button is the lifecycle marker AND the abort affordance:

- **Default state** — text "Send", high importance, `m.sendCancel == nil`.
  Tap routes through `sendOrAbort` → `send()` which launches the
  goroutine.
- **In-flight state** — text "Abort", warning importance,
  `m.sendCancel` holds the active context's cancel func. Tap routes
  through `sendOrAbort` → `m.sendCancel()` which propagates
  cancellation to `httpclient.Client.Do` (via `http.NewRequestWithContext`),
  to `scripting.runWithTimeout` (which calls `vm.Interrupt` on
  ctx-cancel), and out through chain.Resolve's `ctx.Err()` checks
  in the UI's return paths.
- **Teardown** — every fyne.Do return path (success, error, panic,
  abort) calls `resetSendButton` to clear `m.sendCancel` and restore
  default appearance. `cancel()` is also `defer`-called inside the
  goroutine to release context resources regardless of the exit
  path. The recover handler additionally routes a synthetic
  `panicResponse(r)` through `deliverResponse(initTab, …)` (#110), so a
  recovered panic replaces the originating tab's stale cached response
  with the error instead of leaving the prior one in place.

The Enter shortcut and `URL.OnSubmitted` reach `m.send` directly
(not `sendOrAbort`), but `send()` guards on `m.sendCancel != nil`
so keyboard repeats during an in-flight Send no-op rather than
leaking the in-flight cancel func by overwriting the field. To
abort via keyboard the user clicks Abort or waits for the Send to
complete.

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
3. `updateWindowTitle()` retitles the window to include the active workspace
   (e.g. "Helena — Default", #124). It is also called from `SetWindow` (initial
   title) and `refreshWorkspaceDropdown` (so rename/add/delete keep it current).

The Workspaces… button opens `editWorkspaces`
([workspaces.go:14](workspaces.go#L14)), a list-style dialog with icon buttons
for Add / Rename / Delete (#122) on top of the list and right-aligned; Rename
and Delete stay disabled until a workspace row is selected (#130). After
mutation, `refreshWorkspaceDropdown` reseeds the toolbar Select (and refreshes
the title). Delete also calls `loadRequest(nil, "")` to clear the editor (the
previously open request may have been in the deleted workspace).

The workspace and environment pickers are introduced by `cubes` and
`folder-tree` icon indicators (replacing the old "Workspace:" / "Environment:"
text labels); the Variables (`table-list`) and Manage-environments (`gears`)
buttons are icon buttons. The whole top bar is wrapped in `toolbarTheme` +
`NewPadded` so it gets the same control spacing, margin, and 24px icon size as
the sidebar action toolbar.

## Switching environments

User picks an environment in the toolbar Select:

1. `onEnvChanged(name)` -> `sess.SetActiveEnv("")` for `noEnv`, otherwise
   `sess.SetActiveEnv(name)`.
2. `updateURLPreview()` re-runs the resolver against the new env so the
   italic preview label reflects the change instantly.

The Variables… button opens `editEnvironments` in
[envedit.go](envedit.go):

1. Bails if no collection is open.
2. If the active env name is empty, takes the first env or creates a
   "Default" one.
3. Shows an editable key/value list (like the Headers tab, #123): one
   `buildVarRow` per variable — an enable checkbox, key + value entries, and a
   delete button — plus an **Add variable** button. The editor keeps the real
   values in a working copy and masks only the *display* of a `Secret` value:
   its value entry shows `envSecretMask` and is **disabled** until a **Reveal
   secret values** checkbox is toggled, so editing other rows can never clobber
   a hidden secret (#43). An unchecked row is kept but marked `Enabled=false`
   (this replaces the old `# key = value` disable syntax).
4. On Save: `pruneEmptyVars` drops blank-key rows ->
   `SetActiveEnvironmentVariables` -> `SaveActiveCollection` ->
   `refreshEnvironments` + `updateURLPreview`. Because the working copy holds
   the real values throughout, an un-revealed secret round-trips untouched with
   no parse/restore step.

   Note on what `Secret` guarantees today: it controls **display masking in the
   editor** and round-trips through storage; it is **not** encryption at rest —
   values are still stored as plaintext YAML (tracked separately).

The Manage-environments (`gears`) button opens `manageEnvironments`
([environments.go](environments.go)) — a list of the collection's environments
with icon buttons for **New / Rename / Delete / Set active** on top of the list
and right-aligned; Rename/Delete/Set-active stay disabled until a row is
selected. Each wires to the session's `AddEnvironment` / `RenameEnvironment` /
`DeleteEnvironment` / `SetActiveEnv` (which persist to the collection YAML),
then refreshes the toolbar dropdown + URL preview. The Variables (`table-list`)
button edits the active environment's key/value pairs; Manage is the
multi-environment lifecycle surface.

## Changing theme

The Settings (cog) toolbar button opens `editSettings` ([settings.go](settings.go)). The Theme
row is a Select pre-populated via `themeName(s.Theme)`. On Save:

1. `themeFromName(themeSelect.Selected)` maps the label back to a
   `model.Theme`.
2. `sess.SetSettings(...)` persists the new value.
3. `ApplyTheme(fyne.CurrentApp(), newTheme)` installs `helenaTheme`
   (see [helena_theme.go](helena_theme.go)) pinned to the chosen variant — the
   change takes effect without restart. The custom theme embeds the stock
   theme, so any name it doesn't override behaves as before.
4. `m.pv.SetTheme(variantFor(newTheme), …)` rebuilds the response PrettyView at
   the new variant — `SetTheme` swaps the theme object but does **not** flip the
   app's light/dark variant, so the body would otherwise keep its old colours.
5. `m.refreshThemedCanvas()` re-tints the raw `canvas.Text` / `canvas.Rectangle`
   objects whose colours were set imperatively (the CORS banner; the active-tab
   highlight via `rebuildTabBar`) and refreshes the tree. Theme-driven widgets
   repaint themselves on `SetTheme`; bare canvas objects do not. Method chips
   use fixed brand colours, so they need no re-tint.

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
`m.sess.TokenCache().ClearNamespace(m.sess.ActiveCollectionDir())` and sets
a status message — scoped to the active collection so other collections'
cached tokens survive. Use it when rotating a client secret or recovering
from a stale token without restarting the app; the next Send re-fetches.
