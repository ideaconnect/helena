# Mac App Store sandbox — validation harness

A **throwaway** standalone program that proves Helena's "Option A" file-access
approach works on a **real signed + sandboxed** macOS build **before** we wire
any of it into Helena. It is a separate Go module (own `go.mod`) so the main
repo's `go build ./...`, tests, and CI never touch it.

## Why it exists

The Mac App Store mandates **App Sandbox**. Helena's model — collections are
OpenCollection folders *anywhere on disk*, remembered and re-read on launch —
is illegal under the sandbox unless we use the native **NSOpenPanel** (powerbox)
plus **app-scope security-scoped bookmarks**. Three things must be verified on a
real sandboxed build, and **a plain `go build` cannot** (a dev build gets an
unrestricted sandbox that silently masks every failure):

1. NSOpenPanel actually grants access to a user-picked folder under the sandbox.
2. An app-scope bookmark, minted at pick time, **resolves and re-grants access
   on a later launch** (`startAccessingSecurityScopedResource`).
3. **The blocker:** Helena's `storage.Save` stages its atomic temp/rename dirs
   as **siblings in the collection's parent** — a write *outside* the bookmarked
   scope. Does that fail on relaunch? (If so, `Save` must stage *inside* the
   collection dir.)

## Prerequisites

- macOS 11+.
- **Go** (the same 1.26.x you build Helena with).
- **Xcode Command Line Tools** (`xcode-select --install`) for the Objective-C
  compiler + AppKit headers.
- **No Apple Developer account required** — ad-hoc signing is enough to activate
  the sandbox locally.

## Run it

```sh
cd packaging/darwin/appstore/harness
./build.sh

# then, from the same terminal:
./HelenaSandboxHarness.app/Contents/MacOS/harness   # 1st run — pick a folder
./HelenaSandboxHarness.app/Contents/MacOS/harness   # 2nd run — across-relaunch test
```

On the **first run** a native folder panel appears — create and choose a new
empty folder **outside** `~/Library/Containers` (e.g. `~/Documents/helena-sbtest`).
The harness mints + saves a bookmark and runs the Load/Save test with the fresh
grant. **Quit, then run it again**: the second run resolves the *saved* bookmark
(no fresh grant) and runs the same test — this is exactly Helena's every-launch
situation.

Reset between full re-tests:

```sh
rm -rf ~/Library/Containers/tech.idct.helena.harness
```

## How to read the output

```
config dir (os.UserConfigDir): /Users/you/Library/Containers/tech.idct.helena.harness/Data/Library/Application Support
```
→ if this path is **inside** `Library/Containers`, the sandbox is active (good).
If it's your real `~/Library/Application Support`, the app is **not** sandboxed —
fix signing/entitlements before trusting any result.

Per-test lines:

| Line | Meaning |
| --- | --- |
| `RESOLVE+START … ok` on the **relaunch** run | Bookmarks work across launches ✅ (the core of Option A) |
| `RESOLVE+START … FAIL` | Missing `files.bookmarks.app-scope` entitlement or a signing problem |
| `LOAD … PASS` | Reading the collection dir works under scope |
| `SAVE-I … PASS` | Staging temp dirs **inside** the collection dir works |
| `SAVE-P … PASS/FAIL` | Staging a **sibling in the parent** (Helena's *current* `storage.Save`) |
| `HTTPS GET … PASS` | `network.client` entitlement works |

**The decisive result** is on the **relaunch** run:

- `SAVE-P FAIL` + `SAVE-I PASS` → **blocker confirmed**: restructure
  `internal/storage/store.go` to stage its `.helena-save` / `.helena-old` temp
  trees *inside* the collection dir, not as siblings in the parent.
- `SAVE-P PASS` → the app-scope bookmark also covers parent writes; `storage.Save`
  can stay as-is. (Less likely, but the harness tells us for sure.)

Please paste the full output of **both** runs back and I'll build the real
`internal/sandbox` package + `storage.Save` change accordingly.

## Notes / gotchas

- Run the binary **directly** (not `open …`) so stdout is visible in your terminal.
- If `RESOLVE+START`/bookmarks misbehave under ad-hoc signing, rebuild with a
  real cert: `SIGN_ID="Apple Development: you@example.com (TEAMID)" ./build.sh`
  (a free Apple ID in Xcode is enough to create one).
- This bundle id is `tech.idct.helena.harness` — separate from the real app, so
  it never touches Helena's config or collections.
- Delete this whole `harness/` directory once validation is done; it is not part
  of any Helena build.
