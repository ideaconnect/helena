; Inno Setup script for the Helena Windows installer.
; Build (on a Windows runner with Inno Setup installed):
;   iscc /DAppVersion=%HELENA_VERSION% /DSourceExe=dist\helena-windows-amd64.exe packaging\windows\helena.iss
; Produces dist\helena-setup-<version>.exe.

#ifndef AppVersion
  #define AppVersion "0.0.0"
#endif
#ifndef SourceExe
  #define SourceExe "dist\helena-windows-amd64.exe"
#endif

[Setup]
; A stable, Helena-specific upgrade GUID (do not change between versions).
AppId={{B2E9F1A4-7C3D-4E58-9A1B-6F0D2C8E5A47}
AppName=Helena
AppVersion={#AppVersion}
AppPublisher=IDCT
AppPublisherURL=https://github.com/ideaconnect/helena
DefaultDirName={autopf}\Helena
DefaultGroupName=Helena
UninstallDisplayIcon={app}\helena.exe
Compression=lzma2
SolidCompression=yes
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputDir=dist
OutputBaseFilename=helena-setup-{#AppVersion}
WizardStyle=modern

[Files]
Source: "{#SourceExe}"; DestDir: "{app}"; DestName: "helena.exe"; Flags: ignoreversion
Source: "..\..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\README.md"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\Helena"; Filename: "{app}\helena.exe"
Name: "{autodesktop}\Helena"; Filename: "{app}\helena.exe"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "Create a &desktop shortcut"; GroupDescription: "Additional icons:"

[Run]
Filename: "{app}\helena.exe"; Description: "Launch Helena"; Flags: nowait postinstall skipifsilent
