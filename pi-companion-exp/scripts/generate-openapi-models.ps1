param(
  [string]$ServerUrl = "http://127.0.0.1:3141",
  [string]$Output = "app/src/main/java/com/example/picompanion/data/api/generated"
)

$ErrorActionPreference = "Stop"
if (-not (Get-Command npx -ErrorAction SilentlyContinue)) {
  throw "npx is required. Install Node.js, then run this script again."
}

$projectRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$outputPath = if ([IO.Path]::IsPathRooted($Output)) { $Output } else { Join-Path $projectRoot $Output }
$schema = Join-Path ([IO.Path]::GetTempPath()) "pi-server-openapi-$([guid]::NewGuid().ToString('N')).json"

try {
  Invoke-WebRequest "$($ServerUrl.TrimEnd('/'))/openapi.json" -OutFile $schema
  # Generated output is committed so normal Android builds do not depend on a daemon.
  # npx may prompt to download the generator. Review that package before approving it.
  npx @openapitools/openapi-generator-cli generate `
    -i $schema `
    -g kotlin `
    -o $outputPath `
    --additional-properties packageName=com.example.picompanion.data.api.generated,library=jvm-okhttp4,serializationLibrary=kotlinx_serialization,modelPackage=com.example.picompanion.data.api.generated.model,apiPackage=com.example.picompanion.data.api.generated.api
  if ($LASTEXITCODE -ne 0) { throw "OpenAPI generator exited with code $LASTEXITCODE" }
} finally {
  Remove-Item -LiteralPath $schema -Force -ErrorAction SilentlyContinue
}

Write-Host "Generated Kotlin OpenAPI client at $outputPath. Review and commit generated sources."
