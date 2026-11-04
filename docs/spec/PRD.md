# Beeline Relay — Product Requirements Document (PRD)

**Document status:** Refined hackathon MVP specification  
**Product name:** Beeline Relay (working name; retain “Beeline” branding if the repository already uses it)

## 1. Product summary

Beeline Relay is a developer-facing reliability and reconciliation layer for applications that initiate cross-chain payments through NEAR Intents. It helps an integrating application create a payment session, preserve the exact approved payment parameters, track lifecycle events without unsafe state regressions, reconcile payment status against documented provider/network evidence, and export a portable evidence record.

Beeline Relay does **not** replace NEAR Intents, provide a new bridge, or claim that payment routing itself is novel. NEAR Intents remains the planned execution/routing integration for the verified route. The product's proposed differentiation is the application-level correctness, idempotency, observability, and recovery workflow around a payment.

**Product principle:** Every payment state should be explainable, auditable, and safe to retry—or explicitly unsafe to retry.

## 2. Problem

Payment applications must handle more than a quote and a transaction hash. They need to deal with expired quotes, duplicate client requests, repeated provider events, delayed settlement, provider timeouts with uncertain outcomes, and the difference between a source transaction being submitted and a destination payment being settled. If these conditions are mishandled, an application may show a false success, create duplicate payment attempts, or leave an operator without enough evidence to resolve a case.

Existing cross-chain routing infrastructure handles parts of execution and status reporting. Beeline Relay focuses on the application's lifecycle contract: stable payment identifiers, idempotent operations, validated transitions, an append-only event timeline, reconciliation, evidence export, and deterministic tests for failure cases.

This is a product hypothesis. During implementation, verify which capabilities are already supplied by the current NEAR Intents APIs and avoid rebuilding them unnecessarily. Beeline Relay should normalize and improve the integrating application's workflow, not claim to replace underlying provider guarantees.

## 3. Target users

### Primary users
- Developers building wallets, marketplaces, payment experiences, and fintech applications that use cross-chain payments.
- Small product teams that need reliable lifecycle handling without each implementing an ad hoc state machine.
- Developers of agentic commerce applications that need safe payment status and audit records.

### Secondary users
- Support and operations staff investigating delayed, failed, or uncertain payments.
- Technical evaluators who need to reproduce and test payment failure handling.

## 4. Core use case

An application creates a payment session with a specific quote and a stable client reference. Beeline Relay binds the session to the exact source/destination parameters, records the required authorization/execution steps, tracks provider/network observations, and only marks the payment completed when the configured verification policy is satisfied. If the provider outcome is uncertain, Beeline Relay records that uncertainty and directs the operator toward reconciliation rather than blindly retrying a value-moving action.

## 5. Product goals

1. Verify one currently supported NEAR Intents payment route end to end.
2. Keep each payment bound to immutable, normalized payment parameters and its accepted quote.
3. Make create/submit operations idempotent where the provider contract permits safe semantics.
4. Enforce backend-controlled state transitions and an append-only event history.
5. Distinguish source submission, processing, verified settlement, failure, expiry, and uncertain outcomes.
6. Reconcile state using the current documented NEAR Intents status flow and any additional evidence actually available for the selected route.
7. Export a machine-readable evidence packet with provenance and clear limitations.
8. Demonstrate reliability using deterministic fault-injection tests that are clearly separated from real payment activity.
9. Provide a small developer console and website that explain and demonstrate the product honestly.

## 6. MVP scope (P0)

- A standalone Go API and PostgreSQL persistence.
- A verified NEAR Intents adapter for one explicitly documented route and environment.
- A canonical capabilities/network/asset catalogue containing only verified options.
- Live quote retrieval and strict quote-to-payment binding.
- Payment-session creation with idempotency keys and conflict detection.
- A documented provider-compatible execution flow requiring explicit wallet authorization where the route requires user signing.
- A validated payment state machine and append-only payment event log.
- Reconciliation using documented provider status and available transaction references.
- A payment evidence endpoint/export that identifies what each status claim is based on.
- A developer console showing payment sessions, lifecycle timeline, current status, retry/recovery guidance, and evidence JSON.
- A deterministic local test harness for failure cases; simulated cases must be visibly labelled and must never masquerade as real settlements.
- An updated website and complete setup, API, test, environment, and limitation documentation.
- Tests for parameters, quote expiry, idempotency, state transitions, evidence provenance, and recovery-required behavior.

## 7. P1 — only after P0 is reliable

- A versioned public SDK or generated typed client.
- Additional verified NEAR Intents routes/assets.
- Webhook delivery to integrating applications, if retries, signing, and delivery semantics are implemented.
- Configurable alerting and notifications.
- Workspace/project management and broader multi-tenant administration.
- Additional payment providers behind the same normalized adapter interface.
- Hosted sandbox and scheduled reliability reports.

## 8. Explicitly out of scope

- A new bridge, solver network, or blockchain.
- A general consumer payment frontend competing on routing convenience.
- Custody of user funds, private keys, or seed phrases.
- Automatic transaction signing or value-moving retries without explicit authorization and verified provider semantics.
- Claims of guaranteed settlement times or universal route availability.
- General-purpose smart-contract escrow, AI arbitration, or a generic agent marketplace.
- A production multi-provider abstraction before one integration works end to end.
- Production fault-injection endpoints that can affect real payments.
- Token issuance, lending, trading, yield products, or fiat payouts.

## 9. Primary user journey

1. An integrating application or the developer console requests a live quote for exact source/destination parameters.
2. Beeline Relay validates the parameters, normalizes the provider response, and records the quote expiry and provider references.
3. The application creates a payment session using the quote ID and an `Idempotency-Key`.
4. The backend verifies that the quote belongs to the exact requested parameters and is still valid; it persists the session and its initial event.
5. The client reviews all material payment details and explicitly authorizes any required wallet action.
6. Execution follows the **current official provider flow**. Beeline Relay does not invent a generic submit flow if the provider requires a different one.
7. The backend records submission and subsequent provider/network observations as append-only events.
8. Reconciliation updates the normalized state without allowing stale or duplicate observations to corrupt the lifecycle.
9. The developer inspects the session timeline and exports an evidence packet.
10. If the outcome cannot safely be determined, the state becomes `recovery_required` (or remains pending where appropriate); the system must not fabricate a successful result or blindly resubmit a value-moving action.

## 10. Product principles

- **Correctness over cosmetics:** state and evidence are the primary product.
- **Idempotent by design:** repeated requests must have explicit, tested semantics.
- **Evidence over assumption:** `completed` requires evidence defined for the selected integration.
- **Uncertainty is a valid state:** do not hide ambiguous outcomes behind success or failure labels.
- **No custody:** users keep their signing keys and explicitly authorize any required transaction.
- **Provider-aware, not provider-blind:** preserve important provider state and references; document normalization decisions.
- **Honest boundaries:** clearly label mocks, test scenarios, unsupported routes, and evidence limitations.
- **Narrow first release:** prove one route and failure model before expanding support.

## 11. Success criteria

The MVP is demonstrable only when:

- The current official NEAR Intents integration flow has been reviewed and recorded in the repository.
- One real route succeeds end to end in the chosen environment, with no fabricated quote, hash, or status.
- The same `Idempotency-Key` and same request return the same logical payment; reuse with a conflicting request is rejected.
- Expired or parameter-mismatched quotes cannot be used.
- State transitions are enforced by the backend and events persist across restarts.
- Source submission alone never means `completed`.
- Reconciliation handles duplicate and stale observations without regressing terminal state.
- A delayed or ambiguous provider result leads to pending/recovery handling, not blind re-submission.
- Evidence export includes status provenance and explicitly states missing evidence.
- At least eight deterministic fault scenarios are tested and reproducible.
- The developer console distinguishes simulated test runs from real payments.
- Setup, limitations, API contract, and commands are documented; automated tests pass.

## 12. Integration references

Use current official documentation and verify it at implementation time:
- NEAR Intents developer documentation: https://docs.near-intents.org/
- NEAR Intents swap/status tutorial: https://docs.near.org/tutorials/intents/swap
- NEAR Intents FAQ: https://docs.near.org/chain-abstraction/intents/faq

These links are starting points, not permission to assume a particular payload, endpoint, authentication method, route, or settlement guarantee. Record the exact docs and route verified during implementation.
