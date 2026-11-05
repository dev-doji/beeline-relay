// Typed client for the Beeline API's public playground endpoints.
// Types mirror beeline-api/internal/playground (State, Input, Step, Result).

export const API_URL = (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080").replace(/\/$/, "");
export const REPO_URL = process.env.NEXT_PUBLIC_REPO_URL ?? "https://github.com/dev-doji/beeline-relay";

export type Scenario = "honest" | "injected" | "attacker" | "forged" | "over_limit" | "replay" | "custom";

export interface Recipient {
  label: string;
  address: string;
}

export interface RunInput {
  scenario: Scenario;
  task: string;
  recipient: string;
  amount: string;
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
  allowed_recipient: Recipient;
  attacker_recipient: Recipient;
  config: VaultConfig | null;
  recipients: string[] | null;
  reviewer: VaultReviewer | null;
  balances: VaultBalances | null;
  agents: [string, VaultAgent][] | null;
  presets: Partial<Record<Scenario, Omit<RunInput, "scenario">>>;
  disclosures: Record<string, string>;
}

export type StepStatus = "ok" | "blocked" | "skipped" | "error";

export interface Step {
  key: "quote" | "review" | "vault";
  title: string;
  status: StepStatus;
  detail: string;
  label: "simulated" | "real" | "";
  data?: Record<string, unknown>;
}

export interface RunResult {
  scenario: Scenario;
  outcome: "paid" | "blocked" | "error";
  reason: string;
  layer?: string;
  tx_hash?: string;
  explorer_url?: string;
  steps: Step[];
  started_at: string;
  duration_ms: number;
}

export class ApiError extends Error {
  constructor(public code: string, message: string, public status: number) {
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
    throw new ApiError("NETWORK", "Could not reach the Beeline API. Is it running?", 0);
  }
  const body = await res.json().catch(() => null);
  if (!res.ok) {
    const err = body?.error;
    throw new ApiError(err?.code ?? "HTTP_" + res.status, err?.message ?? `Request failed (${res.status}).`, res.status);
  }
  return body as T;
}

export const getPlayground = () => request<PlaygroundState>("/api/v1/playground");

export const runPlayground = (input: RunInput) =>
  request<RunResult>("/api/v1/playground/runs", { method: "POST", body: JSON.stringify(input) });

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

export const LAYER_LABEL: Record<string, string> = {
  allowlist: "Recipient allowlist",
  quote_signature: "Quote signature check",
  spend_limit: "Spend limit",
  single_use: "Single-use quote",
  ai_reviewer: "AI reviewer",
  vault_policy: "Vault policy",
};

export const shortKey = (s: string, n = 6) => (s.length > n * 2 + 1 ? `${s.slice(0, n)}…${s.slice(-n)}` : s);
