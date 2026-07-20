param(
  [string]$Binary = "$PSScriptRoot\..\pi-server.exe",
  [string]$Name = "pi-server",
  [string]$Addr = "127.0.0.1:3141"
)
$Binary = [System.IO.Path]::GetFullPath($Binary)
if (-not (Test-Path -LiteralPath $Binary -PathType Leaf)) {
  throw "pi-server binary not found: $Binary. Run scripts/build.sh or pass -Binary explicitly."
}
$binPath = "`"$Binary`" --addr $Addr"
if (Get-Service -Name $Name -ErrorAction SilentlyContinue) {
  sc.exe delete $Name | Out-Null
}
sc.exe create $Name binPath= $binPath start= auto DisplayName= "Pi Server" | Out-Null
Write-Host "Installed $Name. Start with: Start-Service $Name"
