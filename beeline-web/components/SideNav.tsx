"use client";

import Link from "next/link";
import { Activity, Banknote, Github, Landmark, MessagesSquare, Radar, Store, X } from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import { type PlaygroundState, REPO_URL, formatUSDC } from "@/lib/api";

/** Sidebar shared by the Chat and Monitor views. `children` adds view-specific sections. */
export function SideNav({
  active,
  state,
  onClose,
  onNewChat,
  children,
}: {
  active: "chat" | "monitor" | "vendor" | "attacks" | "fund";
  state: PlaygroundState | null;
  onClose: () => void;
  onNewChat?: () => void;
  children?: React.ReactNode;
}) {
  return (
    <aside className="side" aria-label="Playground navigation">
      <div className="side-top">
        <Link href="/" className="brand" aria-label="Beeline home">
          <BeeMark size={26} /> Beeline
        </Link>
        <button className="icon-btn ghost-btn mobile-only" onClick={onClose} aria-label="Close menu">
          <X size={18} />
        </button>
      </div>
      <Link className={`side-item ${active === "vendor" ? "active" : ""}`} href="/vendor">
        <Store size={18} aria-hidden /> Create a pay link
      </Link>
      <Link className={`side-item ${active === "fund" ? "active" : ""}`} href="/fund">
        <Banknote size={18} aria-hidden /> Fund with local currency
      </Link>
      <Link className={`side-item ${active === "attacks" ? "active" : ""}`} href="/attacks">
        <Activity size={18} aria-hidden /> Attack log
      </Link>
      <Link className={`side-item ${active === "monitor" ? "active" : ""}`} href="/monitor">
        <Radar size={18} aria-hidden /> Monitor
      </Link>
      {active === "chat" && onNewChat ? (
        <button className="side-item active" onClick={onNewChat}>
          <MessagesSquare size={18} aria-hidden /> Chat with Bee
        </button>
      ) : (
        <Link className="side-item" href="/playground">
          <MessagesSquare size={18} aria-hidden /> Chat with Bee
        </Link>
      )}
      <a className="side-item" href={state?.vault_url ?? "#"} target="_blank" rel="noreferrer">
        <Landmark size={18} aria-hidden /> Vault on NearBlocks
      </a>
      <a className="side-item" href={REPO_URL} target="_blank" rel="noreferrer">
        <Github size={18} aria-hidden /> Source code
      </a>

      {children}

      <div className="side-foot">
        <div className="vault-chip">
          <span className="vc-ic" aria-hidden>
            <BeeMark size={22} />
          </span>
          <div>
            <b>{state ? `${formatUSDC(state.balances?.pool)} USDC` : "…"}</b>
            <span>
              <span className="live-dot" aria-hidden /> vault on NEAR testnet
            </span>
          </div>
        </div>
      </div>
    </aside>
  );
}
