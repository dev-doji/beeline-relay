import type { Metadata } from "next";

const API = (process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080").replace(/\/$/, "");

/**
 * Link previews: when a pay link is pasted into WhatsApp, Facebook, X, Telegram, LinkedIn or a DM,
 * the platform fetches this page and shows the title below. Read from the API with a short timeout,
 * and fall back to a generic title if the API is asleep or the invoice is unknown.
 */
export async function generateMetadata({ params }: { params: Promise<{ id: string }> }): Promise<Metadata> {
  const { id } = await params;
  const generic: Metadata = {
    title: "Pay a signed invoice | Beeline",
    description: "A signed invoice, paid in USDC on NEAR. The vault pays only the exact vendor and amount.",
  };
  if (!/^inv_[a-z0-9]{10,40}$/i.test(id)) return generic;
  try {
    const res = await fetch(`${API}/api/v1/playground/invoices/${id}`, { signal: AbortSignal.timeout(3500), cache: "no-store" });
    if (!res.ok) return generic;
    const inv = (await res.json()) as { vendor_name?: string; amount?: string; memo?: string };
    if (!inv.vendor_name || !inv.amount || !/^\d+$/.test(inv.amount)) return generic;
    const padded = inv.amount.padStart(7, "0");
    const usdc = `${padded.slice(0, -6)}.${padded.slice(-6).replace(/0+$/, "").padEnd(2, "0")}`;
    const title = `Pay ${inv.vendor_name}: ${usdc} USDC`;
    return {
      title: `${title} | Beeline`,
      description: inv.memo ? `${inv.memo}. Signed invoice, paid on NEAR.` : "Signed invoice, paid on NEAR.",
      openGraph: { title, description: "Signed invoice, paid on NEAR.", type: "website" },
      twitter: { card: "summary", title },
    };
  } catch {
    return generic;
  }
}

export default function PayLayout({ children }: { children: React.ReactNode }) {
  return children;
}
