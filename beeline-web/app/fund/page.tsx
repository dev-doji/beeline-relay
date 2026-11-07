"use client";

import { useEffect, useMemo, useState } from "react";
import { ArrowDown, ArrowRight, Banknote, Info, Landmark, Smartphone } from "lucide-react";
import { AppFrame } from "@/components/AppFrame";
import { ApiError, type FundCorridor, type FundQuote, getFundCoverage, getFundQuote } from "@/lib/api";

const NAMES: Record<string, string> = {
  NG: "Nigeria", GH: "Ghana", KE: "Kenya", CD: "DR Congo", UG: "Uganda", CI: "Côte d’Ivoire", SN: "Senegal", RW: "Rwanda",
  BJ: "Benin", ML: "Mali", TZ: "Tanzania", ZM: "Zambia", SL: "Sierra Leone", MW: "Malawi", LR: "Liberia", GM: "Gambia",
  EG: "Egypt", BW: "Botswana", ZA: "South Africa", ET: "Ethiopia", GA: "Gabon", GN: "Guinea", CM: "Cameroon",
};

const flag = (cc: string) => String.fromCodePoint(...[...cc].map((c) => 127397 + c.charCodeAt(0)));
const money = (n: number, max = 2) => n.toLocaleString("en-US", { maximumFractionDigits: max });

/** Funding preview: a live quote from Switch for local currency to USDC. No money moves. */
export default function FundPage() {
  const [corridors, setCorridors] = useState<FundCorridor[] | null>(null);
  const [mode, setMode] = useState<"live" | "sandbox">("sandbox");
  const [country, setCountry] = useState("NG");
  const [channel, setChannel] = useState<string>("");
  const [amount, setAmount] = useState("50000");
  const [quote, setQuote] = useState<FundQuote | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    getFundCoverage()
      .then((r) => {
        setCorridors(r.data);
        setMode(r.mode);
      })
      .catch((e) => setError(e instanceof ApiError ? e.message : "Could not load the supported countries."));
  }, []);

  const corridor = useMemo(() => corridors?.find((c) => c.country === country), [corridors, country]);
  const currency = corridor?.currency[0] ?? "";
  const channels = corridor?.channel ?? [];
  const activeChannel = channel && channels.includes(channel as "BANK") ? channel : corridor?.default_channel ?? "";
  const n = Number(amount);
  const amountOk = amount.trim() !== "" && Number.isFinite(n) && n > 0;
  const limit = corridor && activeChannel ? corridor.payout_limit[activeChannel] : undefined;
  const limitText = limit && typeof limit === "object" ? `${limit.min ?? "?"} to ${limit.max ?? "?"} per transaction` : null;

  async function get() {
    if (!corridor || !amountOk || busy) return;
    setBusy(true);
    setError(null);
    try {
      const r = await getFundQuote({ country, amount: n, channel: activeChannel || undefined });
      setQuote(r.data);
    } catch (e) {
      setQuote(null);
      setError(e instanceof ApiError ? e.message : "Could not get a quote.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <AppFrame active="fund" title="Fund with local currency" icon={<Banknote size={16} aria-hidden />}>
      <div className="frame-inner">
        <div className="page-head">
          <span className="pill pill-honey">
            <Banknote size={13} aria-hidden /> Quote preview · Switch {mode === "live" ? "live" : "sandbox"} prices
          </span>
          <h1>From naira to a vault your agent can’t abuse</h1>
          <p>
            Pay with bank transfer or mobile money, receive USDC. See today’s real rate below. This page only asks for a quote: no
            money moves.
          </p>
        </div>

        <div className="split">
          <div className="panel">
            <h2>How much?</h2>
            <label className="field">
              Country
              <select value={country} onChange={(e) => (setCountry(e.target.value), setChannel(""), setQuote(null))} disabled={!corridors}>
                {(corridors ?? [{ country: "NG", currency: ["NGN"] } as FundCorridor]).map((c) => (
                  <option key={c.country} value={c.country}>
                    {flag(c.country)} {NAMES[c.country] ?? c.country} ({c.currency.join(", ")})
                  </option>
                ))}
              </select>
            </label>

            {channels.length > 1 && (
              <div className="field" role="group" aria-label="Payment method">
                Pay with
                <div className="seg">
                  {channels.map((c) => (
                    <button key={c} type="button" className={c === activeChannel ? "on" : ""} onClick={() => (setChannel(c), setQuote(null))}>
                      {c === "BANK" ? <Landmark size={15} aria-hidden /> : <Smartphone size={15} aria-hidden />} {c === "BANK" ? "Bank transfer" : "Mobile money"}
                    </button>
                  ))}
                </div>
              </div>
            )}

            <label className="field">
              You pay ({currency || "…"})
              <input value={amount} inputMode="decimal" onChange={(e) => (setAmount(e.target.value.replace(/,/g, "").trim()), setQuote(null))} />
              {!amountOk && <span className="err">Enter an amount greater than zero</span>}
              {limitText && <span className="help">Switch limits: {limitText}. {corridor && activeChannel && corridor.settlement_time[activeChannel] ? `Settles in ${corridor.settlement_time[activeChannel]}.` : ""}</span>}
            </label>

            {error && (
              <div className="alert" role="alert">
                {error}
              </div>
            )}
            <button className="btn btn-honey" onClick={() => void get()} disabled={!corridor || !amountOk || busy}>
              {busy ? "Asking Switch…" : "Get a live quote"} <ArrowRight size={18} aria-hidden />
            </button>
          </div>

          <div style={{ display: "grid", gap: 16, alignContent: "start" }}>
            <div className="panel quote-card" aria-live="polite">
              {quote ? (
                <>
                  <span className="cap-label">You pay</span>
                  <b className="q-big">
                    {money(quote.source.amount, 2)} <small>{quote.source.currency}</small>
                  </b>
                  <span className="q-arrow" aria-hidden>
                    <ArrowDown size={18} />
                  </span>
                  <span className="cap-label">You receive on {quote.destination.network.charAt(0) + quote.destination.network.slice(1).toLowerCase()}</span>
                  <b className="q-big ok">
                    {money(quote.destination.amount, 2)} <small>{quote.destination.currency}</small>
                  </b>
                  <dl className="q-meta">
                    <div>
                      <dt>Rate</dt>
                      <dd>
                        1 USD ≈ {money(quote.rate, 2)} {quote.source.currency}
                      </dd>
                    </div>
                    <div>
                      <dt>Worth</dt>
                      <dd>${money(quote.destination.amount_usd, 2)}</dd>
                    </div>
                    <div>
                      <dt>Settles in</dt>
                      <dd>{quote.settlement}</dd>
                    </div>
                    <div>
                      <dt>Quote valid until</dt>
                      <dd>{new Date(quote.expiry).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</dd>
                    </div>
                  </dl>
                </>
              ) : (
                <p className="q-empty">Your live quote appears here.</p>
              )}
            </div>

            <div className="panel route">
              <h2>The route</h2>
              <ol>
                <li>
                  <b>Pay</b> naira (or another currency) by bank transfer or mobile money through <b>Switch</b>.
                </li>
                <li>
                  <b>Receive</b> USDC on Base.
                </li>
                <li className="todo">
                  <b>Bridge</b> into the Beeline vault on NEAR with NEAR Intents. <span>Not built yet</span>
                </li>
                <li>
                  <b>Your agents pay</b> vendors from the vault, guarded by every lock.
                </li>
              </ol>
              <p className="route-note">
                <Info size={14} aria-hidden /> Switch doesn’t support NEAR, so step 3 needs NEAR Intents and a small mainnet payment to prove. Until then this
                page is a quote only.
              </p>
            </div>
          </div>
        </div>
      </div>
    </AppFrame>
  );
}
