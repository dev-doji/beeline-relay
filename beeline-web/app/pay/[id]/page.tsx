"use client";

import { use, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { ArrowUpRight, Bot, Coins, FlaskConical, Repeat, Replace } from "lucide-react";
import { Receipt } from "@/components/invoice/Receipt";
import { BotFrame, Thinking, Typewriter } from "@/components/chat/Chat";
import { Outcome, QuoteCard, ReviewCard, VaultCard } from "@/components/chat/cards";
import {
  ApiError,
  type Invoice,
  type PlaygroundState,
  type RunResult,
  type Tamper,
  formatUSDC,
  getInvoice,
  getPlayground,
  payInvoice,
  shortKey,
} from "@/lib/api";

interface Attempt {
  tamper: Tamper;
  label: string;
  result?: RunResult;
  error?: string;
}

const TAMPERS: { id: Tamper; label: string; short: string; icon: React.ReactNode; hint: string }[] = [
  { id: "swap_payout", label: "Pay a copy with the scammer’s wallet", short: "Swapped payout address", icon: <Replace size={16} />, hint: "A scammer forwards your link, but with their own address in it." },
  { id: "edit_amount", label: "Pay a copy with 10× the amount", short: "Inflated amount", icon: <Coins size={16} />, hint: "The amount was changed after the vendor created the invoice." },
];

export default function PayPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [invoice, setInvoice] = useState<Invoice | null>(null);
  const [state, setState] = useState<PlaygroundState | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [current, setCurrent] = useState<Attempt | null>(null);
  const [busy, setBusy] = useState(false);
  const [history, setHistory] = useState<Attempt[]>([]);

  const load = useCallback(async () => {
    try {
      const [inv, st] = await Promise.all([getInvoice(id), getPlayground().catch(() => null)]);
      setInvoice(inv);
      setState(st);
    } catch (e) {
      setLoadError(e instanceof ApiError ? e.message : "Could not load this invoice.");
    }
  }, [id]);

  useEffect(() => {
    void load();
  }, [load]);

  async function attempt(tamper: Tamper, label: string) {
    if (!invoice || busy) return;
    setBusy(true);
    const a: Attempt = { tamper, label };
    setCurrent(a);
    try {
      const res = await payInvoice(invoice.id, tamper);
      setInvoice(res.invoice);
      const done = { ...a, result: res.run };
      setCurrent(done);
      setHistory((h) => [done, ...h].slice(0, 6));
    } catch (e) {
      const done = { ...a, error: e instanceof ApiError ? e.message : "The payment attempt failed." };
      setCurrent(done);
    } finally {
      setBusy(false);
      document.getElementById("attempt")?.scrollIntoView({ behavior: "smooth", block: "start" });
    }
  }

  if (loadError) {
    return (
      <div className="pay-wrap">
        <div className="alert" role="alert">
          {loadError}
        </div>
        <Link className="btn" href="/vendor">
          Create an invoice
        </Link>
      </div>
    );
  }
  if (!invoice) {
    return (
      <div className="pay-wrap" aria-busy>
        <div className="thinking">
          <div className="think-line">
            <span className="spinner" aria-hidden /> Loading invoice…
          </div>
        </div>
      </div>
    );
  }

  const paid = invoice.status === "paid";
  return (
    <div className="pay-wrap">
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
        <span className="pill pill-honey">Pay link · NEAR testnet</span>
        <Link href="/vendor" className="pill">
          Create your own invoice
        </Link>
      </div>

      <Receipt
        data={{
          id: invoice.id,
          vendor_name: invoice.vendor_name,
          vendor_handle: invoice.vendor_handle,
          memo: invoice.memo,
          items: invoice.items,
          amount: invoice.amount,
          payout: invoice.payout,
          status: invoice.status,
          expires_at: invoice.expires_at,
          signed: true,
        }}
      />

      {paid && invoice.paid_tx && (
        <a className="btn" href={`https://testnet.nearblocks.io/txns/${invoice.paid_tx}`} target="_blank" rel="noreferrer">
          Paid on NEAR testnet · {shortKey(invoice.paid_tx, 6)} <ArrowUpRight size={16} aria-hidden />
        </a>
      )}

      {!paid && (
        <button className="btn btn-honey" style={{ minHeight: 52, fontSize: 16 }} onClick={() => void attempt("none", "Genuine invoice")} disabled={busy}>
          <Bot size={18} aria-hidden /> Pay {formatUSDC(invoice.amount)} USDC with Bee (AI agent)
        </button>
      )}

      <div className="lab">
        <h3>
          <FlaskConical size={18} color="var(--bad)" aria-hidden /> Fraud lab: try to cheat this vendor
        </h3>
        <p>Each button sends a real testnet transaction. The vault should refuse every one of them.</p>
        <div className="suggestions">
          {TAMPERS.map((t) => (
            <button key={t.id} className="suggestion danger" onClick={() => void attempt(t.id, t.short)} disabled={busy} title={t.hint}>
              {t.icon}
              {t.label}
            </button>
          ))}
          {paid && (
            <button className="suggestion danger" onClick={() => void attempt("none", "Paid twice")} disabled={busy}>
              <Repeat size={16} /> Pay this invoice again
            </button>
          )}
        </div>
      </div>

      <div id="attempt" style={{ display: "grid", gap: 10, scrollMarginTop: 20 }}>
        {current && !current.result && !current.error && (
          <Thinking summary={`${current.label}: ${formatUSDC(invoice.amount)} USDC → ${invoice.vendor_name}`} reviewExpected={!!state?.reviewer} />
        )}
        {current?.error && (
          <BotFrame>
            <div className="bubble">{current.error}</div>
          </BotFrame>
        )}
        {current?.result && <AttemptView attempt={current} invoice={invoice} state={state} />}
      </div>

      {history.length > 1 && (
        <div className="attempt-list" aria-label="Your attempts">
          {history.map((h, i) => (
            <div className="attempt" key={i}>
              <span>{h.label}</span>
              <span className={`pill ${h.result?.outcome === "paid" ? "pill-ok" : "pill-bad"}`}>{h.result?.outcome ?? "error"}</span>
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function AttemptView({ attempt, invoice, state }: { attempt: Attempt; invoice: Invoice; state: PlaygroundState | null }) {
  const r = attempt.result!;
  const quote = r.steps.find((s) => s.key === "quote");
  const review = r.steps.find((s) => s.key === "review");
  const scam = attempt.tamper !== "none";
  const text =
    r.outcome === "paid"
      ? `Paid. The vault verified ${invoice.vendor_name}’s signed invoice, the AI reviewer approved, and the money went to the vendor’s wallet only.`
      : scam
        ? `Caught it. This copy of the link was edited (${attempt.label.toLowerCase()}), so the invoice signature no longer matches and the vault refused on-chain. ${invoice.vendor_name} keeps their money safe.`
        : r.outcome === "blocked"
          ? `The vault refused: ${r.reason}. No money moved.`
          : `This attempt didn’t finish: ${r.reason}.`;
  return (
    <BotFrame>
      <div className="bubble">
        <Typewriter text={text} />
      </div>
      {quote && <QuoteCard step={quote} state={state} delay={200} payeeLabel={attempt.tamper === "swap_payout" ? "Scammer’s wallet" : invoice.vendor_name} />}
      {review && <ReviewCard step={review} delay={450} />}
      <VaultCard result={r} reviewStep={review} delay={700} invoiceMode />
      <Outcome result={r} delay={1500} />
    </BotFrame>
  );
}
