"use client";

import { useEffect, useRef, useState } from "react";
import { ArrowUpRight, Check, Fingerprint, KeyRound, Landmark, X } from "lucide-react";
import type { PlaygroundState, RunResult } from "@/lib/api";
import { LAYER_LABEL, formatUSDC, shortKey } from "@/lib/api";
import { RoleIcon, roleOf } from "@/components/chat/cards";

const ROLE_TEXT: Record<string, string> = {
  approved: "Approved supplier",
  vendor: "Vendor · paid via signed invoices",
  attacker: "Never approved",
};

/** Live balances of the vault and the demo accounts, read from NEAR testnet after every run. */
export function Ledger({ state, runs, refreshing }: { state: PlaygroundState | null; runs: RunResult[]; refreshing: boolean }) {
  const prev = useRef<Record<string, string>>({});
  const [deltas, setDeltas] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!state) return;
    const now: Record<string, string> = { vault: state.balances?.pool ?? "" };
    for (const a of state.accounts) now[a.account] = a.balance;
    const changed: Record<string, string> = {};
    for (const [k, v] of Object.entries(now)) {
      const before = prev.current[k];
      if (before && v && before !== v && /^\d+$/.test(before) && /^\d+$/.test(v)) {
        const d = BigInt(v) - BigInt(before);
        changed[k] = (d > 0n ? "+" : "−") + formatUSDC((d > 0n ? d : -d).toString());
      }
    }
    prev.current = now;
    if (Object.keys(changed).length) {
      setDeltas(changed);
      const t = setTimeout(() => setDeltas({}), 4000);
      return () => clearTimeout(t);
    }
  }, [state]);

  if (!state) {
    return (
      <aside className="ledger" aria-busy>
        <div className="ledger-head">
          <b>Live on NEAR testnet</b>
        </div>
        {[0, 1, 2, 3].map((i) => (
          <div className="acct skeleton-row" key={i} />
        ))}
      </aside>
    );
  }

  const reviewerOn = !!state.reviewer;
  return (
    <aside className="ledger" aria-label="Live balances on NEAR testnet">
      <div className="ledger-head">
        <b>
          <span className="live-dot" aria-hidden /> Live on NEAR testnet
        </b>
        <span className="faint" style={{ fontSize: 12.5 }}>
          {refreshing ? "reading the chain…" : "balances read from the chain"}
        </span>
      </div>

      <a className={`acct vault ${deltas.vault ? "changed" : ""}`} href={state.vault_url} target="_blank" rel="noreferrer">
        <span className="acct-ic vault-ic" aria-hidden>
          <Landmark size={18} />
        </span>
        <span className="acct-main">
          <b>Your vault</b>
          <span className="mono">{shortKey(state.vault, 12)}</span>
        </span>
        <span className="acct-bal">
          {formatUSDC(state.balances?.pool)}
          <small>USDC</small>
          {deltas.vault && <span className="delta down">{deltas.vault}</span>}
        </span>
      </a>
      <div className="vault-rules">
        <span className="pill">
          <KeyRound size={12} aria-hidden /> Bee’s key: pay only
        </span>
        <span className="pill">≤ {formatUSDC(state.config?.max_per_payment)} USDC each</span>
        <span className={`pill ${reviewerOn ? "pill-honey" : ""}`}>
          <Fingerprint size={12} aria-hidden /> AI review {reviewerOn ? "on" : "off"}
        </span>
      </div>

      {state.accounts.map((a) => (
        <a key={a.account} className={`acct role-${a.role} ${deltas[a.account] ? "changed" : ""}`} href={a.url} target="_blank" rel="noreferrer">
          <span className="acct-ic" aria-hidden>
            <RoleIcon role={a.role} size={18} />
          </span>
          <span className="acct-main">
            <b>{a.label}</b>
            <span className="mono">{shortKey(a.account, 12)}</span>
            <span className="acct-role">{ROLE_TEXT[a.role]}</span>
          </span>
          <span className="acct-bal">
            {a.balance ? formatUSDC(a.balance) : "—"}
            <small>USDC</small>
            {deltas[a.account] && <span className="delta up">{deltas[a.account]}</span>}
          </span>
        </a>
      ))}

      <div className="feed">
        <div className="feed-head">On-chain decisions</div>
        {runs.length === 0 && <p className="faint" style={{ margin: 0, fontSize: 13.5 }}>Every payment Bee attempts shows up here with its transaction.</p>}
        {runs.slice(0, 6).map((r, i) => {
          const who = r.recipient ? roleOf(r.recipient, state).label : "";
          return (
            <div className={`feed-row ${r.outcome}`} key={r.started_at + i}>
              <span className="feed-ic" aria-hidden>
                {r.outcome === "paid" ? <Check size={13} /> : <X size={13} />}
              </span>
              <span className="feed-txt">
                {r.outcome === "paid"
                  ? `Paid ${who} ${formatUSDC(r.amount)}`
                  : r.outcome === "blocked"
                    ? `Blocked · ${LAYER_LABEL[r.layer ?? ""] ?? "vault"}`
                    : r.outcome === "reply"
                      ? "No payment"
                      : "Not finished"}
              </span>
              {r.explorer_url && (
                <a href={r.explorer_url} target="_blank" rel="noreferrer" aria-label="View transaction">
                  <ArrowUpRight size={14} />
                </a>
              )}
            </div>
          );
        })}
      </div>
    </aside>
  );
}
