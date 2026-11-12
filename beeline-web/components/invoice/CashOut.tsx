"use client";

import { useState } from "react";
import { Landmark } from "lucide-react";
import { ApiError, type CashoutQuote, type CashoutStart, cashoutQuote, cashoutStart } from "@/lib/api";

const AMOUNT = /^(0|[1-9]\d{0,8})(\.\d{1,6})?$/;

/**
 * Cash out to a Nigerian bank: USDC on NEAR -> (NEAR Intents) -> USDC on Base -> (Switch) -> naira.
 * The quote is read-only. "Start" opens a Switch SANDBOX payout session only; moving the USDC is a
 * wallet step that is not built, and the panel says so.
 */
export function CashOut() {
  const [amount, setAmount] = useState("25");
  const [quote, setQuote] = useState<CashoutQuote | null>(null);
  const [started, setStarted] = useState<CashoutStart | null>(null);
  const [bank, setBank] = useState({ bank_code: "", account_number: "", holder_name: "" });
  const [busy, setBusy] = useState<"" | "quote" | "start">("");
  const [error, setError] = useState<string | null>(null);

  const amountOk = AMOUNT.test(amount) && Number(amount) > 0;
  const bankOk = /^\d{3,6}$/.test(bank.bank_code) && /^\d{10}$/.test(bank.account_number) && bank.holder_name.trim().length >= 2;

  async function run<T>(kind: "quote" | "start", fn: () => Promise<T>, set: (v: T) => void) {
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

  const ngn = quote?.payout?.destination;
  return (
    <div className="panel">
      <h2>
        <Landmark size={18} aria-hidden style={{ verticalAlign: "-3px" }} /> Cash out to your bank (preview)
      </h2>
      <p className="help">
        Get paid in USDC on NEAR, then cash out to a Nigerian bank account in naira. Behind the scenes: NEAR Intents moves USDC from NEAR to Base,
        then Switch pays your bank. This is a price preview. No money moves.
      </p>
      <label className="field">
        Amount to cash out (USDC)
        <input value={amount} inputMode="decimal" onChange={(e) => (setAmount(e.target.value), setQuote(null), setStarted(null))} />
        {!amountOk && <span className="err">Enter an amount like 25 or 25.50.</span>}
      </label>
      <button className="btn" disabled={!amountOk || busy !== ""} onClick={() => void run("quote", () => cashoutQuote(amount), setQuote)}>
        {busy === "quote" ? "Getting prices…" : "Get a price"}
      </button>

      {error && (
        <div className="alert" role="alert">
          {error}
        </div>
      )}

      {quote && (
        <div style={{ display: "grid", gap: 8, marginTop: 12 }}>
          <p>
            <b>
              {ngn?.amount != null ? `≈ ${ngn.amount.toLocaleString("en-US")} ${ngn.currency ?? "NGN"}` : "Switch returned a quote"}
            </b>{" "}
            for {quote.amount_usdc} USDC
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

          {quote.mode === "sandbox" && (
            <>
              <h3 style={{ margin: "8px 0 0" }}>Try a sandbox payout session</h3>
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
              <button className="btn" disabled={!bankOk || busy !== ""} onClick={() => void run("start", () => cashoutStart(amount, bank), setStarted)}>
                {busy === "start" ? "Opening…" : "Open sandbox payout"}
              </button>
            </>
          )}
          {started && (
            <div className="help" role="status">
              Sandbox session {started.data.reference} is <b>{started.data.status}</b>. {started.note}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
