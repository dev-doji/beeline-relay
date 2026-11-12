<#
.SYNOPSIS
  Step 1 of docs/REAL_MONEY_TEST.md: buy a few dollars of USDC on Base with REAL naira through Switch.

.DESCRIPTION
  Run this on YOUR PC. It does nothing until you type confirmations:
    1. asks for your Switch service key (typed here, hidden, never saved or printed);
    2. fetches a live quote and shows exactly what you pay and receive, and where it goes;
    3. only after you confirm (twice) does it ask Switch for a bank account to pay into;
    4. prints that bank account and exact amount, then watches the status until it completes.
  Nothing is ever signed with a private key here. USDC is sent by Switch to the Base address you give.
  A hard cap of USD 10 per run. Use -QuoteOnly to see the quote and stop.

.PARAMETER WalletId
  The id of a Switch wallet you created with scripts/switch-wallet.ps1 -Action Create (24 hex characters).
  USDC is credited to that wallet. Switch wallets can send USDC onward with the fee paid in stablecoin,
  so no ETH is needed. This is the recommended route for the real-money test.

.PARAMETER BaseAddress
  Alternative to -WalletId: YOUR OWN wallet address on the BASE network (0x + 40 hex characters).
  A wrong address loses the money.

.PARAMETER HolderName
  With -BaseAddress only: your name as the recipient label for the wallet.

.EXAMPLE
  .\scripts\switch-onramp.ps1 -WalletId 0123456789abcdef01234567 -QuoteOnly
  .\scripts\switch-onramp.ps1 -WalletId 0123456789abcdef01234567 -AmountNGN 4500
  .\scripts\switch-onramp.ps1 -BaseAddress 0xYourBaseAddress -HolderName "Your Name" -QuoteOnly
#>
[CmdletBinding(DefaultParameterSetName = 'Wallet')]
param(
  [Parameter(ParameterSetName = 'Wallet', Mandatory = $true)][ValidatePattern('^[a-f0-9]{24}$')][string]$WalletId,
  [Parameter(ParameterSetName = 'Address', Mandatory = $true)][string]$BaseAddress,
  [Parameter(ParameterSetName = 'Address', Mandatory = $true)][string]$HolderName,
  [ValidateRange(1000, 20000)][decimal]$AmountNGN = 4500,
  [switch]$QuoteOnly,
  [string]$BaseUrl = 'https://api.onswitch.xyz'   # only changed by tests against a local fake server
)

$ErrorActionPreference = 'Stop'
$CapUsd = 10
$Asset = 'base:usdc'
$Country = 'NG'

$useWallet = $PSCmdlet.ParameterSetName -eq 'Wallet'
if (-not $useWallet) {
  if ($BaseAddress -notmatch '^0x[0-9a-fA-F]{40}$') { throw 'BaseAddress must be 0x followed by 40 hex characters (an EVM address).' }
  if ($HolderName.Trim().Length -lt 2) { throw 'HolderName is required.' }
}

# --- the key: from the environment (tests) or typed hidden. Never written anywhere. ---
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
  $args = @{ Uri = "$BaseUrl$path"; Method = $method; Headers = $headers; TimeoutSec = 40 }
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

# --- 1. live quote (moves no money) ---
Write-Host ''
Write-Host 'Asking Switch for a live quote (no money moves)...' -ForegroundColor Cyan
$q = (Call 'POST' '/onramp/quote' @{ amount = $AmountNGN; country = $Country; asset = $Asset; channel = 'BANK' }).data
if (-not $q) { throw 'Switch returned no quote.' }
$usd = [decimal]$q.destination.amount_usd
Write-Host ('  You pay      : {0:N2} {1}  (bank transfer)' -f $q.source.amount, $q.source.currency)
Write-Host ('  You receive  : {0:N2} {1} on {2}  (about USD {3:N2})' -f $q.destination.amount, $q.destination.currency, $q.destination.network, $usd)
Write-Host ('  Rate         : 1 USD = {0:N2} {1}' -f $q.rate, $q.source.currency)
Write-Host ('  Settles in   : {0}' -f $q.settlement)
Write-Host ('  Quote expires: {0}' -f $q.expiry)
if ($usd -gt $CapUsd) { throw "This is USD $usd, over the USD $CapUsd cap. Lower -AmountNGN." }
if ($usd -lt 1.0) { throw ('This would deliver about USD {0:N2}, below Switch''s USD 1 minimum for Nigeria. Raise -AmountNGN.' -f $usd) }
if ($QuoteOnly) { Write-Host "`nQuote only: stopping here. Nothing was bought." -ForegroundColor Green; return }

# --- 2. explicit confirmation ---
Write-Host ''
if ($useWallet) {
  $dest = $WalletId
  $baseAddr = ''
  try { $baseAddr = (Call 'GET' "/wallet/$WalletId" $null).data.address.BASE } catch { Write-Host "  (could not read the wallet's address: $_)" }
  Write-Host 'USDC will be credited by Switch to YOUR Switch wallet:' -ForegroundColor Yellow
  Write-Host "  wallet id $WalletId" -ForegroundColor Yellow
  if ($baseAddr) { Write-Host "  Base address $baseAddr" -ForegroundColor Yellow }
  Write-Host 'This must be the wallet you created with scripts/switch-wallet.ps1. Check the id against what that script printed.'
} else {
  $dest = $BaseAddress
  Write-Host 'USDC will be sent by Switch to:' -ForegroundColor Yellow
  Write-Host "  $BaseAddress   on the BASE network" -ForegroundColor Yellow
  Write-Host 'If this is not YOUR wallet, or not a Base address, the money is lost. Open your wallet and compare it character by character.'
}
$tail = $dest.Substring($dest.Length - 6).ToLower()
$c1 = Read-Host "Type the LAST 6 characters ($tail) to confirm the destination"
if ($c1.Trim().ToLower() -ne $tail) { throw 'Destination confirmation did not match. Stopped. Nothing was bought.' }
$c2 = Read-Host ('Type BUY to ask Switch for a bank account to pay {0:N2} NGN into' -f $q.source.amount)
if ($c2 -cne 'BUY') { throw 'Not confirmed. Stopped. Nothing was bought.' }

# --- 3. initiate ---
$ref = [guid]::NewGuid().ToString()
$beneficiary = if ($useWallet) { @{ wallet = $WalletId } } else { @{ holder_type = 'INDIVIDUAL'; holder_name = $HolderName.Trim(); wallet_address = $BaseAddress } }
$resp = Call 'POST' '/onramp/initiate' @{
  amount = $AmountNGN; country = $Country; asset = $Asset; channel = 'BANK'; reference = $ref
  beneficiary = $beneficiary
}
$d = $resp.data
if (-not $d -or -not $d.deposit) { throw 'Switch did not return a bank account to pay.' }
$dep = $d.deposit
Write-Host ''
Write-Host 'PAY THIS FROM YOUR OWN BANK APP, ONCE, WITHIN 30 MINUTES:' -ForegroundColor Green
Write-Host ('  Bank          : {0}' -f $dep.bank_name)
Write-Host ('  Account number: {0}' -f $dep.account_number)
Write-Host ('  Account name  : {0}' -f $dep.account_name)
Write-Host ('  EXACT amount  : {0} {1}' -f $dep.amount, $q.source.currency)
Write-Host ('  Expires       : {0}' -f $dep.expires_at)
foreach ($n in @($dep.note)) { if ($n) { Write-Host ('  Note          : {0}' -f $n) } }
Write-Host '  This account works once. A different amount, or paying twice, may not be credited automatically.'
Write-Host ('  Reference     : {0}' -f $ref)

$logDir = Join-Path (Split-Path -Parent $PSScriptRoot) '.realmoney'
New-Item -ItemType Directory -Force $logDir | Out-Null
$log = Join-Path $logDir "switch-onramp-$ref.json"
@{ reference = $ref; destination = $dest; amount_ngn = $AmountNGN; started = (Get-Date).ToString('o'); quote = $q; deposit = $dep } | ConvertTo-Json -Depth 8 | Set-Content -Encoding utf8 $log
Write-Host "  (details saved to $log, which is git-ignored and holds no key)"

# --- 4. watch ---
Write-Host ''
Write-Host 'Waiting for your payment. Leave this window open (it stops by itself after 35 minutes).' -ForegroundColor Cyan
$last = ''
$deadline = (Get-Date).AddMinutes(35)
while ((Get-Date) -lt $deadline) {
  Start-Sleep -Seconds 10
  try { $s = (Call 'GET' "/payment/status?reference=$ref" $null).data } catch { Write-Host "  (status check failed, retrying: $_)"; continue }
  if ($s.status -ne $last) { Write-Host ('  [{0}] status: {1}' -f (Get-Date -Format 'HH:mm:ss'), $s.status); $last = $s.status }
  if ($s.status -eq 'COMPLETED') {
    Write-Host ''
    Write-Host ('DONE. Received {0} {1} on {2}.' -f $s.destination.amount, $s.destination.currency, $s.destination.network) -ForegroundColor Green
    if ($s.meta.hash) { Write-Host ('  Transaction: {0}' -f $s.meta.hash) }
    if ($s.meta.explorer_url) { Write-Host ('  Explorer   : {0}' -f $s.meta.explorer_url) }
    Write-Host '  Tell Claude "step 1 done". Check the balance with: .\scripts\switch-wallet.ps1 -Action Balance -WalletId <id>'
    return
  }
  if ($s.status -eq 'FAILED') { throw "Switch reports FAILED for reference $ref. Do not pay again. Tell Claude the reference." }
}
Write-Host "No completion within 35 minutes. Do NOT pay again. Check the status later with reference $ref and tell Claude." -ForegroundColor Yellow
