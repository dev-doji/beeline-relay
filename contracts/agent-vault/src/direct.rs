//! Direct payments to NEAR accounts (`pay_direct`).
//!
//! `pay_quote` pays through NEAR Intents 1Click, which settles cross-chain but has no testnet.
//! `pay_direct` pays a NEAR account directly with the vault's token, so the full flow (agent key,
//! guardrails, AI reviewer, real token transfer to a real account) runs on NEAR testnet as well
//! as mainnet. Every guardrail is the same:
//!
//! * signed only with an active agent key of this vault;
//! * recipient on the allowlist, or the payee of a signed vendor invoice;
//! * per-payment limit, daily budget, enough funds in the pool;
//! * fresh (issued within `max_quote_age_secs`), bound to this vault, single use (nonce);
//! * at or above the reviewer threshold, a NEAR AI approval of this exact payment request.
//!
//! The payment request is the JSON the agent submits:
//! `{"v":1,"kind":"direct","vault":"<this vault>","recipient":"<account>","amount":"<base units>",
//!   "nonce":"<8-64 chars>","issued_at":<unix secs>,"memo":"<text>"}`.
//! Its hash (base58 sha256 of the exact bytes) is what the AI reviewer must approve.

use near_sdk::json_types::U128;
use near_sdk::serde_json::Value;
use near_sdk::{env, near, require, AccountId, NearToken, Promise};

use crate::invoice::{self, Invoice};
use crate::review::{self, Review};
use crate::{
    emit, reject, AgentVault, AgentVaultExt, GAS_CALLBACK, GAS_FT_TRANSFER, NS_PER_DAY, NS_PER_SEC,
};

const STORAGE_DEPOSIT: NearToken = NearToken::from_yoctonear(1_250_000_000_000_000_000_000);
const GAS_STORAGE_DEPOSIT: near_sdk::Gas = near_sdk::Gas::from_tgas(10);

/// The parsed payment request.
pub struct DirectRequest {
    pub recipient: AccountId,
    pub amount: u128,
    pub nonce: String,
    pub issued_at: u64,
    pub hash: String,
}

/// Parses a direct payment request and computes the hash the reviewer approves.
pub fn parse(raw: &str, vault: &AccountId) -> Result<DirectRequest, String> {
    let v: Value = near_sdk::serde_json::from_str(raw)
        .map_err(|_| "payment request is not valid JSON".to_string())?;
    let s = |k: &str| {
        v.get(k)
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string()
    };
    if v.get("v").and_then(Value::as_u64) != Some(1) || s("kind") != "direct" {
        return Err("unsupported payment request".into());
    }
    if s("vault") != vault.as_str() {
        return Err("payment request is for a different vault".into());
    }
    let recipient: AccountId = s("recipient")
        .parse()
        .map_err(|_| "recipient is not a valid NEAR account".to_string())?;
    let amount: u128 = s("amount")
        .parse()
        .map_err(|_| "amount is not an integer".to_string())?;
    let nonce = s("nonce");
    if nonce.len() < 8
        || nonce.len() > 64
        || !nonce
            .chars()
            .all(|c| c.is_ascii_alphanumeric() || c == '_' || c == '-')
    {
        return Err("nonce must be 8-64 letters, digits, '_' or '-'".into());
    }
    let issued_at = v
        .get("issued_at")
        .and_then(Value::as_u64)
        .ok_or("payment request has no issued_at")?;
    let hash = bs58::encode(env::sha256(raw.as_bytes())).into_string();
    Ok(DirectRequest {
        recipient,
        amount,
        nonce,
        issued_at,
        hash,
    })
}

#[near]
impl AgentVault {
    /// Pays a NEAR account directly, if every guardrail passes. Callable only with an active
    /// agent key of this vault. `review` is required at or above the reviewer threshold;
    /// `invoice` lets the agent pay a non-allowlisted vendor for exactly that signed invoice.
    pub fn pay_direct(
        &mut self,
        payment_json: String,
        review: Option<Review>,
        invoice: Option<Invoice>,
    ) -> Promise {
        if self.paused {
            reject("vault is paused");
        }
        require!(
            env::predecessor_account_id() == env::current_account_id(),
            "BEELINE_VAULT_REJECTED: pay_direct must be signed with an agent key of this vault"
        );
        let agent_pk = env::signer_account_pk();
        let mut agent = match self.agents.get(&agent_pk) {
            Some(a) if a.active => a.clone(),
            _ => reject("signing key is not an active agent"),
        };

        let p = parse(&payment_json, &env::current_account_id()).unwrap_or_else(|e| reject(&e));
        let recipient = p.recipient.to_string();

        // Recipient: on the owner's allowlist, or the payee of a signed vendor invoice.
        let invoice_terms = match &invoice {
            None => {
                if !self.recipients.contains(&recipient) {
                    reject(&format!("recipient {recipient} is not on the allowlist"));
                }
                None
            }
            Some(inv) => {
                let issuer =
                    invoice::issuer().unwrap_or_else(|| reject("this vault accepts no invoices"));
                let t = invoice::verify(inv, &issuer).unwrap_or_else(|e| reject(&e));
                if recipient != t.payout {
                    reject(&format!(
                        "payment pays {recipient} but invoice {} pays {}",
                        t.id, t.payout
                    ));
                }
                if p.amount.to_string() != t.amount {
                    reject(&format!(
                        "payment amount {} does not match invoice {} amount {}",
                        p.amount, t.id, t.amount
                    ));
                }
                if t.asset != format!("nep141:{}", self.token) {
                    reject("invoice asset is not this vault's token");
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

        if p.amount == 0 || p.amount > self.max_per_payment {
            reject(&format!(
                "amount {} exceeds the per-payment limit {}",
                p.amount, self.max_per_payment
            ));
        }

        // Freshness and single use.
        let now = env::block_timestamp();
        let now_secs = now / NS_PER_SEC;
        if now_secs.saturating_sub(p.issued_at) > self.max_quote_age_secs
            || p.issued_at > now_secs + 300
        {
            reject("payment request is too old; create a fresh one");
        }
        let key = format!("direct:{}", p.nonce);
        if self.used_deposits.contains(&key) {
            reject("this payment request has already been paid");
        }

        // AI reviewer co-signature over this exact request.
        let approval = match &self.reviewer {
            Some(reviewer) if p.amount >= reviewer.threshold => {
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
                        &payment_json,
                        &p.hash,
                        now_secs,
                        self.max_quote_age_secs,
                    )
                    .unwrap_or_else(|e| reject(&e)),
                )
            }
            _ => None,
        };

        // Daily budget and funds (direct payments draw on the pool).
        let today = now / NS_PER_DAY;
        if agent.day != today {
            agent.day = today;
            agent.spent_today = U128(0);
        }
        let spent = agent
            .spent_today
            .0
            .checked_add(p.amount)
            .unwrap_or_else(|| reject("overflow"));
        if spent > self.daily_limit_per_agent {
            reject(&format!(
                "daily budget exceeded: {} already spent today, limit {}",
                agent.spent_today.0, self.daily_limit_per_agent
            ));
        }
        if p.amount > self.pool {
            reject(&format!("the vault pool has {} available", self.pool));
        }

        // Effects before the transfer; the callback reverts them if it fails.
        self.pool -= p.amount;
        self.in_flight += p.amount;
        agent.spent_today = U128(spent);
        agent.total_spent = U128(agent.total_spent.0 + p.amount);
        agent.payments += 1;
        self.agents.insert(agent_pk.clone(), agent.clone());
        self.used_deposits.insert(key.clone());
        if let Some(t) = &invoice_terms {
            invoice::mark_paid(&t.id, &key);
            emit(
                "invoice_payment",
                near_sdk::serde_json::json!({ "invoice_id": t.id, "vendor": t.vendor, "payout": t.payout, "amount": t.amount, "deposit_address": key }),
            );
        }
        if let (Some(a), Some(reviewer)) = (&approval, &self.reviewer) {
            emit(
                "review_approved",
                near_sdk::serde_json::json!({
                    "agent": agent.label, "model": reviewer.model, "request_sha256": a.request_sha256,
                    "response_sha256": a.response_sha256, "payment_hash": p.hash,
                }),
            );
        }
        emit(
            "payment_requested",
            near_sdk::serde_json::json!({
                "rail": "direct", "agent": agent.label, "recipient": recipient,
                "amount": p.amount.to_string(), "payment_hash": p.hash, "deposit_address": key,
            }),
        );

        Promise::new(self.token.clone())
            .function_call(
                "storage_deposit".to_string(),
                near_sdk::serde_json::json!({ "account_id": recipient, "registration_only": true })
                    .to_string()
                    .into_bytes(),
                STORAGE_DEPOSIT,
                GAS_STORAGE_DEPOSIT,
            )
            .function_call(
                "ft_transfer".to_string(),
                near_sdk::serde_json::json!({
                    "receiver_id": recipient,
                    "amount": p.amount.to_string(),
                    "memo": format!("beeline:{}", p.hash),
                })
                .to_string()
                .into_bytes(),
                NearToken::from_yoctonear(1),
                GAS_FT_TRANSFER,
            )
            .then(
                Self::ext(env::current_account_id())
                    .with_static_gas(GAS_CALLBACK)
                    .on_transfer_complete(agent_pk, key, U128(p.amount), today, None),
            )
    }
}
