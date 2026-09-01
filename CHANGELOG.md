# Changelog

All notable changes to Helena are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and Helena adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html):
tags are `vMAJOR.MINOR.PATCH`.

- **MAJOR** — incompatible changes to the on-disk format (config / collection
  schema) or to documented behaviour that a user would have to react to.
- **MINOR** — new, backward-compatible functionality.
- **PATCH** — backward-compatible bug fixes.

Pre-1.0, the public surface is still settling; minor versions may include small
breaking changes, called out under **Changed** with a migration note.

Released notes are also generated on GitHub from merged PRs, categorized by
label via [`.github/release.yml`](.github/release.yml).

## [Unreleased]

### Added
- **The response viewer remembers its soft-wrap toggle.** Turning word wrap on
  (the viewer toolbar's wrap button) now sticks: the mode is stored in the
  session UI state and restored on the next launch, instead of resetting to
  horizontal scroll every time Helena starts.

### Changed
- Bumped `go-fyne-pretty-view` to `v2.5.1-alpha` for its new
  `SetOnWrapChanged` hook, which is what lets Helena observe (and persist) a
  wrap toggle made through the viewer's own toolbar.

### Security
- **Go toolchain pinned to 1.26.7**, closing six standard-library advisories
  `govulncheck` reports as reachable from Helena:
  [GO-2026-6218](https://pkg.go.dev/vuln/GO-2026-6218) (`net/url`),
  [GO-2026-6090](https://pkg.go.dev/vuln/GO-2026-6090) (`crypto/tls`),
  [GO-2026-6089](https://pkg.go.dev/vuln/GO-2026-6089) and
  [GO-2026-5026](https://pkg.go.dev/vuln/GO-2026-5026) (`net/http`),
  [GO-2026-6088](https://pkg.go.dev/vuln/GO-2026-6088) (`encoding/xml`
  recursion depth — reached by the WSDL/OpenAPI importers) and
  [GO-2026-5972](https://pkg.go.dev/vuln/GO-2026-5972) (`encoding/asn1`).
  All are fixed in 1.26.6; 1.26.7 is the current patch.
- **`golang.org/x/text` bumped to v0.39.0**, closing
  [GO-2026-5970](https://pkg.go.dev/vuln/GO-2026-5970), reachable through
  goja's Unicode normalization on the scripting path.

## [0.7.0] - 2026-07-15

### Added
- **Unsaved-edit markers on request tabs.** An open request tab shows a trailing
  `*` while its content differs from what's on disk, and clears it on Save (or an
  edit back to the saved value) — the same per-tab dirtiness the quit guard uses.
- **Alternate redo chord.** Ctrl/Cmd+Shift+Z now redoes in any text field (in
  addition to Ctrl+Y). Undo (Ctrl+Z) and redo already worked in the fields and
  the body/response editor; the F1 keyboard-shortcuts dialog and the user guide
  now document these editing shortcuts.

## [0.6.0] - 2026-07-15

### Added
- **Path parameters.** A new **Path** tab fills the single-brace `{name}`
  placeholders in a request URL's path (e.g. `{{base_url}}/users/{id}`). The
  tab lists one row per placeholder, derived live from the URL, so you set a
  value instead of hand-editing the raw URL; the resolved preview under the URL
  shows the real target and flags any placeholder still unfilled. Path values
  may themselves reference `{{variables}}` and ask-at-send `{{?prompts}}`, and
  the OpenAPI importer pre-fills them from each `in: path` parameter's schema
  default and description. (This is distinct from `{{name}}` variables — double
  braces resolve from environments/collections, single braces are per-request
  path values.)

### Fixed
- **Open Collection `type: path` parameters now fill the URL path instead of
  the query string.** A path parameter authored by Bruno (or hand-written) was
  loaded as an ordinary query param and appended to the query string on send;
  it is now routed to the Path tab and substituted into its `{name}`
  placeholder as intended.

## [0.5.3] - 2026-07-15

### Fixed
- **Keyboard shortcuts now work while a text field or editor has focus.** Every
  app shortcut (Send, Save, Open, Import, New request/collection, Duplicate,
  Undo delete, Edit environments, Settings) was registered only against the
  window canvas; Fyne only dispatches a canvas-level shortcut when the
  focused widget doesn't itself handle shortcuts — and the URL bar, every
  auth field, the body/response editors, and the other text fields all do.
  In practice a shortcut went silently dead the moment any of them had
  focus, which is most of a normal session. Shortcuts now also reach the
  app's action table from inside a focused entry or editor.

### Security
- **`go-fyne-pretty-view` bumped to v2.4.1-alpha**, closing the same
  [GO-2026-5856](https://pkg.go.dev/vuln/GO-2026-5856) `crypto/tls` toolchain
  vulnerability in that dependency's own pinned build toolchain (already
  fixed for Helena's own toolchain in 0.5.2). The bump also carries the
  `PrettyView.SetHostShortcuts` API the fix above depends on.

## [0.5.2] - 2026-07-15

### Added
- **Microsoft Store listing is live.** Helena is now published on the Microsoft
  Store (<https://apps.microsoft.com/detail/9NWPKK6CTDR1>) for a one-click,
  auto-updating install on Windows. The README, website (download page, roadmap,
  and top-bar CTA), and packaging docs now link to the live listing instead of a
  "coming soon" placeholder.

### Fixed
- **Imported OpenAPI/Swagger requests now populate their body.** Specs that
  describe a body with a `$ref` schema and no inline example (the common case)
  previously imported with a blank body; Helena now synthesizes a representative
  JSON body from the schema. Swagger-2 body parameters with no `consumes` (a
  `*/*` media type) are also treated as JSON. (#183)
- **No double slash between base URL and path on import.** A server URL with a
  trailing slash used to render `base//path`; the hoisted `base_url` is now
  trimmed so the join stays single-slash. (#183)
- **Imported environments are named after the spec's server.** The environment
  now takes the OpenAPI server's `description` (e.g. "Api aplikacji
  developerskiej") instead of a generic "Default". (#183)
- **Postman collections using the `request` string shorthand import again.** A
  bare-URL `request` (valid Postman v2.1) no longer fails the whole import. (#188)
- **A pre-request script can no longer hang the app.** A hostile accessor on the
  request object (e.g. an infinite-loop getter) wedged the Send worker forever;
  the post-script read-back is now bounded by the same interrupt guard as
  execution. (#187)
- **Runaway script output is capped.** A `while(true) console.log(...)` (or a
  runaway `test(...)`) no longer grows unbounded and OOMs/freezes the UI —
  console and test output are limited with a truncation marker. (#190)
- **A stalled WebSocket server no longer freezes the app.** Frame writes (called
  from the UI thread) now use a write deadline, so a server that stops reading
  can't block indefinitely. (#189)
- **Microsoft Store taskbar icon no longer shows a blue background.** The MSIX
  package now ships unplated icon variants (and a resources.pri) so Windows draws
  the cat without the system-accent plate. (#184)

### Security
- **Go toolchain bumped to 1.26.5** to pick up the `crypto/tls` fix for
  [GO-2026-5856](https://pkg.go.dev/vuln/GO-2026-5856) (Encrypted Client Hello
  privacy leak), which `govulncheck` flagged as reachable via the WebSocket/TLS
  paths. No source changes: `go.mod`'s `go` directive is pinned to the exact
  patch (`go 1.26.5`) so CI's `setup-go` installs the fixed toolchain instead of
  a cached `1.26.x`.

## [0.5.1] - 2026-07-08

### Fixed
- **Microsoft Store package name**: the Store `.msixbundle` now declares the
  reserved product name **Helena API Client** as its package display name
  (`Package/Properties/DisplayName`). v0.5.0's bundle was rejected at upload for
  using the unreserved name "Helena". The on-device app/tile name stays
  "Helena", and the desktop binaries are functionally identical to v0.5.0 apart
  from the version stamp.

## [0.5.0] - 2026-07-08

### Added
- **Status bar** with the running version and an **opt-in "Check for updates"**
  that queries the latest GitHub release only when clicked — never
  automatically, keeping the no-background-traffic guarantee.
- **About dialog tribute**: Help → About now shows a photo of Helena, the cat
  the app is named after, with a short note.
- **Help menu**: a **Website** link (idct.tech/helena) and a **Buy me a coffee**
  link, and the issue-tracker link cleaned up.
- A brief **"Saving…" spinner** when closing the window, so a close that flushes
  state shows feedback instead of a frozen window.
- **Roomier Open / Save / Folder dialogs** — they open larger than Fyne's
  cramped default.

### Changed
- **Releases are now cut by publishing a GitHub Release** (Releases → Draft a new
  release → publish): CI builds every platform + the Linux packages and attaches
  them, and separately builds the **Windows Store `.msixbundle`** as a workflow
  artifact. A bare tag push no longer creates a release.
- Docs/website accuracy sweep: README's feature list now covers request
  history, the cookie jar, drag-and-drop reordering, and the full variables
  system; the website names the Postman importer and the `.deb`/`.rpm`
  release packages; the roadmap no longer presents winget/Scoop/Inno Setup as
  shipped (committed but not wired into CI); the User Guide gained Real-time,
  Request history, and Headless runs sections plus complete auth/body/export
  enumerations.

## [0.4.0] - 2026-07-05

### Added
- Request history (**Help → History**): a bounded, restart-persistent log of
  your recent sends — method, URL, status, and time — with **Restore** (reopen
  the request in a new tab), **Resend**, and **Clear**. Snapshots are
  secret-scrubbed before they touch disk, so `history.yml` never stores a
  credential (a resend re-resolves auth from the active environment), matching
  the collection YAML's secret externalization.
- Quitting with unsaved request edits now asks for confirmation instead of
  dropping them silently: closing the window while any open request has edits
  not yet Saved (or a scratch tab with content) shows a **Discard & quit /
  Cancel** dialog. Everything else (add / rename / delete / move, and
  collection / folder / environment / global variables) still saves as you go,
  so the prompt only appears for editor edits genuinely pending a Save.
- `helena run --format json|junit` — machine-readable reports for the headless
  collection runner. `json` emits totals + a `failed` flag + per-request
  status/duration/checks; `junit` emits JUnit XML (one `<testcase>` per request)
  for CI dashboards. The default stays the human-readable `text` summary. Flags
  may now be written before *or* after the collection dir (they were previously
  ignored when placed after it).
- Folder-scoped runs: the runner can now execute a single folder's subtree
  instead of the whole collection. In the GUI, selecting a folder and pressing
  **Run** runs just that folder; headlessly, `helena run <dir> --folder
  <name-path>` (e.g. `--folder Auth/OAuth`) does the same. Report paths stay
  collection-relative either way.
- Schema-versioned config with forward migration.
- Configurable response-body size cap (Settings → Max response).
- `helena --version` reporting the build's tag + commit.
- Dedicated environment manager and structured form-body editor.
- Native **Windows on ARM (arm64)** build, attached to each release as
  `helena-windows-arm64.zip`. Built on GitHub's `windows-11-arm` runner with
  llvm-mingw's native aarch64 cgo toolchain (the runner's stock gcc is x86-64
  and can't assemble arm64 cgo); its CI leg runs without `-race`, which has no
  windows/arm64 support.

### Changed
- Token-endpoint and transport errors are redacted of secrets before display.
- `storage.Save` is now atomic (stage-and-swap); a failed save leaves the
  on-disk collection untouched and the in-memory model rolls back to match.
- Sends reuse a per-session HTTP transport (connection-pool reuse).
- Dependencies bumped to latest, notably the response/request body viewer
  `go-fyne-pretty-view` v2.2.0-alpha → v2.3.0-alpha. As a result, pressing
  **Tab** in the request-body or GraphQL-variables editor now inserts a tab
  character instead of moving focus to the next field (use the mouse to leave
  the editor). The pre/post-request scripting engine (goja) also gained
  ES2018 named capture groups, so `$<name>` backreferences and `.groups` work
  in user scripts.
- The README "Memory & rendering" section now documents the rendering path:
  Helena draws through Fyne's OpenGL painter (a standard desktop GL 2.1+
  context via GLFW) and is **hardware-accelerated whenever the OS provides a
  GPU-backed OpenGL driver** — textures and framebuffers then live in VRAM.
  The shipped binary contains no software-rendering path or flag; software GL
  (VM / RDP / Basic Display Adapter) is an OS-side substitution. Includes how
  to check `GL_RENDERER` (`glxinfo` / `wglinfo` / Task Manager's GPU column).

### Fixed
- Reordering collections (drag-and-drop in the sidebar) no longer scrambles
  which environment each one has selected. The active-environment map was keyed
  by collection index and wasn't remapped on a move, so after a reorder a
  collection could resolve `{{variables}}` against a different collection's
  environment — and the mis-selection could then be persisted.
- Undo (restore a deleted request/folder) now re-targets its parent by a stable
  ID, so a delete followed by an index-shifting edit (e.g. duplicating a sibling
  folder) then an undo restores the item into the right container instead of the
  one that shifted into its old slot. A delete whose save fails no longer leaves
  undo armed, which could have re-inserted a duplicate with the same ID.
- Importing a malformed OpenAPI/Swagger spec no longer crashes the app. A spec
  with `servers: [null]` (a nil server), or various null sub-objects that made
  kin-openapi's Swagger-2 conversion panic, now returns a clean error — this
  mattered most for **Import from URL**, which ran the parse off the UI thread
  with no panic guard, so a hostile server could take the whole app down. Path
  ordering is also `O(n log n)` now, so a spec with tens of thousands of paths
  doesn't import quadratically.
- A chain step that references its target only by stable ID (`RequestID` set,
  the human path left blank — a valid, documented state) now resolves and runs
  instead of being rejected as having "no request reference".
- Chain aliases that are JavaScript object built-ins (`__proto__`, `constructor`,
  `toString`, …) are now rejected with a clear error; previously they passed
  validation and corrupted the script-side `chain` object (e.g. `__proto__`
  reassigned its prototype) instead of binding a step result.
- Variable resolution memoizes each variable's expansion within a single pass,
  so a value that references another more than once (`{{a}}{{a}}`) no longer
  re-expands exponentially — a deep, branching set of collection/environment
  variables could previously wedge a send for minutes.
- Credentials are no longer forwarded over cleartext on an `https`→`http`
  redirect. A same-host protocol downgrade previously kept the `Authorization`
  header and any caller-flagged credential header (e.g. an API key), because the
  stripping keyed only on a host change; the downgrade now drops them too.
- A failed **Digest** or **NTLM** authentication handshake now surfaces the
  original `401` (status, headers, body) instead of a confusing
  `read on closed response body` error. The challenge response's body is
  preserved until the authenticated retry actually supersedes it — relevant when
  a proxy drops connection affinity mid-NTLM or the Digest retry's connection is
  reset.
- The per-send request snapshot now also detaches the request's resolved
  **Auth** credential sub-struct: editing the Auth tab (e.g. a password) while a
  send or SSE stream was in flight could race the off-UI worker, which
  dereferences the same allocation when signing the request. The slice fields
  were already detached; auth was re-aliased by the inherit-flattening step that
  runs after the snapshot. Request/Auth deep-copying now lives in one place
  (`model.Request.Clone` / `model.Auth.Clone`) that the send, stream, history,
  and secret-scrub paths all share, so the field lists can't drift.
- Restoring or resending a **History** entry and then editing it no longer
  corrupts the stored log: `Store.Entries()` now hands back a deep copy, so an
  in-place header/param edit on the reopened request can't reach back and
  rewrite a past entry to a value that was never sent (and persist it).
- The quit guard no longer shows a phantom "unsaved changes" prompt for a
  restored-but-never-opened tab whose URL carries an inline query. Saving a
  sibling tab used to baseline such a tab in its pre-fold form; its first
  open then folded the query into params and read as an edit. Never-opened
  tabs are now left unbaselined until their first activation captures the
  correct post-fold state.
- `helena run --format junit` now emits the `<testcase>` `time` as a plain
  decimal even for sub-100µs requests (previously a raw float rendered as
  `time="5e-05"`, which strict `xs:decimal` JUnit consumers reject).
- The per-send request snapshot now also detaches the request's **Variables**
  and **Assertions** slices (it already detached params/headers/body-form/chain):
  editing those two tabs while a send was in flight could race the off-UI
  worker on their shared backing array. This also keeps a request-history entry
  fully independent of the live request it was captured from.
- The per-collection environment selection persists across launches again:
  constructing the UI used to fire the Environment dropdown's change handler,
  which deleted the saved choice from config.yml before it could be restored.
- `config.yml` is now written atomically (staged file + rename). It is
  rewritten on every tab/env/workspace change and at quit; a crash mid-write
  could previously truncate it, and the next launch would silently fall back
  to an empty session with no workspaces.
- Closing the WebSocket dialog while the connection attempt was still in
  flight used to leak the worker goroutine (and, on late success, the socket)
  indefinitely; the dial is now cancelled, and a server that accepts TCP but
  never answers the upgrade can no longer hang the worker forever. WebSocket
  messages reassembled from continuation frames are also capped at 64 MiB,
  matching the existing per-frame cap.
- SSE streams are no longer killed by the request timeout: the client-wide
  deadline covers the whole exchange including the body read — which for a
  stream is the stream itself — so every stream died at Timeout (default
  30 s). The timeout now bounds only the connect + response-header phase; an
  open stream runs until the server closes it or you press Stop.
- Removing a collection no longer discards the other collections' unsaved
  in-memory edits: it used to reload the entire workspace from disk (also
  O(workspace) work for a one-entry delete); it now removes just that entry,
  keeping each surviving collection's edits and environment selection intact.

### Performance
- Release builds ship with `-tags no_emoji`, dropping Fyne's bundled colour-emoji
  font (which Fyne parses fresh per theme scope): **-75 MB resident (-23%,
  326 → 251 MB measured)** and a ~4 MB smaller binary. Colour-emoji glyphs render
  blank; response text is unaffected. Replaced or cleared large response bodies
  (send, tab close, stream start) also promptly return their freed memory, so
  repeated big sends no longer ratchet RSS up across a session. See the README
  "Memory & rendering" note — most remaining memory is the OpenGL driver, which
  balloons under software rendering (VM / RDP / no GPU driver), not Helena's code.
- A response body is now held once and shared between the tab cache and the
  response viewer instead of being copied twice more per send, and switching
  tabs re-displays the cached body without re-copying it. Closed or dropped
  tabs release their cached responses immediately (they could previously stay
  pinned through the tab strip's backing array).
- SSE streaming repaints are coalesced: a burst of events triggers one
  re-parse of the accumulated transcript instead of one per event (the old
  per-event repaint made long streams quadratically expensive and could
  saturate the UI queue).
- Chainless sends no longer deep-copy the whole active collection (that
  snapshot only exists for chain resolution); the headless collection runner
  reuses one HTTP client for the whole run instead of building a transport per
  request; idle keep-alive sockets now expire after 90 s instead of living for
  the session; and post-response script format sniffing no longer copies the
  body. OAuth2 token-endpoint responses are capped at 1 MiB.
- Startup: main-UI construction is **7× faster (208 → 28 ms) with 80% less
  allocation (245 → 49 MB)**, measured by the new `BenchmarkNewMainUI`. The
  bulk comes from cutting Fyne theme scopes from 11 to 3 — each
  `container.NewThemeOverride` made Fyne re-parse the embedded fonts per
  scope × text style and eagerly walk the wrapped subtree. The splits' thin
  hairline divider is now a dedicated widget and the root's flush joins a
  plain layout, instead of theme-scope overrides; as a side effect the
  hairlines are now genuinely flush (the old zero-padding root scope never
  reached Fyne's layout code, which reads the global theme, so there were
  silent 4 px gaps — website screenshots regenerated). Also: collections load
  concurrently at startup, toolbar icons are built once instead of per
  lookup, the embedded window icon is a 256×256 downscale (−1 MB binary and
  resident, ~12× less pixel data decoded on the GL thread before first
  frame), and switching tabs no longer rewrites config.yml when nothing
  changed. Steady-state RSS on a software-GL box drops ~20 MB (223 → 203 MB
  measured; the GL driver dominates there — hardware-GL setups keep most of
  the win as faster startup).
- Request path: pre/post scripts compile once per distinct source (the goja
  program is cached process-wide and runs on each send's fresh, isolated VM),
  instead of recompiling on every send and twice per chain step. The response
  viewer now renders at most the first 16 MiB of a body — its parse runs
  synchronously on the UI goroutine at ~5–7× the source size, so an uncapped
  100 MiB body (the HTTP cap's default) froze the UI for a ~600 MB parse; the
  truncation is flagged on the status line and **Save response** always writes
  the full body.

### Security
- Bearer-token, API-key, and Secret environment values are masked in the UI.
- Added `SECURITY.md`; CI gates on `govulncheck`.

<!--
When cutting a release, move the relevant Unreleased entries under a new
`## [vX.Y.Z] - YYYY-MM-DD` heading.
-->
