<#
.SYNOPSIS
  One-time TESTNET setup so every playground payment is a real token transfer to a real account.

.DESCRIPTION
  Signed by YOUR testnet owner account through the NEAR CLI keychain. Testnet only.
    1. Upgrade the vault to the latest CI build (adds pay_direct; state is kept).
    2. Create three real testnet accounts for the demo (skipped if they exist):
         claw-electronics.<owner>  an approved supplier (Claw Electronics)
         ms-clothing.<owner>       a vendor who sends invoices (MS Clothing)
         scammer.<owner>           the attacker (never approved)
    3. Approve claw-electronics.<owner> on the vault's allowlist.
    4. Register the TESTNET invoice issuer key (signed pay links).
    5. Re-issue Bee's agent key so it may call pay_direct (it still can't call anything else).
  Run after playground-setup.ps1.

.EXAMPLE
  .\scripts\live-setup.ps1 -Owner goni-beeline.testnet
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

if ($Owner -notmatch '\.testnet$') { throw 'This setup is for testnet only: use your .testnet owner account.' }

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

function ArgsFile($name, $obj) {
  New-Item -ItemType Directory -Force $Work | Out-Null
  $path = Join-Path $Work "$name.json"
  [IO.File]::WriteAllText($path, ($obj | ConvertTo-Json -Compress -Depth 5))
  return $path
}

function Test-Account($account) {
  $body = @{ jsonrpc = '2.0'; id = 1; method = 'query'; params = @{ request_type = 'view_account'; finality = 'final'; account_id = $account } } | ConvertTo-Json -Compress
  try {
    $r = Invoke-RestMethod -Method Post -Uri 'https://rpc.testnet.fastnear.com' -ContentType 'application/json' -Body $body
    return $null -ne $r.result
  } catch { return $false }
}

Step 1 "Upgrade $Vault to the latest contract built by CI (adds pay_direct; state is kept)"
$wasmDir = Join-Path $Work 'wasm'
if (Test-Path $wasmDir) { Remove-Item -Recurse -Force $wasmDir }
$runId = & gh run list --repo dev-doji/beeline-relay --branch main --workflow ci.yml --status success --limit 1 --json databaseId -q '.[0].databaseId'
& gh run download $runId --repo dev-doji/beeline-relay --name agent-vault-wasm --dir $wasmDir
Ensure 'Downloading the contract'
& near contract deploy $Vault use-file (Join-Path $wasmDir 'agent_vault.wasm') without-init-call @Net sign-with-keychain send | Out-Host
Ensure 'Upgrading the vault'

Step 2 "Create the demo accounts claw-electronics.$Owner, ms-clothing.$Owner and scammer.$Owner (0.05 test NEAR each)"
foreach ($name in 'claw-electronics', 'ms-clothing', 'scammer') {
  $acct = "$name.$Owner"
  if (Test-Account $acct) {
    Write-Host "$acct already exists, skipping." -ForegroundColor DarkGray
    continue
  }
  & near account create-account fund-myself $acct '0.05 NEAR' autogenerate-new-keypair save-to-keychain sign-as $Owner @Net sign-with-keychain send | Out-Host
  Ensure "Creating $acct"
}

Step 3 "Approve claw-electronics.$Owner on the vault's allowlist"
& near contract call-function as-transaction $Vault add_recipient file-args (ArgsFile 'add-claw' @{ recipient = "claw-electronics.$Owner" }) prepaid-gas '100.0 Tgas' attached-deposit '0 NEAR' sign-as $Owner @Net sign-with-keychain send | Out-Host
Ensure 'Approving Claw Electronics'

Step 4 'Register the TESTNET invoice issuer key (signed pay links)'
Push-Location $Api
& go run ./cmd/vault-testnet invoice-issuer-args | Out-Host
$code = $LASTEXITCODE
Pop-Location
Ensure 'Preparing the invoice issuer key' $code
& near contract call-function as-transaction $Vault set_invoice_issuer file-args (Join-Path $Work 'set-invoice-issuer.json') prepaid-gas '100.0 Tgas' attached-deposit '0 NEAR' sign-as $Owner @Net sign-with-keychain send | Out-Host
Ensure 'Registering the invoice issuer'

Step 5 "Re-issue Bee's agent key so it may call pay_direct as well as pay_quote (and nothing else)"
$agentFile = Join-Path $Work 'agent-key.json'
$AgentPk = (Get-Content $agentFile -Raw | ConvertFrom-Json).public_key
& near contract call-function as-transaction $Vault set_agent_allowance file-args (ArgsFile 'allowance' @{ public_key = $AgentPk; allowance = '3000000000000000000000000' }) prepaid-gas '100.0 Tgas' attached-deposit '0 NEAR' sign-as $Owner @Net sign-with-keychain send | Out-Host
Ensure "Re-issuing Bee's key"

Write-Host ''
& near contract call-function as-read-only $Vault get_recipients json-args '{}' @Net now | Out-Host
Write-Host "Done. Every playground payment now goes to a real testnet account. Explorer: https://testnet.nearblocks.io/address/$Vault" -ForegroundColor Cyan
