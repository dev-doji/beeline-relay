"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { ArrowLeft, ExternalLink, ShieldCheck } from "lucide-react";
import { Receipt } from "@/components/invoice/Receipt";
import { ShareLink } from "@/components/invoice/ShareLink";
import { AttemptList } from "@/components/attacks/AttemptList";
import { type Attempt, type Invoice, formatUSDC, getInvoiceAttempts } from "@/lib/api";

/**
 * The vendor's view of one invoice, inside the app (the sidebar stays). It shows the invoice the way the
 * customer sees it, the share tools, and every payment Beeline refused for this invoice.
 */
export function InvoiceDetail({ invoice, onBack }: { invoice: Invoice; onBack: () => void }) {
  const [attempts, setAttempts] = useState<Attempt[]>([]);
  useEffect(() => {
    let stop = false;
    const load = () =>
      getInvoiceAttempts(invoice.id)
        .then((r) => !stop && setAttempts(r.data))
        .catch(() => undefined);
    void load();
    const t = setInterval(load, 15000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, [invoice.id]);

  const link = `${window.location.origin}/pay/${invoice.id}`;
  const blocked = attempts.filter((a) => a.outcome === "blocked").length;

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
            }}
          />
          <Link className="cx-link" href={`/pay/${invoice.id}`} target="_blank">
            <ExternalLink size={13} aria-hidden /> Open the page your customer sees
          </Link>
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
            <AttemptList attempts={attempts} empty="Nothing suspicious so far. Payments are checked automatically." />
          </div>
        </div>
      </div>
    </div>
  );
}
