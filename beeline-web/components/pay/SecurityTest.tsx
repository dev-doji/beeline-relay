"use client";

import { use, useState } from "react";
import Link from "next/link";
import { ArrowLeft, Check, Copy, Send, ShieldCheck, Store } from "lucide-react";
import { BotFrame, Thinking } from "@/components/chat/Chat";
import { AttemptList } from "@/components/attacks/AttemptList";
import { formatUSDC, shortKey, toBaseUnits } from "@/lib/api";
import { AttemptView } from "./AttemptView";
import { usePay } from "./usePay";

const NEAR_ACCOUNT = /^(([a-z\d]+[-_])*[a-z\d]+\.)*([a-z\d]+[-_])*[a-z\d]+$/;

/**
 * The security test: for developers and judges, not for customers. Pretend to be a scammer, try to
 * make the test agent pay the wrong account or amount, and watch the vault refuse on-chain.
 */
export function SecurityTest({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { invoice, state, attempts, loadError, current, busy, send } = usePay(id);
  const [message, setMessage] = useState("");
  const [payout, setPayout] = useState("");
  const [amount, setAmount] = useState("");
  const [copied, setCopied] = useState(false);

  if (loadError) {
    return (
      <div className="px-wrap narrow">
        <div className="alert" role="alert">
          {loadError}
        </div>
      </div>
    );
  }
  if (!invoice) {
    return (
      <div className="px-wrap narrow" aria-busy>
        <div className="thinking">
          <div className="think-line">
            <span className="spinner" aria-hidden /> Loading invoice…
          </div>
        </div>
      </div>
    );
  }

  const payee = state?.accounts.find((a) => a.account === invoice.payout);
  const payoutOk = payout === "" || (payout.length >= 2 && payout.length <= 64 && NEAR_ACCOUNT.test(payout));
  const amountOk = amount === "" || toBaseUnits(amount) !== null;
  const hasAttack = message.trim() !== "" || payout !== "" || amount !== "";
  const blockedCount = attempts.filter((a) => a.kind === "attack" && a.outcome === "blocked").length;

  return (
    <div className="px-wrap narrow">
      <div className="px-row">
        <Link href={`/pay/${invoice.id}`} className="px-back">
          <ArrowLeft size={14} aria-hidden /> Back to the invoice
        </Link>
        <button
          className="px-btn sm"
          onClick={() => {
            void navigator.clipboard?.writeText(window.location.href);
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          }}
        >
          {copied ? <Check size={13} aria-hidden /> : <Copy size={13} aria-hidden />} {copied ? "Copied" : "Copy test link"}
        </button>
      </div>

      <div className="px-trick">
        <div className="px-trick-head">
          <span className="px-trick-ic" aria-hidden>
            <ShieldCheck size={15} />
          </span>
          <h2>Security test</h2>
        </div>
        <p>
          This is how we show the protection working. Pretend to be a scammer: write a fake message to Bee, the test AI agent that pays this
          invoice, and try to change who gets paid or how much. Beeline’s vault on NEAR checks every payment against the signed invoice and
          refuses anything that does not match. No real money moves.
        </p>

        <label className="px-field">
          <span>Your message to the agent</span>
          <textarea
            value={message}
            maxLength={1000}
            rows={3}
            placeholder={`A fake email, a "system note", a new bank account…`}
            onChange={(e) => setMessage(e.target.value)}
          />
          <em>{message.length}/1000</em>
        </label>

        <div className="px-grid2">
          <label className="px-field">
            <span>Pay a different account (optional)</span>
            <input value={payout} spellCheck={false} placeholder="any NEAR account" onChange={(e) => setPayout(e.target.value.trim().toLowerCase())} />
            {!payoutOk && <strong>Not a NEAR account</strong>}
          </label>
          <label className="px-field">
            <span>Pay a different amount (optional)</span>
            <input value={amount} inputMode="decimal" placeholder={`was ${formatUSDC(invoice.amount)}`} onChange={(e) => setAmount(e.target.value.trim())} />
            {!amountOk && <strong>Use a USDC amount like 4.5</strong>}
          </label>
        </div>

        <button
          className="px-btn honey full"
          disabled={busy || !hasAttack || !payoutOk || !amountOk}
          onClick={() => void send({ message: message.trim() || undefined, payout: payout || undefined, amount: amount || undefined }, true)}
        >
          <Send size={14} aria-hidden /> {busy ? "Bee is working…" : "Run the test"}
        </button>
        <p className="px-fine">Attempts are public in the attack log, so don’t include personal information. Test USDC on NEAR testnet.</p>
      </div>

      <div className="px-acct static">
        <span className="px-acct-ic" aria-hidden>
          <Store size={14} />
        </span>
        <span className="px-acct-main">
          <b>{invoice.vendor_name}’s account (the real payee)</b>
          <span>{shortKey(invoice.payout, 12)}</span>
        </span>
        <span className="px-acct-bal">
          {payee?.balance ? formatUSDC(payee.balance) : "—"}
          <small>USDC</small>
        </span>
      </div>

      <div id="attempt" className="px-attempt">
        {current && !current.result && !current.error && <Thinking invoice reviewExpected={!!state?.reviewer} />}
        {current?.error && (
          <BotFrame>
            <div className="bubble">{current.error}</div>
          </BotFrame>
        )}
        {current?.result && <AttemptView current={current} invoice={invoice} state={state} />}
      </div>

      <section className="px-att">
        <div className="px-att-head">
          <h2>Every attempt on this invoice</h2>
          <span>
            {attempts.length} attempt{attempts.length === 1 ? "" : "s"} · {blockedCount} blocked
          </span>
        </div>
        <AttemptList attempts={attempts} empty="No one has tried yet." />
      </section>
    </div>
  );
}
