param(
  [string]$ServerUrl = "http://127.0.0.1:3141",
  [string]$Output = "app/src/main/java/com/example/picompanion/data/api/generated"
)

$ErrorActionPreference = "Stop"
$schema = Join-Path $PSScriptRoot "..\openapi.json"
Invoke-WebRequest "$($ServerUrl.TrimEnd('/'))/openapi.json" -OutFile $schema
# Generated output is committed so normal Android builds do not depend on a daemon.
npx --yes @openapitools/openapi-generator-cli generate `
  -i $schema `
  -g kotlin `
  -o $Output `
  --additional-properties packageName=com.example.picompanion.data.api.generated,library=jvm-okhttp4,serializationLibrary=kotlinx_serialization,modelPackage=com.example.picompanion.data.api.generated.model,apiPackage=com.example.picompanion.data.api.generated.api
Remove-Item $schema
Write-Host "Generated Kotlin OpenAPI client at $Output. Review and commit generated sources."
