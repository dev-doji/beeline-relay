# Coding Agent Prompt — Beeline Relay Go Backend

## Your role

Act as a senior Go backend engineer implementing Beeline Relay's payment-session reliability and reconciliation API. Read `AGENTS.md` and the relevant files in `docs/` before coding. Inspect the existing repository and preserve working conventions.

## Product context

Beeline Relay is a developer-facing reliability layer around a supported cross-chain payment route. It does not replace NEAR Intents or claim routing itself is novel. The core requirements are exact quote binding, idempotent session creation, backend-enforced state transitions, append-only events, reconciliation against documented evidence, and export of a provenance-aware evidence packet.

NEAR Intents is the planned P0 provider. Its current API, auth, supported route IDs, quote/execution protocol, and status semantics must be verified from official current documentation before route-specific code is finalized.

## Required stack

- Go using `net/http` or Chi.
- PostgreSQL and migrations.
- `log/slog` structured logging.
- OpenAPI contract and synchronized `API_SPEC.md`.
- Bruno collection for local/manual API tests.
- Standard Go tests and deterministic provider test doubles for fault scenarios.

## Core tasks

1. Inspect the current repository and record a short implementation plan.
2. Verify and document one currently supported NEAR Intents route, including quote, authorization, execution, status lookup and evidence limitations.
3. Build configuration, graceful startup/shutdown, health endpoint, workspace auth boundary, structured logging and typed errors.
4. Implement verified capability/network/asset catalogue endpoints without guessed values.
5. Implement strict amount parsing using decimal strings or integer base units; never floating point.
6. Build a provider adapter for the documented quote/execution/status flow. Keep provider-specific mapping and status semantics encapsulated.
7. Store normalized quote snapshots with an immutable parameter fingerprint and expiry.
8. Implement payment-session creation with `Idempotency-Key`, same-request replay semantics, and conflicting-payload rejection.
9. Persist payment sessions and append-only event records; use transactions/constraints where appropriate.
10. Implement a single domain service for allowed state transitions and duplicate/stale event handling.
11. Implement the documented execution flow. Only add `POST /payments/{id}/submit` if the actual provider flow requires a separate client submission step.
12. Implement read-oriented reconciliation that cannot initiate another transfer.
13. Implement versioned evidence JSON with provenance, transaction references when available, and explicit limitations.
14. Add rate limiting, safe logs, CORS allowlists, request IDs, input validation and ownership checks.
15. Add migrations, `.env.example`, setup documentation, and a Bruno collection.
16. Add tests for normal flow, error handling, idempotency, quote binding/expiry, invalid transitions, duplicate/out-of-order observations, uncertain submission, and evidence redaction.

## API areas

Implement the contract in `docs/API_SPEC.md` and `api/openapi.yaml`:

- `GET /api/v1/health`
- `GET /api/v1/capabilities`
- `GET /api/v1/networks`
- `GET /api/v1/assets?network={network_id}`
- `POST /api/v1/quotes`
- `POST /api/v1/payments`
- `POST /api/v1/payments/{id}/submit` only if needed by the verified route
- `GET /api/v1/payments/{id}`
- `GET /api/v1/payments/{id}/events`
- `POST /api/v1/payments/{id}/reconcile`
- `GET /api/v1/payments/{id}/evidence`
- `GET /api/v1/payments` only when workspace ownership/history access is secure

Do not add a public production fault-injection API. The simulation harness belongs in tests or an isolated local/development command and must not use real provider credentials to fabricate outcomes.

## State and evidence rules

- Client never sets state directly.
- Source submission does not imply settlement.
- Provider-native statuses remain available in event/evidence metadata after safe normalization.
- Mark a status `completed` only when the documented verification policy for the selected route is satisfied.
- Explicitly distinguish provider-reported status from independent on-chain verification.
- Reconciliation must not initiate a transfer.
- A timeout with unknown provider acceptance must not trigger a blind repeat value-moving call.
- Repeated observations must be idempotent; stale observations must not regress a confirmed terminal state.
- Preserve a recovery-required state when the correct next action cannot safely be inferred.

## Security requirements

- Never store private keys or seed phrases.
- Treat input and provider responses as untrusted; validate both.
- Bind quote/session to the exact networks, assets, amount, recipient and expiry.
- Workspace identity comes from authenticated credentials/session, never request JSON.
- Do not expose payment records across workspaces.
- Do not log secrets, auth tokens, signatures or unnecessary personal data.
- Do not store credentials in committed files.
- If provider credentials or route details are missing, document the blocker and keep the provider test double clearly isolated. Never report mocked success as a live integration.

## Working method and completion report

- Implement in small slices; run `gofmt`, unit tests, relevant static checks and API tests.
- Keep API docs/OpenAPI/Bruno/tests synchronized.
- Report actual route verified, exact commands run, results, environment variables, tests that are simulated versus live, and all remaining blockers.
