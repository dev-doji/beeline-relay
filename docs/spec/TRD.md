# Beeline Relay Technical Requirements Document (TRD)

## 1. Technology stack

### Frontend monorepo

- Next.js App Router, TypeScript, React, Tailwind CSS.
- A small consistent UI component layer; shadcn/ui may be used if already present/useful.
- TanStack Query for API state where helpful.
- Zod for form and response validation at the UI boundary.
- Wallet adapter(s) only for the verified route and only if the documented execution requires a browser wallet interaction.
- pnpm workspaces; Turborepo optional.

Applications:
- `apps/website`: public Beeline Relay marketing/site content.
- `apps/app`: developer console and payment-session inspector. If the repo already has a different convention, inspect and retain it rather than renaming blindly.

Shared packages, added only where useful:
- `packages/ui`
- `packages/api-client`
- `packages/config`
- `packages/wallet` only if a reusable wallet layer is warranted.

### Backend

- Go using `net/http` or Chi.
- PostgreSQL for durable records.
- `golang-migrate` or equivalent migrations.
- `log/slog` structured logs.
- OpenAPI as the API contract.
- Bruno for manual/repeatable API testing.
- Standard Go tests for deterministic reliability scenarios; a dedicated simulation framework is not required for the MVP.

### Provider integration

- NEAR Intents only for P0.
- Read current official docs before implementing provider-specific requests, auth, quote IDs, transaction format, status semantics or supported networks/assets.
- Isolate provider-specific logic inside `internal/intents/` or an adapter package.
- Do not assume the provider is a generic bridge API or that each route uses the same execution lifecycle.

Official starting points:
- https://docs.near-intents.org/
- https://docs.near.org/tutorials/intents/swap
- https://docs.near.org/chain-abstraction/intents/faq

## 2. Suggested repository layout

### Frontend

```text
beeline-web/
  apps/
    website/
    app/
  packages/
    ui/
    api-client/
    config/
```

### Backend

```text
beeline-api/
  cmd/
    api/
  internal/
    config/
    http/
    auth/
    payment/
    quote/
    intents/
    reconciliation/
    evidence/
    storage/
    validation/
    models/
  migrations/
  api/
    openapi.yaml
  bruno/
    bruno.json
    environments/
    health/
    capabilities/
    quotes/
    payments/
    reconciliation/
    evidence/
  testdata/
    provider-responses/
    scenarios/
  Dockerfile
  go.mod
  README.md
```

Adapt to an existing repository where appropriate. Do not move or overwrite working code just to match this suggested layout.

## 3. Component responsibilities

### Website
- Explain the payment reliability/reconciliation problem and Beeline Relay's proposed role.
- Link to developer console, API docs and repository only when URLs exist.
- Avoid consumer-payment promises such as universal cross-chain support.

### Developer console
- Create/view a session through the API and review quote details.
- Show normalized state, timestamps, source of observations, transaction references and event timeline.
- Make provider-reported evidence distinguishable from independently verified chain evidence.
- Provide a JSON evidence view/export.
- Separate simulated scenario results from real route records.
- Never contain provider secrets or authoritative payment logic.

### Go backend
- Validate request shape, amounts, identifiers, destination data and workspace authorization.
- Maintain a verified capability/network/asset catalogue.
- Obtain live quotes from the documented NEAR Intents integration.
- Persist immutable normalized quote snapshots and payment-session records.
- Apply idempotency and valid state transitions at a domain-service boundary.
- Reconcile using documented provider status mechanisms and available evidence.
- Emit append-only event records and build evidence packets.
- Apply rate limits, safe logs, consistent errors and secure configuration.
- Never custody wallet private keys.

## 4. Persistence model (minimum)

### `quotes`
- Internal quote ID, workspace ID, normalized parameters/fingerprint, source/destination amounts and identifiers, expiry, provider reference, safe normalized fee/details, creation time.
- Store only provider data needed to execute and explain the quote.

### `payment_sessions`
- Internal payment ID, workspace ID, quote ID, client reference, normalized current state, immutable parameter fingerprint, created/updated timestamps, provider/transaction references, last reconciliation time.
- Keep client metadata constrained and redact sensitive fields.

### `idempotency_records`
- Workspace ID, key, request fingerprint, resulting resource ID, creation/expiry timestamps.
- Unique constraint on workspace + key. Conflicting fingerprints must not return a different payment silently.

### `payment_events`
- Event ID, payment ID, monotonically assigned sequence, event type, source, observed/recorded timestamps, prior/new state where relevant, safe provider references, redacted metadata and correlation ID.
- Append-only at application level. Corrections should be new events, not silent rewrites.

A separate evidence table/blob store is optional. Evidence may be assembled from durable records if the result is deterministic, versioned, and reproducible.

## 5. API contract

See `API_SPEC.md` and `api/openapi.yaml`.

Core contract areas:
- Health and verified capabilities/catalogue.
- Live quote request/review.
- Idempotent payment-session creation.
- Provider-specific submission endpoint only if the verified flow needs it.
- Payment detail, event timeline and read-oriented reconciliation.
- Evidence packet/export.
- Workspace-aware access control.

Do not add a generic public fault-injection endpoint. Keep simulated scenario controls inside test tooling or a development-only isolated environment.

## 6. State model

Normalized states:
- `created`
- `quoted`
- `awaiting_authorization`
- `submitted`
- `processing`
- `completed`
- `failed`
- `expired`
- `recovery_required`

Use `refunded` only if the selected provider exposes and evidences refunds and the behavior is implemented/tested. Provider-native state remains preserved in events/evidence.

State transitions must be implemented in a single tested service. A client must never assign arbitrary payment state.

## 7. Security and reliability

- Never request, store, transmit or log private keys/seed phrases.
- Require explicit wallet authorization for any user-signed transaction.
- Use a backend-derived workspace identity; never trust a caller-supplied workspace ID.
- Do not expose backend/provider API keys in the frontend.
- Validate addresses against the chosen destination network.
- Use canonical asset IDs; token symbol is not sufficient.
- Bind quotes to exact source/destination networks, assets, amounts, recipient and expiry.
- Idempotency for session creation and any operation that might move value.
- Do not blindly retry a value-moving provider operation when acceptance is uncertain.
- Duplicate callbacks/observations must be idempotent.
- Out-of-order observations must not regress verified terminal state.
- Do not mark `completed` from client events or source submission alone.
- Label evidence provenance precisely; a provider status is not automatically an independent chain proof.
- Rate-limit public endpoints; enforce CORS allowlists and HTTPS in deployed environments.
- Never log tokens, secrets, full wallet signatures or unnecessary personal data.
- Keep the fault test harness isolated from production provider configuration.

## 8. Configuration

Frontend:
- `NEXT_PUBLIC_API_BASE_URL` only if the API endpoint is intentionally public and no secret is required.
- `NEXT_PUBLIC_APP_URL` for site/console navigation.
- Prefer a server-side Next.js API proxy for authenticated developer-console calls; never embed an API secret in `NEXT_PUBLIC_*`.

Backend examples:
- `PORT`
- `APP_ENV`
- `DATABASE_URL`
- `CORS_ALLOWED_ORIGINS`
- workspace/API authentication configuration
- `NEAR_INTENTS_API_BASE_URL` only as documented/configured
- `NEAR_INTENTS_API_KEY` only if current official integration requires it
- route/network RPC configuration only when independently verified checks need it
- reconciliation polling parameters if used

Provide `.env.example` with placeholder values only. Document which values are required for unit tests, local app startup, provider integration, and live-route verification. Never commit real credentials.

## 9. Testing strategy

### Unit tests
- Decimal/base-unit amount parsing.
- Network/address validation.
- Quote expiry and exact-parameter binding.
- Idempotency same-key/same-request behavior.
- Idempotency conflicting payload behavior.
- State transition matrix.
- Duplicate and stale observation handling.
- Evidence packet provenance and redaction.

### Integration tests
- PostgreSQL persistence and migrations.
- Provider adapter parsing with sanitized fixtures.
- Reconciliation service behavior with deterministic provider test doubles.
- API workspace isolation and error envelopes.

### Reliability scenarios
At minimum: duplicate create, idempotency conflict, expired quote, parameter mismatch, duplicate provider event, out-of-order event, submitted-but-pending, uncertain provider timeout, unsupported route, illegal terminal-state transition. All simulations must clearly use test data and may not assert a fake real-world transaction hash.

### Live integration test
- Mark it separately from unit tests and ensure it cannot run accidentally on normal `go test ./...` unless the developer opts in.
- Verify one actual supported route in the intended environment before claiming end-to-end completion.
- Use small, controlled values and clearly document the operator action and risks.

## 10. Delivery order

1. Inspect existing repositories and keep useful working conventions.
2. Verify the current NEAR Intents flow and one route; record the findings.
3. Define canonical IDs, quote binding and settlement evidence requirements.
4. Implement API skeleton, config, migrations, health and capabilities.
5. Implement live quote adapter and expiry validation.
6. Implement payment session/idempotency/event/state services.
7. Implement documented execution and reconciliation behavior.
8. Implement evidence packet and fault scenarios.
9. Build developer console against the API contract.
10. Reposition website and docs.
11. Run tests, Bruno, lint/type checks and production builds; document remaining blockers honestly.
