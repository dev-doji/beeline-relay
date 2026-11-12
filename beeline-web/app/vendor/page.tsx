"use client";

import { useMemo, useState } from "react";
import { ArrowRight, Check, Plus, Store, Trash2 } from "lucide-react";
import { ShareLink } from "@/components/invoice/ShareLink";
import { CashOut } from "@/components/invoice/CashOut";
import { Receipt, lineTotal } from "@/components/invoice/Receipt";
import { AppFrame } from "@/components/AppFrame";
import { ApiError, type Invoice, type InvoiceItem, createInvoice, formatUSDC, toBaseUnits } from "@/lib/api";

interface DraftItem {
  name: string;
  quantity: string;
  price: string; // USDC, e.g. "0.15"
}

// Nothing is pre-filled: the vendor enters their own shop, account and items.
const START: DraftItem[] = [{ name: "", quantity: "1", price: "" }];

const dare = (vendor: string, link: string) =>
  `Can you trick my AI payment agent into paying you instead of ${vendor}? Try anything: ${link}`;

const NEAR_ACCOUNT = /^(([a-z\d]+[-_])*[a-z\d]+\.)*([a-z\d]+[-_])*[a-z\d]+$/;

export default function VendorPage() {
  const [vendor, setVendor] = useState("");
  const [handle, setHandle] = useState("");
  const [memo, setMemo] = useState("");
  const [payout, setPayout] = useState("");
  const [items, setItems] = useState<DraftItem[]>(START);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [created, setCreated] = useState<Invoice | null>(null);

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

  // The permanent link carries the payee in its address; each payer who opens it gets a fresh,
  // single-use signed invoice for the amount they choose.
  const permanent =
    payoutOk && typeof window !== "undefined"
      ? `${window.location.origin}/pay/to?${new URLSearchParams({ name: vendor.trim() || payout, to: payout }).toString()}`
      : "";

  async function submit() {
    if (!valid || !payoutOk || overCap || busy || !vendor.trim()) return;
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
    <AppFrame active="vendor" title="Vendor pay links" icon={<Store size={16} aria-hidden />}>
    <div className="frame-inner">
      <div className="page-head">
        <span className="pill pill-honey">
          <Store size={13} aria-hidden /> Step 1 of 2 · NEAR testnet
        </span>
        <h1>Create a payment link. Share it anywhere.</h1>
        <p>
          Online or in the shop: Beeline signs your invoice, you send the link on any app or show the QR at the counter, and the
          customer pays in USDC. Every payment is tracked, and the vault blocks anything that doesn’t match the signed invoice.
        </p>
      </div>

      <div className="split">
        <div className="panel">
          <h2>Your shop</h2>
          <div className="row2">
            <label className="field">
              Shop name
              <input value={vendor} maxLength={60} placeholder="Your shop or business name" onChange={(e) => (setVendor(e.target.value), setCreated(null))} />
            </label>
            <label className="field">
              Handle
              <input value={handle} maxLength={60} placeholder="@yourshop (optional)" onChange={(e) => (setHandle(e.target.value), setCreated(null))} />
            </label>
          </div>
          <label className="field">
            Payout account (NEAR testnet)
            <input value={payout} spellCheck={false} placeholder="yourshop.yourname.testnet" onChange={(e) => (setPayout(e.target.value.trim().toLowerCase()), setCreated(null))} />
            {payoutOk ? (
              <span className="help">A real testnet account. It is NOT on the vault’s approved list: only the signed invoice lets Bee pay it, and the money really lands there.</span>
            ) : payout ? (
              <span className="err">Enter a NEAR account, e.g. yourshop.yourname.testnet</span>
            ) : (
              <span className="help">The NEAR testnet account that should receive the money.</span>
            )}
          </label>
          <label className="field">
            Note for the buyer
            <input value={memo} maxLength={200} placeholder="Order number or what this is for (optional)" onChange={(e) => (setMemo(e.target.value), setCreated(null))} />
          </label>

          <div className="field" role="group" aria-label="Items">
            Items
            <div className="items">
              {items.map((it, i) => (
                <div className="item-row" key={i}>
                  <input aria-label={`Item ${i + 1} name`} value={it.name} placeholder="Item name" onChange={(e) => update(i, { name: e.target.value })} />
                  <input aria-label={`Item ${i + 1} quantity`} value={it.quantity} inputMode="numeric" onChange={(e) => update(i, { quantity: e.target.value })} />
                  <input aria-label={`Item ${i + 1} price in USDC`} value={it.price} placeholder="0.00" inputMode="decimal" onChange={(e) => update(i, { price: e.target.value })} />
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
            {!valid && items.some((x) => x.name || x.price) && <span className="err">Every item needs a name, a whole quantity (1-999) and a price like 0.15.</span>}
            {overCap && <span className="err">Playground invoices are capped at 5 test USDC (the testnet vault’s limit).</span>}
          </div>

          {error && (
            <div className="alert" role="alert">
              {error}
            </div>
          )}
          <button className="btn btn-honey" onClick={() => void submit()} disabled={!valid || !payoutOk || overCap || busy || !vendor.trim()}>
            {busy ? "Signing your invoice…" : "Create payment link"} <ArrowRight size={18} aria-hidden />
          </button>
        </div>

        <div style={{ display: "grid", gap: 16, position: "sticky", top: 16 }}>
          {permanent && (
            <div className="panel">
              <h2>Or: one permanent link</h2>
              <p className="help">
                For a freelancer or a shop that wants a single link for good. The customer chooses the amount, and Beeline signs a new
                one-time invoice for each payment, so the vault still pays only that payee and amount, once.
              </p>
              <ShareLink id="" link={permanent} vendor={vendor.trim() || payout} amountLabel="" dareText={dare(vendor, permanent)} />
            </div>
          )}
          <CashOut />
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
                <Check size={18} color="var(--ok)" aria-hidden style={{ verticalAlign: "-3px" }} /> Step 2: send it to your customer
              </h2>
              <ShareLink id={created.id} link={link} vendor={vendor} amountLabel={`${formatUSDC(total)} USDC`} dareText={dare(vendor, link)} />
            </div>
          )}
        </div>
      </div>
    </div>
    </AppFrame>
  );
}
