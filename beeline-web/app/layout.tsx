import type { Metadata } from "next";
import { Inter, JetBrains_Mono } from "next/font/google";
import Link from "next/link";
import "./globals.css";
import { REPO_URL } from "@/lib/api";

const sans = Inter({ subsets: ["latin"], variable: "--font-sans", display: "swap" });
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-mono", display: "swap" });

export const metadata: Metadata = {
  title: "Beeline: guardrails for AI agent payments on NEAR",
  description:
    "An on-chain NEAR vault that lets AI agents spend, but never misuse, money: signed quotes, allowlists, limits and a TEE-signed AI reviewer, enforced by the contract.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${sans.variable} ${mono.variable}`}>
      <body>
        <header className="wrap nav">
          <Link href="/" className="brand" aria-label="Beeline home">
            <span className="brand-mark" aria-hidden>
              ⬢
            </span>
            Beeline
          </Link>
          <nav className="nav-links" aria-label="Main">
            <Link href="/playground">Playground</Link>
            <a href={REPO_URL} target="_blank" rel="noreferrer">
              GitHub
            </a>
          </nav>
        </header>
        <main>{children}</main>
      </body>
    </html>
  );
}
