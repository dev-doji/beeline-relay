# Beeline Relay: 14-Day NEAR Hackathon Plan (v2, aiming for the grand prize)

> **v3 update (4 Nov 2026), which overrides the sections below where they conflict:**
> - **Primary track is now AI Security & Defense.** The track asks for "spending limits, policy engines, sandboxed wallets… function call keys". Agent Economy and Real-World Money are secondary.
> - **New P0: an on-chain NEAR agent vault.** It is a smart contract holding the funds. The agent gets only a function-call key, and per-payment limits, daily budget and recipient allowlist are enforced on-chain. This gives two layers of defense: Beeline's API (reliability and policy) and the vault (hard limits even if the agent and the server are compromised).
> - **The demo hero scene is a prompt-injection attack** that both layers block, with an on-chain refusal shown in the explorer.
> - **Route:** USDC on NEAR (paid from the vault) → USDC on Base. If contract-to-1Click deposits on NEAR prove unworkable by Day 7, fall back to Base → NEAR.
> - **Schedule is compressed** because building started on 4 Nov:
>
> | Day | Work |
> |---|---|
> | 4 | Repo, layout, CI, Neon, health ✅ |
> | 5 | Quotes, payments, idempotency, state machine |
> | 6 | Vault contract, plus tests on NEAR testnet |
> | 7 | Vault paying 1Click on mainnet: **live run #1** |
> | 8 | Off-chain policy, signed-quote binding, reconcile, evidence |
> | 9 | MCP server, reference agent, prompt-injection scenario |
> | 10 | Console: timeline, evidence, approval queue |
> | 11 | Attack and chaos mode (simulated, clearly labelled) |
> | 12 | Deploy, **live run #2**, landing page |
> | 13 | README, demo video |
> | 14 | Submit by 12:00 UTC |

**Goal:** win the **$55K grand prize** and **1st in Agent Economy**. Fallback targets: placing in AI Security & Defense or Real-World Money.
**Build window:** 1 Nov 00:00 UTC → **14 Nov 23:59 UTC**. Our own deadline is **14 Nov 12:00 UTC**.
**Tracks to select:** Agent Economy (primary), AI Security & Defense, Real-World Money.
**Positioning:** see `POSITIONING.md`. One line: **"Payment guardrails for AI agents on NEAR Intents."**

---

## What wins a grand prize (and how we hit each point)

| What grand prizes tend to reward | How we deliver it |
|---|---|
| **Real, not mocked.** A live mainnet payment on video. | Live run #1 on Day 6, #2 on the deployed stack on Day 12, and #3 as the hero shot of the demo (an agent paying autonomously). |
| **A story that fits the moment.** Right now that's agents plus intents plus privacy. | Agents move money through NEAR Intents, and Beeline stops them double-paying, paying into expired deposits, or overspending. The evidence packet works even though 1Click now settles on a private shard. |
| **A demo moment people remember** | **"Chaos mode"**: an agent pays, then the network breaks in five ways live, and every failure is caught, explained and recorded. |
| **Polish and depth together** | Go core with 10+ deterministic fault tests, a typed API, OpenAPI and Bruno, and a console that is a pleasure to look at. |
| **Someone else can use it today** | Public MCP server and API. `npx`/`go install` one-liner. Other hackathon teams' agents can pay through it. |
| **A believable business** | Revenue through 1Click's built-in `appFees` (bps on each routed payment, split 50/50). There's no billing system to build. |
| **The AI judges read the repo** | Green CI, `go test ./...` covers all scenarios, a README that separates **live / simulated / not yet verified**, and no unbacked claims. |

## What's new in v2 (compared with the original spec)

1. **MCP server (P0).** `beeline-mcp` exposes the tools `get_quote`, `create_payment`, `submit_deposit`, `reconcile_payment`, `get_payment`, `get_evidence` and `check_policy`. It's a thin wrapper over the REST API. Idempotency keys are derived from the agent's task ID, so retries are safe by construction.
2. **Spend policies (P0).** These are the agent guardrails, enforced server-side when a payment is created:
   - limit per payment, plus a daily budget per agent key;
   - recipient allowlist;
   - asset and route allowlist;
   - **human approval above a threshold**: the payment goes to `awaiting_approval` and a human approves it in the console;
   - every check is recorded as an event (`policy_passed` / `policy_blocked`) and appears in the evidence packet.
3. **Agent identity.** Each agent gets its own API key (scoped and revocable), and every payment and event records which agent did it.
4. **Reference agent (P0, for the demo).** A small agent runtime with a model, an invoice task, and the MCP client.
   - **Signing stays outside Beeline**, and Beeline never sees keys. Mode A: the agent proposes, a human signs in the console wallet. Mode B: the agent signs with its own operator-funded hot wallet held in the agent process, with a small balance and a spend policy.
   - Model: test **NEAR AI Cloud** on Day 1 (it fits the NEAR ecosystem and private AI). Fall back to the Claude API.
5. **Chaos mode (P0).** This replaces the plain scenario list. It's a sandbox where you pick a failure, run the agent against the provider test double, and watch the timeline catch it, with a plain-English reason on every event. It's clearly labelled SIMULATED and can't be mistaken for a real payment.
6. **Cut to make room:** the website shrinks to a single landing page (half a day). `GET /payments` list filters are dropped. P1 items stay frozen.

---

## Day-by-day

| Day | Date | Goal | Done when |
|---|---|---|---|
| 1 | Sat 1 Nov | Public repo. Docs and research brought in. Monorepo: `beeline-api/` (Go, chi, slog, config, graceful shutdown), `beeline-web/` (pnpm: `apps/website`, `apps/app`), `beeline-mcp/`. Neon project and migrations. `/health`. CI (go test, vet, lint, web build, gitleaks). **Spike (1 hr):** NEAR AI Cloud API access. | CI green; health on Neon; agent model decided |
| 2 | Sun 2 Nov | Catalogue limited to the route allowlist. Strict base-unit amount parsing. 1Click client (tokens, quote) with fixtures. `POST /quotes`: signed quote stored, fingerprint, expiry. Agent API keys (hashed) and workspace auth. | Tests for amounts, fingerprint, expiry and auth |
| 3 | Mon 3 Nov | `POST /payments` with `Idempotency-Key` (replay or conflict). **Spend-policy engine**, including `awaiting_approval`. State-machine service and transition matrix. Append-only events. | Idempotency, policy and transition tests pass |
| 4 | Tue 4 Nov | `/submit` (txHash, forwarded to `/v0/deposit/submit`). `/reconcile` (status mapping). `/approve` (human approval, console only). Provider test double. Deadline guard. | Duplicate, stale, out-of-order and deadline tests pass |
| 5 | Wed 5 Nov | Evidence packet v1.0 (provenance, limitations, signed quote, policy decisions, agent ID). Independent Base receipt check. Redaction. **All 12 scenarios** as Go tests (the 10 MVP ones plus `policy_blocked` and `approval_required`). | `go test ./...` covers every scenario |
| 6 | Thu 6 Nov | **LIVE RUN #1:** 1–2 USDC Base→NEAR through the API using curl/Bruno and a wallet. Record the real status sequence and fixtures, and close the open questions. OpenAPI and Bruno in sync. | Real payment `completed` with evidence. Only now is the route "verified". |
| 7 | Fri 7 Nov | **MCP server** (`beeline-mcp`, TypeScript MCP SDK, calls the REST API). **Reference agent** with the invoice task. Mode B signer (local hot wallet, never sent to the API). | An agent completes a real payment from a prompt |
| 8 | Sat 8 Nov | Console foundation: design tokens, layout, typed client (Zod), server-side proxy. Payments list scoped to the workspace. Session detail and **event timeline with provenance badges**. | Real payments visible with their full timeline |
| 9 | Sun 9 Nov | Console: quote review, wallet send (wagmi, Base, deadline-locked), **approval queue** for Mode A, reconcile, evidence viewer and download, every error state. | Human-approval flow works end to end |
| 10 | Mon 10 Nov | **Chaos mode:** pick a failure → the agent runs against the test double → timeline animates with the reason on each event. Agent spend dashboard (budget used, blocked attempts; real data only). | Demo-quality; labelled SIMULATED |
| 11 | Tue 11 Nov | Landing page (half a day). **Quickstart:** "give your agent safe payments in 3 minutes" (MCP config snippet). Publish the MCP package. **Outreach:** post in the NEAR hackathon channels offering the MCP server to other teams. | A stranger can connect an agent in under 5 minutes |
| 12 | Wed 12 Nov | Deploy: API (Fly, Railway or Render), web (Vercel), Neon prod branch, CORS and auth. **LIVE RUN #2** on the deployed stack, agent-driven. | Public URLs; second real payment evidenced |
| 13 | Thu 13 Nov | README (live / simulated / unverified table). Architecture diagram. **Record the demo video.** Write the X thread and the form description (≤2,000 chars). | Video and copy final |
| 14 | Fri 14 Nov | Buffer. Fix only what's broken. **Submit by 12:00 UTC.** Post the X thread and tag @NEARProtocol and the hackathon account. | Submitted ✅ |

**Checkpoints (be honest at each):**
- **End of Day 6:** if the live route isn't working, everything stops until it is.
- **End of Day 9:** if the console is behind, chaos mode becomes a simpler list, and the agent demo and live payment still ship.
- **Day 12:** feature freeze.

## Stretch goals (only after the Day 10 checkpoint)
1. Verify quote signatures in Go (needs the 1Click public key).
2. Verify the destination tx independently on NEAR RPC, so delivery shows as `chain_verified`.
3. Policy for **agent-to-agent payments**, where an agent pays another agent's service under its budget.
4. Reverse route NEAR → Base via the NEAR wallet selector.

## Demo video (2:30, scripted, recorded on Day 13)
1. **0:00 Hook.** Black screen with the text: "Your AI agent just paid an invoice. The request timed out. Did the money move? Will it pay again?"
2. **0:15 Agent pays for real.** A prompt asks the agent to pay invoice #42 of 2 USDC to an address. The agent calls `get_quote` then `create_payment`, and its wallet signs. The timeline runs live through submitted, processing and completed in about 35 seconds. The evidence packet shows **chain-verified deposit · provider-reported swap (private shard) · delivery**.
3. **1:00 Chaos mode.** A rapid montage, each failure caught with its reason on screen:
   - the agent retries and gets **the same payment, not a second one**;
   - a key is reused with a different amount and is **rejected**;
   - a stale "failed" event arrives after completion and **can't undo it**;
   - a timeout leads to **recovery required**, and the agent is told to stop instead of paying again;
   - an over-budget payment is **blocked by policy**, and an amount over the threshold **waits for human approval**.
4. **1:50 Why it matters.** "Agents will move money on their own. Beeline makes every payment explainable, auditable, and safe to retry, or explicitly unsafe to retry."
5. **2:10 Proof.** Repo, green CI, 12 fault tests, MCP one-liner. A table of what's live and what's simulated.
6. **2:25 Close.** Logo, URL, "Built at NEAR Hackathon".

## Risks and mitigations
| Risk | Mitigation |
|---|---|
| Too much scope (API, MCP, agent, console, website) | MCP and agent are thin layers over the API. The website is half a day. Checkpoints on Day 6, 9 and 12. |
| Partner key not approved | Unauthenticated requests work today. Accept the possible fee on a few dollars. |
| Provider differs from docs (confidential-only since 27 Oct) | Live run on Day 6 leaves 8 days to adapt. Real fixtures recorded. |
| NEAR AI Cloud unavailable or flaky | Fall back to the Claude API. The runtime keeps the model swappable. |
| Hot-wallet risk in Mode B | Lives only in the agent process. Dedicated wallet, balance ≤ $10, enforced by spend policy. Documented. |
| Live payment stuck on recording day | Record early (Day 6, 7, 12). A stuck payment is itself a real `recovery_required` demo. |
| Secrets in a public repo | Placeholder-only `.env.example`. gitleaks in CI from Day 1. |
| Over-claiming hurts with the AI judges | README table: live / simulated / not yet verified. Every website claim links to code or tests. |

## Your pre-hackathon checklist (before 1 Nov)
- [ ] Apply for a partner API key at https://partners.near-intents.org
- [ ] **Two** dedicated Base wallets: one for you/console (~10 USDC + $2 ETH) and one for the agent hot wallet (~5 USDC + $1 ETH)
- [ ] NEAR account to receive, plus a NEAR wallet address for the prize
- [ ] Check whether you can get a NEAR AI Cloud account or API key (I'll spike it on Day 1)
- [ ] GitHub (public repo on 1 Nov), X account, Vercel and Fly/Railway/Render accounts
- [ ] Join the hackathon Telegram/Discord. Day 11 outreach happens there.
