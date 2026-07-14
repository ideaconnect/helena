# Helena MSIX package (Microsoft Store)

Scaffold for packaging Helena as an **MSIX** and submitting it to the
**Microsoft Store**. The end-to-end store walkthrough (account, pricing,
revenue split, submission) lives in
[docs/PACKAGING.md → Store distribution & monetization](../../../docs/PACKAGING.md#store-distribution--monetization);
this file is the mechanical "how to build the package" reference.

## Files

| File | Purpose |
| ---- | ------- |
| `AppxManifest.xml` | Package manifest. The product identity is baked in (below); only `@VERSION@` / `@ARCH@` are build-time tokens. Full-trust Win32 app (`Windows.FullTrustApplication` + `runFullTrust`). |
| `build-store-bundle.ps1` | One-shot orchestrator: builds both arches and combines them into `dist\helena.msixbundle`. What CI runs, and the easiest local entry point. |
| `build-msix.ps1` | Lower-level: stages one arch's exe + assets, stamps version/arch, builds `resources.pri` with `makepri`, runs `makeappx pack`, optionally self-signs for local testing. |
| `Assets/` | Store logo PNGs (Square 44/71/150/310, Wide 310x150, StoreLogo 50, SplashScreen 620x300), generated from `assets/app_icon.png`. Includes `Square44x44Logo.targetsize-{16,24,32,48,256}[_altform-unplated].png` — the **unplated** variants give the taskbar/Start icon no background plate. Without them Windows draws the icon on the system-accent plate (the blue background from issue #182). These qualified variants only take effect once indexed into `resources.pri`. |

## Product identity (from Partner Center)

Baked into `AppxManifest.xml`; must match Partner Center exactly. For reference:

| Field | Value |
| ----- | ----- |
| Package/Identity/Name | `IdeaConnectBartoszPachoek.HelenaAPIClient` |
| Package/Identity/Publisher | `CN=776E87F3-6B20-4A52-B4D8-AA515F574757` |
| Package/Properties/DisplayName | `Helena API Client` — must match a **reserved** app name; `Helena` alone is not reserved. (The on-device tile name stays `Helena`.) |
| PublisherDisplayName | `IDCT Bartosz Pachołek` |
| Package Family Name | `IdeaConnectBartoszPachoek.HelenaAPIClient_aec6mzqn7e0rm` |
| Store ID | `9NWPKK6CTDR1` |
| Store page (live) | <https://apps.microsoft.com/detail/9NWPKK6CTDR1> |

## Prerequisites

- The **Windows SDK** (`makeappx.exe`, `makepri.exe`, and `signtool.exe` for
  `-Sign`). The script finds them under `Windows Kits\10\bin` or on `PATH`; a
  *Developer Command Prompt for VS* has them ready.
- Prebuilt Helena executables for each arch, built with the release flags in
  [docs/BUILDING.md](../../../docs/BUILDING.md#release-grade-build):
  `dist\helena-windows-amd64.exe` and `dist\helena-windows-arm64.exe`.

## Build in CI (recommended)

When you **publish a GitHub Release**, the `msix` job in
[`.github/workflows/ci.yml`](../../../.github/workflows/ci.yml) builds the bundle
for you and uploads it as a workflow artifact named **`helena-store-msixbundle`**.
Download it from that run's summary page and upload it in Partner Center — no
local Windows SDK needed. (The bundle is unsigned; Microsoft signs it.)

**Regenerate without a new release.** To rebuild the bundle for an
already-released version — e.g. after Partner Center rejects a submission and
you fix the manifest — run the CI workflow manually (Actions → CI → *Run
workflow*) with the **`version`** input set to that release (e.g. `0.5.0`), or
`gh workflow run ci.yml -f version=0.5.0`. The `msix` job rebuilds and re-uploads
`helena-store-msixbundle` without cutting another GitHub Release.

## Build locally

One command builds both arches and the bundle:

```powershell
.\build-store-bundle.ps1 `
  -Amd64Exe ..\..\..\dist\helena-windows-amd64.exe `
  -Arm64Exe ..\..\..\dist\helena-windows-arm64.exe `
  -Version 0.4.0    # 3- or 4-part, with or without a leading "v"; padded to x.y.z.0
```

That writes `dist\helena.msixbundle`. To drive a single architecture (or
self-sign for side-load testing), call the lower-level `build-msix.ps1` directly:

```powershell
.\build-msix.ps1 -ExePath ..\..\..\dist\helena-windows-amd64.exe -Version 0.4.0.0 -Arch x64
.\build-msix.ps1 -ExePath ..\..\..\dist\helena-windows-arm64.exe -Version 0.4.0.0 -Arch arm64
makeappx bundle /d ..\..\..\dist\msix-bundle-input /p ..\..\..\dist\helena.msixbundle /bv 0.4.0.0
```

The **package version**'s final part must be `0` (e.g. `0.4.0.0`) — Microsoft
reserves it for Store repackaging. Keep the first three parts in step with
[`FyneApp.toml`](../../../FyneApp.toml).

## Test locally

The Store re-signs your package, so a Store-signed build can't run until it's
installed from the Store. To smoke-test the packaged app first, self-sign:

```powershell
.\build-msix.ps1 ... -Arch x64 -Sign
```

Then trust the generated test certificate (export its public key, import into
`Cert:\LocalMachine\TrustedPeople` as admin) and `Add-AppxPackage dist\helena-x64.msix`.
The self-signed cert is for local testing only — never for the submission.

## Submit

Upload `helena.msixbundle` in Partner Center, set a minimal price + markets,
complete the age rating, add screenshots + description, and set the privacy
policy to <https://idct.tech/helena/privacy/>. Microsoft signs and hosts the
package and delivers updates automatically. Full checklist in
[docs/PACKAGING.md](../../../docs/PACKAGING.md#windows--microsoft-store-msix).

> The `.msixbundle` is built automatically by the `msix` CI job on every
> published (non-pre-release) GitHub Release. Only the Partner Center submission
> itself stays manual — identity verification, name reservation, and upload need
> the owner's Microsoft account and can't be scripted.
