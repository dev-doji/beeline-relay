<#
.SYNOPSIS
  Prints the secret settings to paste into Render (the API host), on YOUR screen only.

.DESCRIPTION
  Reads beeline-api/.env and the TESTNET keys in beeline-api/.testnet, and prints KEY=value
  lines for the Render dashboard. Nothing is sent anywhere. Copy the lines into Render ->
  your service -> Environment, then close this window. Never paste them into a chat or commit them.

.EXAMPLE
  .\scripts\hosting-env.ps1 -WebUrl https://beeline-relay.vercel.app
#>
param(
  [Parameter(Mandatory = $true)][string]$WebUrl
)

$ErrorActionPreference = 'Stop'
$Api = Join-Path (Split-Path -Parent $PSScriptRoot) 'beeline-api'

function EnvValue($name) {
  $line = Get-Content (Join-Path $Api '.env') | Where-Object { $_ -match "^$name=" } | Select-Object -Last 1
  if (-not $line) { return '' }
  return ($line -replace "^$name=", '').Trim().Trim('"')
}
function KeyFile($file) {
  return (Get-Content (Join-Path $Api ".testnet\$file") -Raw | ConvertFrom-Json).private_key
}

$vault = EnvValue 'PLAYGROUND_VAULT'
$lines = @(
  "DATABASE_URL=$(EnvValue 'DATABASE_URL')",
  "CORS_ALLOWED_ORIGINS=$($WebUrl.TrimEnd('/'))",
  "PLAYGROUND_VAULT=$vault",
  "PLAYGROUND_AGENT_KEY=$(KeyFile 'agent-key.json')",
  "PLAYGROUND_INVOICE_ISSUER_KEY=$(KeyFile 'invoice-issuer.json')",
  "NEAR_AI_API_KEY=$(EnvValue 'NEAR_AI_API_KEY')",
  "NEAR_INTENTS_API_KEY=$(EnvValue 'NEAR_INTENTS_API_KEY')"
)

Write-Host ''
Write-Host 'Paste these into Render -> beeline-api -> Environment (one per variable).' -ForegroundColor Cyan
Write-Host 'They are SECRET: do not share them, screenshot them, or paste them into a chat.' -ForegroundColor Yellow
Write-Host ''
$lines | ForEach-Object { Write-Host $_ }
Write-Host ''
Write-Host 'The agent and invoice keys are TESTNET-only keys; they cannot touch mainnet funds.' -ForegroundColor DarkGray
