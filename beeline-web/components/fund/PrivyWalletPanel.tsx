"use client";

import { useCallback, useEffect, useState } from "react";
import { PrivyProvider, useLogout, usePrivy, useSendTransaction, useWallets } from "@privy-io/react-auth";
import { ArrowUpRight, Check, Copy, LogIn, LogOut, Send, Wallet } from "lucide-react";
import { isAddress } from "viem";
import { toBaseUnits } from "@/lib/api";
import { BASE_CHAIN_ID, BASE_EXPLORER, SEND_CAP_UNITS, encodeUsdcTransfer, formatUnits, readUsdcBalance } from "@/lib/base";

const APP_ID = process.env.NEXT_PUBLIC_PRIVY_APP_ID ?? "";

/**
 * The beginner-friendly front door: sign in with an email and Privy creates a wallet on Base.
 * No extension, no recovery phrase to manage. Privy holds the wallet's key material, not Beeline.
 * Every send is confirmed by the signed-in user, in this browser.
 */
export default function PrivyWalletPanel() {
  return (
    <PrivyProvider
      appId={APP_ID}
      config={{
        loginMethods: ["email"],
        embeddedWallets: { ethereum: { createOnLogin: "users-without-wallets" } },
        appearance: { theme: "light", accentColor: "#16162a" },
      }}
    >
      <Inner />
    </PrivyProvider>
  );
}

function Inner() {
  const { ready, authenticated, login, user } = usePrivy();
  const { logout } = useLogout();
  const { wallets } = useWallets();
  const wallet = wallets.find((w) => w.walletClientType === "privy") ?? wallets[0];
  const address = wallet?.address;
  const [balance, setBalance] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [stalled, setStalled] = useState(false);

  // If Privy never becomes ready (wrong App ID, or this site's address isn't allowed in the Privy dashboard), say so.
  useEffect(() => {
    if (ready) return;
    const t = setTimeout(() => setStalled(true), 12000);
    return () => clearTimeout(t);
  }, [ready]);

  const refresh = useCallback(async () => {
    if (!address) return;
    try {
      setBalance(await readUsdcBalance(address));
    } catch {
      setBalance(null);
    }
  }, [address]);

  useEffect(() => {
    void refresh();
    const t = setInterval(() => void refresh(), 20000);
    return () => clearInterval(t);
  }, [refresh]);

  if (!ready) {
    return (
      <div className="panel wallet-card">
        <h2>
          <Wallet size={18} aria-hidden style={{ verticalAlign: "-3px" }} /> Your Base wallet
        </h2>
        {stalled ? (
          <div className="alert" role="alert">
            Sign-in could not start. The Privy App ID may be wrong, or this site&rsquo;s address ({typeof window !== "undefined" ? window.location.origin : ""}) is not in
            the app&rsquo;s allowed origins in the Privy dashboard.
          </div>
        ) : (
          <p className="wallet-note">Loading wallet…</p>
        )}
      </div>
    );
  }

  if (!authenticated) {
    return (
      <div className="panel wallet-card">
        <h2>
          <Wallet size={18} aria-hidden style={{ verticalAlign: "-3px" }} /> Your Base wallet
        </h2>
        <p className="wallet-note">Sign in with your email. A wallet on Base is created for you: no extension to install and no recovery phrase to keep safe.</p>
        <button className="btn btn-honey" onClick={() => login()}>
          <LogIn size={17} aria-hidden /> Sign in with email
        </button>
      </div>
    );
  }

  const email = user?.email?.address;
  return (
    <div className="panel wallet-card">
      <h2>
        <Wallet size={18} aria-hidden style={{ verticalAlign: "-3px" }} /> Your Base wallet
      </h2>
      {email && <span className="wallet-note">Signed in as {email}</span>}
      {address ? (
        <>
          <div className="linkbox">
            <code>{address}</code>
            <button
              className="icon-btn"
              aria-label="Copy wallet address"
              onClick={() => {
                void navigator.clipboard?.writeText(address);
                setCopied(true);
                setTimeout(() => setCopied(false), 1500);
              }}
            >
              {copied ? <Check size={16} /> : <Copy size={16} />}
            </button>
          </div>
          <div className="wallet-bal">
            <b>{balance === null ? "…" : formatUnits(balance)}</b> <small>USDC on Base</small>
            <a href={`${BASE_EXPLORER}/address/${address}`} target="_blank" rel="noreferrer">
              BaseScan <ArrowUpRight size={12} aria-hidden />
            </a>
          </div>
          <p className="wallet-note">
            Send only <b>USDC on the Base network</b> to this address. Funding it with real naira: run{" "}
            <code>.\scripts\switch-onramp.ps1 -BaseAddress {address} -HolderName &quot;Your Name&quot; -QuoteOnly</code> on your PC.
          </p>
          <SendBox address={address} balance={balance} onSent={refresh} />
        </>
      ) : (
        <p className="wallet-note">Creating your wallet…</p>
      )}
      <button className="btn ghost" style={{ justifySelf: "start" }} onClick={() => void logout()}>
        <LogOut size={15} aria-hidden /> Sign out
      </button>
    </div>
  );
}

function SendBox({ address, balance, onSent }: { address: string; balance: string | null; onSent: () => Promise<void> }) {
  const { sendTransaction } = useSendTransaction();
  const [to, setTo] = useState("");
  const [amount, setAmount] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [hash, setHash] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Strict check: a mixed-case address must have a valid EIP-55 checksum, which catches most typos.
  const toOk = isAddress(to, { strict: true });
  const units = amount ? toBaseUnits(amount) : null;
  const overCap = units !== null && BigInt(units) > SEND_CAP_UNITS;
  const overBalance = units !== null && balance !== null && BigInt(units) > BigInt(balance);
  const tail = toOk ? to.slice(-6).toLowerCase() : "";
  const confirmed = toOk && confirm.trim().toLowerCase() === tail;
  const ready = toOk && units !== null && !overCap && !overBalance && confirmed && !busy;

  async function send() {
    if (!ready || units === null) return;
    setBusy(true);
    setError(null);
    setHash(null);
    try {
      const res = await sendTransaction(
        { to: "0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913", data: encodeUsdcTransfer(to, units), chainId: BASE_CHAIN_ID },
        { address, sponsor: true },
      );
      setHash(res.hash);
      setAmount("");
      setConfirm("");
      setTimeout(() => void onSent(), 6000);
    } catch (e) {
      setError(e instanceof Error ? e.message : "The transfer failed.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="send-box">
      <h3>
        <Send size={15} aria-hidden /> Send USDC from this wallet
      </h3>
      <label className="field">
        To (an address on Base)
        <input value={to} spellCheck={false} placeholder="0x…" onChange={(e) => setTo(e.target.value.trim())} />
        {to && !toOk && <span className="err">Not a valid EVM address (check every character; a wrong checksum is rejected)</span>}
      </label>
      <label className="field">
        Amount (USDC)
        <input value={amount} inputMode="decimal" placeholder="1.00" onChange={(e) => setAmount(e.target.value.trim())} />
        {amount && units === null && <span className="err">Use an amount like 1.5</span>}
        {overCap && <span className="err">Test cap: at most {formatUnits(SEND_CAP_UNITS.toString())} USDC per send from this page</span>}
        {overBalance && <span className="err">More than the wallet holds</span>}
      </label>
      {toOk && (
        <label className="field">
          Type the last 6 characters of the address ({tail}) to confirm it
          <input value={confirm} spellCheck={false} onChange={(e) => setConfirm(e.target.value)} />
        </label>
      )}
      {error && (
        <div className="alert" role="alert">
          {error}
        </div>
      )}
      {hash && (
        <a className="btn" href={`${BASE_EXPLORER}/tx/${hash}`} target="_blank" rel="noreferrer">
          <Check size={16} aria-hidden /> Sent: {hash.slice(0, 10)}… <ArrowUpRight size={14} aria-hidden />
        </a>
      )}
      <button className="btn btn-honey" onClick={() => void send()} disabled={!ready}>
        {busy ? "Sending…" : "Send"} <Send size={16} aria-hidden />
      </button>
      <span className="wallet-note">Network fee: requested as sponsored (no ETH needed). If sponsorship isn’t switched on for this app, the send fails and nothing moves.</span>
    </div>
  );
}
