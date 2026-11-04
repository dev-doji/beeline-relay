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
//!   6. the deposit address has **never been paid** before.
//!
//! A prompt-injected agent, or a compromised Beeline server, cannot get past these checks: it
//! cannot forge 1Click's signature or change the allowlist, and every rejection is a failed
//! transaction anyone can see on the explorer.

mod quote;

use near_sdk::json_types::U128;
use near_sdk::store::{IterableMap, IterableSet, LookupSet};
use near_sdk::{
    env, near, require, AccountId, Allowance, BorshStorageKey, Gas, NearToken, PanicOnDefault,
    Promise, PromiseResult, PublicKey,
};

pub use quote::parse_rfc3339_utc;

const NS_PER_DAY: u64 = 86_400 * 1_000_000_000;
const NS_PER_SEC: u64 = 1_000_000_000;
/// NEP-145 registration for the deposit account on the token contract; any excess is refunded.
const STORAGE_DEPOSIT: NearToken = NearToken::from_yoctonear(1_250_000_000_000_000_000_000);
const GAS_STORAGE_DEPOSIT: Gas = Gas::from_tgas(10);
const GAS_FT_TRANSFER: Gas = Gas::from_tgas(15);
const GAS_CALLBACK: Gas = Gas::from_tgas(10);
/// Gas-fee allowance granted to each agent key (it pays for the agent's transactions only).
const AGENT_KEY_ALLOWANCE: NearToken = NearToken::from_millinear(250);

#[near(serializers = [borsh])]
#[derive(BorshStorageKey)]
enum StorageKey {
    Recipients,
    DestinationAssets,
    Agents,
    UsedDeposits,
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
}

fn reject(reason: &str) -> ! {
    env::panic_str(&format!("BEELINE_VAULT_REJECTED: {reason}"))
}

/// Emits a NEP-297 event log.
fn emit(event: &str, data: near_sdk::serde_json::Value) {
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
    /// Callable only with an active agent access key on this account.
    pub fn pay_quote(&mut self, quote_json: String) -> Promise {
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
        if q.swap_type != "EXACT_INPUT" || q.deposit_type != "ORIGIN_CHAIN" {
            reject("only EXACT_INPUT quotes with ORIGIN_CHAIN deposits are supported");
        }
        if q.origin_asset != format!("nep141:{}", self.token) {
            reject("quote does not spend this vault's token");
        }
        if q.refund_to != env::current_account_id().as_str() || q.refund_type != "ORIGIN_CHAIN" {
            reject("refunds must return to this vault");
        }
        if !self.destination_assets.contains(&q.destination_asset) {
            reject(&format!(
                "destination asset {} is not allowed",
                q.destination_asset
            ));
        }
        if q.recipient_type != "DESTINATION_CHAIN" || !self.recipients.contains(&q.recipient) {
            reject(&format!(
                "recipient {} is not on the allowlist",
                q.recipient
            ));
        }
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

        // Effects before the cross-contract call; the callback reverts them if the transfer fails.
        agent.spent_today = U128(spent);
        agent.total_spent = U128(agent.total_spent.0 + amount);
        agent.payments += 1;
        self.agents.insert(agent_pk.clone(), agent.clone());
        self.used_deposits.insert(q.deposit_address.clone());

        emit(
            "payment_requested",
            near_sdk::serde_json::json!({
                "agent": agent.label,
                "deposit_address": q.deposit_address,
                "recipient": q.recipient,
                "destination_asset": q.destination_asset,
                "amount": amount.to_string(),
                "quote_hash": q.hash,
            }),
        );

        let deposit: AccountId = q
            .deposit_address
            .parse()
            .unwrap_or_else(|_| reject("deposit address is not a NEAR account"));
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
                    .on_transfer_complete(agent_pk, q.deposit_address, U128(amount), today),
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
    ) -> bool {
        #[allow(deprecated)] // the result body is not read, so its size cannot exhaust gas
        let ok = env::promise_results_count() == 1
            && matches!(env::promise_result(0), PromiseResult::Successful(_));
        if ok {
            emit(
                "payment_sent",
                near_sdk::serde_json::json!({ "deposit_address": deposit_address, "amount": amount }),
            );
            return true;
        }
        if let Some(agent) = self.agents.get_mut(&agent_pk) {
            if agent.day == day {
                agent.spent_today = U128(agent.spent_today.0.saturating_sub(amount.0));
            }
            agent.total_spent = U128(agent.total_spent.0.saturating_sub(amount.0));
            agent.payments = agent.payments.saturating_sub(1);
        }
        self.used_deposits.remove(&deposit_address);
        emit(
            "payment_failed",
            near_sdk::serde_json::json!({ "deposit_address": deposit_address, "amount": amount }),
        );
        false
    }

    // ----- Owner administration ----------------------------------------------------------

    fn assert_owner(&self) {
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

    pub fn set_paused(&mut self, paused: bool) {
        self.assert_owner();
        self.paused = paused;
        emit("paused", near_sdk::serde_json::json!({ "paused": paused }));
    }

    /// Returns vault tokens to an account of the owner's choosing.
    pub fn withdraw(&mut self, amount: U128, receiver_id: AccountId) -> Promise {
        self.assert_owner();
        Promise::new(self.token.clone()).function_call(
            "ft_transfer".to_string(),
            near_sdk::serde_json::json!({ "receiver_id": receiver_id, "amount": amount })
                .to_string()
                .into_bytes(),
            NearToken::from_yoctonear(1),
            GAS_FT_TRANSFER,
        )
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
        }
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
