# internal/ui

Helena's Fyne-based view layer. This module owns the main window layout
(collections sidebar, request editor, response viewer), every user action
exposed in the toolbar and tree, the Settings/Workspaces/Environments dialogs,
the Markdown docs tab, the theme switch, and the keyboard shortcut table.

The module is intentionally GUI-only: it never touches disk, the network, or
collection parsing directly. It calls into `internal/session` for state and
persistence, `internal/httpclient` to actually send requests, `internal/exporter`
and `internal/importer` for cURL/wget output and OpenAPI/WSDL ingestion, and
`internal/responsefmt` for request-body pretty-print/validate and the status-line
size/duration/header formatting. Both the response **Body** viewer and the
**request body editor** are the external
[go-fyne-pretty-view](https://github.com/ideaconnect/go-fyne-pretty-view) `/v2`
widget (JSON/XML/HTML/raw, auto-detected, virtualized) — the request body is
constructed `WithEditable()` for live-highlighted, in-place editing. All of that lets shell.go
focus on widget wiring while the rest of the codebase stays headless and
testable.

`MainUI` is the central type. It is built by `NewMainUI(*session.Session)` and
then handed a `fyne.Window` via `SetWindow` (called late from `cmd/helena`).
Until `SetWindow` runs, `m.win` is nil and every dialog-opening action
short-circuits — see STRUCTURE.md.

## Public API

The exported surface is small; almost everything is methods on `*MainUI`.

- `MainUI` (struct) — primary widget container; see STRUCTURE.md.
- `NewMainUI(sess *session.Session) *MainUI` — builds the layout against a
  session and returns it ready to place.
- `(*MainUI).Root() fyne.CanvasObject` — the assembled root container.
- `(*MainUI).SetWindow(w fyne.Window)` — records the dialog parent and
  installs keyboard shortcuts.
- `ApplyTheme(app fyne.App, t model.Theme)` — switches Fyne theme to match a
  Helena `model.Theme`.

### Actions on MainUI

Grouped by category. All are methods on `*MainUI` and most are wired to a
toolbar button, sidebar button, or keyboard shortcut.

#### Tree CRUD (sidebar)

- `actionNewCollection` — prompt + folder picker, create empty collection
  ([collections.go](collections.go)).
- `actionNewRequest` / `actionNewFolder` — insert into the selected parent
  ([items.go](items.go)).
- `actionRename` / `actionDuplicate` / `actionDelete` — operate on the
  currently selected tree node ([items.go](items.go)).
- `openCollection` — folder picker that loads an existing on-disk collection
  ([shell.go](shell.go)).

#### Send / Save

- `send` — execute the active request off the UI goroutine, marshal results
  back via `fyne.Do` ([shell.go](shell.go)).
- `saveRequest` — persist edits through `sess.SaveActiveCollection()`
  ([shell.go](shell.go)).
- `validateBody` / `formatBody` — JSON/XML body helpers in the Body tab
  ([shell.go](shell.go)).
- `addParam` / `addHeader` — append a blank KV row to the active request
  ([shell.go](shell.go)).
- `loadRequest` — populate widgets from a `*model.Request` with the loading
  flag set ([shell.go](shell.go)).

#### Settings (workspaces, environments, app-level)

- `editWorkspaces` — list-style workspace manager
  ([workspaces.go](workspaces.go)).
- `editEnvironments` — key/value list editor for the active environment
  ([envedit.go](envedit.go)).
- `editSettings` — theme / TLS / CORS / redirects / timeout dialog
  ([shell.go](shell.go)).
- `refreshEnvironments` / `refreshWorkspaceDropdown` — reseed toolbar selects
  from the session.

#### Import / Export

- `actionImport` — chooser for URL or local file
  ([import.go](import.go)).
- `importFromFile` / `importFromURL` / `chooseImportDestination` — concrete
  import paths.
- `actionExport` — cURL/wget tabbed dialog with Copy buttons
  ([export.go](export.go)).

#### Theme / Shortcuts / Docs

- `ApplyTheme` (exported), `themeName`, `themeFromName`
  ([theme.go](theme.go)).
- `registerShortcuts` / `showShortcuts` / `shortcutSpec`
  ([shortcuts.go](shortcuts.go)); `shortcutEntry`
  ([shortcutentry.go](shortcutentry.go)) and `PrettyView.SetHostShortcuts`
  keep the same bindings firing while a text entry or the body/response
  editor has focus, which a canvas-only registration can't do (see
  WORKFLOW.md).
- `buildDocsTab` / `refreshDocsPreview` — Markdown editor + preview subtabs
  ([docs.go](docs.go)).

## Dependencies

- `fyne.io/fyne/v2` (and `container`, `canvas`, `dialog`, `widget`, `theme`,
  `driver/desktop`, `storage`, `test`) — widget toolkit.
- `github.com/ideaconnect/go-fyne-pretty-view/v2` — the response Body viewer and the editable request-body widget
  (virtualized JSON/XML/HTML/raw with fold, syntax highlighting, search,
  soft-wrap). Pinned at `v2.3.0-alpha`; same author / org as Helena.
- `github.com/idct/helena/internal/session` — workspace/collection/env state
  and persistence.
- `github.com/idct/helena/internal/model` — request/response types, Theme,
  Settings, KeyValue.
- `github.com/idct/helena/internal/httpclient` — actually sending requests.
- `github.com/idct/helena/internal/importer` — OpenAPI / Swagger / WSDL parse.
- `github.com/idct/helena/internal/exporter` — cURL/wget rendering.
- `github.com/idct/helena/internal/responsefmt` — request-body JSON/XML
  pretty-print + size / duration / header formatting.
- `github.com/idct/helena/internal/storage` — used from collections.go and
  import.go to write fresh collections to disk.
