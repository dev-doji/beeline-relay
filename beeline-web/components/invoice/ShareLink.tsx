"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { QRCodeSVG } from "qrcode.react";
import { Check, Copy, ExternalLink, Mail, MessageSquare, Printer, QrCode, Send, Share2, X } from "lucide-react";

/**
 * Share a pay link anywhere: the buttons open each platform's own share page with the message filled in
 * (no accounts or API keys needed), the phone's share sheet covers every other app (Instagram, TikTok,
 * Messenger, Signal...), and the big QR is for a counter or a printed receipt.
 */
export function ShareLink({
  id,
  link,
  vendor,
  amountLabel,
  dareText,
}: {
  id: string;
  link: string;
  vendor: string;
  /** e.g. "0.20 USDC"; empty for a permanent link where the payer chooses the amount (then `id` is empty too). */
  amountLabel: string;
  dareText: string;
}) {
  const [copied, setCopied] = useState(false);
  const [counter, setCounter] = useState(false);
  const [canNative, setCanNative] = useState(false);
  const [dare, setDare] = useState(false);

  useEffect(() => {
    setCanNative(typeof navigator !== "undefined" && typeof navigator.share === "function");
  }, []);
  useEffect(() => {
    if (!counter) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setCounter(false);
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [counter]);

  const text = dare
    ? dareText
    : amountLabel
      ? `Pay ${vendor} ${amountLabel} here (signed invoice, paid on NEAR):`
      : `Pay ${vendor} here, any amount (Beeline signs a one-time invoice, paid on NEAR):`;
  const full = `${text} ${link}`;
  const q = encodeURIComponent;
  const targets: { name: string; href: string }[] = [
    { name: "WhatsApp", href: `https://wa.me/?text=${q(full)}` },
    { name: "Facebook", href: `https://www.facebook.com/sharer/sharer.php?u=${q(link)}&quote=${q(text)}` },
    { name: "X", href: `https://x.com/intent/post?text=${q(text)}&url=${q(link)}` },
    { name: "Telegram", href: `https://t.me/share/url?url=${q(link)}&text=${q(text)}` },
    { name: "LinkedIn", href: `https://www.linkedin.com/sharing/share-offsite/?url=${q(link)}` },
  ];

  async function copy() {
    try {
      await navigator.clipboard?.writeText(link);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      /* clipboard blocked: the link is visible in the box */
    }
  }

  return (
    <div style={{ display: "grid", gap: 12, minWidth: 0, width: "100%" }}>
      <div className="linkbox">
        <code>{link}</code>
        <button className="icon-btn" aria-label="Copy link" onClick={() => void copy()}>
          {copied ? <Check size={16} /> : <Copy size={16} />}
        </button>
      </div>

      <div className="actions" role="group" aria-label="Share the pay link">
        {canNative && (
          <button
            className="btn btn-honey"
            onClick={() => void navigator.share({ title: amountLabel ? `${vendor}: ${amountLabel}` : `Pay ${vendor}`, text, url: link }).catch(() => undefined)}
          >
            <Share2 size={16} aria-hidden /> Share…
          </button>
        )}
        {targets.map((t) => (
          <a key={t.name} className="btn" href={t.href} target="_blank" rel="noreferrer">
            <Send size={15} aria-hidden /> {t.name}
          </a>
        ))}
        <a className="btn" href={`mailto:?subject=${q(`Invoice from ${vendor}`)}&body=${q(full)}`}>
          <Mail size={15} aria-hidden /> Email
        </a>
        <a className="btn" href={`sms:?&body=${q(full)}`}>
          <MessageSquare size={15} aria-hidden /> SMS
        </a>
        <button className="btn" onClick={() => setCounter(true)}>
          <QrCode size={15} aria-hidden /> Show QR at the counter
        </button>
      </div>
      <span className="help">
        Instagram, TikTok and Messenger have no web share button: copy the link into a DM or your bio{canNative ? ", or use Share…" : ""}.
      </span>

      <div className="actions">
        <Link className="btn btn-honey" href={id ? `/pay/${id}` : link}>
          Open the pay page <ExternalLink size={16} aria-hidden />
        </Link>
        {dareText && (
          <button className="btn ghost" onClick={() => setDare((d) => !d)}>
            {dare ? "Use the payment message" : "Dare a friend to trick it instead"}
          </button>
        )}
      </div>

      {counter && (
        <div className="counter" role="dialog" aria-modal="true" aria-label="Pay at the counter">
          <div className="counter-card">
            <button className="icon-btn counter-x" aria-label="Close" onClick={() => setCounter(false)}>
              <X size={18} />
            </button>
            <h2>{vendor}</h2>
            <p className="counter-amt">{amountLabel || "Scan to pay"}</p>
            <QRCodeSVG value={link} size={280} level="M" />
            <p className="help">Scan with a phone camera to pay. Signed invoice, paid on NEAR.</p>
            <button className="btn" onClick={() => window.print()}>
              <Printer size={15} aria-hidden /> Print
            </button>
          </div>
        </div>
      )}
    </div>
  );
}
