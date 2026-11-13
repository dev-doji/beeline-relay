"use client";

import { useRef, useState } from "react";
import { Loader2 } from "lucide-react";
import { PayAnywhere } from "./PayAnywhere";
import { ApiError, type Invoice, claimPayment, formatUSDC, invoiceMemo, tokenContract } from "@/lib/api";

type Phase = { kind: "idle" } | { kind: "working"; text: string } | { kind: "problem"; text: string };

const GAS = "30000000000000"; // 30 Tgas, plenty for one ft_transfer
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

/**
 * How a customer pays. A real-money invoice leads with "pay from any wallet or exchange" (scan a QR code, nothing
 * to paste back) and offers a NEAR wallet as the other way. Either way the server decides: the wallet path reads
 * the transaction from NEAR, the QR path needs NEAR Intents' delivery plus the vendor's balance rising. Nothing the
 * browser says marks an invoice paid (docs/PAYMENT_FLOW.md).
 */
export function PaySheet({ invoice, onPaid }: { invoice: Invoice; onPaid: () => void }) {
  const [phase, setPhase] = useState<Phase>({ kind: "idle" });
  const running = useRef(false);

  const real = invoice.network === "mainnet";
  const working = phase.kind === "working";

  // Asks the server until it says verified, refuses, or a minute and a half has passed.
  async function confirm(txHash: string, from: string) {
    for (let i = 0; i < 30; i++) {
      try {
        const r = await claimPayment(invoice.id, txHash, from);
        if (r.status === "verified") {
          setPhase({ kind: "idle" });
          onPaid();
          return;
        }
        setPhase({ kind: "working", text: "Confirming on NEAR. This takes a few seconds…" });
      } catch (e) {
        setPhase({ kind: "problem", text: e instanceof ApiError ? e.message : "Could not check the payment. Try again in a moment." });
        return;
      }
      await sleep(3000);
    }
    setPhase({ kind: "problem", text: "We could not confirm this payment on NEAR yet. If it went through, it will show here shortly. Reload this page in a minute." });
  }

  async function payWithWallet() {
    if (running.current) return;
    running.current = true;
    try {
      setPhase({ kind: "working", text: "Opening your wallet…" });
      // Loaded only now: the wallet package needs a browser.
      const { NearConnector } = await import("@hot-labs/near-connect");
      const connector = new NearConnector({ network: real ? "mainnet" : "testnet" });
      const wallet = await connector.connect();
      const account = (await wallet.getAccounts())[0]?.accountId;
      if (!account) throw new Error("no account");
      setPhase({ kind: "working", text: "Approve the payment in your wallet…" });
      const outcome = await wallet.signAndSendTransaction({
        signerId: account,
        receiverId: tokenContract(invoice.asset),
        actions: [
          {
            type: "FunctionCall",
            params: { methodName: "ft_transfer", args: { receiver_id: invoice.payout, amount: invoice.amount, memo: invoiceMemo(invoice.id) }, gas: GAS, deposit: "1" },
          },
        ],
      });
      const o = outcome as { transaction?: { hash?: string }; transaction_outcome?: { id?: string } };
      const txHash = o.transaction?.hash ?? o.transaction_outcome?.id;
      if (!txHash) throw new Error("no hash");
      setPhase({ kind: "working", text: "Confirming on NEAR. This takes a few seconds…" });
      await confirm(txHash, account);
    } catch {
      setPhase({ kind: "problem", text: "The payment was not completed in your wallet, so nothing was charged. You can try again." });
    } finally {
      running.current = false;
    }
  }

  return (
    <div className="px-sheet">
      {real && (
        <section className="px-sheet-sec">
          <PayAnywhere invoice={invoice} onPaid={onPaid} />
        </section>
      )}

      <section className="px-sheet-sec">
        {real && <b>Or pay from a NEAR wallet</b>}
        <button className={real ? "px-btn full" : "px-btn dark full"} onClick={() => void payWithWallet()} disabled={working}>
          {working ? <Loader2 size={15} className="px-spin" aria-hidden /> : null}
          {real ? "Connect a NEAR wallet" : `Pay ${formatUSDC(invoice.amount)} test USDC with a NEAR testnet wallet`}
        </button>
        {phase.kind === "working" && (
          <div className="px-status" role="status">
            <Loader2 size={14} className="px-spin" aria-hidden /> {phase.text}
          </div>
        )}
        {phase.kind === "problem" && (
          <div className="alert" role="alert">
            {phase.text}
          </div>
        )}
      </section>
    </div>
  );
}
