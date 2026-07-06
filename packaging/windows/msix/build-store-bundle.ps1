<#
.SYNOPSIS
  Build the Microsoft Store submission artifact: a multi-arch .msixbundle for Helena.

.DESCRIPTION
  Orchestrates build-msix.ps1 for x64 and arm64 from prebuilt Helena executables,
  then combines the two .msix into dist\helena.msixbundle with makeappx. The
  bundle is UNSIGNED - Microsoft signs Store submissions - and is what you upload
  in Partner Center.

  This is what the `msix` CI job runs on a published release; it is equally
  runnable locally. Requires the Windows SDK (makeappx.exe).

.PARAMETER Amd64Exe
  Path to the prebuilt Windows amd64 executable (e.g. dist\helena-windows-amd64.exe).

.PARAMETER Arm64Exe
  Path to the prebuilt Windows arm64 executable (e.g. dist\helena-windows-arm64.exe).

.PARAMETER Version
  Release version, 3- or 4-part, with or without a leading "v" (e.g. v0.4.0,
  0.4.0, or 0.4.0.0). Normalised to the Store's required 4-part form with the
  revision (4th part) forced to 0.

.EXAMPLE
  .\build-store-bundle.ps1 -Amd64Exe ..\..\..\dist\helena-windows-amd64.exe `
    -Arm64Exe ..\..\..\dist\helena-windows-arm64.exe -Version v0.4.0
#>
[CmdletBinding()]
param(
  [Parameter(Mandatory = $true)] [string] $Amd64Exe,
  [Parameter(Mandatory = $true)] [string] $Arm64Exe,
  [Parameter(Mandatory = $true)] [string] $Version
)

$ErrorActionPreference = "Stop"
$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$repoRoot  = Resolve-Path (Join-Path $scriptDir "..\..\..")

# Normalise to a 4-part package version; the Store reserves the 4th part (0).
$parts = $Version.TrimStart('v').Split('.')
if ($parts.Count -lt 3 -or $parts.Count -gt 4) {
  throw "Version must be 3- or 4-part (e.g. 0.4.0); got '$Version'"
}
while ($parts.Count -lt 4) { $parts += '0' }
$parts[3] = '0'
$pkgVersion = ($parts -join '.')

function Resolve-SdkTool([string] $name) {
  $cmd = Get-Command $name -ErrorAction SilentlyContinue
  if ($cmd) { return $cmd.Source }
  foreach ($root in @("${env:ProgramFiles(x86)}\Windows Kits\10\bin", "${env:ProgramFiles}\Windows Kits\10\bin")) {
    if (Test-Path $root) {
      $found = Get-ChildItem -Path $root -Recurse -Filter $name -ErrorAction SilentlyContinue |
        Where-Object { $_.FullName -match "\\x64\\" } | Sort-Object FullName -Descending | Select-Object -First 1
      if ($found) { return $found.FullName }
    }
  }
  throw "$name not found. Install the Windows SDK or run from a Developer Command Prompt."
}
$makeappx = Resolve-SdkTool "makeappx.exe"

# Per-arch packages (build-msix.ps1 writes dist\helena-<arch>.msix).
& (Join-Path $scriptDir "build-msix.ps1") -ExePath $Amd64Exe -Version $pkgVersion -Arch x64
& (Join-Path $scriptDir "build-msix.ps1") -ExePath $Arm64Exe -Version $pkgVersion -Arch arm64

# Collect the two .msix into a clean input dir and bundle them.
$bundleInput = Join-Path $repoRoot "dist\msix-bundle-input"
if (Test-Path $bundleInput) { Remove-Item -Recurse -Force $bundleInput }
New-Item -ItemType Directory -Force -Path $bundleInput | Out-Null
Copy-Item (Join-Path $repoRoot "dist\helena-x64.msix")   $bundleInput
Copy-Item (Join-Path $repoRoot "dist\helena-arm64.msix") $bundleInput

$bundle = Join-Path $repoRoot "dist\helena.msixbundle"
if (Test-Path $bundle) { Remove-Item -Force $bundle }

Write-Host "Bundling $bundle (version $pkgVersion) ..."
& $makeappx bundle /d $bundleInput /p $bundle /bv $pkgVersion /o
if ($LASTEXITCODE -ne 0) { throw "makeappx bundle failed ($LASTEXITCODE)" }

Write-Host ""
Write-Host "Built $bundle (unsigned - Microsoft signs Store submissions)."
Write-Host "Upload it in Partner Center; do NOT sign it yourself."
