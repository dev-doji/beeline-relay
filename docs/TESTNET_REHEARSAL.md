# Testnet rehearsal: agent vault

A free dress rehearsal of the vault on NEAR **testnet**, before any real money is involved. Everything uses test NEAR and a test USDC token (`usdc.fakes.testnet`).

**What it proves:**
- the vault deploys and initializes;
- an agent key can pay an allowed recipient;
- the contract **rejects four attacks on-chain**, each with a public explorer link:
  1. paying a recipient that isn't on the allowlist;
  2. a forged quote;
  3. paying over the limit;
  4. replaying a quote that was already paid.

**What it doesn't prove:** the real 1Click flow. 1Click has no testnet, so quotes here are signed by a local **test signer** standing in for 1Click, and the test deposit accounts don't swap anything. The real 1Click signature check is covered by unit tests against a live quote, and the real route is exercised on mainnet in a later step.

## Before you start (one time)

1. **Create a funded testnet account directly in the NEAR CLI** (recommended). The faucet gives it free test NEAR and the CLI keeps its key, so no browser step is needed:
   ```powershell
   near account create-account sponsor-by-faucet-service yourname-beeline.testnet autogenerate-new-keypair save-to-keychain network-config testnet create
   ```
   Then run the rehearsal with `-SkipImport`. A wallet account from testnet.mynearwallet.com also works through the browser import in step 1, but only if it holds more than 4 test NEAR (new wallet accounts can start with 0).
2. **Install the NEAR CLI.** In PowerShell, run the official installer from NEAR's GitHub releases:
   ```powershell
   irm https://github.com/near/near-cli-rs/releases/latest/download/near-cli-rs-installer.ps1 | iex
   ```
   Close and reopen PowerShell afterwards, then check that `near --version` prints a version.
3. **Check the other tools.** The GitHub CLI (`gh`) and Go are already installed on this machine.

## Run it

From the repository folder:

```powershell
.\scripts\testnet-rehearsal.ps1 -Owner yourname-beeline.testnet -SkipImport
```

The script pauses before every step. In step 1 a browser window opens and **you** approve the connection in your wallet, so the CLI signs only with your approval.

| Step | What happens | Expected |
|---|---|---|
| 1 | Connect your testnet account to the CLI | Browser approval |
| 2 | Create `vault.yourname.testnet` with 4 test NEAR (skipped if it already exists, so the script can be rerun) | Success |
| 2b | Top the vault up with 2 test NEAR if it holds less than 5.5 (the ~335 KB contract locks about 3.4 NEAR of storage) | Success |
| 3 | Download the CI-built contract | Success |
| 4 | Create the agent key and test signer (saved in `beeline-api/.testnet/`, which git ignores) | Prints two public keys |
| 5 | Deploy and initialize the vault | Success |
| 6 | Register the vault with test USDC, mint 20 test USDC, then `sync_balance` credits it to the vault's pool | Success; `get_balances` shows `pool: 20000000` |
| 7 | Register the agent (key limited to `pay_quote`) | Success |
| 8 | Agent pays 1 USDC to the allowed recipient | ✅ Success |
| 9 | Prompt-injected payment to an attacker | ❌ Rejected: "not on the allowlist" |
| 10 | Forged quote | ❌ Rejected: "signature is not from 1Click" |
| 11 | 6 USDC, over the 5 USDC limit | ❌ Rejected: "exceeds the per-payment limit" |
| 12 | Replay of the step 8 quote | ❌ Rejected: "already been paid" |
| 13 | Show the agent's spend | 1 payment, 1 USDC |

The explorer for your vault is `https://testnet.nearblocks.io/address/vault.yourname.testnet`.

## Results: first full run (2026-11-05)

The rehearsal ran end to end on NEAR testnet against `vault.goni-beeline.testnet`, using the wasm built by CI run 41996517700 (commit `ecd44c6`). Owner: `goni-beeline.testnet`. Token: `usdc.fakes.testnet`. Quotes were TEST-signed (1Click has no testnet). The agent signed with its function-call key, which can only call `pay_quote`.

| Step | Transaction | On-chain result |
|---|---|---|
| Deploy and initialize the vault | [9xjUUKjTiU…](https://testnet.nearblocks.io/txns/9xjUUKjTiUVkQ3gzLDc6jwycVPkvq8n2oEuYFEGkKWaf) | ✅ Success |
| Agent pays 1 test USDC to the allowlisted recipient | [C82h5aBx6b…](https://testnet.nearblocks.io/txns/C82h5aBx6bXtVeKM8R6RPHqdL4ZD8NHVy1FeceGCyCea) | ✅ Paid (`payment_requested` → token transfer → `payment_sent`) |
| Attack: prompt-injected payment to `0x6666…` | [6MY4PeZXxN…](https://testnet.nearblocks.io/txns/6MY4PeZXxNDt6kavkxgekGqa11uof1wEb2DVakmnVGGE) | ❌ `BEELINE_VAULT_REJECTED: recipient 0x6666… is not on the allowlist` |
| Attack: forged quote (signed by a different key) | [AjpeNfacc8…](https://testnet.nearblocks.io/txns/AjpeNfacc842Eevk5ZXcp7UJppmnFjTTqSFBAbHvgWEe) | ❌ `BEELINE_VAULT_REJECTED: quote signature is not from 1Click` |
| Attack: 6 USDC, over the 5 USDC limit | [9cT4DFf1En…](https://testnet.nearblocks.io/txns/9cT4DFf1EnprRmTxZT47HB1FMZHRNTdXGYSmGtkptf5D) | ❌ `BEELINE_VAULT_REJECTED: amount 6000000 exceeds the per-payment limit 5000000` |
| Attack: replay of the already-paid quote | [FFT7NMBgFa…](https://testnet.nearblocks.io/txns/FFT7NMBgFaiwAdk5epHToWskLnVP9dxaixwRVUdd93C1) | ❌ `BEELINE_VAULT_REJECTED: this deposit address has already been paid` |

Afterwards `get_agents` showed the agent with 1 payment and 1,000,000 (1 test USDC) spent today. The rejected attacks moved nothing. All transactions: https://testnet.nearblocks.io/address/vault.goni-beeline.testnet

## Status

The script was written on 2026-11-04 and **passed its first full run on 2026-11-05** (results above). The near CLI command syntax is based on near-cli-rs. If a command's wording has changed, the CLI prints the accepted options; share the output and we'll adjust the script.
