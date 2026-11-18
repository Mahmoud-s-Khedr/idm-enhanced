$ErrorActionPreference = 'Stop'
$install = Join-Path $env:RUNNER_TEMP 'idm-installed'
$setup = (Get-ChildItem build/packages/*-setup.exe).FullName
$p = Start-Process $setup -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART',"/DIR=`"$install`"") -Wait -PassThru
if ($p.ExitCode -ne 0) { throw "Install failed: $($p.ExitCode)" }
$hostExe = Join-Path $install 'native-host.exe'
if (!(Test-Path $hostExe)) { throw 'Native host missing' }
& $hostExe --register
if ($LASTEXITCODE -ne 0) { throw 'Registration failed' }
foreach ($browser in @('Mozilla','Google\Chrome','Chromium')) {
 $key = "HKCU:\Software\$browser\NativeMessagingHosts\com.idmenhanced.desktop"
 $manifest = (Get-Item $key).GetValue('')
 if (!(Test-Path $manifest)) { throw "Missing manifest for $browser" }
 $m = Get-Content $manifest | ConvertFrom-Json
 if ($m.path -ne $hostExe) { throw 'Wrong native host path' }
}
$state = Join-Path $env:APPDATA 'idm-enhanced'
New-Item -ItemType Directory -Force $state | Out-Null
Set-Content (Join-Path $state 'package-smoke-marker') 'preserve'
$p = Start-Process (Join-Path $install 'unins000.exe') -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART') -Wait -PassThru
if ($p.ExitCode -ne 0) { throw "Uninstall failed: $($p.ExitCode)" }
if (Test-Path $hostExe) { throw 'Uninstall retained executable' }
if (Test-Path 'HKCU:\Software\Mozilla\NativeMessagingHosts\com.idmenhanced.desktop') { throw 'Uninstall retained registry entry' }
if (!(Test-Path (Join-Path $state 'package-smoke-marker'))) { throw 'Uninstall deleted user state' }
Write-Output 'PASS setup, user host registration, uninstall and data retention'
