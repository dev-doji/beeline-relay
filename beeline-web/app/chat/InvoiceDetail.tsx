"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ArrowLeft, ExternalLink, ShieldCheck } from "lucide-react";
import { Receipt } from "@/components/invoice/Receipt";
import { ShareLink } from "@/components/invoice/ShareLink";
import { AttemptList } from "@/components/attacks/AttemptList";
import { type Attempt, type Invoice, type InvoiceEvent, MAINNET_TX_URL, TX_URL, formatUSDC, getInvoiceAttempts, getInvoiceEvents, shortKey } from "@/lib/api";

// Plain labels for the reasons Beeline refuses a payment claim.
const EVENT_TITLE: Record<string, string> = {
  WRONG_TOKEN: "Wrong token",
  WRONG_AMOUNT: "Wrong amount",
  WRONG_RECEIVER: "Wrong payee",
  WRONG_MEMO: "Not matched to this invoice",
  WRONG_DEPOSIT: "Not a plain transfer",
  WRONG_SENDER: "Sender did not match",
  UNSUPPORTED_CALL: "Unsupported transaction",
  TX_FAILED: "Transaction failed",
  TX_NOT_FOUND: "Transaction not found",
  EXPIRED: "Paid after the invoice expired",
  ALREADY_PAID: "Second payment refused",
  TX_REUSED: "Transaction already used",
  PAYIN_REFUNDED: "Cross-chain payment refunded",
  PAYIN_FAILED: "Cross-chain payment failed",
  PAYIN_INCOMPLETE: "Cross-chain payment too small",
};

/**
 * The vendor's view of one invoice, inside the app (the sidebar stays). It shows the invoice the way the
 * customer sees it, the share tools, and every payment Beeline refused for this invoice.
 */
export function InvoiceDetail({ invoice, onBack }: { invoice: Invoice; onBack: () => void }) {
  const [attempts, setAttempts] = useState<Attempt[]>([]);
  const [events, setEvents] = useState<InvoiceEvent[]>([]);
  useEffect(() => {
    let stop = false;
    const load = () => {
      getInvoiceAttempts(invoice.id)
        .then((r) => !stop && setAttempts(r.data))
        .catch(() => undefined);
      getInvoiceEvents(invoice.id)
        .then((r) => !stop && setEvents(r.data))
        .catch(() => undefined);
    };
    void load();
    const t = setInterval(load, 15000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, [invoice.id]);

  const link = `${window.location.origin}/pay/${invoice.id}`;
  const blocked = attempts.filter((a) => a.outcome === "blocked").length + events.length;

  return (
    <div className="cx-panel-view">
      <button className="cx-back" onClick={onBack}>
        <ArrowLeft size={14} aria-hidden /> All invoices
      </button>
      <div className="cx-detail">
        <div className="cx-detail-main">
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
              network: invoice.network,
            }}
          />
          <Link className="cx-link" href={`/pay/${invoice.id}`} target="_blank">
            <ExternalLink size={13} aria-hidden /> Open the page your customer sees
          </Link>
          <div className="cx-box">
            <div className="cx-box-top">
              <b className="cx-sec">Payment</b>
              <span className={invoice.status === "paid" ? "cx-tag ok" : "cx-tag"}>{invoice.status === "paid" ? "Paid" : "Waiting"}</span>
            </div>
            {invoice.payment?.verification === "intents-delivered" ? (
              <p className="cx-small">
                <b>Delivered by NEAR Intents, confirmed by Beeline.</b> A customer paid from {invoice.payment.sender.replace("cross-chain:", "another chain (")}
                {invoice.payment.sender.startsWith("cross-chain:") ? ")" : ""}. NEAR Intents reported delivery and your USDC balance on NEAR rose by {formatUSDC(invoice.payment.amount)} USDC.
                {!invoice.payment.tx_hash.startsWith("intents:") && (
                  <>
                    {" "}
                    <a className="cx-link" href={MAINNET_TX_URL + invoice.payment.tx_hash} target="_blank" rel="noreferrer">
                      Delivery {shortKey(invoice.payment.tx_hash, 5)}
                    </a>
                  </>
                )}
              </p>
            ) : invoice.payment ? (
              <p className="cx-small">
                <b>Verified on-chain.</b> Beeline read this payment from NEAR: {formatUSDC(invoice.payment.amount)} USDC from {invoice.payment.sender}, in a final block on{" "}
                {new Date(invoice.payment.block_time).toLocaleString()}.{" "}
                <a className="cx-link" href={(invoice.network === "mainnet" ? MAINNET_TX_URL : TX_URL) + invoice.payment.tx_hash} target="_blank" rel="noreferrer">
                  Transaction {shortKey(invoice.payment.tx_hash, 5)}
                </a>
              </p>
            ) : invoice.status === "paid" ? (
              <p className="cx-small">Paid by a demo payment (a test agent paid from the shared test vault), not a customer wallet.</p>
            ) : (
              <p className="cx-small">Nothing received yet. A payment counts only after Beeline reads it from NEAR and it matches this invoice exactly.</p>
            )}
          </div>
        </div>
        <div className="cx-detail-side">
          <div className="cx-box">
            <div className="cx-box-top">
              <b>Send it to {formatUSDC(invoice.amount)} USDC</b>
            </div>
            <ShareLink id={invoice.id} link={link} vendor={invoice.vendor_name} amountLabel={`${formatUSDC(invoice.amount)} USDC`} dareText="" />
          </div>
          <div className="cx-box">
            <div className="cx-box-top">
              <b className="cx-sec">
                <ShieldCheck size={15} aria-hidden /> Security events
              </b>
              <span className={blocked > 0 ? "cx-tag" : "cx-tag ok"}>{blocked > 0 ? `${blocked} blocked` : "All clear"}</span>
            </div>
            <p className="cx-muted cx-small">
              Beeline checks every payment against this signed invoice. A different account, a different amount, an edited link or a second payment is blocked
              automatically, and it shows up here.
            </p>
            {events.length > 0 && (
              <ul className="cx-events">
                {events.map((e, i) => (
                  <li key={i}>
                    <b>{EVENT_TITLE[e.kind] ?? e.kind}</b>
                    <span>{e.reason}</span>
                    <time>{new Date(e.created_at).toLocaleString()}</time>
                  </li>
                ))}
              </ul>
            )}
            <AttemptList attempts={attempts} empty={events.length ? "" : "Nothing suspicious so far. Payments are checked automatically."} />
          </div>
        </div>
      </div>
    </div>
  );
}
