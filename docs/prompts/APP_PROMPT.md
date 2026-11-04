# Coding Agent Prompt — Beeline Relay Developer Console

## Your role

Act as a senior frontend engineer and product designer building Beeline Relay's developer console. Read `AGENTS.md`, `docs/PRD.md`, `docs/TRD.md`, `docs/ARCHITECTURE.md`, `docs/API_SPEC.md`, and `docs/MVP_SCOPE.md` first. Inspect the repository and current working tree before coding.

## Goal

Build a polished developer-facing console for inspecting and testing cross-chain payment reliability. Beeline Relay is not being positioned as another consumer send-money app. The console should demonstrate payment-session correctness, lifecycle events, reconciliation, evidence provenance, and failure handling around one verified NEAR Intents route.

## Stack and location

- Next.js App Router, TypeScript, React, Tailwind CSS.
- Zod for form/schema validation.
- TanStack Query where useful for API state.
- Existing UI/shared packages where present.
- Current default location: `beeline-web/apps/app/`. If the repository uses another path, retain the working convention and report it; do not rename the app without need.
- Go API runs separately. Provider credentials must never be embedded in browser code.

## Required screens and flow

1. **Overview:** concise explanation of Beeline Relay and clearly labelled environment (local/test/live) sourced from configuration/API, never inferred.
2. **Create session / playground:** enter exact source/destination payment parameters or select a saved quote; validate inputs and show the idempotency key behavior in a developer-friendly way.
3. **Quote review:** show source asset/network, source amount, destination asset/network, recipient, expected output, available fees, expiry, quote/provider reference where safe, and any required next action.
4. **Session detail:** payment ID, client reference, current normalized state, latest reconciliation timestamp, source/destination references where available, and a clear distinction between provider status and chain-verified facts.
5. **Event timeline:** append-only lifecycle observations with event type, time, source/provenance and transition outcome. Explain stale/duplicate events when useful.
6. **Reconciliation:** invoke only the API's read-oriented reconciliation endpoint. Never create or resubmit a payment as part of refresh.
7. **Evidence packet:** formatted JSON viewer and download/export; clearly display evidence limitations and schema version.
8. **Reliability scenarios:** show deterministic test runs (duplicate request, idempotency conflict, expired quote, stale event, uncertain provider timeout, etc.) in a separate test/simulation area that cannot be mistaken for real payment activity.
9. **States and errors:** loading, empty, invalid input, unsupported route, quote expired, provider unavailable, authorization rejected, pending, completed, failed, and recovery-required states.

## Functional requirements

- Consume the Go API according to `docs/API_SPEC.md` and the actual OpenAPI contract.
- Do not duplicate backend validation/state machine logic in the frontend.
- Use typed responses/errors and safe retry behavior.
- Never allow the browser to set payment state to `completed`.
- Do not retry value-moving actions automatically after timeouts.
- Do not include provider/API secrets in `NEXT_PUBLIC_*` variables. Use a server-side proxy/session boundary where authentication requires a secret.
- Any wallet authorization follows the exact verified provider flow and only follows an explicit user action after quote review.
- Display simulated events and real provider events with distinct labels, colors and copy.
- Do not show fake balances, fake settlement hashes, fabricated route availability or made-up reliability metrics.

## Design direction

- Professional developer tooling / fintech infrastructure aesthetic: precise typography, generous whitespace, clear status hierarchy, compact but readable tables/timelines.
- Strong information design rather than a generic crypto dashboard.
- Make event provenance and uncertainty visually obvious.
- Restrained motion, accessible contrast, keyboard focus, semantic markup and reduced-motion support.
- Responsive at mobile, tablet and desktop widths.

## Acceptance criteria

- Console builds and passes lint/type checks.
- A developer can create/retrieve a session through the API where the backend and route are configured.
- Quote expiry and typed API errors are surfaced clearly.
- Session states and timeline come from the backend, not local guesses.
- Reconciliation triggers no transfer.
- Evidence can be inspected/exported and limitations are visible.
- Simulated test results cannot be mistaken for a real payment.
- No secret, private key or seed phrase is requested or persisted.
- No dead controls or unsupported claims.

## Working method

Inspect the repository, state a brief plan, and implement incrementally. Use clearly labelled mock data only for isolated UI work if the API is not ready. Run relevant tests, lint, type checks and build. At the end, report files changed, commands run, tested workflows, missing environment values and remaining blockers.
