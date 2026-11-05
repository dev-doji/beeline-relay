<#
.SYNOPSIS
  One-time TESTNET setup of the vault the public Playground uses. Run after testnet-rehearsal.ps1.

.DESCRIPTION
  Signed by YOUR testnet owner account through the NEAR CLI keychain. Testnet only, test tokens only.
    1. Upgrade the vault's code to the latest CI build (keeps its state).
    2. Raise the agent key's gas allowance to 3 NEAR (the key can still only call pay_quote).
    3. Set limits: 5 test USDC per payment, 1000 per agent per day.
    4. Mint 1000 test USDC into the vault and credit it to the pool.
    5. Register the NEAR AI reviewer (key checked live against NEAR AI's TEE attestation),
       required for every payment (threshold 1).

.EXAMPLE
  .\scripts\playground-setup.ps1 -Owner goni-beeline.testnet
#>
param(
  [Parameter(Mandatory = $true)][string]$Owner
)

$ErrorActionPreference = 'Stop'
$Root = Split-Path -Parent $PSScriptRoot
$Api = Join-Path $Root 'beeline-api'
$Work = Join-Path $Api '.testnet'
$Vault = "vault.$Owner"
$Token = 'usdc.fakes.testnet'
$Net = @('network-config', 'testnet')

if ($Owner -notmatch '\.testnet$') { throw 'The playground runs on testnet only: use your .testnet owner account.' }

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

# PowerShell 5.1 mangles JSON passed to programs, so every argument goes through a file.
function ArgsFile($name, $obj) {
  New-Item -ItemType Directory -Force $Work | Out-Null
  $path = Join-Path $Work "$name.json"
  [IO.File]::WriteAllText($path, ($obj | ConvertTo-Json -Compress -Depth 5))
  return $path
}

function OwnerCall($contract, $method, $argsPath, $deposit = '0 NEAR') {
  & near contract call-function as-transaction $contract $method file-args $argsPath prepaid-gas '100.0 Tgas' attached-deposit $deposit sign-as $Owner @Net sign-with-keychain send | Out-Host
  Ensure "$method"
}

$agentFile = Join-Path $Work 'agent-key.json'
if (-not (Test-Path $agentFile)) { throw "No $agentFile. Run testnet-rehearsal.ps1 first." }
$AgentPk = (Get-Content $agentFile -Raw | ConvertFrom-Json).public_key

Step 1 "Upgrade $Vault to the latest contract built by CI (state is kept)"
$wasmDir = Join-Path $Work 'wasm'
if (Test-Path $wasmDir) { Remove-Item -Recurse -Force $wasmDir }
$runId = & gh run list --repo dev-doji/beeline-relay --branch main --workflow ci.yml --status success --limit 1 --json databaseId -q '.[0].databaseId'
& gh run download $runId --repo dev-doji/beeline-relay --name agent-vault-wasm --dir $wasmDir
Ensure 'Downloading the contract'
& near contract deploy $Vault use-file (Join-Path $wasmDir 'agent_vault.wasm') without-init-call @Net sign-with-keychain send | Out-Host
Ensure 'Upgrading the vault'

Step 2 'Raise the agent key gas allowance to 3 NEAR (it can still only call pay_quote)'
OwnerCall $Vault 'set_agent_allowance' (ArgsFile 'allowance' @{ public_key = $AgentPk; allowance = '3000000000000000000000000' })

Step 3 'Set limits: 5 test USDC per payment, 1000 test USDC per agent per day'
OwnerCall $Vault 'set_limits' (ArgsFile 'limits' @{ max_per_payment = '5000000'; daily_limit_per_agent = '1000000000' })

Step 4 'Mint 1000 test USDC into the vault and credit it to the pool'
OwnerCall $Token 'mint' (ArgsFile 'mint-pg' @{ account_id = $Vault; amount = '1000000000' })
OwnerCall $Vault 'sync_balance' (ArgsFile 'empty' @{})

Step 5 'Register the NEAR AI reviewer (attestation checked live) for every payment'
Push-Location $Api
& go run ./cmd/vault-testnet reviewer-args -threshold 1 | Out-Host
$code = $LASTEXITCODE
Pop-Location
Ensure 'Checking the NEAR AI attestation' $code
OwnerCall $Vault 'set_reviewer' (Join-Path $Work 'set-reviewer.json')

Write-Host ''
Write-Host 'Vault state:' -ForegroundColor Cyan
& near contract call-function as-read-only $Vault get_reviewer json-args '{}' @Net now | Out-Host
& near contract call-function as-read-only $Vault get_balances json-args '{}' @Net now | Out-Host
Write-Host "Done. Explorer: https://testnet.nearblocks.io/address/$Vault" -ForegroundColor Cyan
