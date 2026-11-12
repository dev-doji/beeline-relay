"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { usePrivy } from "@privy-io/react-auth";
import { useCreateWallet } from "@privy-io/react-auth/extended-chains";
import { ArrowRight, Ban } from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import { Honeycomb } from "@/components/landing/Honeycomb";
import { ApiError, meChat, meGet, meInvoices, meMessages } from "@/lib/api";
import { ChatApp, type ChatData } from "./ChatApp";

type NearWallet = { type: string; chainType?: string; address?: string };

export default function ChatPage() {
  const { ready, authenticated, login, logout, user, getAccessToken } = usePrivy();
  const { createWallet } = useCreateWallet();
  const [walletError, setWalletError] = useState<string | null>(null);
  const creating = useRef(false);

  const wallet = useMemo(
    () => (user?.linkedAccounts as NearWallet[] | undefined)?.find((a) => a.type === "wallet" && a.chainType === "near")?.address ?? "",
    [user],
  );
  const email = (user?.email as { address?: string } | undefined)?.address ?? "";

  const token = useCallback(async () => {
    const t = await getAccessToken();
    if (!t) throw new ApiError("UNAUTHORIZED", "Please sign in again.", 401);
    return t;
  }, [getAccessToken]);

  // Give every new user a NEAR wallet, created and held through Privy (never by Beeline).
  useEffect(() => {
    if (!ready || !authenticated || !user || wallet || creating.current) return;
    creating.current = true;
    createWallet({ chainType: "near" }).catch(() => {
      creating.current = false;
      setWalletError("Could not create your NEAR wallet yet. You can still tell Bee your own NEAR account.");
    });
  }, [ready, authenticated, user, wallet, createWallet]);

  const load = useCallback(async (): Promise<ChatData> => {
    const t = await token();
    const [profile, m, i] = await Promise.all([meGet(t), meMessages(t), meInvoices(t)]);
    return { profile, messages: m.data, invoices: i.data };
  }, [token]);
  const loadInvoices = useCallback(async () => (await meInvoices(await token())).data, [token]);
  const sendMessage = useCallback(async (message: string) => void (await meChat(await token(), message, wallet)), [token, wallet]);

  if (!ready) return <div className="cx-center"><p className="cx-muted">Loading…</p></div>;

  if (!authenticated) {
    return (
      <div className="lp auth-lp">
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
              <Link href="/attacks">Attack log</Link>
              <Link href="/">Home</Link>
            </nav>
          </div>
        </header>

        <section className="lp-wrap lp-hero">
          <div className="lp-hero-text">
            <p className="cap rise">sign in with your email. no password.</p>
            <h1 className="rise" style={{ animationDelay: "80ms" }}>
              <span className="line">Ask Bee.</span>
              <span className="line">Get paid <mark>safe.</mark></span>
              <span className="line l-ghost">Fraud gets blocked.</span>
            </h1>
            <p className="auth-lead rise" style={{ animationDelay: "140ms" }}>
              Tell Bee what to bill. It makes a signed invoice you can share on any app or show as a QR at the till, and a NEAR vault pays only the exact person and amount.
            </p>
            <div className="lp-cta rise" style={{ animationDelay: "200ms" }}>
              <button className="pbtn dark" onClick={() => login({ loginMethods: ["email"] })}>
                Continue with email <ArrowRight size={18} aria-hidden />
              </button>
              <Link href="/attacks" className="pbtn">See the attack log</Link>
            </div>
            <div className="auth-fine rise" style={{ animationDelay: "260ms" }}>
              <span className="tag">one-time code by email</span>
              <span className="tag">NEAR wallet created for you</span>
              <span className="tag">test USDC on testnet</span>
            </div>
          </div>

          <div aria-hidden>
            <div className="lp-collage">
              <div className="tile t-chat pop" style={{ animationDelay: "150ms" }}>
                <span className="cap">you</span>
                <p>invoice 25 for logo design</p>
              </div>
              <div className="tile t-art pop" style={{ animationDelay: "250ms" }}>
                <Honeycomb />
                <span className="art-word">bee</span>
              </div>
              <div className="tile t-review pop" style={{ animationDelay: "350ms" }}>
                <span className="cap">bee made an invoice</span>
                <span className="auth-invoice">25 USDC</span>
                <span className="l-stamp ok">SIGNED</span>
              </div>
              <div className="tile t-block pop" style={{ animationDelay: "450ms" }}>
                <span className="l-bigic"><Ban size={22} /></span>
                <div>
                  <b>Changed the amount?</b>
                  <span className="mono">blocked by the vault</span>
                </div>
              </div>
            </div>
            <p className="cap auth-note">an example. your real invoices appear after you sign in.</p>
          </div>
        </section>
      </div>
    );
  }

  return (
    <ChatApp
      email={email}
      wallet={wallet}
      logout={() => void logout()}
      load={load}
      loadInvoices={loadInvoices}
      sendMessage={sendMessage}
      walletError={walletError}
    />
  );
}
