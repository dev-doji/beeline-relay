"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { ComponentType } from "react";
import Link from "next/link";
import {
  ArrowRight,
  ChevronDown,
  CirclePlus,
  Copy,
  FileText,
  Landmark,
  Link2,
  ListChecks,
  LogOut,
  Menu,
  MessageCircle,
  PanelLeftClose,
  PanelLeftOpen,
  Receipt,
  Send,
  ShieldCheck,
  Store,
} from "lucide-react";
import { BeeMark } from "@/components/BeeMark";
import { ShareLink } from "@/components/invoice/ShareLink";
import { CashOut } from "@/components/invoice/CashOut";
import { ApiError, type ChatMessage, type Invoice, type Profile, formatUSDC, shortKey } from "@/lib/api";

type View = "welcome" | "chat" | "invoices" | "cash";
type Icon = ComponentType<{ size?: number; "aria-hidden"?: boolean }>;
type Action = {
  id: string;
  icon: Icon;
  title: string;
  sub: string;
  desc: string;
  note: string;
  send?: string;
  prefill?: string;
  view?: View;
};

// What Bee can do. These are the cards on the welcome screen and the "/" menu.
const ACTIONS: Action[] = [
  { id: "shop", icon: Store, title: "Set up your shop", sub: "Account", desc: "Tell me your shop name and the NEAR account that gets paid.", note: "Start here", send: "Set up my shop" },
  { id: "invoice", icon: FileText, title: "Make an invoice", sub: "Invoices", desc: "Say what you are billing and I make a signed pay link to share.", note: "Most used", prefill: "Invoice 25 for " },
  { id: "permanent", icon: Link2, title: "Permanent pay link", sub: "Payment links", desc: "One link to keep. Each customer chooses the amount.", note: "Keep forever", send: "Give me a permanent pay link" },
  { id: "list", icon: ListChecks, title: "Show my invoices", sub: "Invoices", desc: "See your invoices and which ones are paid.", note: "Anytime", send: "Show my invoices" },
  { id: "cash", icon: Landmark, title: "Cash out to bank", sub: "Cash out", desc: "Turn USDC into naira in a Nigerian bank account.", note: "Price preview", view: "cash" },
  { id: "vault", icon: ShieldCheck, title: "How the vault protects you", sub: "Safety", desc: "Why changed amounts and swapped payees are blocked.", note: "Learn", send: "Explain how the vault protects me" },
];

const PERMANENT = /\/pay\/to\?[^\s)]+/;

const dare = (vendor: string, link: string) => `Can you trick my AI payment agent into paying you instead of ${vendor}? Try anything: ${link}`;

function BeeAvatar({ size = 30 }: { size?: number }) {
  return (
    <span className="cx-av" style={{ width: size, height: size }} aria-hidden>
      <BeeMark size={Math.round(size * 0.62)} />
    </span>
  );
}

function StatusTag({ status }: { status: string }) {
  return <span className={status === "paid" ? "cx-tag ok" : "cx-tag"}>{status === "paid" ? "Paid" : "Open"}</span>;
}

export interface ChatData {
  profile: Profile;
  messages: ChatMessage[];
  invoices: Invoice[];
}

/**
 * The signed-in assistant. It knows nothing about Privy or the network: the page hands it the
 * user's email and wallet, and functions to load data and send a message.
 */
export function ChatApp({
  email,
  wallet,
  logout,
  load,
  loadInvoices,
  sendMessage,
  walletError,
}: {
  email: string;
  wallet: string;
  logout: () => void;
  load: () => Promise<ChatData>;
  loadInvoices: () => Promise<Invoice[]>;
  sendMessage: (message: string) => Promise<void>;
  walletError?: string | null;
}) {
  const [profile, setProfile] = useState<Profile | null>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([]);
  const [invoices, setInvoices] = useState<Invoice[]>([]);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [view, setView] = useState<View>("welcome");
  // null = automatic: open on desktop, closed on phones (decided by CSS). A click overrides it.
  const [side, setSide] = useState<boolean | null>(null);
  const [slash, setSlash] = useState(false);
  const [acct, setAcct] = useState(false);
  const [copied, setCopied] = useState(false);
  const bottom = useRef<HTMLDivElement>(null);
  const input = useRef<HTMLTextAreaElement>(null);

  const fail = (e: unknown) => setError(e instanceof ApiError ? e.message : "Something went wrong. Try again.");

  const refresh = useCallback(async () => {
    try {
      const d = await load();
      setProfile(d.profile);
      setMessages(d.messages);
      setInvoices(d.invoices);
    } catch (e) {
      fail(e);
    }
  }, [load]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Pick up "paid" status without a reload.
  useEffect(() => {
    const t = setInterval(() => {
      loadInvoices().then(setInvoices).catch(() => undefined);
    }, 15000);
    return () => clearInterval(t);
  }, [loadInvoices]);

  useEffect(() => {
    if (view === "chat") bottom.current?.scrollIntoView({ behavior: "smooth" });
  }, [messages, busy, view]);

  // "/" opens the menu of everything Bee can do; Escape closes it.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (e.key === "Escape") setSlash(false);
      if (e.key === "/" && tag !== "INPUT" && tag !== "TEXTAREA") {
        e.preventDefault();
        input.current?.focus();
        setSlash(true);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  const byId = useMemo(() => new Map(invoices.map((i) => [i.id, i])), [invoices]);

  async function send(message: string) {
    const m = message.trim();
    if (!m || busy) return;
    setBusy(true);
    setError(null);
    setText("");
    setSlash(false);
    setView("chat");
    setMessages((xs) => [...xs, { id: -Date.now(), role: "user", body: m, invoice_id: null, created_at: new Date().toISOString() }]);
    try {
      await sendMessage(m);
      await refresh();
    } catch (e) {
      fail(e);
    } finally {
      setBusy(false);
      input.current?.focus();
    }
  }

  function run(a: Action) {
    setSlash(false);
    if (a.view) return setView(a.view);
    if (a.send) return void send(a.send);
    if (a.prefill) {
      setText(a.prefill);
      input.current?.focus();
    }
  }

  async function copyWallet() {
    try {
      await navigator.clipboard?.writeText(wallet);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* the address is visible */
    }
  }

  const needsSetup = !profile || !profile.shop_name || !profile.near_account;
  const initial = (email || profile?.shop_name || "B").trim().charAt(0).toUpperCase();
  const nav: { id: View; icon: Icon; label: string }[] = [
    { id: "welcome", icon: CirclePlus, label: "New Chat" },
    { id: "chat", icon: MessageCircle, label: "Recent Chat" },
    { id: "invoices", icon: Receipt, label: "Invoices" },
    { id: "cash", icon: Landmark, label: "Cash out" },
  ];

  return (
    <div className={side === true ? "cx-shell side-open" : side === false ? "cx-shell side-closed" : "cx-shell"}>
      <aside className="cx-side" aria-label="Navigation">
        <div className="cx-side-head">
          <Link href="/" className="cx-brand" aria-label="Beeline home">
            <span className="cx-logo"><BeeMark size={16} /></span>
            <span>Beeline</span>
          </Link>
          <button className="cx-icon" aria-label="Close sidebar" onClick={() => setSide(false)}>
            <PanelLeftClose size={16} aria-hidden />
          </button>
        </div>

        <nav className="cx-nav">
          {nav.map((n) => (
            <button
              key={n.label}
              className={view === n.id ? "cx-nav-item active" : "cx-nav-item"}
              onClick={() => {
                setView(n.id);
                if (window.innerWidth < 860) setSide(false);
                if (n.id === "welcome") input.current?.focus();
              }}
            >
              <n.icon size={16} aria-hidden /> {n.label}
            </button>
          ))}
          <Link href="/attacks" className="cx-nav-item">
            <ShieldCheck size={16} aria-hidden /> Attack log
          </Link>
        </nav>

        <hr className="cx-rule" />
        <div className="cx-recent-label">RECENT INVOICES</div>
        <div className="cx-recent">
          {invoices.length === 0 && <span className="cx-muted cx-pad">No invoices yet.</span>}
          {invoices.slice(0, 12).map((i) => (
            <Link key={i.id} href={`/pay/${i.id}`} className="cx-recent-item">
              <b>{formatUSDC(i.amount)} USDC</b>
              <span>
                <i className={i.status === "paid" ? "dot ok" : "dot"} aria-hidden />
                {i.status === "paid" ? "paid" : "open"} · {new Date(i.created_at).toLocaleDateString(undefined, { day: "numeric", month: "short" })}
              </span>
            </Link>
          ))}
        </div>

        <div className="cx-side-foot">
          <div className="cx-acct-wrap">
            {acct && (
              <div className="cx-menu" role="menu">
                {wallet && (
                  <button role="menuitem" onClick={() => void copyWallet()}>
                    <Copy size={14} aria-hidden /> {copied ? "Copied" : `Wallet ${shortKey(wallet, 5)}`}
                  </button>
                )}
                <button role="menuitem" onClick={logout}>
                  <LogOut size={14} aria-hidden /> Sign out
                </button>
              </div>
            )}
            <button className="cx-acct" aria-expanded={acct} onClick={() => setAcct((a) => !a)}>
              <span className="cx-user-av">{initial}</span>
              <span className="cx-acct-name">{email || profile?.shop_name || "Signed in"}</span>
              <ChevronDown size={15} aria-hidden />
            </button>
          </div>
        </div>
      </aside>
      {side === true && <button className="cx-scrim" aria-label="Close sidebar" onClick={() => setSide(false)} />}

      <section className="cx-main">
        <header className="cx-top">
          <button className="cx-icon cx-menu-btn" aria-label="Open sidebar" onClick={() => setSide(true)}>
            <span className="cx-hide-desktop"><Menu size={18} aria-hidden /></span>
            <span className="cx-hide-mobile"><PanelLeftOpen size={18} aria-hidden /></span>
          </button>
          <span className="cx-spacer" />
          <button className="cx-pill" onClick={() => setView("cash")}>
            <Landmark size={14} aria-hidden /> Cash out
          </button>
        </header>

        <div className="cx-scroll">
          {view === "welcome" && (
            <div className="cx-welcome">
              <div className="cx-greet">
                <BeeAvatar size={44} />
                <h1>Hi, I&apos;m Bee{profile?.shop_name ? `, for ${profile.shop_name}` : ""}</h1>
                <p>{needsSetup ? "Let's start by setting up your shop. Pick a card, or just tell me what you need." : "Pick a card, or just tell me what you need."}</p>
              </div>
              <div className="cx-cards" role="list">
                {ACTIONS.map((a) => (
                  <div key={a.id} className="cx-card" role="listitem">
                    <div className="cx-card-top">
                      <span className="cx-card-ic"><a.icon size={16} aria-hidden /></span>
                      <div>
                        <b>{a.title}</b>
                        <span>{a.sub}</span>
                      </div>
                    </div>
                    <p>{a.desc}</p>
                    <div className="cx-card-foot">
                      <button className="cx-get" onClick={() => run(a)}>
                        Get this <ArrowRight size={13} aria-hidden />
                      </button>
                      <span className="cx-note">{a.note}</span>
                    </div>
                  </div>
                ))}
              </div>
              <p className="cx-hint">
                <kbd>/</kbd> Press / to see everything I can do
              </p>
              {(error || walletError) && <div className="alert" role="alert">{error || walletError}</div>}
            </div>
          )}

          {view === "chat" && (
            <div className="cx-thread">
              {messages.length === 0 && <p className="cx-muted cx-center-text">No messages yet. Pick New Chat to get started.</p>}
              {messages.map((m) => {
                const inv = m.invoice_id ? byId.get(m.invoice_id) : undefined;
                const link = inv ? `${window.location.origin}/pay/${inv.id}` : "";
                const perm = m.role === "assistant" ? m.body.match(PERMANENT)?.[0] : undefined;
                if (m.role === "user") {
                  return (
                    <div key={m.id} className="cx-row me">
                      <p className="cx-bubble">{m.body}</p>
                    </div>
                  );
                }
                return (
                  <div key={m.id} className="cx-row bot">
                    <BeeAvatar />
                    <div className="cx-reply">
                      <p>{m.body.replace(PERMANENT, "").trim()}</p>
                      {perm && (
                        <div className="cx-box">
                          <div className="cx-box-top"><b>Your permanent pay link</b></div>
                          <ShareLink id="" link={`${window.location.origin}${perm}`} vendor={profile?.shop_name || "my shop"} amountLabel="" dareText="" />
                        </div>
                      )}
                      {inv && (
                        <div className="cx-box">
                          <div className="cx-box-top">
                            <b>{formatUSDC(inv.amount)} USDC</b>
                            <StatusTag status={inv.status} />
                          </div>
                          <ShareLink id={inv.id} link={link} vendor={inv.vendor_name} amountLabel={`${formatUSDC(inv.amount)} USDC`} dareText={dare(inv.vendor_name, link)} />
                        </div>
                      )}
                    </div>
                  </div>
                );
              })}
              {busy && (
                <div className="cx-row bot">
                  <BeeAvatar />
                  <div className="cx-reply">
                    <span className="cx-typing" role="status" aria-label="Bee is typing">
                      <i /><i /><i />
                    </span>
                  </div>
                </div>
              )}
              {error && <div className="alert" role="alert">{error}</div>}
              <div ref={bottom} />
            </div>
          )}

          {view === "invoices" && (
            <div className="cx-panel-view">
              <h2>Your invoices</h2>
              {invoices.length === 0 && <p className="cx-muted">No invoices yet. Ask Bee to make one.</p>}
              <div className="cx-grid">
                {invoices.map((i) => {
                  const link = `${window.location.origin}/pay/${i.id}`;
                  return (
                    <div key={i.id} className="cx-box">
                      <div className="cx-box-top">
                        <b>{formatUSDC(i.amount)} USDC</b>
                        <StatusTag status={i.status} />
                      </div>
                      <span className="cx-muted">{i.items.map((x) => x.name).join(", ") || i.memo || "Invoice"}</span>
                      <details>
                        <summary>Share</summary>
                        <ShareLink id={i.id} link={link} vendor={i.vendor_name} amountLabel={`${formatUSDC(i.amount)} USDC`} dareText={dare(i.vendor_name, link)} />
                      </details>
                      <Link className="cx-link" href={`/pay/${i.id}`}>Open the pay page</Link>
                    </div>
                  );
                })}
              </div>
            </div>
          )}

          {view === "cash" && (
            <div className="cx-panel-view">
              <CashOut />
            </div>
          )}
        </div>

        <div className="cx-dock">
          {slash && (
            <div className="cx-slash" role="menu" aria-label="Everything Bee can do">
              {ACTIONS.map((a) => (
                <button key={a.id} role="menuitem" onClick={() => run(a)}>
                  <span className="cx-card-ic"><a.icon size={15} aria-hidden /></span>
                  <span>
                    <b>{a.title}</b>
                    <i>{a.desc}</i>
                  </span>
                </button>
              ))}
            </div>
          )}
          <form className="cx-composer" onSubmit={(e) => (e.preventDefault(), void send(text))}>
            <button type="button" className="cx-plus" aria-label="Everything Bee can do" aria-expanded={slash} onClick={() => setSlash((s) => !s)}>
              <CirclePlus size={20} aria-hidden />
            </button>
            <textarea
              ref={input}
              value={text}
              rows={1}
              maxLength={500}
              placeholder={needsSetup ? "Tell Bee your shop name and NEAR account" : "Tell me what you need!"}
              disabled={busy}
              onChange={(e) => {
                setText(e.target.value);
                if (e.target.value === "/") setSlash(true);
                else if (slash) setSlash(false);
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter" && !e.shiftKey) {
                  e.preventDefault();
                  void send(text);
                }
              }}
            />
            <button className="cx-send" type="submit" aria-label="Send" disabled={!text.trim() || busy}>
              <Send size={16} aria-hidden />
            </button>
          </form>
          <p className="cx-foot">Test USDC on NEAR testnet, invoices up to 5 USDC. Bee can make mistakes, but it cannot send or move money.</p>
        </div>
      </section>
    </div>
  );
}
