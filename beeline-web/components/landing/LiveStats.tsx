"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { type AttackStats, getAttackLog } from "@/lib/api";

/** Live totals from the public attack log (real visitors, decided on-chain). */
export function LiveStats() {
  const [stats, setStats] = useState<AttackStats | null>(null);
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    getAttackLog()
      .then((r) => setStats(r.stats))
      .catch(() => setFailed(true));
  }, []);

  const n = (v: number | undefined) => (stats ? String(v ?? 0) : failed ? "–" : "…");
  return (
    <Link href="/attacks" className="lp-stat rise live-stat" style={{ animationDelay: "320ms" }}>
      <div className="stat-top">
        <b>{n(stats?.attacks)}</b>
        <span className="l-rule" />
        <b className="zero">{n(stats?.misdirected)}</b>
      </div>
      <p className="cap">
        <span>attacks by visitors</span>
        <span>paid to the wrong account</span>
      </p>
      <div className="tags">
        <span className="tag solid">
          <span className="live-dot" aria-hidden /> Live attack log
        </span>
        <span className="tag">NEAR testnet</span>
        <span className="tag">Open source</span>
      </div>
    </Link>
  );
}
