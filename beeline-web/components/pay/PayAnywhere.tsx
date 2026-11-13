"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { QRCodeSVG } from "qrcode.react";
import { Check, Copy, Loader2 } from "lucide-react";
import {
  ApiError,
  type Invoice,
  type PayinOption,
  type PayinQuote,
  createPayin,
  getPayin,
  getPayinOptions,
} from "@/lib/api";

const TERMINAL = new Set(["paid", "incomplete", "refunded", "failed", "expired"]);

function CopyRow({ label, value }: { label: string; value: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="px-copy">
      <span className="px-copy-label">{label}</span>
      <code title={value}>{value}</code>
      <button
        type="button"
        className="px-copy-btn"
        aria-label={`Copy ${label}`}
        onClick={() => {
          void navigator.clipboard?.writeText(value);
          setCopied(true);
          setTimeout(() => setCopied(false), 1400);
        }}
      >
        {copied ? <Check size={13} aria-hidden /> : <Copy size={13} aria-hidden />}
      </button>
    </div>
  );
}

const STEPS: { key: PayinQuote["status"][]; text: string }[] = [
  { key: ["waiting"], text: "Waiting for your payment" },
  { key: ["processing"], text: "Payment seen, converting" },
  { key: ["confirming", "paid"], text: "Delivered to the vendor" },
];

function useCountdown(deadline: string | undefined, active: boolean) {
  const [left, setLeft] = useState<number | null>(null);
  useEffect(() => {
    if (!deadline || !active) return;
    const tick = () => setLeft(Math.max(0, Math.floor((new Date(deadline).getTime() - Date.now()) / 1000)));
    tick();
    const t = setInterval(tick, 1000);
    return () => clearInterval(t);
  }, [deadline, active]);
  if (left === null) return "";
  return `${Math.floor(left / 60)}:${String(left % 60).padStart(2, "0")}`;
}

/**
 * Pay a real-money invoice from any wallet or exchange: pick what you hold, say where a refund should go, then
 * scan the QR code or copy the address. Nothing to paste back: the page only shows progress. The server marks
 * the invoice paid, and only when NEAR Intents reports delivery AND the vendor's balance on NEAR rose
 * (docs/PAYMENT_FLOW.md).
 */
export function PayAnywhere({ invoice, onPaid }: { invoice: Invoice; onPaid: () => void }) {
  const [options, setOptions] = useState<PayinOption[] | null>(null);
  const [optError, setOptError] = useState<string | null>(null);
  const [asset, setAsset] = useState("");
  const [chain, setChain] = useState("");
  const [refund, setRefund] = useState("");
  const [quote, setQuote] = useState<PayinQuote | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const paidOnce = useRef(false);
  const live = !!quote && !TERMINAL.has(quote.status);
  const countdown = useCountdown(quote?.deadline, live);

  useEffect(() => {
    let on = true;
    getPayinOptions()
      .then((r) => {
        if (!on) return;
        setOptions(r.data);
        if (r.data[0]) {
          setAsset(r.data[0].asset_id);
          setChain(r.data[0].chain);
        }
      })
      .catch((e) => on && setOptError(e instanceof ApiError ? e.message : "Could not load the list of tokens."));
    // A quote made earlier (page reloaded) is picked up again.
    getPayin(invoice.id)
      .then((r) => on && r.quote && !TERMINAL.has(r.quote.status) && setQuote(r.quote))
      .catch(() => undefined);
    return () => {
      on = false;
    };
  }, [invoice.id]);

  // While a deposit address is live, ask the server how it is going.
  useEffect(() => {
    if (!live) return;
    let on = true;
    const tick = async () => {
      try {
        const r = await getPayin(invoice.id);
        if (!on) return;
        if (r.quote) setQuote(r.quote);
        if (r.invoice_status === "paid" && !paidOnce.current) {
          paidOnce.current = true;
          onPaid();
        }
      } catch {
        /* keep the last known state; the next tick tries again */
      }
    };
    const t = setInterval(() => void tick(), 4000);
    return () => {
      on = false;
      clearInterval(t);
    };
  }, [live, invoice.id, onPaid]);

  const chosen = useMemo(() => options?.find((o) => o.asset_id === asset), [options, asset]);
  const chains = useMemo(() => Array.from(new Set((options ?? []).map((o) => o.chain))), [options]);
  const tokensOnChain = (options ?? []).filter((o) => o.chain === chain);

  function pickChain(c: string) {
    setChain(c);
    const first = (options ?? []).find((o) => o.chain === c);
    if (first) setAsset(first.asset_id);
  }

  async function start() {
    if (busy || !chosen) return;
    setBusy(true);
    setProblem(null);
    try {
      const r = await createPayin(invoice.id, chosen.asset_id, refund.trim());
      setQuote(r.quote);
    } catch (e) {
      setProblem(e instanceof ApiError ? e.message : "Could not get a payment address. Try again in a moment.");
    } finally {
      setBusy(false);
    }
  }

  if (optError) return <div className="alert" role="alert">{optError}</div>;
  if (!options) {
    return (
      <div className="px-status" role="status">
        <Loader2 size={14} className="px-spin" aria-hidden /> Loading ways to pay…
      </div>
    );
  }

  // ---- A payment address exists ----
  if (quote) {
    const finished = TERMINAL.has(quote.status);
    const stepIdx = STEPS.findIndex((s) => s.key.includes(quote.status));
    return (
      <div className="px-sheet-sec">
        {!finished && (
          <>
            <div className="px-qr">
              <div className="px-qr-box">
                <QRCodeSVG value={quote.deposit_address} size={176} level="M" marginSize={1} />
              </div>
              <div className="px-qr-amt">
                <span>Send exactly</span>
                <b>
                  {quote.amount_in} {quote.symbol}
                </b>
                <span>
                  on <b>{quote.chain_name}</b>
                  {countdown && <em> · expires in {countdown}</em>}
                </span>
              </div>
            </div>
            <CopyRow label="Address" value={quote.deposit_address} />
            {quote.deposit_memo && <CopyRow label="Memo" value={quote.deposit_memo} />}
            <p className="px-fine">
              Scan with your wallet or exchange, or copy the address. Send only {quote.symbol} on {quote.chain_name}: other tokens or networks can be lost.
              This address is for this invoice only. Some exchanges take their withdrawal fee out of the amount you enter: make sure the address receives at least {quote.amount_in} {quote.symbol}, or it will be refunded.
            </p>
            <ol className="px-steps" aria-label="Payment progress">
              {STEPS.map((s, i) => (
                <li key={s.text} className={i < stepIdx ? "done" : i === stepIdx ? "now" : ""}>
                  {s.text}
                </li>
              ))}
            </ol>
          </>
        )}
        <div className={finished && quote.status !== "paid" ? "alert" : "px-status"} role={finished ? "alert" : "status"}>
          {!finished && <Loader2 size={14} className="px-spin" aria-hidden />} {quote.message}
        </div>
        {finished && quote.status !== "paid" && (
          <button className="px-btn full" onClick={() => setQuote(null)}>
            Start again
          </button>
        )}
      </div>
    );
  }

  // ---- Choose what to pay with ----
  const refundOk = refund.trim().length >= 20;
  return (
    <div className="px-sheet-sec">
      <b>Pay from any wallet or exchange</b>
      <div className="px-grid2 tight">
        <label className="px-in">
          <span>Network</span>
          <select value={chain} onChange={(e) => pickChain(e.target.value)}>
            {chains.map((c) => (
              <option key={c} value={c}>
                {options.find((o) => o.chain === c)?.chain_name ?? c}
              </option>
            ))}
          </select>
        </label>
        <label className="px-in">
          <span>Token</span>
          <select value={asset} onChange={(e) => setAsset(e.target.value)}>
            {tokensOnChain.map((o) => (
              <option key={o.asset_id} value={o.asset_id}>
                {o.symbol}
              </option>
            ))}
          </select>
        </label>
      </div>
      <label className="px-in">
        <span>Your {chosen?.chain_name ?? ""} address, for refunds only</span>
        <input value={refund} spellCheck={false} placeholder="Paste an address you control" onChange={(e) => setRefund(e.target.value)} />
      </label>
      <button className="px-btn dark full" onClick={() => void start()} disabled={!refundOk || busy}>
        {busy ? <Loader2 size={15} className="px-spin" aria-hidden /> : null} Show payment QR
      </button>
      <p className="px-fine">If a payment fails, it is returned to that address.</p>
      {problem && (
        <div className="alert" role="alert">
          {problem}
        </div>
      )}
    </div>
  );
}
