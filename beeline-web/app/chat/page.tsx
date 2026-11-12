"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import Link from "next/link";
import { usePrivy } from "@privy-io/react-auth";
import { useCreateWallet } from "@privy-io/react-auth/extended-chains";
import { ArrowUp, LogOut, Plus, Wallet } from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
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

const EXAMPLES = ["Set up my shop", "Invoice 2 for logo design", "Give me a permanent pay link", "Show my invoices"];

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

  if (!ready) return <div className="chat-center"><p className="help">Loading…</p></div>;

  if (!authenticated) {
    return (
      <div className="chat-center">
        <div className="chat-signin">
          <BeeMark size={44} />
          <h1>Get paid, protected</h1>
          <p>Sign in with your email. Beeline sets up your wallet, then you ask for an invoice in a chat and share the pay link on any app. A NEAR vault blocks anything that does not match the signed invoice.</p>
          <button className="btn btn-honey" onClick={() => login({ loginMethods: ["email"] })}>Continue with email</button>
          <span className="help">You get a one-time code by email. No password. <Link href="/vendor">Or make a link without an account</Link>.</span>
        </div>
      </div>
    );
  }

  const needsSetup = !profile || !profile.shop_name || !profile.near_account;
  return (
    <div className="chat-shell">
      <aside className="chat-side" aria-label="Your invoices">
        <Link href="/" className="chat-brand" aria-label="Beeline home">
          <BeeMark size={26} /> <b>Beeline</b>
        </Link>
        <button className="btn" onClick={() => (setShowCash(false), input.current?.focus())}>
          <Plus size={15} aria-hidden /> New invoice
        </button>
        <div className="chat-list">
          <span className="help">Your invoices</span>
          {invoices.length === 0 && <span className="help">None yet. Ask for one in the chat.</span>}
          {invoices.map((i) => (
            <Link key={i.id} href={`/pay/${i.id}`} className="chat-inv">
              <span>{formatUSDC(i.amount)} USDC</span>
              <span className={i.status === "paid" ? "tag ok" : "tag"}>{i.status === "paid" ? "Paid" : "Open"}</span>
            </Link>
          ))}
        </div>
        <button className="btn ghost" onClick={() => setShowCash((s) => !s)}>
          <Wallet size={15} aria-hidden /> Cash out to bank
        </button>
        <div className="chat-user">
          <span title={email}>{email || "Signed in"}</span>
          {wallet && <span className="help" title={wallet}>Wallet {shortKey(wallet, 5)}</span>}
          <button className="btn ghost" onClick={() => void logout()}>
            <LogOut size={15} aria-hidden /> Sign out
          </button>
        </div>
      </aside>

      <section className="chat-main">
        <div className="chat-scroll">
          <div className="chat-col">
            {showCash && <CashOut />}
            <div className="msg bot">
              <p>
                Hi, I&apos;m Bee, your payment assistant{profile?.shop_name ? ` for ${profile.shop_name}` : ""}. Here is what I can do:
              </p>
              <ul className="chat-caps">
                {CAPABILITIES.map((c) => (
                  <li key={c}>{c}</li>
                ))}
              </ul>
              <p>{needsSetup ? "Let's start: what is your shop or business name, and which NEAR account should receive the money? You can say \"use my wallet\" and I will use the wallet I created for you." : "What would you like to do?"}</p>
              {messages.length === 0 && (
                <div className="chips">
                  {EXAMPLES.map((e) => (
                    <button key={e} className="chip" onClick={() => void send(e)}>{e}</button>
                  ))}
                </div>
              )}
            </div>
            {messages.map((m) => {
              const inv = m.invoice_id ? byId.get(m.invoice_id) : undefined;
              const link = inv && typeof window !== "undefined" ? `${window.location.origin}/pay/${inv.id}` : "";
              return (
                <div key={m.id} className={m.role === "user" ? "msg me" : "msg bot"}>
                  <p>{m.body.replace(PERMANENT, "").trim()}</p>
                  {m.role === "assistant" && PERMANENT.test(m.body) && typeof window !== "undefined" && (
                    <div className="panel msg-card">
                      <b>Your permanent pay link</b>
                      <ShareLink id="" link={`${window.location.origin}${m.body.match(PERMANENT)![0]}`} vendor={profile?.shop_name || "my shop"} amountLabel="" dareText="" />
                    </div>
                  )}
                  {inv && (
                    <div className="panel msg-card">
                      <div className="msg-card-top">
                        <b>{formatUSDC(inv.amount)} USDC</b>
                        <span className={inv.status === "paid" ? "tag ok" : "tag"}>{inv.status === "paid" ? "Paid" : "Open"}</span>
                      </div>
                      <ShareLink id={inv.id} link={link} vendor={inv.vendor_name} amountLabel={`${formatUSDC(inv.amount)} USDC`} dareText={dare(inv.vendor_name, link)} />
                    </div>
                  )}
                </div>
              );
            })}
            {busy && <div className="msg bot"><p className="help">Making your invoice…</p></div>}
            {error && <div className="alert" role="alert">{error}</div>}
            <div ref={bottom} />
          </div>
        </div>
        <form className="composer" onSubmit={(e) => (e.preventDefault(), void send(text))}>
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
          <button className="send" type="submit" aria-label="Send" disabled={!text.trim() || busy}>
            <ArrowUp size={18} />
          </button>
        </form>
        <p className="chat-foot">Test USDC on NEAR testnet. Invoices are capped at 5 USDC. I understand simple invoice requests, not general questions.</p>
      </section>
    </div>
  );
}
