"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight, Store } from "lucide-react";
import { AppFrame } from "@/components/AppFrame";
import { ApiError, createInvoice, formatUSDC, toBaseUnits } from "@/lib/api";

const NEAR_ACCOUNT = /^(([a-z\d]+[-_])*[a-z\d]+\.)*([a-z\d]+[-_])*[a-z\d]+$/;
const CAP = 5_000_000n;

/**
 * A permanent pay link. The vendor shares this one link for good; the payer chooses an amount and
 * Beeline signs a fresh single-use invoice for exactly that payer, payee and amount.
 */
export function OpenPay({ name, to, note }: { name: string; to: string; note: string }) {
  const router = useRouter();
  const [amount, setAmount] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const payee = to.trim().toLowerCase();
  const payeeOk = payee.length >= 2 && NEAR_ACCOUNT.test(payee);
  const shop = name.trim() || payee;
  const units = toBaseUnits(amount);
  const tooBig = !!units && BigInt(units) > CAP;
  const ready = payeeOk && !!units && !tooBig && !busy;

  async function go() {
    if (!ready || !units) return;
    setBusy(true);
    setError(null);
    try {
      const inv = await createInvoice({
        vendor_name: shop.slice(0, 60),
        vendor_handle: "",
        payout: payee,
        memo: note || "Payment",
        items: [{ name: (note || "Payment").slice(0, 80), quantity: 1, unit_price: units }],
      });
      router.push(`/pay/${inv.id}`);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Could not create your invoice.");
      setBusy(false);
    }
  }

  return (
    <AppFrame active="vendor" title="Pay" icon={<Store size={16} aria-hidden />}>
      <div className="frame-inner">
        <div className="page-head">
          <span className="pill pill-honey">
            <Store size={13} aria-hidden /> Permanent link · NEAR testnet
          </span>
          <h1>{payeeOk ? `Pay ${shop}` : "This link is incomplete"}</h1>
          <p>
            {payeeOk
              ? "Choose an amount. Beeline signs a one-time invoice for you, and the vault pays only this exact payee and amount, once."
              : "A permanent pay link needs a payee account. Ask the vendor to send it again."}
          </p>
        </div>
        {payeeOk && (
          <div className="panel" style={{ maxWidth: 520 }}>
            <h2>Payee</h2>
            <p className="help" style={{ wordBreak: "break-all" }}>
              {payee} {note && <>· {note}</>}
            </p>
            <p className="help">
              Check this account is the one you expect. This link carries the payee in its address; Beeline cannot tell you the vendor owns it.
            </p>
            <label className="field">
              Amount (test USDC)
              <input value={amount} inputMode="decimal" placeholder="0.50" onChange={(e) => setAmount(e.target.value)} />
              {amount && !units && <span className="err">Enter an amount like 0.50.</span>}
              {tooBig && <span className="err">Playground payments are capped at 5 test USDC (the testnet vault’s limit).</span>}
              {units && !tooBig && <span className="help">You will pay {formatUSDC(units)} USDC.</span>}
            </label>
            {error && (
              <div className="alert" role="alert">
                {error}
              </div>
            )}
            <button className="btn btn-honey" onClick={() => void go()} disabled={!ready}>
              {busy ? "Signing your invoice…" : "Continue"} <ArrowRight size={18} aria-hidden />
            </button>
          </div>
        )}
      </div>
    </AppFrame>
  );
}
