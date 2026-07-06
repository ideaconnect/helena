# Packaging & distribution

Status and conventions for shipping Helena. App identity/version come from
[`FyneApp.toml`](../FyneApp.toml) (consumed by `fyne package`); the app ID is
`tech.idct.helena`.

## Current distribution

CI builds native per-OS binaries on every push (no cross-compile):
`helena-linux-amd64`, `helena-windows-amd64.exe`, `helena-windows-arm64.exe`,
`helena-darwin-arm64`. Binaries embed their version (`helena --version`). The
Windows-on-ARM binary is built on the `windows-11-arm` runner with llvm-mingw's
native aarch64 cgo toolchain (the runner's stock gcc is x86-64 and can't
assemble arm64 cgo) and is tested there without `-race` (unsupported on
windows/arm64; the amd64 leg covers the race suite).

Release binaries (and `make build` / `make.bat build`) are built with
**`-tags no_emoji`**, which drops Fyne's bundled 4.2 MB colour-emoji font. Fyne
parses that font fresh per theme scope, so excluding it cuts resident memory by
75 MB (326 → 251 MB when it landed) and shrinks the binary by ~4 MB; response
text still renders — colour-emoji glyphs come out blank. The later
theme-scope reduction (11 → 3 scopes) brought the same software-GL box to
**~200 MB resident** total. A plain `go build` /
`go run` keeps emoji for development.

The free GitHub Release is the primary distribution channel. **Publishing a
GitHub Release** (Releases -> Draft a new release -> pick or create a `v*` tag,
write the notes, Publish) triggers CI: it runs the full build/test matrix on
every platform and then attaches these assets to that release (issues #27/#35):

- **Archives** — `helena-linux-amd64.tar.gz`, `helena-darwin-arm64.tar.gz`,
  `helena-windows-amd64.zip`, `helena-windows-arm64.zip` (each bundling the
  binary + `LICENSE` + `README.md`).
- **Linux packages** — `helena_<version>_amd64.deb` and
  `helena-<version>.x86_64.rpm` (nfpm, from the same binary + freedesktop assets).
- **`SHA256SUMS`** — SHA-256 checksums over every asset.
- **`helena.sbom.spdx.json`** — an SPDX software bill of materials.
- **Provenance attestation** — a keyless (Sigstore) build-provenance
  attestation for the archives + packages, verifiable with `gh attestation verify`.

CI attaches assets to the release you published; it does not author the release
or its notes (you write those in the UI, using its "Generate release notes"
button if you like). Asset upload is gated on the full test matrix passing, so a
release never ships binaries from a commit that fails tests.

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
assets. The release CI job runs it when a GitHub Release is published
(`HELENA_VERSION=${tag#v} nfpm pkg --packager deb|rpm`) and uploads the packages
alongside the archives. AppImage
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

## Store distribution & monetization

Helena is and stays free to **build from source** (see
[BUILDING.md](BUILDING.md)) and free to download from GitHub Releases. Store
listings are an optional, paid-but-cheap convenience channel: a one-click
install, auto-updates, and — for those who want to support the project — a
minimal price. This section is the how-to for each store, and the honest state
of monetization per platform.

### Windows — Microsoft Store (MSIX)

The Microsoft Store is the recommended paid channel: as of 2026 it is the most
favourable of the major app stores for a small indie tool.

**Why it's worth it (2026 terms):**

- **Registration is free.** Microsoft waived the one-time individual developer
  fee in late 2025 and the company fee in May 2026; new individual accounts
  verify with a government ID + selfie instead of a card. (Historically this was
  a US$19 one-time fee — budget for it only if you register in a market where
  the new free-onboarding flow hasn't rolled out yet.)
- **Microsoft signs, hosts, and updates for you.** Submit an MSIX and the Store
  code-signs it for free (no code-signing certificate to buy), hosts the binary
  on its CDN, and pushes updates to users automatically. That removes the two
  biggest Windows-distribution costs — a signing cert and an update mechanism —
  which Helena deliberately does not build itself (no runtime update check; see
  [Updates](#updates-package-manager-manual-no-phone-home-decided-2026-06-16)).
- **Revenue split favours you.** Using Microsoft's commerce you keep **85%**
  (Microsoft takes 15% for non-game apps); use your own commerce engine and you
  keep 100%. For a "minimal price" listing the 15% is negligible.

**Deployment path (step by step):**

1. **Create a Partner Center account** at
   <https://partner.microsoft.com/dashboard> → *Windows & Xbox* program, and
   complete identity verification.
2. **Reserve the app name** (e.g. `Helena`) under *Apps and games → New
   product*. Partner Center then assigns you three identity values — copy them:
   *Package/Identity/Name*, *Package/Identity/Publisher* (a `CN=…` string), and
   *Publisher display name*. The MSIX manifest must match these exactly or the
   Store rejects the upload.
3. **Build the Windows binaries** with the release flags (see
   [BUILDING.md](BUILDING.md#release-grade-build)) — one `helena-windows-amd64.exe`
   and, to cover Windows-on-ARM, one `helena-windows-arm64.exe`.
4. **Build the MSIX package(s)** with the committed scaffold in
   [`packaging/windows/msix/`](../packaging/windows/msix/) — an `AppxManifest.xml`
   template, the Store logo assets, and a `build-msix.ps1` that stamps your
   identity values and runs `makeappx`. Produce one `.msix` per architecture,
   then combine them into a single `.msixbundle` so one submission serves both.
   See that directory's [README](../packaging/windows/msix/README.md).
5. **Test the package locally** by self-signing it (the script's `-Sign` switch)
   and side-loading — the Store-signed build can't be run until it's installed
   from the Store, so a self-signed copy is how you smoke-test the packaged app.
6. **Create the submission** in Partner Center: upload the `.msixbundle`, set a
   **minimal price** and markets, complete the age rating (IARC) questionnaire,
   add screenshots (reuse `make screenshots` output) and a description, and point
   the privacy-policy field at <https://idct.tech/helena/privacy/>. Submit for
   certification.

> **Not yet automated.** The MSIX build is a documented manual/per-release step
> today; wiring `build-msix.ps1` into the release CI and automating submission
> via the Store Submission API is a follow-up. The identity-verification and
> name-reservation steps are inherently manual (they need the owner's Microsoft
> account) and can't be scripted from CI.

**Updates on the Store — automatic, out-of-process, no in-app updater (decided
2026-07-06).** MSIX packages installed from the Store are updated
**automatically by the Store client**: it checks for new package versions and
installs them silently in the background. Publishing a new version in Partner
Center is the whole update mechanism. This is done by the OS, not by Helena, so
it stays fully within the [no-phone-home guarantee](#updates-package-manager-manual-no-phone-home-decided-2026-06-16):
Helena still ships **no runtime update check**. We deliberately do **not** add an
in-app "check for updates" call or a Store "Update" button — an app-level
background check would violate that guarantee, and a manual Store deep-link would
be redundant with the Store's own auto-update (and could only be wired up once a
listing exists, since it needs the Store product ID). If that ever changes, the
only privacy-safe shape is a *user-initiated* button that opens the Store product
page via `OpenURL` on click, gated to the Store build alone.

### Linux — monetization is weak; pick reach + donations

There is **no dominant paid Linux storefront**, and Linux users overwhelmingly
expect software to be free and buildable from source. A hard paywall on Linux
mostly routes people to the (free, documented) source build. The realistic
options, best-first for Helena:

| Channel | Monetization | Reality in 2026 | Fit for Helena |
| ------- | ------------ | --------------- | -------------- |
| **[itch.io](https://itch.io)** | Fixed **or** "pay what you want" (set a minimum, e.g. \$2); configurable revenue share (default 10%) | Works today, cross-platform, no distro lock-in, one-click paid download | **Best if you want an actual price tag on Linux.** Upload the `.tar.gz` / AppImage, set a minimum price. |
| **[Flathub](https://flathub.org)** | None yet (donation links + developer verification only) | The default cross-distro store; maximum reach; paid apps have been "coming" since 2023 but are **not live** as of mid-2026 | **Best for reach.** Ship a Flatpak (it reuses the same `.desktop` + icon + AppStream metainfo already in `packaging/linux/`), free, with your Sponsor/Buy-me-a-coffee links attached. |
| **[elementary AppCenter](https://appcenter.elementary.io)** | "Pay what you want" (suggested price; users may pay \$0) | Real and has paid developers, but elementary-OS-specific and Flatpak-based via a reviewed GitHub submission | Optional extra reach into the elementary audience; price is not enforced. |
| **Snap Store** | No meaningful third-party paid model | Free distribution only | Skip for monetization. |
| **GitHub Sponsors + Buy Me a Coffee** | Direct donations | Already wired into the website nav/footer | **Already done** — this is Helena's primary Linux "monetization" and needs no store. |

**Recommendation:** don't gate Linux behind a store paywall. Publish a **free
Flathub** build for reach, keep the existing **GitHub Sponsors + Buy Me a
Coffee** links as the donation path, and — if you want a genuine one-click paid
download on Linux — put a "pay what you want, suggested \$X" listing on
**itch.io**. That captures willing payers without alienating the audience or
fragmenting into a distro-specific store.

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
