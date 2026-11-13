"use client";

import Link from "next/link";
import { Radar } from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import "./pay.css";

/**
 * The page a customer sees when they open a pay link. It is for strangers, so it carries none of the
 * vendor app's navigation: a slim top bar and a normal page that scrolls with the browser.
 */
export function PayShell({ children }: { children: React.ReactNode }) {
  return (
    <div className="px-page">
      <header className="px-top">
        <Link href="/" className="px-brand" aria-label="Beeline home">
          <span className="px-logo">
            <BeeMark size={16} onDark />
          </span>
          Beeline
        </Link>
        <span className="px-spacer" />
        <Link href="/attacks" className="px-link">
          <Radar size={14} aria-hidden /> Attack log
        </Link>
        <Link href="/chat" className="px-btn dark sm">
          Open the app
        </Link>
      </header>
      <main className="px-main">{children}</main>
    </div>
  );
}
