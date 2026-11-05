//! Signed vendor invoices ("pay links").
//!
//! A trusted invoice issuer (registered by the owner) signs each invoice when a vendor creates
//! it. An agent may then pay a vendor who is NOT on the vault's allowlist, but only for that
//! exact invoice: the quote's recipient, amount and destination asset must equal the signed
//! terms, the invoice must not be expired, and each invoice can be paid once. A scammer who
//! edits a copied link (new payout address, bigger amount) breaks the signature.
//!
//! The issuer key, paid-invoice marks and deposit->invoice links live under raw storage keys
//! (not in the contract struct), so adding this feature needs no state migration.

use near_sdk::serde_json::Value;
use near_sdk::{env, near};

const ISSUER_KEY: &[u8] = b"beeline:inv_issuer";
const PAID_PREFIX: &[u8] = b"beeline:inv_paid:";
const DEPOSIT_PREFIX: &[u8] = b"beeline:inv_dep:";

/// A signed invoice as submitted with `pay_quote`.
#[near(serializers = [json])]
#[derive(Clone, Debug)]
pub struct Invoice {
    /// The exact JSON the issuer signed:
    /// {"v":1,"id","vendor","payout","asset","amount","expires_at","memo"}.
    pub payload: String,
    /// Hex ed25519 signature over the payload's UTF-8 bytes.
    pub signature: String,
}

/// The terms the vault enforces.
#[derive(Debug, Clone, PartialEq)]
pub struct Terms {
    pub id: String,
    pub vendor: String,
    pub payout: String,
    pub asset: String,
    pub amount: String,
    pub expires_at: u64,
}

pub fn issuer() -> Option<[u8; 32]> {
    env::storage_read(ISSUER_KEY).and_then(|v| v.try_into().ok())
}

pub fn set_issuer(key: Option<[u8; 32]>) {
    match key {
        Some(k) => {
            env::storage_write(ISSUER_KEY, &k);
        }
        None => {
            env::storage_remove(ISSUER_KEY);
        }
    }
}

fn key(prefix: &[u8], id: &str) -> Vec<u8> {
    [prefix, id.as_bytes()].concat()
}

pub fn is_paid(id: &str) -> bool {
    env::storage_has_key(&key(PAID_PREFIX, id))
}

/// Marks an invoice paid and remembers which deposit paid it (to undo on a failed transfer).
pub fn mark_paid(id: &str, deposit_address: &str) {
    env::storage_write(&key(PAID_PREFIX, id), deposit_address.as_bytes());
    env::storage_write(&key(DEPOSIT_PREFIX, deposit_address), id.as_bytes());
}

/// Undoes `mark_paid` for the invoice a failed deposit was paying, if any. Returns its id.
pub fn release_for_deposit(deposit_address: &str) -> Option<String> {
    let dk = key(DEPOSIT_PREFIX, deposit_address);
    let id = String::from_utf8(env::storage_read(&dk)?).ok()?;
    env::storage_remove(&dk);
    env::storage_remove(&key(PAID_PREFIX, &id));
    Some(id)
}

/// Verifies the issuer's signature and parses the terms.
pub fn verify(inv: &Invoice, issuer: &[u8; 32]) -> Result<Terms, String> {
    let sig = crate::review::from_hex::<64>(&inv.signature)
        .map_err(|_| "invoice signature must be 64 hex-encoded bytes".to_string())?;
    if !env::ed25519_verify(&sig, inv.payload.as_bytes(), issuer) {
        return Err(
            "invoice was not signed by the trusted invoice issuer (edited or fake link)".into(),
        );
    }
    let v: Value = near_sdk::serde_json::from_str(&inv.payload)
        .map_err(|_| "invoice payload is not valid JSON".to_string())?;
    let s = |k: &str| -> Result<String, String> {
        v.get(k)
            .and_then(Value::as_str)
            .filter(|x| !x.is_empty())
            .map(str::to_string)
            .ok_or_else(|| format!("invoice is missing {k}"))
    };
    if v.get("v").and_then(Value::as_u64) != Some(1) {
        return Err("unsupported invoice version".into());
    }
    Ok(Terms {
        id: s("id")?,
        vendor: s("vendor")?,
        payout: s("payout")?,
        asset: s("asset")?,
        amount: s("amount")?,
        expires_at: v
            .get("expires_at")
            .and_then(Value::as_u64)
            .ok_or("invoice is missing expires_at")?,
    })
}
