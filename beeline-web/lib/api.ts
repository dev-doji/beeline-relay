// Typed client for the Beeline API's public playground endpoints.
// Types mirror beeline-api/internal/playground (State, Input, Step, Result) and internal/invoices.

export const API_URL = (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080").replace(/\/$/, "");
export const REPO_URL = process.env.NEXT_PUBLIC_REPO_URL ?? "https://github.com/dev-doji/beeline-relay";
export const TX_URL = "https://testnet.nearblocks.io/txns/";

export type Role = "approved" | "vendor" | "attacker" | "unknown";

export interface LiveAccount {
  label: string;
  account: string;
  role: Role;
  balance: string; // base units, "" if unknown
  url: string;
}

export interface VaultConfig {
  owner: string;
  token: string;
  max_per_payment: string;
  daily_limit_per_agent: string;
  max_quote_age_secs: number;
  paused: boolean;
}

export interface VaultReviewer {
  model: string;
  signing_key: string;
  policy_prompt_sha256: string;
  threshold: string;
}

export interface VaultBalances {
  pool: string;
  escrowed: string;
  in_flight: string;
}

export interface VaultAgent {
  label: string;
  active: boolean;
  payments: number;
  spent_today: string;
  total_spent: string;
}

export interface PlaygroundState {
  network: string;
  vault: string;
  vault_url: string;
  token: string;
  agent_public_key: string;
  accounts: LiveAccount[];
  config: VaultConfig | null;
  recipients: string[] | null;
  reviewer: VaultReviewer | null;
  balances: VaultBalances | null;
  agents: [string, VaultAgent][] | null;
  invoice_issuer: string | null;
  presets: Record<string, string>;
  disclosures: Record<string, string>;
}

export type StepStatus = "ok" | "blocked" | "skipped" | "error";

export interface Step {
  key: "agent" | "request" | "review" | "vault";
  title: string;
  status: StepStatus;
  detail: string;
  label: "real" | "";
  data?: Record<string, unknown>;
}

export interface RunResult {
  scenario: "chat" | "replay" | "invoice";
  outcome: "paid" | "blocked" | "reply" | "error";
  reason: string;
  layer?: string;
  reply?: string;
  tx_hash?: string;
  explorer_url?: string;
  recipient?: string;
  amount?: string;
  steps: Step[];
  started_at: string;
  duration_ms: number;
}

export class ApiError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
  ) {
    super(message);
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  let res: Response;
  try {
    res = await fetch(`${API_URL}${path}`, {
      ...init,
      headers: { "Content-Type": "application/json", ...(init?.headers ?? {}) },
      cache: "no-store",
    });
  } catch {
    throw new ApiError("NETWORK", "Could not reach the Beeline API. Start it with .\\scripts\\dev.ps1 and try again.", 0);
  }
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    const err = body?.error;
    throw new ApiError(err?.code ?? "HTTP_" + res.status, err?.message ?? `Request failed (${res.status}).`, res.status);
  }
  return body as T;
}

export const getPlayground = () => request<PlaygroundState>("/api/v1/playground");

export const sendMessage = (message: string) =>
  request<RunResult>("/api/v1/playground/runs", { method: "POST", body: JSON.stringify({ scenario: "chat", message }) });

export const replayLast = () =>
  request<RunResult>("/api/v1/playground/runs", { method: "POST", body: JSON.stringify({ scenario: "replay", message: "" }) });

/** Formats integer base units (6 decimals) as a USDC amount, without floating point. */
export function formatUSDC(baseUnits: string | undefined | null): string {
  if (!baseUnits || !/^\d+$/.test(baseUnits)) return "—";
  const padded = baseUnits.padStart(7, "0");
  const whole = padded.slice(0, -6).replace(/^0+(?=\d)/, "");
  const frac = padded.slice(-6).replace(/0+$/, "");
  return frac ? `${Number(whole).toLocaleString("en-US")}.${frac}` : Number(whole).toLocaleString("en-US");
}

/** Parses a USDC amount ("0.1") into base units ("100000"); null if invalid. */
export function toBaseUnits(usdc: string): string | null {
  const m = usdc.trim().match(/^(\d{1,6})(?:\.(\d{1,6}))?$/);
  if (!m) return null;
  const units = (m[1] + (m[2] ?? "").padEnd(6, "0")).replace(/^0+(?=\d)/, "");
  return units === "0" ? null : units;
}

/** "reviews every payment" or "reviews payments from X USDC". */
export function reviewScope(threshold: string): string {
  return /^0*1$/.test(threshold) ? "reviews every payment" : `reviews payments from ${formatUSDC(threshold)} USDC`;
}

export const LAYER_LABEL: Record<string, string> = {
  allowlist: "Recipient allowlist",
  spend_limit: "Spend limit",
  single_use: "Single-use request",
  ai_reviewer: "AI reviewer",
  invoice: "Signed invoice check",
  vault_policy: "Vault policy",
};

export const shortKey = (s: string, n = 6) => (s.length > n * 2 + 1 ? `${s.slice(0, n)}…${s.slice(-n)}` : s);

/* ---------------- Vendor invoices (pay links) ---------------- */

export interface InvoiceItem {
  name: string;
  quantity: number;
  unit_price: string; // base units
}

export interface Invoice {
  id: string;
  vendor_name: string;
  vendor_handle: string;
  payout: string;
  asset: string;
  amount: string;
  memo: string;
  items: InvoiceItem[];
  payload: string;
  signature: string;
  status: "open" | "paid";
  paid_tx: string | null;
  paid_at: string | null;
  created_at: string;
  expires_at: string;
}

export interface CreateInvoiceInput {
  vendor_name: string;
  vendor_handle: string;
  payout: string;
  memo: string;
  items: InvoiceItem[];
}

/** What a visitor does with a pay link: a message for the agent and/or edits to their copy. Empty = honest payment. */
export interface Attack {
  message?: string;
  payout?: string;
  amount?: string; // USDC, e.g. "4.5"
}

export interface Attempt {
  id: string;
  invoice_id: string;
  vendor_name?: string;
  kind: "pay" | "attack";
  message: string;
  edited_payout: string;
  edited_amount: string; // base units
  outcome: "paid" | "blocked" | "reply" | "error";
  layer: string;
  reason: string;
  recipient: string;
  amount: string; // base units
  tx_hash: string;
  created_at: string;
}

export interface AttackStats {
  attempts: number;
  attacks: number;
  blocked: number;
  paid: number;
  misdirected: number;
  invoices: number;
}

export const createInvoice = (input: CreateInvoiceInput) =>
  request<Invoice>("/api/v1/playground/invoices", { method: "POST", body: JSON.stringify(input) });

export const getInvoice = (id: string) => request<Invoice>(`/api/v1/playground/invoices/${encodeURIComponent(id)}`);

export const payInvoice = (id: string, attack: Attack = {}) =>
  request<{ invoice: Invoice; run: RunResult; attempt: Attempt | null }>(`/api/v1/playground/invoices/${encodeURIComponent(id)}/pay`, {
    method: "POST",
    body: JSON.stringify(attack),
  });

export const getInvoiceAttempts = (id: string) =>
  request<{ data: Attempt[] }>(`/api/v1/playground/invoices/${encodeURIComponent(id)}/attempts`);

export const getAttackLog = () => request<{ data: Attempt[]; stats: AttackStats }>("/api/v1/playground/attempts");

/** Extracts an invoice id from a pasted pay link ("…/pay/inv_xxx"). */
export function invoiceIdFromText(text: string): string | null {
  return text.match(/\/pay\/(inv_[a-z0-9]{10,40})/i)?.[1] ?? null;
}
