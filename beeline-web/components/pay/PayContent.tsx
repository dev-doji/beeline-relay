"use client";

import { use } from "react";
import Link from "next/link";
import { ArrowUpRight, Check, ShieldCheck } from "lucide-react";
import { Receipt } from "@/components/invoice/Receipt";
import { TX_URL, formatUSDC, shortKey } from "@/lib/api";
import { usePay } from "./usePay";

/**
 * What a customer sees when they open a pay link: the signed invoice, one pay button and a plain
 * explanation of how the payment is protected. The "try to trick the agent" demo lives on its own
 * page (/pay/<id>/security-test), not here.
 */
export function PayContent({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { invoice, loadError, current, busy, send } = usePay(id);

  if (loadError) {
    return (
      <div className="px-wrap narrow">
        <div className="alert" role="alert">
          {loadError}
        </div>
        <Link className="px-btn dark" href="/chat">
          Make an invoice
        </Link>
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

  const paid = invoice.status === "paid";
  const r = current?.result;
  // A payment that Beeline refused or that did not finish is explained in plain words.
  const problem = current?.error
    ? current.error
    : r && r.outcome !== "paid"
      ? r.outcome === "blocked"
        ? `Beeline stopped this payment: ${r.reason}. No money moved.`
        : `This payment did not finish: ${r.reason}. No money moved.`
      : null;

  return (
    <div className="px-wrap narrow">
      <span className="px-chip">Invoice · NEAR testnet</span>

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

      {paid && invoice.paid_tx ? (
        <a className="px-btn ok lg full" href={TX_URL + invoice.paid_tx} target="_blank" rel="noreferrer">
          <Check size={16} aria-hidden /> Paid · view the transaction {shortKey(invoice.paid_tx, 5)} <ArrowUpRight size={14} aria-hidden />
        </a>
      ) : (
        <>
          <button className="px-btn dark lg full" onClick={() => void send({})} disabled={busy}>
            {busy ? "Paying…" : `Pay ${formatUSDC(invoice.amount)} USDC`}
          </button>
          <p className="px-note">Test payment on NEAR testnet. No real money moves.</p>
        </>
      )}

      {problem && (
        <div className="alert" role="alert">
          {problem}
        </div>
      )}

      <div className="px-secure">
        <div className="px-secure-head">
          <ShieldCheck size={16} aria-hidden /> <b>Protected by Beeline</b>
        </div>
        <ul>
          <li>It pays {invoice.vendor_name}’s account and exactly {formatUSDC(invoice.amount)} USDC. Nothing else.</li>
          <li>It works once. A second payment for this invoice is refused.</li>
          <li>The invoice is signed. If anyone edits the link, the signature breaks and the payment is blocked.</li>
        </ul>
      </div>

      <p className="px-foot">
        <Link href={`/pay/${invoice.id}/security-test`}>Security test</Link> · <Link href="/attacks">Attack log</Link>
      </p>
    </div>
  );
}
