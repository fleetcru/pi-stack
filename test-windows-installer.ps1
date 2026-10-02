# No real installation, scheduled task, registry, or Pi process is changed.
$ErrorActionPreference = 'Stop'
# A parent PowerShell 7 can put its modules first in Windows PowerShell's path.
Import-Module (Join-Path $PSHOME 'Modules\Microsoft.PowerShell.Utility\Microsoft.PowerShell.Utility.psd1') -Force
. (Join-Path $PSScriptRoot 'windows-installer-common.ps1')
. (Join-Path $PSScriptRoot 'dev-launcher-common.ps1')
function Assert-Test($Condition, $Message) { if (-not $Condition) { throw $Message } }
function Assert-Rejected([scriptblock]$Action, [string]$Message) {
    $rejected = $false
    try { & $Action | Out-Null } catch { $rejected = $true }
    Assert-Test $rejected $Message
}
function Read-ScriptAst([string]$Path) {
    $tokens = $null; $errors = $null
    $ast = [Management.Automation.Language.Parser]::ParseFile($Path, [ref]$tokens, [ref]$errors)
    Assert-Test ($errors.Count -eq 0) "Syntax errors in $Path, $errors"
    return $ast
}
$temp = Join-Path ([IO.Path]::GetTempPath()) "pi-installer-test-$([guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $temp | Out-Null
try {
    $paths = @('install-server-user.ps1', 'install-server.ps1', 'install-exp-external-bridge.ps1', 'start-exp-server.ps1', 'start-exp-live-stack.ps1', 'stop-exp-live-stack.ps1', 'dev-launcher-common.ps1', 'pi-server-tray/install.ps1', 'pi-server-tray/uninstall.ps1', 'pi-server-exp/scripts/install-windows-service.ps1')
    foreach ($path in $paths) { Read-ScriptAst (Join-Path $PSScriptRoot $path) | Out-Null }
    # Verify command quoting against a real, harmless child PowerShell process.
    $marker = Join-Path $temp "space and ' quote.txt"
    $child = Start-DevPowerShell -Command "[IO.File]::WriteAllText($(Quote-DevPowerShell $marker), 'quoted correctly')" -WindowStyle Hidden
    Assert-Test ($child.WaitForExit(15000)) 'Encoded child PowerShell did not exit.'
    Assert-Test ([IO.File]::ReadAllText($marker) -eq 'quoted correctly') 'Encoded command corrupted its path.'
    $listener = New-Object Net.Sockets.TcpListener ([Net.IPAddress]::Loopback), 0
    $listener.Start()
    try { Assert-Rejected { Assert-DevPortAvailable $listener.LocalEndpoint.Port } 'Busy port was accepted.' }
    finally { $listener.Stop() }
    # Record native descendants as well as the wrapper, and prove PID reuse
    # cannot make stop.ps1 target an unrelated process.
    $childPidFile = Join-Path $temp 'child-pid.txt'
    $powerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    $command = "`$p = Start-Process -FilePath $(Quote-DevPowerShell $powerShell) -ArgumentList '-NoProfile -Command Start-Sleep -Seconds 30' -WindowStyle Hidden -PassThru; [IO.File]::WriteAllText($(Quote-DevPowerShell $childPidFile), [string]`$p.Id); Start-Sleep -Seconds 30"
    $tree = Start-DevPowerShell -Command $command -WindowStyle Hidden
    try {
        for ($attempt = 0; $attempt -lt 100 -and -not (Test-Path -LiteralPath $childPidFile); $attempt++) { Start-Sleep -Milliseconds 50 }
        Assert-Test (Test-Path -LiteralPath $childPidFile) 'Harmless test child did not start.'
        $descendant = Get-Process -Id ([int][IO.File]::ReadAllText($childPidFile))
        $records = @(Get-DevProcessRecords -Processes @($tree))
        Assert-Test (@($records | Where-Object { $_.id -eq $descendant.Id }).Count -eq 1) 'Descendant process was not recorded.'
        $roundTrip = ($records | ConvertTo-Json -Depth 5) | ConvertFrom-Json
        $record = $roundTrip | Where-Object { $_.id -eq $tree.Id }
        Assert-Test (Test-DevProcessRecord $tree $record) 'Process identity failed after JSON round trip.'
        Assert-Test (-not (Test-DevProcessRecord $tree @{ startedAt = '0' })) 'Reused PID identity was accepted.'
        $legacy = @{ startedAt = $tree.StartTime.ToUniversalTime().ToString('o') } | ConvertTo-Json | ConvertFrom-Json
        Assert-Test (Test-DevProcessRecord $tree $legacy) 'Legacy ISO date record was not recognized.'
    } finally { Stop-DevProcessTree $tree; [void]$tree.WaitForExit(10000) }
    Assert-Test ($descendant.WaitForExit(10000)) 'Test child survived process-tree cleanup.'
    $stopData = Join-Path $temp 'stop-record'
    New-Item -ItemType Directory -Path $stopData | Out-Null
    $fakeState = @{ buildId = [guid]::NewGuid().ToString('N'); processes = @(@{ id = $PID; startedAt = '0' }) }
    $statePath = Join-Path $stopData 'dev-stack.json'
    [IO.File]::WriteAllText($statePath, ($fakeState | ConvertTo-Json -Depth 5))
    & (Join-Path $PSScriptRoot 'stop-exp-live-stack.ps1') -DataDir $stopData -WhatIf
    Assert-Test (Test-Path -LiteralPath $statePath) 'Stack -WhatIf removed its record.'
    & (Join-Path $PSScriptRoot 'stop-exp-live-stack.ps1') -DataDir $stopData
    Assert-Test (-not (Test-Path -LiteralPath $statePath)) 'Stale process record was not removed.'

    Add-Type @'
using System.Net;
public sealed class InstallerTestResponse : WebResponse {
    public int StatusCode { get; private set; }
    public InstallerTestResponse(int status) { StatusCode = status; }
}
'@
    $global:InstallerTest = @{}
    # These functions shadow external services only in this test process.
    function Invoke-RestMethod {
        param($Uri, $TimeoutSec)
        if ($Uri -like '*/tags/server-dev') {
            if ($global:InstallerTest.httpStatus) {
                $response = New-Object InstallerTestResponse $global:InstallerTest.httpStatus
                throw (New-Object Net.WebException 'fixture HTTP failure', $null, ([Net.WebExceptionStatus]::ProtocolError), $response)
            }
            return [PSCustomObject]@{ draft = $false; tag_name = 'server-dev' }
        }
        $global:InstallerTest.pages++
        if ($global:InstallerTest.paginated -and $Uri -like '*page=1') {
            $items = @(1..100 | ForEach-Object { [PSCustomObject]@{ tag_name = "v1.0.$_"; draft = $false; prerelease = $false } })
            Write-Output -NoEnumerate $items
            return
        }
        Write-Output -NoEnumerate @(
            [PSCustomObject]@{ tag_name = 'v99.0.0'; draft = $false; prerelease = $false },
            [PSCustomObject]@{ tag_name = 'tray-v99.0.0'; draft = $false; prerelease = $false },
            [PSCustomObject]@{ tag_name = 'server-v0.9.0'; draft = $false; prerelease = $false },
            [PSCustomObject]@{ tag_name = 'server-v0.10.0'; draft = $false; prerelease = $false },
            [PSCustomObject]@{ tag_name = 'server-v2.0.0'; draft = $true; prerelease = $false },
            [PSCustomObject]@{ tag_name = 'server-v3.0.0-rc.1'; draft = $false; prerelease = $true }
        )
    }
    function Invoke-WebRequest {
        param($Uri, $OutFile, [switch]$UseBasicParsing, $TimeoutSec)
        if ($Uri -like '*/healthz') { return [PSCustomObject]@{ StatusCode = 200 } }
        $global:InstallerTest.downloads++
        if ($global:InstallerTest.failure -eq 'download') { throw 'fixture download failure' }
        if ($Uri -like '*/SHA256SUMS') {
            $hash = (Get-FileHash -LiteralPath (Join-Path (Split-Path $OutFile) 'pi-server.exe')).Hash
            if ($global:InstallerTest.failure -eq 'checksum') { $hash = '0' * 64 }
            [IO.File]::WriteAllText($OutFile, "$hash  pi-server-windows-amd64.exe`n")
        } else { [IO.File]::WriteAllBytes($OutFile, [byte[]](5, 6, 7, 8)) }
    }
    function Get-Command { param($Name); return [PSCustomObject]@{ Source = 'C:\fixture\pi.exe' } }
    function Get-ScheduledTask {
        param($TaskName)
        if ($global:InstallerTest.taskExists) {
            $argument = '-File "' + (Join-Path $global:InstallerTest.root 'start.ps1') + '"'
            return [PSCustomObject]@{ State = $global:InstallerTest.taskState; Actions = @([PSCustomObject]@{ Execute = 'powershell.exe'; Arguments = $argument }) }
        }
    }
    function Export-ScheduledTask { return '<fixture-original-task />' }
    function Stop-ScheduledTask { $global:InstallerTest.stops++; $global:InstallerTest.taskState = 'Ready' }
    function Register-ScheduledTask {
        param($TaskName, $Xml)
        if ($Xml) { $global:InstallerTest.restored = $true }
        elseif ($global:InstallerTest.failure -eq 'register') { throw 'fixture registration failure' }
        $global:InstallerTest.taskExists = $true
    }
    function Unregister-ScheduledTask { $global:InstallerTest.taskExists = $false }
    function Start-ScheduledTask {
        $global:InstallerTest.starts++
        $global:InstallerTest.started = $true
        $global:InstallerTest.taskState = if ($global:InstallerTest.failure -eq 'startup' -and -not $global:InstallerTest.restored) { 'Ready' } else { 'Running' }
    }
    function New-ScheduledTaskAction { return 'fixture action' }
    function New-ScheduledTaskTrigger { return 'fixture trigger' }
    function New-ScheduledTaskSettingsSet { return 'fixture settings' }
    function New-ScheduledTaskPrincipal { return 'fixture principal' }
    function Start-Sleep { }
    function Get-NetTCPConnection {
        if ($global:InstallerTest.started) { return [PSCustomObject]@{ OwningProcess = 123 } }
    }
    function Get-CimInstance {
        param($ClassName, $Filter)
        if ($Filter -like 'ProcessId=*') { return [PSCustomObject]@{ ExecutablePath = (Join-Path $global:InstallerTest.root 'pi-server.exe') } }
    }
    function icacls.exe {
        $global:LASTEXITCODE = if ($global:InstallerTest.failure -eq 'acl' -and $args[0] -like '*.env') { 1 } else { 0 }
    }
    function git { $global:LASTEXITCODE = 7 }
    function go {
        $index = [Array]::IndexOf($args, '-o')
        if ($index -lt 0) { throw 'Unexpected Go invocation in script tests.' }
        [IO.File]::WriteAllBytes($args[$index + 1], [byte[]](5, 6, 7, 8))
        $global:LASTEXITCODE = 0
    }

    $asset = Join-Path $temp 'asset.exe'
    $sums = Join-Path $temp 'SHA256SUMS'
    [IO.File]::WriteAllBytes($asset, [byte[]](1, 2, 3, 4))
    $hash = (Get-FileHash -LiteralPath $asset).Hash
    foreach ($installer in @('install-server-user.ps1', 'install-server.ps1')) {
        $path = Join-Path $PSScriptRoot $installer
        $ast = Read-ScriptAst $path
        foreach ($name in @('Get-ExpectedReleaseHash', 'Assert-ReleaseChecksum', 'Get-ServerReleaseBase')) {
            $definition = $ast.Find({ param($node) $node -is [Management.Automation.Language.FunctionDefinitionAst] -and $node.Name -eq $name }, $true)
            Assert-Test ($null -ne $definition) "Missing helper $name in $installer"
            . ([scriptblock]::Create($definition.Extent.Text))
        }
        [IO.File]::WriteAllText($sums, "  $($hash.ToLowerInvariant()) *asset.exe  `n")
        Assert-ReleaseChecksum $asset (Get-ExpectedReleaseHash $sums 'asset.exe')
        Assert-Rejected { Get-ExpectedReleaseHash $sums 'missing.exe' } 'Missing checksum entry was accepted.'
        Assert-Rejected { Assert-ReleaseChecksum $asset ('0' * 64) } 'Incorrect checksum was accepted.'
        [IO.File]::WriteAllText($sums, "$hash asset.exe`n$hash asset.exe`n")
        Assert-Rejected { Get-ExpectedReleaseHash $sums 'asset.exe' } 'Duplicate checksum entry was accepted.'
        [IO.File]::WriteAllText($sums, 'invalid asset.exe')
        Assert-Rejected { Get-ExpectedReleaseHash $sums 'asset.exe' } 'Malformed checksum entry was accepted.'
        $global:InstallerTest = @{ pages = 0; paginated = $true }
        Assert-Test ((Get-ServerReleaseBase 'fleetcru/pi-stack' stable) -like '*/server-v0.10.0') 'Wrong product or version selected.'
        Assert-Test ($global:InstallerTest.pages -eq 2) 'Stable release lookup did not paginate.'
        $global:InstallerTest = @{ pages = 0; httpStatus = 404 }
        Assert-Test ((Get-ServerReleaseBase 'fleetcru/pi-stack' dev) -like '*/server-v0.10.0') 'Missing dev release did not fall back to stable.'
        $global:InstallerTest = @{ pages = 0; httpStatus = 403 }
        Assert-Rejected { Get-ServerReleaseBase 'fleetcru/pi-stack' dev } '403 incorrectly triggered stable fallback.'
        Assert-Test ($global:InstallerTest.pages -eq 0) 'Failure changed release channels.'

        foreach ($failure in @('', 'download', 'checksum', 'register', 'startup', 'acl', 'source')) {
            $root = Join-Path $temp "$installer-$failure"
            New-Item -ItemType Directory -Path (Join-Path $root 'config') -Force | Out-Null
            $exe = Join-Path $root 'pi-server.exe'
            $envFile = Join-Path $root 'config\pi-server.env'
            $wrapper = Join-Path $root 'start.ps1'
            $config = "# keep this comment`nPI_SERVER_ADDR=0.0.0.0:43123`nPI_SERVER_AUTH_TOKEN=saved-credential`nPI_SERVER_MAX_SESSIONS=3`nPI_SERVER_ALLOWED_ROOTS=C:\custom-projects`n"
            [IO.File]::WriteAllBytes($exe, [byte[]](1, 2, 3, 4))
            [IO.File]::WriteAllText($envFile, $config)
            [IO.File]::WriteAllText($wrapper, '# previous wrapper')
            $global:InstallerTest = @{ root = $root; failure = $failure; taskExists = $true; taskState = 'Running'; stops = 0; starts = 0; downloads = 0; pages = 0 }
            $source = [IO.File]::ReadAllText($path).Replace('#Requires -RunAsAdministrator', '')
            $assignment = '$InstallDir = ' + (Quote-DevPowerShell $root)
            $source = $source.Replace('$InstallDir = Join-Path $env:LOCALAPPDATA "pi-server"', $assignment).Replace('$InstallDir = "C:\pi-server"', $assignment)
            Assert-Test (-not $source.Contains('$InstallDir = "C:\pi-server"')) 'Sandbox replacement failed.'
            $arguments = @{}
            if ($failure -eq 'source') { $arguments.BuildFromSource = $true }
            if ($failure) { Assert-Rejected { & ([scriptblock]::Create($source)) @arguments } "$installer accepted $failure failure." }
            else { & ([scriptblock]::Create($source)) @arguments | Out-Null }
            Assert-Test ([IO.File]::ReadAllText($envFile) -eq $config) "$installer lost existing configuration during $failure."
            if ($failure) {
                Assert-Test ((Get-FileHash $exe).Hash -eq $hash) "$installer failed to restore the original binary during $failure."
                Assert-Test ([IO.File]::ReadAllText($wrapper) -eq '# previous wrapper') "$installer lost the wrapper during rollback."
                if ($failure -in @('download', 'checksum', 'source')) { Assert-Test ($global:InstallerTest.stops -eq 0) 'Unverified download stopped the existing server.' }
            }
            Assert-Test ($global:InstallerTest.taskState -eq 'Running') 'Original task was left stopped.'
            Assert-Test (@(Get-ChildItem -LiteralPath $root -Filter '.install-*' -Directory).Count -eq 0) 'Temporary install files were leaked.'
        }
    }

    # Exercise bridge JSON under Windows PowerShell 5.1, with profile/registry
    # effects redirected or removed from the sandbox copy, never the real file.
    $homeDir = Join-Path $temp 'bridge-home'
    $configDir = Join-Path $homeDir '.pi\agent'
    New-Item -ItemType Directory -Path $configDir -Force | Out-Null
    $bridgeConfig = Join-Path $configDir 'bridge-config.json'
    [IO.File]::WriteAllText($bridgeConfig, '{"relayUrl":"http://127.0.0.1:43123","relayToken":"keep-me","custom":"preserve"}')
    $source = [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'install-exp-external-bridge.ps1'))
    $source = $source.Replace('$HOME', (Quote-DevPowerShell $homeDir)).Replace('$PSScriptRoot', (Quote-DevPowerShell $PSScriptRoot))
    $source = $source.Replace('[Environment]::SetEnvironmentVariable(''PI_EXTERNAL_RELAY_URL'', $relayUrl, ''User'')', '')
    $source = $source.Replace('[Environment]::SetEnvironmentVariable(''PI_EXTERNAL_RELAY_TOKEN'', $null, ''User'')', '')
    Assert-Test (-not $source.Contains('SetEnvironmentVariable')) 'Bridge sandbox would change the registry.'
    $global:InstallerTest.failure = ''
    & ([scriptblock]::Create($source)) | Out-Null
    $saved = [IO.File]::ReadAllText($bridgeConfig) | ConvertFrom-Json
    Assert-Test ($saved.relayToken -eq 'keep-me' -and $saved.custom -eq 'preserve') 'Bridge reinstall discarded settings.'
    Assert-Test ([IO.File]::ReadAllBytes($bridgeConfig)[0] -ne 239) 'Bridge JSON contains a UTF-8 BOM.'
    & ([scriptblock]::Create($source)) -RelayUrl 'http://127.0.0.1:43124' | Out-Null
    $saved = [IO.File]::ReadAllText($bridgeConfig) | ConvertFrom-Json
    Assert-Test ($saved.relayToken -eq '') 'Bridge forwarded a credential to a different relay.'
    Assert-Rejected { & ([scriptblock]::Create($source)) -RelayUrl 'https://user:password@example.com/' } 'Credential-bearing relay URL was accepted.'
    # Tray installation and removal must not delete neighboring user files.
    $trayModule = Join-Path $temp 'tray-module'
    $trayHome = Join-Path $temp 'tray-home'
    $trayInstall = Join-Path $temp 'tray-install'
    $shortcut = Join-Path $temp 'tray-startup.lnk'
    New-Item -ItemType Directory -Path $trayModule, $trayHome, $trayInstall | Out-Null
    [IO.File]::WriteAllText($shortcut, 'old shortcut')
    $source = [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'pi-server-tray/install.ps1'))
    $source = $source.Replace('$TrayDir = $PSScriptRoot', ('$TrayDir = ' + (Quote-DevPowerShell $trayModule)))
    $source = $source.Replace('Join-Path ([Environment]::GetFolderPath("Startup")) "Pi Server Tray.lnk"', (Quote-DevPowerShell $shortcut))
    $source = $source.Replace('$HOME', (Quote-DevPowerShell $trayHome))
    Assert-Test (-not $source.Contains('GetFolderPath("Startup")')) 'Tray sandbox would change the real startup folder.'
    & ([scriptblock]::Create($source)) -InstallDir $trayInstall -NoStartup -NoLaunch | Out-Null
    Assert-Test (-not (Test-Path -LiteralPath $shortcut)) '-NoStartup left the existing shortcut enabled.'
    Assert-Test (Test-Path -LiteralPath (Join-Path $trayInstall 'pi-server-tray.exe')) 'Tray binary was not installed.'
    [IO.File]::WriteAllText((Join-Path $trayInstall 'keep.txt'), 'user file')
    $source = [IO.File]::ReadAllText((Join-Path $PSScriptRoot 'pi-server-tray/uninstall.ps1'))
    $source = $source.Replace('Join-Path ([Environment]::GetFolderPath("Startup")) "Pi Server Tray.lnk"', (Quote-DevPowerShell $shortcut))
    $source = $source.Replace('$HOME', (Quote-DevPowerShell $trayHome))
    & ([scriptblock]::Create($source)) -InstallDir $trayInstall -WhatIf | Out-Null
    Assert-Test (Test-Path -LiteralPath (Join-Path $trayInstall 'pi-server-tray.exe')) 'Tray -WhatIf removed files.'
    & ([scriptblock]::Create($source)) -InstallDir $trayInstall | Out-Null
    Assert-Test (Test-Path -LiteralPath (Join-Path $trayInstall 'keep.txt')) 'Tray uninstall deleted another file.'
    Assert-Test (-not (Test-Path -LiteralPath (Join-Path $trayInstall 'pi-server-tray.exe'))) 'Tray uninstall left its executable.'
    $previousDataDir = $env:PI_SERVER_DATA_DIR
    try {
        $env:PI_SERVER_DATA_DIR = $trayHome
        Assert-Rejected { & ([scriptblock]::Create($source)) -InstallDir $trayInstall -RemoveData } 'Tray uninstall accepted deletion of the home directory.'
    } finally { $env:PI_SERVER_DATA_DIR = $previousDataDir }
    Write-Host 'Windows script tests passed. Installer upgrades and failures ran only against mocks.'
} finally {
    Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
    Remove-Variable InstallerTest -Scope Global -ErrorAction SilentlyContinue
}
