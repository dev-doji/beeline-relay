"use client";

import { use, useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { ArrowUpRight, Bot, Check, Copy, Radar, Send, Skull, Store } from "lucide-react";
import { Receipt } from "@/components/invoice/Receipt";
import { AppFrame } from "@/components/AppFrame";
import { BotFrame, Thinking, Typewriter } from "@/components/chat/Chat";
import { Outcome, PlanCard, ReviewCard, VaultCard } from "@/components/chat/cards";
import { AttemptList } from "@/components/attacks/AttemptList";
import {
  ApiError,
  type Attack,
  type Attempt,
  type Invoice,
  type PlaygroundState,
  type RunResult,
  TX_URL,
  formatUSDC,
  getInvoice,
  getInvoiceAttempts,
  getPlayground,
  payInvoice,
  shortKey,
  toBaseUnits,
} from "@/lib/api";

interface Current {
  attack: Attack;
  result?: RunResult;
  error?: string;
}

const NEAR_ACCOUNT = /^(([a-z\d]+[-_])*[a-z\d]+\.)*([a-z\d]+[-_])*[a-z\d]+$/;

export default function PayPage({ params }: { params: Promise<{ id: string }> }) {
  return (
    <AppFrame active="vendor" title="Pay link" icon={<Store size={16} aria-hidden />}>
      <PayContent params={params} />
    </AppFrame>
  );
}

function PayContent({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [invoice, setInvoice] = useState<Invoice | null>(null);
  const [state, setState] = useState<PlaygroundState | null>(null);
  const [attempts, setAttempts] = useState<Attempt[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [current, setCurrent] = useState<Current | null>(null);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [payout, setPayout] = useState("");
  const [amount, setAmount] = useState("");
  const [copied, setCopied] = useState(false);

  const loadState = useCallback(async () => {
    const st = await getPlayground().catch(() => null);
    if (st) setState(st);
  }, []);
  const loadAttempts = useCallback(async () => {
    const r = await getInvoiceAttempts(id).catch(() => null);
    if (r) setAttempts(r.data);
  }, [id]);

  useEffect(() => {
    (async () => {
      try {
        setInvoice(await getInvoice(id));
        await Promise.all([loadState(), loadAttempts()]);
      } catch (e) {
        setLoadError(e instanceof ApiError ? e.message : "Could not load this invoice.");
      }
    })();
    // Other people may be attacking this link right now.
    const t = setInterval(() => void loadAttempts(), 15000);
    return () => clearInterval(t);
  }, [id, loadState, loadAttempts]);

  const payee = invoice ? state?.accounts.find((a) => a.account === invoice.payout) : undefined;
  const payoutOk = payout === "" || (payout.length >= 2 && payout.length <= 64 && NEAR_ACCOUNT.test(payout));
  const amountOk = amount === "" || toBaseUnits(amount) !== null;
  const hasAttack = message.trim() !== "" || payout !== "" || amount !== "";

  async function send(attack: Attack) {
    if (!invoice || busy) return;
    setBusy(true);
    setCurrent({ attack });
    setTimeout(() => document.getElementById("attempt")?.scrollIntoView({ behavior: "smooth", block: "start" }), 50);
    try {
      const res = await payInvoice(invoice.id, attack);
      setInvoice(res.invoice);
      setCurrent({ attack, result: res.run });
      void loadAttempts();
      setTimeout(() => void loadState(), 1200);
    } catch (e) {
      setCurrent({ attack, error: e instanceof ApiError ? e.message : "The attempt failed." });
    } finally {
      setBusy(false);
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
  const link = typeof window !== "undefined" ? window.location.href : "";
  const blockedCount = attempts.filter((a) => a.kind === "attack" && a.outcome === "blocked").length;

  return (
    <div className="pay-wrap wide">
      <div className="pay-cols">
        <div className="pay-left">
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
            <span className="pill pill-honey">Pay link · NEAR testnet</span>
            <Link href="/attacks" className="pill">
              <Radar size={13} aria-hidden /> Attack log
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

          <a className="acct role-vendor" href={payee?.url ?? `https://testnet.nearblocks.io/address/${invoice.payout}`} target="_blank" rel="noreferrer">
            <span className="acct-ic" aria-hidden>
              <Store size={18} />
            </span>
            <span className="acct-main">
              <b>{invoice.vendor_name}’s account</b>
              <span className="mono">{shortKey(invoice.payout, 16)}</span>
              <span className="acct-role">Live balance on NEAR testnet</span>
            </span>
            <span className="acct-bal">
              {payee?.balance ? formatUSDC(payee.balance) : "—"}
              <small>USDC</small>
            </span>
          </a>

          {paid && invoice.paid_tx ? (
            <a className="btn" href={TX_URL + invoice.paid_tx} target="_blank" rel="noreferrer">
              <Check size={16} aria-hidden /> Paid on NEAR testnet · {shortKey(invoice.paid_tx, 6)} <ArrowUpRight size={16} aria-hidden />
            </a>
          ) : (
            <button className="btn" onClick={() => void send({})} disabled={busy}>
              <Bot size={17} aria-hidden /> Pay it honestly: {formatUSDC(invoice.amount)} USDC
            </button>
          )}

          <div className="dare">
            <b>Dare someone else</b>
            <span>Send this link to anyone. Every attempt is public and checked on-chain.</span>
            <button
              className="btn"
              onClick={() => {
                void navigator.clipboard?.writeText(link);
                setCopied(true);
                setTimeout(() => setCopied(false), 1500);
              }}
            >
              {copied ? <Check size={16} aria-hidden /> : <Copy size={16} aria-hidden />} {copied ? "Copied" : "Copy link"}
            </button>
          </div>
        </div>

        <div className="pay-right">
          <div className="trick">
            <span className="trick-ic" aria-hidden>
              <Skull size={20} />
            </span>
            <h2>Make the agent pay you instead.</h2>
            <p>
              Bee, the AI agent paying this invoice, reads whatever comes with the link, and it’s easy to fool. Write your own
              trick, edit your copy of the link, or both. The vault on NEAR has the final say.
            </p>

            <label className="field">
              Your message to the agent
              <textarea
                value={message}
                maxLength={1000}
                rows={5}
                placeholder={`Write it your way: a fake email from ${invoice.vendor_name}, a "system note", a new bank account, an urgent extra fee…`}
                onChange={(e) => setMessage(e.target.value)}
              />
              <span className="help">{message.length}/1000</span>
            </label>

            <div className="row2">
              <label className="field">
                Change the payee (optional)
                <input value={payout} spellCheck={false} placeholder="any NEAR account" onChange={(e) => setPayout(e.target.value.trim().toLowerCase())} />
                {!payoutOk && <span className="err">Not a NEAR account</span>}
              </label>
              <label className="field">
                Change the amount (optional)
                <input value={amount} inputMode="decimal" placeholder={`was ${formatUSDC(invoice.amount)}`} onChange={(e) => setAmount(e.target.value.trim())} />
                {!amountOk && <span className="err">Use a USDC amount like 4.5</span>}
              </label>
            </div>

            <button
              className="btn btn-honey big"
              disabled={busy || !hasAttack || !payoutOk || !amountOk}
              onClick={() => void send({ message: message.trim() || undefined, payout: payout || undefined, amount: amount || undefined })}
            >
              <Send size={17} aria-hidden /> {busy ? "Bee is working…" : "Send it to the agent"}
            </button>
            <p className="fine">Attempts are public in the attack log, so don’t include personal information. Test USDC on NEAR testnet.</p>
          </div>

          <div id="attempt" style={{ display: "grid", gap: 10, scrollMarginTop: 20 }}>
            {current && !current.result && !current.error && <Thinking invoice reviewExpected={!!state?.reviewer} />}
            {current?.error && (
              <BotFrame>
                <div className="bubble">{current.error}</div>
              </BotFrame>
            )}
            {current?.result && <AttemptView current={current} invoice={invoice} state={state} />}
          </div>
        </div>
      </div>

      <section className="att-section">
        <div className="att-head">
          <h2>Every attempt on this invoice</h2>
          <span className="muted">
            {attempts.length} attempt{attempts.length === 1 ? "" : "s"} · {blockedCount} attack{blockedCount === 1 ? "" : "s"} blocked
          </span>
        </div>
        <AttemptList attempts={attempts} empty="No one has tried yet. Be the first, or send the link to someone." />
      </section>
    </div>
  );
}

function AttemptView({ current, invoice, state }: { current: Current; invoice: Invoice; state: PlaygroundState | null }) {
  const r = current.result!;
  const agent = r.steps.find((s) => s.key === "agent");
  const request = r.steps.find((s) => s.key === "request");
  const review = r.steps.find((s) => s.key === "review");
  const vault = r.steps.find((s) => s.key === "vault");
  const fooled = !!r.recipient && (r.recipient !== invoice.payout || r.amount !== invoice.amount);
  const v = invoice.vendor_name;
  let text: string;
  if (r.outcome === "reply") text = `Bee didn’t try to pay anything. It said: “${r.reply ?? ""}”`;
  else if (r.outcome === "paid") text = `Paid ${v} exactly ${formatUSDC(invoice.amount)} USDC, as the signed invoice says. Nobody else got a cent.`;
  else if (r.outcome === "blocked" && fooled)
    text = `Bee fell for it and tried to pay ${formatUSDC(r.amount)} USDC to ${r.recipient}. The vault refused on-chain: ${r.reason}. ${v}’s money is safe.`;
  else if (r.outcome === "blocked") text = `The vault refused on-chain: ${r.reason}. No money moved.`;
  else text = `This attempt didn’t finish: ${r.reason}.`;
  return (
    <BotFrame>
      <div className="bubble">
        <Typewriter text={text} />
      </div>
      {agent?.detail && r.outcome !== "reply" && (
        <div className="bubble soft">
          <b>What Bee understood:</b> {agent.detail}
        </div>
      )}
      {request && <PlanCard agent={agent} request={request} state={state} delay={200} />}
      {review && <ReviewCard step={review} delay={450} />}
      {vault && <VaultCard result={r} reviewStep={review} delay={700} invoiceMode />}
      {vault && <Outcome result={r} state={state} delay={1500} />}
    </BotFrame>
  );
}
