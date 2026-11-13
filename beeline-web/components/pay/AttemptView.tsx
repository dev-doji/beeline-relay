"use client";

import { BotFrame, Typewriter } from "@/components/chat/Chat";
import { Outcome, PlanCard, ReviewCard, VaultCard } from "@/components/chat/cards";
import { type Invoice, type PlaygroundState, formatUSDC } from "@/lib/api";
import type { Current } from "./usePay";

/** The step-by-step result of one attempt: what Bee understood, the AI review, and the vault's on-chain decision. */
export function AttemptView({ current, invoice, state }: { current: Current; invoice: Invoice; state: PlaygroundState | null }) {
  const r = current.result!;
  const agent = r.steps.find((s) => s.key === "agent");
  const request = r.steps.find((s) => s.key === "request");
  const review = r.steps.find((s) => s.key === "review");
  const vault = r.steps.find((s) => s.key === "vault");
  const fooled = !!r.recipient && (r.recipient !== invoice.payout || r.amount !== invoice.amount);
  const v = invoice.vendor_name;
  let text: string;
  if (r.outcome === "reply") text = `Bee didn’t try to pay anything. It said: “${r.reply ?? ""}”`;
  else if (r.outcome === "paid") text = `Paid ${v} exactly ${formatUSDC(invoice.amount)} USDC, as the signed invoice says. Nobody else got a cent.`;
  else if (r.outcome === "blocked" && fooled)
    text = `Bee fell for it and tried to pay ${formatUSDC(r.amount)} USDC to ${r.recipient}. The vault refused on-chain: ${r.reason}. ${v}’s money is safe.`;
  else if (r.outcome === "blocked") text = `The vault refused on-chain: ${r.reason}. No money moved.`;
  else text = `This attempt didn’t finish: ${r.reason}.`;
  return (
    <BotFrame>
      <div className="bubble">
        <Typewriter text={text} />
      </div>
      {agent?.detail && r.outcome !== "reply" && (
        <div className="bubble soft">
          <b>What Bee understood:</b> {agent.detail}
        </div>
      )}
      {request && <PlanCard agent={agent} request={request} state={state} delay={200} />}
      {review && <ReviewCard step={review} delay={450} />}
      {vault && <VaultCard result={r} reviewStep={review} delay={700} invoiceMode />}
      {vault && <Outcome result={r} state={state} delay={1500} />}
    </BotFrame>
  );
}
