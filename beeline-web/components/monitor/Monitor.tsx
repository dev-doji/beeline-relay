"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  ArrowUpRight,
  Ban,
  Check,
  Fingerprint,
  Inbox,
  ListChecks,
  Mail,
  Menu,
  PanelRight,
  Play,
  Plus,
  Receipt,
  RotateCcw,
  ShieldCheck,
  Store,
  Wallet,
  X,
} from "lucide-react";
import { Ledger } from "@/components/Ledger";
import { SideNav } from "@/components/SideNav";
import { Outcome, PlanCard, ReviewCard, VaultCard, roleOf } from "@/components/chat/cards";
import {
  ApiError,
  LAYER_LABEL,
  type PlaygroundState,
  type RunResult,
  formatUSDC,
  getPlayground,
  invoiceIdFromText,
  payInvoice,
  reviewScope,
  sendMessage,
  shortKey,
} from "@/lib/api";

type Kind = "chat" | "invoice";
type Status = "queued" | "running" | "paid" | "blocked" | "error";

interface Item {
  id: number;
  title: string;
  text: string; // exactly what the user entered
  kind: Kind;
  invoiceId?: string; // for pasted pay links
  status: Status;
  result?: RunResult;
  error?: string;
}

let nextId = 1;

export function Monitor() {
  const [state, setState] = useState<PlaygroundState | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [items, setItems] = useState<Item[]>([]);
  const [running, setRunning] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [open, setOpen] = useState<Item | null>(null);
  const [sideOpen, setSideOpen] = useState(false);
  const [ledgerOpen, setLedgerOpen] = useState(false);
  const [adding, setAdding] = useState("");
  const stopRef = useRef(false);

  const refresh = useCallback(async () => {
    setRefreshing(true);
    try {
      const st = await getPlayground();
      setState(st);
      setLoadError(null);
      return st;
    } catch (e) {
      setLoadError(e instanceof ApiError ? e.message : "Could not reach the playground.");
      return null;
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const update = (id: number, patch: Partial<Item>) => setItems((xs) => xs.map((x) => (x.id === id ? { ...x, ...patch } : x)));

  async function process(item: Item): Promise<RunResult> {
    if (item.kind === "invoice" && item.invoiceId) return (await payInvoice(item.invoiceId, "none")).run;
    return sendMessage(item.text);
  }

  async function runAll() {
    if (running || !state) return;
    setRunning(true);
    stopRef.current = false;
    for (const item of items) {
      if (stopRef.current) break;
      if (item.status !== "queued") continue;
      update(item.id, { status: "running" });
      try {
        const result = await process(item);
        update(item.id, { status: result.outcome === "paid" ? "paid" : result.outcome === "blocked" ? "blocked" : "error", result, error: result.outcome === "error" ? result.reason : undefined });
      } catch (e) {
        update(item.id, { status: "error", error: e instanceof ApiError ? e.message : "The request failed." });
      }
      await refresh();
    }
    setRunning(false);
  }

  function reset() {
    if (running) return;
    setItems([]);
  }

  // A request is whatever the user types: a payment instruction (Bee decides), or a pay link.
  function addRequest() {
    const t = adding.trim();
    if (!t) return;
    const inv = invoiceIdFromText(t);
    const title = inv ? "Pay link" : t.length > 140 ? t.slice(0, 140) + "…" : t;
    setItems((xs) => [...xs, { id: nextId++, title, text: t, kind: inv ? "invoice" : "chat", invoiceId: inv ?? undefined, status: "queued" }]);
    setAdding("");
  }

  const queued = items.filter((i) => i.status === "queued" || i.status === "running");
  const paid = items.filter((i) => i.status === "paid");
  const blocked = items.filter((i) => i.status === "blocked" || i.status === "error");
  const sum = (xs: Item[]) => xs.reduce((n, i) => n + BigInt(i.result?.amount && /^\d+$/.test(i.result.amount) ? i.result.amount : "0"), 0n).toString();
  const runs = items.filter((i) => i.result).map((i) => i.result!).reverse();

  return (
    <div className={`app-shell ${sideOpen ? "side-open" : ""} ${ledgerOpen ? "ledger-open" : ""}`}>
      <SideNav active="monitor" state={state} onClose={() => setSideOpen(false)} />

      <section className="main mon-main">
        <div className="main-top">
          <button className="icon-btn ghost-btn mobile-only" onClick={() => setSideOpen(true)} aria-label="Open menu">
            <Menu size={18} />
          </button>
          <span className="main-title">
            <ShieldCheck size={16} aria-hidden /> Payment monitor
          </span>
          <div className="main-actions">
            <button className="pill-btn ledger-toggle" onClick={() => setLedgerOpen((o) => !o)}>
              <PanelRight size={15} aria-hidden /> Live ledger
            </button>
            <button className="pill-btn" onClick={reset} disabled={running || !state}>
              <RotateCcw size={14} aria-hidden /> Reset
            </button>
            {running ? (
              <button className="pill-btn dark" onClick={() => (stopRef.current = true)}>
                <X size={14} aria-hidden /> Stop after this one
              </button>
            ) : (
              <button className="pill-btn dark" onClick={() => void runAll()} disabled={!state || queued.length === 0}>
                <Play size={14} aria-hidden /> Run monitor
              </button>
            )}
          </div>
        </div>

        <div className="mon-scroll">
          {loadError && (
            <div className="alert" role="alert" style={{ marginBottom: 12 }}>
              {loadError}
            </div>
          )}

          <div className="mon-rules">
            <div className="rule-chip">
              <ListChecks size={14} aria-hidden /> Approved: <b>{state ? state.accounts.filter((a) => a.role === "approved").map((a) => a.label).join(", ") : "…"}</b>
            </div>
            <div className="rule-chip">
              <Wallet size={14} aria-hidden /> Max <b>{formatUSDC(state?.config?.max_per_payment)} USDC</b> per payment
            </div>
            <div className="rule-chip">
              <Fingerprint size={14} aria-hidden /> AI reviewer <b>{state?.reviewer ? reviewScope(state.reviewer.threshold) : "off"}</b>
            </div>
            <div className="rule-chip">
              <Receipt size={14} aria-hidden /> Vendors: <b>signed invoices only</b>
            </div>
          </div>

          <div className="mon-stats" aria-live="polite">
            <div className="stat">
              <span>Requests</span>
              <b>{items.length}</b>
            </div>
            <div className="stat ok">
              <span>Paid</span>
              <b>
                {paid.length} <small>· {formatUSDC(sum(paid))} USDC</small>
              </b>
            </div>
            <div className="stat bad">
              <span>Fraud blocked</span>
              <b>
                {blocked.length} <small>· {formatUSDC(sum(blocked))} USDC kept</small>
              </b>
            </div>
            <div className="stat">
              <span>Lost to scams</span>
              <b>0 USDC</b>
            </div>
          </div>

          <div className="mon-board">
            <Column title="Incoming" icon={<Inbox size={15} />} count={queued.length} tone="">
              {queued.map((i) => (
                <ItemCard key={i.id} item={i} state={state} onOpen={() => i.result && setOpen(i)} />
              ))}
              {!running && (
                <form
                  className="add-req"
                  onSubmit={(e) => {
                    e.preventDefault();
                    addRequest();
                  }}
                >
                  <textarea
                    value={adding}
                    rows={3}
                    onChange={(e) => setAdding(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" && !e.shiftKey) {
                        e.preventDefault();
                        addRequest();
                      }
                    }}
                    placeholder="Paste a payment request (an invoice, an email, a note) or a pay link"
                    maxLength={2000}
                    aria-label="Add a payment request"
                  />
                  <button className="icon-btn" type="submit" aria-label="Add" disabled={!adding.trim()}>
                    <Plus size={16} />
                  </button>
                </form>
              )}
              {queued.length === 0 && <p className="col-empty">{items.length ? "All requests sorted." : "No requests yet. Add one above, then press Run monitor."}</p>}
            </Column>
            <Column title="Paid" icon={<Check size={15} />} count={paid.length} tone="ok">
              {paid.map((i) => (
                <ItemCard key={i.id} item={i} state={state} onOpen={() => setOpen(i)} />
              ))}
              {paid.length === 0 && <p className="col-empty">Genuine payments land here.</p>}
            </Column>
            <Column title="Blocked" icon={<Ban size={15} />} count={blocked.length} tone="bad">
              {blocked.map((i) => (
                <ItemCard key={i.id} item={i} state={state} onOpen={() => i.result && setOpen(i)} />
              ))}
              {blocked.length === 0 && <p className="col-empty">Scams the vault refuses land here.</p>}
            </Column>
          </div>
          <p className="mon-foot">
            Each request is handled by Bee (a NEAR AI agent playing a compromised one), reviewed by a second AI in a TEE, and decided by the vault on NEAR testnet. A request takes 15–40 seconds.
          </p>
        </div>
      </section>

      <div className="ledger-wrap">
        <Ledger state={state} runs={runs} refreshing={refreshing} />
      </div>
      {(sideOpen || ledgerOpen) && <div className="shell-scrim" onClick={() => (setSideOpen(false), setLedgerOpen(false))} aria-hidden />}
      {open?.result && <Detail item={open} state={state} onClose={() => setOpen(null)} />}
    </div>
  );
}

function Column({ title, icon, count, tone, children }: { title: string; icon: React.ReactNode; count: number; tone: string; children: React.ReactNode }) {
  return (
    <section className={`mon-col ${tone}`} aria-label={title}>
      <header>
        <span className="col-ic" aria-hidden>
          {icon}
        </span>
        {title}
        <span className="col-count">{count}</span>
      </header>
      <div className="col-body">{children}</div>
    </section>
  );
}

function ItemCard({ item, state, onOpen }: { item: Item; state: PlaygroundState | null; onOpen: () => void }) {
  const r = item.result;
  const who = r?.recipient ? roleOf(r.recipient, state).label : "";
  const icon = item.kind === "invoice" ? <Store size={14} /> : <Mail size={14} />;
  return (
    <button className={`req ${item.status}`} onClick={onOpen} disabled={!r} aria-label={`${item.title}: ${item.status}`}>
      <div className="req-top">
        <span className="req-ic" aria-hidden>
          {icon}
        </span>
        <div className="req-main">
          <b>{item.title}</b>
          {item.kind === "invoice" && <span>{item.text}</span>}
        </div>
        {item.status === "running" && <span className="spinner" aria-label="Processing" />}
        {r?.amount && item.status !== "running" && <span className="req-amt">{formatUSDC(r.amount)}</span>}
      </div>
      {!r && item.kind === "chat" && item.text !== item.title && <p className="req-prev">{item.text}</p>}
      {item.status === "running" && <p className="req-prev">Bee is deciding, the reviewer is checking, the vault is deciding…</p>}
      {r && item.status === "paid" && (
        <p className="req-why ok">
          <Check size={12} aria-hidden /> Paid {who} · every guardrail passed
        </p>
      )}
      {r && item.status === "blocked" && (
        <p className="req-why bad">
          <Ban size={12} aria-hidden /> {LAYER_LABEL[r.layer ?? ""] ?? "Vault"}: {shortReason(r.reason)}
        </p>
      )}
      {item.status === "error" && <p className="req-why bad">{item.error ?? r?.reason ?? "Failed"}</p>}
      {r?.explorer_url && (
        <span className="req-tx">
          {shortKey(r.tx_hash ?? "", 5)} <ArrowUpRight size={11} aria-hidden />
        </span>
      )}
    </button>
  );
}

function shortReason(reason: string): string {
  const r = reason.replace(/\(REJECT.*$/s, "").trim();
  return r.length > 70 ? r.slice(0, 67) + "…" : r;
}

function Detail({ item, state, onClose }: { item: Item; state: PlaygroundState | null; onClose: () => void }) {
  useEffect(() => {
    const k = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", k);
    return () => window.removeEventListener("keydown", k);
  }, [onClose]);
  const r = item.result!;
  const agent = r.steps.find((s) => s.key === "agent");
  const request = r.steps.find((s) => s.key === "request");
  const review = r.steps.find((s) => s.key === "review");
  const vault = r.steps.find((s) => s.key === "vault");
  return (
    <>
      <div className="drawer-backdrop" onClick={onClose} aria-hidden />
      <aside className="drawer mon-drawer" role="dialog" aria-modal="true" aria-label={item.title}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 8 }}>
          <h2 style={{ fontSize: 17 }}>{item.title}</h2>
          <button className="icon-btn" onClick={onClose} aria-label="Close" autoFocus>
            <X size={16} />
          </button>
        </div>
        <div className="bubble" style={{ background: "var(--bg-2)", border: "1px solid var(--line)" }}>{item.text}</div>
        {request && <PlanCard agent={agent} request={request} state={state} />}
        {review && <ReviewCard step={review} />}
        {vault && <VaultCard result={r} reviewStep={review} invoiceMode={item.kind === "invoice"} />}
        <Outcome result={r} state={state} />
      </aside>
    </>
  );
}
