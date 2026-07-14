# Mac App Store — sandbox file-access design & continuation guide

Status: **design done + validation harness ready; blocked on a real-Mac test run.**
This file is the single source of truth to continue the work in any future
session. Decisions, the validated architecture, the one open question, and the
step-by-step plan are all here.

---

## TL;DR — how to continue

1. **On a Mac (owner, ~Saturday):** run the validation harness in
   [`harness/`](harness/) — see [`harness/README.md`](harness/README.md). Paste
   **both runs'** output back.
2. **The harness answers one blocking question** — does `storage.Save`'s
   parent-directory staging work under a security-scoped bookmark on relaunch
   (`SAVE-P`)? The answer decides whether [Phase 2](#phase-2--storagesave) is
   needed.
3. **Then build the real thing** in phases below. Almost all of it compiles and
   is testable on Linux; only the darwin Cgo file and the signing/packaging need
   the Mac.

The in-chat **release runbook** (Apple certs, `fyne release`, entitlements,
Transporter, App Store Connect) should be folded into
[`../../../docs/PACKAGING.md`](../../../docs/PACKAGING.md) once the design lands.

---

## Decision log

- **Ship on the Mac App Store.** MAS mandates **App Sandbox**.
- **Option A chosen** (over container-only storage): keep Helena's model —
  collections are OpenCollection folders *anywhere on disk*, remembered and
  re-read on launch — made sandbox-legal with the native **NSOpenPanel**
  (powerbox) + **app-scope security-scoped bookmarks**. Option B (collections
  live only in the app container) was rejected: it kills the "collection lives
  in your git repo" workflow that distinguishes Helena from Postman.
- **Bookmark storage: a mac-only sidecar file** (not inline in `config.yml`).
  Keeps `config.yml` clean/hand-editable, needs no schema version bump, and has
  zero footprint on Windows/Linux/direct-download builds.
- **Build tag: `darwin && appstore`.** Only the Mac App Store build compiles the
  Cgo/native path; the notarized *direct-download* macOS build keeps Fyne's
  dialog + plain paths. **Never tag it plain `darwin`.**

---

## Why a native picker is required (verified)

Fyne 2.7.4's desktop macOS file dialog is **not** `NSOpenPanel` —
`dialog/file_darwin.go` only computes favorite URIs and the browser lists
directories with in-process `os.ReadDir`. Under the sandbox it can neither browse
outside the container nor trigger a powerbox sandbox extension. Only the real
`NSOpenPanel`, hosted out-of-process by powerbox, grants access to a
user-selected path under `com.apple.security.files.user-selected.read-write`.

## Threading model (verified against Fyne source — the risky part is fine)

`internal/driver/glfw/loop.go` locks `main.main` to the OS main thread and runs
`runGL` as a select loop that, every 1/60s, calls `glfw.PollEvents()` — which on
macOS pumps the NSApp event queue. So **the AppKit main thread IS the Fyne main
goroutine**, and glfw hand-pumps events (never `[NSApp run]`). Therefore:

- Present `NSOpenPanel` with **`-beginWithCompletionHandler:`** — its completion
  block is serviced by Fyne's existing pump, on the main thread, no nested loop.
- Enter it by hopping to main with the **non-blocking `fyne.Do`**. **Never
  `fyne.DoAndWait`** — from the main goroutine that trips `async.EnsureNotMain`,
  which reroutes onto a fresh goroutine and would run AppKit off-main → crash.
- Return the result to Go via a `runtime/cgo.Handle`-tagged `//export` callback.
- **Mint the app-scope bookmark at pick time** (fresh powerbox URL) — it cannot
  be recreated later from a bare path string.

## Entitlements (`packaging/darwin/appstore/entitlements.plist`)

```
com.apple.security.app-sandbox                        true   # mandatory
com.apple.security.network.client                     true   # Helena is an HTTP client
com.apple.security.files.user-selected.read-write     true   # NSOpenPanel-picked folders
com.apple.security.files.bookmarks.app-scope          true   # create/resolve the bookmarks
```

The bookmarks entitlement is the one people forget; without it resolve /
`startAccessingSecurityScopedResource` fails on relaunch.

---

## The one open question the harness answers

`storage.Save` ([`../../../internal/storage/store.go`](../../../internal/storage/store.go), ~lines 32-79)
stages its atomic temp/rename trees (`<dir>.helena-save`, `<dir>.helena-old`) as
**siblings in the collection's PARENT dir**, then renames. A security-scoped
bookmark on the collection dir grants the dir + its descendants, **not siblings
in its parent**. So Save works during the create/import session (the fresh
powerbox parent grant is still held) but may **fail EPERM on the next launch**,
when only the child-dir bookmark is resolved.

The harness tests both staging strategies on a relaunch:

- **`SAVE-P FAIL` + `SAVE-I PASS`** → **Phase 2 required**: restructure
  `storage.Save` to stage its temp trees *inside* the collection dir. (Trade-off:
  the current sibling-swap is a clean two-rename atomic tree replace; staging
  inside loses that, so the refactor must preserve crash-safety another way — a
  temp subdir + per-file swap, or accept a fsync'd write-then-rename per file.)
- **`SAVE-P PASS`** → the app-scope bookmark also covers parent writes; Save can
  stay as-is and Phase 2 is skipped.

---

## Wiring map (what changes, with file:line)

**Filesystem access that must go through the sandbox seam:**

| Site | Today | Under Option A |
| --- | --- | --- |
| `internal/session/session.go:148` (`reload` → `storage.Load` per persisted dir) | plain `Load(dir)` | resolve bookmark + `startAccessing` **before** `Load`; hold for session |
| `internal/session/session.go:230` (`OpenCollection` → `Load`) | plain `Load` | same |
| `internal/session/session.go:1070` (`persistCollection` → `storage.Save`) | plain `Save` | run under the held scope |
| `internal/session/dotenv.go:32` (`<dir>/.env` read) | `os.ReadFile` | under the held scope |
| `internal/httpclient/httpclient.go:666` (`Body.FilePath` read at send) | `os.ReadFile` | resolve the **body-file** bookmark in UI/session before dispatch; keep httpclient platform-neutral |

**Dialog picks that must use the native `NSOpenPanel`/`NSSavePanel`:**

| Site | Flow |
| --- | --- |
| `internal/ui/shell.go:949` `openCollection` | pick collection dir → mint+store bookmark → `OpenCollection` |
| `internal/ui/collections.go:22` `actionNewCollection` | pick **parent** → create subdir (while parent scope held) → **bookmark the child** → open |
| `internal/ui/import.go:202` `chooseImportDestination` | same parent→child pattern as new-collection |
| `internal/ui/import.go:154` `importFromFile` | native file-open; transient read, no persistent bookmark |
| `internal/ui/bodyfile.go:45` `chooseBodyFile` | native file-open; **persistent** bookmark keyed by the file path (read later at send) |
| `internal/ui/saveresponse.go:46` `saveResponseToFile` | native `NSSavePanel`; transient write |

**Confirmed NOT a problem (inside the container, no bookmark needed):**
`config.yml`, `history.yml`, `secrets/`, bundled-sample dir — all under
`os.UserConfigDir()`, which resolves inside the sandbox container. Export is
clipboard-only; there is no OS file drag-drop and no recent-files feature.

**Gotchas to honor:**
- **Body files** need a *persistent* bookmark keyed by the **file path** (not a
  collection dir) — so the bookmark store must be path-keyed, not dir-keyed.
- **In-container / empty-bookmark collections** (the bundled sample) must fall
  back to a plain `Load`, not be treated as a resolve error.
- **Normalize the picked path** once at the picker boundary (`OpenCollection`
  dedupes by dir string; a symlink/case-different powerbox path would diverge).
- `HELENA_LOG` / `-log-file` / `HELENA_SECRETS_DIR` pointed outside the container
  are denied under sandbox — document as a MAS-build limitation.

---

## Phased implementation plan

- **Phase 0 — VALIDATE (Mac, owner):** run [`harness/`](harness/); settle the
  `SAVE-P`/`SAVE-I` question and confirm the threading + entitlements + bookmark
  lifecycle on a real signed sandboxed build.
- **Phase 1 — scaffolding (Linux, testable, CI-green):**
  - `internal/sandbox` package: `//go:build darwin && appstore` Cgo impl +
    `//go:build !(darwin && appstore)` no-op stub. API roughly
    `Pick(cb)`, `Create(path) (bookmark, err)`, `Resolve(path, bookmark) (release, refreshed, err)`.
  - Mac-only sidecar bookmark store (path-keyed, base64), like
    `internal/storage/secrets.go`.
  - Session wiring behind the stub (resolve-before-Load, hold, release on
    remove/quit, empty-bookmark → plain Load, stale write-back).
  - UI picker seam (`m.pickFolder` / file variants) — default wraps Fyne's dialog.
  - Tests for the stub + store on Linux; docs per CLAUDE.md.
- **Phase 2 — `storage.Save`** (only if the harness shows `SAVE-P` fails): stage
  temp trees inside the collection dir, preserving crash-safety; regression test.
- **Phase 3 — darwin Cgo + packaging (authored on Linux, built on Mac):** the
  `NSOpenPanel` + bookmark `.m`/`.go`, entitlements plist, `fyne release` →
  re-codesign → `productbuild` script (mirror `packaging/windows/msix/`), a
  `macos-latest` CI job gated on release (mirror the `msix` job), plus a
  `go build -tags appstore,no_emoji ./cmd/helena` compile-check on the existing
  macOS matrix leg so the Cgo file can't rot.
- **Phase 4 — submit:** App Store Connect record (`tech.idct.helena`), certs +
  provisioning profile, upload the signed `.pkg` via Transporter, submit.

## Housekeeping

The [`harness/`](harness/) directory is a **throwaway** — delete it once Phase 0
is done. This design doc stays.
