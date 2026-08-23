function Get-ExpectedReleaseHash {
    param(
        [Parameter(Mandatory)][string]$ChecksumPath,
        [Parameter(Mandatory)][string]$AssetName
    )
    $escapedName = [Regex]::Escape($AssetName)
    $line = Get-Content -LiteralPath $ChecksumPath | Where-Object { $_ -match "\s+\*?$escapedName$" } | Select-Object -First 1
    if (-not $line) { throw "SHA256SUMS does not contain $AssetName" }
    $hash = ($line -split '\s+')[0]
    if ($hash -notmatch '^[0-9a-fA-F]{64}$') { throw "SHA256SUMS contains an invalid hash for $AssetName" }
    return $hash.ToUpperInvariant()
}

function Assert-ReleaseChecksum {
    param(
        [Parameter(Mandatory)][string]$FilePath,
        [Parameter(Mandatory)][string]$ExpectedHash
    )
    $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath $FilePath).Hash
    if ($actual -ne $ExpectedHash) { throw "Downloaded pi-server checksum mismatch" }
}
