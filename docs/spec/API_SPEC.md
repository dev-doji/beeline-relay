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
- `POST /playground/invoices/{id}/pay` with `{tamper: "none" | "swap_payout" | "edit_amount"}` has the agent pay through the vault. Tamper modes simulate a scammer's edited link. It returns `{invoice, run}`, where `run` is a playground run result (layer `invoice` when the signed-invoice check blocks). The invoice is marked paid only after the vault accepted and sent a genuine one.

Both run endpoints share the playground rate limit.

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
