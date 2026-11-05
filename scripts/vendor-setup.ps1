<#
.SYNOPSIS
  One-time TESTNET setup for vendor invoices (pay links). Run after playground-setup.ps1.

.DESCRIPTION
  Signed by YOUR testnet owner account through the NEAR CLI keychain. Testnet only.
    1. Upgrade the vault's code to the latest CI build (keeps its state; invoices need no migration).
    2. Register the TESTNET invoice issuer key, so agents can pay vendors who are not on the
       allowlist, but only for invoices this key signed (exact payee, amount, once).

.EXAMPLE
  .\scripts\vendor-setup.ps1 -Owner goni-beeline.testnet
#>
param(
  [Parameter(Mandatory = $true)][string]$Owner
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Api = Join-Path $Root 'beeline-api'
$Work = Join-Path $Api '.testnet'
$Vault = "vault.$Owner"
$Net = @('network-config', 'testnet')

if ($Owner -notmatch '\.testnet$') { throw 'Vendor invoices in the playground run on testnet only: use your .testnet owner account.' }

function Step($n, $title) {
  Write-Host ''
  Write-Host "=== Step ${n}: $title ===" -ForegroundColor Cyan
  Read-Host 'Press Enter to run it (Ctrl+C to stop)' | Out-Null
}

function Ensure($what, $code = $global:LASTEXITCODE) {
  if ($code -ne 0) {
    Write-Host "!! $what failed (see the error above). Fix it, then run the script again." -ForegroundColor Red
    exit 1
  }
}

Step 1 "Upgrade $Vault to the latest contract built by CI (state is kept)"
$wasmDir = Join-Path $Work 'wasm'
if (Test-Path $wasmDir) { Remove-Item -Recurse -Force $wasmDir }
$runId = & gh run list --repo dev-doji/beeline-relay --branch main --workflow ci.yml --status success --limit 1 --json databaseId -q '.[0].databaseId'
& gh run download $runId --repo dev-doji/beeline-relay --name agent-vault-wasm --dir $wasmDir
Ensure 'Downloading the contract'
& near contract deploy $Vault use-file (Join-Path $wasmDir 'agent_vault.wasm') without-init-call @Net sign-with-keychain send | Out-Host
Ensure 'Upgrading the vault'

Step 2 'Register the TESTNET invoice issuer key'
Push-Location $Api
& go run ./cmd/vault-testnet invoice-issuer-args | Out-Host
$code = $LASTEXITCODE
Pop-Location
Ensure 'Preparing the invoice issuer key' $code
& near contract call-function as-transaction $Vault set_invoice_issuer file-args (Join-Path $Work 'set-invoice-issuer.json') prepaid-gas '100.0 Tgas' attached-deposit '0 NEAR' sign-as $Owner @Net sign-with-keychain send | Out-Host
Ensure 'Registering the invoice issuer'

& near contract call-function as-read-only $Vault get_invoice_issuer json-args '{}' @Net now | Out-Host
Write-Host "Done. Vendors can now be paid through signed invoices. Explorer: https://testnet.nearblocks.io/address/$Vault" -ForegroundColor Cyan
