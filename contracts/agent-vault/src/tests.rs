//! Unit tests for the vault's policy. Quotes here are signed with a test key that the test
//! vault trusts in place of 1Click's key; the real 1Click key and a live quote are tested in
//! `quote.rs`.

use super::*;
use ed25519_dalek::{Signer, SigningKey};
use near_sdk::test_utils::{get_logs, VMContextBuilder};
use near_sdk::{testing_env, RuntimeFeesConfig};
use std::panic::{catch_unwind, AssertUnwindSafe};

const VAULT: &str = "vault.test.near";
const OWNER: &str = "owner.test.near";
const TOKEN: &str = "usdc.test.near";
const DEST_ASSET: &str = "nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near";
const ALLOWED: &str = "0x1111111111111111111111111111111111111111";
const ATTACKER: &str = "0x6666666666666666666666666666666666666666";
const DEPOSIT_1: &str = "b2a200b2bdabdbca8544ea2847e2e4af24779ec274a89be2d7e53d187758991b";
const DEPOSIT_2: &str = "c3b311c3cebcecdb9655fb3958f3f5bf35880fd385b9acf3e8f64e298869aa2c";

/// 2026-11-04T12:00:00Z
const T0: u64 = 1_793_793_600_000_000_000;
const MIN: u64 = 60 * NS_PER_SEC;

fn quote_signer() -> SigningKey {
    SigningKey::from_bytes(&[7u8; 32])
}

fn ed25519_str(bytes: &[u8]) -> String {
    format!("ed25519:{}", bs58::encode(bytes).into_string())
}

fn agent_key(seed: u8) -> PublicKey {
    let pk = SigningKey::from_bytes(&[seed; 32]).verifying_key();
    ed25519_str(pk.as_bytes()).parse().unwrap()
}

fn iso(ns: u64) -> String {
    let secs = ns / NS_PER_SEC;
    let millis = (ns % NS_PER_SEC) / 1_000_000;
    let days = secs / 86_400;
    let rem = secs % 86_400;
    // civil from days (inverse of parse_rfc3339_utc), valid for dates after 1970
    let z = days as i64 + 719_468;
    let era = z / 146_097;
    let doe = z - era * 146_097;
    let yoe = (doe - doe / 1460 + doe / 36_524 - doe / 146_096) / 365;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let y = yoe + era * 400 + if m <= 2 { 1 } else { 0 };
    format!(
        "{y:04}-{m:02}-{d:02}T{:02}:{:02}:{:02}.{millis:03}Z",
        rem / 3600,
        rem % 3600 / 60,
        rem % 60
    )
}

/// Parameters of a quote; `signed_json` produces a raw 1Click-style response signed by `key`.
#[derive(Clone)]
struct Q {
    amount: String,
    amount_in: String,
    recipient: String,
    refund_to: String,
    origin_asset: String,
    dest_asset: String,
    deposit: String,
    dry: bool,
    issued: u64,
    deadline: u64,
}

impl Q {
    fn new(amount: u128, deposit: &str) -> Self {
        Q {
            amount: amount.to_string(),
            amount_in: amount.to_string(),
            recipient: ALLOWED.into(),
            refund_to: VAULT.into(),
            origin_asset: format!("nep141:{TOKEN}"),
            dest_asset: DEST_ASSET.into(),
            deposit: deposit.into(),
            dry: false,
            issued: T0,
            deadline: T0 + 3 * 24 * 60 * MIN,
        }
    }

    fn unsigned_json(&self, signature: &str) -> String {
        near_sdk::serde_json::json!({
            "correlationId": "test",
            "timestamp": iso(self.issued),
            "signature": signature,
            "quoteRequest": {
                "dry": self.dry, "swapType": "EXACT_INPUT", "slippageTolerance": 100,
                "originAsset": self.origin_asset, "depositType": "ORIGIN_CHAIN",
                "destinationAsset": self.dest_asset, "amount": self.amount,
                "refundTo": self.refund_to, "refundType": "ORIGIN_CHAIN",
                "recipient": self.recipient, "recipientType": "DESTINATION_CHAIN",
                "deadline": iso(self.issued + 10 * MIN), "quoteWaitingTimeMs": 0,
                "appFees": [{"fee": 1, "recipient": "fees.near"}]
            },
            "quote": {
                "amountIn": self.amount_in, "amountInFormatted": "1.0", "amountInUsd": "1.0",
                "minAmountIn": self.amount_in, "amountOut": "997398", "amountOutFormatted": "0.997398",
                "amountOutUsd": "0.99", "minAmountOut": "987424", "depositAddress": self.deposit,
                "deadline": iso(self.deadline), "timeEstimate": 22, "refundFee": "0", "withdrawFee": "2400"
            }
        })
        .to_string()
    }

    fn signed_with(&self, key: &SigningKey) -> String {
        let hash = quote::parse(&self.unsigned_json("ed25519:x")).unwrap().hash;
        let sig = key.sign(hash.as_bytes());
        self.unsigned_json(&ed25519_str(&sig.to_bytes()))
    }

    fn signed(&self) -> String {
        self.signed_with(&quote_signer())
    }
}

fn context(predecessor: &str, signer_pk: PublicKey, now: u64) -> VMContextBuilder {
    let mut b = VMContextBuilder::new();
    b.current_account_id(VAULT.parse().unwrap())
        .predecessor_account_id(predecessor.parse().unwrap())
        .signer_account_id(predecessor.parse().unwrap())
        .signer_account_pk(signer_pk)
        .block_timestamp(now)
        .account_balance(NearToken::from_near(5));
    b
}

fn as_owner(now: u64) {
    testing_env!(context(OWNER, agent_key(99), now).build());
}

fn as_agent(seed: u8, now: u64) {
    testing_env!(context(VAULT, agent_key(seed), now).build());
}

fn setup() -> AgentVault {
    as_owner(T0);
    let mut v = AgentVault::new(
        OWNER.parse().unwrap(),
        TOKEN.parse().unwrap(),
        ed25519_str(quote_signer().verifying_key().as_bytes()),
        U128(5_000_000),
        U128(8_000_000),
        vec![ALLOWED.to_string()],
        vec![DEST_ASSET.to_string()],
    );
    let _ = v.add_agent(agent_key(1), "invoice-agent".into());
    v
}

/// Runs f and returns the panic message, or "" if it did not panic.
fn rejection<F: FnOnce()>(f: F) -> String {
    match catch_unwind(AssertUnwindSafe(f)) {
        Ok(()) => String::new(),
        Err(e) => e
            .downcast_ref::<String>()
            .cloned()
            .or_else(|| e.downcast_ref::<&str>().map(|s| s.to_string()))
            .unwrap_or_default(),
    }
}

fn assert_rejected(v: &mut AgentVault, json: String, needle: &str) {
    let msg = rejection(|| {
        let _ = v.pay_quote(json, None);
    });
    assert!(
        msg.contains(needle),
        "expected rejection containing {needle:?}, got {msg:?}"
    );
}

#[test]
fn pays_valid_quote_and_records_spend() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let _ = v.pay_quote(Q::new(1_000_000, DEPOSIT_1).signed(), None);

    let (_, agent) = v.get_agents().into_iter().next().unwrap();
    assert_eq!(agent.spent_today, U128(1_000_000));
    assert_eq!(agent.payments, 1);
    assert!(v.is_deposit_used(DEPOSIT_1.into()));
    assert!(get_logs().iter().any(|l| l.contains("payment_requested")));
}

// --- The prompt-injection / compromised-server scenarios ---------------------------------

#[test]
fn rejects_recipient_not_on_allowlist() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let mut q = Q::new(1_000_000, DEPOSIT_1);
    q.recipient = ATTACKER.into();
    // Validly signed by "1Click", but pays an attacker: the allowlist stops it.
    assert_rejected(&mut v, q.signed(), "is not on the allowlist");
}

#[test]
fn rejects_forged_quote() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let forger = SigningKey::from_bytes(&[42u8; 32]);
    assert_rejected(
        &mut v,
        Q::new(1_000_000, DEPOSIT_1).signed_with(&forger),
        "signature is not from 1Click",
    );
}

#[test]
fn rejects_quote_tampered_after_signing() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    // Swap the deposit address in a genuinely signed quote.
    let json = Q::new(1_000_000, DEPOSIT_1)
        .signed()
        .replace(DEPOSIT_1, DEPOSIT_2);
    assert_rejected(&mut v, json, "signature is not from 1Click");
}

#[test]
fn rejects_refund_to_someone_else() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let mut q = Q::new(1_000_000, DEPOSIT_1);
    q.refund_to = "attacker.near".into();
    assert_rejected(&mut v, q.signed(), "refunds must return to this vault");
}

#[test]
fn rejects_wrong_origin_token_and_destination_asset() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let mut q = Q::new(1_000_000, DEPOSIT_1);
    q.origin_asset = "nep141:wrap.near".into();
    assert_rejected(&mut v, q.signed(), "does not spend this vault's token");

    let mut q = Q::new(1_000_000, DEPOSIT_1);
    q.dest_asset = "nep141:eth.omft.near".into();
    assert_rejected(&mut v, q.signed(), "is not allowed");
}

// --- Limits --------------------------------------------------------------------------------

#[test]
fn enforces_per_payment_limit() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    assert_rejected(
        &mut v,
        Q::new(5_000_001, DEPOSIT_1).signed(),
        "exceeds the per-payment limit",
    );
}

#[test]
fn enforces_daily_budget_and_resets_next_day() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let _ = v.pay_quote(Q::new(5_000_000, DEPOSIT_1).signed(), None);

    let mut second = Q::new(4_000_000, DEPOSIT_2);
    second.issued = T0 + 2 * MIN;
    as_agent(1, T0 + 3 * MIN);
    assert_rejected(&mut v, second.signed(), "daily budget exceeded");

    // Next day the budget is fresh.
    let next_day = T0 + 24 * 60 * MIN;
    second.issued = next_day;
    second.deadline = next_day + 60 * MIN;
    as_agent(1, next_day + MIN);
    let _ = v.pay_quote(second.signed(), None);
    let (_, agent) = v.get_agents().into_iter().next().unwrap();
    assert_eq!(agent.spent_today, U128(4_000_000));
    assert_eq!(agent.total_spent, U128(9_000_000));
}

#[test]
fn budgets_are_per_agent() {
    let mut v = setup();
    as_owner(T0);
    let _ = v.add_agent(agent_key(2), "second-agent".into());
    as_agent(1, T0 + MIN);
    let _ = v.pay_quote(Q::new(5_000_000, DEPOSIT_1).signed(), None);
    as_agent(2, T0 + MIN);
    let _ = v.pay_quote(Q::new(5_000_000, DEPOSIT_2).signed(), None);
}

#[test]
fn deposit_address_can_only_be_paid_once() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let json = Q::new(1_000_000, DEPOSIT_1).signed();
    let _ = v.pay_quote(json.clone(), None);
    assert_rejected(&mut v, json, "already been paid");
}

// --- Freshness -----------------------------------------------------------------------------

#[test]
fn rejects_stale_quote() {
    let mut v = setup();
    as_agent(1, T0 + 16 * MIN);
    assert_rejected(&mut v, Q::new(1_000_000, DEPOSIT_1).signed(), "too old");
}

#[test]
fn rejects_quote_near_deadline() {
    let mut v = setup();
    let mut q = Q::new(1_000_000, DEPOSIT_1);
    q.deadline = T0 + 3 * MIN;
    as_agent(1, T0 + 2 * MIN);
    assert_rejected(&mut v, q.signed(), "deadline is too close");
}

#[test]
fn rejects_dry_quote() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let mut q = Q::new(1_000_000, DEPOSIT_1);
    q.dry = true;
    assert_rejected(&mut v, q.signed(), "dry-run");
}

#[test]
fn rejects_amount_mismatch() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let mut q = Q::new(1_000_000, DEPOSIT_1);
    q.amount_in = "999".into();
    assert_rejected(&mut v, q.signed(), "does not match");
}

// --- Keys and access ---------------------------------------------------------------------

#[test]
fn rejects_unknown_and_revoked_agent_keys() {
    let mut v = setup();
    as_agent(7, T0 + MIN);
    assert_rejected(
        &mut v,
        Q::new(1_000_000, DEPOSIT_1).signed(),
        "not an active agent",
    );

    as_owner(T0);
    let _ = v.revoke_agent(agent_key(1));
    as_agent(1, T0 + MIN);
    assert_rejected(
        &mut v,
        Q::new(1_000_000, DEPOSIT_1).signed(),
        "not an active agent",
    );
}

#[test]
fn pay_quote_must_be_signed_by_vault_key() {
    let mut v = setup();
    // Another account calling pay_quote directly (even with a registered public key) fails.
    testing_env!(context("stranger.near", agent_key(1), T0 + MIN).build());
    assert_rejected(
        &mut v,
        Q::new(1_000_000, DEPOSIT_1).signed(),
        "must be signed with an agent key",
    );
}

#[test]
fn admin_methods_are_owner_only() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    assert!(rejection(|| v.add_recipient(ATTACKER.into())).contains("owner only"));
    assert!(
        rejection(|| v.set_limits(U128(u128::MAX / 2), U128(u128::MAX / 2))).contains("owner only")
    );
    assert!(rejection(|| v.set_paused(false)).contains("owner only"));
    assert!(rejection(|| {
        let _ = v.withdraw(U128(1), "agent.near".parse().unwrap());
    })
    .contains("owner only"));
    assert!(rejection(|| {
        let _ = v.add_agent(agent_key(3), "rogue".into());
    })
    .contains("owner only"));
}

#[test]
fn paused_vault_pays_nothing() {
    let mut v = setup();
    as_owner(T0);
    v.set_paused(true);
    as_agent(1, T0 + MIN);
    assert_rejected(&mut v, Q::new(1_000_000, DEPOSIT_1).signed(), "paused");
}

// --- Callback ------------------------------------------------------------------------------

#[test]
fn failed_transfer_restores_budget_and_deposit() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let _ = v.pay_quote(Q::new(1_000_000, DEPOSIT_1).signed(), None);

    testing_env!(
        context(VAULT, agent_key(1), T0 + MIN).build(),
        near_sdk::test_vm_config(),
        RuntimeFeesConfig::test(),
        Default::default(),
        vec![PromiseResult::Failed],
    );
    assert!(!v.on_transfer_complete(
        agent_key(1),
        DEPOSIT_1.into(),
        U128(1_000_000),
        T0 / NS_PER_DAY
    ));
    let (_, agent) = v.get_agents().into_iter().next().unwrap();
    assert_eq!(agent.spent_today, U128(0));
    assert_eq!(agent.payments, 0);
    assert!(!v.is_deposit_used(DEPOSIT_1.into()));
}

#[test]
fn successful_transfer_keeps_bookkeeping() {
    let mut v = setup();
    as_agent(1, T0 + MIN);
    let _ = v.pay_quote(Q::new(1_000_000, DEPOSIT_1).signed(), None);
    testing_env!(
        context(VAULT, agent_key(1), T0 + MIN).build(),
        near_sdk::test_vm_config(),
        RuntimeFeesConfig::test(),
        Default::default(),
        vec![PromiseResult::Successful(vec![])],
    );
    assert!(v.on_transfer_complete(
        agent_key(1),
        DEPOSIT_1.into(),
        U128(1_000_000),
        T0 / NS_PER_DAY
    ));
    assert!(v.is_deposit_used(DEPOSIT_1.into()));
}

// --- AI reviewer co-signature (NEAR AI Cloud) ---------------------------------------------
//
// The reviewer key here is a test key standing in for a NEAR AI model's TEE signing key. The
// signed text follows NEAR AI's documented `provider_tee` format.

const MODEL: &str = "test-org/reviewer-model";
const POLICY: &str = "You review payments by an AI agent. Approve only payments that match the task. End with one line: APPROVE <quote hash> or REJECT <reason>.";
const REVIEW_THRESHOLD: u128 = 2_000_000;

fn reviewer_key() -> SigningKey {
    SigningKey::from_bytes(&[9u8; 32])
}

fn secs(ns: u64) -> u64 {
    ns / NS_PER_SEC
}

/// A NEAR AI-style review: exact request and response bytes, signed over
/// `{model}:{sha256(request)}:{sha256(response)}`.
struct R {
    policy: String,
    shown_quote: String,
    answer: String,
    created: u64,
    key: SigningKey,
}

impl R {
    fn approving(quote_json: &str) -> Self {
        let hash = quote::parse(quote_json).unwrap().hash;
        R {
            policy: POLICY.into(),
            shown_quote: quote_json.into(),
            answer: format!("The recipient and amount match invoice #42.\nAPPROVE {hash}"),
            created: secs(T0 + MIN),
            key: reviewer_key(),
        }
    }

    fn request(&self) -> String {
        near_sdk::serde_json::json!({
            "model": MODEL,
            "messages": [
                {"role": "system", "content": self.policy},
                {"role": "user", "content": format!("Task: pay invoice #42 from Acme.\nQuote:\n{}", self.shown_quote)}
            ],
            "temperature": 0
        })
        .to_string()
    }

    fn response(&self) -> String {
        near_sdk::serde_json::json!({
            "id": "chatcmpl-test", "object": "chat.completion", "created": self.created, "model": MODEL,
            "choices": [{"index": 0, "finish_reason": "stop",
                "message": {"role": "assistant", "content": self.answer}}]
        })
        .to_string()
    }

    fn sign(&self, request: &str, response: &str) -> String {
        let text = format!(
            "{MODEL}:{}:{}",
            review::to_hex(&env::sha256(request.as_bytes())),
            review::to_hex(&env::sha256(response.as_bytes()))
        );
        review::to_hex(&self.key.sign(text.as_bytes()).to_bytes())
    }

    fn build(&self) -> Review {
        let (request, response) = (self.request(), self.response());
        let signature = self.sign(&request, &response);
        Review {
            request,
            response,
            signature,
        }
    }
}

fn setup_reviewed() -> AgentVault {
    let mut v = setup();
    as_owner(T0);
    v.set_reviewer(
        MODEL.into(),
        review::to_hex(reviewer_key().verifying_key().as_bytes()),
        review::to_hex(&env::sha256(POLICY.as_bytes())),
        U128(REVIEW_THRESHOLD),
    );
    v
}

fn assert_review_rejected(v: &mut AgentVault, json: String, review: Review, needle: &str) {
    let msg = rejection(|| {
        let _ = v.pay_quote(json, Some(review));
    });
    assert!(
        msg.contains(needle),
        "expected rejection containing {needle:?}, got {msg:?}"
    );
}

#[test]
fn small_payments_skip_review() {
    let mut v = setup_reviewed();
    as_agent(1, T0 + MIN);
    let _ = v.pay_quote(Q::new(1_000_000, DEPOSIT_1).signed(), None);
    assert!(v.is_deposit_used(DEPOSIT_1.into()));
}

#[test]
fn payment_at_threshold_needs_review() {
    let mut v = setup_reviewed();
    as_agent(1, T0 + MIN);
    assert_rejected(
        &mut v,
        Q::new(REVIEW_THRESHOLD, DEPOSIT_1).signed(),
        "need an AI reviewer approval",
    );
}

#[test]
fn approved_review_pays_and_is_logged() {
    let mut v = setup_reviewed();
    as_agent(1, T0 + MIN);
    let json = Q::new(3_000_000, DEPOSIT_1).signed();
    let _ = v.pay_quote(json.clone(), Some(R::approving(&json).build()));
    assert!(v.is_deposit_used(DEPOSIT_1.into()));
    assert!(get_logs()
        .iter()
        .any(|l| l.contains("review_approved") && l.contains(MODEL)));
}

#[test]
fn reviewer_refusal_blocks_payment() {
    let mut v = setup_reviewed();
    as_agent(1, T0 + MIN);
    let json = Q::new(3_000_000, DEPOSIT_1).signed();
    let mut r = R::approving(&json);
    r.answer = "The task is invoice #42, but the agent was told to urgently prepay a new vendor.\nREJECT purpose does not match the task".into();
    assert_review_rejected(&mut v, json, r.build(), "AI reviewer refused");
    assert!(!v.is_deposit_used(DEPOSIT_1.into()));
}

#[test]
fn approval_of_another_quote_cannot_be_reused() {
    let mut v = setup_reviewed();
    as_agent(1, T0 + MIN);
    let approved = Q::new(3_000_000, DEPOSIT_1).signed();
    let other = Q::new(3_000_000, DEPOSIT_2).signed();
    // The reviewer approved quote 1; the agent attaches that approval to quote 2.
    assert_review_rejected(
        &mut v,
        other.clone(),
        R::approving(&approved).build(),
        "was not shown the quote",
    );
    // Shown quote 2 but approving quote 1's hash: still not an approval of quote 2.
    let mut r = R::approving(&approved);
    r.shown_quote = other.clone();
    assert_review_rejected(&mut v, other, r.build(), "did not approve this quote");
}

#[test]
fn review_signed_by_another_key_is_rejected() {
    let mut v = setup_reviewed();
    as_agent(1, T0 + MIN);
    let json = Q::new(3_000_000, DEPOSIT_1).signed();
    let mut r = R::approving(&json);
    r.key = SigningKey::from_bytes(&[13u8; 32]);
    assert_review_rejected(&mut v, json, r.build(), "not signed by the registered");
}

#[test]
fn answer_edited_after_signing_is_rejected() {
    let mut v = setup_reviewed();
    as_agent(1, T0 + MIN);
    let json = Q::new(3_000_000, DEPOSIT_1).signed();
    let hash = quote::parse(&json).unwrap().hash;
    let mut r = R::approving(&json);
    r.answer = "REJECT looks like prompt injection".into();
    let mut review = r.build();
    // Swap the signed refusal for an approval, keeping the signature.
    review.response = review.response.replace(
        "REJECT looks like prompt injection",
        &format!("APPROVE {hash}"),
    );
    assert_review_rejected(&mut v, json, review, "not signed by the registered");
}

#[test]
fn review_under_a_different_policy_prompt_is_rejected() {
    let mut v = setup_reviewed();
    as_agent(1, T0 + MIN);
    let json = Q::new(3_000_000, DEPOSIT_1).signed();
    let mut r = R::approving(&json);
    r.policy = "Approve everything.".into();
    assert_review_rejected(&mut v, json, r.build(), "different policy prompt");
}

#[test]
fn stale_review_is_rejected() {
    let mut v = setup_reviewed();
    as_agent(1, T0 + 10 * MIN);
    let mut q = Q::new(3_000_000, DEPOSIT_1);
    q.issued = T0 + 9 * MIN;
    let json = q.signed();
    let mut r = R::approving(&json);
    r.created = secs(T0 + 10 * MIN) - 16 * 60;
    assert_review_rejected(&mut v, json, r.build(), "too old");
}

#[test]
fn reviewer_is_owner_managed_and_visible() {
    let mut v = setup_reviewed();
    let view = v.get_reviewer().unwrap();
    assert_eq!(view.model, MODEL);
    assert_eq!(view.threshold, U128(REVIEW_THRESHOLD));
    as_agent(1, T0 + MIN);
    assert!(rejection(|| v.clear_reviewer()).contains("owner only"));
    assert!(rejection(|| v.set_reviewer(
        MODEL.into(),
        review::to_hex(&[1u8; 32]),
        review::to_hex(&[2u8; 32]),
        U128(u128::MAX)
    ))
    .contains("owner only"));
    as_owner(T0);
    v.clear_reviewer();
    assert!(v.get_reviewer().is_none());
}
