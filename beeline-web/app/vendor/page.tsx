"use client";

import { useEffect, useMemo, useState } from "react";
import Link from "next/link";
import { QRCodeSVG } from "qrcode.react";
import { ArrowRight, Check, Copy, ExternalLink, MessageCircle, Plus, Store, Trash2 } from "lucide-react";
import { Receipt, lineTotal } from "@/components/invoice/Receipt";
import { ApiError, type Invoice, type InvoiceItem, createInvoice, getPlayground, toBaseUnits } from "@/lib/api";

interface DraftItem {
  name: string;
  quantity: string;
  price: string; // USDC, e.g. "0.15"
}

const START: DraftItem[] = [
  { name: "Linen shirt", quantity: "3", price: "0.15" },
  { name: "Express delivery", quantity: "1", price: "0.05" },
];

const NEAR_ACCOUNT = /^(([a-z\d]+[-_])*[a-z\d]+\.)*([a-z\d]+[-_])*[a-z\d]+$/;

export default function VendorPage() {
  const [vendor, setVendor] = useState("MS Clothing");
  const [handle, setHandle] = useState("@msclothing");
  const [memo, setMemo] = useState("Order #1842");
  const [payout, setPayout] = useState("");

  // The demo vendor's real testnet account (ms-clothing.<owner>), read from the API.
  useEffect(() => {
    getPlayground()
      .then((st) => {
        const v = st.accounts.find((a) => a.role === "vendor");
        if (v) setPayout((p) => p || v.account);
      })
      .catch(() => undefined);
  }, []);
  const [items, setItems] = useState<DraftItem[]>(START);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<Invoice | null>(null);
  const [copied, setCopied] = useState(false);

  const parsed: (InvoiceItem | null)[] = items.map((it) => {
    const units = toBaseUnits(it.price);
    const q = Number(it.quantity);
    if (!it.name.trim() || !units || !Number.isInteger(q) || q < 1 || q > 999) return null;
    return { name: it.name.trim(), quantity: q, unit_price: units };
  });
  const valid = parsed.every(Boolean) && parsed.length > 0;
  const total = useMemo(
    () => parsed.reduce((sum, it) => (it ? sum + BigInt(lineTotal(it)) : sum), 0n).toString(),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [items],
  );
  const overCap = BigInt(total) > 5_000_000n;
  const payoutOk = payout.length >= 2 && payout.length <= 64 && NEAR_ACCOUNT.test(payout);

  const link = created && typeof window !== "undefined" ? `${window.location.origin}/pay/${created.id}` : "";

  async function submit() {
    if (!valid || !payoutOk || overCap || busy) return;
    setBusy(true);
    setError(null);
    try {
      const inv = await createInvoice({
        vendor_name: vendor,
        vendor_handle: handle,
        payout,
        memo,
        items: parsed.filter((x): x is InvoiceItem => !!x),
      });
      setCreated(inv);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Could not create the invoice.");
    } finally {
      setBusy(false);
    }
  }

  function update(i: number, patch: Partial<DraftItem>) {
    setItems((xs) => xs.map((x, j) => (j === i ? { ...x, ...patch } : x)));
    setCreated(null);
  }

  return (
    <div className="wrap">
      <div className="page-head">
        <span className="pill pill-honey">
          <Store size={13} aria-hidden /> Vendor demo · NEAR testnet
        </span>
        <h1>Send a pay link that can’t be hijacked</h1>
        <p>
          Create an invoice like you would on Instagram or WhatsApp. Beeline signs it, so if a scammer copies your link and
          swaps in their account, or changes the amount, the payment is refused on-chain. Genuine payments land in your real
          testnet account.
        </p>
      </div>

      <div className="split">
        <div className="panel">
          <h2>Your shop</h2>
          <div className="row2">
            <label className="field">
              Shop name
              <input value={vendor} maxLength={60} onChange={(e) => (setVendor(e.target.value), setCreated(null))} />
            </label>
            <label className="field">
              Handle
              <input value={handle} maxLength={60} onChange={(e) => (setHandle(e.target.value), setCreated(null))} />
            </label>
          </div>
          <label className="field">
            Payout account (NEAR testnet)
            <input value={payout} spellCheck={false} onChange={(e) => (setPayout(e.target.value.trim().toLowerCase()), setCreated(null))} />
            {payoutOk ? (
              <span className="help">A real testnet account. It is NOT on the vault’s approved list: only the signed invoice lets Bee pay it, and the money really lands there.</span>
            ) : (
              <span className="err">Enter a NEAR account, e.g. ms-clothing.yourname.testnet</span>
            )}
          </label>
          <label className="field">
            Note for the buyer
            <input value={memo} maxLength={200} onChange={(e) => (setMemo(e.target.value), setCreated(null))} />
          </label>

          <div className="field" role="group" aria-label="Items">
            Items
            <div className="items">
              {items.map((it, i) => (
                <div className="item-row" key={i}>
                  <input aria-label={`Item ${i + 1} name`} value={it.name} placeholder="Item" onChange={(e) => update(i, { name: e.target.value })} />
                  <input aria-label={`Item ${i + 1} quantity`} value={it.quantity} inputMode="numeric" onChange={(e) => update(i, { quantity: e.target.value })} />
                  <input aria-label={`Item ${i + 1} price in USDC`} value={it.price} inputMode="decimal" onChange={(e) => update(i, { price: e.target.value })} />
                  <button
                    className="icon-btn"
                    aria-label={`Remove item ${i + 1}`}
                    onClick={() => (setItems((xs) => xs.filter((_, j) => j !== i)), setCreated(null))}
                    disabled={items.length === 1}
                  >
                    <Trash2 size={16} />
                  </button>
                </div>
              ))}
            </div>
            <span className="help">Name · quantity · price each (test USDC)</span>
            {items.length < 10 && (
              <button className="btn ghost" style={{ justifySelf: "start" }} onClick={() => setItems((xs) => [...xs, { name: "", quantity: "1", price: "0.1" }])}>
                <Plus size={16} aria-hidden /> Add item
              </button>
            )}
            {!valid && <span className="err">Every item needs a name, a whole quantity (1-999) and a price like 0.15.</span>}
            {overCap && <span className="err">Playground invoices are capped at 5 test USDC (the testnet vault’s limit).</span>}
          </div>

          {error && (
            <div className="alert" role="alert">
              {error}
            </div>
          )}
          <button className="btn btn-honey" onClick={() => void submit()} disabled={!valid || !payoutOk || overCap || busy}>
            {busy ? "Signing your invoice…" : "Create pay link"} <ArrowRight size={18} aria-hidden />
          </button>
        </div>

        <div style={{ display: "grid", gap: 16, position: "sticky", top: 16 }}>
          <Receipt
            data={{
              id: created?.id,
              vendor_name: vendor,
              vendor_handle: handle,
              memo,
              items: parsed.filter((x): x is InvoiceItem => !!x),
              amount: total,
              payout,
              status: created ? created.status : "draft",
              expires_at: created?.expires_at,
              signed: !!created,
            }}
          />
          {created && (
            <div className="panel" style={{ animation: "rise .45s var(--ease-out) both" }}>
              <h2>
                <Check size={18} color="var(--ok)" aria-hidden style={{ verticalAlign: "-3px" }} /> Your pay link is ready
              </h2>
              <div className="share">
                <div className="qr" aria-label="QR code for the pay link">
                  <QRCodeSVG value={link} size={112} level="M" />
                </div>
                <div style={{ display: "grid", gap: 10, minWidth: 0, width: "100%" }}>
                  <div className="linkbox">
                    <code>{link}</code>
                    <button
                      className="icon-btn"
                      aria-label="Copy link"
                      onClick={() => {
                        void navigator.clipboard?.writeText(link);
                        setCopied(true);
                        setTimeout(() => setCopied(false), 1500);
                      }}
                    >
                      {copied ? <Check size={16} /> : <Copy size={16} />}
                    </button>
                  </div>
                  <div className="actions">
                    <Link className="btn btn-honey" href={`/pay/${created.id}`}>
                      Open as buyer <ExternalLink size={16} aria-hidden />
                    </Link>
                    <a
                      className="btn"
                      href={`https://wa.me/?text=${encodeURIComponent(`Hi! Here's your invoice from ${vendor}: ${link}`)}`}
                      target="_blank"
                      rel="noreferrer"
                    >
                      <MessageCircle size={16} aria-hidden /> WhatsApp
                    </a>
                  </div>
                </div>
              </div>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
