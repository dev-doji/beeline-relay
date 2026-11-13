"use client";

import { useCallback, useEffect, useState } from "react";
import {
  ApiError,
  type Attack,
  type Attempt,
  type Invoice,
  type PlaygroundState,
  type RunResult,
  getInvoice,
  getInvoiceAttempts,
  getPlayground,
  payInvoice,
} from "@/lib/api";

export interface Current {
  attack: Attack;
  result?: RunResult;
  error?: string;
}

/** Loads one invoice and everything around it, and sends payments or test attacks for it. */
export function usePay(id: string) {
  const [invoice, setInvoice] = useState<Invoice | null>(null);
  const [state, setState] = useState<PlaygroundState | null>(null);
  const [attempts, setAttempts] = useState<Attempt[]>([]);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [current, setCurrent] = useState<Current | null>(null);
  const [busy, setBusy] = useState(false);

  const loadState = useCallback(async () => {
    const st = await getPlayground().catch(() => null);
    if (st) setState(st);
  }, []);
  const loadAttempts = useCallback(async () => {
    const r = await getInvoiceAttempts(id).catch(() => null);
    if (r) setAttempts(r.data);
  }, [id]);

  useEffect(() => {
    (async () => {
      try {
        setInvoice(await getInvoice(id));
        await Promise.all([loadState(), loadAttempts()]);
      } catch (e) {
        setLoadError(e instanceof ApiError ? e.message : "Could not load this invoice.");
      }
    })();
    // Other people may be attacking this link right now.
    const t = setInterval(() => void loadAttempts(), 15000);
    return () => clearInterval(t);
  }, [id, loadState, loadAttempts]);

  async function send(attack: Attack, scrollToResult = false) {
    if (!invoice || busy) return;
    setBusy(true);
    setCurrent({ attack });
    if (scrollToResult) setTimeout(() => document.getElementById("attempt")?.scrollIntoView({ behavior: "smooth", block: "start" }), 50);
    try {
      const res = await payInvoice(invoice.id, attack);
      setInvoice(res.invoice);
      setCurrent({ attack, result: res.run });
      void loadAttempts();
      setTimeout(() => void loadState(), 1200);
    } catch (e) {
      setCurrent({ attack, error: e instanceof ApiError ? e.message : "The attempt failed." });
    } finally {
      setBusy(false);
    }
  }

  return { invoice, state, attempts, loadError, current, busy, send };
}
