import type { Metadata, Viewport } from "next";
import { IBM_Plex_Sans, JetBrains_Mono, Space_Grotesk } from "next/font/google";
import Link from "next/link";
import "./globals.css";
import { REPO_URL } from "@/lib/api";
import { BeeMark } from "@/components/BeeMark";

const display = Space_Grotesk({ subsets: ["latin"], variable: "--font-display", display: "swap" });
const sans = IBM_Plex_Sans({ subsets: ["latin"], weight: ["400", "500", "600", "700"], variable: "--font-sans", display: "swap" });
const mono = JetBrains_Mono({ subsets: ["latin"], variable: "--font-mono", display: "swap" });

export const metadata: Metadata = {
  title: "Beeline: guardrails for AI agent payments on NEAR",
  description:
    "Chat with an AI payment agent and try to trick it. A NEAR vault enforces genuine quotes, allowlists, limits and a TEE-signed AI reviewer on-chain.",
};

export const viewport: Viewport = { themeColor: "#0b1020", width: "device-width", initialScale: 1 };

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en" className={`${display.variable} ${sans.variable} ${mono.variable}`}>
      <body>
        <header className="wrap nav">
          <Link href="/" className="brand" aria-label="Beeline home">
            <BeeMark size={30} />
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
