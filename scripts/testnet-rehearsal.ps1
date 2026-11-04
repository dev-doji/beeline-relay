<#
.SYNOPSIS
  Guided TESTNET rehearsal of the Beeline agent vault. Uses only testnet accounts and test tokens.

.DESCRIPTION
  Walks through deploying the vault, funding it with test USDC, registering an agent key, one
  allowed payment and four attacks that the contract must reject on-chain. Each step pauses
  so you can read what happens. Transactions are signed by YOUR near CLI (your testnet account,
  approved by you in the browser), never by anyone else.

  1Click does not run on testnet, so quotes are signed by a local TEST signer that stands in for
  1Click. A mainnet vault trusts only 1Click's real key and can never pay these test quotes.

.EXAMPLE
  .\scripts\testnet-rehearsal.ps1 -Owner alice.testnet
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
$DestAsset = 'nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near'
$Allowed = '0x1111111111111111111111111111111111111111'   # allowlisted demo recipient
$Attacker = '0x6666666666666666666666666666666666666666'  # what a prompt-injected agent is told to pay
$Net = @('network-config', 'testnet')

if ($Owner -notmatch '^[a-z0-9_-]+(\.[a-z0-9_-]+)*\.testnet$' -or $Owner -eq 'yourname.testnet') {
  throw "Use YOUR testnet account name, for example -Owner alice.testnet (not the placeholder 'yourname.testnet')."
}

# Stops the rehearsal when a required command fails, so later steps don't run on a broken setup.
function Ensure($what, $code = $global:LASTEXITCODE) {
  if ($code -ne 0) {
    Write-Host "!! $what failed (see the error above). Fix it, then run the script again." -ForegroundColor Red
    exit 1
  }
}

function Step($n, $title) {
  Write-Host ''
  Write-Host "=== Step ${n}: $title ===" -ForegroundColor Cyan
  Read-Host 'Press Enter to run it (Ctrl+C to stop)' | Out-Null
}

# PowerShell 5.1 mangles JSON passed to programs, so every argument goes through a file.
function ArgsFile($name, $obj) {
  New-Item -ItemType Directory -Force $Work | Out-Null
  $path = Join-Path $Work "$name.json"
  [IO.File]::WriteAllText($path, ($obj | ConvertTo-Json -Compress -Depth 5))
  return $path
}

function Call($contract, $method, $argsPath, $signAs, $deposit = '0 NEAR', [switch]$AgentKey) {
  $cmd = @('contract', 'call-function', 'as-transaction', $contract, $method, 'file-args', $argsPath,
    'prepaid-gas', '100.0 Tgas', 'attached-deposit', $deposit, 'sign-as', $signAs) + $Net
  if ($AgentKey) { $cmd += @('sign-with-access-key-file', (Join-Path $Work 'agent-key.json'), 'send') }
  else { $cmd += @('sign-with-keychain', 'send') }
  & near @cmd | Out-Host   # print the CLI output instead of returning it
  return $LASTEXITCODE -eq 0
}

function Expect-Rejected($label, $ok) {
  if ($ok) { Write-Host "!! $label was NOT rejected. This is a bug: stop and report it." -ForegroundColor Red }
  else { Write-Host "OK: $label was rejected on-chain (see the BEELINE_VAULT_REJECTED reason above)." -ForegroundColor Green }
}

foreach ($tool in 'near', 'gh', 'go') {
  if (-not (Get-Command $tool -ErrorAction SilentlyContinue)) {
    throw "'$tool' is not installed. See docs/TESTNET_REHEARSAL.md, section 'Before you start'."
  }
}

Step 1 "Connect your testnet account ($Owner) to the NEAR CLI (a browser window asks you to approve)"
& near account import-account using-web-wallet @Net
Ensure 'Connecting your account'

Step 2 "Create the vault account $Vault with 4 test NEAR from $Owner"
& near account create-account fund-myself $Vault '4 NEAR' autogenerate-new-keypair save-to-keychain sign-as $Owner @Net sign-with-keychain send
Ensure "Creating $Vault (if it already exists from an earlier run, use a new -Owner account)"

Step 3 'Download the contract built by CI (agent_vault.wasm) from GitHub'
$wasmDir = Join-Path $Work 'wasm'
if (Test-Path $wasmDir) { Remove-Item -Recurse -Force $wasmDir }
& gh run download --repo dev-doji/beeline-relay --name agent-vault-wasm --dir $wasmDir
Ensure 'Downloading the contract'
$Wasm = Join-Path $wasmDir 'agent_vault.wasm'

Step 4 'Create the agent key and the TEST quote signer'
Push-Location $Api
$keys = & go run ./cmd/vault-testnet keys -vault $Vault
$keysOk = $LASTEXITCODE
Pop-Location
Ensure 'Creating keys' $keysOk
$Signer = ($keys | Select-String 'TEST_QUOTE_SIGNER=(.+)').Matches[0].Groups[1].Value
$AgentPk = ($keys | Select-String 'AGENT_PUBLIC_KEY=(.+)').Matches[0].Groups[1].Value
Write-Host "Test quote signer: $Signer"
Write-Host "Agent public key:  $AgentPk"

Step 5 'Deploy and initialize the vault (limits: 5 USDC per payment, 8 USDC per agent per day)'
$init = ArgsFile 'init' @{
  owner = $Owner; token = $Token; quote_signer = $Signer
  max_per_payment = '5000000'; daily_limit_per_agent = '8000000'
  allowed_recipients = @($Allowed); allowed_destination_assets = @($DestAsset)
}
& near contract deploy $Vault use-file $Wasm with-init-call new file-args $init prepaid-gas '100.0 Tgas' attached-deposit '0 NEAR' @Net sign-with-keychain send
Ensure 'Deploying the vault'

Step 6 'Register the vault with the test USDC token and mint 20 test USDC into it'
$reg = ArgsFile 'storage' @{ account_id = $Vault }
Call $Token 'storage_deposit' $reg $Owner '0.00125 NEAR' | Out-Null
$mint = ArgsFile 'mint' @{ account_id = $Vault; amount = '20000000' }
if (-not (Call $Token 'mint' $mint $Owner)) {
  Write-Host "Minting failed. Send test USDC to $Vault another way, then continue." -ForegroundColor Yellow
}

Step 7 'Register the agent: the vault gives it a key that can ONLY call pay_quote'
$agent = ArgsFile 'add-agent' @{ public_key = $AgentPk; label = 'invoice-agent' }
if (-not (Call $Vault 'add_agent' $agent $Owner)) { Ensure 'Registering the agent' 1 }

Push-Location $Api
& go run ./cmd/vault-testnet quote -vault $Vault -recipient $Allowed -amount 1000000 -name allowed | Out-Host
& go run ./cmd/vault-testnet quote -vault $Vault -recipient $Attacker -amount 1000000 -name attacker | Out-Host
& go run ./cmd/vault-testnet quote -vault $Vault -recipient $Allowed -amount 1000000 -name forged -forged | Out-Host
& go run ./cmd/vault-testnet quote -vault $Vault -recipient $Allowed -amount 6000000 -name over-limit | Out-Host
Pop-Location

Step 8 'The agent pays 1 test USDC to the ALLOWED recipient (should succeed)'
if (Call $Vault 'pay_quote' (Join-Path $Work 'pay-allowed.json') $Vault -AgentKey) {
  Write-Host 'OK: allowed payment went through.' -ForegroundColor Green
} else {
  Write-Host '!! The allowed payment failed. Stop here and share the output.' -ForegroundColor Red
}

Step 9 'ATTACK: a prompt-injected agent tries to pay the attacker (must be rejected)'
Expect-Rejected 'Payment to a non-allowlisted recipient' (Call $Vault 'pay_quote' (Join-Path $Work 'pay-attacker.json') $Vault -AgentKey)

Step 10 'ATTACK: a forged quote, not signed by the trusted signer (must be rejected)'
Expect-Rejected 'Forged quote' (Call $Vault 'pay_quote' (Join-Path $Work 'pay-forged.json') $Vault -AgentKey)

Step 11 'ATTACK: 6 USDC, above the 5 USDC per-payment limit (must be rejected)'
Expect-Rejected 'Over-limit payment' (Call $Vault 'pay_quote' (Join-Path $Work 'pay-over-limit.json') $Vault -AgentKey)

Step 12 'ATTACK: replay the already-paid quote (must be rejected)'
Expect-Rejected 'Replayed quote' (Call $Vault 'pay_quote' (Join-Path $Work 'pay-allowed.json') $Vault -AgentKey)

Step 13 "Show the vault's view of the agent (1 payment, 1 USDC spent today)"
& near contract call-function as-read-only $Vault get_agents json-args '{}' @Net now

Write-Host ''
Write-Host "Rehearsal finished. Explorer: https://testnet.nearblocks.io/address/$Vault" -ForegroundColor Cyan
