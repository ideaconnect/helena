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
| `build-msix.ps1` | Stages the exe + assets, stamps version/arch into the manifest, runs `makeappx`, optionally self-signs for local testing. Run once per architecture. |
| `Assets/` | Store logo PNGs (Square 44/71/150/310, Wide 310x150, StoreLogo 50, SplashScreen 620x300), generated from `assets/app_icon.png`. |

## Product identity (from Partner Center)

Baked into `AppxManifest.xml`; must match Partner Center exactly. For reference:

| Field | Value |
| ----- | ----- |
| Package/Identity/Name | `IdeaConnectBartoszPachoek.HelenaAPIClient` |
| Package/Identity/Publisher | `CN=776E87F3-6B20-4A52-B4D8-AA515F574757` |
| PublisherDisplayName | `IDCT Bartosz Pachołek` |
| Package Family Name | `IdeaConnectBartoszPachoek.HelenaAPIClient_aec6mzqn7e0rm` |
| Store ID | `9NWPKK6CTDR1` |
| Store page (once live) | `https://apps.microsoft.com/detail/9NWPKK6CTDR1` |

## Prerequisites

- The **Windows SDK** (`makeappx.exe`, and `signtool.exe` for `-Sign`). The
  script finds them under `Windows Kits\10\bin` or on `PATH`; a
  *Developer Command Prompt for VS* has them ready.
- Prebuilt Helena executables for each arch, built with the release flags in
  [docs/BUILDING.md](../../../docs/BUILDING.md#release-grade-build):
  `dist\helena-windows-amd64.exe` and `dist\helena-windows-arm64.exe`.

## Build

Run once per architecture, then bundle:

```powershell
# amd64
.\build-msix.ps1 -ExePath ..\..\..\dist\helena-windows-amd64.exe -Version 0.4.0.0 -Arch x64

# arm64
.\build-msix.ps1 -ExePath ..\..\..\dist\helena-windows-arm64.exe -Version 0.4.0.0 -Arch arm64

# combine both into one submission artifact
makeappx bundle /d ..\..\..\dist\msix-bundle-input /p ..\..\..\dist\helena.msixbundle
```

(For the bundle step, drop `helena-x64.msix` and `helena-arm64.msix` into a
folder — e.g. `dist\msix-bundle-input\` — and point `makeappx bundle` at it.)

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

> Not yet wired into release CI — the MSIX build is manual per release for now.
> The identity-verification and name-reservation steps need the owner's
> Microsoft account and can't be scripted.
