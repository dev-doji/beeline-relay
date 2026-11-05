# Agent vault (NEAR contract)

A sandboxed wallet for AI agents. The vault holds USDC for an owner. Agents can spend it **only** by presenting a quote signed by NEAR Intents 1Click, and only within limits enforced on-chain.

## Why it exists

An AI agent with a normal wallet key can be prompt-injected into paying anyone. An agent using the vault cannot:

| Attack | What stops it |
|---|---|
| Agent is told to pay an attacker's address | The final recipient must be on the owner's **allowlist** |
| Agent is fed a fake "deposit address" | The vault verifies **1Click's ed25519 signature** on the quote. A forged or edited quote fails. |
| Quote edited after signing (address, amount, recipient) | Same signature check |
| Refund redirected to the attacker | `refundTo` must be **the vault itself** |
| Agent drains funds over many payments | **Per-payment limit** and **per-agent daily budget** |
| Same quote replayed to pay twice | Each **deposit address is single-use** |
| Old or about-to-expire quote | **Quote age** and **deadline margin** checks |
| Agent key stolen | It is a **function-call key** that can only call `pay_quote`, and the owner can revoke it. It cannot transfer NEAR, deploy code or call admin methods. |
| Beeline's own server compromised | The server never holds the vault's keys, and every check above is on-chain |
| Agent tricked into a payment that *passes* the rules (allowed recipient, wrong reason) | Above a threshold, an **AI reviewer on NEAR AI Cloud** must approve the exact quote, and the vault verifies the model's signature on-chain |
| One swarm agent overspends a shared job budget, or an outsider agent spends it | A **job escrow** gives each member agent its own cap, and only members can spend it |
| The swarm operator (or a bug) spends a hirer's escrowed money elsewhere | Job money can leave only as a policy-checked payment for that job, or as a refund to the **funder**. `withdraw` and ordinary payments can't touch it. |

Every rejection is a failed transaction with a `BEELINE_VAULT_REJECTED: <reason>` message, visible on the explorer.

## How the 1Click signature is checked

1Click signs `base58(sha256(stableStringify({...signedRequest, ...signedQuote, timestamp})))` with ed25519. The field list and the public key (`ed25519:reYaWhvwu8Jzo3WUM3zhn6VrhuMEF4eADL17qtRVifc`) come from the official SDK, `defuse-protocol/one-click-sdk-typescript/src/quote-signature.ts`. `src/quote.rs` reproduces it, and a unit test verifies a **real** 1Click quote with the real key. The Go API has the same implementation, tested against the same quote.

## How the AI reviewer is checked

Allowlists can't catch a payment to an *allowed* recipient for the *wrong reason*, for example an injected "urgent prepayment". For payments at or above `threshold`, the agent must attach a review from a reviewer model on [NEAR AI Cloud](https://docs.near.ai/cloud/verification), whose models run in TEEs and sign every completion. NEAR AI signs `{model}:{sha256(request)}:{sha256(response)}` with an ed25519 key (`provider_tee` signatures, `signing_algo=ed25519`). The agent submits the exact request and response bytes and the signature, and `src/review.rs` checks that:

1. the signature is from the reviewer key the owner registered;
2. the request used the registered model and the owner's **exact policy prompt** (compared by SHA-256), so a compromised agent can't swap in "approve everything";
3. the reviewer was shown **the exact signed quote** being paid;
4. the answer is fresh and its last line is `APPROVE <quote hash>`. A `REJECT ...` answer is reported in the failed transaction.

**Limitation:** the vault doesn't verify NEAR AI's TEE attestation itself (Intel TDX and NVIDIA evidence). Before calling `set_reviewer`, the owner verifies the attestation report off-chain and checks that its ed25519 `signing_address` equals the key being registered. Instances of the same model share one signing key. The unit tests use a test key and NEAR AI's documented format, and `src/testdata/go_review.json` (a review built by the Go client in `beeline-api/internal/nearai`) must pass the same check. A live NEAR AI signature hasn't been checked yet.

## Swarm job escrow

A hirer (the job's **funder**) escrows a budget for one job, and a swarm of agents is paid from it:

1. The owner calls `create_job(job_id, funder, members, recipients, expires_at_secs)`. Each member is an active agent key with its own `cap` for this job. `recipients`, if given, must already be on the vault allowlist, so a job can only **narrow** where money goes, never widen it.
2. The funder calls `ft_transfer_call` on the token to the vault with `msg = {"fund_job":"<job_id>"}`. Deposits from anyone else, or to a closed, expired or unknown job, are rejected and the token contract returns them.
3. Member agents call `pay_quote(quote_json, review, job_id)`. Every vault check still applies (1Click signature, allowlist, limits, daily budget, freshness, single-use deposit, AI reviewer), plus: the agent is a member, it stays within its job cap, the job has enough escrow, the job is open and not expired, and the recipient is allowed for the job.
4. `close_job(job_id)` refunds what's left **to the funder only**. The owner or the funder can close an open job, and anyone can close an expired one. If a payment in progress fails after closing, or the refund transfer fails, the money returns to the job and calling `close_job` again refunds it.

**Accounting.** Each token the vault tracks is in one bucket: the owner's `pool`, a job's balance (`escrowed` in total), or `in_flight` (sent and waiting for the transfer result). An `ft_transfer_call` with an empty `msg` funds the pool. Tokens that arrive any other way (1Click refunds, plain `ft_transfer`s, testnet mints) stay untracked until the owner calls `sync_balance`. It reads the token balance and credits the surplus to the pool. It never over-credits, even when a transfer completes while it is running. Ordinary payments and `withdraw` use only the pool.

There's no marketplace or bidding. Who hires whom, and what the work is, happens off-chain. The vault enforces only where the money can go.

## Interface

| Method | Who | Purpose |
|---|---|---|
| `new(owner, token, quote_signer, max_per_payment, daily_limit_per_agent, allowed_recipients, allowed_destination_assets)` | deployer | Initialize |
| `pay_quote(quote_json, review?, job_id?)` | agent key | Verify and pay a 1Click quote's deposit address. `review` = `{request, response, signature}` is needed at or above the reviewer threshold. With `job_id`, the payment comes from that job's escrow; otherwise from the pool. |
| `ft_on_transfer(sender_id, amount, msg)` | token contract | Fund the pool (`msg` empty) or a job (`{"fund_job":"<id>"}`, funder only) |
| `create_job(job_id, funder, members[{public_key, cap}], recipients, expires_at_secs)` | owner | Open a swarm job |
| `close_job(job_id)` | owner, funder, or anyone after expiry | Close and refund the rest to the funder |
| `sync_balance()` | owner | Credit untracked tokens (refunds, mints) to the pool |
| `add_agent(public_key, label)` / `revoke_agent(public_key)` | owner | Issue or delete an agent's function-call key |
| `set_reviewer(model, signing_key, policy_prompt_sha256, threshold)` / `clear_reviewer()` | owner | Require an AI reviewer approval above a threshold (hex key and hash) |
| `set_limits`, `set_freshness`, `add_recipient`, `remove_recipient`, `add_destination_asset`, `set_paused` | owner | Administration |
| `withdraw(amount, receiver_id)` | owner | Withdraw from the pool only |
| `get_config`, `get_reviewer`, `get_recipients`, `get_agents`, `is_deposit_used`, `get_balances`, `get_job`, `get_jobs` | anyone | Views |

Events use NEP-297 (`EVENT_JSON:`, standard `beeline-agent-vault`): `payment_requested`, `payment_sent`, `payment_failed`, `review_approved`, `agent_added`, `agent_revoked`, `reviewer_set`, `reviewer_cleared`, `paused`, `pool_funded`, `pool_synced`, `balance_shortfall`, `job_created`, `job_funded`, `job_closed`, `job_refunded`, `job_refund_failed`. Payment events carry `job_id` when paid from a job.

## Build and test

```bash
cargo test                                   # 52 unit tests
cargo near build non-reproducible-wasm       # optimized wasm for deployment
```

On Windows, set `AWS_LC_SYS_PREBUILT_NASM=1` first: a test dependency needs NASM otherwise.

## Status

Unit-tested only. It has **not been deployed** to testnet or mainnet yet, and **has not been audited**. It is a hackathon prototype: hold only small amounts in it.
