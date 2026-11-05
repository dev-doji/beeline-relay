//! Reproduces 1Click's quote signing scheme so the vault can check, on-chain, that a deposit
//! address really comes from a quote 1Click signed.
//!
//! Scheme (from the official SDK, defuse-protocol/one-click-sdk-typescript,
//! src/quote-signature.ts): message = base58(sha256(stableStringify(
//! {...signedRequest, ...signedQuote, timestamp}))), signed with ed25519. The vault verifies
//! the signature over the UTF-8 bytes of that base58 string. The Go API implements the same
//! scheme in beeline-api/internal/intents/signature.go; both are tested against a live quote.

use near_sdk::serde_json::{Map, Value};
use std::collections::BTreeMap;

const SIGNED_REQUEST_FIELDS: &[&str] = &[
    "dry",
    "swapType",
    "slippageTolerance",
    "originAsset",
    "depositType",
    "destinationAsset",
    "amount",
    "refundTo",
    "refundType",
    "recipient",
    "recipientType",
    "deadline",
];
const OPTIONAL_REQUEST_FIELDS: &[&str] = &[
    "quoteWaitingTimeMs",
    "referral",
    "virtualChainRecipient",
    "virtualChainRefundRecipient",
    "customRecipientMsg",
];
const SIGNED_QUOTE_FIELDS: &[&str] = &[
    "amountIn",
    "amountInFormatted",
    "amountInUsd",
    "minAmountIn",
    "amountOut",
    "amountOutFormatted",
    "amountOutUsd",
    "minAmountOut",
];
const OPTIONAL_QUOTE_FIELDS: &[&str] = &[
    "depositAddress",
    "depositMemo",
    "deadline",
    "timeWhenInactive",
    "timeEstimate",
    "refundFee",
    "withdrawFee",
];

/// The fields of a signed quote the vault makes decisions on.
#[derive(Debug, Clone, PartialEq)]
pub struct SignedQuote {
    pub dry: bool,
    pub swap_type: String,
    pub origin_asset: String,
    pub deposit_type: String,
    pub destination_asset: String,
    pub amount: String,
    pub refund_to: String,
    pub refund_type: String,
    pub recipient: String,
    pub recipient_type: String,
    pub amount_in: String,
    pub deposit_address: String,
    pub has_deposit_memo: bool,
    /// Provider deadline (quote.deadline, which overrides the request deadline when present).
    pub deadline: String,
    /// When 1Click issued the quote.
    pub timestamp: String,
    pub signature: String,
    /// The base58 hash that the signature covers.
    pub hash: String,
}

/// JavaScript truthiness for JSON values (the SDK maps falsy optional fields to undefined).
fn truthy(v: &Value) -> bool {
    match v {
        Value::Null => false,
        Value::Bool(b) => *b,
        Value::String(s) => !s.is_empty(),
        Value::Number(n) => n.as_f64().is_none_or(|f| f != 0.0),
        _ => true,
    }
}

fn str_field(obj: &Map<String, Value>, key: &str) -> String {
    obj.get(key)
        .and_then(Value::as_str)
        .unwrap_or_default()
        .to_string()
}

/// Parses a raw 1Click quote response and computes the hash 1Click signed.
pub fn parse(raw: &str) -> Result<SignedQuote, String> {
    let root: Value =
        near_sdk::serde_json::from_str(raw).map_err(|_| "quote is not valid JSON".to_string())?;
    let req = root
        .get("quoteRequest")
        .and_then(Value::as_object)
        .ok_or("quote is missing quoteRequest")?;
    let quote = root
        .get("quote")
        .and_then(Value::as_object)
        .ok_or("quote is missing quote")?;
    let timestamp = root
        .get("timestamp")
        .and_then(Value::as_str)
        .ok_or("quote is missing timestamp")?
        .to_string();
    let signature = root
        .get("signature")
        .and_then(Value::as_str)
        .ok_or("quote is missing signature")?
        .to_string();
    let dry = req.get("dry").and_then(Value::as_bool).unwrap_or(false);

    // A BTreeMap serializes with sorted keys, like json-stable-stringify. serde_json's own Map
    // cannot be relied on: it keeps insertion order when any dependency enables preserve_order.
    let mut payload: BTreeMap<String, Value> = BTreeMap::new();
    for k in SIGNED_REQUEST_FIELDS {
        if let Some(v) = req.get(*k).filter(|v| !v.is_null()) {
            payload.insert((*k).to_string(), v.clone());
        }
    }
    for k in OPTIONAL_REQUEST_FIELDS {
        if let Some(v) = req.get(*k).filter(|v| truthy(v)) {
            payload.insert((*k).to_string(), v.clone());
        }
    }
    for k in SIGNED_QUOTE_FIELDS {
        if let Some(v) = quote.get(*k).filter(|v| !v.is_null()) {
            payload.insert((*k).to_string(), v.clone());
        }
    }
    if !dry {
        for k in OPTIONAL_QUOTE_FIELDS {
            if let Some(v) = quote.get(*k).filter(|v| truthy(v)) {
                payload.insert((*k).to_string(), v.clone());
            }
        }
    }
    payload.insert("timestamp".to_string(), Value::String(timestamp.clone()));

    // Payload values are scalars, so sorting the top level is sufficient.
    let canonical = near_sdk::serde_json::to_string(&payload)
        .map_err(|_| "could not serialize quote".to_string())?;
    let digest = near_sdk::env::sha256(canonical.as_bytes());
    let hash = bs58::encode(digest).into_string();

    Ok(SignedQuote {
        dry,
        swap_type: str_field(req, "swapType"),
        origin_asset: str_field(req, "originAsset"),
        deposit_type: str_field(req, "depositType"),
        destination_asset: str_field(req, "destinationAsset"),
        amount: str_field(req, "amount"),
        refund_to: str_field(req, "refundTo"),
        refund_type: str_field(req, "refundType"),
        recipient: str_field(req, "recipient"),
        recipient_type: str_field(req, "recipientType"),
        amount_in: str_field(quote, "amountIn"),
        deposit_address: str_field(quote, "depositAddress"),
        has_deposit_memo: quote.get("depositMemo").map(truthy).unwrap_or(false),
        deadline: payload
            .get("deadline")
            .and_then(Value::as_str)
            .unwrap_or_default()
            .to_string(),
        timestamp,
        signature,
        hash,
    })
}

/// Decodes "ed25519:<base58>" into raw bytes of the expected length.
pub fn decode_ed25519<const N: usize>(value: &str) -> Result<[u8; N], String> {
    let encoded = value.strip_prefix("ed25519:").unwrap_or(value);
    let bytes = bs58::decode(encoded)
        .into_vec()
        .map_err(|_| "invalid base58".to_string())?;
    bytes
        .try_into()
        .map_err(|_| "wrong key or signature length".to_string())
}

/// Verifies that `quote` was signed by `signer` (32-byte ed25519 public key).
pub fn verify(quote: &SignedQuote, signer: &[u8; 32]) -> bool {
    match decode_ed25519::<64>(&quote.signature) {
        Ok(sig) => near_sdk::env::ed25519_verify(&sig, quote.hash.as_bytes(), signer),
        Err(_) => false,
    }
}

/// Parses an RFC 3339 UTC timestamp ("2026-11-07T13:32:05.506Z") into Unix nanoseconds.
/// Only the "Z" form is accepted; 1Click always returns UTC.
pub fn parse_rfc3339_utc(s: &str) -> Option<u64> {
    let b = s.as_bytes();
    if b.len() < 20
        || b[4] != b'-'
        || b[7] != b'-'
        || b[10] != b'T'
        || b[13] != b':'
        || b[16] != b':'
    {
        return None;
    }
    if *b.last()? != b'Z' {
        return None;
    }
    let num = |r: std::ops::Range<usize>| -> Option<u64> { s.get(r)?.parse().ok() };
    let (year, month, day) = (num(0..4)? as i64, num(5..7)?, num(8..10)?);
    let (hour, min, sec) = (num(11..13)?, num(14..16)?, num(17..19)?);
    if !(1..=12).contains(&month) || !(1..=31).contains(&day) || hour > 23 || min > 59 || sec > 60 {
        return None;
    }
    let mut nanos = 0u64;
    let rest = &s[19..s.len() - 1];
    if let Some(frac) = rest.strip_prefix('.') {
        if frac.is_empty() || frac.len() > 9 || !frac.bytes().all(|c| c.is_ascii_digit()) {
            return None;
        }
        nanos = frac.parse::<u64>().ok()? * 10u64.pow(9 - frac.len() as u32);
    } else if !rest.is_empty() {
        return None;
    }
    // Days from civil date (Howard Hinnant's algorithm).
    let y = if month <= 2 { year - 1 } else { year };
    let era = if y >= 0 { y } else { y - 399 } / 400;
    let yoe = y - era * 400;
    let m = month as i64;
    let doy = (153 * (if m > 2 { m - 3 } else { m + 9 }) + 2) / 5 + day as i64 - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    let days = era * 146_097 + doe - 719_468;
    if days < 0 {
        return None;
    }
    let secs = days as u64 * 86_400 + hour * 3_600 + min * 60 + sec;
    secs.checked_mul(1_000_000_000)?.checked_add(nanos)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Real 1Click quote fetched on 2026-11-04 (no funds were sent). Same fixture as the Go test.
    pub const LIVE_QUOTE: &str = r#"{"quote": {"amountIn": "1000000", "deadline": "2026-11-07T13:32:05.506Z", "amountOut": "997398", "refundFee": "0", "amountInUsd": "0.999515000000", "minAmountIn": "1000000", "withdrawFee": "2400", "amountOutUsd": "0.996914261970", "minAmountOut": "987424", "timeEstimate": 22, "depositAddress": "b2a200b2bdabdbca8544ea2847e2e4af24779ec274a89be2d7e53d187758991b", "timeWhenInactive": "2026-11-07T13:32:05.506Z", "amountInFormatted": "1.0", "amountOutFormatted": "0.997398"}, "signature": "ed25519:1Kf341fWaooBVUWYfxt8RiTq6WWpGV11cVSr8nFTznG7mUQNb9zdqdzC6y54hM4jMP8Kv4GUhkAaUiw8o3eDs7p", "timestamp": "2026-11-04T13:22:33.625Z", "quoteRequest": {"dry": false, "amount": "1000000", "appFees": [{"fee": 1, "recipient": "5880ad2b362620fadf759cbceb1cd5737ce8c6ed7fb8e9942881e6731f9247dd", "limitOrderId": null}], "insured": false, "deadline": "2026-11-04T13:32:05.506Z", "refundTo": "intents.near", "swapType": "EXACT_INPUT", "recipient": "0x2222222222222222222222222222222222222222", "refundType": "ORIGIN_CHAIN", "depositMode": "SIMPLE", "depositType": "ORIGIN_CHAIN", "originAsset": "nep141:17208628f84f5d6ad33f0da3bbbeb27ffcb398eac501a31bd6ad2011e36133a1", "recipientType": "DESTINATION_CHAIN", "confidentiality": "public", "destinationAsset": "nep141:base-0x833589fcd6edb6e08f4c7c32d4f71b54bda02913.omft.near", "slippageTolerance": 100, "quoteWaitingTimeMs": 0}, "correlationId": "ccc4674d-f500-4a92-b5c9-61cb05643390"}"#;

    pub const ONE_CLICK_KEY: &str = "ed25519:reYaWhvwu8Jzo3WUM3zhn6VrhuMEF4eADL17qtRVifc";

    /// A real INTENTS-deposit quote (NEAR USDC via the Intents verifier to Base), fetched live on
    /// 2026-11-05 after 1Click stopped quoting NEAR ORIGIN_CHAIN deposits.
    #[test]
    fn live_intents_quote_signature_verifies() {
        let raw = include_str!("testdata/live_intents_quote.json");
        let q = parse(raw).unwrap();
        let key = decode_ed25519::<32>(ONE_CLICK_KEY).unwrap();
        assert!(verify(&q, &key), "real 1Click INTENTS quote must verify");
        assert_eq!(q.deposit_type, "INTENTS");
        assert_eq!(q.refund_type, "INTENTS");
        assert_eq!(q.deposit_address.len(), 64);
        let tampered = raw.replace(&q.deposit_address, &"a".repeat(64));
        assert!(!verify(&parse(&tampered).unwrap(), &key));
    }

    #[test]
    fn live_quote_signature_verifies() {
        let q = parse(LIVE_QUOTE).unwrap();
        let key = decode_ed25519::<32>(ONE_CLICK_KEY).unwrap();
        assert!(
            verify(&q, &key),
            "real 1Click signature must verify on-chain"
        );
        assert_eq!(
            q.deposit_address,
            "b2a200b2bdabdbca8544ea2847e2e4af24779ec274a89be2d7e53d187758991b"
        );
        assert_eq!(
            q.deadline, "2026-11-07T13:32:05.506Z",
            "quote deadline overrides request deadline"
        );
        assert_eq!(q.recipient, "0x2222222222222222222222222222222222222222");
    }

    #[test]
    fn tampered_live_quote_fails() {
        let key = decode_ed25519::<32>(ONE_CLICK_KEY).unwrap();
        for (from, to) in [
            ("b2a200b2bdab", "aaaa00b2bdab"),
            (
                "0x2222222222222222222222222222222222222222",
                "0x3333333333333333333333333333333333333333",
            ),
            (
                r#""refundTo": "intents.near""#,
                r#""refundTo": "attacker.near""#,
            ),
        ] {
            let q = parse(&LIVE_QUOTE.replacen(from, to, 1)).unwrap();
            assert!(
                !verify(&q, &key),
                "tampering {from} must break the signature"
            );
        }
    }

    #[test]
    fn rfc3339() {
        assert_eq!(parse_rfc3339_utc("1970-01-01T00:00:00Z"), Some(0));
        assert_eq!(
            parse_rfc3339_utc("2026-11-04T13:22:33.625Z"),
            Some(1_793_798_553_625_000_000)
        );
        assert_eq!(
            parse_rfc3339_utc("2000-03-01T00:00:00.000000001Z"),
            Some(951_868_800_000_000_001)
        );
        for bad in [
            "",
            "2026-11-04 13:22:33Z",
            "2026-11-04T13:22:33+01:00",
            "2026-13-04T00:00:00Z",
            "2026-11-04T13:22:33.Z",
        ] {
            assert_eq!(parse_rfc3339_utc(bad), None, "{bad}");
        }
    }
}
