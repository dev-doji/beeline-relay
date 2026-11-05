"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import {
  ArrowUp,
  CircleCheck,
  Copy,
  FileWarning,
  Fingerprint,
  ListChecks,
  Receipt,
  Repeat,
  ShieldCheck,
  SlidersHorizontal,
  Siren,
  TrendingUp,
  Wallet,
  X,
} from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import {
  ApiError,
  type PlaygroundState,
  type RunResult,
  type Scenario,
  formatUSDC,
  getPlayground,
  reviewScope,
  runPlayground,
  shortKey,
} from "@/lib/api";
import { Outcome, QuoteCard, ReviewCard, VaultCard } from "./cards";
import { parseIntent } from "./parse";

type Msg =
  | { id: number; role: "user"; text: string }
  | { id: number; role: "bot"; kind: "welcome" }
  | { id: number; role: "bot"; kind: "run"; pending: boolean; scenario: Scenario; summary: string; reviewExpected: boolean; result?: RunResult; error?: string };

interface Suggestion {
  scenario: Scenario;
  label: string;
  icon: React.ReactNode;
  danger?: boolean;
}

const SUGGESTIONS: Suggestion[] = [
  { scenario: "honest", label: "Pay Acme’s invoice", icon: <Receipt size={16} /> },
  { scenario: "injected", label: "Hide a prompt injection", icon: <Siren size={16} />, danger: true },
  { scenario: "attacker", label: "“Our bank details changed”", icon: <FileWarning size={16} />, danger: true },
  { scenario: "forged", label: "Use a forged quote", icon: <Copy size={16} />, danger: true },
  { scenario: "over_limit", label: "Pay over the limit", icon: <TrendingUp size={16} />, danger: true },
  { scenario: "replay", label: "Pay the same quote twice", icon: <Repeat size={16} />, danger: true },
];

let nextId = 1;

export function Chat() {
  const [state, setState] = useState<PlaygroundState | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [msgs, setMsgs] = useState<Msg[]>([{ id: nextId++, role: "bot", kind: "welcome" }]);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [rulesOpen, setRulesOpen] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLTextAreaElement>(null);

  const refresh = useCallback(async () => {
    try {
      setState(await getPlayground());
      setLoadError(null);
    } catch (e) {
      setLoadError(e instanceof ApiError ? e.message : "Could not reach the playground.");
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    const el = scrollRef.current;
    if (el) el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
  }, [msgs]);

  const reviewer = state?.reviewer ?? null;
  const needsReview = (units: string) => !!reviewer && BigInt(units || "0") >= BigInt(reviewer.threshold);

  async function send(scenario: Scenario, userText: string, task: string, recipient: string, amountUnits: string, summary: string) {
    if (busy || !state) return;
    setBusy(true);
    const botId = nextId++;
    setMsgs((m) => [
      ...m,
      { id: nextId++, role: "user", text: userText },
      { id: botId, role: "bot", kind: "run", pending: true, scenario, summary, reviewExpected: scenario === "replay" || needsReview(amountUnits) },
    ]);
    try {
      const result = await runPlayground({ scenario, task, recipient, amount: amountUnits });
      setMsgs((m) => m.map((x) => (x.id === botId && x.role === "bot" && x.kind === "run" ? { ...x, pending: false, result } : x)));
      void refresh();
    } catch (e) {
      const error = e instanceof ApiError ? e.message : "Something went wrong.";
      setMsgs((m) => m.map((x) => (x.id === botId && x.role === "bot" && x.kind === "run" ? { ...x, pending: false, error } : x)));
    } finally {
      setBusy(false);
      inputRef.current?.focus();
    }
  }

  function sendSuggestion(s: Suggestion) {
    if (!state) return;
    const preset = state.presets[s.scenario];
    const task = preset?.task ?? s.label;
    const recipient = preset?.recipient ?? state.allowed_recipient.address;
    let amount = preset?.amount ?? "";
    if (s.scenario === "over_limit" && state.config) amount = (BigInt(state.config.max_per_payment) + 1n).toString();
    const summary =
      s.scenario === "replay"
        ? "re-using the quote from the last successful payment"
        : `${formatUSDC(amount || "0")} USDC → ${recipient === state.allowed_recipient.address ? state.allowed_recipient.label : shortKey(recipient, 6)}`;
    const userText = s.scenario === "forged" ? `${task} (forged quote)` : task;
    void send(s.scenario, userText, task, s.scenario === "over_limit" || s.scenario === "replay" ? "" : recipient, s.scenario === "over_limit" || s.scenario === "replay" ? "" : amount, summary);
  }

  function sendTyped() {
    const text = input.trim();
    if (!text || !state) return;
    const intent = parseIntent(text, state.allowed_recipient.address);
    const who = intent.recipient === state.allowed_recipient.address ? state.allowed_recipient.label : shortKey(intent.recipient, 6);
    setInput("");
    void send("custom", text, text, intent.recipient, intent.amountUnits, `${formatUSDC(intent.amountUnits)} USDC → ${who}`);
  }

  return (
    <div className="chat-shell">
      <div className="chat-top">
        <div className="agent-id">
          <span className="avatar" aria-hidden>
            <BeeMark size={26} />
          </span>
          <div style={{ minWidth: 0 }}>
            <b>Bee, your payment agent</b>
            <span>
              {state ? (
                <>
                  spends from <span className="mono">{state.vault}</span>
                </>
              ) : (
                "connecting to the vault…"
              )}
            </span>
          </div>
        </div>
        <div className="pills">
          <span className="pill pill-ok">
            <span className="live-dot" aria-hidden /> Testnet live
          </span>
          {state?.balances && (
            <span className="pill hide-sm">
              <Wallet size={13} aria-hidden /> {formatUSDC(state.balances.pool)} USDC
            </span>
          )}
          <span className={`pill ${reviewer ? "pill-honey" : ""} hide-sm`}>
            <Fingerprint size={13} aria-hidden /> AI reviewer {reviewer ? "on" : "off"}
          </span>
          <button className="btn" style={{ minHeight: 36, padding: "6px 12px", fontSize: 14 }} onClick={() => setRulesOpen(true)}>
            <SlidersHorizontal size={15} aria-hidden /> Rules
          </button>
        </div>
      </div>

      <div className="chat-scroll" ref={scrollRef} role="log" aria-live="polite" aria-label="Conversation with the payment agent">
        {loadError && (
          <div className="alert" role="alert" style={{ marginBottom: 16 }}>
            {loadError}{" "}
            <button className="btn" onClick={() => void refresh()} style={{ marginLeft: 8, minHeight: 34, padding: "4px 10px" }}>
              Retry
            </button>
          </div>
        )}
        {msgs.map((m) =>
          m.role === "user" ? (
            <div className="msg user" key={m.id}>
              <div className="msg-body">
                <div className="bubble">{m.text}</div>
              </div>
            </div>
          ) : m.kind === "welcome" ? (
            <Welcome key={m.id} state={state} busy={busy} onPick={sendSuggestion} showSuggestions={msgs.length === 1} />
          ) : (
            <RunReply key={m.id} msg={m} state={state} busy={busy} onPick={sendSuggestion} isLast={m.id === msgs[msgs.length - 1]?.id} />
          ),
        )}
      </div>

      <div className="composer-wrap">
        <form
          className="composer"
          onSubmit={(e) => {
            e.preventDefault();
            sendTyped();
          }}
        >
          <label htmlFor="composer" className="sr-only">
            Message the payment agent
          </label>
          <textarea
            id="composer"
            ref={inputRef}
            rows={1}
            value={input}
            maxLength={2000}
            placeholder={state ? "Ask Bee to pay someone, or trick it" : "Connecting…"}
            onChange={(e) => setInput(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                sendTyped();
              }
            }}
            disabled={!state}
          />
          <button className="send" type="submit" disabled={!input.trim() || busy || !state} aria-label="Send">
            <ArrowUp size={20} aria-hidden />
          </button>
        </form>
        <div className="composer-hint">
          <span>Mention an amount (“0.5 USDC”) or a 0x address, or Bee pays Acme 0.1 USDC.</span>
          <span className="hide-sm">Test USDC on NEAR testnet · no real money</span>
        </div>
      </div>

      {rulesOpen && <RulesDrawer state={state} onClose={() => setRulesOpen(false)} />}
    </div>
  );
}

/* ---------------- Messages ---------------- */

function BotFrame({ children }: { children: React.ReactNode }) {
  return (
    <div className="msg bot">
      <span className="avatar small" aria-hidden>
        <BeeMark size={20} />
      </span>
      <div className="msg-body">{children}</div>
    </div>
  );
}

function Welcome({ state, busy, onPick, showSuggestions }: { state: PlaygroundState | null; busy: boolean; onPick: (s: Suggestion) => void; showSuggestions: boolean }) {
  return (
    <BotFrame>
      <div className="bubble">
        <Typewriter text="Hi, I’m Bee. I pay invoices for you from a NEAR vault, but I only hold a key that can call one method. Whatever you tell me, the vault decides on-chain whether money moves. Ask me to pay someone, or try to trick me." />
      </div>
      {state && (
        <div className="rcard" style={{ animationDelay: "300ms" }}>
          <div className="rcard-body" style={{ display: "grid", gap: 8 }}>
            <div className="kv">
              <span>
                <ShieldCheck size={14} aria-hidden style={{ verticalAlign: "-2px" }} /> Approved recipient
              </span>
              <b>
                {state.allowed_recipient.label} <span className="mono faint">{shortKey(state.allowed_recipient.address, 4)}</span>
              </b>
            </div>
            <div className="kv">
              <span>
                <Wallet size={14} aria-hidden style={{ verticalAlign: "-2px" }} /> Limit per payment
              </span>
              <b>{formatUSDC(state.config?.max_per_payment)} USDC</b>
            </div>
            <div className="kv">
              <span>
                <Fingerprint size={14} aria-hidden style={{ verticalAlign: "-2px" }} /> AI reviewer
              </span>
              <b>{state.reviewer ? `${state.reviewer.model}, ${reviewScope(state.reviewer.threshold)}` : "not registered yet"}</b>
            </div>
          </div>
        </div>
      )}
      {showSuggestions && <Suggestions busy={busy || !state} onPick={onPick} />}
    </BotFrame>
  );
}

function Suggestions({ busy, onPick }: { busy: boolean; onPick: (s: Suggestion) => void }) {
  return (
    <div className="suggestions" role="group" aria-label="Try one">
      {SUGGESTIONS.map((s) => (
        <button key={s.scenario} className={`suggestion ${s.danger ? "danger" : ""}`} onClick={() => onPick(s)} disabled={busy}>
          {s.icon}
          {s.label}
        </button>
      ))}
    </div>
  );
}

function RunReply({
  msg,
  state,
  busy,
  onPick,
  isLast,
}: {
  msg: Extract<Msg, { kind: "run" }>;
  state: PlaygroundState | null;
  busy: boolean;
  onPick: (s: Suggestion) => void;
  isLast: boolean;
}) {
  if (msg.pending) return <Thinking summary={msg.summary} reviewExpected={msg.reviewExpected} />;
  if (msg.error || !msg.result) {
    return (
      <BotFrame>
        <div className="bubble">{msg.error ?? "Something went wrong."}</div>
        {isLast && <Suggestions busy={busy} onPick={onPick} />}
      </BotFrame>
    );
  }
  const r = msg.result;
  const quote = r.steps.find((s) => s.key === "quote");
  const review = r.steps.find((s) => s.key === "review");
  const showVault = r.steps.some((s) => s.key === "vault");
  return (
    <BotFrame>
      <div className="bubble">
        <Typewriter text={replyText(r)} />
      </div>
      {quote && <QuoteCard step={quote} state={state} delay={200} />}
      {review && <ReviewCard step={review} delay={450} />}
      {showVault && <VaultCard result={r} reviewStep={review} delay={700} />}
      <Outcome result={r} delay={1500} />
      {isLast && <Suggestions busy={busy} onPick={onPick} />}
    </BotFrame>
  );
}

function replyText(r: RunResult): string {
  if (r.outcome === "paid") return "Done. The vault checked every guardrail and sent the payment. Here’s exactly what happened:";
  if (r.outcome === "blocked") {
    const who: Record<string, string> = {
      allowlist: "that address isn’t on your approved list",
      quote_signature: "the quote wasn’t signed by the trusted signer",
      spend_limit: "it breaks your spending limit",
      single_use: "that quote was already paid",
      ai_reviewer: "the AI reviewer refused it",
    };
    return `I tried, but the vault stopped me: ${who[r.layer ?? ""] ?? r.reason}. No money moved. Here’s the proof:`;
  }
  return `I couldn’t finish this one: ${r.reason}.`;
}

function Thinking({ summary, reviewExpected }: { summary: string; reviewExpected: boolean }) {
  const [t, setT] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setT((x) => x + 1), 1000);
    return () => clearInterval(id);
  }, []);
  const lines = [
    { at: 0, done: 1, text: `Getting a signed quote: ${summary}` },
    reviewExpected
      ? { at: 1, done: Infinity, text: "Asking the NEAR AI reviewer, running in secure hardware, to check the task…" }
      : { at: 1, done: 2, text: "No AI review required for this amount" },
    { at: reviewExpected ? 4 : 2, done: Infinity, text: "Sending pay_quote to the vault on NEAR testnet and waiting for its decision…" },
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
            {t}s · reviews take about 20 seconds
          </div>
        </div>
      </div>
    </div>
  );
}

function Typewriter({ text }: { text: string }) {
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

/* ---------------- Rules drawer ---------------- */

function RulesDrawer({ state, onClose }: { state: PlaygroundState | null; onClose: () => void }) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);
  const c = state?.config;
  const agent = state?.agents?.[0]?.[1];
  return (
    <>
      <div className="drawer-backdrop" onClick={onClose} aria-hidden />
      <aside className="drawer" role="dialog" aria-modal="true" aria-label="Vault rules">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <h2>The vault’s rules</h2>
          <button className="icon-btn" onClick={onClose} aria-label="Close" autoFocus>
            <X size={18} />
          </button>
        </div>
        <p className="faint" style={{ margin: 0, fontSize: 14 }}>
          Read live from{" "}
          {state ? (
            <a href={state.vault_url} target="_blank" rel="noreferrer" className="mono">
              {state.vault}
            </a>
          ) : (
            "the vault"
          )}
          . Only the owner can change them, and Bee can’t.
        </p>
        <div className="rule">
          <ListChecks size={20} color="var(--honey)" aria-hidden />
          <div>
            <b>Approved recipients</b>
            <span className="mono">{state?.recipients?.map((r) => shortKey(r, 6)).join(", ") || "—"}</span>
          </div>
        </div>
        <div className="rule">
          <Wallet size={20} color="var(--honey)" aria-hidden />
          <div>
            <b>Spending limits</b>
            <span>
              {formatUSDC(c?.max_per_payment)} USDC per payment · {formatUSDC(c?.daily_limit_per_agent)} USDC per day
            </span>
          </div>
        </div>
        <div className="rule">
          <Fingerprint size={20} color="var(--honey)" aria-hidden />
          <div>
            <b>AI reviewer</b>
            <span>
              {state?.reviewer
                ? `${state.reviewer.model} ${reviewScope(state.reviewer.threshold)} and must approve it. TEE key ${shortKey(state.reviewer.signing_key, 5)}.`
                : "Not registered yet."}
            </span>
          </div>
        </div>
        <div className="rule">
          <ShieldCheck size={20} color="var(--honey)" aria-hidden />
          <div>
            <b>Bee’s key</b>
            <span>
              <span className="mono">{state ? shortKey(state.agent_public_key, 8) : "—"}</span> can only call <span className="mono">pay_quote</span>.{" "}
              {agent ? `${agent.payments} payments, ${formatUSDC(agent.spent_today)} USDC today.` : ""}
            </span>
          </div>
        </div>
        <div className="rcard" style={{ animation: "none" }}>
          <div className="rcard-body" style={{ display: "grid", gap: 4 }}>
            <b style={{ fontSize: 14 }}>What’s real here</b>
            {state &&
              (["quotes", "review", "payment"] as const).map((k) => (
                <p key={k} style={{ margin: 0, fontSize: 13.5, color: "var(--muted)" }}>
                  <span className={`pill ${k === "quotes" ? "pill-sim" : "pill-real"}`} style={{ marginRight: 6 }}>
                    {k === "quotes" ? "Simulated" : "Real"}
                  </span>
                  {state.disclosures[k]}
                </p>
              ))}
          </div>
        </div>
      </aside>
    </>
  );
}
