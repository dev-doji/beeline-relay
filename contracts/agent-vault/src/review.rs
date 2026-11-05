//! AI-reviewer co-signature, verified on-chain.
//!
//! Payments at or above the owner's threshold need an approval from a reviewer model served by
//! **NEAR AI Cloud** inside a TEE. NEAR AI signs every completion with a per-model ed25519 key
//! (`GET /v1/signature/{chat_id}?signing_algo=ed25519`). For `provider_tee` signatures the signed
//! text is
//!
//! ```text
//! {model_id}:{hex(sha256(exact request body))}:{hex(sha256(exact response body))}
//! ```
//!
//! (docs.near.ai/cloud/verification/reference/response-signature-verification). The agent
//! submits the exact request and response bytes plus the signature. The vault recomputes the
//! text itself, so it can check *what was asked* and *what was answered*, not just that some
//! answer was signed:
//!
//! 1. the signature is from the reviewer key the owner registered;
//! 2. the request used the registered model and the owner's exact policy prompt (by SHA-256),
//!    so the agent cannot swap in a friendlier prompt;
//! 3. the reviewer was shown the exact signed quote being paid;
//! 4. the answer is fresh and its final line is exactly `APPROVE <quote hash>`.
//!
//! Limitation, stated plainly: the vault does not verify NEAR AI's TEE attestation itself. The
//! owner verifies the attestation report off-chain (Intel TDX + NVIDIA evidence, whose
//! `signing_address` must equal the key) before calling `set_reviewer`.

use near_sdk::near;
use near_sdk::serde_json::Value;

/// The evidence an agent submits with a payment that needs review.
#[near(serializers = [json])]
#[derive(Clone, Debug)]
pub struct Review {
    /// Exact bytes of the chat-completion request body sent to NEAR AI Cloud (non-streaming).
    pub request: String,
    /// Exact bytes of the response body NEAR AI Cloud returned.
    pub response: String,
    /// Hex ed25519 signature from `GET /v1/signature/{chat_id}?signing_algo=ed25519`.
    pub signature: String,
}

/// The reviewer the owner trusts.
#[near(serializers = [borsh])]
#[derive(Clone, Debug)]
pub struct Reviewer {
    pub model: String,
    pub signing_key: [u8; 32],
    pub policy_prompt_sha256: [u8; 32],
    /// Payments of at least this amount (token base units) need an approval.
    pub threshold: u128,
}

/// What an approval proves, for the event log.
pub struct Approval {
    pub request_sha256: String,
    pub response_sha256: String,
}

pub fn to_hex(bytes: &[u8]) -> String {
    const DIGITS: &[u8; 16] = b"0123456789abcdef";
    let mut s = String::with_capacity(bytes.len() * 2);
    for b in bytes {
        s.push(DIGITS[(b >> 4) as usize] as char);
        s.push(DIGITS[(b & 0x0f) as usize] as char);
    }
    s
}

pub fn from_hex<const N: usize>(value: &str) -> Result<[u8; N], String> {
    let v = value.strip_prefix("0x").unwrap_or(value);
    if v.len() != N * 2 {
        return Err(format!("expected {N} hex bytes"));
    }
    let mut out = [0u8; N];
    for (i, chunk) in v.as_bytes().chunks(2).enumerate() {
        let pair = std::str::from_utf8(chunk).map_err(|_| "invalid hex".to_string())?;
        out[i] = u8::from_str_radix(pair, 16).map_err(|_| "invalid hex".to_string())?;
    }
    Ok(out)
}

fn sha256_hex(data: &[u8]) -> String {
    to_hex(&near_sdk::env::sha256(data))
}

/// The final non-empty line of the reviewer's answer, which carries the verdict.
fn verdict_line(content: &str) -> &str {
    content
        .lines()
        .map(str::trim)
        .rfind(|l| !l.is_empty())
        .unwrap_or("")
}

/// Checks a review against the trusted reviewer, the quote being paid and the current time.
pub fn verify(
    review: &Review,
    reviewer: &Reviewer,
    quote_json: &str,
    quote_hash: &str,
    now_secs: u64,
    max_age_secs: u64,
) -> Result<Approval, String> {
    // 1. Signature over the text NEAR AI signs, rebuilt from the exact bytes.
    let signature = from_hex::<64>(&review.signature)
        .map_err(|_| "AI review signature must be 64 hex-encoded bytes".to_string())?;
    let request_sha256 = sha256_hex(review.request.as_bytes());
    let response_sha256 = sha256_hex(review.response.as_bytes());
    let text = format!("{}:{request_sha256}:{response_sha256}", reviewer.model);
    if !near_sdk::env::ed25519_verify(&signature, text.as_bytes(), &reviewer.signing_key) {
        return Err("AI review is not signed by the registered NEAR AI reviewer".into());
    }

    // 2–3. What the reviewer was asked.
    let request: Value = near_sdk::serde_json::from_str(&review.request)
        .map_err(|_| "AI review request is not valid JSON".to_string())?;
    if request.get("model").and_then(Value::as_str) != Some(reviewer.model.as_str()) {
        return Err("AI review used a different model".into());
    }
    if request.get("stream").and_then(Value::as_bool) == Some(true) {
        return Err("AI review must be a non-streaming completion".into());
    }
    let messages = request
        .get("messages")
        .and_then(Value::as_array)
        .filter(|m| m.len() == 2)
        .ok_or("AI review request must have exactly a system and a user message")?;
    let content = |i: usize, role: &str| -> Option<&str> {
        let m = &messages[i];
        (m.get("role").and_then(Value::as_str) == Some(role))
            .then(|| m.get("content").and_then(Value::as_str))
            .flatten()
    };
    let policy = content(0, "system").ok_or("AI review request has no system policy prompt")?;
    if near_sdk::env::sha256(policy.as_bytes()) != reviewer.policy_prompt_sha256 {
        return Err("AI review used a different policy prompt than the owner registered".into());
    }
    let task = content(1, "user").ok_or("AI review request has no user message")?;
    if !task.contains(quote_json) {
        return Err("AI reviewer was not shown the quote being paid".into());
    }

    // 4. What the reviewer answered, and when.
    let response: Value = near_sdk::serde_json::from_str(&review.response)
        .map_err(|_| "AI review response is not valid JSON".to_string())?;
    let created = response
        .get("created")
        .and_then(Value::as_u64)
        .ok_or("AI review response has no creation time")?;
    if now_secs.saturating_sub(created) > max_age_secs {
        return Err("AI review is too old; ask the reviewer again".into());
    }
    let answer = response
        .pointer("/choices/0/message/content")
        .and_then(Value::as_str)
        .ok_or("AI review response has no answer")?;
    let verdict = verdict_line(answer);
    if verdict == format!("APPROVE {quote_hash}") {
        return Ok(Approval {
            request_sha256,
            response_sha256,
        });
    }
    if verdict.starts_with("REJECT") {
        let reason: String = verdict.chars().take(160).collect();
        return Err(format!("AI reviewer refused this payment ({reason})"));
    }
    Err("AI reviewer did not approve this quote".into())
}

#[cfg(test)]
mod tests {
    use super::*;
    use near_sdk::test_utils::VMContextBuilder;
    use near_sdk::testing_env;

    /// A review produced by the Go client (beeline-api/internal/nearai) with a test key. The
    /// vault's own verification must accept it, proving the two implementations agree.
    #[test]
    fn accepts_review_built_by_the_go_client() {
        testing_env!(VMContextBuilder::new().build());
        let f: Value =
            near_sdk::serde_json::from_str(include_str!("testdata/go_review.json")).unwrap();
        let s = |k: &str| f[k].as_str().unwrap().to_string();
        let reviewer = Reviewer {
            model: s("model"),
            signing_key: from_hex::<32>(&s("signing_key")).unwrap(),
            policy_prompt_sha256: from_hex::<32>(&s("policy_sha256")).unwrap(),
            threshold: 1,
        };
        let review = Review {
            request: f["review"]["request"].as_str().unwrap().into(),
            response: f["review"]["response"].as_str().unwrap().into(),
            signature: f["review"]["signature"].as_str().unwrap().into(),
        };
        let created = f["created"].as_u64().unwrap();
        let approval = verify(
            &review,
            &reviewer,
            &s("quote_json"),
            &s("quote_hash"),
            created + 60,
            900,
        )
        .expect("the vault must accept a review built by the Go client");
        assert_eq!(approval.request_sha256.len(), 64);

        // The same review cannot approve a different quote.
        let err = verify(
            &review,
            &reviewer,
            &s("quote_json"),
            "OTHER",
            created + 60,
            900,
        )
        .err()
        .unwrap();
        assert!(err.contains("did not approve"), "{err}");
    }

    fn load(raw: &str) -> (Value, Reviewer, Review) {
        let f: Value = near_sdk::serde_json::from_str(raw).unwrap();
        let s = |k: &str| f[k].as_str().unwrap().to_string();
        let reviewer = Reviewer {
            model: s("model"),
            signing_key: from_hex::<32>(&s("signing_key")).unwrap(),
            policy_prompt_sha256: from_hex::<32>(&s("policy_sha256")).unwrap(),
            threshold: 1,
        };
        let review = Review {
            request: f["review"]["request"].as_str().unwrap().into(),
            response: f["review"]["response"].as_str().unwrap().into(),
            signature: f["review"]["signature"].as_str().unwrap().into(),
        };
        (f, reviewer, review)
    }

    /// A REAL NEAR AI Cloud review (z-ai/glm-5.3-flash, provider_tee ed25519 signature),
    /// captured live on 2026-11-05: the reviewer approved a payment that matches its task.
    #[test]
    fn accepts_live_near_ai_approval() {
        testing_env!(VMContextBuilder::new().build());
        let (f, reviewer, review) = load(include_str!("testdata/live_review_approve.json"));
        let created = f["created"].as_u64().unwrap();
        verify(
            &review,
            &reviewer,
            f["quote_json"].as_str().unwrap(),
            f["quote_hash"].as_str().unwrap(),
            created + 60,
            900,
        )
        .expect("a live NEAR AI approval must pass the vault's checks");

        // Too late: the same approval is refused once it is older than the freshness window.
        let err = verify(
            &review,
            &reviewer,
            f["quote_json"].as_str().unwrap(),
            f["quote_hash"].as_str().unwrap(),
            created + 901,
            900,
        )
        .err()
        .unwrap();
        assert!(err.contains("too old"), "{err}");
    }

    /// A REAL NEAR AI Cloud review captured live on 2026-11-05: a prompt-injected "urgent 5 USDC
    /// prepayment". The signature is genuine, and the vault refuses because the model said REJECT.
    #[test]
    fn live_near_ai_refusal_blocks_the_injected_payment() {
        testing_env!(VMContextBuilder::new().build());
        let (f, reviewer, review) = load(include_str!("testdata/live_review_reject.json"));
        let created = f["created"].as_u64().unwrap();
        let err = verify(
            &review,
            &reviewer,
            f["quote_json"].as_str().unwrap(),
            f["quote_hash"].as_str().unwrap(),
            created + 60,
            900,
        )
        .err()
        .unwrap();
        assert!(err.contains("AI reviewer refused"), "{err}");

        // Flipping the signed REJECT into an APPROVE breaks NEAR AI's signature.
        let mut forged = review.clone();
        let hash = f["quote_hash"].as_str().unwrap();
        let reject_line = forged
            .response
            .split("\\n")
            .last()
            .unwrap()
            .split('"')
            .next()
            .unwrap()
            .to_string();
        forged.response = forged
            .response
            .replace(&reject_line, &format!("APPROVE {hash}"));
        assert_ne!(forged.response, review.response);
        let err = verify(
            &forged,
            &reviewer,
            f["quote_json"].as_str().unwrap(),
            hash,
            created + 60,
            900,
        )
        .err()
        .unwrap();
        assert!(err.contains("not signed by the registered"), "{err}");
    }
}
