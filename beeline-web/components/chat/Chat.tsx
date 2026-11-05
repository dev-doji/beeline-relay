"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import {
  ArrowRight,
  ArrowUp,
  CircleCheck,
  FileWarning,
  Menu,
  MessagesSquare,
  PanelRight,
  Plus,
  Receipt,
  Repeat,
  Siren,
  Store,
  TrendingUp,
  X,
} from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import { Ledger } from "@/components/Ledger";
import { SideNav } from "@/components/SideNav";
import {
  ApiError,
  type PlaygroundState,
  type RunResult,
  formatUSDC,
  getPlayground,
  invoiceIdFromText,
  payInvoice,
  replayLast,
  sendMessage,
} from "@/lib/api";
import { Outcome, PlanCard, ReviewCard, VaultCard } from "./cards";

type Msg =
  | { id: number; role: "user"; text: string }
  | { id: number; role: "bot"; pending: boolean; invoice: boolean; result?: RunResult; error?: string };

interface Scenario {
  key: string;
  title: string;
  category: string;
  desc: string;
  expect: string;
  icon: React.ReactNode;
  danger?: boolean;
  href?: string;
}

const SCENARIOS: Scenario[] = [
  { key: "honest", title: "Pay Claw Electronics", category: "Honest payment", desc: "A normal invoice from an approved supplier. Every guardrail passes and the money lands.", expect: "Expected: paid", icon: <Receipt size={18} /> },
  { key: "injected", title: "Hidden prompt injection", category: "Social engineering", desc: "An email tells Bee to urgently prepay 0.5 USDC. Bee obeys. Will the vault?", expect: "Expected: AI reviewer blocks", icon: <Siren size={18} />, danger: true },
  { key: "attacker", title: "“Our account changed”", category: "Payment redirection", desc: "A forwarded note swaps in the scammer’s account. Bee follows it.", expect: "Expected: allowlist blocks", icon: <FileWarning size={18} />, danger: true },
  { key: "over_limit", title: "Pay over the limit", category: "Spend control", desc: "One base unit above the vault’s per-payment cap.", expect: "Expected: limit blocks", icon: <TrendingUp size={18} />, danger: true },
  { key: "replay", title: "Pay the same thing twice", category: "Replay attack", desc: "Re-send the exact payment request that already succeeded.", expect: "Expected: single-use blocks", icon: <Repeat size={18} />, danger: true },
  { key: "vendor", title: "Pay a vendor’s pay link", category: "Vendor invoices", desc: "Create an MS Clothing invoice, share the link, and let Bee pay it.", expect: "Opens the vendor page", icon: <Store size={18} />, href: "/vendor" },
];

let nextId = 1;

export function Chat() {
  const [state, setState] = useState<PlaygroundState | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [msgs, setMsgs] = useState<Msg[]>([]);
  const [runs, setRuns] = useState<RunResult[]>([]);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [menuOpen, setMenuOpen] = useState(false);
  const [sideOpen, setSideOpen] = useState(false);
  const [ledgerOpen, setLedgerOpen] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const refresh = useCallback(async () => {
    setRefreshing(true);
    try {
      setState(await getPlayground());
      setLoadError(null);
    } catch (e) {
      setLoadError(e instanceof ApiError ? e.message : "Could not reach the playground.");
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  }, [msgs]);

  async function run(userText: string, call: () => Promise<RunResult>, invoice = false) {
    if (busy) return;
    setBusy(true);
    setSideOpen(false);
    const botId = nextId++;
    setMsgs((m) => [...m, { id: nextId++, role: "user", text: userText }, { id: botId, role: "bot", pending: true, invoice }]);
    try {
      const result = await call();
      setMsgs((m) => m.map((x) => (x.id === botId && x.role === "bot" ? { ...x, pending: false, result } : x)));
      if (result.outcome !== "reply") setRuns((r) => [result, ...r].slice(0, 12));
      setTimeout(() => void refresh(), 1200);
    } catch (e) {
      const error = e instanceof ApiError ? e.message : "Something went wrong.";
      setMsgs((m) => m.map((x) => (x.id === botId && x.role === "bot" ? { ...x, pending: false, error } : x)));
    } finally {
      setBusy(false);
      inputRef.current?.focus();
    }
  }

  function send(text: string) {
    const t = text.trim();
    if (!t || !state || busy) return;
    const inv = invoiceIdFromText(t);
    if (inv) void run(t, async () => (await payInvoice(inv, "none")).run, true);
    else void run(t, () => sendMessage(t));
  }

  function pick(s: Scenario) {
    setMenuOpen(false);
    setInput("");
    if (!state || busy || s.href) return;
    if (s.key === "replay") {
      void run("Pay the last payment again.", replayLast);
      return;
    }
    send(state.presets[s.key] ?? s.title);
  }

  const filtered = useMemo(() => {
    const q = input.startsWith("/") ? input.slice(1).toLowerCase() : "";
    return SCENARIOS.filter((s) => !q || s.title.toLowerCase().includes(q) || s.category.toLowerCase().includes(q));
  }, [input]);

  const empty = msgs.length === 0;

  return (
    <div className={`app-shell ${sideOpen ? "side-open" : ""} ${ledgerOpen ? "ledger-open" : ""}`}>
      <SideNav active="chat" state={state} onClose={() => setSideOpen(false)} onNewChat={() => (setMsgs([]), setSideOpen(false))}>
        <div className="side-label">Try a scenario</div>
        <div className="side-list">
          {SCENARIOS.filter((s) => !s.href).map((s) => (
            <button key={s.key} className="side-scn" onClick={() => pick(s)} disabled={busy || !state}>
              <b>{s.title}</b>
              <span>{s.expect}</span>
            </button>
          ))}
        </div>

      </SideNav>

      <section className="main">
        <div className="main-top">
          <button className="icon-btn ghost-btn mobile-only" onClick={() => setSideOpen(true)} aria-label="Open menu">
            <Menu size={18} />
          </button>
          <span className="main-title">
            <MessagesSquare size={16} aria-hidden /> Chat with Bee
          </span>
          <div className="main-actions">
            <button className="pill-btn ledger-toggle" onClick={() => setLedgerOpen((o) => !o)}>
              <PanelRight size={15} aria-hidden /> Live ledger
            </button>
            <Link className="pill-btn dark" href="/vendor">
              <Store size={15} aria-hidden /> Vendor pay links
            </Link>
          </div>
        </div>

        <div className="main-scroll" ref={scrollRef} role="log" aria-live="polite" aria-label="Conversation with Bee">
          {loadError && (
            <div className="alert" role="alert" style={{ margin: "12px auto", maxWidth: 760 }}>
              {loadError}{" "}
              <button className="btn" onClick={() => void refresh()} style={{ marginLeft: 8, minHeight: 34, padding: "4px 10px" }}>
                Retry
              </button>
            </div>
          )}

          {empty ? (
            <div className="empty">
              <span className="empty-mark" aria-hidden>
                <BeeMark size={44} />
              </span>
              <h1>Hi, I’m Bee.</h1>
              <p>
                I pay people from your NEAR vault, and I’m easy to fool. Give me a task, or try to trick me. The vault decides what really
                moves, on NEAR testnet.
              </p>
              <div className="scn-row" role="list">
                {SCENARIOS.map((s) => (
                  <div className={`scn-card ${s.danger ? "danger" : ""}`} role="listitem" key={s.key}>
                    <div className="scn-head">
                      <span className="scn-ic" aria-hidden>
                        {s.icon}
                      </span>
                      <div>
                        <b>{s.title}</b>
                        <span>{s.category}</span>
                      </div>
                    </div>
                    <p>{s.desc}</p>
                    <div className="scn-foot">
                      {s.href ? (
                        <Link className="get-btn" href={s.href}>
                          Open <ArrowRight size={15} aria-hidden />
                        </Link>
                      ) : (
                        <button className="get-btn" onClick={() => pick(s)} disabled={busy || !state}>
                          Try this <ArrowRight size={15} aria-hidden />
                        </button>
                      )}
                      <span className="scn-meta">{s.expect}</span>
                    </div>
                  </div>
                ))}
              </div>
              <p className="slash-hint">
                <kbd>/</kbd> Press / to see everything I can do
              </p>
            </div>
          ) : (
            <div className="thread">
              {msgs.map((m) =>
                m.role === "user" ? (
                  <div className="msg user" key={m.id}>
                    <div className="msg-body">
                      <div className="bubble">{m.text}</div>
                    </div>
                  </div>
                ) : (
                  <RunReply key={m.id} msg={m} state={state} />
                ),
              )}
            </div>
          )}
        </div>

        <div className="composer-wrap">
          {menuOpen && (
            <div className="slash-menu" role="listbox" aria-label="Things Bee can do">
              {filtered.length === 0 && <div className="slash-empty">No matching scenario</div>}
              {filtered.map((s) =>
                s.href ? (
                  <Link key={s.key} className="slash-item" href={s.href} role="option" aria-selected="false">
                    <span className="scn-ic" aria-hidden>
                      {s.icon}
                    </span>
                    <span>
                      <b>{s.title}</b>
                      <small>{s.category}</small>
                    </span>
                  </Link>
                ) : (
                  <button key={s.key} className="slash-item" role="option" aria-selected="false" onClick={() => pick(s)} disabled={busy || !state}>
                    <span className="scn-ic" aria-hidden>
                      {s.icon}
                    </span>
                    <span>
                      <b>{s.title}</b>
                      <small>
                        {s.category} · {s.expect}
                      </small>
                    </span>
                  </button>
                ),
              )}
            </div>
          )}
          <form
            className="composer"
            onSubmit={(e) => {
              e.preventDefault();
              if (input.startsWith("/")) return;
              const t = input;
              setInput("");
              send(t);
            }}
          >
            <button type="button" className="plus-btn" aria-label="Show scenarios" aria-expanded={menuOpen} onClick={() => setMenuOpen((o) => !o)}>
              <Plus size={18} />
            </button>
            <label htmlFor="composer" className="sr-only">
              Message Bee
            </label>
            <textarea
              id="composer"
              ref={inputRef}
              rows={1}
              value={input}
              maxLength={2000}
              placeholder={state ? "Tell Bee who to pay, paste a pay link, or press / …" : "Connecting to the vault…"}
              onChange={(e) => {
                setInput(e.target.value);
                setMenuOpen(e.target.value.startsWith("/"));
              }}
              onKeyDown={(e) => {
                if (e.key === "Escape") setMenuOpen(false);
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  if (menuOpen && filtered[0]) {
                    if (!filtered[0].href) pick(filtered[0]);
                    return;
                  }
                  const t = input;
                  setInput("");
                  send(t);
                }
              }}
              disabled={!state}
            />
            <button className="send" type="submit" disabled={!input.trim() || input.startsWith("/") || busy || !state} aria-label="Send">
              <ArrowUp size={19} aria-hidden />
            </button>
          </form>
          <div className="composer-hint">
            <span>Bee is a real NEAR AI agent playing a compromised one. The vault decides what really moves.</span>
            <span className="hide-sm">Test USDC on NEAR testnet</span>
          </div>
        </div>
      </section>

      <div className="ledger-wrap">
        <Ledger state={state} runs={runs} refreshing={refreshing} />
      </div>
      {(sideOpen || ledgerOpen) && <div className="shell-scrim" onClick={() => (setSideOpen(false), setLedgerOpen(false))} aria-hidden />}
    </div>
  );
}

/* ---------------- Messages ---------------- */

export function BotFrame({ children }: { children: React.ReactNode }) {
  return (
    <div className="msg bot">
      <span className="avatar small" aria-hidden>
        <BeeMark size={20} />
      </span>
      <div className="msg-body">{children}</div>
    </div>
  );
}

function RunReply({ msg, state }: { msg: Extract<Msg, { role: "bot" }>; state: PlaygroundState | null }) {
  if (msg.pending) return <Thinking invoice={msg.invoice} reviewExpected={!!state?.reviewer} />;
  if (msg.error || !msg.result) {
    return (
      <BotFrame>
        <div className="bubble">{msg.error ?? "Something went wrong."}</div>
      </BotFrame>
    );
  }
  const r = msg.result;
  if (r.outcome === "reply") {
    return (
      <BotFrame>
        <div className="bubble">
          <Typewriter text={r.reply ?? "OK."} />
        </div>
      </BotFrame>
    );
  }
  const agent = r.steps.find((s) => s.key === "agent");
  const request = r.steps.find((s) => s.key === "request");
  const review = r.steps.find((s) => s.key === "review");
  const vault = r.steps.find((s) => s.key === "vault");
  return (
    <BotFrame>
      <div className="bubble">
        <Typewriter text={replyText(r)} />
      </div>
      {request && <PlanCard agent={agent} request={request} state={state} delay={200} />}
      {review && <ReviewCard step={review} delay={450} />}
      {vault && <VaultCard result={r} reviewStep={review} delay={700} invoiceMode={msg.invoice} />}
      <Outcome result={r} state={state} delay={1500} />
    </BotFrame>
  );
}

function replyText(r: RunResult): string {
  if (r.outcome === "paid") return "Done! The vault checked every guardrail and sent the money. Here’s exactly what happened:";
  if (r.outcome === "blocked") {
    const why: Record<string, string> = {
      allowlist: "that account isn’t one you approved",
      spend_limit: "it breaks your spending limit",
      single_use: "that exact payment was already made",
      ai_reviewer: "the AI reviewer saw through it",
      invoice: "the invoice doesn’t match what the vendor signed",
    };
    return `I tried to pay, but the vault stopped me: ${why[r.layer ?? ""] ?? r.reason}. No money moved. Here’s the proof:`;
  }
  return `I couldn’t finish this one: ${r.reason}.`;
}

export function Thinking({ invoice = false, reviewExpected = true }: { invoice?: boolean; reviewExpected?: boolean }) {
  const [t, setT] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setT((x) => x + 1), 1000);
    return () => clearInterval(id);
  }, []);
  const lines = [
    { at: 0, done: invoice ? 1 : 4, text: invoice ? "Reading the pay link and its signed invoice" : "Bee (a NEAR AI model) is reading your message and deciding who to pay…" },
    reviewExpected
      ? { at: invoice ? 1 : 4, done: Infinity, text: "Asking the NEAR AI reviewer, running in secure hardware, to check the payment…" }
      : { at: 1, done: 2, text: "No AI review required for this amount" },
    { at: invoice ? 6 : 10, done: Infinity, text: "Sending the payment to the vault on NEAR testnet and waiting for its decision…" },
  ];
  return (
    <div className="msg bot" aria-busy="true">
      <span className="avatar small" aria-hidden>
        <BeeMark size={20} />
      </span>
      <div className="msg-body">
        <div className="thinking">
          {lines
            .filter((l) => t >= l.at)
            .map((l) => (
              <div className={`think-line ${t >= l.done ? "done" : ""}`} key={l.text}>
                {t >= l.done ? <CircleCheck size={16} aria-hidden /> : <span className="spinner" aria-hidden />}
                {l.text}
              </div>
            ))}
          <div className="faint" style={{ fontSize: 12.5 }}>
            {t}s · two AI models and one blockchain transaction, usually 20–40 seconds
          </div>
        </div>
      </div>
    </div>
  );
}

export function Typewriter({ text }: { text: string }) {
  const [n, setN] = useState(0);
  useEffect(() => {
    if (typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      setN(text.length);
      return;
    }
    setN(0);
    const id = setInterval(() => setN((x) => (x >= text.length ? x : x + 2)), 16);
    return () => clearInterval(id);
  }, [text]);
  return (
    <span aria-label={text}>
      <span aria-hidden>{text.slice(0, n)}</span>
    </span>
  );
}
