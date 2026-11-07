import Link from "next/link";
import { ArrowUpRight, Ban, Check, CircleAlert, MessageSquare } from "lucide-react";
import { type Attempt, LAYER_LABEL, TX_URL, formatUSDC, shortKey } from "@/lib/api";

export function timeAgo(iso: string): string {
  const s = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (s < 60) return `${s}s ago`;
  if (s < 3600) return `${Math.round(s / 60)}m ago`;
  if (s < 86400) return `${Math.round(s / 3600)}h ago`;
  return `${Math.round(s / 86400)}d ago`;
}

const OUTCOME: Record<Attempt["outcome"], { label: string; cls: string; icon: React.ReactNode }> = {
  blocked: { label: "Blocked", cls: "pill-bad", icon: <Ban size={13} aria-hidden /> },
  paid: { label: "Paid", cls: "pill-ok", icon: <Check size={13} aria-hidden /> },
  reply: { label: "Bee replied", cls: "pill-sim", icon: <MessageSquare size={13} aria-hidden /> },
  error: { label: "Didn’t finish", cls: "", icon: <CircleAlert size={13} aria-hidden /> },
};

/** Real attempts on pay links, newest first. `showInvoice` links each row to its invoice (attack log). */
export function AttemptList({ attempts, showInvoice = false, empty }: { attempts: Attempt[]; showInvoice?: boolean; empty?: React.ReactNode }) {
  if (attempts.length === 0) return <div className="att-empty">{empty ?? "No attempts yet."}</div>;
  return (
    <ol className="att-list">
      {attempts.map((a) => {
        const o = OUTCOME[a.outcome] ?? OUTCOME.error;
        const tried = a.recipient && a.amount ? `${formatUSDC(a.amount)} USDC → ${shortKey(a.recipient, 14)}` : null;
        return (
          <li className={`att att-${a.outcome}`} key={a.id}>
            <div className="att-top">
              <span className={`pill ${o.cls}`}>
                {o.icon} {o.label}
              </span>
              <span className="att-kind">{a.kind === "attack" ? "Attack" : "Honest payment"}</span>
              {showInvoice && (
                <Link className="att-inv" href={`/pay/${a.invoice_id}`}>
                  {a.vendor_name ?? "invoice"} · {shortKey(a.invoice_id, 5)}
                </Link>
              )}
              <span className="att-time">{timeAgo(a.created_at)}</span>
            </div>
            {a.message && <p className="att-msg">“{a.message.length > 280 ? a.message.slice(0, 280) + "…" : a.message}”</p>}
            {(a.edited_payout || a.edited_amount) && (
              <p className="att-edit">
                Edited the link:
                {a.edited_payout && <> payee → <code>{shortKey(a.edited_payout, 14)}</code></>}
                {a.edited_amount && <> amount → <code>{formatUSDC(a.edited_amount)} USDC</code></>}
              </p>
            )}
            <div className="att-foot">
              {tried && <span>Agent tried: <code>{tried}</code></span>}
              {a.outcome === "blocked" && <span className="att-layer">Stopped by: {LAYER_LABEL[a.layer] ?? a.layer}</span>}
              {a.tx_hash && (
                <a href={TX_URL + a.tx_hash} target="_blank" rel="noreferrer">
                  {shortKey(a.tx_hash, 5)} <ArrowUpRight size={12} aria-hidden />
                </a>
              )}
            </div>
          </li>
        );
      })}
    </ol>
  );
}
