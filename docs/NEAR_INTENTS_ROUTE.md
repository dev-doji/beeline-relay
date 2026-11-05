# NEAR Intents Integration Research — Beeline Relay

**Researched:** 2026-10-28 (pre-hackathon; no product code written)
**Status:** Docs reviewed, and one live *dry* quote run. The route is **NOT verified end to end**. Do not use the word "verified" on the website or in `/capabilities` until the Day 6 live run passes.

This file is meant to become `docs/NEAR_INTENTS_ROUTE.md` in the repo on Day 1.

---

## 1. Which integration to use: the 1Click Swap API

NEAR Intents offers several integration surfaces: the 1Click REST API, a React widget, TS/Go/Rust SDKs, the Verifier contract, and the Market Maker message bus. **The 1Click Swap API is the documented path for apps and distribution channels**, and it is what we use. The SDKs are generated wrappers around it.

- Base URL: `https://1click.chaindefuser.com`
- OpenAPI: `https://1click.chaindefuser.com/docs/v0/openapi.yaml` (we should vendor a copy into `beeline-api/testdata/provider/` on Day 1)
- Go SDK: `github.com/defuse-protocol/one-click-sdk-go`. We'll write our own thin client for the 4 endpoints we need instead, so that fixtures and fault doubles stay simple. The SDK is a reference for types.
- **No testnet, and none planned** (FAQ). All live testing is mainnet with small amounts.

Sources:
- https://docs.near-intents.org/integration/distribution-channels/1click-api/quickstart/making-a-request.md
- https://docs.near-intents.org/integration/distribution-channels/1click-api/sdk.md
- https://docs.near-intents.org/integration/distribution-channels/1click-api/authentication.md
- https://docs.near-intents.org/integration/distribution-channels/1click-api/swap-types.md
- https://docs.near-intents.org/resources/fees.md
- https://docs.near-intents.org/resources/faqs.md
- https://docs.near-intents.org/changelog/overview.md

## 2. Authentication and fees

- Partner JWT from https://partners.near-intents.org. Send it as `X-API-Key: <jwt>`; `Authorization: Bearer <jwt>` is also accepted.
- The OpenAPI lists `X-API-Key` / `JWT-auth` security on `/v0/quote`, `/v0/status` and `/v0/deposit/submit`. In practice, an **unauthenticated dry quote and status call both worked** on 2026-10-28.
- Fees as documented:
  - Without a key: +25 bps platform fee.
  - With a key: 20 bps, or **1 bp for stablecoin pairs and same-asset multichain routes**.
  - Plus a 0.0001% on-chain protocol fee.
- What we actually saw: the unauthenticated dry quote for USDC→USDC returned `appFees: [{fee: 1}]` and ~1.5 bps total cost. That doesn't match the documented +25 bps. **Don't put fee numbers in UI copy.** Show only what the quote returns.
- The JWT is a **backend-only secret** (`NEAR_INTENTS_API_KEY`). It must never appear in `NEXT_PUBLIC_*` variables.

## 3. Endpoints we need

| Purpose | Method and path | Notes |
|---|---|---|
| Asset catalogue | `GET /v0/tokens` | No auth. 208 tokens on 36 chains (live, 2026-10-28). Fields: `assetId`, `decimals`, `blockchain`, `symbol`, `price`, `priceUpdatedAt`, `contractAddress`. |
| Quote | `POST /v0/quote` | `dry: true` means validate and price only, with no `depositAddress`. `dry: false` returns `depositAddress`, `deadline` and a **`signature`**. |
| Deposit notify | `POST /v0/deposit/submit` | Body: `{txHash, depositAddress, nearSenderAccount?, memo?}`. **Optional**: it only speeds up detection, and **it does not move value**. |
| Status | `GET /v0/status?depositAddress=…&depositMemo=…` | Returns `status`, `updatedAt` and `swapDetails`, plus the original `quoteResponse`. An unknown address returns 404 with `{message, error, statusCode}`. |

Not needed for P0: `/v0/generate-intent` and `/v0/submit-intent` (these are for the `INTENTS` deposit type), orders, account, auth, and any-input.

### Quote request (key fields)
`dry, swapType (EXACT_INPUT|EXACT_OUTPUT|FLEX_INPUT|ANY_INPUT), slippageTolerance (bps), originAsset, depositType (ORIGIN_CHAIN|INTENTS|CONFIDENTIAL_INTENTS), destinationAsset, amount (smallest unit, string), refundTo, refundType, recipient, recipientType (DESTINATION_CHAIN|INTENTS|CONFIDENTIAL_INTENTS), deadline (ISO), confidentiality (public|basic|advanced), appFees[]`

`deadline` is "when the user refund begins if the swap isn't completed by then. It must exceed the time required for the deposit transaction to be mined."

### Quote response (key fields)
- `correlationId`, `timestamp`
- `signature`: "Signature of the 1Click service confirming the quote for the specific deposit address. **Must be saved on the client side (along with the whole quote) in order to resolve any disputes**."
- `quoteRequest`: echoed back. The server fills in defaults such as `depositMode`, `appFees` and `insured`.
- `quote`: `depositAddress`, `depositMemo` (if present it is **required**), `amountIn`, `minAmountIn`, `amountOut`, `minAmountOut`, `deadline`, `timeWhenInactive`, `timeEstimate` (seconds), `refundFee`, `withdrawFee`, plus `*Formatted` and `*Usd` variants.
- `quote.deadline` is described as: "Time when the deposit address becomes inactive and **funds may be lost**".

**This is a quote-binding gift.** The provider signs the exact request plus response, and the docs tell integrators to keep it for disputes. Beeline Relay stores the full signed quote immutably and puts it in the evidence packet.

Signature verification: the message is `base58(sha256(json-stable-stringify({...quoteRequest, ...quoteResponse, timestamp})))`, and the signature is `ed25519:…`. Only the TS SDK ships `verifyQuoteSignature()`. **To verify:** where the 1Click public key comes from. Until we find it, the evidence packet says "signature stored, not independently verified".

### Status values (provider-native)
`PENDING_DEPOSIT, KNOWN_DEPOSIT_TX, PROCESSING, SUCCESS, INCOMPLETE_DEPOSIT, REFUNDED, FAILED`. The terminal ones are `SUCCESS`, `REFUNDED` and `FAILED`.

### `swapDetails` (evidence fields)
`intentHashes[]`, `nearTxHashes[]`, `originChainTxHashes[{hash, explorerUrl}]`, `destinationChainTxHashes[{hash, explorerUrl}]`, `amountIn`, `amountOut`, `slippage`, `depositedAmount`, `refundedAmount`, `refundReason` (e.g. `PARTIAL_DEPOSIT`), plus `*Formatted`/`*Usd`.

## 4. ⚠️ Change from yesterday: 1Click is confidential-only (2026-10-27)

> "From October 27, 2026, 1Click swaps roll out as confidential only. The API is unchanged."

The swap itself now settles on the **NEAR Private Shard**. `nearTxHashes` lists only the public shield and unshield transactions, so there is **no public hash for the swap step**. The docs suggest foreign-to-foreign deposits and receipts still look like normal external-chain transfers, but they don't confirm whether `originChainTxHashes` and `destinationChainTxHashes` stay populated.

**Impact:** this fits our evidence-provenance story well. The evidence packet can honestly say:
- the origin deposit is **chain-verified** (we read the Base tx receipt ourselves);
- the swap step is **provider-reported only** (private shard, by design);
- the destination delivery is **chain-verified** if we read the NEAR or destination tx ourselves, and provider-reported otherwise.

This is exactly the "provider status ≠ chain proof" distinction the PRD wants, and it's now *forced* by the protocol. It's a strong point for the pitch.

**To verify on the Day 6 live run:** whether the origin and destination hash arrays are populated after this change.

## 5. Candidate P0 route

**USDC on Base → USDC on NEAR** (`ORIGIN_CHAIN` deposit → `DESTINATION_CHAIN` recipient, `EXACT_INPUT`)

| | Canonical asset ID (from live `/v0/tokens`) | Decimals |
|---|---|---|
| Source | `nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near` | 6 |
| Destination | `nep141:17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1` | 6 |

Live dry quote (2026-10-28, 1 USDC): `amountOut 999849`, `minAmountOut 989850` at 1% slippage, `timeEstimate 35s`, `refundFee 2400` (0.0024 USDC).

Why this route:
- **Stablecoins:** the cost is tiny, the value is easy to read on video, and authenticated fees are 1 bp.
- **Fast:** a ~35 s estimate means the full happy path fits in the demo video.
- **Simple authorization:** a standard EVM wallet sends an ERC-20 `transfer` to `depositAddress`. That's wagmi/viem, which is well-trodden. EVM deposits are detected quickly, and the FAQ says they can skip `PENDING`.
- **Independent origin verification is easy:** `eth_getTransactionReceipt` on a public Base RPC, then check for a `Transfer(from, depositAddress, amount)` log on the USDC contract.
- **NEAR is the destination**, which keeps it on-theme for a NEAR hackathon.

Alternative or stretch: USDC on NEAR → USDC on Base, using the NEAR wallet selector and `ft_transfer` to the deposit address. It's more NEAR-native, but it brings more unknowns (storage deposits, `nearSenderAccount`, memo handling).

**To verify:** whether a NEAR recipient account needs USDC storage registration before receiving.

## 6. How 1Click maps onto the Beeline Relay design

| Beeline concept | 1Click reality |
|---|---|
| `POST /quotes` | `POST /v0/quote` with `dry: false`. Store the whole signed response, a parameter fingerprint, and `quote.deadline` as the expiry. Use `dry: true` for "preview" only, and never let a dry quote back a payment. |
| `awaiting_authorization` | We have a deposit address and are waiting for the **user's wallet transfer**. That transfer is the only value-moving step, and our backend never makes it. |
| `POST /payments/{id}/submit` | **Justified by the docs.** The client reports the wallet's `txHash`. We record it, forward it to `/v0/deposit/submit` (non-value-moving, so safe to retry), and move to `submitted`. Make it idempotent on `(payment_id, txHash)`, and reject a *different* txHash once one is recorded. |
| `POST /reconcile` | `GET /v0/status`, plus an optional Base RPC receipt check. Purely read-only. |
| Uncertain outcome | A wallet tx that was broadcast with no receipt, a status 404 after submission, or a status timeout. **Never prompt a second deposit** to the same address. Reconcile first. |
| Deadline hazard | "deposit address becomes inactive and funds may be lost". The console must **refuse to start a wallet transfer near or after `deadline`**, with a safety margin of maybe 60 s. Good demo moment. |
| Evidence | Signed quote, status snapshots with `correlationId`, `originChainTxHashes`, `destinationChainTxHashes`, `nearTxHashes`, our own chain-receipt check, and explicit limitations (the private-shard swap step). |

### Proposed status mapping (needs confirming in tests and the live run)

| 1Click status | Normalized state | Basis label |
|---|---|---|
| (quote stored) | `quoted` | application |
| (session created) | `awaiting_authorization` | application |
| `PENDING_DEPOSIT` | stays `awaiting_authorization` or `submitted` (no regression) | provider_reported |
| client-reported txHash | `submitted` | wallet (client-reported; not proof) |
| `KNOWN_DEPOSIT_TX` | `submitted` | provider_reported |
| `PROCESSING` | `processing` | provider_reported |
| `SUCCESS` | `completed` *(policy: provider SUCCESS + destination hash present)* | provider_reported, plus chain_verified if checked |
| `INCOMPLETE_DEPOSIT` | `recovery_required` (deposit below minimum; refund expected by deadline) | provider_reported |
| `REFUNDED` | `refunded`. The spec allows adding it because the provider evidences refunds via `refundedAmount` and `refundReason`. | provider_reported |
| `FAILED` | `failed` | provider_reported |
| `deadline` passed, no tx, still `PENDING_DEPOSIT` | `expired` | reconciler |
| 404 or timeout or 5xx on status | no state change; append a `reconcile_failed` observation | reconciler |

Ordering: use `updatedAt` from the status response as the observation version. Never move backwards from a terminal state.

## 6a. Live findings (2026-11-04, through the Beeline API, no funds moved)

The route is USDC on NEAR → USDC on Base: `ORIGIN_CHAIN` deposit, `EXACT_INPUT`, 1 USDC.

- **A non-dry quote works without a partner key.** It returned `depositAddress`, `signature`, `correlationId`, `amountOut` 997398, `minAmountOut` 987424 and `timeEstimate` 22 s.
- **The NEAR-origin deposit address is a 64-hex implicit NEAR account.** The vault must `ft_transfer` USDC to that implicit account. Open: whether that account needs a USDC `storage_deposit` first. Check on Day 7.
- **`refundTo` must be an existing NEAR account.** A well-formed but non-existent account returns **HTTP 400 `{"message":"Internal server error"}`**. Beeline should check that the account exists via NEAR RPC before quoting, so the error is clear.
- **1Click rejects some recipients.** The burn address `0x…dEaD` returns 400 "recipient is not valid".
- **1Click extends the deadline.** We requested 10 minutes and the provider returned `deadline` 3 days out. Beeline keeps the *stricter* 10-minute expiry for creating payments.
- **The deadline format is not the cause of failures.** RFC 3339 with milliseconds and with nanoseconds were both accepted.

## 6b. Live findings (2026-11-05): NEAR-chain deposits now go through the Intents verifier

These are dry quotes, plus one non-dry quote (no funds moved), sent to `https://1click.chaindefuser.com/v0/quote`, both with and without the partner JWT. Results were the same either way.

| Route | Result |
|---|---|
| ETH USDC → Base USDC, Arb USDC → Base USDC, Base USDC → Arb USDC (`ORIGIN_CHAIN` → `DESTINATION_CHAIN`) | 201, quoted |
| NEAR USDC → Base/Arbitrum USDC, NEAR USDC → NEAR USDT, wNEAR → Base USDC with `depositType: ORIGIN_CHAIN` | 400 "Quoting for this pair is not available" |
| Base USDC → NEAR USDC with `recipientType: DESTINATION_CHAIN` | 400 "Quoting for this pair is not available" |
| Base USDC → NEAR USDC with `recipientType: INTENTS` | 201, quoted |
| **NEAR USDC with `depositType: INTENTS`, `refundType: INTENTS` → Base USDC (`DESTINATION_CHAIN`)** | **201, quoted. The non-dry quote is signed, with a 64-hex `depositAddress` and no memo, `timeEstimate` 12s.** |

The same NEAR → Base `ORIGIN_CHAIN` quote worked on 2026-11-04 (section 6a). Adding `confidentiality: basic|advanced` didn't change the result. The changelog has no entry for this (its last entry is the 2026-10-27 confidential settlement). **We don't know whether this is a temporary outage of NEAR-chain deposits or a lasting change.**

**What it means for the vault.** Today the vault pays an `ORIGIN_CHAIN` quote with `storage_deposit` + `ft_transfer` to the deposit account. To pay an `INTENTS` quote instead, it would deposit into the Intents verifier on behalf of the deposit address. The verifier docs (deposits page) say: `ft_transfer_call` on the token with `receiver_id: "intents.near"`, `msg: "<account id>"`, which assigns ownership of the deposited tokens to that account. Refunds with `refundType: INTENTS` would land in the vault's own balance inside `intents.near`, and withdrawing them needs the verifier's withdraw method (not documented on the deposits page; to verify).

**Not verified yet:** that 1Click detects an `ft_transfer_call` deposit made *on behalf of* the deposit address as funding an `INTENTS` quote. This needs one small live mainnet payment.

## 7. Open questions to settle on Day 1–2
1. Is a partner JWT required for non-dry quotes or status in practice? (Dry quotes and status work without one today.)
2. Where is the 1Click signing public key published, so we can verify quote signatures in Go?
3. Are `originChainTxHashes` and `destinationChainTxHashes` still populated after the confidential-only change?
4. Does a fresh NEAR recipient need USDC `storage_deposit`?
5. What is the minimum viable amount on this route? (1 USDC quoted fine.)
6. Rate limits on `/v0/status`. These aren't documented, so poll conservatively (e.g. every 5 s with backoff).

## 8. What you need to do before 1 Nov
- [ ] **Apply for a partner API key** at https://partners.near-intents.org. Approval may take days, so do this first. We can build without it, but we'd pay the higher unauthenticated fee.
- [ ] **EVM wallet on Base** with ~10 USDC and ~$1–2 of ETH for gas. Use a dedicated test wallet, not your main one.
- [ ] **NEAR account** to receive USDC (a named `.near` or implicit account). Keep it separate from your prize wallet if you prefer.
- [ ] **NEAR wallet address for prizes**, needed by the submission form.
- [ ] **GitHub:** be ready to create a **public** repo on 1 Nov.
- [ ] **X account** for the required demo post.
- [ ] **Hosting accounts** (Vercel for web, and Fly.io, Railway or Render for the Go API). Neon is already connected for Postgres.
