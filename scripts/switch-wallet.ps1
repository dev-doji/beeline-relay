<#
.SYNOPSIS
  Your Switch wallet for the real-money test (docs/REAL_MONEY_TEST.md): create it, check its balance,
  and send USDC out of it. Switch wallets pay the network fee in stablecoin, so no ETH is needed.

.DESCRIPTION
  Run this on YOUR PC. The Switch service key is typed at a hidden prompt (or read from the
  SWITCH_API_KEY environment variable) and is never saved or printed.

  Actions:
    Create   makes a new Switch wallet and prints its id and deposit addresses.
             Switch returns the wallet's private key once; this script DISCARDS it without printing or saving it.
             Your Switch service key controls the wallet (Switch can export the key with POST /wallet/export),
             so protect the service key like the wallet's key.
    Balance  shows the wallet's USDC balance.
    Send     sends USDC from the wallet to an address. It shows the fee first and needs two typed
             confirmations (last 6 characters of the recipient, then SEND). Capped at USD 10 per run.

.EXAMPLE
  .\scripts\switch-wallet.ps1 -Action Create
  .\scripts\switch-wallet.ps1 -Action Balance -WalletId 0123456789abcdef01234567
  .\scripts\switch-wallet.ps1 -Action Send -WalletId 0123456789abcdef01234567 -Recipient 0xDepositAddress -Amount 3
#>
param(
  [Parameter(Mandatory = $true)][ValidateSet('Create', 'Balance', 'Send')][string]$Action,
  [ValidatePattern('^[a-f0-9]{24}$')][string]$WalletId,
  [string]$Name = 'BeelineTest',   # Switch only accepts letters and numbers (at least one letter) and rejects crypto-related words
  [string]$Recipient,
  [decimal]$Amount,
  [string]$BaseUrl = 'https://api.onswitch.xyz'   # only changed by tests against a local fake server
)

$ErrorActionPreference = 'Stop'
$CapUsd = 10
$Asset = 'base:usdc'

$key = $env:SWITCH_API_KEY
if (-not $key) {
  $sec = Read-Host 'Paste your Switch service key (hidden, not saved)' -AsSecureString
  $bstr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($sec)
  try { $key = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($bstr) } finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($bstr) }
}
$key = if ($key) { $key.Trim().Trim('"').Trim("'") } else { $key }
if (-not $key) { throw 'No key entered.' }
$headers = @{ 'x-service-key' = $key; 'Accept' = 'application/json' }

function Call($method, $path, $body) {
  $args = @{ Uri = "$BaseUrl$path"; Method = $method; Headers = $headers; TimeoutSec = 60 }
  if ($body) { $args.Body = ($body | ConvertTo-Json -Depth 6); $args.ContentType = 'application/json' }
  try { return Invoke-RestMethod @args }
  catch {
    # Windows PowerShell 5.1 keeps the response body in ErrorDetails; fall back to the stream, then to the plain message.
    $status = ''
    try { $status = [int]$_.Exception.Response.StatusCode } catch {}
    $body = $_.ErrorDetails.Message
    if (-not $body) { try { $body = (New-Object IO.StreamReader($_.Exception.Response.GetResponseStream())).ReadToEnd() } catch {} }
    if (-not $body) { $body = $_.Exception.Message }
    $hint = ''
    if ($status -eq 401 -or $status -eq 403) { $hint = ' (the key was refused: check it is the live service key from the Switch dashboard, copied without spaces, and that this product is enabled for your account)' }
    throw "Switch returned HTTP $status for $method $path`: $body$hint"
  }
}

if ($Action -eq 'Create' -and ($Name -notmatch '^[A-Za-z0-9]{3,40}$' -or $Name -notmatch '[A-Za-z]')) {
  throw "-Name must be 3 to 40 letters and numbers only (no spaces, hyphens or symbols, at least one letter), and must not contain crypto-related words like wallet, usdc or bitcoin. Example: BeelineTest"
}

function NeedWallet { if (-not $WalletId) { throw '-WalletId is required for this action (the id printed by -Action Create).' } }

switch ($Action) {

  'Create' {
    $r = Call 'POST' '/wallet/create' @{ name = $Name }
    $w = $r.data
    if (-not $w -or -not $w.id) { throw 'Switch did not return a wallet.' }
    # The private key is dropped here on purpose: never shown, never written anywhere.
    $w.PSObject.Properties.Remove('private_key')
    Write-Host ''
    Write-Host 'Switch wallet created.' -ForegroundColor Green
    Write-Host ('  Wallet id   : {0}' -f $w.id)
    foreach ($p in $w.address.PSObject.Properties) { Write-Host ('  {0,-12}: {1}' -f $p.Name, $p.Value) }
    Write-Host ''
    Write-Host 'The wallet key was discarded by this script (not shown, not saved). Your Switch service key controls this wallet.' -ForegroundColor Yellow
    $dir = Join-Path (Split-Path -Parent $PSScriptRoot) '.realmoney'
    New-Item -ItemType Directory -Force $dir | Out-Null
    $file = Join-Path $dir 'switch-wallet.json'
    @{ id = $w.id; name = $w.name; address = $w.address; created = (Get-Date).ToString('o') } | ConvertTo-Json -Depth 5 | Set-Content -Encoding utf8 $file
    Write-Host "  (id and addresses saved to $file; git-ignored, holds no key)"
    Write-Host ''
    Write-Host 'Next: buy USDC into it with  .\scripts\switch-onramp.ps1 -WalletId <id> -QuoteOnly'
  }

  'Balance' {
    NeedWallet
    $b = (Call 'GET' "/wallet/$WalletId/balance" $null).data
    Write-Host ''
    Write-Host ('Wallet {0}' -f $WalletId) -ForegroundColor Cyan
    Write-Host ('  Balance: {0} (about USD {1})' -f $b.balance, $b.balance_in_usd)
    foreach ($x in @($b.breakdown)) { if ($x) { Write-Host ('  - {0}' -f ($x | ConvertTo-Json -Compress -Depth 4)) } }
  }

  'Send' {
    NeedWallet
    if (-not $Recipient) { throw '-Recipient is required (a 0x address on Base).' }
    if ($Recipient -notmatch '^0x[0-9a-fA-F]{40}$') { throw 'Recipient must be 0x followed by 40 hex characters (an address on Base).' }
    if (-not ($Amount -gt 0)) { throw '-Amount (USDC) is required and must be above zero.' }
    if ($Amount -gt $CapUsd) { throw "Amount is over the USD $CapUsd cap for this script." }

    $bal = (Call 'GET' "/wallet/$WalletId/balance" $null).data
    if ([decimal]$bal.balance_in_usd -lt $Amount) { throw ('The wallet holds about USD {0}, less than {1}. Nothing was sent.' -f $bal.balance_in_usd, $Amount) }
    $fee = (Call 'POST' '/wallet/fee' @{ wallet_id = $WalletId; amount = $Amount; asset = $Asset }).data
    Write-Host ''
    Write-Host 'About to send, from your Switch wallet:' -ForegroundColor Yellow
    Write-Host ('  Amount   : {0} USDC on Base' -f $Amount)
    Write-Host ('  To       : {0}' -f $Recipient)
    Write-Host ('  Fee      : about USD {0} (paid in stablecoin, no ETH needed)' -f $fee.estimated_fee)
    Write-Host 'Sending to a wrong address or the wrong network loses the money. Compare the address character by character with where it came from.'
    $tail = $Recipient.Substring($Recipient.Length - 6).ToLower()
    $c1 = Read-Host "Type the LAST 6 characters of the recipient ($tail) to confirm it"
    if ($c1.Trim().ToLower() -ne $tail) { throw 'Recipient confirmation did not match. Stopped. Nothing was sent.' }
    $c2 = Read-Host ('Type SEND to send {0} USDC' -f $Amount)
    if ($c2 -cne 'SEND') { throw 'Not confirmed. Stopped. Nothing was sent.' }

    $t = (Call 'POST' '/wallet/transfer' @{ wallet_id = $WalletId; amount = $Amount; asset = $Asset; recipient = $Recipient }).data
    Write-Host ''
    Write-Host ('Submitted. Status: {0}' -f $t.status) -ForegroundColor Green
    if ($t.transaction_hash) { Write-Host ('  Transaction: {0}' -f $t.transaction_hash) }
    if ($t.explorer_url) { Write-Host ('  Explorer   : {0}' -f $t.explorer_url) }
    if ($t.reference) { Write-Host ('  Reference  : {0}' -f $t.reference) }
    Write-Host '  Do NOT send again. Tell Claude the transaction line; I will check the status on the receiving side.'
  }
}
