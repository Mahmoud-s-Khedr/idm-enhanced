$ErrorActionPreference = 'Stop'
$install = Join-Path $env:RUNNER_TEMP 'idm-installed'
$setup = (Get-ChildItem build/packages/*-setup.exe).FullName
$predecessor = (Get-ChildItem build/predecessor/*-setup.exe).FullName
$p = Start-Process $predecessor -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART',"/DIR=`"$install`"") -Wait -PassThru
if ($p.ExitCode -ne 0) { throw "Predecessor install failed: $($p.ExitCode)" }
$fixtureState = Join-Path $env:RUNNER_TEMP 'idm-upgrade-state'
$fixtureDest = Join-Path $env:RUNNER_TEMP 'idm-retained.bin'
$cli = Join-Path $install 'downloader.exe'
$seed = & $cli --state-dir $fixtureState enqueue -o $fixtureDest https://example.invalid/retained 2>&1
if ($LASTEXITCODE -ne 0) { throw 'Could not seed durable upgrade job' }
if (($seed -join "`n") -notmatch '^Job: ([0-9a-f]{32})$') { throw 'Could not identify seeded upgrade job' }
$jobId = $Matches[1]
$before = & $cli --state-dir $fixtureState status $jobId 2>&1
if ($LASTEXITCODE -ne 0) { throw 'Could not read predecessor jobs' }
if (($before -join "`n") -notmatch "^$jobId state=queued durable=0 ") { throw 'Seeded queued job missing before upgrade' }
$p = Start-Process $setup -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART',"/DIR=`"$install`"") -Wait -PassThru
if ($p.ExitCode -ne 0) { throw "Install failed: $($p.ExitCode)" }
$after = & $cli --state-dir $fixtureState status $jobId 2>&1
if ($LASTEXITCODE -ne 0 -or ($before -join "`n") -ne ($after -join "`n")) { throw 'Upgrade changed durable jobs' }
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
Write-Output 'PASS setup, controlled version upgrade with durable queue retention, user host registration, uninstall and data retention'
