# Shared by development launchers. No processes start when this file is loaded.
function Assert-DevPortAvailable {
    param([int]$Port)
    $listener = New-Object Net.Sockets.TcpListener ([Net.IPAddress]::Any), $Port
    $listener.Server.ExclusiveAddressUse = $true
    try { $listener.Start() }
    catch { throw "Port $Port is already in use. Stop the existing service or choose another port." }
    finally { $listener.Stop() }
}

function Wait-DevHttpReady {
    param([string]$Url, [Diagnostics.Process]$Process, [int]$TimeoutSec = 30)
    $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSec)
    while ([DateTime]::UtcNow -lt $deadline) {
        if ($Process.HasExited) { throw "Process $($Process.Id) exited with code $($Process.ExitCode) before $Url became ready." }
        try {
            $response = Invoke-WebRequest -Uri $Url -UseBasicParsing -TimeoutSec 2 -ErrorAction Stop
            if ($response.StatusCode -eq 200) {
                Start-Sleep -Milliseconds 250
                if ($Process.HasExited) { throw "Process $($Process.Id) exited during startup." }
                return
            }
        } catch { }
        Start-Sleep -Milliseconds 250
    }
    throw "Timed out waiting for $Url after ${TimeoutSec}s. Check the startup logs."
}

function Start-DevPowerShell {
    param([string]$Command, [string]$WindowStyle = 'Normal')
    # Start-Process joins ArgumentList with spaces. EncodedCommand avoids
    # reparsing paths containing spaces, quotes, ampersands, or dollar signs.
    $encoded = [Convert]::ToBase64String([Text.Encoding]::Unicode.GetBytes($Command))
    $powerShell = Join-Path $env:SystemRoot 'System32\WindowsPowerShell\v1.0\powershell.exe'
    Start-Process -FilePath $powerShell -WindowStyle $WindowStyle -ArgumentList @('-NoProfile', '-ExecutionPolicy', 'RemoteSigned', '-EncodedCommand', $encoded) -PassThru
}

function Stop-DevProcessTree {
    param([Diagnostics.Process]$Process)
    if ($Process -and -not $Process.HasExited) {
        & taskkill.exe /PID $Process.Id /T /F | Out-Null
        if ($LASTEXITCODE -ne 0 -and -not $Process.HasExited) { Write-Warning "Could not stop process tree $($Process.Id)." }
    }
}

function Quote-DevPowerShell {
    param([string]$Value)
    return "'" + $Value.Replace("'", "''") + "'"
}

function Build-DevServer {
    param([string]$ServerDir, [string]$Output)
    Push-Location $ServerDir
    try {
        & go build -trimpath -o $Output ./cmd/pi-server
        if ($LASTEXITCODE -ne 0) { throw "Server build failed with exit code $LASTEXITCODE." }
    } finally { Pop-Location }
}

function Get-DevProcessRecords {
    param([Diagnostics.Process[]]$Processes)
    $snapshot = @(Get-CimInstance Win32_Process)
    $queue = New-Object 'Collections.Generic.Queue[int]'
    $seen = @{}
    $rootStarts = @{}
    foreach ($process in $Processes) {
        if (-not $process.HasExited) {
            $rootStarts[$process.Id] = $process.StartTime.ToUniversalTime().Ticks
            $queue.Enqueue($process.Id)
        }
    }
    while ($queue.Count) {
        $processId = $queue.Dequeue()
        if ($seen.ContainsKey($processId)) { continue }
        $seen[$processId] = $true
        $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
        if (-not $process) { continue }
        $started = $process.StartTime.ToUniversalTime().Ticks
        if ($rootStarts.ContainsKey($processId)) {
            if ($started -ne $rootStarts[$processId]) { continue }
        } else {
            $original = $snapshot | Where-Object { $_.ProcessId -eq $processId } | Select-Object -First 1
            # WMI rounds creation timestamps to microseconds. Reject a new
            # process that reused a descendant's PID after the snapshot.
            if (-not $original -or [Math]::Abs($original.CreationDate.ToUniversalTime().Ticks - $started) -gt 10) { continue }
        }
        # Digits stay strings in both PS 5.1 and PS 7 JSON readers. ISO dates
        # can become DateTime objects and silently break string comparisons.
        @{ id = $processId; startedAt = $started.ToString([Globalization.CultureInfo]::InvariantCulture) }
        foreach ($child in $snapshot) { if ($child.ParentProcessId -eq $processId) { $queue.Enqueue([int]$child.ProcessId) } }
    }
}

function Test-DevProcessRecord {
    param([Diagnostics.Process]$Process, $Record)
    if (-not $Process -or $Process.HasExited) { return $false }
    $expected = $Record.startedAt
    if ($expected -is [DateTime]) { $expected = $expected.ToUniversalTime().Ticks }
    elseif ([string]$expected -notmatch '^[0-9]+$') {
        $expected = [DateTime]::Parse([string]$expected, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::RoundtripKind).ToUniversalTime().Ticks
    }
    return $Process.StartTime.ToUniversalTime().Ticks -eq [int64]$expected
}

function Get-DevNetworkAddresses {
    Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue |
        Where-Object { $_.AddressState -eq 'Preferred' -and $_.IPAddress -match '^(10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.)' } |
        Select-Object -ExpandProperty IPAddress -Unique
}
