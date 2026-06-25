# internal/ui — structure

## Files

| File | Purpose |
| ---- | ------- |
| [doc.go](doc.go) | Package doc comment. |
| [shell.go](shell.go) | The `MainUI` struct, `NewMainUI` (the main layout), and the core glue that didn't belong to a focused file: `Root`/`SetWindow`, `buildTree`/`applyTreeFilter` (the cross-collection sidebar search #67)/`refreshSidebarActions`, `loadRequest`/`saveRequest`/`updateURLPreview`, `onWorkspaceChanged`, `openCollection`, `windowTitleFor`/`updateWindowTitle` (the title reflects the active workspace, e.g. "Helena — Default", #124), and `loadErrorReport`/`SurfaceLoadErrors` (the non-transient diagnostic for collections the session failed to load, shown via `SetOnStarted` in `cmd/helena`, #108). The top bar is a Border wrapped in `toolbarTheme` + `NewPadded` so it gets the same inter-control spacing, margin, and 24px icon size as the sidebar action toolbar (the root theme zeroes padding otherwise): leading workspace/environment controls — `cubes` / `folder-tree` icon indicators (`ttwidget.Icon`, centred) replace the old text labels, with the `Variables` (`table-list`) and `Manage environments` (`gears`) pickers as icon buttons and a vertical separator (with margin) between the two groups — and trailing Cookies (`cookie-bite`, the session cookie-jar viewer #91) + Settings (`theme.SettingsIcon`) + Help (`theme.HelpIcon`) icon buttons (#125-#129). Decomposed from a single 1800+ LOC file (#55); the send path, request-editor rows, env editor, settings, execution glue, and small helpers now live in the files below. |
| [send.go](send.go) | The Send lifecycle: `send` (off-UI goroutine → chain.Resolve → ExecuteOnce → `fyne.Do` deliver), `sendOrAbort`/`resetSendButton`/`setAbortButton`, `snapshotRequest`, `sessionTransport` (the cached per-session connection pool, #52), `panicResponse` (#110), `guard` (panic-recovery for callbacks, #48/#49), the `errorBanner` (#51) / `emptyState` (#58) toggles, and the `{{?Name}}` prompt-variable flow (#86): `requestTemplateStrings` gathers the request's template fields, `promptForVars` collects values in a Send-time dialog and re-enters `send` with them stashed in `promptSnap`. |
| [saveresponse.go](saveresponse.go) | "Save response to file" (#66): `currentResponseBytes` reads the active tab's raw body bytes (binary-safe), `writeResponseTo` writes them byte-for-byte (testable), and `saveResponseToFile` wires the `dialog.NewFileSave` flow behind the response-toolbar download button. |
| [editor.go](editor.go) | The request-editor row machinery: body validate/format (`formatForBodyType`, `syncBodyFromEditor`, `validateBody`, `formatBody`), the form-body rows, the Params/Headers rows (`rebuildParamsRows`, `syncParamsRowsInPlace` for in-place URL-edit updates #53, `rebuildHeadersRows`), `applyURLEdit`/`syncURLFieldFromParams`, `applyImpliedContentType`, and `kvRow`/`buildKVRow`. The body uses an **editable** `go-fyne-pretty-view` widget (`BodyContent`). `refreshBodyEditorVisibility` swaps the text editor / form panel / file panel by body type. |
| [bodyfile.go](bodyfile.go) | The BodyFile editor (#24): `buildBodyFilePanel` (chosen-file label + Choose/Clear + Content-Type entry), `chooseBodyFile` (file-open dialog → `Body.FilePath`), `clearBodyFile`, and `loadBodyFilePanel` (populates the widgets from a request body under `m.loading`). |
| [execution.go](execution.go) | `sessionEnvBridge` (the `scripting.EnvBridge` adapter), `nilFinder` (the no-collection `chain.RequestFinder`), and `chainExecutor` — the single execution path that runs pre-script → `client.Do` → post-script for both chain steps and the leaf — plus `chainViewToScripting`. |
| [envedit.go](envedit.go) | The active-environment editor: `editEnvironments` — an editable key/value list like the Headers tab (#123): one row per variable (enable check + key + value entries + delete), plus a right-aligned icon "Add variable" button (sharing its row with the reveal toggle when present); `onEnvChanged`, `refreshEnvironments`, and the `buildVarRow`/`varRowValue`/`pruneEmptyVars`/`envSecretMask` helpers. The editor holds the real values in a working copy and masks only the *display* of a `Secret` value (shown as `envSecretMask` and disabled until "Reveal secret values" is toggled), so editing other rows can't clobber a hidden secret (#43); an unchecked row is kept but marked `Enabled=false` (replacing the old `# key = value` disable syntax). The editor body is the shared `showVariablesEditor` (used by the collection-variables editor too, #80). (The create/rename/delete *manager* is in [environments.go](environments.go).) |
| [collectionvars.go](collectionvars.go) | `editCollectionVariables` — opens `showVariablesEditor` on the ACTIVE collection's `Variables` (#80, a resolver scope below the environment) and saves them back via `SaveActiveCollection`. Reached from the sidebar toolbar's `sliders` button. |
| [requestvars.go](requestvars.go) | `buildVarsTab` / `editRequestVariables` — the request editor's "Vars" tab (#82); opens `showVariablesEditor` on the loaded request's `Variables` (highest static scope) and writes back to `currentRequest` (persisted on the next Save). `enabledRequestVars` builds the resolver scope used by the Send path. |
| [globalvars.go](globalvars.go) | `editGlobalVariables` — opens `showVariablesEditor` on the app-wide global variables (#83, the lowest scope) and persists them via `SetGlobalVariables`. Reached from the "Global variables…" button in the Settings dialog. |
| [cookies.go](cookies.go) | The cookie-jar viewer/editor (#91): `showCookies` lists `session.CookieJar().All()` with Add/Edit/Delete (selection-gated, mirroring the workspaces dialog) and Clear-all; `editCookie` is the per-cookie form (Domain/Path/Name/Value + Secure/HttpOnly/"Send to subdomains" checks, with Expires carried through read-only); `cookieSummary` renders a row incl. host-only-vs-+subdomains scope. Add → `jar.Set` (defaults host-only), Edit → `jar.Replace` (no orphan / no reorder), Delete → `jar.Remove`, Clear → confirm + `jar.Clear`. Edits hit the live session jar directly. Reached from the top-bar `cookie-bite` button. |
| [settings.go](settings.go) | `editSettings` (the Theme / TLS / CORS / redirects / timeout / max-response dialog, plus a "Global variables…" button #83), `refreshThemedCanvas`, and the `sanitizeTimeoutSeconds` / `sanitizeMaxResponseMiB` field validators. |
| [helpers.go](helpers.go) | Small UI utilities: `enableButton`, `tipButton` (named bundled SVG) / `tipButtonRes` (a pre-built icon resource, e.g. a Fyne `theme.*Icon()`), `thinHSplit`/`thinVSplit`, `methodNames`, `bodyTypeNames`, `pruneEmptyKV`, `pruneEmptyChain`. |
| [query.go](query.go) | Pure helpers for the two-way **Query**↔URL sync: `splitURLQuery`, `parseQueryParams`, `buildQueryString`, `displayURL`, `mergeQueryFromURL` (keeps disabled rows), and `encodeQueryComponent` (percent-encodes but leaves `{{vars}}` intact). The UI glue (`applyURLEdit`, `syncURLFieldFromParams`, `kvRow`, `syncParamsRowsInPlace`) lives in [editor.go](editor.go). `currentRequest.Params` stays the send-path source of truth, so the backend is unchanged. |
| [items.go](items.go) | Tree CRUD actions (new request, new folder, rename, duplicate, delete) plus `actionUndoDelete` (restores the last deleted folder/request via `Session.RestoreLastDeleted`, #68) and `parentForNew`, `promptName`, `nameOfNode`, `isAncestor` helpers. |
| [workspaces.go](workspaces.go) | `editWorkspaces` dialog — icon buttons for Add/Rename/Delete (#122) on top of the list and right-aligned; Rename and Delete stay disabled until a workspace row is selected (#130). `refreshWorkspaceDropdown` reseeds the toolbar Select and also refreshes the window title. |
| [environments.go](environments.go) | `manageEnvironments` — the create/rename/delete/set-active manager for a collection's named environments (wires to the session env CRUD). Action buttons are icons (`square-plus` / `pen-to-square` / `trash-can` / `theme.ConfirmIcon`) on top of the list and right-aligned; Rename/Delete/Set-active stay disabled until a row is selected (like the Workspaces dialog). "Variables…" edits the active env's pairs. |
| [collections.go](collections.go) | `actionNewCollection` — prompt + folder picker + empty YAML write. |
| [import.go](import.go) | `actionImport` chooser plus URL / file / **Paste cURL** paths (`importFromCurl` parses the pasted command via `importer.FromCurl` and opens it in a scratch tab through `openScratchWith`, #71) and `slugify` / `uniqueCollectionDir` helpers. `loadSample`/`loadSampleFrom` materialize the embedded `examples` sample (via `sampleDestDir`) and open it for the first-run try-it path (#57). |
| [export.go](export.go) | `actionExport` — the snippet dialog with one tab per codegen target (cURL, wget, JavaScript fetch, Python, Go; #95), each with its own Copy button via the inline `mkTab` helper, plus `newSnippetEntry`. |
| [help.go](help.go) | The Help menu behind the Help (question-mark) toolbar icon button (#61, #128): `helpMenuItems` (getting-started, shortcuts, web links to the user guide + issues, About), `showHelpMenu`, `showGettingStarted`, `showAbout`, `openURL`, and `SetVersion` (feeds About). |
| [docs.go](docs.go) | `buildDocsTab` and `refreshDocsPreview` — per-request Markdown editor with rendered preview subtab. |
| [scripts.go](scripts.go) | `buildScriptsTab` — the Pre-request / Post-response code editors and the read-only Console panel below. `loadScriptsTab` populates the editors during `loadRequest`; `setScriptConsole` renders the captured console output after each Send. |
| [chain.go](chain.go) | `buildChainTab` — the list of (Alias, Request path) rows for declaring before-hooks. `loadChainTab` / `rebuildChainRows` / `addChainStep` / `buildChainRow` follow the same patterns as the Params and Headers tabs. |
| [auth.go](auth.go) | `buildAuthTab`, `loadAuthTab`, `refreshAuthVisibility`, `refreshAuthInheritLabel`, and the `ensureBasic`/`ensureBearer`/`ensureAPIKey`/`ensureOAuth2` lazy allocators for the per-type sub-structs. |
| [oauth2.go](oauth2.go) | `fyneAuthCodeStarter` — adapter that hands the authorization URL to `fyne.CurrentApp().OpenURL`. The `newAuthCodeStarter` package-level var lets tests swap in a fake. |
| [theme.go](theme.go) | `ApplyTheme` (installs the custom theme pinned to the variant for the setting) plus `themeName` / `themeFromName` and `variantFor` (Helena `model.Theme` → `fyne.ThemeVariant`, used to repaint the response PrettyView on a theme switch). |
| [helena_theme.go](helena_theme.go) | `helenaTheme` — Helena's custom `fyne.Theme`: a layered surface ramp, a single green brand accent (Primary/Focus/Selection/ForegroundOnPrimary), denser sizing, and the embedded Inter (UI) + JetBrains Mono (monospace) faces. Embeds `theme.DefaultTheme()` and delegates anything it doesn't override; `forced` pins the served variant for Light/Dark. `newHelenaTheme` builds it from a `model.Theme`. The scoped sub-themes all embed `delegatingTheme` — a stateless base that passes every `fyne.Theme` method through to the live app theme (`appTheme`) — and override only the few `Size`/`Color` cases that differ, so the pass-through boilerplate lives in one place (#56). `sidebarTheme` — a tree-scoped override (applied via `container.NewThemeOverride`) that only shrinks `SizeNameInlineIcon` + `SizeNamePadding` to make the collections tree's per-level indentation shallower and its icons denser. `toolbarTheme` is the same idea but enlarges `SizeNameInlineIcon` to 24 for the sidebar action toolbar. `splitTheme` thins a `container.Split` divider into a hairline (shrinks `SizeNamePadding` → 3px divider, recolours `ColorNameShadow` to the separator colour, hides the grab handle) and `paneTheme` (pure delegation — just the embedded base) restores normal sizing inside the split's panes; both are used by the `thinHSplit` / `thinVSplit` helpers in [shell.go](shell.go). `rootTheme` zeroes `SizeNamePadding` on the root Border so the body sits flush against the header/footer separator lines (the vertical split divider meets them with no gap); root-level children restore their padding via their own overrides. `themedIcon(name)` wraps an embedded SVG in a `theme.ThemedResource` so Fyne recolours it to the foreground (Fyne only recolours high-importance button icons, so plain `currentColor` resources would render black on the medium/low toolbar buttons). |
| [tabs.go](tabs.go) | Editor tab-strip state on `MainUI`: the `openTab` / `tabResponse` types and `openOrActivate` / `activateTab` / `closeTab` / `closeAllTabs` / `reconcileTabs` / `newScratchTab` / `saveScratchTabAs` / `commitScratchTab`, the per-tab response helpers (`deliverResponse` / `applyResponse` / `clearResponsePanel`), persistence (`persistTabs` / `restoreTabs`), the pool-based `rebuildTabBar`, drag-reorder (`dragTab` / `otherTabCenters` / `applyDragTarget` / `dragEnd` + the pure `moveTabModel` / `dropIndex`), and the overflow menu (`tabMenuItems` / `showTabMenu`). |
| [treerow.go](treerow.go) | `treeRow` — the per-row widget for the Collections tree (BaseWidget + SimpleRenderer): a brand-colored bold method chip (requests) + an ellipsis-truncating name (branches: just the label). It implements `fyne.Draggable` (drag source for reordering — see treedrag.go) and `desktop.Cursorable` (a grab/pointer cursor while a drag is in flight, via a `dragging func() bool` probe) but NOT Tappable, so the tree node still owns selection (full-width tap, distinct from a drag) and paints the full-width hover + selection backgrounds. Node actions live on the sidebar toolbar (shell.go). `setRequest` / `setBranch` update the widgets + captured node id. Layout is a small custom `treeRowLayout`: for a request it pulls the method chip LEFT into the disclosure-arrow column (by `iconSize + pad`, the gap the tree leaves between the arrow and the content origin) so the chip lines up vertically with same-depth folders' arrows, and flows the name after it; for a branch the chip is hidden and the name renders at the content origin (unchanged). The chip overhangs the content container's left edge — harmless, since the node's full-width background sits under it and the scroll clip is far to the left. |
| [treedrag.go](treedrag.go) | Drag-and-drop reordering of the tree. `dragTreeNode` / `dropTreeNode` are the row callbacks; `computeDrop` resolves a `dropPlan` from the pointer position (`rowAt` hit-tests the registered rows by absolute geometry), `planNodeDrop` / `planCollectionDrop` decide into-container vs sibling-before/after vs collection reorder, `applyDrop` calls `Session.MoveNode` / `MoveCollection` then refreshes tree + tabs, and `showDropIndicator` draws the insert line / into-outline overlays. Plus node-id string helpers (`isCollectionID`, `collectionIndexOf`, `nodeKind`, `splitNode`). |
| [tabstrip.go](tabstrip.go) | `requestTab` — the per-tab custom widget (BaseWidget + `Tapped` + `Draggable` + a custom `requestTabRenderer`): a natural-width colored method chip, the name, and a `circle-xmark` close button, all vertically centred (with left padding matching the close icon's right inset), over a rounded-top background. Active tabs fill with `ColorNameBackground` so they connect to the editor content and cover the under-strip separator line (Bruno-style); inactive tabs are transparent over the strip band. The band + line + leading/top spacing live in the strip `Stack` in [shell.go](shell.go) (`tabStripBg` + a bottom `widget.Separator`). |
| [shortcuts.go](shortcuts.go) | `shortcutSpec`, `registerShortcuts`, `showShortcuts`, `shortcutModifierName`, and `shortcutRowLayout`. |
| [shell_test.go](shell_test.go) | `NewMainUI` construction + headless layout smoke test. |
| [query_test.go](query_test.go) | Query↔URL helpers (split/parse/build/encode round-trips, `{{var}}` preservation, disabled-row preservation) and the live two-way sync + load-time fold. |
| [response_test.go](response_test.go) | Response viewer: `applyResponse` feeds the PrettyView (JSON auto-detects to structured, plain text / malformed / binary / error → raw, nil clears), Body tab selected, status set; `variantFor` theme mapping. |
| [tabs_test.go](tabs_test.go) | Tab open/activate dedup, per-tab response capture + restore (incl. inactive-tab delivery), scratch `+` + Save-As conversion, reconcile-after-delete drop+remap, close-tab neighbor/clear, launch restore, drag-reorder (`dropIndex` / `moveTabModel` / `applyDragTarget` keeps active + persists), and the overflow menu items. |
| [treerow_test.go](treerow_test.go) | `treeRow`: requests show a brand-colored method chip, branches hide it, the name uses ellipsis truncation, drag callbacks forward the row id, and the cursor switches to grab only while dragging. |
| [treedrag_test.go](treedrag_test.go) | Drop-plan logic: `planNodeDrop` (sibling before/after, into-folder, folder-before-folder, reject self/descendant + cross-collection), `planCollectionDrop` (reorder + reject self), and the node-id helpers. |
| [treedrag_e2e_test.go](treedrag_e2e_test.go) | End-to-end drag through a laid-out headless window: real row registry + absolute-position hit-testing + `MoveNode`, asserting a request reorders. |
| [sidebar_test.go](sidebar_test.go) | Sidebar node-action toolbar: buttons exist and are wired, and `refreshSidebarActions` enables/disables them by selection (add request/folder always on; rename/clone/delete need a selection, clone only a request). |
| [tabstrip_test.go](tabstrip_test.go) | `requestTab` widget: label/active toggling and tap → select / close callbacks. |
| [docs_test.go](docs_test.go) | Docs editor load + preview + write-back, plus clear-on-nil behaviour. |
| [scripts_test.go](scripts_test.go) | Scripts tab load + write-back, loading-flag suppression across request swaps, clear-on-nil for both editors and the console, console rendering (incl. truncation), and the `sessionEnvBridge` adapter. |
| [chain_test.go](chain_test.go) | Chain tab load + add/delete, loading-flag suppression across request swaps, and `pruneEmptyChain` save-time filter. |
| [auth_test.go](auth_test.go) | Auth tab load + write-back for Bearer / Basic / API-Key, type-change → Auth.Type, and m.loading suppression. |
| [shortcuts_test.go](shortcuts_test.go) | Shortcut registration, modifier label, dialog open, and nil-window short-circuit. |
| [theme_test.go](theme_test.go) | `themeName` / `themeFromName` round-trip and `ApplyTheme` panic-safety. |
| [helena_theme_test.go](helena_theme_test.go) | `helenaTheme`: every overridden colour present in both variants, the green accent pinned, unknown names delegate to base, forced-variant behaviour (Light/Dark ignore the passed variant, System follows it), `Font` style mapping (incl. symbol delegation), `Size` overrides + delegation, and `ApplyTheme` installing `helenaTheme`. |

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
| `Method` | `*methodPicker` | HTTP method picker on the address bar (custom widget — brand-colored bold text + pop-up chooser; see [method.go](method.go)). |
| `URL` | `*widget.Entry` | URL entry; Enter triggers `send`. |
| `urlPreview` | `*widget.Label` | Italic label under the URL showing the resolved form (or unresolved-vars warning). Hidden when nothing to show. |
| `Save` | `*ttwidget.Button` | Icon-only (`floppy-disk`) with a hover tooltip; disabled until a request is loaded. Export sits beside it as a local `file-export` `tipButton`. |
| `Send` | `*widget.Button` | Icon-only (`location-arrow`) high-importance by default / text "Abort" while a Send is in flight (warning importance). Tap routes through `sendOrAbort` which dispatches based on `sendCancel`. |
| `Tree` | `*widget.Tree` | Collections sidebar tree. Rows are clean display widgets; the tree node paints full-width hover/selection. |
| `sbAddReq` / `sbAddFolder` / `sbRename` / `sbClone` / `sbDelete` | `*ttwidget.Button` | Sidebar node-action toolbar (icon-only buttons with hover tooltips — `fyne-tooltip`, built via the `tipButton` helper) operating on the selected node. `refreshSidebarActions` enables/disables them by selection (clone for a folder/request; rename/delete need any selection; add request/folder fall back to the active collection). The window content is wrapped in `fynetooltip.AddWindowToolTipLayer` in `cmd/helena/main.go` for the tooltips to render. |
| `treeRows` | `map[*treeRow]string` | Live row → bound node id, rebuilt on each tree bind; used by `rowAt` to hit-test the drop target during a drag. |
| `treeSearch` | `*widget.Entry` | Sidebar cross-collection search box (#67), above the tree. Its `OnChanged` calls `applyTreeFilter`. |
| `treeFilter` | `map[string]bool` | Visible node IDs when a search is active (from `Tree.Search`); nil means show everything. The tree's `childUIDs` callback intersects `ChildIDs` with this set. |
| `dragActive` / `dragSrcID` / `dragLastAbs` | `bool` / `string` / `fyne.Position` | In-flight tree drag state (the dragged node and last pointer position), consumed on `DragEnd`. |
| `dropIndicator` / `dropInto` | `*canvas.Rectangle` | Drag overlays in a `WithoutLayout` layer over the tree: a thin primary line for insert-between, an outlined box for drop-into-container. Hidden at rest. |
| `Request` | `*container.AppTabs` | Request editor tabs, in order: Body, Auth, Headers, Query, Vars, Scripts, Chain, Docs. ("Query" = the query-string params, two-way synced with the URL field — see `query.go`; "Vars" = request-scoped variables, #82, see `requestvars.go`.) |
| `Response` | `*container.AppTabs` | Response tabs: **Body** (the `pv` PrettyView + its toolbar) and **Headers**. `applyResponse` selects Body on each new response. |
| `Status` | `*widget.Label` | Footer status line. |
| `paramsRows` | `*fyne.Container` | VBox of KV rows for the **Query** tab (query-string params), two-way synced with the URL field via `applyURLEdit` / `syncURLFieldFromParams` (guarded by `syncing`). |
| `headersRows` | `*fyne.Container` | VBox of KV rows for headers. |
| `BodyType` | `*widget.Select` | Body type select (none / json / xml / text / form / multipart). |
| `BodyContent` | `*prettyview.PrettyView` | Editable **raw** request-body widget (json/xml/text) — the same [go-fyne-pretty-view](https://github.com/ideaconnect/go-fyne-pretty-view) widget as `pv`, constructed `WithEditable()` + `WithLineNumbers()` so the user types/pastes with live syntax highlighting and a caret. Fed via `SetData(content, formatForBodyType(type))` in `loadRequest`; `Reparse`d when `BodyType` changes; reformatted in place by `formatBody`. Its `OnChanged` write-back is **debounced**, so `syncBodyFromEditor` pulls `Source()` synchronously at Save/Send/Validate/Format. **Hidden** (via `refreshBodyEditorVisibility`) for form-urlencoded/multipart, which use `bodyFormRows` instead. Repainted on theme change via `SetTheme(variantFor(...))`. |
| `bodyFormRows` / `bodyFormPanel` | `*fyne.Container` | Structured `Body.Form` KV editor (key/value/enabled rows reusing `buildKVRow`) shown in place of `BodyContent` for **form-urlencoded / multipart** bodies. `rebuildBodyFormRows` rebuilds from `currentRequest.Body.Form`; `addBodyFormField` appends a row; `refreshBodyEditorVisibility(type)` swaps among the text editor, form panel, and file panel (all live in a `container.Stack`). |
| `bodyFilePanel` / `bodyFilePathLabel` / `bodyFileContentType` | `*fyne.Container` / `*widget.Label` / `*widget.Entry` | The **file** body editor (#24, see [bodyfile.go](bodyfile.go)): chosen-file path + Choose/Clear buttons + a Content-Type entry, shown for `BodyFile`. The entry writes `currentRequest.Body.ContentType` (guarded by `m.loading`); the picker sets `Body.FilePath`. |
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
| `authOAuth2ClearTokens` | `*widget.Button` | "Clear cached tokens" button on the OAuth2 panel — calls `Session.TokenCache().ClearNamespace(ActiveCollectionDir())` so a rotated client secret forces the next Send to refetch, scoped to the active collection (other collections' tokens survive). |
| `pv` | `*prettyview.PrettyView` | The response **Body** viewer ([go-fyne-pretty-view](https://github.com/ideaconnect/go-fyne-pretty-view)): one virtualized widget rendering JSON/XML/HTML/raw with auto-detect, fold, syntax highlighting, search and soft-wrap. Fed via `pv.SetData` in `applyResponse`; subsumes the former Structured tree + Raw text viewer. Repainted on theme change via `pv.SetTheme(variantFor(...))`. |
| `headersText` | `*widget.Entry` | Response headers view. |
| `corsBanner` | `*canvas.Text` | Orange banner above the response panel surfacing CORS warnings. |
| `currentRequest` | `*model.Request` | Pointer to the request bound to the editor widgets — the active tab's request (a live tree pointer, or a scratch tab's owned value). Direct writes happen via `OnChanged` callbacks. |
| `currentRequestID` | `string` | Tree **node ID** for `currentRequest` (`""` for a scratch tab). Re-derived on every tab activation / `reconcileTabs`; consumed as a path by `EffectiveAuth`, `refreshAuthInheritLabel`, and the delete guard, so it must stay a node ID, never a `Request.ID`. |
| `lastSelectedNodeID` | `string` | Last node the user selected; the basis for `parentForNew`, rename, delete, and duplicate targets. |
| `tabs` | `[]*openTab` | Open editor tabs, in strip order. Each holds a `Request.ID` identity, owning collection dir (scratch: `""` + an owned `scratchReq`), a cached node ID (re-derived by `reconcileTabs`), and a cached `tabResponse`. See [tabs.go](tabs.go). |
| `activeTabIdx` | `int` | Index of the active tab in `tabs`, or `-1` when none is open. |
| `tabBar` | `*fyne.Container` | HBox (inside an HScroll) of `requestTab` widgets + the trailing `newTabBtn`; rebuilt by `rebuildTabBar`. |
| `tabWidgets` | `map[*openTab]*requestTab` | Pool of one `requestTab` per open tab. Reusing widgets across rebuilds keeps a drag gesture bound to its instance through a live reorder. |
| `newTabBtn` | `*widget.Button` | The `+` affordance that opens a blank scratch tab via `newScratchTab`. |
| `tabOverflowBtn` | `*widget.Button` | The `⋮` button right of the strip; opens the overflow menu (`showTabMenu`) listing every tab for quick jumps. Hidden when no tab is open. |
| `tabStripBg` | `*canvas.Rectangle` | Header-coloured band behind the tab strip; the bottom separator line sits over it and the active tab covers its segment. Re-tinted on theme switch by `refreshThemedCanvas`. |
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
   exported fields, then calls `restoreTabs` to reopen the previously open tabs
   (falling back to the legacy single open request).
4. The window is created, then `mainUI.SetWindow(w)` records the dialog parent
   and registers shortcuts against `w.Canvas()`.
5. `w.SetContent(mainUI.Root())` shows the UI.
