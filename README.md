# Beeline Relay

**Payment guardrails for AI agents on NEAR Intents.**

Agents retry, lose context and act unsupervised — dangerous when they move money. Beeline Relay sits between an agent (or app) and the NEAR Intents 1Click API and makes every payment idempotent, policy-checked, bound to a signed quote, tracked through a server-enforced lifecycle, and exported as a provenance-labelled evidence packet. A NEAR smart-contract vault enforces spend limits on-chain, so even a compromised agent cannot drain funds.

> Built at NEAR Hackathon (1–14 Nov 2026). Tracks: AI Security & Defense, Agent Economy, Real-World Money.

## Status — what is live, simulated, or not yet verified

| Capability | Status |
|---|---|
| Go API skeleton, health endpoint, migrations | ✅ Implemented |
| 1Click quote / payment sessions / idempotency | ⏳ In progress |
| On-chain agent vault (NEAR) | ⏳ Planned |
| Live mainnet route (USDC NEAR → Base) | ❌ Not yet verified |
| MCP server for agents | ⏳ Planned |
| Developer console | ⏳ Planned |

This table is updated as work lands. Nothing is marked live until a real end-to-end payment has been run and recorded.

## Repository layout

```text
beeline-api/     Go API (chi, PostgreSQL, slog)
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

Run tests:

```bash
cd beeline-api
go test ./...
```

## License

MIT
