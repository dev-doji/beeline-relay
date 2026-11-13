"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Activity, ArrowRight } from "lucide-react";
import { AppFrame } from "@/components/AppFrame";
import { AttemptList } from "@/components/attacks/AttemptList";
import { ApiError, type AttackStats, type Attempt, getAttackLog } from "@/lib/api";

/** The public attack log: every real attempt on every pay link, with totals computed by the API. */
export default function AttacksPage() {
  const [data, setData] = useState<Attempt[] | null>(null);
  const [stats, setStats] = useState<AttackStats | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const r = await getAttackLog();
      setData(r.data);
      setStats(r.stats);
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Could not load the attack log.");
    }
  }, []);

  useEffect(() => {
    void load();
    const t = setInterval(() => void load(), 15000);
    return () => clearInterval(t);
  }, [load]);

  return (
    <AppFrame active="attacks" title="Attack log" icon={<Activity size={16} aria-hidden />}>
      <div className="frame-inner">
        <div className="page-head">
          <span className="pill pill-honey">
            <span className="live-dot" aria-hidden /> Live · NEAR testnet
          </span>
          <h1>Everyone who tried</h1>
          <p>Every attempt on every pay link, written by real visitors and decided on-chain by the vault. Updates every 15 seconds.</p>
        </div>

        <div className="stat-row" aria-label="Totals">
          <div className="stat-card">
            <b>{stats?.attacks ?? "…"}</b>
            <span>attacks tried</span>
          </div>
          <div className="stat-card bad">
            <b>{stats?.blocked ?? "…"}</b>
            <span>blocked on-chain</span>
          </div>
          <div className="stat-card ok">
            <b>{stats?.paid ?? "…"}</b>
            <span>honest payments</span>
          </div>
          <div className={`stat-card ${stats && stats.misdirected > 0 ? "bad" : "zero"}`}>
            <b>{stats?.misdirected ?? "…"}</b>
            <span>paid to the wrong account</span>
          </div>
        </div>

        {error && (
          <div className="alert" role="alert">
            {error}
          </div>
        )}

        <AttemptList
          attempts={data ?? []}
          showInvoice
          empty={
            data === null ? (
              "Loading…"
            ) : (
              <>
                No attempts yet.{" "}
                <Link href="/chat">
                  Make a pay link and dare someone <ArrowRight size={14} aria-hidden />
                </Link>
              </>
            )
          }
        />
      </div>
    </AppFrame>
  );
}
