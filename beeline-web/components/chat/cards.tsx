"use client";

import {
  ArrowUpRight,
  Ban,
  Bot,
  Check,
  CircleDollarSign,
  Cpu,
  FileSignature,
  Fingerprint,
  ListChecks,
  Lock,
  Minus,
  Receipt,
  Repeat,
  ShieldCheck,
  ShieldX,
  Wallet,
  X,
} from "lucide-react";
import type { PlaygroundState, RunResult, Step } from "@/lib/api";
import { formatUSDC, shortKey } from "@/lib/api";

/* ---------------- Quote receipt ---------------- */

export function QuoteCard({ step, state, delay = 0, payeeLabel }: { step: Step; state: PlaygroundState | null; delay?: number; payeeLabel?: string }) {
  const d = step.data ?? {};
  const recipient = String(d.recipient ?? "");
  const amount = String(d.amount ?? "");
  const allowed = payeeLabel ? undefined : state?.recipients?.includes(recipient);
  const forged = step.detail.includes("throwaway key");
  return (
    <div className="rcard" style={{ animationDelay: `${delay}ms` }}>
      <div className="rcard-head">
        <span className="t">
          <Receipt size={16} aria-hidden /> Payment quote
        </span>
        <span className="pill pill-sim" title="1Click has no testnet, so the quote is signed by the vault's test signer">
          Simulated quote
        </span>
      </div>
      <div className="rcard-body">
        <div className="amount">
          {formatUSDC(amount)}
          <small>test USDC</small>
        </div>
        <div className="route" style={{ marginTop: 14 }}>
          <div className="party">
            <small>From</small>
            <div>Agent vault</div>
            <div className="mono">{state ? shortKey(state.vault, 10) : "vault"}</div>
          </div>
          <div className="flow" aria-hidden>
            <span className="coin">
              <CircleDollarSign size={14} />
            </span>
          </div>
          <div className="party">
            <small>To (on Base)</small>
            <div>{payeeLabel ?? (allowed ? state?.allowed_recipient.label ?? "Approved recipient" : recipient === state?.attacker_recipient.address ? "Unknown address" : "Recipient")}</div>
            <div className="mono">{shortKey(recipient, 6)}</div>
          </div>
        </div>
        <div className="receipt-meta">
          {payeeLabel ? (
            <span className={`pill ${payeeLabel.startsWith("Scammer") ? "pill-bad" : "pill-honey"}`}>
              <Receipt size={13} aria-hidden /> Paying a vendor invoice
            </span>
          ) : allowed ? (
            <span className="pill pill-ok">
              <Check size={13} aria-hidden /> On your approved list
            </span>
          ) : (
            <span className="pill pill-bad">
              <X size={13} aria-hidden /> Not on your approved list
            </span>
          )}
          {forged ? (
            <span className="pill pill-bad">
              <FileSignature size={13} aria-hidden /> Signed by an unknown key
            </span>
          ) : (
            <span className="pill">
              <FileSignature size={13} aria-hidden /> Signed quote
            </span>
          )}
          <span className="pill">Route: NEAR → Base via NEAR Intents</span>
        </div>
      </div>
    </div>
  );
}

/* ---------------- AI reviewer ---------------- */

export function ReviewCard({ step, delay = 0 }: { step: Step; delay?: number }) {
  const d = step.data ?? {};
  const answer = typeof d.answer === "string" ? d.answer : "";
  const approved = d.approved === true;
  const verified = d.signature_verified === true;

  if (step.status === "skipped") {
    return (
      <div className="rcard" style={{ animationDelay: `${delay}ms` }}>
        <div className="rcard-head">
          <span className="t">
            <Cpu size={16} aria-hidden /> AI reviewer
          </span>
          <span className="pill">Not needed</span>
        </div>
        <div className="rcard-body muted">{step.detail}</div>
      </div>
    );
  }
  if (step.status === "error") {
    return (
      <div className="rcard" style={{ animationDelay: `${delay}ms` }}>
        <div className="rcard-head">
          <span className="t">
            <Cpu size={16} aria-hidden /> AI reviewer
          </span>
          <span className="pill pill-bad">No review</span>
        </div>
        <div className="rcard-body">{step.detail}</div>
      </div>
    );
  }
  const reasoning = summarize(answer);
  return (
    <div className="rcard" style={{ animationDelay: `${delay}ms` }}>
      <div className="rcard-head">
        <span className="t">
          <Cpu size={16} aria-hidden /> AI reviewer · {step.title.replace(/^AI reviewer \(NEAR AI Cloud, |\)$/g, "")}
        </span>
        <span className="pill pill-real">Real · NEAR AI</span>
      </div>
      <div className="rcard-body">
        <div className="reviewer">
          <div className="model-badge" aria-hidden>
            <Bot size={22} />
          </div>
          <div className="review-quote">
            <span className={`stamp ${approved ? "ok" : "bad"}`}>{approved ? "APPROVED" : "REFUSED"}</span>
            <div className="review-text">“{reasoning}”</div>
          </div>
        </div>
        <div className="receipt-meta">
          <span className={`pill ${verified ? "pill-ok" : "pill-bad"}`}>
            <Fingerprint size={13} aria-hidden /> {verified ? "TEE signature verified" : "Signature not verified"}
          </span>
          {typeof d.signing_key === "string" && <span className="pill mono">key {shortKey(d.signing_key, 5)}</span>}
          <span className="pill">The vault re-checks this signature on-chain</span>
        </div>
        {answer && (
          <details className="more" style={{ marginTop: 10 }}>
            <summary>
              <ListChecks size={15} aria-hidden /> Read the reviewer’s full answer
            </summary>
            <pre className="answer">{answer}</pre>
          </details>
        )}
      </div>
    </div>
  );
}

/** First sentence or two of the reviewer's reasoning, without the final verdict line. */
function summarize(answer: string): string {
  const lines = answer.split("\n").map((l) => l.trim()).filter(Boolean);
  const body = lines.filter((l) => !/^(APPROVE|REJECT)\b/.test(l)).join(" ").replace(/\*\*/g, "");
  const verdict = lines.find((l) => /^(APPROVE|REJECT)\b/.test(l)) ?? "";
  const text = body || verdict;
  return text.length > 220 ? text.slice(0, 217).trimEnd() + "…" : text;
}

/* ---------------- Vault decision ---------------- */

type Rung = { id: string; label: string; icon: React.ReactNode };
const RUNGS: Rung[] = [
  { id: "quote_signature", label: "Genuine signed quote", icon: <FileSignature size={13} /> },
  { id: "allowlist", label: "Recipient is approved", icon: <ListChecks size={13} /> },
  { id: "limit", label: "Within the per-payment limit", icon: <Wallet size={13} /> },
  { id: "single_use", label: "Quote not used before", icon: <Repeat size={13} /> },
  { id: "ai_reviewer", label: "AI reviewer approved", icon: <Cpu size={13} /> },
  { id: "daily", label: "Within today’s budget", icon: <Lock size={13} /> },
];

/** Which rung failed, in the order the contract checks them (contracts/agent-vault/src/lib.rs). */
function failedRung(result: RunResult): string | null {
  if (result.outcome !== "blocked") return null;
  const r = result.reason;
  if (result.layer === "spend_limit") return r.includes("daily budget") ? "daily" : "limit";
  if (result.layer === "invoice") return "allowlist";
  if (result.layer === "vault_policy") return null;
  return result.layer ?? null;
}

export function VaultCard({ result, reviewStep, delay = 0, invoiceMode = false }: { result: RunResult; reviewStep?: Step; delay?: number; invoiceMode?: boolean }) {
  const vault = result.steps.find((s) => s.key === "vault");
  const failed = failedRung(result);
  const failIndex = failed ? RUNGS.findIndex((r) => r.id === failed) : -1;
  const paid = result.outcome === "paid";
  const blocked = result.outcome === "blocked";
  const reviewSkipped = reviewStep?.status === "skipped";

  return (
    <div className="rcard" style={{ animationDelay: `${delay}ms` }}>
      <div className="rcard-head">
        <span className="t">
          <ShieldCheck size={16} aria-hidden /> Vault contract · NEAR testnet
        </span>
        <span className="pill pill-real">Real transaction</span>
      </div>
      <div className="rcard-body">
        <div className="vault-grid">
          <Shield state={paid ? "ok" : blocked ? "bad" : "idle"} />
          <ul className="ladder" aria-label="Guardrails checked by the vault">
            {RUNGS.map((r, i) => {
              let cls = "pass";
              let why = "";
              if (blocked && failIndex >= 0) {
                if (i === failIndex) {
                  cls = "fail";
                  why = "blocked here";
                } else if (i > failIndex) {
                  cls = "skip";
                  why = "not reached";
                }
              } else if (blocked && failIndex < 0) {
                cls = "skip";
              }
              if (r.id === "ai_reviewer" && cls === "pass" && reviewSkipped) {
                cls = "skip";
                why = "not required";
              }
              if (result.outcome === "error") cls = "skip";
              return (
                <li key={r.id} className={cls} style={{ animationDelay: `${delay + 150 + i * 110}ms` }}>
                  <span className="ic" aria-hidden>
                    {cls === "pass" ? <Check size={13} /> : cls === "fail" ? <X size={13} /> : <Minus size={13} />}
                  </span>
                  {invoiceMode && r.id === "allowlist" ? "Genuine invoice: payee and amount match, unpaid" : r.label}
                  {why && <span className="why">{why}</span>}
                </li>
              );
            })}
          </ul>
        </div>
        {vault && blocked && (
          <p className="mono" style={{ margin: "14px 0 0", color: "var(--bad)", overflowWrap: "anywhere" }}>
            BEELINE_VAULT_REJECTED: {result.reason}
          </p>
        )}
      </div>
    </div>
  );
}

function Shield({ state }: { state: "ok" | "bad" | "idle" }) {
  const color = state === "ok" ? "var(--ok)" : state === "bad" ? "var(--bad)" : "var(--faint)";
  return (
    <div className={`shield ${state === "bad" ? "bad" : ""}`} aria-hidden>
      <svg className="base" viewBox="0 0 112 124">
        <defs>
          <linearGradient id={`sg-${state}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0" stopColor={color} stopOpacity="0.28" />
            <stop offset="1" stopColor={color} stopOpacity="0.06" />
          </linearGradient>
        </defs>
        <path d="M56 6 100 22v34c0 30-19 52-44 62C31 108 12 86 12 56V22Z" fill={`url(#sg-${state})`} stroke={color} strokeWidth="2.5" />
      </svg>
      <span className="glyph" style={{ color }}>
        {state === "ok" ? <ShieldCheck size={44} /> : state === "bad" ? <ShieldX size={44} /> : <Lock size={40} />}
      </span>
    </div>
  );
}

/* ---------------- Outcome ---------------- */

const CONFETTI = ["#f5b324", "#34d399", "#7dd3fc", "#ffd36e", "#c4b5fd"];

export function Outcome({ result, delay = 0 }: { result: RunResult; delay?: number }) {
  const amount = (result.steps.find((s) => s.key === "quote")?.data?.amount as string) ?? "";
  const secs = (result.duration_ms / 1000).toFixed(1);
  const kind = result.outcome;
  return (
    <div className={`outcome ${kind}`} style={{ animationDelay: `${delay}ms` }} role="status">
      {kind === "paid" && (
        <div className="confetti" aria-hidden>
          {Array.from({ length: 18 }).map((_, i) => (
            <i
              key={i}
              style={{ left: `${5 + i * 5.2}%`, background: CONFETTI[i % CONFETTI.length], animationDelay: `${delay + 200 + (i % 6) * 70}ms` }}
            />
          ))}
        </div>
      )}
      <span className="big-ic" aria-hidden>
        {kind === "paid" ? <Check size={24} /> : kind === "blocked" ? <Ban size={22} /> : <X size={22} />}
      </span>
      <div style={{ minWidth: 0 }}>
        <h4>{kind === "paid" ? `Paid ${formatUSDC(amount)} test USDC` : kind === "blocked" ? "Blocked on-chain. No money moved." : "The run didn’t finish"}</h4>
        <p>
          {kind === "paid" ? "Every guardrail passed." : kind === "blocked" ? "The vault refused the transaction." : result.reason} · {secs}s
        </p>
      </div>
      {result.explorer_url && (
        <a className="btn tx" href={result.explorer_url} target="_blank" rel="noreferrer">
          View on NearBlocks <ArrowUpRight size={16} aria-hidden />
        </a>
      )}
    </div>
  );
}
