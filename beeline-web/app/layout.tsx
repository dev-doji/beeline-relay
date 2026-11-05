import type { Metadata, Viewport } from "next";
import "./globals.css";
import { SiteHeader } from "@/components/SiteHeader";


export const metadata: Metadata = {
  title: "Beeline: guardrails for AI agent payments on NEAR",
  description:
    "Chat with an AI payment agent and try to trick it. A NEAR vault enforces genuine quotes, allowlists, limits and a TEE-signed AI reviewer on-chain.",
};

export const viewport: Viewport = { themeColor: "#f7f7fa", width: "device-width", initialScale: 1 };

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
          href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&family=JetBrains+Mono:wght@400;600&family=Plus+Jakarta+Sans:wght@600;700;800&display=swap"
        />
      </head>
      <body>
        <SiteHeader />
        <main>{children}</main>
      </body>
    </html>
  );
}
