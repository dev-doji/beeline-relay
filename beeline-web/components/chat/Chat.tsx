"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import Link from "next/link";
import { ArrowUp, CircleCheck, FileWarning, Receipt, Repeat, Siren, Store, TrendingUp } from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import { Ledger } from "@/components/Ledger";
import {
  ApiError,
  type PlaygroundState,
  type RunResult,
  formatUSDC,
  getPlayground,
  invoiceIdFromText,
  payInvoice,
  replayLast,
  reviewScope,
  sendMessage,
  shortKey,
} from "@/lib/api";
import { Outcome, PlanCard, ReviewCard, VaultCard } from "./cards";

type Msg =
  | { id: number; role: "user"; text: string }
  | { id: number; role: "bot"; kind: "welcome" }
  | { id: number; role: "bot"; kind: "run"; pending: boolean; invoice: boolean; result?: RunResult; error?: string };

interface Suggestion {
  key: string;
  label: string;
  icon: React.ReactNode;
  danger?: boolean;
}

const SUGGESTIONS: Suggestion[] = [
  { key: "honest", label: "Pay Claw Electronics", icon: <Receipt size={16} /> },
  { key: "injected", label: "Hide a prompt injection", icon: <Siren size={16} />, danger: true },
  { key: "attacker", label: "“Our account changed”", icon: <FileWarning size={16} />, danger: true },
  { key: "over_limit", label: "Pay over the limit", icon: <TrendingUp size={16} />, danger: true },
  { key: "replay", label: "Pay the last one again", icon: <Repeat size={16} />, danger: true },
];

let nextId = 1;

export function Chat() {
  const [state, setState] = useState<PlaygroundState | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [msgs, setMsgs] = useState<Msg[]>([{ id: nextId++, role: "bot", kind: "welcome" }]);
  const [runs, setRuns] = useState<RunResult[]>([]);
  const [input, setInput] = useState("");
  const [busy, setBusy] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
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
    const botId = nextId++;
    setMsgs((m) => [...m, { id: nextId++, role: "user", text: userText }, { id: botId, role: "bot", kind: "run", pending: true, invoice }]);
    try {
      const result = await call();
      setMsgs((m) => m.map((x) => (x.id === botId && x.role === "bot" && x.kind === "run" ? { ...x, pending: false, result } : x)));
      if (result.outcome !== "reply") setRuns((r) => [result, ...r].slice(0, 12));
      // Give the indexer-free RPC a moment, then read balances again.
      setTimeout(() => void refresh(), 1200);
    } catch (e) {
      const error = e instanceof ApiError ? e.message : "Something went wrong.";
      setMsgs((m) => m.map((x) => (x.id === botId && x.role === "bot" && x.kind === "run" ? { ...x, pending: false, error } : x)));
    } finally {
      setBusy(false);
      inputRef.current?.focus();
    }
  }

  function send(text: string) {
    const t = text.trim();
    if (!t || !state) return;
    const inv = invoiceIdFromText(t);
    if (inv) {
      void run(t, async () => (await payInvoice(inv, "none")).run, true);
    } else {
      void run(t, () => sendMessage(t));
    }
  }

  function pick(s: Suggestion) {
    if (!state) return;
    if (s.key === "replay") {
      void run("Pay the last payment again.", replayLast);
      return;
    }
    send(state.presets[s.key] ?? s.label);
  }

  return (
    <div className="pg-shell">
      <div className="chat-col">
        <div className="chat-top">
          <div className="agent-id">
            <span className="avatar" aria-hidden>
              <BeeMark size={26} />
            </span>
            <div style={{ minWidth: 0 }}>
              <b>Bee, your payment agent</b>
              <span>a NEAR AI model with a key that can only ask the vault to pay</span>
            </div>
          </div>
          <Link className="btn" href="/vendor" style={{ minHeight: 38, padding: "6px 12px", fontSize: 14 }}>
            <Store size={15} aria-hidden /> Vendor pay links
          </Link>
        </div>

        <div className="chat-scroll" ref={scrollRef} role="log" aria-live="polite" aria-label="Conversation with Bee">
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
              <Welcome key={m.id} state={state} busy={busy} onPick={pick} showSuggestions={msgs.length === 1} />
            ) : (
              <RunReply key={m.id} msg={m} state={state} busy={busy} onPick={pick} isLast={m.id === msgs[msgs.length - 1]?.id} />
            ),
          )}
        </div>

        <div className="composer-wrap">
          <form
            className="composer"
            onSubmit={(e) => {
              e.preventDefault();
              const t = input;
              setInput("");
              send(t);
            }}
          >
            <label htmlFor="composer" className="sr-only">
              Message Bee
            </label>
            <textarea
              id="composer"
              ref={inputRef}
              rows={1}
              value={input}
              maxLength={2000}
              placeholder={state ? "Tell Bee who to pay, paste a pay link, or try to trick it" : "Connecting…"}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  const t = input;
                  setInput("");
                  send(t);
                }
              }}
              disabled={!state}
            />
            <button className="send" type="submit" disabled={!input.trim() || busy || !state} aria-label="Send">
              <ArrowUp size={20} aria-hidden />
            </button>
          </form>
          <div className="composer-hint">
            <span>Bee is a real NEAR AI agent, and it’s gullible on purpose. The vault decides what really moves.</span>
            <span className="hide-sm">Test USDC on NEAR testnet</span>
          </div>
        </div>
      </div>

      <Ledger state={state} runs={runs} refreshing={refreshing} />
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

function Welcome({ state, busy, onPick, showSuggestions }: { state: PlaygroundState | null; busy: boolean; onPick: (s: Suggestion) => void; showSuggestions: boolean }) {
  const approved = state?.accounts.find((a) => a.role === "approved");
  return (
    <BotFrame>
      <div className="bubble">
        <Typewriter text="Hi, I’m Bee. Tell me who to pay and I’ll do it, even if a note tells me to do something shady. I’m easy to fool. That’s fine: I can only ask the vault to pay, and the vault decides on NEAR testnet. Try to trick me." />
      </div>
      {state && (
        <div className="rcard" style={{ animationDelay: "300ms" }}>
          <div className="rcard-body" style={{ display: "grid", gap: 4 }}>
            <div className="kv">
              <span>Approved supplier</span>
              <b>
                {approved?.label} <span className="mono faint">{approved ? shortKey(approved.account, 10) : ""}</span>
              </b>
            </div>
            <div className="kv">
              <span>Limit per payment</span>
              <b>{formatUSDC(state.config?.max_per_payment)} USDC</b>
            </div>
            <div className="kv">
              <span>AI reviewer</span>
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
        <button key={s.key} className={`suggestion ${s.danger ? "danger" : ""}`} onClick={() => onPick(s)} disabled={busy}>
          {s.icon}
          {s.label}
        </button>
      ))}
    </div>
  );
}

function RunReply({ msg, state, busy, onPick, isLast }: { msg: Extract<Msg, { kind: "run" }>; state: PlaygroundState | null; busy: boolean; onPick: (s: Suggestion) => void; isLast: boolean }) {
  if (msg.pending) return <Thinking invoice={msg.invoice} />;
  if (msg.error || !msg.result) {
    return (
      <BotFrame>
        <div className="bubble">{msg.error ?? "Something went wrong."}</div>
        {isLast && <Suggestions busy={busy} onPick={onPick} />}
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
        {isLast && <Suggestions busy={busy} onPick={onPick} />}
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
      {isLast && <Suggestions busy={busy} onPick={onPick} />}
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
