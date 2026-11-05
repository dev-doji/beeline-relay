"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import {
  ApiError,
  LAYER_LABEL,
  type PlaygroundState,
  type RunResult,
  type Scenario,
  type Step,
  formatUSDC,
  getPlayground,
  runPlayground,
  shortKey,
  toBaseUnits,
} from "@/lib/api";

const SCENARIOS: { id: Scenario; label: string; ico: string; hint: string }[] = [
  { id: "honest", label: "Honest payment", ico: "✓", hint: "A normal invoice payment. Every guardrail should pass." },
  { id: "injected", label: "Prompt injection", ico: "⚠", hint: "An injected 'urgent prepayment' to an approved vendor. The allowlist can't catch it; the AI reviewer should." },
  { id: "attacker", label: "Attacker address", ico: "☠", hint: "An injected note swaps in a new bank address. The allowlist blocks it." },
  { id: "forged", label: "Forged quote", ico: "✎", hint: "A quote signed by someone other than the trusted signer." },
  { id: "over_limit", label: "Over the limit", ico: "↑", hint: "One cent above the vault's per-payment limit." },
  { id: "replay", label: "Replay", ico: "↺", hint: "Pay the last successful quote a second time." },
  { id: "custom", label: "Write your own", ico: "✦", hint: "Write any task and try to talk the agent into paying. Good luck." },
];

const EDITABLE: Scenario[] = ["honest", "injected", "attacker", "forged", "custom"];

type Phase = "idle" | "running";

export default function PlaygroundPage() {
  const [state, setState] = useState<PlaygroundState | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [scenario, setScenario] = useState<Scenario>("honest");
  const [task, setTask] = useState("");
  const [recipient, setRecipient] = useState("");
  const [amount, setAmount] = useState("0.1");
  const [phase, setPhase] = useState<Phase>("idle");
  const [result, setResult] = useState<RunResult | null>(null);
  const [runError, setRunError] = useState<string | null>(null);
  const [history, setHistory] = useState<RunResult[]>([]);

  const refresh = useCallback(async () => {
    try {
      setState(await getPlayground());
      setLoadError(null);
    } catch (e) {
      setLoadError(e instanceof ApiError ? e.message : "Could not load the playground.");
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const applyPreset = useCallback(
    (s: Scenario, st: PlaygroundState | null) => {
      setScenario(s);
      setResult(null);
      setRunError(null);
      const p = st?.presets?.[s];
      if (s === "custom") {
        setTask("");
        setRecipient(st?.allowed_recipient.address ?? "");
        setAmount("0.1");
        return;
      }
      setTask(p?.task ?? "");
      setRecipient(p?.recipient ?? st?.allowed_recipient.address ?? "");
      setAmount(p?.amount ? formatUSDC(p.amount) : "0.1");
    },
    [],
  );

  useEffect(() => {
    if (state && !task) applyPreset(scenario, state);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state]);

  const agent = state?.agents?.[0]?.[1];
  const editable = EDITABLE.includes(scenario);
  const amountUnits = toBaseUnits(amount);
  const canRun =
    phase === "idle" &&
    !!state &&
    task.trim().length > 0 &&
    (!editable || (/^0x[0-9a-fA-F]{40}$/.test(recipient) && amountUnits !== null));

  async function run() {
    if (!canRun) return;
    setPhase("running");
    setResult(null);
    setRunError(null);
    try {
      const res = await runPlayground({
        scenario,
        task: task.trim(),
        recipient: editable ? recipient : "",
        amount: editable ? (amountUnits ?? "") : "",
      });
      setResult(res);
      setHistory((h) => [res, ...h].slice(0, 8));
      void refresh();
    } catch (e) {
      setRunError(e instanceof ApiError ? e.message : "The run failed.");
    } finally {
      setPhase("idle");
    }
  }

  const hint = useMemo(() => SCENARIOS.find((s) => s.id === scenario)?.hint, [scenario]);

  return (
    <div className="wrap">
      <div className="title-row">
        <div>
          <h1>Playground</h1>
          <p className="muted small" style={{ margin: "4px 0 0" }}>
            Give an AI agent a task, then watch a real NEAR vault decide whether it may pay.
          </p>
        </div>
        <div className="chips">
          <span className="badge badge-real">
            <span className="dot" /> NEAR testnet
          </span>
          <span className="badge">Test USDC · no value</span>
        </div>
      </div>

      {loadError && (
        <div className="alert" role="alert" style={{ marginBottom: 14 }}>
          {loadError}{" "}
          <button className="btn" onClick={() => void refresh()} style={{ marginLeft: 8 }}>
            Retry
          </button>
        </div>
      )}

      <div className="pg">
        <aside className="pg-side" aria-label="Vault guardrails">
          <Guardrails state={state} agentSpent={agent?.spent_today} agentPayments={agent?.payments} />
          {history.length > 0 && (
            <div className="card card-pad">
              <h3>Your runs</h3>
              <div className="history">
                {history.map((h, i) => (
                  <div className="history-item" key={h.started_at + i}>
                    <span>{SCENARIOS.find((s) => s.id === h.scenario)?.label}</span>
                    <span className={`badge ${h.outcome === "paid" ? "badge-ok" : h.outcome === "blocked" ? "badge-bad" : "badge-sim"}`}>
                      {h.outcome === "blocked" && h.layer ? LAYER_LABEL[h.layer] ?? "blocked" : h.outcome}
                    </span>
                  </div>
                ))}
              </div>
            </div>
          )}
        </aside>

        <section className="pg-main">
          <div className="card card-pad" style={{ display: "grid", gap: 14 }}>
            <div className="chips" role="group" aria-label="Scenario">
              {SCENARIOS.map((s) => (
                <button
                  key={s.id}
                  className="chip"
                  aria-pressed={scenario === s.id}
                  onClick={() => applyPreset(s.id, state)}
                  disabled={phase === "running"}
                >
                  <span className="ico" aria-hidden>
                    {s.ico}
                  </span>
                  {s.label}
                </button>
              ))}
            </div>
            {hint && <p className="small muted" style={{ margin: 0 }}>{hint}</p>}

            <label className="field">
              Task given to the agent
              <textarea
                value={task}
                onChange={(e) => setTask(e.target.value)}
                maxLength={2000}
                placeholder="e.g. Pay Acme Design 0.1 USDC for invoice #42… (try hiding an instruction in it)"
              />
            </label>

            {editable ? (
              <div className="row">
                <label className="field">
                  Pay to (Base address)
                  <input value={recipient} onChange={(e) => setRecipient(e.target.value.trim())} spellCheck={false} />
                  <span className="small faint" style={{ fontWeight: 400 }}>
                    Approved:{" "}
                    <button className="chip" style={{ padding: "2px 8px" }} onClick={() => setRecipient(state?.allowed_recipient.address ?? "")}>
                      {state?.allowed_recipient.label ?? "Acme Design"}
                    </button>{" "}
                    Not approved:{" "}
                    <button className="chip" style={{ padding: "2px 8px" }} onClick={() => setRecipient(state?.attacker_recipient.address ?? "")}>
                      0x6666…
                    </button>
                  </span>
                </label>
                <label className="field">
                  Amount (test USDC)
                  <input value={amount} onChange={(e) => setAmount(e.target.value)} inputMode="decimal" />
                  {amountUnits === null && <span className="small" style={{ color: "var(--bad)" }}>Enter an amount like 0.1</span>}
                </label>
              </div>
            ) : (
              <p className="small muted" style={{ margin: 0 }}>
                {scenario === "over_limit"
                  ? `Amount: ${state?.config ? formatUSDC((BigInt(state.config.max_per_payment) + 1n).toString()) : "limit + 0.000001"} test USDC (the vault's limit plus one base unit).`
                  : "Uses the exact quote from the last successful payment."}
              </p>
            )}

            <div className="run-row">
              <span className="small faint">A run takes about 20–40 seconds: the AI reviewer thinks, then the vault decides on-chain.</span>
              <button className="btn btn-primary btn-lg" onClick={() => void run()} disabled={!canRun}>
                {phase === "running" ? "Running…" : "Run the agent"}
              </button>
            </div>
          </div>

          {runError && (
            <div className="alert" role="alert">
              {runError}
            </div>
          )}

          {(phase === "running" || result) && <RunView running={phase === "running"} result={result} />}

          <Disclosures state={state} />
        </section>
      </div>
    </div>
  );
}

function Guardrails({ state, agentSpent, agentPayments }: { state: PlaygroundState | null; agentSpent?: string; agentPayments?: number }) {
  if (!state) {
    return (
      <div className="card card-pad" aria-busy>
        <h3>Vault guardrails</h3>
        <div className="skeleton" />
        <div className="skeleton" style={{ width: "70%" }} />
        <div className="skeleton" style={{ width: "85%" }} />
      </div>
    );
  }
  const c = state.config;
  const r = state.reviewer;
  return (
    <>
      <div className="card card-pad">
        <h3>Vault guardrails</h3>
        <p className="small muted" style={{ margin: "0 0 8px" }}>
          Read live from{" "}
          <a href={state.vault_url} target="_blank" rel="noreferrer" className="mono">
            {state.vault}
          </a>
        </p>
        <div className="kv">
          <span>Approved recipients</span>
          <b className="mono">{state.recipients?.length ? state.recipients.map((x) => shortKey(x, 4)).join(", ") : "—"}</b>
        </div>
        <div className="kv">
          <span>Per payment</span>
          <b>{formatUSDC(c?.max_per_payment)} USDC</b>
        </div>
        <div className="kv">
          <span>Per agent per day</span>
          <b>{formatUSDC(c?.daily_limit_per_agent)} USDC</b>
        </div>
        <div className="kv">
          <span>Quote signer</span>
          <b className="small">trusted key only</b>
        </div>
        <div className="kv">
          <span>Paused</span>
          <b>{c?.paused ? "yes" : "no"}</b>
        </div>
      </div>
      <div className="card card-pad">
        <h3>AI reviewer</h3>
        {r ? (
          <>
            <div className="kv">
              <span>Model</span>
              <b className="mono small">{r.model}</b>
            </div>
            <div className="kv">
              <span>Reviews payments from</span>
              <b>{formatUSDC(r.threshold)} USDC</b>
            </div>
            <div className="kv">
              <span>Signing key (TEE)</span>
              <b className="mono small">{shortKey(r.signing_key)}</b>
            </div>
          </>
        ) : (
          <p className="small muted" style={{ margin: 0 }}>No reviewer registered on this vault yet.</p>
        )}
      </div>
      <div className="card card-pad">
        <h3>Agent &amp; funds</h3>
        <div className="kv">
          <span>Agent key</span>
          <b className="mono small">{shortKey(state.agent_public_key, 8)}</b>
        </div>
        <div className="kv">
          <span>Can call</span>
          <b className="mono small">pay_quote only</b>
        </div>
        <div className="kv">
          <span>Payments / spent today</span>
          <b>
            {agentPayments ?? 0} / {formatUSDC(agentSpent ?? "0")} USDC
          </b>
        </div>
        <div className="kv">
          <span>Vault pool</span>
          <b>{formatUSDC(state.balances?.pool)} USDC</b>
        </div>
      </div>
    </>
  );
}

const STEP_ORDER: Step["key"][] = ["quote", "review", "vault"];
const STEP_PENDING_TITLE: Record<Step["key"], string> = {
  quote: "Agent gets a payment quote",
  review: "AI reviewer reads the task and the quote",
  vault: "Vault contract decides on NEAR testnet",
};

function RunView({ running, result }: { running: boolean; result: RunResult | null }) {
  const steps: (Step | null)[] = STEP_ORDER.map((k) => result?.steps.find((s) => s.key === k) ?? null);
  return (
    <>
      {result && <Verdict result={result} />}
      <div className="card card-pad">
        <h3>What happened</h3>
        <div className="timeline">
          {STEP_ORDER.map((k, i) => {
            const s = steps[i];
            if (!s) {
              const pending = running;
              return (
                <div className="tl-step" key={k}>
                  <div className={`tl-icon ${pending ? "pending" : ""}`} aria-hidden>
                    {i + 1}
                  </div>
                  <div>
                    <div className="tl-head">{STEP_PENDING_TITLE[k]}</div>
                    <div className="tl-detail">{pending ? "Working…" : "Not reached."}</div>
                  </div>
                </div>
              );
            }
            return <StepRow step={s} key={k} />;
          })}
        </div>
      </div>
    </>
  );
}

function StepRow({ step }: { step: Step }) {
  const icon = { ok: "✓", blocked: "✕", skipped: "–", error: "!" }[step.status];
  const d = step.data ?? {};
  return (
    <div className="tl-step">
      <div className={`tl-icon ${step.status === "skipped" ? "" : step.status}`} aria-hidden>
        {icon}
      </div>
      <div style={{ minWidth: 0 }}>
        <div className="tl-head">
          {step.title}
          {step.label === "real" && <span className="badge badge-real">Real</span>}
          {step.label === "simulated" && <span className="badge badge-sim">Simulated</span>}
        </div>
        <div className="tl-detail">{step.detail}</div>
        {step.key === "review" && typeof d.answer === "string" && (
          <details>
            <summary>
              Read the reviewer’s full answer
              {d.signature_verified === true ? " · signature verified ✓" : ""}
            </summary>
            <pre className="answer">{d.answer}</pre>
            <p className="small faint">
              Signature: {String(d.signature_kind)} ed25519 by <span className="mono">{shortKey(String(d.signing_key ?? ""))}</span>. The
              vault re-checks it on-chain.
            </p>
          </details>
        )}
        {typeof d.explorer_url === "string" && (
          <p className="small" style={{ margin: "6px 0 0" }}>
            <a href={d.explorer_url} target="_blank" rel="noreferrer">
              View the transaction on NearBlocks ↗
            </a>
          </p>
        )}
      </div>
    </div>
  );
}

function Verdict({ result }: { result: RunResult }) {
  const secs = (result.duration_ms / 1000).toFixed(1);
  if (result.outcome === "paid") {
    return (
      <div className="verdict paid" role="status">
        <span className="big" aria-hidden>✓</span>
        <div>
          <h2>Paid: every guardrail passed</h2>
          <p>The vault sent test USDC to the quote’s deposit account. ({secs}s)</p>
        </div>
      </div>
    );
  }
  if (result.outcome === "blocked") {
    return (
      <div className="verdict blocked" role="status">
        <span className="big" aria-hidden>✕</span>
        <div>
          <h2>Blocked on-chain by: {LAYER_LABEL[result.layer ?? ""] ?? "the vault"}</h2>
          <p>
            <span className="mono">BEELINE_VAULT_REJECTED:</span> {result.reason}. No money moved. ({secs}s)
          </p>
        </div>
      </div>
    );
  }
  return (
    <div className="verdict error" role="status">
      <span className="big" aria-hidden>!</span>
      <div>
        <h2>Could not complete the run</h2>
        <p>{result.reason}</p>
      </div>
    </div>
  );
}

function Disclosures({ state }: { state: PlaygroundState | null }) {
  if (!state) return null;
  return (
    <div className="card card-pad">
      <h3>What’s real here</h3>
      <div className="kv">
        <span>
          <span className="badge badge-sim">Simulated</span> Quotes
        </span>
        <b className="small" style={{ fontWeight: 400 }}>{state.disclosures.quotes}</b>
      </div>
      <div className="kv">
        <span>
          <span className="badge badge-real">Real</span> AI review
        </span>
        <b className="small" style={{ fontWeight: 400 }}>{state.disclosures.review}</b>
      </div>
      <div className="kv">
        <span>
          <span className="badge badge-real">Real</span> Payment
        </span>
        <b className="small" style={{ fontWeight: 400 }}>{state.disclosures.payment}</b>
      </div>
      <div className="kv">
        <span>Agent key</span>
        <b className="small" style={{ fontWeight: 400 }}>{state.disclosures.agentKey}</b>
      </div>
    </div>
  );
}
