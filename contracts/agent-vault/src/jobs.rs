//! Swarm job escrow.
//!
//! A hirer (the job's `funder`) escrows a budget for one job in the vault. Several agents, each
//! with its own key and its own cap for this job, spend from that budget through the normal
//! `pay_quote` checks. The escrow adds three guarantees on top of the vault's policy:
//!
//! * job money can only leave as a policy-checked agent payment for this job, or as a refund to
//!   the funder; the owner cannot `withdraw` it and pool payments cannot touch it;
//! * each member agent is limited to its own cap within the job, and only members can spend it;
//! * a job's recipient list can only narrow the vault's allowlist, never widen it.
//!
//! Accounting: every token the vault knows about is in exactly one bucket: the owner's `pool`,
//! a job's `balance` (summed in `escrowed`), or `in_flight` (sent, awaiting the transfer
//! callback). Tokens that arrive without `ft_transfer_call` (1Click refunds, plain transfers)
//! are untracked until the owner calls `sync_balance`, which credits the surplus to the pool.

use near_sdk::json_types::U128;
use near_sdk::{env, near, require, AccountId, NearToken, Promise, PromiseError, PublicKey};

use crate::{emit, reject, AgentVault, AgentVaultExt, GAS_CALLBACK, GAS_FT_TRANSFER, NS_PER_SEC};

const MAX_JOB_MEMBERS: usize = 16;
const MAX_JOB_RECIPIENTS: usize = 16;
const GAS_FT_BALANCE_OF: near_sdk::Gas = near_sdk::Gas::from_tgas(5);

#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq)]
pub enum JobStatus {
    Open,
    Closed,
}

#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq)]
pub struct JobMember {
    pub public_key: PublicKey,
    /// Most this agent may spend from this job in total.
    pub cap: U128,
    pub spent: U128,
}

/// A member as given to `create_job`.
#[near(serializers = [json])]
pub struct JobMemberInput {
    pub public_key: PublicKey,
    pub cap: U128,
}

#[near(serializers = [borsh, json])]
#[derive(Clone, Debug, PartialEq)]
pub struct Job {
    /// The only account that can fund the job, and the only account refunds go to.
    pub funder: AccountId,
    pub members: Vec<JobMember>,
    /// If non-empty, payments for this job may only go to these recipients (each must also be
    /// on the vault's allowlist at payment time).
    pub recipients: Vec<String>,
    /// Unix seconds after which the job pays nothing and anyone may close it.
    pub expires_at_secs: u64,
    /// Escrowed and not yet spent or refunded.
    pub balance: U128,
    pub funded: U128,
    pub spent: U128,
    pub refunded: U128,
    pub status: JobStatus,
}

#[near(serializers = [json])]
pub struct Balances {
    /// The owner's general funds: usable by non-job payments and `withdraw`.
    pub pool: U128,
    /// Sum of all job balances.
    pub escrowed: U128,
    /// Sent and awaiting the transfer result.
    pub in_flight: U128,
}

#[near(serializers = [json])]
struct FundJobMsg {
    fund_job: String,
}

impl AgentVault {
    /// Checks that `agent_pk` may pay `amount` to `recipient` from job `job_id` now. Read-only.
    pub(crate) fn check_job_payment(
        &self,
        job_id: &str,
        agent_pk: &PublicKey,
        recipient: &str,
        amount: u128,
        now_secs: u64,
    ) {
        let job = self
            .jobs
            .get(job_id)
            .unwrap_or_else(|| reject(&format!("unknown job {job_id}")));
        if job.status != JobStatus::Open {
            reject(&format!("job {job_id} is closed"));
        }
        if now_secs >= job.expires_at_secs {
            reject(&format!("job {job_id} has expired"));
        }
        let member = job
            .members
            .iter()
            .find(|m| &m.public_key == agent_pk)
            .unwrap_or_else(|| reject(&format!("agent is not a member of job {job_id}")));
        if !job.recipients.is_empty() && !job.recipients.iter().any(|r| r == recipient) {
            reject(&format!(
                "recipient {recipient} is not allowed for job {job_id}"
            ));
        }
        let spent = member
            .spent
            .0
            .checked_add(amount)
            .unwrap_or_else(|| reject("overflow"));
        if spent > member.cap.0 {
            reject(&format!(
                "agent's cap for job {job_id} exceeded: {} spent, cap {}",
                member.spent.0, member.cap.0
            ));
        }
        if amount > job.balance.0 {
            reject(&format!(
                "job {job_id} has {} left in escrow",
                job.balance.0
            ));
        }
    }

    /// Moves `amount` from the job's escrow to in-flight. Call only after `check_job_payment`.
    pub(crate) fn debit_job(&mut self, job_id: &str, agent_pk: &PublicKey, amount: u128) {
        let job = self.jobs.get_mut(job_id).expect("checked");
        job.balance = U128(job.balance.0 - amount);
        job.spent = U128(job.spent.0 + amount);
        let member = job
            .members
            .iter_mut()
            .find(|m| &m.public_key == agent_pk)
            .expect("checked");
        member.spent = U128(member.spent.0 + amount);
        self.escrowed -= amount;
        self.in_flight += amount;
    }

    /// Undoes `debit_job` after a failed transfer. The job may have closed meanwhile; the money
    /// then waits in its balance until `close_job` is called again, which refunds it.
    pub(crate) fn restore_job(&mut self, job_id: &str, agent_pk: &PublicKey, amount: u128) {
        self.in_flight = self.in_flight.saturating_sub(amount);
        if let Some(job) = self.jobs.get_mut(job_id) {
            job.balance = U128(job.balance.0 + amount);
            job.spent = U128(job.spent.0.saturating_sub(amount));
            if let Some(m) = job.members.iter_mut().find(|m| &m.public_key == agent_pk) {
                m.spent = U128(m.spent.0.saturating_sub(amount));
            }
            self.escrowed += amount;
        }
    }

    fn ft_transfer(&self, receiver_id: &AccountId, amount: u128, memo: &str) -> Promise {
        Promise::new(self.token.clone()).function_call(
            "ft_transfer".to_string(),
            near_sdk::serde_json::json!({
                "receiver_id": receiver_id,
                "amount": amount.to_string(),
                "memo": memo,
            })
            .to_string()
            .into_bytes(),
            NearToken::from_yoctonear(1),
            GAS_FT_TRANSFER,
        )
    }
}

#[near]
impl AgentVault {
    // ----- Funding -----------------------------------------------------------------------

    /// NEP-141 receiver. `msg` empty: credit the owner's pool. `msg` = `{"fund_job":"<id>"}`:
    /// escrow the tokens for that open job; only its funder may do this. Any rejection panics,
    /// so the token contract returns the full amount to the sender.
    pub fn ft_on_transfer(&mut self, sender_id: AccountId, amount: U128, msg: String) -> U128 {
        require!(
            env::predecessor_account_id() == self.token,
            "BEELINE_VAULT_REJECTED: only the vault's token can be deposited"
        );
        require!(amount.0 > 0, "BEELINE_VAULT_REJECTED: zero deposit");
        if msg.is_empty() {
            self.pool += amount.0;
            emit(
                "pool_funded",
                near_sdk::serde_json::json!({ "sender": sender_id, "amount": amount }),
            );
            return U128(0);
        }
        let m: FundJobMsg = near_sdk::serde_json::from_str(&msg)
            .unwrap_or_else(|_| reject("msg must be empty or {\"fund_job\":\"<job id>\"}"));
        let now_secs = env::block_timestamp() / NS_PER_SEC;
        let job = self
            .jobs
            .get_mut(&m.fund_job)
            .unwrap_or_else(|| reject(&format!("unknown job {}", m.fund_job)));
        if job.funder != sender_id {
            reject("only the job's funder can fund it");
        }
        if job.status != JobStatus::Open || now_secs >= job.expires_at_secs {
            reject("job is closed or expired");
        }
        job.balance = U128(job.balance.0 + amount.0);
        job.funded = U128(job.funded.0 + amount.0);
        self.escrowed += amount.0;
        emit(
            "job_funded",
            near_sdk::serde_json::json!({ "job_id": m.fund_job, "funder": sender_id, "amount": amount }),
        );
        U128(0)
    }

    /// Credits tokens the vault holds but does not track (1Click refunds, plain `ft_transfer`s,
    /// test mints) to the owner's pool. Owner only.
    pub fn sync_balance(&mut self) -> Promise {
        self.assert_owner();
        Promise::new(self.token.clone())
            .function_call(
                "ft_balance_of".to_string(),
                near_sdk::serde_json::json!({ "account_id": env::current_account_id() })
                    .to_string()
                    .into_bytes(),
                NearToken::from_yoctonear(0),
                GAS_FT_BALANCE_OF,
            )
            .then(
                Self::ext(env::current_account_id())
                    .with_static_gas(GAS_CALLBACK)
                    .on_balance(U128(self.completed_out)),
            )
    }

    /// `completed_out_before` is the completed-outflow counter when `sync_balance` was called.
    /// Transfers that complete between the balance read and this callback are already out of
    /// `in_flight` but were still in the balance read, so they are subtracted to never over-credit.
    #[private]
    pub fn on_balance(
        &mut self,
        completed_out_before: U128,
        #[callback_result] balance: Result<U128, PromiseError>,
    ) -> U128 {
        let balance = match balance {
            Ok(b) => b.0,
            Err(_) => reject("could not read the token balance"),
        };
        let tracked = self.pool + self.escrowed + self.in_flight;
        let completed_since = self.completed_out.saturating_sub(completed_out_before.0);
        let surplus = balance
            .saturating_sub(tracked)
            .saturating_sub(completed_since);
        if balance < tracked {
            emit(
                "balance_shortfall",
                near_sdk::serde_json::json!({ "balance": U128(balance), "tracked": U128(tracked) }),
            );
        }
        if surplus > 0 {
            self.pool += surplus;
            emit(
                "pool_synced",
                near_sdk::serde_json::json!({ "credited": U128(surplus) }),
            );
        }
        U128(surplus)
    }

    // ----- Jobs --------------------------------------------------------------------------

    /// Creates a job. Members must be active agents; `recipients`, if given, must already be on
    /// the vault's allowlist. Job IDs are never reused. Owner only.
    pub fn create_job(
        &mut self,
        job_id: String,
        funder: AccountId,
        members: Vec<JobMemberInput>,
        recipients: Vec<String>,
        expires_at_secs: u64,
    ) {
        self.assert_owner();
        require!(
            !job_id.is_empty() && job_id.len() <= 64,
            "job_id must be 1-64 characters"
        );
        if self.jobs.get(&job_id).is_some() {
            reject(&format!("job {job_id} already exists"));
        }
        require!(
            !members.is_empty() && members.len() <= MAX_JOB_MEMBERS,
            "a job needs 1-16 members"
        );
        require!(
            recipients.len() <= MAX_JOB_RECIPIENTS,
            "a job can list at most 16 recipients"
        );
        require!(
            expires_at_secs > env::block_timestamp() / NS_PER_SEC,
            "job expiry must be in the future"
        );
        let mut job_members: Vec<JobMember> = Vec::with_capacity(members.len());
        for m in members {
            require!(m.cap.0 > 0, "member cap must be positive");
            if job_members.iter().any(|x| x.public_key == m.public_key) {
                reject("duplicate member key");
            }
            match self.agents.get(&m.public_key) {
                Some(a) if a.active => {}
                _ => reject("job members must be active agents"),
            }
            job_members.push(JobMember {
                public_key: m.public_key,
                cap: m.cap,
                spent: U128(0),
            });
        }
        for r in &recipients {
            if !self.recipients.contains(r) {
                reject(&format!("job recipient {r} is not on the vault allowlist"));
            }
        }
        emit(
            "job_created",
            near_sdk::serde_json::json!({
                "job_id": job_id,
                "funder": funder,
                "members": job_members.iter().map(|m| near_sdk::serde_json::json!({
                    "public_key": m.public_key, "cap": m.cap,
                })).collect::<Vec<_>>(),
                "recipients": recipients,
                "expires_at_secs": expires_at_secs,
            }),
        );
        self.jobs.insert(
            job_id,
            Job {
                funder,
                members: job_members,
                recipients,
                expires_at_secs,
                balance: U128(0),
                funded: U128(0),
                spent: U128(0),
                refunded: U128(0),
                status: JobStatus::Open,
            },
        );
    }

    /// Closes a job and refunds its remaining escrow to the funder. The owner or the funder may
    /// close an open job; anyone may close an expired one (the refund can only go to the
    /// funder). Calling it again on a closed job with a balance (left by a failed transfer)
    /// retries the refund. Returns the amount being refunded.
    pub fn close_job(&mut self, job_id: String) -> U128 {
        let caller = env::predecessor_account_id();
        let now_secs = env::block_timestamp() / NS_PER_SEC;
        let job = self
            .jobs
            .get_mut(&job_id)
            .unwrap_or_else(|| reject(&format!("unknown job {job_id}")));
        let expired = now_secs >= job.expires_at_secs;
        if caller != self.owner && caller != job.funder && !expired {
            reject("only the owner or the funder can close a job before it expires");
        }
        if job.status == JobStatus::Closed && job.balance.0 == 0 {
            reject(&format!("job {job_id} is already closed"));
        }
        job.status = JobStatus::Closed;
        let refund = job.balance.0;
        let funder = job.funder.clone();
        if refund > 0 {
            job.balance = U128(0);
            job.refunded = U128(job.refunded.0 + refund);
            self.escrowed -= refund;
            self.in_flight += refund;
        }
        emit(
            "job_closed",
            near_sdk::serde_json::json!({ "job_id": job_id, "refund": U128(refund), "funder": funder }),
        );
        if refund > 0 {
            // Detached: the callback settles the bookkeeping either way.
            let _ = self
                .ft_transfer(&funder, refund, &format!("beeline:refund:{job_id}"))
                .then(
                    Self::ext(env::current_account_id())
                        .with_static_gas(GAS_CALLBACK)
                        .on_refund_complete(job_id, U128(refund)),
                );
        }
        U128(refund)
    }

    #[private]
    pub fn on_refund_complete(&mut self, job_id: String, amount: U128) -> bool {
        self.in_flight = self.in_flight.saturating_sub(amount.0);
        if crate::transfer_succeeded() {
            self.completed_out += amount.0;
            emit(
                "job_refunded",
                near_sdk::serde_json::json!({ "job_id": job_id, "amount": amount }),
            );
            return true;
        }
        if let Some(job) = self.jobs.get_mut(&job_id) {
            job.balance = U128(job.balance.0 + amount.0);
            job.refunded = U128(job.refunded.0.saturating_sub(amount.0));
            self.escrowed += amount.0;
        }
        emit(
            "job_refund_failed",
            near_sdk::serde_json::json!({ "job_id": job_id, "amount": amount }),
        );
        false
    }

    /// Returns pool tokens to an account of the owner's choosing. Escrowed job funds cannot be
    /// withdrawn. Owner only.
    pub fn withdraw(&mut self, amount: U128, receiver_id: AccountId) -> Promise {
        self.assert_owner();
        if amount.0 == 0 || amount.0 > self.pool {
            reject(&format!(
                "can withdraw at most the pool ({}); escrowed job funds are not withdrawable",
                self.pool
            ));
        }
        self.pool -= amount.0;
        self.in_flight += amount.0;
        self.ft_transfer(&receiver_id, amount.0, "beeline:withdraw")
            .then(
                Self::ext(env::current_account_id())
                    .with_static_gas(GAS_CALLBACK)
                    .on_withdraw_complete(U128(amount.0)),
            )
    }

    #[private]
    pub fn on_withdraw_complete(&mut self, amount: U128) -> bool {
        self.in_flight = self.in_flight.saturating_sub(amount.0);
        if crate::transfer_succeeded() {
            self.completed_out += amount.0;
            return true;
        }
        self.pool += amount.0;
        false
    }

    // ----- Views -------------------------------------------------------------------------

    pub fn get_job(&self, job_id: String) -> Option<Job> {
        self.jobs.get(&job_id).cloned()
    }

    pub fn get_jobs(&self, from_index: Option<u32>, limit: Option<u32>) -> Vec<(String, Job)> {
        self.jobs
            .iter()
            .skip(from_index.unwrap_or(0) as usize)
            .take(limit.unwrap_or(50).min(100) as usize)
            .map(|(k, v)| (k.clone(), v.clone()))
            .collect()
    }

    pub fn get_balances(&self) -> Balances {
        Balances {
            pool: U128(self.pool),
            escrowed: U128(self.escrowed),
            in_flight: U128(self.in_flight),
        }
    }
}
