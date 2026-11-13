# Beeline Relay API Specification

**Status:** Proposed normalized application contract. The selected NEAR Intents flow may require changes after current official integration docs are verified. Keep this document and `api/openapi.yaml` synchronized with the implementation.

Base path: `/api/v1`

## Contract principles

- NEAR Intents remains the execution/routing provider; this API is an application lifecycle and reliability layer.
- Provider-specific request/response mapping stays behind an internal adapter. Do not expose credentials or blindly proxy provider payloads to clients.
- Use canonical network and asset IDs, decimal strings/base units for amounts, and server-generated payment IDs.
- All value-moving operations require explicit idempotency semantics.
- Client-supplied status is never authoritative.
- The examples below illustrate the intended normalized model. Final provider-dependent payloads must match the verified route's documented flow.

## Authentication and ownership

For any deployed/shared environment, derive a `workspace_id` from authenticated credentials/session. Do not accept a client-supplied workspace ID as proof of ownership. Keep server API keys out of browser bundles. The developer console should call the Go API through a server-side boundary or an explicitly designed session mechanism.

Unauthenticated mode is allowed only for local development bound to localhost and enabled by an explicit development setting. It must not be the default for a public deployment. Payment lists, event histories, and evidence packets must not be readable across workspaces.

## `GET /health`

Returns service health without leaking secrets or infrastructure details.

```json
{ "status": "ok" }
```

If readiness is included, expose only safe booleans such as database/provider configuration readiness, not connection strings or credentials.

## `GET /capabilities`

Returns verified capabilities for the running configuration. Only advertise features and routes that are actually enabled and verified.

Illustrative response:

```json
{
  "provider": "near-intents",
  "environment": "testnet-or-mainnet-as-configured",
  "features": {
    "live_quotes": true,
    "payment_sessions": true,
    "reconciliation": true,
    "evidence_export": true
  },
  "route_verification": {
    "status": "verified-or-not_verified",
    "last_verified_at": null,
    "notes": "Replace with repository-recorded verification; do not claim verified before a real test."
  }
}
```

Do not return a fabricated `verified` status. A configuration state is not equivalent to a successful end-to-end route test.

## `GET /networks` and `GET /assets?network={network_id}`

Return only configured, provider-supported networks/assets verified for the build. Assets must use stable canonical IDs and network-specific contract/mint data where applicable. A token symbol alone is not a unique identifier.

## `POST /quotes`

Requests a live quote for exact parameters.

Illustrative request:

```json
{
  "source_network": "verified-network-id",
  "source_asset_id": "canonical-provider-asset-id",
  "source_amount": "50",
  "destination_network": "verified-network-id",
  "destination_asset_id": "canonical-provider-asset-id",
  "destination_address": "recipient-address"
}
```

Successful response should return, when the provider supplies them:

- `quote_id` generated/stored by Beeline Relay and provider reference(s);
- normalized source/destination networks and assets;
- source amount and expected destination amount as strings;
- fees/cost details where available;
- quote expiry and retrieval timestamp;
- route requirements/next action needed for safe execution;
- an immutable normalized-parameter fingerprint.

Never fabricate output amounts, fees, expiry, routes, or provider IDs. Unsupported/unavailable route returns an explicit typed error.

## `POST /payments`

Creates a payment session from a valid quote. This endpoint must persist the logical payment and be idempotent.

Required header:

```text
Idempotency-Key: <client-generated-unique-key>
```

Illustrative request:

```json
{
  "quote_id": "quote_123",
  "client_reference": "invoice-or-order-id"
}
```

The backend must load the quote record, validate expiry, and confirm that the quote parameters still match the payment intent. Do not trust amounts/recipient/network values copied back from the client when a stored quote can be used as the source of truth.

Illustrative response:

```json
{
  "payment_id": "pay_123",
  "state": "awaiting_authorization",
  "created_at": "2026-10-26T10:00:00Z",
  "next_action": {
    "type": "provider-defined-action",
    "description": "Review the required action from the verified integration."
  },
  "replayed": false
}
```

`next_action` must be derived from the verified integration and must not include provider secrets. Repeating the same key and same normalized request returns the same logical payment and sets `replayed: true` where practical. Reusing the key with a different request returns `IDEMPOTENCY_CONFLICT`.

## `POST /payments/{id}/submit`

Implement this endpoint **only if** the verified NEAR Intents flow requires a separate client submission step. Its request schema must match the actual integration and accept only the minimum needed client result (for example, a transaction reference if that is the documented protocol). The endpoint must verify session ownership, expected state, parameter binding, and idempotency.

Do not implement a generic blind-resubmit endpoint. If a submit attempt times out and the result is uncertain, record uncertainty and reconcile first. A retry is allowed only when the provider's idempotency/duplicate semantics are understood and tested.

## `GET /payments/{id}`

Returns the current normalized state, payment details, provider references, safe transaction references, timestamps, and whether reconciliation is needed. The state must be backend-authoritative.

## `GET /payments/{id}/events`

Returns the append-only event timeline ordered by a stable sequence/cursor. Each event should contain:

- `event_id`, `payment_id`, `sequence`, `type`, `occurred_at`;
- `source` (for example `application`, `wallet`, `provider`, `chain_observation`, `reconciler`, or `test_harness`);
- normalized state before/after where relevant;
- safe provider/transaction references;
- redacted metadata and a correlation/request ID.

Do not store private keys, seed phrases, access tokens, or unnecessary personal data in events.

## `POST /payments/{id}/reconcile`

Requests a read-oriented status refresh using the verified provider/network status mechanism. This endpoint must not initiate a transfer. It must be safe to call repeatedly and rate-limited.

Illustrative response:

```json
{
  "payment_id": "pay_123",
  "state": "processing",
  "reconciled_at": "2026-10-26T10:01:00Z",
  "changed": false,
  "reason": "Provider status still indicates processing."
}
```

If the provider outcome is unknown, surface `recovery_required` or a documented pending state. Do not interpret missing status as success.

## `GET /payments/{id}/evidence`

Returns a versioned evidence packet or JSON download. Minimum logical schema:

```json
{
  "schema_version": "1.0",
  "payment_id": "pay_123",
  "client_reference": "invoice-or-order-id",
  "request_fingerprint": "sha256:...",
  "payment": {
    "source_network": "verified-network-id",
    "source_asset_id": "canonical-id",
    "source_amount": "50",
    "destination_network": "verified-network-id",
    "destination_asset_id": "canonical-id",
    "destination_amount_expected": "49.7",
    "destination_address": "redacted-or-policy-approved-value"
  },
  "state": "completed",
  "state_basis": [
    {
      "kind": "provider_status",
      "value": "SETTLED",
      "source": "documented-provider-status-method",
      "observed_at": "2026-10-26T10:01:00Z",
      "reference": "provider-reference"
    }
  ],
  "transaction_references": [],
  "limitations": []
}
```

This is illustrative. Never include example values in production output. Label provider-reported status separately from independently verified on-chain evidence. Do not claim a cryptographic proof or independent verification if the implementation merely stores a provider response. Redact destination details where appropriate for workspace privacy policy.

## `GET /payments`

Returns recent payment sessions for the authenticated workspace, with pagination and filters only as time allows. Never expose another workspace's history. If history authorization cannot be implemented securely, omit this endpoint from the deployed MVP rather than exposing global records.

## Provider webhook intake

Add a provider callback/webhook endpoint only if the current official provider flow supports it and signature/authenticity can be verified. Do not assume that a webhook exists. Repeated events must be idempotent. If polling is the documented/reliable method for the selected integration, use reconciliation polling instead.

## Public playground (testnet demo)

Enabled only when `PLAYGROUND_VAULT` names a `.testnet` vault (`internal/playground`). There's no authentication. Requests are rate limited per client (`PLAYGROUND_RUNS_PER_MINUTE`, default 4) and per UTC day (`PLAYGROUND_RUNS_PER_DAY`, default 300). When disabled, both endpoints return `503 PROVIDER_UNAVAILABLE`.

### `GET /playground`

Returns the vault's live state, read from NEAR testnet views (`get_config`, `get_recipients`, `get_reviewer`, `get_balances`, `get_agents`). It also returns the agent's public key, the preset tasks per scenario, and the real/simulated disclosures.

### `POST /playground/runs`

```json
{ "scenario": "honest | injected | attacker | forged | over_limit | replay | custom",
  "task": "text the agent and the AI reviewer see (1-2000 chars)",
  "recipient": "0x… (ignored for over_limit and replay)",
  "amount": "base units, 6 decimals (ignored for over_limit and replay)" }
```

It runs one agent attempt and returns `{scenario, outcome: paid|blocked|error, reason, layer, tx_hash, explorer_url, steps[], started_at, duration_ms}`. Steps are `quote` (label `simulated`: a TESTNET-SIMULATED quote, signed by the vault's test signer or, for `forged`, a throwaway key), `review` (label `real`: a NEAR AI Cloud review when the vault's reviewer threshold applies, with the signature verified against the vault's registered key), and `vault` (label `real`: the `pay_quote` testnet transaction). `layer` names the guardrail behind a block: `allowlist`, `quote_signature`, `spend_limit`, `single_use`, `ai_reviewer` or `vault_policy`. Validation errors return `400 VALIDATION_ERROR`, and the rate limit returns `429 RATE_LIMITED`.

### Vendor invoices (playground)

- `POST /playground/invoices` with `{vendor_name, vendor_handle, payout, memo, items: [{name, quantity, unit_price}]}`. Amounts are base units, and the total is computed server-side and capped at 5 test USDC. It returns the invoice, including `payload` (the exact signed JSON) and `signature` (hex ed25519 by the TESTNET invoice issuer). Status `201`.
- `GET /playground/invoices/{id}` returns the invoice, with `status: open|paid`, `paid_tx` and `paid_at`.
- `POST /playground/invoices/{id}/pay` with `{message?, payout?, amount?}`: whoever holds the pay link tries to pay it. An empty body is an honest payment. `message` (up to 1000 characters) is free text the agent reads along with the link, where prompt injection lives. `payout` (a NEAR account) and `amount` (USDC, e.g. `"4.5"`) edit the visitor's copy of the link, which breaks the issuer's signature. The agent (a NEAR AI model) decides whom to pay and how much, and the vault decides on-chain. It returns `{invoice, run, attempt}`: `run` is a playground run result (layer `invoice` when the signed-invoice check blocks), and `attempt` is the stored record. The invoice is marked paid only when the vault sent exactly the invoice's payee and amount. All input is untrusted.
- `GET /playground/invoices/{id}/attempts` returns `{data: Attempt[]}`, every attempt on that pay link, newest first (up to 50).
- `POST /me/chat/new` (signed in): starts a fresh conversation. `GET /me/messages` and Bee's history then cover only the new one; earlier messages are kept in the database (migration 0008, `chat_messages.thread`, `users.chat_thread`).
- `POST /playground/invoices/{id}/claims` `{tx_hash, sender_account_id}`: a customer paid from their own wallet and names the transaction. The server reads it from NEAR (`tx` with `wait_until: FINAL`, archival endpoints first; `NEAR_VERIFY_RPC_URLS` overrides) and accepts only a final, successful, direct `ft_transfer` on the invoice's token contract, to the invoice's payout, for the exact amount, with memo `beeline:<invoice id>` and a 1 yoctoNEAR deposit, signed by `sender_account_id`, in a block at or before `expires_at`. Answers: `200 {status: "verified", payment}` (idempotent: repeating returns the same), `202 {status: "pending", message}` (not seen or not final yet: ask again, nothing is recorded), `422 CLAIM_REJECTED` (it does not pay this invoice; recorded as an event), `409 INVOICE_ALREADY_PAID` or `409 CLAIM_REJECTED` for a reused transaction. Only a verified claim moves the invoice `open -> paid`, in one database transaction; `invoices.payment` then holds the chain evidence (`verification: "chain-verified"`). The client can never set `paid`.
- `GET /playground/invoices/{id}/events`: `{data: [{kind, reason, tx_hash, created_at}]}`, the claims that were refused (`WRONG_TOKEN`, `WRONG_AMOUNT`, `WRONG_RECEIVER`, `WRONG_MEMO`, `WRONG_DEPOSIT`, `WRONG_SENDER`, `UNSUPPORTED_CALL`, `TX_FAILED`, `TX_NOT_FOUND`, `EXPIRED`, `ALREADY_PAID`, `TX_REUSED`).
- `GET /payin/options`: `{data: [{asset_id, chain, chain_name, symbol, decimals}], source}`, the stablecoins (USDC, USDT) on Base, Ethereum, Arbitrum, Optimism, Polygon, BNB Chain, Avalanche, Solana, Tron and TON that NEAR Intents lists (`GET /v0/tokens`, cached 10 minutes). Provider-reported. 503 when real-money invoices are off.
- `POST /playground/invoices/{id}/payin` `{origin_asset, refund_to}`: for a **real-money (mainnet) invoice only**, asks NEAR Intents for a real, non-dry `EXACT_OUTPUT` quote that delivers the invoice's exact USDC amount to the invoice payout on NEAR, with `refund_to` as the customer's own address on the origin chain (shape-checked per chain). Answers `{quote: {deposit_address, deposit_memo, amount_in, deadline, status, provider_status, ...}}`. The same invoice, token and refund address returns the live quote instead of a second address; a quote that would deliver less than the invoice is refused; `422` for bad input, a test invoice, or a token not offered. Moves nothing: the customer's own wallet or exchange does the sending.
- `GET /playground/invoices/{id}/payin`: reads NEAR Intents' status for the current quote and the vendor's USDC balance on NEAR, advances the quote (`waiting`, `processing`, `confirming`, `paid`, `incomplete`, `refunded`, `failed`, `expired`) and returns `{quote, invoice_status, payment}`. The invoice becomes `paid` only when the provider reports `SUCCESS` **and** the vendor's balance rose by at least the invoice amount since the quote (`payment.verification: "intents-delivered"`). Refunds, failures and short deposits are recorded as `PAYIN_REFUNDED`, `PAYIN_FAILED` and `PAYIN_INCOMPLETE` events. Read-only towards the provider and the chain.
- Real-money invoices: `POST /me/chat` (Bee) can create them when the signed-in user's own message asks for a real invoice; `invoice.network` is `mainnet` and `asset` is native USDC on NEAR mainnet. Off unless `MAINNET_INVOICES=on`; capped (default 25 USDC, `MAINNET_INVOICE_MAX_UNITS` in base units). The public `POST /playground/invoices` form never makes them. Wallet claims for a mainnet invoice are verified against mainnet RPCs (`NEAR_MAINNET_RPC_URLS`).
- `GET /playground/attempts` is the public attack log: `{data: Attempt[], stats}` across all pay links. `stats` is `{attempts, attacks, blocked, paid, misdirected, invoices}`. `misdirected` counts accepted payments whose recipient or amount differs from the invoice, computed from the records.
- `Attempt` is `{id, invoice_id, vendor_name, kind: "pay" | "attack", message, edited_payout, edited_amount, outcome: "paid" | "blocked" | "reply" | "error", layer, reason, recipient, amount, tx_hash, created_at}`. Amounts are base units. Messages are public: the UI tells visitors not to include personal information.

Both run endpoints share the playground rate limit.

### Cash out to a bank (Switch offramp, route through NEAR Intents)

Enabled only when `SWITCH_API_KEY` is set (`SWITCH_MODE` is `sandbox` or `live`, default sandbox). Rate limited like the playground. Switch docs reviewed 2026-11-12: <https://docs.onswitch.xyz>. Beeline never holds keys and sends nothing: the vendor signs the NEAR transfer in their own wallet.

- `POST /cashout/quote` `{amount_usdc: "25.50"}`: read-only. A NEAR Intents **dry** quote for NEAR USDC to Base USDC, then a Switch `POST /offramp/quote` for the bridge's minimum output. Returns `{mode, amount_usdc, route|null, route_error, payout (Switch's object), payout_basis: "bridge_minimum"|"nominal", note}`.
- `POST /cashout/prepare` `{amount_usdc, near_account, bank: {bank_code, account_number, holder_name}}`: opens a Switch `POST /offramp/initiate` session (NGN bank, `base:usdc`), then requests a **non-dry NEAR Intents `EXACT_OUTPUT` quote** that delivers exactly Switch's `deposit.amount` to Switch's Base address, depositing on NEAR (`ORIGIN_CHAIN`) and refunding to `near_account`. Returns `{mode, payout: {source, reference, status, naira, currency, deposit_amount_usdc, deposit_asset}, route: {deposit_address, send_usdc, send_units, min_send_usdc, delivers_usdc, deliver_to, refund_to, time_estimate_seconds, deadline, token_contract}|null, note}`. `route` is null in Switch sandbox (its deposit address is a placeholder). In **live** mode it is refused (403) unless the server has `CASHOUT_LIVE=on`, and amounts above `CASHOUT_LIVE_MAX_USDC` (default 5) are refused. Nothing moves until the vendor sends the USDC.
- `POST /cashout/deposit` `{deposit_address, tx_hash, near_sender_account?}`: forwards the NEAR transaction hash to NEAR Intents (`POST /v0/deposit/submit`) to speed detection. It sends nothing. Safe to retry.
- `GET /cashout/{reference}?deposit_address=<near address>`: `{source: "provider-reported (Switch)", data: {status: AWAITING_DEPOSIT|PROCESSING|COMPLETED|FAILED, ...}, bridge: {source: "provider-reported (NEAR Intents)", status, ...}|null}`. Each leg is labelled by the provider that reported it; the cash-out is done only when Switch says `COMPLETED`.
- `POST /cashout/start`: older, sandbox-only payout session without the bridge; refused with 403 in live mode.
- Bank details are validated before any provider call and are never logged. Provider amounts are handled as decimal strings and integer base units, never floats.

## Error envelope

```json
{
  "error": {
    "code": "QUOTE_EXPIRED",
    "message": "The quote has expired. Request a fresh quote before continuing.",
    "request_id": "req_123"
  }
}
```

Suggested codes:

- `VALIDATION_ERROR`
- `UNSUPPORTED_NETWORK`
- `UNSUPPORTED_ASSET`
- `ROUTE_UNAVAILABLE`
- `QUOTE_EXPIRED`
- `QUOTE_PARAMETER_MISMATCH`
- `PROVIDER_UNAVAILABLE`
- `PAYMENT_NOT_FOUND`
- `INVALID_STATE_TRANSITION`
- `IDEMPOTENCY_CONFLICT`
- `SETTLEMENT_PENDING`
- `RECONCILIATION_REQUIRED`
- `RECOVERY_REQUIRED`
- `UNAUTHORIZED`
- `RATE_LIMITED`

## State model

Normalized states (adapt only with documented reason):

- `created`
- `quoted`
- `awaiting_authorization`
- `submitted`
- `processing`
- `completed`
- `failed`
- `expired`
- `recovery_required`

`refunded` may be added only if the selected provider exposes and evidences a refund state. Keep provider-native status in the event/evidence record even when mapping it to a normalized state.

## API design rules

- Use decimal strings or integer base units for monetary values; no floating-point money arithmetic.
- Validate address/network format at the backend boundary.
- Bind quote IDs to exact normalized payment parameters and expiry.
- Treat provider responses as untrusted until schema/status validation succeeds.
- Enforce state transitions on the server.
- Reconcile must not move funds.
- Never accept a client-supplied `completed` state as authoritative.
- Never expose provider secrets, stack traces, raw auth headers, or private wallet material.
