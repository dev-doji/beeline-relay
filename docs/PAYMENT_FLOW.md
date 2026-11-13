# Real payment flow: a customer pays from their own wallet

Status: **Phase 1 built 2026-11-13** (verifier `internal/chainverify`, claim API and tables, the pay sheet, vendor evidence and
security events). **Not yet verified with a real signed wallet payment.** Phases 2 and 3 are not built. The old test-agent
payment stays as a labelled "Demo payment". Docs checked on 2026-11-13: NEAR RPC `tx`
(<https://docs.near.org/api/rpc/transactions>), NEP-141 (<https://github.com/near/NEPs/blob/master/neps/nep-0141.md>),
NEAR Connect (<https://docs.near.org/web3-apps/concepts/web-login>).

## The one rule

**An invoice is paid only when Beeline's server has independently read the payment from the chain.** The customer's
browser, a pasted transaction hash, a wallet's "success" screen and our own front end can never mark an invoice paid.
This is how "fake blockchain activity" is trapped: a claim is checked against the chain, and anything that does not
match is refused and recorded as a security event on the vendor's invoice.

## Who and what

| Actor | Needs |
|---|---|
| Customer | One clear "Pay 2 USDC" button, a way to pay from the wallet they already have, and plain feedback |
| Vendor | To know, without confusion, that money arrived for this invoice, and to see attempts that did not match |
| Beeline | A signed invoice (exists), a chain check (new), a record of evidence (new) |

## Findings from real transactions (2026-11-13)

Checked against a real paid-invoice transaction from this project's README (`E4kDMYEn9ijyG2WVcW7f6GFhqPb6Lbhi55xu1RpGAgp9`):

- The NEAR `tx` lookup with `wait_until: FINAL` works and returns the full outcome (statuses and logs per receipt).
- A normal (non-archival) RPC node **timed out** ("NOT_OBSERVED") for this older transaction; the archival endpoints
  (`archival-rpc.testnet.fastnear.com` and `archival-rpc.testnet.near.org`) answered. The verifier must use an archival-capable
  RPC for anything but brand-new transactions.
- The testnet token `usdc.fakes.testnet` logged **no standard NEP-297 `ft_transfer` event** for the transfer. So verification
  cannot rely on events; it reads the transaction's own `ft_transfer` call and the success of its receipts.
- The vault's own events (`payment_sent`, `review_approved`) are there, which is what Path B's evidence will use.

## Honest limit, and the two paths

A plain transfer from a customer's own wallet cannot be stopped by Beeline before it happens: the customer's wallet
signs it and the token contract executes it. So there are two paths with different guarantees:

| Path | Who it is for | What Beeline can guarantee |
|---|---|---|
| **A. Direct payment** (a normal USDC transfer from any wallet) | Every customer, today | Beeline refuses to count anything that does not match the invoice, and warns before the customer pays. It cannot reverse a transfer that already happened. |
| **B. Protected wallet** (the customer's money sits in a Beeline vault) | AI agents and businesses | The vault contract refuses a non-matching payment **before money moves** (this is the existing `agent-vault` contract: signed invoice, exact payee and amount, once, limits, optional AI review) |

Path A is the product's front door. Path B is the premium protection and the Agent Economy story. Build A first.

## Path A: direct payment

### Customer experience

1. The customer opens `/pay/<id>`. The page already shows the signed invoice and "Protected by Beeline".
2. **Pay 2 USDC** opens a small sheet with two choices:
   - **Connect NEAR wallet** (primary): connect with NEAR Connect (`@hot-labs/near-connect`), review, approve in the wallet.
   - **Send from any wallet or exchange**: shows the vendor's account, the exact amount and the **memo to include**, with a copy button and a QR code.
3. After paying: "Payment received, confirming on NEAR" (a few seconds), then "Paid" with a link to the transaction.
4. If something does not match: a plain sentence ("This payment used a different token, so we did not count it. Contact the vendor.") and the vendor sees it too.

### The payment

- Token: the invoice's asset, as a canonical contract ID per network (mainnet native USDC
  `17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1`, testnet `usdc.fakes.testnet`), never a symbol.
- Call: `ft_transfer(receiver_id = vendor payout, amount = exact base units, memo = "beeline:<invoice id>")` with the
  required **1 yoctoNEAR** deposit (NEP-141).
- The memo **binds the payment to one invoice**.

### Verification (server, read-only)

`POST /api/v1/invoices/{id}/claims` with `{tx_hash, sender_account_id}`. The server looks the transaction up with the
NEAR `tx` RPC (`wait_until: FINAL`, polling at a lower level while waiting, because the strict levels can take several
seconds) and accepts only if **all** hold:

1. the transaction **exists** and its status is a success, and every receipt in its outcome succeeded;
2. the transaction is a **direct call to the canonical token contract** for the invoice's asset (its `receiver_id` is that
   contract; a look-alike USDC contract fails here);
3. its single action is `ft_transfer` with `receiver_id` = the invoice's payout, `amount` = the invoice amount exactly and
   `memo` = `beeline:<invoice id>`, attached deposit 1 yoctoNEAR. Beeline reads these from the **transaction itself**, not
   from event logs (see Findings). If the token also emits a NEP-297 `ft_transfer` event, it is used as a cross-check only;
4. the signer matches `sender_account_id`;
5. the transaction hash is **new** (unique in the database) and the invoice is **open** and not expired at the block's time;
6. the block is **final**.

Then, in one database transaction, the invoice moves `open -> paid` and the evidence is stored. Anything else is refused
with a typed reason and recorded as a **security event** on that invoice. The endpoint is idempotent per
`(invoice, tx_hash)`: repeating a claim returns the same answer.

### State model

```
open --claim seen--> confirming --verified & final--> paid
open --expires--> expired
claims that do not match: stay as security events; the invoice state does not change
```

`paid` is never accepted from a client. Provider-reported facts (the RPC's answer) are labelled separately from
anything we infer.

### Threats this closes

| Attack | Result |
|---|---|
| Pasting a made-up transaction hash | No such transaction on the chain: refused |
| Paying with a look-alike token contract | Not the canonical contract: refused |
| Right token, wrong receiver or amount | The call's arguments do not match the invoice: refused |
| Paying through a wrapper (multisig, smart-contract wallet, `ft_transfer_call`) | Not a direct `ft_transfer` in v1: refused, with an explanation, until each wrapper is supported on purpose |
| Reusing one real payment for a second invoice | Memo names the first invoice and the hash is already used: refused |
| Claiming before the chain is final, or a failed transaction | Needs a success status and a final block |
| Editing the pay link (payee or amount) | The server reads the invoice from its own signed record, not from the link |
| Our own API told "paid" by a client | There is no such input |
| One RPC provider lying | Open item: confirm with a second independent RPC endpoint before marking paid |

### Vendor experience

The invoice view in the app shows: **Payment: verified on-chain** with the transaction, block and time; and the
**Security events** list (wrong token, wrong amount, wrong receiver, reused transaction, expired invoice), each with a
plain reason. The sidebar stays.

## Path B: protected wallet (later)

The customer (or their AI agent) funds a Beeline vault and pays through `pay_quote`. The contract already enforces the
signed invoice, exact payee and amount, once, limits and an optional AI review, and refuses before money moves. To offer
this to ordinary users we need: a vault per user (deployed from a factory), signing with the user's Privy NEAR wallet
(Privy signs raw hashes on NEAR; it does not broadcast, so the app builds and sends the transaction), and a funding
step. **Unknown, must be measured first:** the cost of deploying a vault per user and how a user's funds get into it.

## Phases

| Phase | What | Needs |
|---|---|---|
| 1 | `invoice_payments` table and the claim verification above; wallet connect with NEAR Connect on testnet; the "any wallet" instructions with memo; vendor payment evidence and security events. Keep a clearly labelled **Demo payment** button for judges, because they have no testnet USDC. | Nothing external |
| 2 | Auto-detect payments without a pasted hash (needs a way to watch the vendor's account); second-RPC confirmation; vendor payout registration on the token contract | An indexer or account watcher: not chosen |
| 3 | Protected wallet for payers (Path B) | Vault-per-user cost and funding flow decided |

## Open questions (need an answer or a check)

1. **Receiver registration.** NEP-141 defers storage accounting to NEP-145; standard USDC contracts normally reject a
   transfer to an unregistered account. Check the USDC contract used, and make vendor setup register the payout account
   (`storage_deposit`) if needed. Not yet verified.
2. **Wallet support.** Which NEAR Connect wallets can send `ft_transfer` with the 1 yoctoNEAR deposit and a memo from
   a Next 15 page. Not yet tested.
3. **Overpayment.** Proposal for v1: accept **exact amount only**; show anything else as a security event.
4. **Exchanges.** Many exchanges cannot add a memo to a NEAR token transfer. Proposal: require the memo or a claimed hash;
   no amount-and-time guessing.
5. **Network.** Real money is mainnet only. Launch on testnet until an outside review of the verification code is done.
6. **Mainnet USDC events.** Whether the mainnet token contract emits the standard transfer event is not checked; the
   verifier does not need it (see Findings).

## What changes in the product (when built)

- `invoices`: unchanged signed record. New table `invoice_payments (tx_hash unique, invoice_id, sender, receiver, token,
  amount, memo, block_hash, status, evidence json, verified_at)` and `invoice_security_events`.
- API: `POST /api/v1/invoices/{id}/claims`; `GET /api/v1/invoices/{id}` gains a `payment` evidence object
  (`verification: "chain-verified"`).
- Web: the pay sheet (two options), the confirming and paid states, vendor evidence and events. The security-test page and
  the demo payment stay as labelled demo tools.
- Tests: every row of the threat table gets a test against a recorded RPC response; plus idempotency, expiry and replay.

## Path C: pay from any chain or exchange (built, real-money invoices, unverified with real funds)

A real-money invoice (USDC on NEAR mainnet) can also be paid from a wallet or exchange on another chain. Beeline asks NEAR Intents for a one-time deposit address that delivers the invoice's exact USDC amount to the vendor, bound to the invoice (exact output, vendor payout, amount, refund address, expiry). The customer sends to it from anywhere; Beeline holds no keys and signs nothing.

"Paid" needs two independent signals: NEAR Intents reports `SUCCESS` (provider-reported) and the vendor's own USDC balance on NEAR, read from the chain, rose by at least the invoice amount since the quote. Neither alone is enough. The payment is recorded as `intents-delivered`, distinct from `chain-verified` (a direct NEAR transfer read from the chain). Refunds, failures and short deposits leave the invoice open and appear as security events. A terminal quote never changes state.

Honest limits: no real funds have gone through it; NEAR Intents has no testnet; an unrelated USDC transfer to the vendor in the same window could satisfy the balance signal (the provider signal must still be `SUCCESS` for this invoice's address); the signer of NEAR delivery transactions is not known, so the delivery transaction itself is not independently matched; the vendor's account may need a USDC storage registration; the customer must send the right token on the right network.

