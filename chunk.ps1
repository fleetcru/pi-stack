if (-not $PiCommand) { Write-Fail "Pi CLI is not installed or is not available in PATH." }
$PiBinary = $PiCommand.Source

$EnvContent = @(
    "# pi-server configuration"
    "# Edit this file, then restart the task:"
    "#   Stop-ScheduledTask -TaskName '$TaskName'"
    "#   Start-ScheduledTask -TaskName '$TaskName'"
    ""
    "PI_SERVER_ADDR=0.0.0.0:$Port"
    "PI_SERVER_DATA_DIR=$DataDir"
    "PI_SERVER_ALLOWED_ROOTS=$env:USERPROFILE"
    "PI_SERVER_AUTH_TOKEN=$AuthToken"
    "PI_SERVER_PI_BINARY=$PiBinary"
) -join [Environment]::NewLine

# Only add ALLOW_INSECURE if explicitly requested
if ($AllowInsecure) {
    $EnvContent += [Environment]::NewLine + "PI_SERVER_ALLOW_INSECURE=1"
    Write-Warn "Running in INSECURE mode — auth token will not be enforced"
}
