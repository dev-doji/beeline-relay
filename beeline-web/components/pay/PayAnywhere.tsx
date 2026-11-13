"use client";

import { useEffect, useMemo, useRef, useState } from "react";
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
  { key: ["waiting"], text: "Waiting for your deposit" },
  { key: ["processing"], text: "Deposit seen, converting" },
  { key: ["confirming", "paid"], text: "Delivered to the vendor" },
];

/**
 * Pay a real-money invoice from any chain or exchange: pick what you hold, say where a refund should go, and
 * send to the one-time address NEAR Intents gives. The page only shows progress. The server marks the invoice
 * paid, and only when NEAR Intents reports delivery AND the vendor's balance on NEAR rose (docs/PAYMENT_FLOW.md).
 */
export function PayAnywhere({ invoice, onPaid }: { invoice: Invoice; onPaid: () => void }) {
  const [options, setOptions] = useState<PayinOption[] | null>(null);
  const [optError, setOptError] = useState<string | null>(null);
  const [asset, setAsset] = useState("");
  const [refund, setRefund] = useState("");
  const [mine, setMine] = useState(false);
  const [quote, setQuote] = useState<PayinQuote | null>(null);
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const paidOnce = useRef(false);

  useEffect(() => {
    let live = true;
    getPayinOptions()
      .then((r) => {
        if (!live) return;
        setOptions(r.data);
        if (r.data[0]) setAsset(r.data[0].asset_id);
      })
      .catch((e) => live && setOptError(e instanceof ApiError ? e.message : "Could not load the list of tokens."));
    // A quote made earlier (page reloaded) is picked up again.
    getPayin(invoice.id)
      .then((r) => live && r.quote && !TERMINAL.has(r.quote.status) && setQuote(r.quote))
      .catch(() => undefined);
    return () => {
      live = false;
    };
  }, [invoice.id]);

  // While a deposit address is live, ask the server how it is going.
  useEffect(() => {
    if (!quote || TERMINAL.has(quote.status)) return;
    let live = true;
    const tick = async () => {
      try {
        const r = await getPayin(invoice.id);
        if (!live) return;
        if (r.quote) setQuote(r.quote);
        if (r.invoice_status === "paid" && !paidOnce.current) {
          paidOnce.current = true;
          onPaid();
        }
      } catch {
        /* keep the last known state; the next tick tries again */
      }
    };
    const t = setInterval(() => void tick(), 5000);
    return () => {
      live = false;
      clearInterval(t);
    };
  }, [quote, invoice.id, onPaid]);

  const chosen = useMemo(() => options?.find((o) => o.asset_id === asset), [options, asset]);
  const chains = useMemo(() => Array.from(new Set((options ?? []).map((o) => o.chain))), [options]);
  const [chain, setChain] = useState("");
  const activeChain = chain || chosen?.chain || "";
  const tokensOnChain = (options ?? []).filter((o) => o.chain === activeChain);

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
      setProblem(e instanceof ApiError ? e.message : "Could not get a deposit address. Try again in a moment.");
    } finally {
      setBusy(false);
    }
  }

  if (optError) return <div className="alert" role="alert">{optError}</div>;
  if (!options) {
    return (
      <div className="px-status" role="status">
        <Loader2 size={14} className="px-spin" aria-hidden /> Loading what you can pay with…
      </div>
    );
  }

  // ---- A deposit address exists ----
  if (quote) {
    const finished = TERMINAL.has(quote.status);
    const stepIdx = STEPS.findIndex((s) => s.key.includes(quote.status));
    return (
      <div className="px-sheet-sec">
        <b>
          Send {quote.amount_in} {quote.symbol} on {quote.chain_name}
        </b>
        {!finished && (
          <>
            <p>
              Send exactly this amount to the address below from any wallet or exchange. Send only {quote.symbol} on {quote.chain_name}: other tokens or
              networks can be lost.
            </p>
            <CopyRow label="Send to" value={quote.deposit_address} />
            <CopyRow label="Amount" value={`${quote.amount_in}`} />
            {quote.deposit_memo && <CopyRow label="Memo" value={quote.deposit_memo} />}
            <p className="px-fine">
              The address is one-time and valid until {new Date(quote.deadline).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}. If
              something goes wrong, NEAR Intents refunds to {quote.refund_to.slice(0, 8)}…{quote.refund_to.slice(-6)}.
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
        <p className="px-fine">Status from NEAR Intents: {quote.provider_status || "no deposit seen yet"} (provider-reported). Paid is confirmed by Beeline on NEAR.</p>
        {finished && quote.status !== "paid" && (
          <button className="px-btn full" onClick={() => setQuote(null)}>
            Start again
          </button>
        )}
      </div>
    );
  }

  // ---- Choose what to pay with ----
  const refundOk = refund.trim().length >= 20 && mine;
  return (
    <div className="px-sheet-sec">
      <b>Pay from any wallet or exchange</b>
      <p>Binance, Bybit, MetaMask, Phantom, Tonkeeper, Trust and more. You send USDC or USDT, the vendor receives USDC on NEAR.</p>
      <div className="px-grid2 tight">
        <label className="px-in">
          <span>Network</span>
          <select value={activeChain} onChange={(e) => pickChain(e.target.value)}>
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
        <span>Your own {chosen?.chain_name ?? ""} address (for a refund if anything goes wrong)</span>
        <input value={refund} spellCheck={false} placeholder="Paste your own address on that network" onChange={(e) => setRefund(e.target.value)} />
      </label>
      <label className="px-check">
        <input type="checkbox" checked={mine} onChange={(e) => setMine(e.target.checked)} />
        <span>This address is mine, on {chosen?.chain_name ?? "that network"}. Refunds go there. Do not use an exchange deposit address you do not control.</span>
      </label>
      <button className="px-btn dark full" onClick={() => void start()} disabled={!refundOk || busy}>
        {busy ? <Loader2 size={15} className="px-spin" aria-hidden /> : null} Get deposit address
      </button>
      {problem && (
        <div className="alert" role="alert">
          {problem}
        </div>
      )}
    </div>
  );
}
