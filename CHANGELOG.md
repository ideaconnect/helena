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
- Quitting with unsaved request edits now asks for confirmation instead of
  dropping them silently: closing the window while any open request has edits
  not yet Saved (or a scratch tab with content) shows a **Discard & quit /
  Cancel** dialog. Everything else (add / rename / delete / move, and
  collection / folder / environment / global variables) still saves as you go,
  so the prompt only appears for editor edits genuinely pending a Save.
- Schema-versioned config with forward migration.
- Configurable response-body size cap (Settings → Max response).
- `helena --version` reporting the build's tag + commit.
- Dedicated environment manager and structured form-body editor.

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
