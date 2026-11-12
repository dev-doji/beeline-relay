"use client";

import { useEffect, useState } from "react";
import { Menu } from "lucide-react";
import { SideNav } from "@/components/SideNav";
import { type PlaygroundState, getPlayground } from "@/lib/api";

/** The app layout (sidebar + scrolling content) for pages without the live ledger column. */
export function AppFrame({ active, title, icon, children }: { active: "vendor" | "attacks"; title: string; icon: React.ReactNode; children: React.ReactNode }) {
  const [state, setState] = useState<PlaygroundState | null>(null);
  const [sideOpen, setSideOpen] = useState(false);

  useEffect(() => {
    getPlayground()
      .then(setState)
      .catch(() => undefined);
  }, []);

  return (
    <div className={`app-shell no-ledger ${sideOpen ? "side-open" : ""}`}>
      <SideNav active={active} state={state} onClose={() => setSideOpen(false)} />
      <section className="main frame-main">
        <div className="main-top">
          <button className="icon-btn ghost-btn mobile-only" onClick={() => setSideOpen(true)} aria-label="Open menu">
            <Menu size={18} />
          </button>
          <span className="main-title">
            {icon} {title}
          </span>
        </div>
        <div className="frame-scroll">{children}</div>
      </section>
      {sideOpen && <div className="shell-scrim" onClick={() => setSideOpen(false)} aria-hidden />}
    </div>
  );
}
