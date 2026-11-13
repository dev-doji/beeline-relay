"use client";

import { use, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { QRCodeSVG } from "qrcode.react";
import { ArrowUpRight, Check, Copy, QrCode, ShieldCheck } from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import { lineTotal } from "@/components/invoice/Receipt";
import {
  ApiError,
  type Invoice,
  type PayinOption,
  type PayinQuote,
  MAINNET_TX_URL,
  TX_URL,
  claimPayment,
  createPayin,
  formatUSDC,
  getPayin,
  getPayinOptions,
  invoiceMemo,
  shortKey,
  tokenContract,
} from "@/lib/api";
import { usePay } from "./usePay";
import "./checkout.css";

/**
 * The customer's checkout, one flat card (modelled on the Rhodium checkout, without the bank leg):
 *   merchant, order, the amount, then the way to pay, then a receipt.
 * Nothing the customer types or taps marks anything paid. "I've sent it, check now" asks the server, which
 * asks NEAR Intents and the chain; the page only shows what the server says (docs/PAYMENT_FLOW.md).
 */

const TERMINAL = new Set(["paid", "incomplete", "refunded", "failed", "expired"]);
const GAS = "30000000000000"; // 30 Tgas, plenty for one ft_transfer
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

function useCountdown(deadline: string | undefined, active: boolean) {
  const [left, setLeft] = useState<number | null>(null);
  useEffect(() => {
    if (!deadline || !active) return;
    const tick = () => setLeft(Math.max(0, Math.floor((new Date(deadline).getTime() - Date.now()) / 1000)));
    tick();
    const t = setInterval(tick, 1000);
    return () => clearInterval(t);
  }, [deadline, active]);
  return left === null ? "" : `${Math.floor(left / 60)}:${String(left % 60).padStart(2, "0")}`;
}

function Waiting({ children }: { children: React.ReactNode }) {
  return (
    <div className="ck-waiting" role="status">
      <span className="ck-spin" aria-hidden /> <span>{children}</span>
    </div>
  );
}

export function PayContent({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const { invoice, loadError, reload } = usePay(id);
  const [quote, setQuote] = useState<PayinQuote | null>(null);
  const [method, setMethod] = useState<"wallet" | "qr" | null>(null);

  if (loadError) {
    return (
      <div className="ck-card">
        <Brand />
        <div className="ck-err" role="alert">
          {loadError}
        </div>
        <Link className="ck-btn secondary" href="/chat" style={{ textDecoration: "none" }}>
          Make an invoice
        </Link>
      </div>
    );
  }
  if (!invoice) {
    return (
      <div className="ck-card" aria-busy>
        <Brand />
        <Waiting>Loading your invoice…</Waiting>
      </div>
    );
  }

  const real = invoice.network === "mainnet";
  const paid = invoice.status === "paid";
  const live = quote && !TERMINAL.has(quote.status) ? quote : null;
  const viaIntents = invoice.payment?.verification === "intents-delivered";
  const canLink = !!invoice.paid_tx && !invoice.paid_tx.startsWith("intents:");
  const ref = invoice.id.slice(-6).toUpperCase();

  return (
    <div className="ck-card">
      <Brand />
      <h1 className="ck-h1">{invoice.vendor_name}</h1>
      <div className="ck-order">
        Invoice {ref} · due {new Date(invoice.expires_at).toLocaleDateString(undefined, { month: "short", day: "numeric" })}
      </div>
      <ul className="ck-items">
        {invoice.items.map((it, i) => (
          <li key={i}>
            <span>
              {it.quantity} × {it.name}
            </span>
            <span>{formatUSDC(lineTotal(it))} USDC</span>
          </li>
        ))}
      </ul>

      {/* The amount to act on leads: once a payment address exists, it is what the customer sends. */}
      {live && !paid ? (
        <div className="ck-amount">
          {live.amount_in}
          <span className="ck-unit">{live.symbol}</span>
          <span className="ck-sub">Seller receives {formatUSDC(invoice.amount)} USDC</span>
        </div>
      ) : (
        <div className="ck-amount">
          {formatUSDC(invoice.amount)}
          <span className="ck-unit">USDC</span>
        </div>
      )}
      <div className="ck-chip">
        <i className={real ? "real" : ""} />
        {live ? `${live.symbol} on ${live.chain_name}` : real ? "Real money · pay from any network" : "Test USDC on NEAR testnet · no real money"}
      </div>

      {paid ? (
        <div className="ck-ok">
          <div className="ck-check">
            <Check aria-hidden />
          </div>
          <h2>Payment received</h2>
          <span className="ck-badge">
            <ShieldCheck size={12} aria-hidden />
            {viaIntents ? "Delivered by NEAR Intents, confirmed by Beeline" : invoice.payment ? "Verified on NEAR" : "Paid"}
          </span>
          {canLink && invoice.paid_tx && (
            <>
              <br />
              <a className="ck-explorer" href={(real ? MAINNET_TX_URL : TX_URL) + invoice.paid_tx} target="_blank" rel="noreferrer">
                View the transaction <ArrowUpRight size={14} aria-hidden />
              </a>
              <div className="ck-tx">{shortKey(invoice.paid_tx, 10)}</div>
            </>
          )}
          <br />
          <Link className="ck-explorer" href={`/receipt/${invoice.id}`}>
            View your receipt
          </Link>
        </div>
      ) : !method ? (
        <ChoosePay real={real} onPick={setMethod} />
      ) : method === "qr" ? (
        <>
          <RealPay invoice={invoice} quote={quote} setQuote={setQuote} onPaid={() => void reload()} />
          <BackLink onClick={() => setMethod(null)} />
        </>
      ) : (
        <>
          <WalletPay invoice={invoice} onPaid={() => void reload()} />
          <BackLink onClick={() => setMethod(null)} />
        </>
      )}

      <div className="ck-foot">
        <ShieldCheck size={12} aria-hidden style={{ verticalAlign: "-2px" }} /> Protected by Beeline: this invoice is signed by {invoice.vendor_name}. If anyone edits the payee or
        amount, the signature breaks and the payment is blocked. It works once.
        <br />
        <Link href={`/pay/${invoice.id}/security-test`}>Security test</Link> · <Link href="/attacks">Attack log</Link>
      </div>
    </div>
  );
}

/** Step one, as on Rhodium's checkout: how do you want to pay? A wallet only opens when the customer picks it. */
function ChoosePay({ real, onPick }: { real: boolean; onPick: (m: "wallet" | "qr") => void }) {
  return (
    <>
      <div className="ck-label">How do you want to pay?</div>
      <button className="ck-btn" onClick={() => onPick("qr")} disabled={!real}>
        <QrCode aria-hidden /> Scan a QR code
        <span className="ck-sub2">any wallet or exchange</span>
      </button>
      <button className="ck-btn secondary" onClick={() => onPick("wallet")}>
        Pay with a NEAR wallet
      </button>
      <div className="ck-hint">
        {real ? (
          <>
            <b>You do not need a NEAR wallet.</b> Pick the network you hold money on (Base, Ethereum, BNB Chain, Solana, Tron, TON and more), then scan or copy the address with
            Binance, Bybit, Tokkenly, MetaMask, Trust or any wallet. NEAR Intents converts it behind the scenes and the seller receives USDC on NEAR. You get a receipt.
          </>
        ) : (
          <>
            Paying from other wallets and exchanges works on <b>real-money invoices</b>: NEAR Intents has no test network. This is a test invoice, so use a NEAR testnet wallet.
          </>
        )}
      </div>
    </>
  );
}

function BackLink({ onClick }: { onClick: () => void }) {
  return (
    <button className="ck-btn secondary" onClick={onClick} style={{ marginTop: 10 }}>
      Change how I pay
    </button>
  );
}

function Brand() {
  return (
    <div className="ck-brand">
      <span className="ck-logo">
        <BeeMark size={12} onDark />
      </span>
      Secure checkout
    </div>
  );
}

function CopyButton({ value, label }: { value: string; label: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <button
      type="button"
      className="ck-btn"
      onClick={async () => {
        try {
          await navigator.clipboard.writeText(value);
        } catch {
          window.prompt(`Copy the ${label.toLowerCase()}:`, value);
        }
        setCopied(true);
        setTimeout(() => setCopied(false), 1600);
      }}
    >
      {copied ? <Check aria-hidden /> : <Copy aria-hidden />} {copied ? "Copied" : `Copy ${label.toLowerCase()}`}
    </button>
  );
}

/** Pay from a NEAR wallet: the wallet itself reports the transaction; the server reads it from the chain. */
function useWalletPay(invoice: Invoice, onPaid: () => void) {
  const [state, setState] = useState<{ kind: "idle" } | { kind: "working"; text: string } | { kind: "problem"; text: string }>({ kind: "idle" });
  const running = useRef(false);
  const real = invoice.network === "mainnet";

  async function confirm(txHash: string, from: string) {
    for (let i = 0; i < 30; i++) {
      try {
        const r = await claimPayment(invoice.id, txHash, from);
        if (r.status === "verified") {
          setState({ kind: "idle" });
          onPaid();
          return;
        }
        setState({ kind: "working", text: "Confirming on NEAR. This takes a few seconds…" });
      } catch (e) {
        setState({ kind: "problem", text: e instanceof ApiError ? e.message : "Could not check the payment. Try again in a moment." });
        return;
      }
      await sleep(3000);
    }
    setState({ kind: "problem", text: "We could not confirm this payment on NEAR yet. If it went through it will show here shortly: reload this page in a minute." });
  }

  async function pay() {
    if (running.current) return;
    running.current = true;
    try {
      setState({ kind: "working", text: "Opening your wallet…" });
      const { NearConnector } = await import("@hot-labs/near-connect"); // loaded now: it needs a browser
      const connector = new NearConnector({ network: real ? "mainnet" : "testnet" });
      const wallet = await connector.connect();
      const account = (await wallet.getAccounts())[0]?.accountId;
      if (!account) throw new Error("no account");
      setState({ kind: "working", text: "Confirm in your wallet…" });
      const outcome = await wallet.signAndSendTransaction({
        signerId: account,
        receiverId: tokenContract(invoice.asset),
        actions: [{ type: "FunctionCall", params: { methodName: "ft_transfer", args: { receiver_id: invoice.payout, amount: invoice.amount, memo: invoiceMemo(invoice.id) }, gas: GAS, deposit: "1" } }],
      });
      const o = outcome as { transaction?: { hash?: string }; transaction_outcome?: { id?: string } };
      const txHash = o.transaction?.hash ?? o.transaction_outcome?.id;
      if (!txHash) throw new Error("no hash");
      setState({ kind: "working", text: "Confirming on NEAR. This takes a few seconds…" });
      await confirm(txHash, account);
    } catch {
      setState({ kind: "problem", text: "Cancelled in your wallet, so nothing was charged. You can try again." });
    } finally {
      running.current = false;
    }
  }
  return { state, pay };
}

/** A test invoice: the only way to pay is a NEAR testnet wallet. */
function WalletPay({ invoice, onPaid }: { invoice: Invoice; onPaid: () => void }) {
  const { state, pay } = useWalletPay(invoice, onPaid);
  return (
    <>
      <button className="ck-btn" onClick={() => void pay()} disabled={state.kind === "working"}>
        Pay with a NEAR wallet
      </button>
      {state.kind === "working" && <Waiting>{state.text}</Waiting>}
      {state.kind === "problem" && (
        <div className="ck-err" role="alert">
          {state.text}
        </div>
      )}
      <div className="ck-hint">
        {invoice.network === "mainnet" ? (
          <>Your wallet will ask you to confirm. The money goes straight to the seller.</>
        ) : (
          <>
            Your wallet will ask you to confirm. This is a test invoice on NEAR testnet: <b>no real money moves</b>.
          </>
        )}
      </div>
    </>
  );
}

/** A real-money invoice: pick what you hold, then scan the QR code or copy the address. */
function RealPay({ invoice, quote, setQuote, onPaid }: { invoice: Invoice; quote: PayinQuote | null; setQuote: (q: PayinQuote | null) => void; onPaid: () => void }) {
  const [options, setOptions] = useState<PayinOption[] | null>(null);
  const [optError, setOptError] = useState<string | null>(null);
  const [asset, setAsset] = useState("");
  const [chain, setChain] = useState("");
  const [refund, setRefund] = useState("");
  const [busy, setBusy] = useState(false);
  const [problem, setProblem] = useState<string | null>(null);
  const [checking, setChecking] = useState(false);
  const [checkNote, setCheckNote] = useState<string | null>(null);
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
      .catch((e) => on && setOptError(e instanceof ApiError ? e.message : "Could not load the ways to pay."));
    getPayin(invoice.id)
      .then((r) => on && r.quote && !TERMINAL.has(r.quote.status) && setQuote(r.quote)) // a reload picks the address up again
      .catch(() => undefined);
    return () => {
      on = false;
    };
  }, [invoice.id, setQuote]);

  // While an address is live the page watches for the payment, whoever sends it and from wherever.
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
        /* the next tick tries again */
      }
    };
    const t = setInterval(() => void tick(), 4000);
    return () => {
      on = false;
      clearInterval(t);
    };
  }, [live, invoice.id, onPaid, setQuote]);

  const chosen = useMemo(() => options?.find((o) => o.asset_id === asset), [options, asset]);
  const chains = useMemo(() => Array.from(new Set((options ?? []).map((o) => o.chain))), [options]);
  const tokens = (options ?? []).filter((o) => o.chain === chain);

  async function start() {
    if (busy || !chosen) return;
    setBusy(true);
    setProblem(null);
    try {
      setQuote((await createPayin(invoice.id, chosen.asset_id, refund.trim())).quote);
    } catch (e) {
      setProblem(e instanceof ApiError ? e.message : "Could not get a payment address. Try again in a moment.");
    } finally {
      setBusy(false);
    }
  }

  // "I've sent it, check now" asks the server. Pressing it on an unpaid invoice changes nothing.
  async function checkNow() {
    setChecking(true);
    setCheckNote(null);
    try {
      const r = await getPayin(invoice.id);
      if (r.quote) setQuote(r.quote);
      if (r.invoice_status === "paid") {
        paidOnce.current = true;
        onPaid();
      } else {
        setCheckNote("Not showing yet. It can take a few minutes to confirm: we are still watching.");
      }
    } catch {
      setCheckNote("Could not check just now. We are still watching for your payment.");
    } finally {
      setChecking(false);
    }
  }

  if (optError) {
    return (
      <div className="ck-err" role="alert">
        {optError}
      </div>
    );
  }
  if (!options) return <Waiting>Loading ways to pay…</Waiting>;

  // ---- A payment address exists ----
  if (quote) {
    const finished = TERMINAL.has(quote.status);
    if (finished) {
      return (
        <>
          <div className={quote.status === "paid" ? "ck-note" : "ck-err"} role="alert">
            {quote.message}
          </div>
          {quote.status !== "paid" && (
            <button className="ck-btn secondary" onClick={() => setQuote(null)}>
              Start again
            </button>
          )}
        </>
      );
    }
    return (
      <>
        <div className="ck-rows">
          <div className="ck-row">
            <span className="k">Send exactly</span>
            <b>
              {quote.amount_in} {quote.symbol}
            </b>
          </div>
          <div className="ck-row">
            <span className="k">Network</span>
            <b>{quote.chain_name}</b>
          </div>
          <div className="ck-row">
            <span className="k">Seller receives</span>
            <b>{formatUSDC(invoice.amount)} USDC</b>
          </div>
          {countdown && (
            <div className="ck-row">
              <span className="k">Address valid for</span>
              <b>{countdown}</b>
            </div>
          )}
        </div>
        <div className="ck-qr">
          <div>
            <QRCodeSVG value={quote.deposit_address} size={168} level="M" marginSize={1} />
          </div>
        </div>
        <div className="ck-label">Scan, or send to this address</div>
        <div className="ck-addr">{quote.deposit_address}</div>
        {quote.deposit_memo && (
          <>
            <div className="ck-label">Memo (required)</div>
            <div className="ck-addr">{quote.deposit_memo}</div>
          </>
        )}
        <CopyButton value={quote.deposit_address} label="Address" />
        <button className="ck-btn secondary" onClick={() => void checkNow()} disabled={checking}>
          <Check aria-hidden /> {checking ? "Checking…" : "I’ve sent it, check now"}
        </button>
        <Waiting>{quote.status === "waiting" ? "Waiting for your payment…" : quote.message}</Waiting>
        {checkNote && <div className="ck-hint">{checkNote}</div>}
        <div className="ck-hint">
          Send <b>{quote.symbol}</b> on <b>{quote.chain_name}</b> only. Another token, another network or a smaller amount cannot be matched and is refunded. Some exchanges take
          their withdrawal fee out of the amount you enter: make sure the address receives at least <b>{quote.amount_in}</b>. This address is for this invoice only, and the page
          updates by itself.
        </div>
      </>
    );
  }

  // ---- Choose what to pay with ----
  const refundOk = refund.trim().length >= 20;
  return (
    <>
      <div className="ck-two">
        <label className="ck-field">
          <span>Network</span>
          <select
            value={chain}
            onChange={(e) => {
              setChain(e.target.value);
              const first = options.find((o) => o.chain === e.target.value);
              if (first) setAsset(first.asset_id);
            }}
          >
            {chains.map((c) => (
              <option key={c} value={c}>
                {options.find((o) => o.chain === c)?.chain_name ?? c}
              </option>
            ))}
          </select>
        </label>
        <label className="ck-field">
          <span>Token</span>
          <select value={asset} onChange={(e) => setAsset(e.target.value)}>
            {tokens.map((o) => (
              <option key={o.asset_id} value={o.asset_id}>
                {o.symbol}
              </option>
            ))}
          </select>
        </label>
      </div>
      <label className="ck-field">
        <span>Your {chosen?.chain_name ?? ""} address, used only to refund you</span>
        <input value={refund} spellCheck={false} placeholder="Paste an address you control" onChange={(e) => setRefund(e.target.value)} />
      </label>
      <button className="ck-btn" onClick={() => void start()} disabled={!refundOk || busy}>
        {busy ? "Setting up…" : "Continue"}
      </button>
      {problem && (
        <div className="ck-err" role="alert">
          {problem}
        </div>
      )}
      <div className="ck-hint">Scan the QR code on the next step with any wallet or exchange. Nothing to paste back.</div>
    </>
  );
}
