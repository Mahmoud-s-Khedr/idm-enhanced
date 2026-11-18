#ifndef SourceRoot
  #error SourceRoot must point to the repository
#endif
#ifndef AppVersion
  #error AppVersion must be supplied by tools/package
#endif
[Setup]
AppId=IDMEnhancedAlpha
AppName=IDM Enhanced
AppVersion={#AppVersion}
AppPublisher=IDM Enhanced contributors
DefaultDirName={localappdata}\Programs\IDM Enhanced
DefaultGroupName=IDM Enhanced
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
OutputBaseFilename=idm-enhanced-{#AppVersion}-windows-amd64-setup
Compression=lzma2
SolidCompression=yes
UninstallDisplayIcon={app}\desktop.exe
CloseApplications=yes
LicenseFile={#SourceRoot}\LICENSE
[Files]
Source: "{#SourceRoot}\build\desktop.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceRoot}\build\native-host.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceRoot}\build\downloader.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#SourceRoot}\LICENSE"; DestDir: "{app}"
[Icons]
Name: "{group}\IDM Enhanced"; Filename: "{app}\desktop.exe"
[Run]
Filename: "{app}\native-host.exe"; Parameters: "--register"; Flags: runhidden waituntilterminated
[UninstallRun]
Filename: "{app}\native-host.exe"; Parameters: "--unregister"; Flags: runhidden waituntilterminated; RunOnceId: "RemoveNativeHost"
