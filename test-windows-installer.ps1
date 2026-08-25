$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot "windows-installer-common.ps1")

$temp = Join-Path ([IO.Path]::GetTempPath()) "pi-installer-test-$([guid]::NewGuid().ToString('N'))"
New-Item -ItemType Directory -Path $temp | Out-Null
try {
    $binary = Join-Path $temp "pi-server-windows-amd64.exe"
    $checksums = Join-Path $temp "SHA256SUMS"
    [IO.File]::WriteAllBytes($binary, [byte[]](1, 2, 3, 4))
    $hash = (Get-FileHash -Algorithm SHA256 -LiteralPath $binary).Hash
    Set-Content -LiteralPath $checksums -Value "$($hash.ToLowerInvariant())  pi-server-windows-amd64.exe"

    $expected = Get-ExpectedReleaseHash -ChecksumPath $checksums -AssetName "pi-server-windows-amd64.exe"
    Assert-ReleaseChecksum -FilePath $binary -ExpectedHash $expected

    $missingRejected = $false
    try { Get-ExpectedReleaseHash -ChecksumPath $checksums -AssetName "missing.exe" | Out-Null } catch { $missingRejected = $true }
    if (-not $missingRejected) { throw "Missing checksum entry was accepted" }

    $mismatchRejected = $false
    try { Assert-ReleaseChecksum -FilePath $binary -ExpectedHash ('0' * 64) } catch { $mismatchRejected = $true }
    if (-not $mismatchRejected) { throw "Checksum mismatch was accepted" }

    Write-Host "Windows installer checksum tests passed."
} finally {
    Remove-Item -LiteralPath $temp -Recurse -Force -ErrorAction SilentlyContinue
}
