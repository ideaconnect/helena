# Packaging & distribution

Status and conventions for shipping Helena. App identity/version come from
[`FyneApp.toml`](../FyneApp.toml) (consumed by `fyne package`); the app ID is
`tech.idct.helena`.

## Current distribution

CI builds native per-OS binaries on every push (no cross-compile):
`helena-linux-amd64`, `helena-windows-amd64.exe`, `helena-darwin-arm64`. Binaries
embed their version (`helena --version`).

Release binaries (and `make build` / `make.bat build`) are built with
**`-tags no_emoji`**, which drops Fyne's bundled 4.2 MB colour-emoji font. Fyne
parses that font fresh per theme scope, so excluding it cuts resident memory by
75 MB (-23%, 326 → 251 MB measured) and shrinks the binary by ~4 MB; response
text still renders — colour-emoji glyphs come out blank. A plain `go build` /
`go run` keeps emoji for development.

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

A packaged install bundles all four plus the `helena` binary on `PATH`. These
files are the gateway: every Linux packaging format embeds the same `.desktop`
+ icon + AppStream metainfo.

### Linux packages — `.deb` / `.rpm` (#37)

[`packaging/nfpm.yaml`](../packaging/nfpm.yaml) drives [nfpm](https://nfpm.goreleaser.com)
to build `.deb` and `.rpm` from the native Linux binary + the freedesktop
assets. The release CI job runs it on a tag (`HELENA_VERSION=${tag#v} nfpm pkg
--packager deb|rpm`) and uploads the packages alongside the archives. AppImage
and Flatpak are the natural next formats (they consume the same `.desktop` +
icon + metainfo); they're not yet wired into CI.

### Windows — installer + winget / Scoop (#38)

- [`packaging/windows/helena.iss`](../packaging/windows/helena.iss) — an Inno
  Setup script; build on a Windows runner with `iscc /DAppVersion=… /DSourceExe=…`
  to produce `helena-setup-<version>.exe`.
- [`packaging/scoop/helena.json`](../packaging/scoop/helena.json) — a Scoop
  manifest (with `autoupdate`) pointing at the release `.zip`; submit to a Scoop
  bucket, or fill `version`/`hash` per release.
- [`packaging/winget/IDCT.Helena.installer.yaml`](../packaging/winget/IDCT.Helena.installer.yaml)
  — a winget installer-manifest template; fill version + URL + SHA-256 and submit
  to `microsoft/winget-pkgs`.

> These Windows configs are committed but not yet wired into CI (the installer
> build + manifest submission are manual/per-release for now), and the whole
> packaging path is first exercised on a real release tag — expect a round of
> iteration there.

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
