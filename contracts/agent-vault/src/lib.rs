//! # Beeline Relay agent vault
//!
//! A NEAR contract that holds a fungible token (USDC) on behalf of an owner and lets AI agents
//! spend it only through NEAR Intents 1Click quotes, under limits enforced on-chain.
//!
//! * Agents never hold funds. Each agent gets a **function-call access key on the vault
//!   account** that can call only `pay_quote`. The key cannot transfer NEAR, deploy code or
//!   call any other method, and the owner can revoke it at any time.
//! * `pay_quote` takes a raw 1Click quote and pays its deposit address only if:
//!   1. the quote carries a valid **1Click signature** (so the deposit address is genuine),
//!   2. the final **recipient is on the owner's allowlist**,
//!   3. refunds go back to **this vault**, in the vault's own token,
//!   4. the amount is within the **per-payment limit** and the agent's **daily budget**,
//!   5. the quote is **fresh** and its deadline is not close,
//!   6. the deposit address has **never been paid** before,
//!   7. at or above the owner's threshold, a **NEAR AI reviewer model approved this exact
//!      quote** under the owner's policy prompt, proven by the model's TEE signing key
//!      (see `review.rs`),
//!   8. the vault holds enough **of the right funds**: the owner's pool for ordinary payments,
//!      or, for a swarm job, that job's escrow within the agent's job cap (see `jobs.rs`).
//!
//! A prompt-injected agent, or a compromised Beeline server, cannot get past these checks: it
//! cannot forge 1Click's signature or change the allowlist, and every rejection is a failed
//! transaction anyone can see on the explorer.

mod direct;
mod invoice;
mod jobs;
mod quote;
mod review;

use near_sdk::json_types::U128;
use near_sdk::store::{IterableMap, IterableSet, LookupSet};
use near_sdk::{
    env, near, require, AccountId, Allowance, BorshStorageKey, Gas, NearToken, PanicOnDefault,
    Promise, PromiseResult, PublicKey,
};

pub use invoice::Invoice;
pub use jobs::{Balances, Job, JobMember, JobMemberInput, JobStatus};
pub use quote::parse_rfc3339_utc;
pub use review::Review;
use review::Reviewer;

pub(crate) const NS_PER_DAY: u64 = 86_400 * 1_000_000_000;
pub(crate) const NS_PER_SEC: u64 = 1_000_000_000;
/// NEP-145 registration for the deposit account on the token contract; any excess is refunded.
const STORAGE_DEPOSIT: NearToken = NearToken::from_yoctonear(1_250_000_000_000_000_000_000);
const GAS_STORAGE_DEPOSIT: Gas = Gas::from_tgas(10);
pub(crate) const GAS_FT_TRANSFER: Gas = Gas::from_tgas(15);
pub(crate) const GAS_CALLBACK: Gas = Gas::from_tgas(10);
/// ft_transfer_call into the NEAR Intents verifier (covers its ft_on_transfer and the resolve).
const GAS_FT_TRANSFER_CALL: Gas = Gas::from_tgas(60);
const GAS_INTENTS_WITHDRAW: Gas = Gas::from_tgas(60);
/// The NEAR Intents verifier contract on mainnet.
const DEFAULT_INTENTS_CONTRACT: &str = "intents.near";
/// Gas-fee allowance granted to each agent key (it pays for the agent's transactions only).
const AGENT_KEY_ALLOWANCE: NearToken = NearToken::from_millinear(250);

#[near(serializers = [borsh])]
#[derive(BorshStorageKey)]
enum StorageKey {
    Recipients,
    DestinationAssets,
    Agents,
    UsedDeposits,
    Jobs,
}

#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq)]
pub struct Agent {
    pub label: String,
    pub active: bool,
    /// Day index (Unix days) that `spent_today` refers to.
    pub day: u64,
    pub spent_today: U128,
    pub total_spent: U128,
    pub payments: u64,
}

#[near(serializers = [json])]
pub struct Config {
    pub owner: AccountId,
    pub token: AccountId,
    pub quote_signer: String,
    pub max_per_payment: U128,
    pub daily_limit_per_agent: U128,
    pub max_quote_age_secs: u64,
    pub min_deadline_margin_secs: u64,
    pub paused: bool,
    pub intents_contract: AccountId,
}

#[near(contract_state)]
#[derive(PanicOnDefault)]
pub struct AgentVault {
    owner: AccountId,
    token: AccountId,
    quote_signer: [u8; 32],
    max_per_payment: u128,
    daily_limit_per_agent: u128,
    max_quote_age_secs: u64,
    min_deadline_margin_secs: u64,
    paused: bool,
    recipients: IterableSet<String>,
    destination_assets: IterableSet<String>,
    agents: IterableMap<PublicKey, Agent>,
    used_deposits: LookupSet<String>,
    reviewer: Option<Reviewer>,
    /// The owner's general funds (see `jobs.rs` for the accounting).
    pool: u128,
    /// Sum of all job balances.
    escrowed: u128,
    /// Sent and awaiting the transfer callback.
    in_flight: u128,
    /// Total of all outgoing transfers that completed; used by `sync_balance`.
    completed_out: u128,
    jobs: IterableMap<String, Job>,
    /// NEAR Intents verifier used for `INTENTS`-deposit quotes.
    intents_contract: AccountId,
}

/// The registered AI reviewer, as shown by `get_reviewer`.
#[near(serializers = [json])]
pub struct ReviewerView {
    pub model: String,
    /// Hex ed25519 public key (NEAR AI's `signing_address` for ed25519).
    pub signing_key: String,
    pub policy_prompt_sha256: String,
    pub threshold: U128,
}

pub(crate) fn reject(reason: &str) -> ! {
    env::panic_str(&format!("BEELINE_VAULT_REJECTED: {reason}"))
}

/// True if the single promise this callback waited on succeeded. The result body is not read,
/// so its size cannot exhaust gas.
pub(crate) fn transfer_succeeded() -> bool {
    #[allow(deprecated)]
    let ok = env::promise_results_count() == 1
        && matches!(env::promise_result(0), PromiseResult::Successful(_));
    ok
}

/// The amount an `ft_transfer_call` reports as used (0 if it failed or returned nothing usable).
/// The result comes from this vault's own token contract.
fn intents_deposit_used() -> u128 {
    #[allow(deprecated)]
    if env::promise_results_count() != 1 {
        return 0;
    }
    #[allow(deprecated)]
    match env::promise_result(0) {
        PromiseResult::Successful(bytes) => near_sdk::serde_json::from_slice::<U128>(&bytes)
            .map(|v| v.0)
            .unwrap_or(0),
        _ => 0,
    }
}

/// Emits a NEP-297 event log.
pub(crate) fn emit(event: &str, data: near_sdk::serde_json::Value) {
    env::log_str(&format!(
        "EVENT_JSON:{}",
        near_sdk::serde_json::json!({
            "standard": "beeline-agent-vault",
            "version": "1.0.0",
            "event": event,
            "data": [data],
        })
    ));
}

#[near]
impl AgentVault {
    #[init]
    #[allow(clippy::too_many_arguments)]
    pub fn new(
        owner: AccountId,
        token: AccountId,
        quote_signer: String,
        max_per_payment: U128,
        daily_limit_per_agent: U128,
        allowed_recipients: Vec<String>,
        allowed_destination_assets: Vec<String>,
    ) -> Self {
        let signer = quote::decode_ed25519::<32>(&quote_signer).unwrap_or_else(|e| reject(&e));
        require!(
            max_per_payment.0 > 0 && daily_limit_per_agent.0 >= max_per_payment.0,
            "invalid limits"
        );
        let mut vault = Self {
            owner,
            token,
            quote_signer: signer,
            max_per_payment: max_per_payment.0,
            daily_limit_per_agent: daily_limit_per_agent.0,
            max_quote_age_secs: 15 * 60,
            min_deadline_margin_secs: 120,
            paused: false,
            recipients: IterableSet::new(StorageKey::Recipients),
            destination_assets: IterableSet::new(StorageKey::DestinationAssets),
            agents: IterableMap::new(StorageKey::Agents),
            used_deposits: LookupSet::new(StorageKey::UsedDeposits),
            reviewer: None,
            pool: 0,
            escrowed: 0,
            in_flight: 0,
            completed_out: 0,
            jobs: IterableMap::new(StorageKey::Jobs),
            intents_contract: DEFAULT_INTENTS_CONTRACT.parse().unwrap(),
        };
        for r in allowed_recipients {
            vault.recipients.insert(r);
        }
        for a in allowed_destination_assets {
            vault.destination_assets.insert(a);
        }
        vault
    }

    // ----- Agent entry point -------------------------------------------------------------

    /// Pays the deposit address of a 1Click-signed quote, if every policy check passes.
    /// Callable only with an active agent access key on this account. `review` is required
    /// when a reviewer is registered and the amount is at or above its threshold. With
    /// `job_id`, the payment is drawn from that swarm job's escrow; otherwise from the pool.
    pub fn pay_quote(
        &mut self,
        quote_json: String,
        review: Option<Review>,
        job_id: Option<String>,
        invoice: Option<Invoice>,
    ) -> Promise {
        if self.paused {
            reject("vault is paused");
        }
        require!(
            env::predecessor_account_id() == env::current_account_id(),
            "BEELINE_VAULT_REJECTED: pay_quote must be signed with an agent key of this vault"
        );
        let agent_pk = env::signer_account_pk();
        let mut agent = match self.agents.get(&agent_pk) {
            Some(a) if a.active => a.clone(),
            _ => reject("signing key is not an active agent"),
        };

        let q = quote::parse(&quote_json).unwrap_or_else(|e| reject(&e));
        if !quote::verify(&q, &self.quote_signer) {
            reject("quote signature is not from 1Click");
        }

        // What the payment is, and where value can go back to.
        if q.dry {
            reject("dry-run quotes cannot be paid");
        }
        // Two ways to fund a quote: ORIGIN_CHAIN (send the token to the deposit account) or
        // INTENTS (deposit into the NEAR Intents verifier on behalf of the deposit address).
        // Refunds use the same rail and must return to this vault either way.
        let via_intents = match q.deposit_type.as_str() {
            "ORIGIN_CHAIN" => false,
            "INTENTS" => true,
            _ => reject("only ORIGIN_CHAIN or INTENTS deposits are supported"),
        };
        if q.swap_type != "EXACT_INPUT" {
            reject("only EXACT_INPUT quotes are supported");
        }
        if q.origin_asset != format!("nep141:{}", self.token) {
            reject("quote does not spend this vault's token");
        }
        if q.refund_to != env::current_account_id().as_str() || q.refund_type != q.deposit_type {
            reject("refunds must return to this vault");
        }
        if !self.destination_assets.contains(&q.destination_asset) {
            reject(&format!(
                "destination asset {} is not allowed",
                q.destination_asset
            ));
        }
        // Recipient: on the owner's allowlist, or the payee of a signed vendor invoice.
        let invoice_terms = match &invoice {
            None => {
                if q.recipient_type != "DESTINATION_CHAIN"
                    || !self.recipients.contains(&q.recipient)
                {
                    reject(&format!(
                        "recipient {} is not on the allowlist",
                        q.recipient
                    ));
                }
                None
            }
            Some(inv) => {
                let issuer =
                    invoice::issuer().unwrap_or_else(|| reject("this vault accepts no invoices"));
                let t = invoice::verify(inv, &issuer).unwrap_or_else(|e| reject(&e));
                if q.recipient_type != "DESTINATION_CHAIN" || q.recipient != t.payout {
                    reject(&format!(
                        "quote pays {} but invoice {} pays {}",
                        q.recipient, t.id, t.payout
                    ));
                }
                if q.amount != t.amount {
                    reject(&format!(
                        "quote amount {} does not match invoice {} amount {}",
                        q.amount, t.id, t.amount
                    ));
                }
                if q.destination_asset != t.asset {
                    reject("quote asset does not match the invoice");
                }
                if env::block_timestamp() / NS_PER_SEC > t.expires_at {
                    reject(&format!("invoice {} has expired", t.id));
                }
                if invoice::is_paid(&t.id) {
                    reject(&format!("invoice {} has already been paid", t.id));
                }
                Some(t)
            }
        };
        if q.deposit_address.is_empty() || q.has_deposit_memo {
            reject("quote has no usable deposit address");
        }
        if q.amount != q.amount_in {
            reject("quote amountIn does not match the requested amount");
        }
        let amount: u128 = q
            .amount
            .parse()
            .unwrap_or_else(|_| reject("amount is not an integer"));
        if amount == 0 || amount > self.max_per_payment {
            reject(&format!(
                "amount {amount} exceeds the per-payment limit {}",
                self.max_per_payment
            ));
        }

        // Freshness.
        let now = env::block_timestamp();
        let issued =
            parse_rfc3339_utc(&q.timestamp).unwrap_or_else(|| reject("bad quote timestamp"));
        let deadline =
            parse_rfc3339_utc(&q.deadline).unwrap_or_else(|| reject("bad quote deadline"));
        if now.saturating_sub(issued) > self.max_quote_age_secs * NS_PER_SEC {
            reject("quote is too old; request a fresh quote");
        }
        if deadline <= now + self.min_deadline_margin_secs * NS_PER_SEC {
            reject("quote deadline is too close; request a fresh quote");
        }

        // Single use.
        if self.used_deposits.contains(&q.deposit_address) {
            reject("this deposit address has already been paid");
        }

        // AI reviewer co-signature.
        let approval = match &self.reviewer {
            Some(reviewer) if amount >= reviewer.threshold => {
                let review = review.as_ref().unwrap_or_else(|| {
                    reject(&format!(
                        "payments of {} or more need an AI reviewer approval",
                        reviewer.threshold
                    ))
                });
                Some(
                    review::verify(
                        review,
                        reviewer,
                        &quote_json,
                        &q.hash,
                        now / NS_PER_SEC,
                        self.max_quote_age_secs,
                    )
                    .unwrap_or_else(|e| reject(&e)),
                )
            }
            _ => None,
        };

        // Daily budget.
        let today = now / NS_PER_DAY;
        if agent.day != today {
            agent.day = today;
            agent.spent_today = U128(0);
        }
        let spent = agent
            .spent_today
            .0
            .checked_add(amount)
            .unwrap_or_else(|| reject("overflow"));
        if spent > self.daily_limit_per_agent {
            reject(&format!(
                "daily budget exceeded: {} already spent today, limit {}",
                agent.spent_today.0, self.daily_limit_per_agent
            ));
        }

        // Funds: a job's escrow, or the owner's pool. Never one for the other.
        match &job_id {
            Some(id) => {
                self.check_job_payment(id, &agent_pk, &q.recipient, amount, now / NS_PER_SEC)
            }
            None if amount > self.pool => reject(&format!(
                "the vault pool has {} available; escrowed job funds need a job_id",
                self.pool
            )),
            None => {}
        }

        // Effects before the cross-contract call; the callback reverts them if the transfer fails.
        match &job_id {
            Some(id) => self.debit_job(id, &agent_pk, amount),
            None => {
                self.pool -= amount;
                self.in_flight += amount;
            }
        }
        agent.spent_today = U128(spent);
        agent.total_spent = U128(agent.total_spent.0 + amount);
        agent.payments += 1;
        self.agents.insert(agent_pk.clone(), agent.clone());
        self.used_deposits.insert(q.deposit_address.clone());
        if let Some(t) = &invoice_terms {
            invoice::mark_paid(&t.id, &q.deposit_address);
            emit(
                "invoice_payment",
                near_sdk::serde_json::json!({
                    "invoice_id": t.id,
                    "vendor": t.vendor,
                    "payout": t.payout,
                    "amount": t.amount,
                    "deposit_address": q.deposit_address,
                }),
            );
        }

        if let (Some(a), Some(reviewer)) = (&approval, &self.reviewer) {
            emit(
                "review_approved",
                near_sdk::serde_json::json!({
                    "agent": agent.label,
                    "model": reviewer.model,
                    "request_sha256": a.request_sha256,
                    "response_sha256": a.response_sha256,
                    "quote_hash": q.hash,
                }),
            );
        }
        emit(
            "payment_requested",
            near_sdk::serde_json::json!({
                "agent": agent.label,
                "deposit_address": q.deposit_address,
                "recipient": q.recipient,
                "destination_asset": q.destination_asset,
                "amount": amount.to_string(),
                "quote_hash": q.hash,
                "job_id": job_id,
            }),
        );

        let deposit: AccountId = q
            .deposit_address
            .parse()
            .unwrap_or_else(|_| reject("deposit address is not a NEAR account"));
        if via_intents {
            // Deposit into the verifier, owned by the deposit address (verifier ft_on_transfer:
            // `msg` is the AccountId of the internal recipient).
            return Promise::new(self.token.clone())
                .function_call(
                    "ft_transfer_call".to_string(),
                    near_sdk::serde_json::json!({
                        "receiver_id": self.intents_contract,
                        "amount": amount.to_string(),
                        "memo": format!("beeline:{}", q.hash),
                        "msg": deposit,
                    })
                    .to_string()
                    .into_bytes(),
                    NearToken::from_yoctonear(1),
                    GAS_FT_TRANSFER_CALL,
                )
                .then(
                    Self::ext(env::current_account_id())
                        .with_static_gas(GAS_CALLBACK)
                        .on_intents_deposit_complete(
                            agent_pk,
                            q.deposit_address,
                            U128(amount),
                            today,
                            job_id,
                        ),
                );
        }
        Promise::new(self.token.clone())
            .function_call(
                "storage_deposit".to_string(),
                near_sdk::serde_json::json!({ "account_id": deposit, "registration_only": true })
                    .to_string()
                    .into_bytes(),
                STORAGE_DEPOSIT,
                GAS_STORAGE_DEPOSIT,
            )
            .function_call(
                "ft_transfer".to_string(),
                near_sdk::serde_json::json!({
                    "receiver_id": deposit,
                    "amount": amount.to_string(),
                    "memo": format!("beeline:{}", q.hash),
                })
                .to_string()
                .into_bytes(),
                NearToken::from_yoctonear(1),
                GAS_FT_TRANSFER,
            )
            .then(
                Self::ext(env::current_account_id())
                    .with_static_gas(GAS_CALLBACK)
                    .on_transfer_complete(agent_pk, q.deposit_address, U128(amount), today, job_id),
            )
    }

    /// Reverts budget and single-use bookkeeping if the token transfer failed, because then no
    /// value moved and the agent may safely try again with the same quote.
    #[private]
    pub fn on_transfer_complete(
        &mut self,
        agent_pk: PublicKey,
        deposit_address: String,
        amount: U128,
        day: u64,
        job_id: Option<String>,
    ) -> bool {
        self.settle_payment(
            transfer_succeeded(),
            agent_pk,
            deposit_address,
            amount,
            day,
            job_id,
        )
    }

    /// Settles an INTENTS-rail payment. `ft_transfer_call` returns the amount the verifier kept
    /// (the token contract refunds the rest), so only a full deposit counts as paid. A partial
    /// deposit is treated as paid for the kept part, and the rest is returned to its source.
    #[private]
    pub fn on_intents_deposit_complete(
        &mut self,
        agent_pk: PublicKey,
        deposit_address: String,
        amount: U128,
        day: u64,
        job_id: Option<String>,
    ) -> bool {
        let used = intents_deposit_used();
        if used >= amount.0 {
            return self.settle_payment(true, agent_pk, deposit_address, amount, day, job_id);
        }
        if used == 0 {
            return self.settle_payment(false, agent_pk, deposit_address, amount, day, job_id);
        }
        let unsent = amount.0 - used;
        match &job_id {
            Some(id) => self.restore_job(id, &agent_pk, unsent),
            None => {
                self.in_flight = self.in_flight.saturating_sub(unsent);
                self.pool += unsent;
            }
        }
        emit(
            "payment_partial",
            near_sdk::serde_json::json!({ "deposit_address": deposit_address, "amount": amount, "deposited": U128(used), "job_id": job_id }),
        );
        self.settle_payment(true, agent_pk, deposit_address, U128(used), day, job_id)
    }

    /// Owner: pulls tokens from this vault's own balance inside the NEAR Intents verifier (for
    /// example 1Click refunds of INTENTS quotes) back to the vault. Call `sync_balance` after it
    /// completes to credit them to the pool.
    pub fn withdraw_from_intents(&mut self, amount: U128) -> Promise {
        self.assert_owner();
        require!(amount.0 > 0, "amount must be positive");
        Promise::new(self.intents_contract.clone()).function_call(
            "ft_withdraw".to_string(),
            near_sdk::serde_json::json!({
                "token": self.token,
                "receiver_id": env::current_account_id(),
                "amount": amount,
                "memo": "beeline:intents-refund",
            })
            .to_string()
            .into_bytes(),
            NearToken::from_yoctonear(1),
            GAS_INTENTS_WITHDRAW,
        )
    }

    /// Owner: sets the NEAR Intents verifier account (default `intents.near`).
    pub fn set_intents_contract(&mut self, account_id: AccountId) {
        self.assert_owner();
        emit(
            "intents_contract_set",
            near_sdk::serde_json::json!({ "account_id": account_id }),
        );
        self.intents_contract = account_id;
    }
}

impl AgentVault {
    fn settle_payment(
        &mut self,
        ok: bool,
        agent_pk: PublicKey,
        deposit_address: String,
        amount: U128,
        day: u64,
        job_id: Option<String>,
    ) -> bool {
        if ok {
            self.in_flight = self.in_flight.saturating_sub(amount.0);
            self.completed_out += amount.0;
            emit(
                "payment_sent",
                near_sdk::serde_json::json!({ "deposit_address": deposit_address, "amount": amount, "job_id": job_id }),
            );
            return true;
        }
        match &job_id {
            Some(id) => self.restore_job(id, &agent_pk, amount.0),
            None => {
                self.in_flight = self.in_flight.saturating_sub(amount.0);
                self.pool += amount.0;
            }
        }
        if let Some(agent) = self.agents.get_mut(&agent_pk) {
            if agent.day == day {
                agent.spent_today = U128(agent.spent_today.0.saturating_sub(amount.0));
            }
            agent.total_spent = U128(agent.total_spent.0.saturating_sub(amount.0));
            agent.payments = agent.payments.saturating_sub(1);
        }
        self.used_deposits.remove(&deposit_address);
        if let Some(id) = invoice::release_for_deposit(&deposit_address) {
            emit(
                "invoice_released",
                near_sdk::serde_json::json!({ "invoice_id": id }),
            );
        }
        emit(
            "payment_failed",
            near_sdk::serde_json::json!({ "deposit_address": deposit_address, "amount": amount, "job_id": job_id }),
        );
        false
    }
}

#[near]
impl AgentVault {
    // ----- Owner administration ----------------------------------------------------------

    pub(crate) fn assert_owner(&self) {
        require!(
            env::predecessor_account_id() == self.owner,
            "BEELINE_VAULT_REJECTED: owner only"
        );
    }

    /// Registers an agent and gives it a function-call key that can only call `pay_quote`.
    pub fn add_agent(&mut self, public_key: PublicKey, label: String) -> Promise {
        self.assert_owner();
        require!(
            !label.is_empty() && label.len() <= 64,
            "label must be 1-64 characters"
        );
        require!(
            self.agents.get(&public_key).is_none(),
            "agent key already registered"
        );
        self.agents.insert(
            public_key.clone(),
            Agent {
                label: label.clone(),
                active: true,
                day: 0,
                spent_today: U128(0),
                total_spent: U128(0),
                payments: 0,
            },
        );
        emit(
            "agent_added",
            near_sdk::serde_json::json!({ "label": label, "public_key": public_key }),
        );
        Promise::new(env::current_account_id()).add_access_key_allowance(
            public_key,
            Allowance::limited(AGENT_KEY_ALLOWANCE).expect("non-zero allowance"),
            env::current_account_id(),
            "pay_quote".to_string(),
        )
    }

    /// Deactivates an agent and deletes its access key.
    pub fn revoke_agent(&mut self, public_key: PublicKey) -> Promise {
        self.assert_owner();
        let agent = self
            .agents
            .get_mut(&public_key)
            .unwrap_or_else(|| reject("unknown agent"));
        agent.active = false;
        emit(
            "agent_revoked",
            near_sdk::serde_json::json!({ "public_key": public_key }),
        );
        Promise::new(env::current_account_id()).delete_key(public_key)
    }

    /// Re-issues an active agent's function-call key with a new gas allowance (in yoctoNEAR,
    /// at most 10 NEAR). The key can still only call `pay_quote`; the allowance caps how much
    /// of the vault's NEAR the agent can burn on gas.
    pub fn set_agent_allowance(&mut self, public_key: PublicKey, allowance: U128) -> Promise {
        self.assert_owner();
        require!(
            matches!(self.agents.get(&public_key), Some(a) if a.active),
            "BEELINE_VAULT_REJECTED: unknown or inactive agent"
        );
        let allowance = NearToken::from_yoctonear(allowance.0);
        require!(
            !allowance.is_zero() && allowance <= NearToken::from_near(10),
            "allowance must be between 1 yoctoNEAR and 10 NEAR"
        );
        emit(
            "agent_allowance_set",
            near_sdk::serde_json::json!({ "public_key": public_key, "allowance": U128(allowance.as_yoctonear()) }),
        );
        Promise::new(env::current_account_id())
            .delete_key(public_key.clone())
            .add_access_key_allowance(
                public_key,
                Allowance::limited(allowance).expect("non-zero allowance"),
                env::current_account_id(),
                "pay_quote".to_string(),
            )
    }

    pub fn set_limits(&mut self, max_per_payment: U128, daily_limit_per_agent: U128) {
        self.assert_owner();
        require!(
            max_per_payment.0 > 0 && daily_limit_per_agent.0 >= max_per_payment.0,
            "invalid limits"
        );
        self.max_per_payment = max_per_payment.0;
        self.daily_limit_per_agent = daily_limit_per_agent.0;
    }

    pub fn set_freshness(&mut self, max_quote_age_secs: u64, min_deadline_margin_secs: u64) {
        self.assert_owner();
        require!(
            max_quote_age_secs > 0,
            "max_quote_age_secs must be positive"
        );
        self.max_quote_age_secs = max_quote_age_secs;
        self.min_deadline_margin_secs = min_deadline_margin_secs;
    }

    pub fn add_recipient(&mut self, recipient: String) {
        self.assert_owner();
        self.recipients.insert(recipient);
    }

    pub fn remove_recipient(&mut self, recipient: String) {
        self.assert_owner();
        self.recipients.remove(&recipient);
    }

    pub fn add_destination_asset(&mut self, asset_id: String) {
        self.assert_owner();
        self.destination_assets.insert(asset_id);
    }

    /// Registers the NEAR AI reviewer. Verify the model's TEE attestation report off-chain
    /// first: its ed25519 `signing_address` must equal `signing_key`.
    pub fn set_reviewer(
        &mut self,
        model: String,
        signing_key: String,
        policy_prompt_sha256: String,
        threshold: U128,
    ) {
        self.assert_owner();
        require!(
            !model.is_empty() && model.len() <= 128,
            "model must be 1-128 characters"
        );
        let signing_key = review::from_hex::<32>(&signing_key).unwrap_or_else(|e| reject(&e));
        let policy_prompt_sha256 =
            review::from_hex::<32>(&policy_prompt_sha256).unwrap_or_else(|e| reject(&e));
        emit(
            "reviewer_set",
            near_sdk::serde_json::json!({
                "model": model,
                "signing_key": review::to_hex(&signing_key),
                "policy_prompt_sha256": review::to_hex(&policy_prompt_sha256),
                "threshold": threshold,
            }),
        );
        self.reviewer = Some(Reviewer {
            model,
            signing_key,
            policy_prompt_sha256,
            threshold: threshold.0,
        });
    }

    /// Registers the key that signs vendor invoices (hex ed25519), or removes it with `null`.
    /// Agents may then pay non-allowlisted vendors, but only for invoices this key signed.
    pub fn set_invoice_issuer(&mut self, signing_key: Option<String>) {
        self.assert_owner();
        let key = signing_key
            .as_deref()
            .map(|k| review::from_hex::<32>(k).unwrap_or_else(|e| reject(&e)));
        invoice::set_issuer(key);
        emit(
            "invoice_issuer_set",
            near_sdk::serde_json::json!({ "signing_key": key.map(|k| review::to_hex(&k)) }),
        );
    }

    pub fn get_invoice_issuer(&self) -> Option<String> {
        invoice::issuer().map(|k| review::to_hex(&k))
    }

    pub fn is_invoice_paid(&self, invoice_id: String) -> bool {
        invoice::is_paid(&invoice_id)
    }

    /// Removes the reviewer requirement.
    pub fn clear_reviewer(&mut self) {
        self.assert_owner();
        self.reviewer = None;
        emit("reviewer_cleared", near_sdk::serde_json::json!({}));
    }

    pub fn set_paused(&mut self, paused: bool) {
        self.assert_owner();
        self.paused = paused;
        emit("paused", near_sdk::serde_json::json!({ "paused": paused }));
    }

    // ----- Views -------------------------------------------------------------------------

    pub fn get_config(&self) -> Config {
        Config {
            owner: self.owner.clone(),
            token: self.token.clone(),
            quote_signer: format!("ed25519:{}", bs58::encode(self.quote_signer).into_string()),
            max_per_payment: U128(self.max_per_payment),
            daily_limit_per_agent: U128(self.daily_limit_per_agent),
            max_quote_age_secs: self.max_quote_age_secs,
            min_deadline_margin_secs: self.min_deadline_margin_secs,
            paused: self.paused,
            intents_contract: self.intents_contract.clone(),
        }
    }

    pub fn get_reviewer(&self) -> Option<ReviewerView> {
        self.reviewer.as_ref().map(|r| ReviewerView {
            model: r.model.clone(),
            signing_key: review::to_hex(&r.signing_key),
            policy_prompt_sha256: review::to_hex(&r.policy_prompt_sha256),
            threshold: U128(r.threshold),
        })
    }

    pub fn get_recipients(&self) -> Vec<String> {
        self.recipients.iter().cloned().collect()
    }

    pub fn get_agents(&self) -> Vec<(PublicKey, Agent)> {
        self.agents
            .iter()
            .map(|(k, v)| (k.clone(), v.clone()))
            .collect()
    }

    pub fn is_deposit_used(&self, deposit_address: String) -> bool {
        self.used_deposits.contains(&deposit_address)
    }
}

#[cfg(test)]
mod tests;
