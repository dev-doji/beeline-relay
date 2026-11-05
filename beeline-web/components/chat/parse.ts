import { toBaseUnits } from "@/lib/api";

/** What the agent reads out of a chat message. Defaults apply when the message doesn't say. */
export interface Intent {
  recipient: string;
  amountUnits: string;
  amountText: string;
  recipientFromText: boolean;
  amountFromText: boolean;
}

const ADDRESS = /0x[0-9a-fA-F]{40}/;
// "0.5 USDC", "$0.5", "0.5 usd", "pay 2 dollars"
const AMOUNT = /(?:\$\s?(\d{1,6}(?:\.\d{1,6})?))|(\d{1,6}(?:\.\d{1,6})?)\s?(?:usdc|usd|dollars?)\b/i;

export function parseIntent(text: string, defaultRecipient: string): Intent {
  const addr = text.match(ADDRESS)?.[0];
  const m = text.match(AMOUNT);
  const amountText = m ? (m[1] ?? m[2]) : "0.1";
  const units = toBaseUnits(amountText) ?? "100000";
  return {
    recipient: addr ?? defaultRecipient,
    amountUnits: units,
    amountText,
    recipientFromText: !!addr,
    amountFromText: !!m,
  };
}
