"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import { ArrowRight } from "lucide-react";
import { PayShell } from "@/components/pay/PayShell";
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
    <PayShell>
      <div className="px-wrap narrow">
        <span className="px-chip" style={{ justifySelf: "start" }}>Permanent link · NEAR testnet</span>
        <div className="px-card">
          <h1>{payeeOk ? `Pay ${shop}` : "This link is incomplete"}</h1>
          <p>
            {payeeOk
              ? "Choose an amount. Beeline signs a one-time invoice for you, and the vault pays only this exact payee and amount, once."
              : "A permanent pay link needs a payee account. Ask the vendor to send it again."}
          </p>
          {payeeOk && (
            <>
              <div className="px-payee">
                {payee}
                {note ? ` · ${note}` : ""}
              </div>
              <p>Check this is the account you expect. The link carries the payee in its address, so Beeline cannot tell you the vendor owns it.</p>
              <label className="px-field">
                <span>Amount (test USDC)</span>
                <input value={amount} inputMode="decimal" placeholder="0.50" onChange={(e) => setAmount(e.target.value)} />
                {amount && !units && <strong style={{ color: "#d92d20", fontSize: 11.5, fontWeight: 500 }}>Enter an amount like 0.50.</strong>}
                {tooBig && <strong style={{ color: "#d92d20", fontSize: 11.5, fontWeight: 500 }}>Playground payments are capped at 5 test USDC.</strong>}
                {units && !tooBig && <em>You will pay {formatUSDC(units)} USDC.</em>}
              </label>
              {error && (
                <div className="alert" role="alert">
                  {error}
                </div>
              )}
              <button className="px-btn dark full" onClick={() => void go()} disabled={!ready}>
                {busy ? "Signing your invoice…" : "Continue"} <ArrowRight size={15} aria-hidden />
              </button>
            </>
          )}
        </div>
      </div>
    </PayShell>
  );
}
