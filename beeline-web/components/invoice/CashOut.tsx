"use client";

import { useEffect, useState } from "react";
import { Copy, Landmark } from "lucide-react";
import {
  ApiError,
  type CashoutPrepared,
  type CashoutQuote,
  type CashoutStatus,
  cashoutDeposit,
  cashoutPrepare,
  cashoutQuote,
  cashoutStatus,
} from "@/lib/api";

const AMOUNT = /^(0|[1-9]\d{0,8})(\.\d{1,6})?$/;
const NEAR = /^(([a-z\d]+[-_])*[a-z\d]+\.)*([a-z\d]+[-_])*[a-z\d]+$/;

/**
 * Cash out to a Nigerian bank: USDC on NEAR -> (NEAR Intents) -> USDC on Base -> (Switch) -> naira.
 *
 * 1. "Get a price" is read-only.
 * 2. "Prepare" opens a Switch payout session and gets a real NEAR Intents deposit address. Still no
 *    money moves, and Beeline never holds a key.
 * 3. The vendor sends the shown amount from THEIR OWN NEAR wallet. Only then does money move.
 * 4. The panel tracks each leg and labels which provider reported it.
 */
export function CashOut() {
  const [amount, setAmount] = useState("5");
  const [quote, setQuote] = useState<CashoutQuote | null>(null);
  const [nearAccount, setNearAccount] = useState("");
  const [bank, setBank] = useState({ bank_code: "", account_number: "", holder_name: "" });
  const [prepared, setPrepared] = useState<CashoutPrepared | null>(null);
  const [status, setStatus] = useState<CashoutStatus | null>(null);
  const [txHash, setTxHash] = useState("");
  const [noticed, setNoticed] = useState<string | null>(null);
  const [busy, setBusy] = useState<"" | "quote" | "prepare" | "notice">("");
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState("");

  const amountOk = AMOUNT.test(amount) && Number(amount) > 0;
  const nearOk = nearAccount.length >= 2 && nearAccount.length <= 64 && NEAR.test(nearAccount);
  const bankOk = /^\d{3,6}$/.test(bank.bank_code) && /^\d{10}$/.test(bank.account_number) && bank.holder_name.trim().length >= 2;
  const route = prepared?.route ?? null;
  const reference = prepared?.payout.reference;

  async function run<T>(kind: "quote" | "prepare" | "notice", fn: () => Promise<T>, set: (v: T) => void) {
    setBusy(kind);
    setError(null);
    try {
      set(await fn());
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Something went wrong. Try again.");
    } finally {
      setBusy("");
    }
  }

  // Poll both providers every 10 s while a real route is waiting for or processing the transfer.
  useEffect(() => {
    if (!reference || !route) return;
    let stop = false;
    const tick = () =>
      cashoutStatus(reference, route.deposit_address)
        .then((s) => !stop && setStatus(s))
        .catch(() => undefined);
    void tick();
    const t = setInterval(() => void tick(), 10_000);
    return () => {
      stop = true;
      clearInterval(t);
    };
  }, [reference, route]);

  async function copy(label: string, value: string) {
    try {
      await navigator.clipboard?.writeText(value);
      setCopied(label);
      setTimeout(() => setCopied(""), 1500);
    } catch {
      /* the value is visible on screen */
    }
  }

  const ngn = quote?.payout?.destination;
  const switchStatus = status?.data?.status ?? prepared?.payout.status;
  return (
    <div className="panel">
      <h2>
        <Landmark size={18} aria-hidden style={{ verticalAlign: "-3px" }} /> Cash out to your bank
      </h2>
      <p className="help">
        Get paid in USDC on NEAR, then cash out to a Nigerian bank account in naira. NEAR Intents moves the USDC from NEAR to Base, then Switch pays your bank.
        You send the USDC from your own NEAR wallet: Beeline never holds your keys and cannot move your money.
      </p>
      <label className="field">
        Amount to cash out (USDC)
        <input value={amount} inputMode="decimal" onChange={(e) => (setAmount(e.target.value), setQuote(null), setPrepared(null), setStatus(null))} />
        {!amountOk && <span className="err">Enter an amount like 5 or 5.50.</span>}
      </label>
      <button className="btn" disabled={!amountOk || busy !== ""} onClick={() => void run("quote", () => cashoutQuote(amount), setQuote)}>
        {busy === "quote" ? "Getting prices…" : "Get a price (no money moves)"}
      </button>

      {error && (
        <div className="alert" role="alert">
          {error}
        </div>
      )}

      {quote && (
        <div style={{ display: "grid", gap: 8, marginTop: 12 }}>
          <p>
            <b>{ngn?.amount != null ? `≈ ${ngn.amount.toLocaleString("en-US")} ${ngn.currency ?? "NGN"}` : "Switch returned a quote"}</b> for {quote.amount_usdc} USDC
            {quote.payout_basis === "bridge_minimum" ? " (priced on the least the bridge guarantees)" : " (bridge not priced, so this is an unadjusted estimate)"}
          </p>
          <ol className="help" style={{ paddingLeft: 18 }}>
            <li>
              USDC on NEAR → USDC on Base via NEAR Intents:{" "}
              {quote.route ? `${quote.route.amount_out_usdc} USDC out (at least ${quote.route.min_amount_out_usdc}), about ${Math.round(quote.route.time_estimate_seconds)} s, dry quote` : quote.route_error}
            </li>
            <li>USDC on Base → naira to your bank via Switch{quote.payout.rate ? `, rate ${quote.payout.rate}` : ""}</li>
          </ol>
          <span className="help">{quote.note}</span>

          <h3 style={{ margin: "8px 0 0" }}>Prepare the cash-out{quote.mode === "sandbox" ? " (sandbox)" : ""}</h3>
          <label className="field">
            Your NEAR account (the one you will send from)
            <input value={nearAccount} spellCheck={false} placeholder="yourshop.near" onChange={(e) => setNearAccount(e.target.value.trim().toLowerCase())} />
            {nearAccount && !nearOk && <span className="err">Enter a NEAR account like yourshop.near.</span>}
          </label>
          <div className="row2">
            <label className="field">
              Bank code
              <input value={bank.bank_code} inputMode="numeric" onChange={(e) => setBank({ ...bank, bank_code: e.target.value.trim() })} />
            </label>
            <label className="field">
              Account number (10 digits)
              <input value={bank.account_number} inputMode="numeric" onChange={(e) => setBank({ ...bank, account_number: e.target.value.trim() })} />
            </label>
          </div>
          <label className="field">
            Account name
            <input value={bank.holder_name} onChange={(e) => setBank({ ...bank, holder_name: e.target.value })} />
          </label>
          <button
            className="btn btn-honey"
            disabled={!nearOk || !bankOk || !amountOk || busy !== "" || !!prepared}
            onClick={() => void run("prepare", () => cashoutPrepare(amount, nearAccount, bank), (p) => (setPrepared(p), setStatus(null)))}
          >
            {busy === "prepare" ? "Preparing…" : prepared ? "Prepared" : "Prepare cash-out (still no money moves)"}
          </button>
        </div>
      )}

      {prepared && (
        <div style={{ display: "grid", gap: 8, marginTop: 12 }} role="region" aria-label="Your cash-out">
          <p className="help">
            Switch (provider-reported): payout <b>{switchStatus}</b>, about {Number(prepared.payout.naira).toLocaleString("en-US")}{" "}
            {prepared.payout.currency} to your bank. Reference {prepared.payout.reference}.
          </p>
          {!route ? (
            <div className="alert" role="status">
              {prepared.note}
            </div>
          ) : (
            <>
              <div className="alert" role="status">
                <b>Nothing has moved yet.</b> To cash out, send exactly <b>{route.send_usdc} USDC</b> (NEAR USDC, contract {route.token_contract}) from{" "}
                <b>{route.refund_to}</b> to the address below
                {route.deadline ? `, before ${new Date(route.deadline).toLocaleTimeString()}` : ""}. Sending a different amount, from another account, or late can fail or be refunded
                to {route.refund_to}. This is real money: check every character.
              </div>
              <div className="linkbox">
                <code>{route.deposit_address}</code>
                <button className="icon-btn" aria-label="Copy deposit address" onClick={() => void copy("addr", route.deposit_address)}>
                  <Copy size={16} />
                </button>
              </div>
              {copied && <span className="help">Copied.</span>}
              <span className="help">
                The address is a NEAR account. If your wallet asks you to register the token for it, approve that. The route delivers {route.delivers_usdc} USDC to Switch on Base
                (minimum you may send {route.min_send_usdc}).
              </span>
              <label className="field">
                After you send, paste the NEAR transaction hash (optional, speeds up detection)
                <input value={txHash} spellCheck={false} onChange={(e) => setTxHash(e.target.value.trim())} />
              </label>
              <button
                className="btn"
                disabled={txHash.length < 43 || busy !== ""}
                onClick={() => void run("notice", () => cashoutDeposit(route.deposit_address, txHash, route.refund_to), (r) => setNoticed(r.status))}
              >
                Tell NEAR Intents I sent it
              </button>
              {noticed && <span className="help">NEAR Intents (provider-reported): {noticed}</span>}
              <p className="help">
                Bridge, NEAR Intents (provider-reported): <b>{status?.bridge?.status ?? "waiting"}</b>. Payout, Switch (provider-reported): <b>{switchStatus}</b>. The cash-out is
                only done when Switch says COMPLETED. Beeline checks again every 10 seconds.
              </p>
            </>
          )}
        </div>
      )}
    </div>
  );
}
