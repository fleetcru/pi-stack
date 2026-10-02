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
