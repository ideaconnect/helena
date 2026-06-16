# Packaging & distribution

Status and conventions for shipping Helena. App identity/version come from
[`FyneApp.toml`](../FyneApp.toml) (consumed by `fyne package`); the app ID is
`tech.idct.helena`.

## Current distribution

CI builds native per-OS binaries on every push (no cross-compile):
`helena-linux-amd64`, `helena-windows-amd64.exe`, `helena-darwin-arm64`. Binaries
embed their version (`helena --version`).

A tagged `v*` push publishes a GitHub Release whose assets are (issues #27/#35):

- **Archives** — `helena-linux-amd64.tar.gz`, `helena-darwin-arm64.tar.gz`,
  `helena-windows-amd64.zip` (each bundling the binary + `LICENSE` + `README.md`).
- **`SHA256SUMS`** — SHA-256 checksums over every asset.
- **`helena.sbom.spdx.json`** — an SPDX software bill of materials.
- **Provenance attestation** — a keyless (Sigstore) build-provenance
  attestation for the archives, verifiable with `gh attestation verify`.

## Linux desktop integration

[`packaging/linux/`](../packaging/linux/) carries the freedesktop assets:

| File | Installs to | Purpose |
| ---- | ----------- | ------- |
| `tech.idct.helena.desktop` | `/usr/share/applications/` | App-menu entry (`desktop-file-validate` clean). |
| `tech.idct.helena.metainfo.xml` | `/usr/share/metainfo/` | AppStream metadata for software centers / Flatpak (`appstreamcli validate` clean). |
| `tech.idct.helena.png` | `/usr/share/icons/hicolor/<size>/apps/` (or `.../512x512/...`) | Application icon, named after the app ID. The source is 896×896; downscale into the hicolor sizes you ship, or install it under a single large size and let the icon theme scale it. |

A packaged install (`.deb`/`.rpm`/Flatpak/AppImage) bundles all four plus the
`helena` binary on `PATH`. These files are the gateway: every Linux packaging
format embeds the same `.desktop` + icon + AppStream metainfo. (The packaging
pipelines themselves — AppImage/Flatpak/.deb/.rpm and the Windows
installer/winget/Scoop — are tracked separately as issues #37 and #38.)

## macOS distribution — deferred (decided 2026-06-16)

macOS is **built and tested in CI** (`macos-latest`, native clang/cgo), but
signed/notarized **distribution is deferred**. Rationale:

- A Gatekeeper-launchable `.dmg` requires a paid Apple Developer ID plus
  `xcrun notarytool`; we have neither set up yet.
- Helena is pre-1.0 and the maintainer's primary platforms are Linux/Windows.

Plan when revisited (issue #39): `fyne package -os darwin` (consuming
`FyneApp.toml`) → `.app` → `.dmg`, sign with a Developer ID cert, notarize +
staple, then add a Homebrew cask. Until then, macOS users build from source.

## Updates — package-manager / manual, no phone-home (decided 2026-06-16)

Helena does **not** check for updates at runtime. A startup update-check would
be a background network request, which contradicts the
[no-background-traffic / no-telemetry guarantee](../README.md#privacy). So
(issue #40):

- The official update channels are **package managers** (Flatpak / Homebrew /
  winget / Scoop, as those land) and **manual re-download** of GitHub Releases.
- There is no opt-in/opt-out toggle because there is nothing to phone home.
- `helena --version` lets you compare your build against the latest release
  yourself.

This is the privacy-preserving choice and is revisited only if a clearly
opt-in, offline-by-default mechanism is designed.
