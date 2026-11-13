"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { ArrowRight } from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import { REPO_URL } from "@/lib/api";

/** Pill navigation for the website. The app pages have their own sidebar, so they hide this. */
export function SiteHeader() {
  const path = usePathname();
  if (["/playground", "/monitor", "/vendor", "/pay", "/attacks", "/chat"].some((p) => path?.startsWith(p))) return null;
  return (
    <header className="lp-wrap pnav-shell">
      <div className="pnav">
        <Link href="/" className="pnav-logo" aria-label="Beeline home">
          <BeeMark size={26} />
        </Link>
        <span className="pnav-name">Beeline</span>
        <span className="pnav-line" aria-hidden>
          <i /><i /><i /><i /><i />
        </span>
        <nav className="pnav-links" aria-label="Main">
          <Link href="/chat">Sign in</Link>
          <Link href="/attacks">Attack log</Link>
          <a href="#routes">Share</a>
          <a href={REPO_URL} target="_blank" rel="noreferrer">
            GitHub
          </a>
        </nav>
        <Link href="/chat" className="pbtn dark small pnav-cta">
          Open the app <ArrowRight size={15} aria-hidden />
        </Link>
      </div>
    </header>
  );
}
