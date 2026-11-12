# Handoff: current state (7 Nov 2026)

## Latest (7 Nov 2026): read this first

- **Hosted:** website https://usebeeline.xyz (Vercel project devopsnormads-projects/beeline, root `beeline-web`); API https://beeline-api-z3tu.onrender.com (Render free plan, `render.yaml`, sleeps after ~15 min idle); database Neon. Secrets live only in the Render dashboard; `scripts/hosting-env.ps1` prints them locally. See `docs/DEPLOY.md`.
- **Product direction (owner, 7 Nov):** no canned "simulate" buttons. The core demo is **dare-a-friend pay links**: a vendor creates a signed invoice (`/vendor`), sends the link to anyone, and visitors attack it on `/pay/[id]` with free text and/or an edited payee or amount. Bee (NEAR AI) decides, and the vault decides on-chain. Every attempt goes to the public attack log (`/attacks`, `GET /api/v1/playground/attempts`, migration 0004) with computed totals, including "paid to the wrong account".
- **SDK:** `packages/beeline-sdk` (`invoices.create/get/pay/attempts`, `attackLog`, `verify` via NEAR RPC). `pnpm test` has 5 unit tests; `pnpm example` runs live on testnet (verified against the hosted API on 7 Nov). **Not published to npm**: that needs the owner's npm account (`beeline-sdk` was free on 7 Nov).
- **Switch (onswitch.xyz):** the owner has an API key and was asked to add `SWITCH_API_KEY` and `SWITCH_MODE` (sandbox or live) to `beeline-api/.env` (never in chat). Switch onramps local currency (21 African countries) to USDC on EVM chains, Solana and Tron, **not NEAR**. Plan: onramp to a NEAR Intents (1Click) deposit address that delivers into the vault. A sandbox key only simulates settlement; label it as such. Not started.
- **NEAR Intents mainnet payout:** still one small live payment away (USDC on 16 chains per the 1Click token list on 7 Nov).


For any coding agent continuing this project (Antigravity, Codex, Claude Code). Read `AGENTS.md` first, then `docs/HACKATHON_PLAN.md`. The **v4 block** at the top of the plan is the current direction and overrides older sections.

## The project in one line

**Beeline: the guardrails that make an agent swarm safe to pay.** It's for the NEAR Hackathon (build window ends **14 Nov 2026 23:59 UTC**, our own deadline is 14 Nov 12:00 UTC). **Re-framed on 2026-11-12:** AI Security & Defense has the most submissions (330 of 695), so we lead with Real-World Money (159) and the original vendor-invoice idea (the owner's earlier product, Rhodium), and enter Agent Economy and AI Security too. Same code; the landing page, README and demo script were rewritten around vendor pay links paid through NEAR Intents. It answers Illia Polosukhin's "Swarm-for-Hire (with guardrails or governance AI review, please)".

The owner is non-technical. They make product decisions, and the agent makes the technical ones.

## Done (all on `main`, CI green)

| Piece | Where | State |
|---|---|---|
| Go API: live 1Click quotes, idempotent payment sessions, state machine, events, API keys, workspace isolation | `beeline-api/` | Unit and integration tested (Neon Postgres). A live 1Click quote was verified. |
| 1Click quote signature verification (Go and Rust) | `beeline-api/internal/intents/signature.go`, `contracts/agent-vault/src/quote.rs` | Verified against a **real** 1Click quote |
| NEAR agent vault: function-call agent keys, allowlist, limits, single-use deposits, refund-to-vault | `contracts/agent-vault/` | 51 unit tests (with escrow). **Not deployed yet.** |
| AI-reviewer co-signature: above a threshold, a NEAR AI Cloud model must approve the exact quote, verified on-chain | `contracts/agent-vault/src/review.rs` | Unit-tested with NEAR AI's documented ed25519 `provider_tee` format. **Not yet checked against a live NEAR AI signature.** |
| Swarm job escrow: a hirer funds a job with `ft_transfer_call`, member agents spend it under per-job caps and a job recipient list (narrows the allowlist), and `close_job` refunds only the funder. Pool/escrow/in-flight accounting; `withdraw` is pool-only; `sync_balance` credits untracked tokens. | `contracts/agent-vault/src/jobs.rs` | Unit-tested (18 tests). **Not deployed.** The rehearsal now calls `sync_balance` after minting. |
| Testnet rehearsal (deploy, 1 allowed payment, 4 on-chain attacks) | `scripts/testnet-rehearsal.ps1`, `docs/TESTNET_REHEARSAL.md` | **Not run successfully yet** (see below) |

## In progress / next

- **Vendor pay links (2026-11-05):** built in the contract (`invoice.rs`), the API (`internal/invoices`, migration 0003) and the web app (`/vendor`, `/pay/[id]`). Invoice creation was verified locally. Next: the owner runs `scripts/vendor-setup.ps1 -Owner goni-beeline.testnet` (vault upgrade plus `set_invoice_issuer`); then live-test the genuine payment, swap_payout, edit_amount and pay-twice, and record the transaction hashes in the README.

- **Playground (2026-11-05):** backend done (`internal/near`, `internal/playground`, `/api/v1/playground*`). `beeline-web` has a landing page and `/playground`, verified locally end to end on testnet (paid, replay blocked, attacker blocked). The owner ran `scripts/playground-setup.ps1` on 2026-11-05: contract upgraded, 3 NEAR agent allowance, limits 5/1000 USDC, pool ~1018 test USDC, NEAR AI reviewer `z-ai/glm-5.3-flash` registered with threshold 1. The playground UI is now a chat with rich cards (`components/chat`). Live proof: injection refused on-chain Dq4sPfqfoFMgAvhn4kxmG2MQgbgBSSgEMi2nunxkopsi; approved payment 5mJkFbgeLLFGf7Cww5vJPCiRtpwzM4Qkyi6pYXwZuB3z. Next: deploy the web app (Vercel) and the API (for example Render) with the PLAYGROUND_* env vars.

0. ✅ **INTENTS rail implemented on 2026-11-05** (vault `pay_quote` with `INTENTS` quotes, `withdraw_from_intents`, `set_intents_contract`; API route rails per route). Remaining: one small live mainnet payment to confirm 1Click picks up the on-behalf-of deposit. Original note: **Route change found on 2026-11-05 (see docs/NEAR_INTENTS_ROUTE.md section 6b).** 1Click currently refuses every quote that deposits from the NEAR chain (`ORIGIN_CHAIN`), but quotes NEAR USDC with `depositType: INTENTS` → Base. The vault pays only `ORIGIN_CHAIN` quotes today. Next: add an `INTENTS` pay path (`ft_transfer_call` to `intents.near` with `msg` = deposit address, refunds to the vault's intents balance), keep the `ORIGIN_CHAIN` path, and confirm with one small live payment. The partner JWT in `.env` now works (not 401).

1. ✅ **Testnet rehearsal PASSED on 2026-11-05** (`vault.goni-beeline.testnet`, owner `goni-beeline.testnet`; transaction hashes are in docs/TESTNET_REHEARSAL.md). Mainnet is optional: the hackathon doesn't require it and the owner has no funds. Next: the Playground web app (app.beeline) on this testnet vault. Old note: The owner's `goni.testnet` has 0 NEAR and the web-wallet import didn't add the CLI key. Next attempt: create a faucet-funded account with `near account create-account sponsor-by-faucet-service <name>.testnet autogenerate-new-keypair save-to-keychain network-config testnet create`, then run `.\scripts\testnet-rehearsal.ps1 -Owner <name>.testnet -SkipImport`. Debug from its output.
2. ✅ **Live NEAR AI signature check: DONE on 2026-11-05.** Use model `z-ai/glm-5.3-flash` (TEE, provider_tee ed25519, about 20s per review; key `5a0210f2442c79355784f9de021985429561da5bacc3a4424b01ac52b7e0caff`). Qwen/Qwen3.8-27B is also provider_tee. DeepSeek and the closed models give only gateway signatures, which the vault rejects. Live fixtures are in `contracts/agent-vault/src/testdata/live_review_*.json`. The attestation binding (TDX report_data = key || nonce) is checked by review-check; the DCAP/NRAS chains are not. The NEAR Intents key in `.env` is rejected by 1Click (401): it is 47 characters, but a partner JWT starts with `eyJ`. Quotes work without it (+0.25% fee). Older notes: The Go client (`beeline-api/internal/nearai`) and `go run ./cmd/review-check` are built. A shared fixture proves the Rust vault accepts a Go-built review byte for byte. The API key is in `beeline-api/.env`, but the live call returns **HTTP 402 "No spending limit configured"**: the owner must add credits at cloud.near.ai. Then run review-check, confirm `signature_kind` is `provider_tee` (try a TEE-hosted open model if not), and verify the attestation report. Original brief: build the Go client that calls `POST https://cloud-api.near.ai/v1/chat/completions` (non-streaming), keeps the exact request and response bytes, fetches `GET /v1/signature/{chat_id}?model=…&signing_algo=ed25519`, and checks it the way `review.rs` does. Also verify the attestation (`GET /v1/attestation/report`) off-chain. Docs: https://docs.near.ai/cloud/verification
3. ~~Swarm job escrow~~ done (see table). Still to do: add a job scene to the testnet rehearsal (create job, fund it with `ft_transfer_call`, a member pays, a non-member is rejected, close and refund).
4. **MCP server and reference swarm agents**, with prompt-injection demo scenes.
5. Off-chain policy, reconcile and evidence. Console. Deploy. **Mainnet live run** (the owner signs and funds: about 5 NEAR plus 15 USDC; mainnet wallet `goni19.near`, also the prize wallet). README and video. Submit.

## Security notes (must read)

- **A NEAR AI API key was accidentally committed** to `.env.example` in commit `bf24609` (removed in `5eb0be0`; it's still in git history). The owner was told to **revoke it** at cloud.near.ai. Whether to rewrite git history is still undecided.
- **Never `git add -A` / `git add .`** here. Stage explicit paths and check `git diff --cached` for keys before each commit. Secrets go only in `beeline-api/.env`.
- Never handle private keys or seed phrases, and never sign or fund on the owner's behalf. Prepare exact commands for the owner to run.

## Environment gotchas (Windows dev machine)

- Rust: set `AWS_LC_SYS_PREBUILT_NASM=1` before `cargo test`. `cargo-near` won't install on Windows, so the optimized wasm comes from the CI artifact `agent-vault-wasm` (`gh run download --name agent-vault-wasm`).
- Go: no cgo locally, so `-race` runs only in CI. Integration tests need `TEST_DATABASE_URL` (Neon branch `test`, in `beeline-api/.env`).
- PowerShell 5.1 mangles JSON passed to native programs. Use `file-args` with the near CLI. Use `gh secret set --body` (piping corrupts values).
- The dev PC clock is about 27s off. The API uses its own clock in UTC.
- 1Click has no testnet. Its `refundTo` must be an existing NEAR account.
