/** Base mainnet helpers for the wallet panel. USDC amounts are integers in base units (6 decimals). */

export const BASE_CHAIN_ID = 8453;
export const BASE_RPC_URL = "https://mainnet.base.org";
export const BASE_EXPLORER = "https://basescan.org";
/** Circle's native USDC on Base (the same token Switch pays out and NEAR Intents lists). */
export const BASE_USDC = "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913";
/** Safety rail for sends from the demo page: 10 USDC. */
export const SEND_CAP_UNITS = 10_000_000n;

const hex = (v: bigint, bytes = 32) => v.toString(16).padStart(bytes * 2, "0");

/** ERC-20 `transfer(address,uint256)` calldata. The address must be 0x + 40 hex characters. */
export function encodeUsdcTransfer(to: string, units: string): string {
  if (!/^0x[0-9a-fA-F]{40}$/.test(to)) throw new Error("recipient is not an EVM address");
  if (!/^\d+$/.test(units) || units === "0") throw new Error("amount must be a positive integer in base units");
  return "0xa9059cbb" + to.slice(2).toLowerCase().padStart(64, "0") + hex(BigInt(units));
}

/** USDC balance in base units via a public Base RPC (`balanceOf`). */
export async function readUsdcBalance(address: string): Promise<string> {
  if (!/^0x[0-9a-fA-F]{40}$/.test(address)) throw new Error("not an EVM address");
  const data = "0x70a08231" + address.slice(2).toLowerCase().padStart(64, "0");
  const res = await fetch(BASE_RPC_URL, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "eth_call", params: [{ to: BASE_USDC, data }, "latest"] }),
  });
  const body = (await res.json()) as { result?: string; error?: { message?: string } };
  if (!res.ok || !body.result || !/^0x[0-9a-fA-F]*$/.test(body.result)) throw new Error(body.error?.message ?? "balance unavailable");
  return BigInt(body.result === "0x" ? "0x0" : body.result).toString();
}

/** "1500000" -> "1.5" (two decimals at least, up to six). */
export function formatUnits(baseUnits: string): string {
  if (!/^\d+$/.test(baseUnits)) return "—";
  const p = baseUnits.padStart(7, "0");
  const whole = p.slice(0, -6).replace(/^0+(?=\d)/, "");
  const frac = p.slice(-6).replace(/0+$/, "").padEnd(2, "0");
  return `${Number(whole).toLocaleString("en-US")}.${frac}`;
}
