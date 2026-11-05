# Handoff: current state (5 Nov 2026)

For any coding agent continuing this project (Antigravity, Codex, Claude Code). Read `AGENTS.md` first, then `docs/HACKATHON_PLAN.md`. The **v4 block** at the top of the plan is the current direction and overrides older sections.

## The project in one line

**Beeline: the guardrails that make an agent swarm safe to pay.** It's for the NEAR Hackathon (build window ends **14 Nov 2026 23:59 UTC**, our own deadline is 14 Nov 12:00 UTC). The primary track is AI Security & Defense. It answers Illia Polosukhin's "Swarm-for-Hire (with guardrails or governance AI review, please)".

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

1. **Testnet rehearsal.** The owner's `goni.testnet` has 0 NEAR and the web-wallet import didn't add the CLI key. Next attempt: create a faucet-funded account with `near account create-account sponsor-by-faucet-service <name>.testnet autogenerate-new-keypair save-to-keychain network-config testnet create`, then run `.\scripts\testnet-rehearsal.ps1 -Owner <name>.testnet -SkipImport`. Debug from its output.
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
