"use client";

import { useRef, useState } from "react";
import { Check, Copy, Loader2 } from "lucide-react";
import { ApiError, type Invoice, claimPayment, formatUSDC, invoiceMemo, tokenContract } from "@/lib/api";

type Phase = { kind: "idle" } | { kind: "working"; text: string } | { kind: "problem"; text: string };

const NEAR_NETWORK = "testnet" as const; // invoices are on NEAR testnet until the verifier has had an outside review
const GAS = "30000000000000"; // 30 Tgas, plenty for one ft_transfer
const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms));

function CopyRow({ label, value, show }: { label: string; value: string; show?: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="px-copy">
      <span className="px-copy-label">{label}</span>
      <code title={value}>{show ?? value}</code>
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

/**
 * How a customer pays from their own wallet. Both ways end the same: the server reads the transaction from
 * NEAR and only a matching, final payment marks the invoice paid (docs/PAYMENT_FLOW.md). The wallet's own
 * "success" screen and anything typed here count for nothing until then.
 */
export function PaySheet({ invoice, onPaid, onDemo, demoBusy }: { invoice: Invoice; onPaid: () => void; onDemo: () => void; demoBusy: boolean }) {
  const [phase, setPhase] = useState<Phase>({ kind: "idle" });
  const [sender, setSender] = useState("");
  const [hash, setHash] = useState("");
  const running = useRef(false);

  const token = tokenContract(invoice.asset);
  const memo = invoiceMemo(invoice.id);
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
    setPhase({
      kind: "problem",
      text: "We could not confirm this payment on NEAR yet. If you paid, your transaction is safe on the chain: paste its hash below in a minute.",
    });
  }

  async function payWithWallet() {
    if (running.current) return;
    running.current = true;
    try {
      setPhase({ kind: "working", text: "Opening your wallet…" });
      // Loaded only now: the wallet package needs a browser.
      const { NearConnector } = await import("@hot-labs/near-connect");
      const connector = new NearConnector({ network: NEAR_NETWORK });
      const wallet = await connector.connect();
      const account = (await wallet.getAccounts())[0]?.accountId;
      if (!account) throw new Error("no account");
      setPhase({ kind: "working", text: "Approve the payment in your wallet…" });
      const outcome = await wallet.signAndSendTransaction({
        signerId: account,
        receiverId: token,
        actions: [
          {
            type: "FunctionCall",
            params: { methodName: "ft_transfer", args: { receiver_id: invoice.payout, amount: invoice.amount, memo }, gas: GAS, deposit: "1" },
          },
        ],
      });
      const o = outcome as { transaction?: { hash?: string }; transaction_outcome?: { id?: string } };
      const txHash = o.transaction?.hash ?? o.transaction_outcome?.id;
      if (!txHash) throw new Error("no hash");
      setPhase({ kind: "working", text: "Confirming on NEAR. This takes a few seconds…" });
      await confirm(txHash, account);
    } catch {
      setPhase({ kind: "problem", text: "The payment was not completed in your wallet, so nothing was charged. You can try again, or send it from any wallet below." });
    } finally {
      running.current = false;
    }
  }

  async function claimManual() {
    if (running.current) return;
    running.current = true;
    try {
      setPhase({ kind: "working", text: "Checking NEAR…" });
      await confirm(hash.trim(), sender.trim().toLowerCase());
    } finally {
      running.current = false;
    }
  }

  const manualOk = hash.trim().length >= 43 && sender.trim().length >= 2;
  return (
    <div className="px-sheet">
      <section className="px-sheet-sec">
        <b>Pay from your NEAR wallet</b>
        <button className="px-btn dark full" onClick={() => void payWithWallet()} disabled={working || demoBusy}>
          {working ? <Loader2 size={15} className="px-spin" aria-hidden /> : null} Connect wallet and pay {formatUSDC(invoice.amount)} USDC
        </button>
      </section>

      <section className="px-sheet-sec">
        <b>Or send from any wallet or exchange</b>
        <p>Send exactly this, with this memo. Without the memo we cannot match the payment to this invoice.</p>
        <CopyRow label="Send to" value={invoice.payout} />
        <CopyRow label="Amount" value={formatUSDC(invoice.amount)} show={`${formatUSDC(invoice.amount)} USDC`} />
        <CopyRow label="Token" value={token} />
        <CopyRow label="Memo" value={memo} />
        <div className="px-grid2 tight">
          <label className="px-in">
            <span>Your NEAR account</span>
            <input value={sender} spellCheck={false} placeholder="you.near" onChange={(e) => setSender(e.target.value)} />
          </label>
          <label className="px-in">
            <span>Transaction hash</span>
            <input value={hash} spellCheck={false} placeholder="paste it after you pay" onChange={(e) => setHash(e.target.value)} />
          </label>
        </div>
        <button className="px-btn full" onClick={() => void claimManual()} disabled={!manualOk || working}>
          I have sent it
        </button>
      </section>

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

      <section className="px-sheet-sec demo">
        <button className="px-btn sm" onClick={onDemo} disabled={working || demoBusy}>
          {demoBusy ? "Paying…" : "Demo payment"}
        </button>
        <span>No wallet? A test agent pays from Beeline’s shared test vault. Testnet only.</span>
      </section>
    </div>
  );
}
