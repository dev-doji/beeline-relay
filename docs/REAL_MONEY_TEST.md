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

**Step 1: get a few dollars of USDC (real naira through Switch).**
Pay about 4,500 NGN by bank transfer (about $3.25 at the 2026-11-07 rate of 1,382.76 NGN per USD; Nigeria's minimum is $1) and receive about 3 USDC on **Base**, at the owner's own Base wallet address. Switch gives a one-time bank account valid for 30 minutes and an exact amount to send. This is the only step that needs a local bank account. If the owner already holds USDC, skip it.

How to run it (on your own PC; the Switch key is typed at a hidden prompt and never saved):

```powershell
.\scripts\switch-onramp.ps1 -BaseAddress 0xYourBaseAddress -HolderName "Your Name" -QuoteOnly   # see the quote, buy nothing
.\scripts\switch-onramp.ps1 -BaseAddress 0xYourBaseAddress -HolderName "Your Name" -AmountNGN 4500
```

The script asks for two confirmations (the last 6 characters of your address, then the word `BUY`) before it asks Switch for a bank account. It caps each run at USD 10, then watches the status until the USDC arrives. It was tested only against a fake Switch server (quote only, a wrong confirmation, and the full path); it has not talked to the real Switch payment endpoints yet. Switch's compliance checks for a real payer are not documented, so expect that the first real run may ask for something we haven't seen.

**Step 2: bring USDC from Base to NEAR.**
A 1Click quote, Base USDC to NEAR USDC (a dry quote worked on 2026-11-05 with `recipientType: INTENTS`). The owner sends from their Base wallet to the quoted deposit address. Expected loss: well under 1%.

**Step 3: the NEAR to Base payout, the one the vault depends on.**
From the owner's own mainnet NEAR account, with no contract deployed: get a real 1Click quote for 1 USDC, NEAR to Base (`depositType: INTENTS`, `refundType: INTENTS`), then send one `ft_transfer_call` to `intents.near` with `msg` set to the quote's deposit address. The agent prepares the exact command. Then it polls the 1Click status until `SUCCESS`, `REFUNDED` or `FAILED`, and checks the Base transaction on the explorer. This is the same call shape the vault makes, so it verifies the mechanism. Needs about 0.1 NEAR on the owner's mainnet account for gas.

**Step 4 (optional, only if step 3 succeeds): a mainnet vault.**
Deploying the vault contract to mainnet locks roughly 4 to 5 NEAR as storage (the contract is about 0.5 MB, and NEAR locks about 1 NEAR per 100 KB), about $15 to $20 at today's $3.64 per NEAR. It is returned if the account is deleted. This is **not recommended before the hackathon deadline**: step 3 already proves the mechanism, and the README can say plainly that the vault itself was only deployed on testnet.

## Cost estimate

| Item | About |
| --- | --- |
| Naira to USDC through Switch | 4,500 NGN (about $3.25), plus Switch's spread |
| Base to NEAR bridge fee | under $0.05 |
| NEAR to Base payout fee | about $0.01 to $0.03 |
| Gas on Base for step 2 (paid in ETH) | about $1 of ETH in the same wallet (a transaction costs about $0.01 to $0.05) |
| NEAR gas for step 3 | about 0.1 NEAR (about $0.40) |
| **Total at risk** | **about $5, never more than the $10 cap** |

## What the README may say afterwards

Only what the transaction hashes show. If step 3 succeeds: "NEAR USDC to Base USDC through NEAR Intents verified on mainnet with 1 USDC on <date>, from a wallet making the same call the vault makes; the vault itself is deployed on testnet only." If it is refunded or fails, the README says so, with the 1Click status.

## What the owner needs

- A bank account in a country Switch covers (Switch lists 18 countries for our key), if using step 1.
- A Base wallet address (a public address only, for example from MetaMask or Coinbase Wallet), holding about $1 of ETH on Base for the network fee of step 2. Switch sends only USDC, so the ETH has to come from elsewhere.
- A mainnet NEAR account with about 0.1 NEAR for gas (the owner's `goni19.near`).
