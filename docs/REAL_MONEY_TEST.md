# Real-money test plan (not run yet)

Status: **plan only. No real money has moved in this project.** Written 2026-11-07. Everything else in the README ran on NEAR testnet with test USDC.

## What this test must prove

Two things in the README are still unproven because they need real money:

1. **NEAR Intents delivers.** The vault's `INTENTS` rail deposits into the Intents verifier *on behalf of* a 1Click deposit address (`ft_transfer_call` to `intents.near`, `msg` = deposit address). We have not verified that 1Click detects such a deposit and pays out on the destination chain (`docs/NEAR_INTENTS_ROUTE.md`, section 6b, "Not verified yet").
2. **Local currency can fund the vault's route.** Switch turns naira (or another local currency) into USDC on Base. Switch doesn't support NEAR, so USDC has to cross from Base to NEAR through NEAR Intents.

## Rules for the test

- **A hard cap of about $10 in total.** Every step below is a few dollars at most.
- **Nobody types a private key into chat or into this repository.** The owner signs every transaction in their own wallet or CLI. The coding agent prepares quotes and commands and reads public chain data.
- **Each step needs the owner's explicit yes** before real money moves. The agent states the amount, the recipient and the fee first.
- **Stop on the first surprise.** An unexpected error or an amount that doesn't match the quote means stopping, not retrying. Value-moving steps are never blindly retried (AGENTS.md): check the 1Click status and the explorer first.
- **Record only what happened.** Transaction hashes go in the README. Anything that fails is written up as a failure.

## Steps

**Step 0: create a Switch wallet** (decided 2026-11-12).
A Switch wallet holds USDC and **sends it to any address with the network fee paid in stablecoin: no ETH and no gas sponsorship to buy** (Switch's transfer endpoint: "No bridging, no swapping or gas tokens required"). That replaces the Privy and MetaMask routes for this test. The wallet's private key is returned by Switch only once; `switch-wallet.ps1` discards it without showing or saving it, and the wallet is controlled by the Switch service key (Switch can export the key with `POST /wallet/export`), so the service key must be protected like the wallet's key. Fine for a test of about $5, not for holding real balances.

```powershell
.\scripts\switch-wallet.ps1 -Action Create        # prints the wallet id and its Base address
```

**Step 1: get a few dollars of USDC (real naira through Switch).**
Pay about 4,500 NGN by bank transfer (about $3.25 at the 2026-11-07 rate of 1,382.76 NGN per USD; Nigeria's minimum is $1) and receive about 3 USDC on **Base**, credited to the Switch wallet. Switch gives a one-time bank account valid for 30 minutes and an exact amount to send. This is the only step that needs a local bank account. If the owner already holds USDC, skip it.

```powershell
.\scripts\switch-onramp.ps1 -WalletId <wallet id> -QuoteOnly   # see the quote, buy nothing
.\scripts\switch-onramp.ps1 -WalletId <wallet id> -AmountNGN 4500
.\scripts\switch-wallet.ps1 -Action Balance -WalletId <wallet id>
```

Both scripts run on your own PC; the Switch key is typed at a hidden prompt and never saved. `switch-onramp.ps1` asks for two confirmations (the last 6 characters of the destination, then the word `BUY`) before it asks Switch for a bank account, caps each run at USD 10, and watches the status until the USDC arrives. They were tested only against a fake Switch server (quote only, wrong confirmations, the full paths, and that the fake private key never reaches the output or any file); they have not talked to the real Switch payment or wallet endpoints yet. Switch's compliance checks for a real payer are not documented, so expect that the first real run may ask for something we haven't seen.

**Step 2: bring USDC from Base to NEAR.**
A 1Click quote, Base USDC to NEAR USDC (a dry quote worked on 2026-11-05 with `recipientType: INTENTS`). The owner sends it from the Switch wallet to the quoted deposit address:

```powershell
.\scripts\switch-wallet.ps1 -Action Send -WalletId <wallet id> -Recipient <deposit address> -Amount 3
```

It shows the fee first and needs the last 6 characters of the recipient and the word `SEND`. Expected loss: well under 1%. **Open question:** with `recipientType: INTENTS` the USDC is credited inside `intents.near` to the owner's account, not as a USDC token balance on NEAR. Step 3 as written needs the token balance, so there may be one extra hop (an `ft_withdraw` from `intents.near`, which needs 1 yoctoNEAR and a little gas) or a dry quote may show a direct NEAR delivery works now (on 2026-11-05 it did not). To be settled with dry quotes before any money moves in step 2.

**Step 3: the NEAR to Base payout, the one the vault depends on.**
From the owner's own mainnet NEAR account, with no contract deployed: get a real 1Click quote for 1 USDC, NEAR to Base (`depositType: INTENTS`, `refundType: INTENTS`), then send one `ft_transfer_call` to `intents.near` with `msg` set to the quote's deposit address. The agent prepares the exact command. Then it polls the 1Click status until `SUCCESS`, `REFUNDED` or `FAILED`, and checks the Base transaction on the explorer. This is the same call shape the vault makes, so it verifies the mechanism. Needs about 0.1 NEAR on the owner's mainnet account for gas.

**Step 4 (optional, only if step 3 succeeds): a mainnet vault.**
Deploying the vault contract to mainnet locks roughly 4 to 5 NEAR as storage (the contract is about 0.5 MB, and NEAR locks about 1 NEAR per 100 KB), about $15 to $20 at today's $3.64 per NEAR. It is returned if the account is deleted. This is **not recommended before the hackathon deadline**: step 3 already proves the mechanism, and the README can say plainly that the vault itself was only deployed on testnet.

## Optional: the Privy wallet

The Privy panel on `/fund` (docs/DEPLOY.md) is not needed for this test. Its sponsored sends need Privy credits that the owner has not bought, so a send from it fails and moves nothing. It can still receive USDC.

## Cost estimate

| Item | About |
| --- | --- |
| Naira to USDC through Switch | 4,500 NGN (about $3.25), plus Switch's spread |
| Base to NEAR bridge fee | under $0.05 |
| NEAR to Base payout fee | about $0.01 to $0.03 |
| Switch wallet send in step 2 | the fee Switch quotes (estimated in USD, paid in stablecoin; a few cents to start with, shown before you confirm) |
| NEAR gas for step 3 | about 0.1 NEAR (about $0.40) |
| **Total at risk** | **about $4 to $5, never more than the $10 cap** |

## What the README may say afterwards

Only what the transaction hashes show. If step 3 succeeds: "NEAR USDC to Base USDC through NEAR Intents verified on mainnet with 1 USDC on <date>, from a wallet making the same call the vault makes; the vault itself is deployed on testnet only." If it is refunded or fails, the README says so, with the 1Click status.

## What the owner needs

- A bank account in a country Switch covers (Switch lists 18 countries for our key), if using step 1.
- Your Switch service key, typed only into your own terminal (never into chat). No separate wallet or ETH is needed: the Switch wallet is created by the script.
- A mainnet NEAR account with about 0.1 NEAR for gas (the owner's `goni19.near`).
