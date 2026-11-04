# Beeline Relay Architecture

## 1. System overview

```text
Integrating Application / Developer Console
        |                         |
        | HTTPS JSON API          | Explicit wallet authorization where required
        v                         v
Standalone Go API <---------- Existing user wallet
        |
        +--> PostgreSQL
        |      +--> payment_sessions
        |      +--> idempotency records
        |      +--> append-only payment_events
        |      +--> normalized quotes / evidence references
        |
        +--> NEAR Intents adapter
        |      +--> live quote / documented execution operations
        |      +--> documented status lookup
        |
        +--> Reconciliation service
        |      +--> status normalization
        |      +--> state-transition validation
        |      +--> evidence packet assembly
        |
        +--> Local reliability test harness (test doubles only)

Next.js Marketing Website ---> Developer Console / Docs CTA
```

The diagram is conceptual. Exact quote, authorization, submission, and status calls must follow the current official NEAR Intents documentation for the chosen route. Do not infer a universal flow from the diagram.

## 2. Responsibilities

### Next.js website
- Explain the developer problem, value proposition, core behavior, limitations, and CTA to the console/docs.
- Make no unsupported performance, route-support, security audit, adoption, or reliability guarantees.

### Next.js developer console (`apps/app` unless repository convention differs)
- Create/view payment sessions using the Go API.
- Display the quote and required authorization step when applicable.
- Show the normalized current state and append-only timeline.
- Trigger a read-only reconciliation request and show its result.
- Display/download the evidence packet.
- Run or inspect deterministic test scenarios only in an explicitly identified test/sandbox view.
- Never hold provider secrets in browser code or determine final payment status itself.

### Go API
- Validate input and authenticated workspace boundaries.
- Bind stored quote to immutable payment parameters.
- Enforce idempotency and state transitions.
- Persist sessions and events transactionally where possible.
- Coordinate provider requests according to documented integration semantics.
- Reconcile status and construct a versioned evidence packet.
- Expose typed errors and safe observability.

### NEAR Intents adapter
- Encapsulate provider-specific API methods, auth, IDs, payload mapping and native status semantics.
- Expose a small internal interface to quote, execute/submit when applicable, and read status.
- Preserve raw status/reference fields needed for diagnosis, with secrets and sensitive fields removed.
- Never assume a submit timeout means the operation did not occur.

### PostgreSQL
- Store payment/session records, normalized quote snapshots, idempotency-key records, and append-only events.
- Use migrations and database constraints where possible to prevent duplicate idempotency records and broken ownership relationships.
- Do not store wallet private keys or seed phrases.

### Reliability test harness
- Operates only with deterministic test doubles/fixtures and isolated test records.
- Simulates duplicate/out-of-order events, timeouts, expiry, mismatches and ambiguous outcomes.
- Clearly labels simulated events. It cannot mint or imitate a real successful provider/chain reference.
- Is not a public production endpoint.

## 3. Main request flow

1. Client requests a live quote with exact source/destination parameters.
2. API validates and normalizes the request, calls the documented provider interface, and stores a quote snapshot including expiry and parameter fingerprint.
3. Client creates a payment session with quote ID and `Idempotency-Key`.
4. API checks workspace ownership, quote expiry, fingerprint match and idempotency record; then persists the session and first event.
5. Client reviews details and explicitly authorizes any wallet action required by the documented route.
6. The verified provider execution flow is followed. Only its documented submission input is accepted by the API.
7. API persists safe submission references and state transitions; repeated events are deduplicated.
8. Reconciliation fetches current status using documented status mechanisms and records the observation plus provenance.
9. A verified state policy maps observations into normalized states. Older observations must not regress a terminal state.
10. The evidence endpoint assembles a versioned packet from persisted records and marks unsupported/missing evidence explicitly.

## 4. State machine and event model

The payment row stores the current normalized state for efficient reads. `payment_events` is append-only and records each accepted observation/transition. State changes are performed through one domain service, not scattered controller logic.

Minimum safeguards:
- Reject impossible state transitions.
- Treat repeated provider observations as idempotent.
- Preserve native provider status and its observed timestamp.
- Use observation ordering/versioning where available; do not rely only on arrival time if provider sequence/timestamps exist.
- Never change `completed` to a non-terminal state because of a stale event.
- If the status is contradictory or an execution outcome is uncertain, record a diagnostic event and move to `recovery_required` only according to the documented state policy.
- `reconcile` is read-oriented and must not submit a new transfer.

## 5. Trust boundaries

- Browser and client payloads are untrusted; validate server-side.
- Wallet is the only place user private signing keys should reside. User authorization must be explicit.
- Workspace authentication determines access; caller-supplied workspace IDs are not authoritative.
- NEAR Intents and underlying networks are external dependencies. Treat timeouts, delays, duplicate observations, schema changes and provider errors as normal failure modes.
- A source-chain submission reference is not necessarily destination settlement evidence.
- A provider-reported `SETTLED`/equivalent status must be labelled as provider-reported unless independent chain verification is implemented and documented.
- The test harness is not a source of production settlement evidence.

## 6. Data ownership and retention

- Store only the fields needed to execute, reconcile, debug and audit a payment.
- Store normalized IDs and minimal provider references; redact credentials and sensitive request attributes from logs/events.
- Keep evidence exports traceable to event IDs and a schema version.
- Decide and document retention/deletion behavior before a public multi-tenant launch; the hackathon MVP can use a single configured demo workspace if access controls are explicit.

## 7. Failure behavior

- **Expired quote:** reject creation/execution and request a fresh quote.
- **Duplicate create:** return the existing payment for the same idempotency key and payload.
- **Conflicting duplicate:** return an idempotency conflict.
- **Provider timeout before known acceptance:** do not automatically repeat a value-moving request unless provider semantics permit it.
- **Uncertain submission:** preserve context and reconcile; report recovery required when evidence cannot establish a safe next action.
- **Delayed settlement:** remain pending/processing.
- **Unsupported route:** return explicit route unavailable error.
- **Reconciliation provider unavailable:** retain last verified state, add a failed observation event, and show last-check timestamp.

## 8. Deployment shape

- `apps/website`: public website.
- `apps/app`: developer console.
- Separate Go API service.
- PostgreSQL database for durable records.
- Provider secrets available only to the backend.
- HTTPS and strict CORS/authentication for deployed environments.
- Local development mocks/test doubles isolated from live integration configuration.
