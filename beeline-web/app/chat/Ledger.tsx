"use client";

import { useEffect, useState } from "react";
import { type LedgerEntry, type LedgerSummary, formatUSDC, shortKey } from "@/lib/api";

/**
 * The vendor's books: totals and the list of confirmed payments. Every entry was recorded by the server after it
 * verified the payment; test money and real money are never added together.
 */
export function Ledger({ load }: { load: () => Promise<{ entries: LedgerEntry[]; summary: LedgerSummary }> }) {
  const [data, setData] = useState<{ entries: LedgerEntry[]; summary: LedgerSummary } | null>(null);
  useEffect(() => {
    let on = true;
    const run = () => load().then((d) => on && setData(d)).catch(() => undefined);
    void run();
    const t = setInterval(run, 20000);
    return () => {
      on = false;
      clearInterval(t);
    };
  }, [load]);
  if (!data) return null;
  const s = data.summary;
  const realOn = s.collected_real !== "0";
  return (
    <section className="cx-ledger" aria-label="Your books">
      <div className="cx-stats">
        <div className="cx-stat">
          <span>Collected{realOn ? " (real)" : " (test)"}</span>
          <b>{formatUSDC(realOn ? s.collected_real : s.collected_test)} USDC</b>
          {realOn && s.collected_test !== "0" && <em>plus {formatUSDC(s.collected_test)} test USDC</em>}
        </div>
        <div className="cx-stat">
          <span>Paid</span>
          <b>{s.paid_count}</b>
          <em>{s.open_count} waiting</em>
        </div>
        <div className="cx-stat">
          <span>Attempts refused</span>
          <b>{s.refused_claims}</b>
          <em>by the server</em>
        </div>
        <div className="cx-stat">
          <span>Guard agent</span>
          <b>{s.guard_flagged}</b>
          <em>
            flagged of {s.guard_reviews} reviewed{s.guard_fraud > 0 ? `, ${s.guard_fraud} as fraud` : ""}
          </em>
        </div>
      </div>
      {data.entries.length > 0 && (
        <div className="cx-ledger-list">
          <b className="cx-sec">Ledger</b>
          {data.entries.slice(0, 12).map((e) => (
            <div key={e.tx_hash} className="cx-ledger-row">
              <span>{new Date(e.paid_at).toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" })}</span>
              <span className="cx-muted">
                {e.payer.replace("cross-chain:", "from ")} · {e.verification === "intents-delivered" ? "NEAR Intents" : "on-chain"} {e.network === "mainnet" ? "· real" : "· test"}
              </span>
              <b>{formatUSDC(e.amount)} USDC</b>
              {!e.tx_hash.startsWith("intents:") && <span className="cx-muted">{shortKey(e.tx_hash, 4)}</span>}
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
