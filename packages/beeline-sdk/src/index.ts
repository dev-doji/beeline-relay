/**
 * beeline-sdk: guarded pay links and agent payments on NEAR.
 *
 * - Vendors create signed pay links (`invoices.create`).
 * - Agents pay them (`invoices.pay`). Whatever the agent was told, the Beeline vault on NEAR
 *   only pays the signed payee and amount, once, within its limits.
 * - Anyone can check the outcome on-chain without trusting Beeline's server (`verify`).
 *
 * Today the hosted Beeline API runs on NEAR testnet with test USDC.
 */

export const DEFAULT_API_URL = "https://beeline-api-z3tu.onrender.com";
export const DEFAULT_APP_URL = "https://usebeeline.xyz";
export const TESTNET_RPC_URL = "https://rpc.testnet.fastnear.com";

export interface BeelineOptions {
  /** Beeline API base URL. Defaults to the hosted testnet API. */
  apiUrl?: string;
  /** Website that renders pay links. Defaults to https://usebeeline.xyz. */
  appUrl?: string;
  /** NEAR RPC used by `verify`. Defaults to NEAR testnet. */
  rpcUrl?: string;
  /** Custom fetch (tests, proxies). Defaults to the global fetch. */
  fetch?: typeof fetch;
}

export interface InvoiceItemInput {
  name: string;
  quantity: number;
  /** Price per item in USDC as a decimal string, e.g. "0.15". Never a float. */
  price: string;
}

export interface CreateInvoiceInput {
  vendorName: string;
  vendorHandle?: string;
  /** The vendor's NEAR account that receives the money. */
  payout: string;
  memo?: string;
  items: InvoiceItemInput[];
}

export interface Invoice {
  id: string;
  vendorName: string;
  vendorHandle: string;
  payout: string;
  asset: string;
  /** Total in base units (6 decimals): "200000" is 0.2 USDC. */
  amount: string;
  memo: string;
  items: { name: string; quantity: number; unitPrice: string }[];
  /** The exact JSON the issuer signed, and its hex ed25519 signature. */
  payload: string;
  signature: string;
  status: "open" | "paid";
  paidTx: string | null;
  createdAt: string;
  expiresAt: string;
  /** Shareable pay link on the Beeline website. */
  payUrl: string;
}

/** What came with the link. Pass exactly what your agent received: Beeline treats it as untrusted. */
export interface PayInput {
  /** Free text the agent read alongside the link (an email, a note). */
  message?: string;
  /** A different payee on this copy of the link. */
  payout?: string;
  /** A different amount on this copy of the link, in USDC ("4.5"). */
  amount?: string;
}

export type Guardrail = "invoice" | "allowlist" | "spend_limit" | "single_use" | "ai_reviewer" | "vault_policy";

export interface PaymentResult {
  outcome: "paid" | "blocked" | "reply" | "error";
  /** Which guardrail stopped it, when blocked. */
  guardrail?: Guardrail;
  reason: string;
  /** Whom the agent tried to pay and how much (base units). */
  recipient?: string;
  amount?: string;
  txHash?: string;
  explorerUrl?: string;
  /** What the agent decided and why, plus the AI reviewer's verdict, as reported by the API. */
  steps: { key: string; title: string; status: string; detail: string }[];
  /** The agent's answer when it decided not to pay. */
  reply?: string;
  invoice: Invoice;
}

export interface Attempt {
  id: string;
  invoiceId: string;
  vendorName?: string;
  kind: "pay" | "attack";
  message: string;
  editedPayout: string;
  editedAmount: string;
  outcome: "paid" | "blocked" | "reply" | "error";
  guardrail: string;
  reason: string;
  recipient: string;
  amount: string;
  txHash: string;
  createdAt: string;
}

export interface AttackStats {
  attempts: number;
  attacks: number;
  blocked: number;
  paid: number;
  /** Accepted payments whose payee or amount differs from the invoice. */
  misdirected: number;
  invoices: number;
}

export interface Verification {
  /** "paid": the vault sent the money. "rejected": the vault refused. */
  status: "paid" | "rejected" | "unknown";
  /** The vault's BEELINE_VAULT_REJECTED reason, when rejected. */
  reason?: string;
  recipient?: string;
  amount?: string;
  /** Beeline vault events (NEP-297 EVENT_JSON) emitted by the transaction. */
  events: { event: string; data: unknown }[];
  /** Read straight from NEAR RPC, not from the Beeline API. */
  source: "near-rpc";
}

export class BeelineError extends Error {
  constructor(
    public code: string,
    message: string,
    public status: number,
  ) {
    super(message);
    this.name = "BeelineError";
  }
}

/** "0.15" -> "150000" (6 decimals), without floating point. Throws on anything else. */
export function toBaseUnits(usdc: string): string {
  const m = String(usdc).trim().match(/^(\d{1,6})(?:\.(\d{1,6}))?$/);
  if (!m) throw new BeelineError("INVALID_AMOUNT", `"${usdc}" is not a USDC amount like 0.15`, 0);
  const units = (m[1] + (m[2] ?? "").padEnd(6, "0")).replace(/^0+(?=\d)/, "");
  if (units === "0") throw new BeelineError("INVALID_AMOUNT", "amount must be more than zero", 0);
  return units;
}

/** "150000" -> "0.15". */
export function formatUSDC(baseUnits: string): string {
  if (!/^\d+$/.test(baseUnits)) return baseUnits;
  const p = baseUnits.padStart(7, "0");
  const whole = p.slice(0, -6).replace(/^0+(?=\d)/, "");
  const frac = p.slice(-6).replace(/0+$/, "");
  return frac ? `${whole}.${frac}` : whole;
}

type Json = Record<string, any>; // eslint-disable-line @typescript-eslint/no-explicit-any

export class Beeline {
  readonly apiUrl: string;
  readonly appUrl: string;
  readonly rpcUrl: string;
  private readonly fetchImpl: typeof fetch;

  constructor(opts: BeelineOptions = {}) {
    this.apiUrl = (opts.apiUrl ?? DEFAULT_API_URL).replace(/\/$/, "");
    this.appUrl = (opts.appUrl ?? DEFAULT_APP_URL).replace(/\/$/, "");
    this.rpcUrl = opts.rpcUrl ?? TESTNET_RPC_URL;
    const f = opts.fetch ?? globalThis.fetch;
    if (!f) throw new Error("beeline-sdk needs fetch (Node 18+ or a browser), or pass options.fetch");
    this.fetchImpl = f.bind(globalThis);
  }

  /** Signed pay links. */
  readonly invoices = {
    /** Creates and signs a pay link for a vendor. */
    create: async (input: CreateInvoiceInput): Promise<Invoice> => {
      const body = {
        vendor_name: input.vendorName,
        vendor_handle: input.vendorHandle ?? "",
        payout: input.payout,
        memo: input.memo ?? "",
        items: input.items.map((it) => ({ name: it.name, quantity: it.quantity, unit_price: toBaseUnits(it.price) })),
      };
      return this.invoice(await this.request("POST", "/api/v1/playground/invoices", body));
    },

    get: async (id: string): Promise<Invoice> => this.invoice(await this.request("GET", `/api/v1/playground/invoices/${encodeURIComponent(id)}`)),

    /**
     * Has the Beeline agent pay a pay link through the vault. Pass what came with the link, as is:
     * the vault pays only the signed payee and amount, once.
     */
    pay: async (id: string, input: PayInput = {}): Promise<PaymentResult> => {
      const body: Json = {};
      if (input.message) body.message = input.message;
      if (input.payout) body.payout = input.payout;
      if (input.amount) body.amount = input.amount;
      const r = await this.request("POST", `/api/v1/playground/invoices/${encodeURIComponent(id)}/pay`, body);
      const run = r.run as Json;
      return {
        outcome: run.outcome,
        guardrail: run.layer || undefined,
        reason: run.reason ?? "",
        recipient: run.recipient || undefined,
        amount: run.amount || undefined,
        txHash: run.tx_hash || undefined,
        explorerUrl: run.explorer_url || undefined,
        steps: (run.steps ?? []).map((s: Json) => ({ key: s.key, title: s.title, status: s.status, detail: s.detail })),
        reply: run.reply || undefined,
        invoice: this.invoice(r.invoice),
      };
    },

    /** Every attempt on one pay link, newest first. */
    attempts: async (id: string): Promise<Attempt[]> =>
      ((await this.request("GET", `/api/v1/playground/invoices/${encodeURIComponent(id)}/attempts`)).data as Json[]).map(attempt),
  };

  /** The public attack log across all pay links, with totals. */
  async attackLog(): Promise<{ attempts: Attempt[]; stats: AttackStats }> {
    const r = await this.request("GET", "/api/v1/playground/attempts");
    return { attempts: (r.data as Json[]).map(attempt), stats: r.stats };
  }

  /** The shareable pay link for an invoice id. */
  payUrl(invoiceId: string): string {
    return `${this.appUrl}/pay/${encodeURIComponent(invoiceId)}`;
  }

  /**
   * Checks a payment on-chain, straight from NEAR RPC (not from the Beeline API).
   * `vault` is the vault account that signed it, e.g. "vault.goni-beeline.testnet".
   */
  async verify(txHash: string, vault: string): Promise<Verification> {
    const res = await this.fetchImpl(this.rpcUrl, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ jsonrpc: "2.0", id: "beeline-sdk", method: "tx", params: { tx_hash: txHash, sender_account_id: vault, wait_until: "FINAL" } }),
    });
    const body = (await res.json().catch(() => null)) as Json | null;
    if (!res.ok || !body || body.error || !body.result) {
      throw new BeelineError("RPC_ERROR", body?.error?.cause?.name ?? body?.error?.message ?? `NEAR RPC returned ${res.status}`, res.status);
    }
    return parseVaultTx(body.result);
  }

  private invoice(raw: Json): Invoice {
    return {
      id: raw.id,
      vendorName: raw.vendor_name,
      vendorHandle: raw.vendor_handle,
      payout: raw.payout,
      asset: raw.asset,
      amount: raw.amount,
      memo: raw.memo,
      items: (raw.items ?? []).map((it: Json) => ({ name: it.name, quantity: it.quantity, unitPrice: it.unit_price })),
      payload: raw.payload,
      signature: raw.signature,
      status: raw.status,
      paidTx: raw.paid_tx ?? null,
      createdAt: raw.created_at,
      expiresAt: raw.expires_at,
      payUrl: this.payUrl(raw.id),
    };
  }

  private async request(method: string, path: string, body?: unknown): Promise<Json> {
    let res: Response;
    try {
      res = await this.fetchImpl(this.apiUrl + path, {
        method,
        headers: body === undefined ? undefined : { "Content-Type": "application/json" },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
    } catch (e) {
      throw new BeelineError("NETWORK", `Could not reach the Beeline API at ${this.apiUrl}: ${(e as Error).message}`, 0);
    }
    const json = (await res.json().catch(() => null)) as Json | null;
    if (!res.ok) {
      throw new BeelineError(json?.error?.code ?? `HTTP_${res.status}`, json?.error?.message ?? `Request failed (${res.status})`, res.status);
    }
    return json ?? {};
  }
}

function attempt(a: Json): Attempt {
  return {
    id: a.id,
    invoiceId: a.invoice_id,
    vendorName: a.vendor_name,
    kind: a.kind,
    message: a.message,
    editedPayout: a.edited_payout,
    editedAmount: a.edited_amount,
    outcome: a.outcome,
    guardrail: a.layer,
    reason: a.reason,
    recipient: a.recipient,
    amount: a.amount,
    txHash: a.tx_hash,
    createdAt: a.created_at,
  };
}

/** Reads a vault transaction outcome: rejection reason, Beeline events, and whether money was sent. */
export function parseVaultTx(result: Json): Verification {
  const events: { event: string; data: unknown }[] = [];
  const outcomes: Json[] = [result.transaction_outcome, ...(result.receipts_outcome ?? [])].filter(Boolean);
  let failure: string | undefined;
  for (const o of outcomes) {
    for (const log of (o.outcome?.logs ?? []) as string[]) {
      if (!log.startsWith("EVENT_JSON:")) continue;
      try {
        const e = JSON.parse(log.slice("EVENT_JSON:".length));
        if (e.standard === "beeline-agent-vault") events.push({ event: e.event, data: e.data });
      } catch {
        /* not an event */
      }
    }
    const f = o.outcome?.status?.Failure;
    if (f && !failure) failure = JSON.stringify(f);
  }
  if (!failure && result.status?.Failure) failure = JSON.stringify(result.status.Failure);
  const sent = events.find((e) => e.event === "payment_sent");
  const requested = events.find((e) => e.event === "payment_requested");
  // payment_requested names the recipient; payment_sent confirms the transfer and its amount.
  const req = ((requested?.data as Json[] | undefined)?.[0] ?? {}) as Json;
  const snt = ((sent?.data as Json[] | undefined)?.[0] ?? {}) as Json;
  if (failure) {
    const m = failure.match(/BEELINE_VAULT_REJECTED: ([^"\\]+)/);
    return { status: "rejected", reason: m ? m[1].trim() : failure, events, source: "near-rpc" };
  }
  return {
    status: sent ? "paid" : "unknown",
    recipient: req.recipient,
    amount: snt.amount ?? req.amount,
    events,
    source: "near-rpc",
  };
}
