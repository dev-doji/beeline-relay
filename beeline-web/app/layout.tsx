import type { Metadata, Viewport } from "next";
import Link from "next/link";
import "./globals.css";
import { REPO_URL } from "@/lib/api";
import { BeeMark } from "@/components/BeeMark";


export const metadata: Metadata = {
  title: "Beeline: guardrails for AI agent payments on NEAR",
  description:
    "Chat with an AI payment agent and try to trick it. A NEAR vault enforces genuine quotes, allowlists, limits and a TEE-signed AI reviewer on-chain.",
};

export const viewport: Viewport = { themeColor: "#0b1020", width: "device-width", initialScale: 1 };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <head>
        {/* Loaded by the browser (not at build time), so builds never depend on a font download. */}
        <link rel="preconnect" href="https://fonts.googleapis.com" />
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="" />
        {/* eslint-disable-next-line @next/next/no-page-custom-font */}
        <link
          rel="stylesheet"
          href="https://fonts.googleapis.com/css2?family=IBM+Plex+Sans:wght@400;500;600;700&family=JetBrains+Mono:wght@400;600&family=Space+Grotesk:wght@500;600;700&display=swap"
        />
      </head>
      <body>
        <header className="wrap nav">
          <Link href="/" className="brand" aria-label="Beeline home">
            <BeeMark size={30} />
            Beeline
          </Link>
          <nav className="nav-links" aria-label="Main">
            <Link href="/playground">Playground</Link>
            <Link href="/vendor">Vendors</Link>
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
