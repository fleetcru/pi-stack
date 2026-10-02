<#
.SYNOPSIS
    Installs pi-server for the current user (no admin required).

.DESCRIPTION
    Downloads the pi-server binary from GitHub releases, installs it to
    %LOCALAPPDATA%\pi-server, and creates a logon task to start automatically.

.PARAMETER Port
    Port to listen on. Default: 3142

.PARAMETER AuthToken
    Optional auth token for API authentication.

.PARAMETER Channel
    Release channel. 'dev' (default) tracks the rolling server-dev build that is
    replaced on every main-branch push. 'stable' pins to the newest immutable
    server-v* release. The dev channel falls back to stable when server-dev has
    not been published yet.

.PARAMETER SourceRevision
    Exact Git commit used with the explicit -BuildFromSource switch.

.PARAMETER BuildFromSource
    Build the pinned revision instead of downloading a release. Requires Git and Go.
    Download or checksum failures never trigger a source build.

.EXAMPLE
    .\install-server-user.ps1
    .\install-server-user.ps1 -Port 9000 -AuthToken "my-secret"
    .\install-server-user.ps1 -Channel stable
#>

param(
    [ValidateRange(1, 65535)][int]$Port = 3142,
    [ValidateScript({ $_ -notmatch '[\r\n\x00]' })][string]$AuthToken = "",
    [switch]$BuildFromSource,
    [ValidateSet('dev', 'stable')]
    [string]$Channel = $(if ($env:PI_SERVER_CHANNEL) { $env:PI_SERVER_CHANNEL } else { 'dev' }),
    [ValidatePattern('^[0-9a-fA-F]{40}$')]
    [string]$SourceRevision = "3ef3f52c2b776b2a913122dc473302d06665e7cc"
)

$ErrorActionPreference = "Stop"
[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

# Keep this script self-contained so it also works when downloaded and run
# directly with `irm https://winuser.fleetcru.dev | iex`.
function Get-ExpectedReleaseHash {
    param(
        [Parameter(Mandatory)][string]$ChecksumPath,
        [Parameter(Mandatory)][string]$AssetName
    )
    $escapedName = [Regex]::Escape($AssetName)
    $lines = @(Get-Content -LiteralPath $ChecksumPath | Where-Object { $_ -match "^\s*[0-9a-fA-F]{64}\s+\*?$escapedName\s*$" })
    if ($lines.Count -ne 1) { throw "SHA256SUMS must contain exactly one valid entry for $AssetName" }
    $hash = ($lines[0].Trim() -split '\s+')[0]
    if ($hash -notmatch '^[0-9a-fA-F]{64}$') { throw "SHA256SUMS contains an invalid hash for $AssetName" }
    return $hash.ToUpperInvariant()
}

function Assert-ReleaseChecksum {
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [Parameter(Mandatory)][string]$ExpectedHash
    )
    $stream = [IO.File]::OpenRead($FilePath)
    $sha256 = [Security.Cryptography.SHA256]::Create()
    try { $actual = [BitConverter]::ToString($sha256.ComputeHash($stream)).Replace('-', '') }
    finally { $sha256.Dispose(); $stream.Dispose() }
    if ($actual -ne $ExpectedHash) { throw "Downloaded pi-server checksum mismatch" }
}

# ── Config ────────────────────────────────────────────────
$Repo = "fleetcru/pi-stack"
$InstallDir = Join-Path $env:LOCALAPPDATA "pi-server"
$DataDir = Join-Path $InstallDir "data"
$ConfigDir = Join-Path $InstallDir "config"
$TaskName = "PiServer-$env:USERNAME"

function Get-ServerReleaseBase {
    param([string]$Repo, [string]$Channel)
    if ($Channel -eq 'dev') {
        try {
            $release = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases/tags/server-dev" -TimeoutSec 30
            if ($release.draft) { throw "server-dev is a draft release" }
            return "https://github.com/$Repo/releases/download/server-dev"
        } catch {
            if (-not $_.Exception.Response -or [int]$_.Exception.Response.StatusCode -ne 404) { throw }
            Write-Warning "server-dev does not exist. Trying a stable server release."
        }
    }
    # /releases/latest can select Companion or tray. Select server tags only.
    $candidates = @()
    for ($page = 1; $page -le 10; $page++) {
        $releases = Invoke-RestMethod "https://api.github.com/repos/$Repo/releases?per_page=100&page=$page" -TimeoutSec 30
        foreach ($release in $releases) {
            if (-not $release.draft -and -not $release.prerelease -and $release.tag_name -match '^server-v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$') {
                $candidates += [PSCustomObject]@{ Tag = $release.tag_name; Version = [version]($release.tag_name.Substring(8)) }
            }
        }
        if ($releases.Count -lt 100) { break }
        if ($page -eq 10) { throw "Release listing exceeded 1,000 entries. Cannot safely select the newest server release." }
    }
    $selected = $candidates | Sort-Object Version -Descending | Select-Object -First 1
    if (-not $selected) { throw "No stable server-v<version> release exists. Try -Channel dev." }
    return "https://github.com/$Repo/releases/download/$($selected.Tag)"
}

# ── Helpers ───────────────────────────────────────────────
function Write-Step($msg) { Write-Host "[info] $msg" -ForegroundColor Cyan }
function Write-Ok($msg)   { Write-Host "[ok] $msg" -ForegroundColor Green }
function Write-Warn($msg) { Write-Host "[warn] $msg" -ForegroundColor Yellow }
function Write-Fail($msg) { throw $msg }

# Check prerequisites before changing an existing installation.
$EnvFile = Join-Path $ConfigDir 'pi-server.env'
$WrapperPath = Join-Path $InstallDir 'start.ps1'
$ExistingConfig = if (Test-Path -LiteralPath $EnvFile) { [IO.File]::ReadAllText($EnvFile) } else { $null }
$ExistingWrapper = if (Test-Path -LiteralPath $WrapperPath) { [IO.File]::ReadAllText($WrapperPath) } else { $null }
if ($ExistingConfig -and -not $PSBoundParameters.ContainsKey('Port') -and $ExistingConfig -match '(?m)^PI_SERVER_ADDR=.*:([0-9]+)\r?$') { $Port = [int]$matches[1] }
$PiCommand = Get-Command pi -ErrorAction SilentlyContinue
if (-not $PiCommand -and -not $ExistingConfig) { throw 'Pi CLI is not installed or is not available in PATH.' }

# ── Create directories ────────────────────────────────────
Write-Step "Creating directories in $InstallDir..."
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null
New-Item -ItemType Directory -Force -Path $DataDir | Out-Null
New-Item -ItemType Directory -Force -Path $ConfigDir | Out-Null
$CurrentUserSid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
& icacls.exe $InstallDir /inheritance:r /grant:r "*$CurrentUserSid`:(OI)(CI)(F)" '*S-1-5-18:(OI)(CI)(F)' '*S-1-5-32-544:(OI)(CI)(F)' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Could not protect the installation directory.' }
& icacls.exe $ConfigDir /inheritance:r /grant:r "*$CurrentUserSid`:(OI)(CI)(F)" '*S-1-5-18:(OI)(CI)(F)' '*S-1-5-32-544:(OI)(CI)(F)' | Out-Null
if ($LASTEXITCODE -ne 0) { throw 'Could not protect the configuration directory.' }
$InstallLock = [IO.File]::Open((Join-Path $InstallDir '.install.lock'), [IO.FileMode]::OpenOrCreate, [IO.FileAccess]::ReadWrite, [IO.FileShare]::None)
try {
# Re-read under the lock so an overlapping installer cannot restore stale settings.
$ExistingConfig = if (Test-Path -LiteralPath $EnvFile) { [IO.File]::ReadAllText($EnvFile) } else { $null }
$ExistingWrapper = if (Test-Path -LiteralPath $WrapperPath) { [IO.File]::ReadAllText($WrapperPath) } else { $null }
if ($ExistingConfig -and -not $PSBoundParameters.ContainsKey('Port') -and $ExistingConfig -match '(?m)^PI_SERVER_ADDR=.*:([0-9]+)\r?$') { $Port = [int]$matches[1] }
Write-Ok 'Directories created'

# Stage and verify before stopping the installed server.
$ExePath = Join-Path $InstallDir 'pi-server.exe'
$StageDir = Join-Path $InstallDir ".install-$([guid]::NewGuid().ToString('N'))"
$StagedExe = Join-Path $StageDir 'pi-server.exe'
$BackupExe = Join-Path $StageDir 'previous.exe'
$PreviousTaskXml = $null
$WasRunning = $false
$Installed = $false
$Committed = $false
$TaskStopped = $false
try {
    New-Item -ItemType Directory -Path $StageDir | Out-Null
    $PreviousTask = Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
    if ($PreviousTask) {
        $ownedAction = @($PreviousTask.Actions | Where-Object { $_.Execute -match '(^|[\\/])powershell\.exe$' -and $_.Arguments -and $_.Arguments.IndexOf($WrapperPath, [StringComparison]::OrdinalIgnoreCase) -ge 0 })
        if (-not $ownedAction.Count) { throw "Task $TaskName does not belong to this installation. It has not been changed." }
        $PreviousTaskXml = Export-ScheduledTask -TaskName $TaskName
        $WasRunning = $PreviousTask.State -eq 'Running'
    }
    foreach ($listener in @(Get-NetTCPConnection -State Listen -LocalPort $Port -ErrorAction SilentlyContinue)) {
        $owner = Get-CimInstance Win32_Process -Filter "ProcessId=$($listener.OwningProcess)" -ErrorAction SilentlyContinue
        if (-not $WasRunning -or -not $owner.ExecutablePath -or -not [string]::Equals($owner.ExecutablePath, $ExePath, [StringComparison]::OrdinalIgnoreCase)) {
            throw "Port $Port is owned by another process. Stop it explicitly or choose a different port."
        }
    }
    if ($BuildFromSource) {
        foreach ($command in @('git', 'go')) {
            if (-not (Get-Command $command -ErrorAction SilentlyContinue)) { throw "$command is required for -BuildFromSource" }
        }
        $SourceDir = Join-Path $StageDir 'source'
        New-Item -ItemType Directory -Path $SourceDir | Out-Null
        foreach ($arguments in @(@('init'), @('remote', 'add', 'origin', "https://github.com/$Repo.git"), @('fetch', '--depth', '1', 'origin', $SourceRevision), @('checkout', '--detach', 'FETCH_HEAD'))) {
            & git -C $SourceDir @arguments
            if ($LASTEXITCODE -ne 0) { throw "git failed with exit code $LASTEXITCODE" }
        }
        Push-Location (Join-Path $SourceDir 'pi-server-exp')
        try {
            & go build -trimpath -o $StagedExe ./cmd/pi-server
            if ($LASTEXITCODE -ne 0) { throw "go build failed with exit code $LASTEXITCODE" }
        } finally { Pop-Location }
    } else {
        Write-Step "Downloading pi-server ($Channel channel)..."
        $ReleaseBase = Get-ServerReleaseBase -Repo $Repo -Channel $Channel
        $ChecksumPath = Join-Path $StageDir 'SHA256SUMS'
        Invoke-WebRequest "$ReleaseBase/pi-server-windows-amd64.exe" -OutFile $StagedExe -UseBasicParsing -TimeoutSec 120
        Invoke-WebRequest "$ReleaseBase/SHA256SUMS" -OutFile $ChecksumPath -UseBasicParsing -TimeoutSec 30
        $ExpectedHash = Get-ExpectedReleaseHash -ChecksumPath $ChecksumPath -AssetName 'pi-server-windows-amd64.exe'
        Assert-ReleaseChecksum -FilePath $StagedExe -ExpectedHash $ExpectedHash
        Write-Ok 'Downloaded and verified binary'
    }
    if ($WasRunning) {
        Write-Step 'Stopping the installed task for upgrade...'
        Stop-ScheduledTask -TaskName $TaskName
        $TaskStopped = $true
        $deadline = [DateTime]::UtcNow.AddSeconds(30)
        while ((Get-ScheduledTask -TaskName $TaskName).State -eq 'Running') {
            if ([DateTime]::UtcNow -ge $deadline) { throw 'Task did not stop. The existing binary has not been replaced.' }
            Start-Sleep -Milliseconds 250
        }
    }
    if ($TaskStopped) {
        $deadline = [DateTime]::UtcNow.AddSeconds(30)
        do {
            $children = @(Get-CimInstance Win32_Process -Filter "Name='pi-server.exe'" -ErrorAction Stop | Where-Object { [string]::Equals($_.ExecutablePath, $ExePath, [StringComparison]::OrdinalIgnoreCase) })
            if (-not $children.Count) { break }
            if ([DateTime]::UtcNow -ge $deadline) {
                $TaskStopped = $false
                throw 'The server child did not stop. Stop it explicitly before upgrading. Its binary is unchanged.'
            }
            Start-Sleep -Milliseconds 250
        } while ($true)
    }
    if (Test-Path -LiteralPath $ExePath) { Move-Item -LiteralPath $ExePath -Destination $BackupExe }
    Move-Item -LiteralPath $StagedExe -Destination $ExePath
    $Installed = $true

# ── Write config ──────────────────────────────────────────
Write-Step "Writing configuration..."
$PiBinary = if ($PiCommand) { $PiCommand.Source } else { '' }

$EnvContent = @(
    "# pi-server configuration"
    "# Edit this file, then restart the task:"
    "#   Stop-ScheduledTask -TaskName '$TaskName'"
    "#   Start-ScheduledTask -TaskName '$TaskName'"
    ""
    "PI_SERVER_ADDR=127.0.0.1:$Port"
    "PI_SERVER_CWD=$env:USERPROFILE"
    "PI_SERVER_DATA_DIR=$DataDir"
    "PI_SERVER_ALLOWED_ROOTS=$env:USERPROFILE"
    "PI_SERVER_PI_BINARY=$PiBinary"
    ""
    "# For LAN/Tailscale access, uncomment and set:"
    "# PI_SERVER_ADDR=0.0.0.0:$Port"
    ""
    "# Set a token to require authentication:"
    "# PI_SERVER_AUTH_TOKEN=your-secret-token"
) -join [Environment]::NewLine
if ($AuthToken) {
    $EnvContent += "`nPI_SERVER_AUTH_TOKEN=$AuthToken"
}

if ($null -ne $ExistingConfig) {
    $EnvContent = $ExistingConfig
    if ($PSBoundParameters.ContainsKey('Port')) {
        if ($EnvContent -match '(?m)^PI_SERVER_ADDR=') {
            $EnvContent = [regex]::Replace($EnvContent, '(?m)^(PI_SERVER_ADDR=.*):[0-9]+\r?$', "`${1}:$Port")
        } else { $EnvContent = $EnvContent.TrimEnd() + "`nPI_SERVER_ADDR=127.0.0.1:$Port`n" }
    }
    if ($PSBoundParameters.ContainsKey('AuthToken')) {
        $EnvContent = [regex]::Replace($EnvContent, '(?m)^PI_SERVER_AUTH_TOKEN=.*\r?\n?', '')
        $EnvContent = $EnvContent.TrimEnd() + "`nPI_SERVER_AUTH_TOKEN=$AuthToken`n"
    }
    Write-Ok 'Preserving existing configuration and credentials'
}
[IO.File]::WriteAllText($EnvFile, $EnvContent, (New-Object Text.UTF8Encoding $false))
$CurrentUserSid = [System.Security.Principal.WindowsIdentity]::GetCurrent().User.Value
& icacls.exe $EnvFile /inheritance:r /grant:r "*$CurrentUserSid`:(F)" "*S-1-5-18`:(F)" "*S-1-5-32-544`:(F)" | Out-Null
if ($LASTEXITCODE -ne 0) { Write-Fail "Could not restrict ACLs on $EnvFile" }
Write-Ok "Config written to $EnvFile with restricted ACLs"

# ── Create wrapper script ────────────────────────────────
Write-Step "Creating service wrapper..."
$WrapperPath = Join-Path $InstallDir "start.ps1"

$WrapperContent = @'
# Task Scheduler owns backgrounding. Return the server's exit code for retries.
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot
$envFile = Join-Path $PSScriptRoot 'config\pi-server.env'
Get-Content -LiteralPath $envFile | ForEach-Object {
    if ($_ -match '^\s*([A-Za-z_][A-Za-z0-9_]*)=(.*)$') {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2], 'Process')
    }
}
try {
    & (Join-Path $PSScriptRoot 'pi-server.exe') --pairing-qr=false
    exit $LASTEXITCODE
} catch {
    $_ | Out-File (Join-Path $PSScriptRoot 'startup-error.log') -Append
    exit 1
}
'@

Set-Content -Path $WrapperPath -Value $WrapperContent -Encoding UTF8
Write-Ok "Wrapper created"

# ── Create scheduled task (logon, current user, no admin) ─
Write-Step "Creating logon task..."

$Action = New-ScheduledTaskAction `
    -Execute (Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe') `
    -Argument "-NoProfile -ExecutionPolicy RemoteSigned -WindowStyle Hidden -File `"$WrapperPath`"" `
    -WorkingDirectory $InstallDir

$Trigger = New-ScheduledTaskTrigger -AtLogOn -User $env:USERNAME

$Settings = New-ScheduledTaskSettingsSet `
    -AllowStartIfOnBatteries `
    -DontStopIfGoingOnBatteries `
    -StartWhenAvailable `
    -RestartCount 3 `
    -RestartInterval (New-TimeSpan -Minutes 1) `
    -MultipleInstances IgnoreNew `
    -ExecutionTimeLimit ([TimeSpan]::Zero)

Register-ScheduledTask `
    -TaskName $TaskName `
    -Action $Action `
    -Trigger $Trigger `
    -Settings $Settings `
    -Description "pi-server Pi coding agent hub (user install)" `
    -Force | Out-Null

Write-Ok "Logon task created: $TaskName"

# ── Start now ─────────────────────────────────────────────
Write-Step "Starting pi-server..."
Start-ScheduledTask -TaskName $TaskName
Start-Sleep -Seconds 2

$Task = Get-ScheduledTask -TaskName $TaskName
if ($Task.State -eq 'Running') {
    $address = if ($EnvContent -match '(?m)^PI_SERVER_ADDR=([^\r\n]+)') { $matches[1].Trim() } else { '0.0.0.0:3142' }
    $probeAddress = $address -replace '^0\.0\.0\.0:', '127.0.0.1:' -replace '^\[::\]:', '[::1]:'
    $probePort = [int]($address -replace '^.*:', '')
    $ready = $false
    $deadline = [DateTime]::UtcNow.AddSeconds(30)
    while ([DateTime]::UtcNow -lt $deadline) {
        if ((Get-ScheduledTask -TaskName $TaskName).State -ne 'Running') { throw 'Server task exited during startup.' }
        $ownedListener = $false
        foreach ($listener in @(Get-NetTCPConnection -State Listen -LocalPort $probePort -ErrorAction SilentlyContinue)) {
            $owner = Get-CimInstance Win32_Process -Filter "ProcessId=$($listener.OwningProcess)" -ErrorAction SilentlyContinue
            if ([string]::Equals($owner.ExecutablePath, $ExePath, [StringComparison]::OrdinalIgnoreCase)) { $ownedListener = $true }
        }
        if ($ownedListener) {
            try {
                $health = Invoke-WebRequest "http://$probeAddress/healthz" -UseBasicParsing -TimeoutSec 2
                if ($health.StatusCode -eq 200) { $ready = $true; break }
            } catch { }
        }
        Start-Sleep -Milliseconds 250
    }
    if (-not $ready) { throw "Server did not become ready. Check $InstallDir\startup-error.log and the configured data directory's pi-server.log." }
    Write-Ok 'pi-server is running and healthy'
} else {
    throw "pi-server failed to start. Check $InstallDir\startup-error.log and the data directory's pi-server.log."
}
$Committed = $true
} catch {
    $failure = $_
    if ($Installed -or (Test-Path -LiteralPath $BackupExe)) {
        Stop-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue
        if (Test-Path -LiteralPath $BackupExe) {
            if (Test-Path -LiteralPath $ExePath) { Remove-Item -LiteralPath $ExePath -Force }
            Move-Item -LiteralPath $BackupExe -Destination $ExePath
        } elseif ($Installed) { Remove-Item -LiteralPath $ExePath -Force }
        if ($null -ne $ExistingConfig) { [IO.File]::WriteAllText($EnvFile, $ExistingConfig, (New-Object Text.UTF8Encoding $false)) }
        else { Remove-Item -LiteralPath $EnvFile -Force -ErrorAction SilentlyContinue }
        if ($null -ne $ExistingWrapper) { [IO.File]::WriteAllText($WrapperPath, $ExistingWrapper, (New-Object Text.UTF8Encoding $false)) }
        else { Remove-Item -LiteralPath $WrapperPath -Force -ErrorAction SilentlyContinue }
        if ($PreviousTaskXml) { Register-ScheduledTask -TaskName $TaskName -Xml $PreviousTaskXml -Force | Out-Null }
        else { Unregister-ScheduledTask -TaskName $TaskName -Confirm:$false -ErrorAction SilentlyContinue }
    }
    if ($WasRunning -and $TaskStopped) { Start-ScheduledTask -TaskName $TaskName }
    throw $failure
} finally {
    if (-not $Committed -and (Test-Path -LiteralPath $BackupExe)) { Write-Warning "Previous binary retained at $BackupExe after a failed rollback." }
    else { Remove-Item -LiteralPath $StageDir -Recurse -Force -ErrorAction SilentlyContinue }
}

# ── Summary ───────────────────────────────────────────────
Write-Host ""
Write-Host "==================================================" -ForegroundColor Green
Write-Host "  pi-server installed for current user!"
Write-Host ""
Write-Host "  URL:      http://127.0.0.1:$Port" -ForegroundColor Cyan
Write-Host "  Config:   $EnvFile"
Write-Host "  Data:     $DataDir"
Write-Host "  Binary:   $ExePath"
Write-Host ""
Write-Host "  Commands:"
Write-Host "    Start-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host "    Stop-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host "    Get-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host "    Unregister-ScheduledTask -TaskName '$TaskName'" -ForegroundColor Cyan
Write-Host ""
Write-Host "  To uninstall: run the Unregister command above,"
Write-Host "  then delete $InstallDir"
Write-Host "==================================================" -ForegroundColor Green
} finally {
    $InstallLock.Dispose()
}
