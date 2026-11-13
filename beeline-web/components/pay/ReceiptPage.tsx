"use client";

import { use, useEffect, useState } from "react";
import Link from "next/link";
import { ArrowUpRight, Check, ShieldCheck } from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import { lineTotal } from "@/components/invoice/Receipt";
import { type GuardReview, type Invoice, MAINNET_TX_URL, TX_URL, formatUSDC, getInvoice, getInvoiceGuard, shortKey } from "@/lib/api";
import "./checkout.css";

/**
 * The receipt a customer keeps (modelled on the Rhodium receipt page): proof of payment on one page, built only
 * from what the server recorded after it verified the payment. Opening it for an unpaid invoice shows no receipt.
 */
export function ReceiptPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [invoice, setInvoice] = useState<Invoice | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [guard, setGuard] = useState<GuardReview | null>(null);

  useEffect(() => {
    getInvoice(id)
      .then(setInvoice)
      .catch(() => setError("We could not find this receipt. The link may be wrong."));
    getInvoiceGuard(id)
      .then((r) => setGuard(r.data.find((x) => x.subject === "payment_confirmed") ?? null))
      .catch(() => undefined);
  }, [id]);

  const brand = (
    <div className="ck-brand">
      <span className="ck-logo">
        <BeeMark size={12} onDark />
      </span>
      Receipt
    </div>
  );

  if (error) {
    return (
      <div className="ck-card">
        {brand}
        <div className="ck-err" role="alert">
          {error}
        </div>
      </div>
    );
  }
  if (!invoice) {
    return (
      <div className="ck-card" aria-busy>
        {brand}
        <div className="ck-waiting">
          <span className="ck-spin" aria-hidden /> <span>Loading…</span>
        </div>
      </div>
    );
  }
  const p = invoice.payment;
  if (invoice.status !== "paid") {
    return (
      <div className="ck-card">
        {brand}
        <h1 className="ck-h1">{invoice.vendor_name}</h1>
        <div className="ck-note">No payment has been confirmed for this invoice yet, so there is no receipt.</div>
        <Link className="ck-btn" href={`/pay/${invoice.id}`} style={{ textDecoration: "none" }}>
          Go to checkout
        </Link>
      </div>
    );
  }
  const real = invoice.network === "mainnet";
  const via = p?.verification === "intents-delivered" ? "NEAR Intents, confirmed by Beeline" : p ? "On NEAR, verified by Beeline" : "Demo payment on NEAR testnet";
  const canLink = !!invoice.paid_tx && !invoice.paid_tx.startsWith("intents:");
  const paidAt = invoice.paid_at ?? p?.verified_at;
  return (
    <div className="ck-card">
      {brand}
      <div className="ck-ok" style={{ textAlign: "left", display: "flex", gap: 12, alignItems: "center", paddingBottom: 0 }}>
        <div className="ck-check" style={{ margin: 0, width: 40, height: 40 }}>
          <Check aria-hidden style={{ width: 20, height: 20 }} />
        </div>
        <div>
          <h2 style={{ margin: 0, fontSize: 17 }}>Payment received</h2>
          <div className="ck-order">{paidAt ? new Date(paidAt).toLocaleString() : ""}</div>
        </div>
      </div>
      <div className="ck-amount">
        {formatUSDC(invoice.amount)}
        <span className="ck-unit">USDC</span>
      </div>
      <div className="ck-chip">
        <i className={real ? "real" : ""} />
        {real ? "Real money on NEAR" : "Test USDC on NEAR testnet · no real money"}
      </div>
      <ul className="ck-items" style={{ marginTop: 14 }}>
        {invoice.items.map((it, i) => (
          <li key={i}>
            <span>
              {it.quantity} × {it.name}
            </span>
            <span>{formatUSDC(lineTotal(it))} USDC</span>
          </li>
        ))}
      </ul>
      <div className="ck-rows">
        <div className="ck-row">
          <span className="k">Paid to</span>
          <b>{invoice.vendor_name}</b>
        </div>
        <div className="ck-row">
          <span className="k">Invoice</span>
          <b>{invoice.id.slice(-6).toUpperCase()}</b>
        </div>
        {p && (
          <div className="ck-row">
            <span className="k">Paid by</span>
            <b>{p.sender.replace("cross-chain:", "another chain: ")}</b>
          </div>
        )}
        <div className="ck-row">
          <span className="k">How it was checked</span>
          <b style={{ maxWidth: "62%" }}>{via}</b>
        </div>
        {guard && (
          <div className="ck-row">
            <span className="k">Guard agent (AI)</span>
            <b>{guard.verdict === "clear" ? "No fraud signs" : guard.verdict === "watch" ? "Flagged to watch" : "Flagged as fraud"}</b>
          </div>
        )}
      </div>
      {canLink && invoice.paid_tx && (
        <a className="ck-explorer" href={(real ? MAINNET_TX_URL : TX_URL) + invoice.paid_tx} target="_blank" rel="noreferrer">
          View the transaction {shortKey(invoice.paid_tx, 5)} <ArrowUpRight size={14} aria-hidden />
        </a>
      )}
      <button className="ck-btn secondary" onClick={() => window.print()}>
        Save or print this receipt
      </button>
      <div className="ck-foot">
        <ShieldCheck size={12} aria-hidden style={{ verticalAlign: "-2px" }} /> Beeline never holds the money: it went straight to {invoice.vendor_name}. This receipt is built from what the
        server verified, not from anything a customer typed.
      </div>
    </div>
  );
}
