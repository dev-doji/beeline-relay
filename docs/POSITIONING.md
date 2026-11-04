# Beeline Relay: Positioning and PRD Addendum (v2)

This file **amends** `PRD.md`, `MVP_SCOPE.md`, `API_SPEC.md` and the three prompts. Where they conflict, this file wins. All the safety and honesty rules in `AGENTS.md` still apply unchanged.

## One-liner
**Beeline Relay: payment guardrails for AI agents on NEAR Intents.**

## Tagline options
- "Agents will move money. Make every payment safe to retry, or explicitly unsafe to retry."
- "Your agent's payment timed out. Beeline knows what happened."
- "Idempotency, spend limits and evidence for agent payments on NEAR Intents."

## Problem (agent framing)
AI agents are now being trusted to move money through cross-chain rails like NEAR Intents. Agents are also the worst possible payment clients:
- they **retry** when a call times out;
- they **lose context** between steps;
- they **can't tell** "submitted" from "settled";
- they act **without a human watching**.

Without guardrails, an agent can double-pay, send funds to an expired deposit address (1Click's docs say funds "may be lost"), overspend, or report success that never happened. There's also no audit trail afterwards, and since 27 Oct 2026 1Click swaps settle on a private shard, so the swap step has no public hash to look up.

## Solution
A non-custodial reliability and policy layer between agents (or apps) and the NEAR Intents 1Click API:
1. **Idempotent payment sessions.** Retries return the same payment, and a conflicting reuse is rejected.
2. **Quote binding.** Each payment is bound to 1Click's *signed* quote, with exact parameters and deadline.
3. **Spend policies.** Per-payment limits, daily budgets, recipient and route allowlists, and human approval above a threshold.
4. **Server-enforced lifecycle.** Append-only events, no stale regressions, and an explicit `recovery_required` state that tells an agent to stop instead of guessing.
5. **Evidence packets.** Every claim is labelled `chain_verified` or `provider_reported`, and the packet says plainly what can't be verified (the private-shard swap step).
6. **MCP server.** Any MCP-capable agent gets safe payments in one config block.
7. **Chaos mode.** Reproducible failure scenarios that prove all of the above (clearly simulated).

**Not:** a bridge, a wallet, a custodian, or a replacement for NEAR Intents routing. Beeline never holds keys, and signing happens in the user's wallet or the operator's own agent process.

## Why now
- Agent frameworks plus MCP made agent-initiated payments practical in 2026.
- NEAR Intents and 1Click make cross-chain execution one API call, so the hard part has moved from **routing** to **knowing what happened and acting safely on it**.
- 1Click went confidential-only on 27 Oct 2026, so provenance-aware evidence is now necessary, not optional.

## Business model (sustainability)
- **Protocol-native revenue.** 1Click's `appFees` parameter adds integrator basis points to each routed swap, split 50/50 with 1Click for authenticated partners. Beeline earns per payment with no billing system.
- Open-source core (API, MCP server, policy engine). Later: hosted workspaces, longer evidence retention, alerting (P1).
- Users: agent developers, agentic-commerce apps, wallets and treasuries running automated payouts.

## Tracks and the pitch angle for each
| Track | Angle |
|---|---|
| **Agent Economy** (primary) | The safe payment primitive for agents transacting on NEAR Intents. MCP-native. |
| **AI Security & Defense** | Spend policies, double-pay prevention, deadline guards and human-in-the-loop approval for autonomous agents. |
| **Real-World Money** | Stablecoin payments with real mainnet settlement and auditable evidence. |

## What each judge will probably look for (based on public background; check for anything recent)
| Judge | What they likely care about | What to make sure they see |
|---|---|---|
| Illia Polosukhin (NEAR) | Intents, chain abstraction, agents, user-owned/private AI | Built properly on 1Click. Agent-first. Evidence across private-shard settlement. Optional NEAR AI Cloud model. |
| Eddy Lazzarin (a16z crypto) | Technical soundness, mechanism and system design | State machine, idempotency semantics, the provenance model, the test suite |
| Mert Mumtaz (Helius) | Real usage, infra that people actually need, no fluff | Real mainnet payments. Other teams' agents using the MCP server. A one-line install. |
| Avichal Garg (Electric Capital) | Developer experience and ecosystem pull | A 3-minute quickstart, OpenAPI, Bruno, a clean repo |
| Kain Warwick (Synthetix) | Mechanisms, incentives, sustainability | The `appFees` revenue model. Policy design. |
| Haseeb Qureshi (Dragonfly) | Market size, why now, defensibility | The agent-payments wave plus the confidential-only shift means a real need for an evidence and policy layer |
| Stepan Gershuni | AI agents plus crypto | MCP server, autonomous agent demo, guardrails |
| Claude Code / Codex (AI judges) | The repo: does it run, are claims true, is the code good | Green CI. `go test ./...` covers the 12 scenarios. README table of live / simulated / unverified. No over-claims. |

## Spec changes (PRD / MVP_SCOPE / API_SPEC)

### New P0 items
- [ ] **Agent API keys:** scoped per workspace, stored hashed, revocable. `agent_id` recorded on every payment and event.
- [ ] **Spend-policy engine:** max per payment, daily budget per agent, recipient allowlist, asset and route allowlist, approval threshold. Evaluated inside the same transaction as payment creation.
- [ ] **New state `awaiting_approval`:** sits between `quoted` and `awaiting_authorization` when the approval threshold is exceeded. Only a human console session can approve. Agent keys can never approve their own payments.
- [ ] **New state `refunded`:** justified because 1Click returns `REFUNDED` together with `refundedAmount` and `refundReason`.
- [ ] **New events:** `policy_passed`, `policy_blocked`, `approval_requested`, `approved`, `rejected`.
- [ ] **New error codes:** `POLICY_LIMIT_EXCEEDED`, `POLICY_RECIPIENT_NOT_ALLOWED`, `POLICY_BUDGET_EXHAUSTED`, `APPROVAL_REQUIRED`, `DEADLINE_TOO_CLOSE`.
- [ ] **New endpoints:**
  - `POST /api/v1/payments/{id}/approve` and `/reject` (console session only)
  - `GET/PUT /api/v1/policies` (console session only)
  - `GET /api/v1/payments` (now P0, scoped to the workspace)
- [ ] **`beeline-mcp`** (TypeScript, official MCP SDK). Tools: `get_quote`, `create_payment`, `submit_deposit`, `reconcile_payment`, `get_payment`, `get_evidence`, `check_policy`. Tool descriptions tell the agent explicitly: *"If state is recovery_required, do not send funds again; report to a human."*
- [ ] **Reference agent:** invoice-paying task, MCP client, swappable model (NEAR AI Cloud or Claude), local signer (Mode B) or human approval (Mode A).
- [ ] **Chaos mode** in the console: 12 scenarios, run against the provider test double through the real API code paths in an isolated sandbox workspace. Labelled SIMULATED everywhere.
- [ ] **Evidence packet adds:** `agent_id`, the policy decisions, the approval record, the signed 1Click quote, and a limitations entry saying "swap step settles on NEAR Private Shard; provider-reported only".

### Updated state list
`created, quoted, awaiting_approval, awaiting_authorization, submitted, processing, completed, failed, refunded, expired, recovery_required`

### Reliability scenarios (12)
1. Duplicate create (same key, same body) returns a replay.
2. Same key with a conflicting body returns `IDEMPOTENCY_CONFLICT`.
3. Expired quote is rejected.
4. Quote and parameter mismatch is rejected.
5. Duplicate provider observation is deduplicated.
6. Out-of-order or stale observation causes no regression.
7. Submitted, but settlement still pending, stays `processing`.
8. Provider timeout with an uncertain outcome leads to `recovery_required` with no resubmit.
9. Unsupported route returns `ROUTE_UNAVAILABLE`.
10. Invalid transition after a terminal state is rejected.
11. **Over budget or a disallowed recipient returns `policy_blocked`.**
12. **Above the threshold, the payment waits in `awaiting_approval`, and the agent can't approve it.**

### Website copy changes
- Hero: "Payment guardrails for AI agents on NEAR Intents."
- Subhead: "Idempotent sessions, spend policies, and provenance-labelled evidence, so your agent never double-pays, never pays into an expired deposit, and never claims a payment it can't prove."
- Primary CTA: "Connect your agent" (MCP quickstart). Secondary: "Open console".
- Keep every honesty rule. Say "verified route" only after live run #1. Use no metrics we haven't measured.

## Submission description draft (≤2,000 characters; finalize on Day 13)
> **Beeline Relay: payment guardrails for AI agents on NEAR Intents.**
> Agents retry, lose context and act unsupervised, which is dangerous when they move money. Beeline Relay is a non-custodial layer between agents and the NEAR Intents 1Click API. It makes every payment idempotent (retries can't double-pay), binds it to 1Click's signed quote, enforces spend policies (budgets, allowlists, human approval above a threshold), blocks deposits near the quote deadline, and runs a server-enforced lifecycle with an explicit "recovery required" state instead of guessing. Every payment exports an evidence packet that labels each claim as chain-verified or provider-reported, including honest handling of 1Click's new private-shard settlement.
> Agents connect through our MCP server in one config block. Chaos mode replays 12 deterministic failure scenarios (duplicate requests, stale events, timeouts, over-budget attempts) so developers can see each one caught.
> Live: [route, number of real mainnet payments, links to explorer txs, filled in from real data only].
> Stack: Go + PostgreSQL (Neon), Next.js console, TypeScript MCP server, OpenAPI + Bruno, CI with 12 scenario tests.
