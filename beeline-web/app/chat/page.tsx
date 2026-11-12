"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { usePrivy } from "@privy-io/react-auth";
import { useCreateWallet } from "@privy-io/react-auth/extended-chains";
import { ArrowRight, ArrowUp, Ban, FileText, Link2, ListChecks, LogOut, Plus, Store, Wallet } from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import { Honeycomb } from "@/components/landing/Honeycomb";
import { ShareLink } from "@/components/invoice/ShareLink";
import { CashOut } from "@/components/invoice/CashOut";
import {
  ApiError,
  type ChatMessage,
  type Invoice,
  type Profile,
  formatUSDC,
  meChat,
  meGet,
  meInvoices,
  meMessages,
  shortKey,
} from "@/lib/api";

const EXAMPLES = [
  { icon: Store, text: "Set up my shop" },
  { icon: FileText, text: "Invoice 2 for logo design" },
  { icon: Link2, text: "Give me a permanent pay link" },
  { icon: ListChecks, text: "Show my invoices" },
];

function BeeAvatar() {
  return (
    <span className="bee-av" aria-hidden>
      <BeeMark size={22} />
    </span>
  );
}

const CAPABILITIES = [
  "Set up your shop and the NEAR account that gets paid",
  "Make an invoice and give you a link to share on WhatsApp, Facebook, X, Telegram, email or SMS, or a QR for your counter",
  "Give you one permanent pay link where each customer chooses the amount",
  "Show your invoices and which ones are paid",
  "Explain how the vault blocks fake or changed payments",
];

const PERMANENT = /\/pay\/to\?[^\s)]+/;

const dare = (vendor: string, link: string) => `Can you trick my AI payment agent into paying you instead of ${vendor}? Try anything: ${link}`;

type NearWallet = { type: string; chainType?: string; address?: string };

export default function ChatPage() {
  const { ready, authenticated, login, logout, user, getAccessToken } = usePrivy();
  const { createWallet } = useCreateWallet();
  const [profile, setProfile] = useState<Profile | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [showCash, setShowCash] = useState(false);
  const creating = useRef(false);
  const bottom = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);

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

  const fail = (e: unknown) => setError(e instanceof ApiError ? e.message : "Something went wrong. Try again.");

  // Give every new user a NEAR wallet, created and held through Privy (never by Beeline).
  useEffect(() => {
    if (!ready || !authenticated || !user || wallet || creating.current) return;
    creating.current = true;
    createWallet({ chainType: "near" }).catch(() => {
      creating.current = false;
      setError("Could not create your NEAR wallet yet. You can still paste your own NEAR account below.");
    });
  }, [ready, authenticated, user, wallet, createWallet]);

  const refresh = useCallback(async () => {
    try {
      const t = await token();
      const [p, m, i] = await Promise.all([meGet(t), meMessages(t), meInvoices(t)]);
      setProfile(p);
      setMessages(m.data);
      setInvoices(i.data);
    } catch (e) {
      fail(e);
    }
  }, [token]);

  useEffect(() => {
    if (ready && authenticated) void refresh();
  }, [ready, authenticated, refresh]);

  // Pick up "paid" status without a reload.
  useEffect(() => {
    if (!authenticated) return;
    const t = setInterval(() => {
      token().then(meInvoices).then((r) => setInvoices(r.data)).catch(() => undefined);
    }, 15000);
    return () => clearInterval(t);
  }, [authenticated, token]);

  useEffect(() => {
    bottom.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, busy]);

  const byId = useMemo(() => new Map(invoices.map((i) => [i.id, i])), [invoices]);

  async function send(message: string) {
    const m = message.trim();
    if (!m || busy) return;
    setBusy(true);
    setError(null);
    setText("");
    setMessages((xs) => [...xs, { id: -Date.now(), role: "user", body: m, invoice_id: null, created_at: new Date().toISOString() }]);
    try {
      const t = await token();
      await meChat(t, m, wallet);
      await refresh();
    } catch (e) {
      fail(e);
    } finally {
      setBusy(false);
      input.current?.focus();
    }
  }

  if (!ready) return <div className="cx-center"><p className="help">Loading…</p></div>;

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

  const needsSetup = !profile || !profile.shop_name || !profile.near_account;
  return (
    <div className="lp cx-shell">
      <aside className="cx-side" aria-label="Your invoices">
        <Link href="/" className="cx-brand" aria-label="Beeline home">
          <span className="cx-logo"><BeeMark size={22} /></span> <span>Beeline</span>
        </Link>
        <button className="pbtn dark" onClick={() => (setShowCash(false), input.current?.focus())}>
          <Plus size={15} aria-hidden /> New invoice
        </button>
        <div className="cx-list">
          <span className="cap">your invoices</span>
          {invoices.length === 0 && <span className="help">None yet. Ask for one in the chat.</span>}
          {invoices.map((i) => (
            <Link key={i.id} href={`/pay/${i.id}`} className="cx-inv">
              <span>{formatUSDC(i.amount)} USDC</span>
              <span className={i.status === "paid" ? "cx-tag ok" : "cx-tag"}>{i.status === "paid" ? "Paid" : "Open"}</span>
            </Link>
          ))}
        </div>
        <button className="pbtn" onClick={() => setShowCash((s) => !s)}>
          <Wallet size={15} aria-hidden /> Cash out to bank
        </button>
        <div className="cx-user">
          <span title={email}>{email || "Signed in"}</span>
          {wallet && <span className="help" title={wallet}>Wallet {shortKey(wallet, 5)}</span>}
          <button className="pbtn small" onClick={() => void logout()}>
            <LogOut size={14} aria-hidden /> Sign out
          </button>
        </div>
      </aside>

      <section className="cx-main">
        <header className="cx-top">
          <BeeAvatar />
          <div>
            <b>Bee</b>
            <span className="cx-status">
              <i aria-hidden /> online · near testnet
            </span>
          </div>
        </header>
        <div className="cx-scroll">
          <div className="cx-col">
            {showCash && <CashOut />}
            <div className="cx-msg bot">
              <BeeAvatar />
              <div className="cx-body">
              <p>
                Hi, I&apos;m Bee, your payment assistant{profile?.shop_name ? ` for ${profile.shop_name}` : ""}. Here is what I can do:
              </p>
              <ul className="cx-caps">
                {CAPABILITIES.map((c) => (
                  <li key={c}>{c}</li>
                ))}
              </ul>
              <p>{needsSetup ? "Let's start: what is your shop or business name, and which NEAR account should receive the money? You can say \"use my wallet\" and I will use the wallet I created for you." : "What would you like to do?"}</p>
              {messages.length === 0 && (
                <div className="cx-chips">
                  {EXAMPLES.map((e) => (
                    <button key={e.text} className="cx-chip" onClick={() => void send(e.text)}>
                      <e.icon size={14} aria-hidden /> {e.text}
                    </button>
                  ))}
                </div>
              )}
              </div>
            </div>
            {messages.map((m) => {
              const inv = m.invoice_id ? byId.get(m.invoice_id) : undefined;
              const link = inv && typeof window !== "undefined" ? `${window.location.origin}/pay/${inv.id}` : "";
              return (
                <div key={m.id} className={m.role === "user" ? "cx-msg me" : "cx-msg bot"}>
                  {m.role === "assistant" && <BeeAvatar />}
                  <div className="cx-body">
                  <p>{m.body.replace(PERMANENT, "").trim()}</p>
                  {m.role === "assistant" && PERMANENT.test(m.body) && typeof window !== "undefined" && (
                    <div className="panel cx-card">
                      <b>Your permanent pay link</b>
                      <ShareLink id="" link={`${window.location.origin}${m.body.match(PERMANENT)![0]}`} vendor={profile?.shop_name || "my shop"} amountLabel="" dareText="" />
                    </div>
                  )}
                  {inv && (
                    <div className="panel cx-card">
                      <div className="cx-card-top">
                        <b>{formatUSDC(inv.amount)} USDC</b>
                        <span className={inv.status === "paid" ? "cx-tag ok" : "cx-tag"}>{inv.status === "paid" ? "Paid" : "Open"}</span>
                      </div>
                      <ShareLink id={inv.id} link={link} vendor={inv.vendor_name} amountLabel={`${formatUSDC(inv.amount)} USDC`} dareText={dare(inv.vendor_name, link)} />
                    </div>
                  )}
                  </div>
                </div>
              );
            })}
            {busy && (
              <div className="cx-msg bot">
                <BeeAvatar />
                <div className="cx-body">
                  <span className="cx-typing" role="status" aria-label="Bee is cx-typing">
                    <i /><i /><i />
                  </span>
                </div>
              </div>
            )}
            {error && <div className="alert" role="alert">{error}</div>}
            <div ref={bottom} />
          </div>
        </div>
        <form className="cx-composer" onSubmit={(e) => (e.preventDefault(), void send(text))}>
          <textarea
            ref={input}
            value={text}
            rows={1}
            maxLength={500}
            placeholder={needsSetup ? "Tell Bee your shop name and NEAR account" : "Message Bee, for example: invoice 25 for logo design"}
            disabled={busy}
            onChange={(e) => setText(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.shiftKey) {
                e.preventDefault();
                void send(text);
              }
            }}
          />
          <button className="cx-send" type="submit" aria-label="Send" disabled={!text.trim() || busy}>
            <ArrowUp size={18} />
          </button>
        </form>
        <p className="cx-foot">test usdc on near testnet, invoices up to 5 usdc. bee can make mistakes, but it cannot cx-send or move money.</p>
      </section>
    </div>
  );
}
