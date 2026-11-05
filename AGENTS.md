# Instructions for Coding Agents — Beeline Relay

## Work process

0. Read `docs/HANDOFF.md` for the current state and next steps.
1. Read `README.md`, `docs/PRD.md`, `docs/TRD.md`, `docs/ARCHITECTURE.md`, `docs/API_SPEC.md`, and `docs/MVP_SCOPE.md` before architecture or product changes.
2. Read the component prompt in `prompts/` before implementing website, developer console, or backend work.
3. Inspect the existing repository and working tree first. Preserve existing conventions and do not overwrite useful work without understanding it.
4. Verify current official NEAR Intents documentation before implementing provider-specific methods, payloads, authentication, route IDs, execution steps, or status mapping.
5. Implement small, verifiable slices. After each slice, run relevant tests, formatting, linting, type checks, and build checks available in the repo.
6. Keep `API_SPEC.md`, OpenAPI, implementation, tests, Bruno requests, and UI types synchronized.
7. Keep `README.md` exactly true after every change: status table, commands and test names. Judging is agentic (Claude Code, Codex and Loops House Agentic Judging check that the project runs and matches its README), so an unbacked or stale claim costs points.
8. Report what is implemented, tested, simulated, live-verified, incomplete, and blocked by external credentials/manual actions.

## Product boundaries

- Beeline Relay is a developer-facing reliability, lifecycle, reconciliation, and evidence layer around supported cross-chain payments. It is not a new bridge or generic consumer payment frontend.
- NEAR Intents is the P0 planned provider. Do not claim the product replaces the provider's routing or settlement guarantees.
- The proposed differentiators are parameter-bound sessions, tested idempotency, validated transitions, append-only events, reconciliation, evidence provenance, and reproducible failure tests. Verify what the provider already supplies and avoid duplicate claims.
- Keep the MVP narrow: one currently supported end-to-end route and a reliable failure-handling model before adding providers or many routes.

## Non-negotiable safety and correctness rules

- Never collect, store, transmit, or log private keys or seed phrases.
- Never sign/submit a user transaction without explicit user authorization through the appropriate wallet.
- Never claim payment completion because the source transaction was signed/submitted.
- Never fabricate quotes, fees, supported routes, chain/asset identifiers, hashes, status, settlement evidence, testimonials, metrics, audits, or uptime claims.
- Treat provider responses, callbacks and all client input as untrusted until validated.
- Do not accept client-supplied `completed` state as authoritative.
- Use canonical network/asset IDs, not symbols alone.
- Use decimal strings or integer base units for amounts; avoid floating-point money arithmetic.
- Bind quotes to exact source/destination networks, assets, amount, recipient and expiry.
- Same idempotency key + same request must not create a second logical payment; conflicting payload must be rejected.
- Do not blindly retry a value-moving operation when the outcome is uncertain or provider duplicate semantics are unknown. Reconcile first.
- Reconciliation is read-oriented and must not submit another transfer.
- Duplicate/out-of-order observations must not duplicate events or regress a confirmed terminal state.
- Label provider-reported state separately from independent chain verification.
- Keep simulations/test fixtures clearly identified; they must never appear as real successful payments.
- Do not expose secrets, provider headers, raw auth tokens, sensitive wallet payloads, or internal stack traces.
- Do not expose another workspace's payments/events/evidence.

## Integration discipline

- Start from current official docs: https://docs.near-intents.org/ and relevant documentation at https://docs.near.org/.
- Record the exact endpoint/method, authentication, supported route, expected authorization input, status method, state semantics, and limitations verified for the chosen route.
- Do not invent provider endpoints or SDK methods. If documentation/credentials block integration, document the blocker and implement only isolated interfaces/test doubles clearly labelled as such.
- Provider-specific logic belongs behind an internal adapter.
- Do not expose a production fault-injection endpoint. Failure tests run through test doubles or isolated development test tooling.

## Code quality

- Prefer clear, explicit code over unnecessary abstractions.
- Add tests for idempotency, quote binding/expiry, state transitions, uncertain provider outcomes, evidence provenance and redaction.
- Keep event history append-only at the application level.
- Use migrations and safe configuration; `.env.example` must contain placeholders only.
- Keep website, console, and backend independently buildable/deployable according to the repository layout.
