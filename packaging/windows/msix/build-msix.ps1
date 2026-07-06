<#
.SYNOPSIS
  Build a Microsoft Store MSIX package for Helena from a prebuilt Windows .exe.

.DESCRIPTION
  Stages the executable + Store logo assets, substitutes the Partner Center
  identity values into AppxManifest.xml, and runs makeappx.exe to produce an
  .msix. Optionally self-signs the package for local side-load testing.

  Run one invocation per architecture (x64, arm64), then combine the resulting
  .msix files into a single .msixbundle with:

    makeappx bundle /d <dir-with-both-msix> /p dist\helena.msixbundle

  Requires the Windows SDK (makeappx.exe, and signtool.exe if -Sign). Open a
  "Developer Command Prompt for VS" or ensure the SDK bin dir is on PATH.

.PARAMETER ExePath
  Path to the prebuilt Helena executable for this architecture
  (e.g. dist\helena-windows-amd64.exe). Built with the release flags from
  docs/BUILDING.md (-tags no_emoji -H windowsgui ...).

.PARAMETER IdentityName
  Package/Identity/Name from Partner Center (the reserved product identity).

.PARAMETER Publisher
  Package/Identity/Publisher from Partner Center (a "CN=..." string). For a
  self-signed test build (-Sign) this MUST match the certificate subject.

.PARAMETER PublisherDisplayName
  The human-readable publisher name shown in the Store (e.g. "IDCT").

.PARAMETER Version
  Four-part package version, e.g. 0.4.0.0. The Store requires the final part to
  be 0 (Microsoft reserves it for its own repackaging).

.PARAMETER Arch
  Target architecture: x64 or arm64. Stamped into the manifest and the output
  filename.

.PARAMETER Sign
  Also create a self-signed cert and sign the package for local side-load
  testing. Never used for the actual Store submission (Microsoft re-signs).

.EXAMPLE
  .\build-msix.ps1 -ExePath ..\..\..\dist\helena-windows-amd64.exe `
    -IdentityName 1234ABCD.Helena -Publisher "CN=ABCD1234-..." `
    -PublisherDisplayName IDCT -Version 0.4.0.0 -Arch x64
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)] [string] $ExePath,
  [Parameter(Mandatory = $true)] [string] $IdentityName,
  [Parameter(Mandatory = $true)] [string] $Publisher,
  [string] $PublisherDisplayName = "IDCT",
  [Parameter(Mandatory = $true)] [string] $Version,
  [Parameter(Mandatory = $true)] [ValidateSet("x64", "arm64")] [string] $Arch,
  [switch] $Sign
)

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot  = Resolve-Path (Join-Path $scriptDir "..\..\..")

if (-not (Test-Path $ExePath)) { throw "ExePath not found: $ExePath" }
if ($Version -notmatch '^\d+\.\d+\.\d+\.\d+$') {
  throw "Version must be four-part (e.g. 0.4.0.0); got '$Version'"
}

function Resolve-SdkTool([string] $name) {
  $cmd = Get-Command $name -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  $roots = @("${env:ProgramFiles(x86)}\Windows Kits\10\bin", "${env:ProgramFiles}\Windows Kits\10\bin")
  foreach ($root in $roots) {
    if (Test-Path $root) {
      $found = Get-ChildItem -Path $root -Recurse -Filter $name -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -match "\\x64\\" } | Sort-Object FullName -Descending | Select-Object -First 1
      if ($found) { return $found.FullName }
    }
  }
  throw "$name not found. Install the Windows SDK or run from a Developer Command Prompt."
}

$makeappx = Resolve-SdkTool "makeappx.exe"

# --- Stage the payload ---------------------------------------------------
$staging = Join-Path $repoRoot "dist\msix-$Arch"
if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
New-Item -ItemType Directory -Force -Path $staging | Out-Null

Copy-Item $ExePath (Join-Path $staging "helena.exe")
Copy-Item (Join-Path $scriptDir "Assets") (Join-Path $staging "Assets") -Recurse

# --- Substitute identity tokens into the manifest ------------------------
$manifest = Get-Content (Join-Path $scriptDir "AppxManifest.xml") -Raw
$manifest = $manifest.
  Replace("@IDENTITY_NAME@", $IdentityName).
  Replace("@PUBLISHER@", $Publisher).
  Replace("@PUBLISHER_DISPLAY_NAME@", $PublisherDisplayName).
  Replace("@VERSION@", $Version).
  Replace("@ARCH@", $Arch)
Set-Content -Path (Join-Path $staging "AppxManifest.xml") -Value $manifest -Encoding UTF8

# --- Pack ----------------------------------------------------------------
$outDir = Join-Path $repoRoot "dist"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$msix = Join-Path $outDir "helena-$Arch.msix"
if (Test-Path $msix) { Remove-Item -Force $msix }

Write-Host "Packing $msix ..."
& $makeappx pack /d $staging /p $msix /o
if ($LASTEXITCODE -ne 0) { throw "makeappx failed ($LASTEXITCODE)" }

# --- Optional self-sign for local side-load testing ----------------------
if ($Sign) {
  $signtool = Resolve-SdkTool "signtool.exe"
  $cert = New-SelfSignedCertificate -Type Custom -Subject $Publisher `
    -KeyUsage DigitalSignature -FriendlyName "Helena MSIX test" `
    -CertStoreLocation "Cert:\CurrentUser\My" `
    -TextExtension @("2.5.29.37={text}1.3.6.1.5.5.7.3.3", "2.5.29.19={text}")
  Write-Host "Self-signing with test cert $($cert.Thumbprint) (subject must equal Publisher)."
  & $signtool sign /fd SHA256 /a /sha1 $cert.Thumbprint $msix
  if ($LASTEXITCODE -ne 0) { throw "signtool failed ($LASTEXITCODE)" }
  Write-Host "To side-load: export the cert's public key, Import-Certificate it into"
  Write-Host "Cert:\LocalMachine\TrustedPeople (admin), then Add-AppxPackage $msix."
}

Write-Host ""
Write-Host "Built $msix"
Write-Host "Next: build the other arch, then 'makeappx bundle' both into a .msixbundle,"
Write-Host "and upload it in Partner Center. Do NOT sign the Store submission - Microsoft signs it."
