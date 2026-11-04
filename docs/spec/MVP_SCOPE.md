# Beeline Relay MVP Scope and Acceptance Criteria

## Priority order

### P0 — required for a credible hackathon demo

- [ ] Rename product messaging from consumer cross-chain payment convenience to developer-facing payment reliability and reconciliation. Keep the name “Beeline Relay” unless the repository's existing brand constraints require otherwise.
- [ ] Inspect current code/repository before changing structure; preserve reusable components and existing conventions.
- [ ] Verify current official NEAR Intents docs and one supported route end to end; record source links, route, environment, required authorization steps, status API, and known limitations.
- [ ] Confirm canonical network/asset identifiers for the selected route; do not hardcode guessed identifiers.
- [ ] Implement health and verified capability/network/asset endpoints.
- [ ] Implement live quote retrieval with strict validation, expiry handling, and normalized provider references.
- [ ] Implement payment-session creation bound to an exact quote and protected by `Idempotency-Key`.
- [ ] Reject reuse of an idempotency key with a different normalized request.
- [ ] Persist payment sessions and append-only events in PostgreSQL.
- [ ] Define and test legal state transitions; clients must not set authoritative states.
- [ ] Implement the exact provider execution flow required for the selected route; require explicit wallet authorization where applicable.
- [ ] Implement reconciliation using documented provider status and available network evidence.
- [ ] Implement evidence JSON export with evidence source, observation time, provider/reference IDs, and known limitations.
- [ ] Implement a read-only or safe reconcile action that cannot itself initiate a second transfer.
- [ ] Build developer console: session list/detail, timeline, current state, recovery-required state, and evidence viewer/export.
- [ ] Build local deterministic fault-test harness with clearly simulated scenarios; no simulated state may be labelled a real payment.
- [ ] Add Bruno collection for local API testing.
- [ ] Update website to explain Beeline Relay's developer reliability use case without unsupported performance/security claims.
- [ ] Document setup, environment variables, API contract, real-route verification, tests, demo flow, and limitations.

### P1 — explicitly deferred

- [ ] Public SDK/package publication.
- [ ] Multiple payment providers or many routes/assets.
- [ ] Production webhook fan-out to client applications.
- [ ] Scheduled reconciliation jobs beyond what the live demo needs.
- [ ] Notifications/alerting integrations.
- [ ] Multi-user workspace administration and billing.
- [ ] Analytics, hosted reliability scorecards, and long-term metrics.
- [ ] A large scenario marketplace or configurable simulation DSL.

## Acceptance criteria

### Integration and quote safety

- Invalid address, amount, network, asset, or unsupported route is rejected with a typed error.
- Only routes and assets verified against current provider documentation/configuration are advertised.
- Quote response is real, fresh, and bound to the exact source/destination parameters.
- Quote expiry is enforced server-side; an expired quote cannot create an executable payment without refreshing/re-quoting.
- A materially changed recipient, amount, source/destination asset, or network invalidates the quote binding.

### Idempotency and execution

- Payment creation requires an idempotency key for value-moving workflow initiation.
- Same key + same normalized request returns the original logical result and does not create another payment.
- Same key + conflicting request returns `IDEMPOTENCY_CONFLICT`.
- An uncertain provider timeout does not trigger a blind second value-moving call.
- Any wallet authorization is initiated only after explicit user action and follows the selected provider's verified flow.
- No private key or seed phrase is ever requested, stored, transmitted to the backend, or logged.

### State machine and event log

- Payment states are validated server-side; invalid transitions are rejected.
- Every accepted state change has a corresponding persisted event with timestamp and source/provenance.
- Duplicate observations are idempotent.
- Older/stale observations cannot regress a confirmed terminal state.
- Source transaction submission alone does not produce `completed`.
- Delayed settlement remains pending; ambiguous outcomes become `recovery_required` when the verification policy cannot safely resolve them.
- Reconciliation is read-oriented and must not create a new payment or silently re-submit a transaction.

### Evidence packet

- Evidence includes session ID, immutable request fingerprint, normalized quote/payment details, lifecycle events, transaction references where available, and the basis for the current state.
- Provider-reported status is labelled as provider-reported; independently checked chain data is labelled as chain-verified. Do not imply one is the other.
- Missing or unavailable evidence is explicit; the packet never invents a transaction hash or proof.
- Export format is stable JSON with a documented schema version.

### Reliability test harness

At minimum, provide repeatable tests for:
1. duplicate create request with same key and payload;
2. same idempotency key with conflicting payload;
3. expired quote;
4. quote/payment parameter mismatch;
5. duplicate provider observation or callback;
6. out-of-order/stale observation;
7. source submitted but destination settlement pending;
8. provider timeout with uncertain execution outcome;
9. unsupported route;
10. invalid transition after a terminal state.

Simulated scenarios must run against test doubles or test fixtures, be visibly identified as simulated, and be impossible to confuse with real payment records in the console.

### Delivery

- The frontend marketing website and developer console remain separate Next.js applications if that matches the current repository layout.
- The Go API runs independently and uses PostgreSQL migrations.
- OpenAPI, API_SPEC.md, implementation, and Bruno collection agree.
- Unit/integration tests, formatter, lint, type checks, and production build pass where available.
- No fake live quote, supported route, transaction reference, settlement, adoption metric, or security certification is shown.
