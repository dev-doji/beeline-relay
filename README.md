# Beeline Relay

**Payment guardrails for AI agents on NEAR Intents.**

Agents retry, lose context and act unsupervised — dangerous when they move money. Beeline Relay sits between an agent (or app) and the NEAR Intents 1Click API and makes every payment idempotent, policy-checked, bound to a signed quote, tracked through a server-enforced lifecycle, and exported as a provenance-labelled evidence packet. A NEAR smart-contract vault enforces spend limits on-chain, so even a compromised agent cannot drain funds.

> Built at NEAR Hackathon (1–14 Nov 2026). Tracks: AI Security & Defense, Agent Economy, Real-World Money.

## Status — what is live, simulated, or not yet verified

| Capability | Status |
|---|---|
| Go API skeleton, health endpoint, migrations | ✅ Implemented |
| Live 1Click quotes, stored with provider signature and parameter fingerprint | ✅ Implemented; live quote fetched 2026-11-04 (no funds moved) |
| Idempotent payment sessions (replay, conflict, concurrent retries), quote expiry, workspace isolation | ✅ Implemented and integration-tested on PostgreSQL |
| Server-side state machine (no stale regressions, terminal states final) | ✅ Implemented and tested; reconciliation that drives it is ⏳ next |
| 1Click quote signature verification (Go and on-chain) | ✅ Implemented; verified against a real 1Click quote and key |
| On-chain agent vault (NEAR): allowlist, signed-quote check, limits, single-use deposits, revocable agent keys | ✅ Implemented and unit-tested (23 tests); ❌ not yet deployed or audited |
| Live mainnet route (USDC NEAR → Base) | ❌ Not yet verified |
| MCP server for agents | ⏳ Planned |
| Developer console | ⏳ Planned |

This table is updated as work lands. Nothing is marked live until a real end-to-end payment has been run and recorded.

## Repository layout

```text
beeline-api/     Go API (chi, PostgreSQL, slog)
contracts/       NEAR agent vault contract (Rust, near-sdk)
docs/            Product spec, provider research, plan
AGENTS.md        Rules for coding agents working in this repo
```

## Run the API locally

Requirements: Go 1.27+, a PostgreSQL database (we use Neon).

```bash
cp .env.example beeline-api/.env   # then fill in DATABASE_URL
cd beeline-api
go run ./cmd/api
curl http://localhost:8080/api/v1/health
```

Create a workspace and API keys (printed once, stored only as hashes):

```bash
cd beeline-api
go run ./cmd/seed -name "Dev workspace"
```

Run tests. Integration tests run when `TEST_DATABASE_URL` points to a disposable database, and are skipped otherwise:

```bash
cd beeline-api
go test ./...
```

## API so far

| Method | Path | Auth | Purpose |
|---|---|---|---|
| GET | `/api/v1/health` | – | Service and database health |
| GET | `/api/v1/capabilities` | – | Configured routes and **route verification status** |
| GET | `/api/v1/networks`, `/api/v1/assets?network=` | – | Supported networks and canonical asset IDs |
| POST | `/api/v1/quotes` | Bearer | Live 1Click quote, stored immutably with signature and fingerprint |
| GET | `/api/v1/quotes/{id}` | Bearer | Read a stored quote |
| POST | `/api/v1/payments` | Bearer + `Idempotency-Key` | Create a payment from a quote: replay-safe, conflict-checked |
| GET | `/api/v1/payments`, `/api/v1/payments/{id}` | Bearer | Workspace-scoped payments |
| GET | `/api/v1/payments/{id}/events` | Bearer | Append-only event timeline |

## License

MIT
