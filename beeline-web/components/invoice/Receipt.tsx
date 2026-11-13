import { ShieldCheck } from "lucide-react";
import { formatUSDC, shortKey, type InvoiceItem } from "@/lib/api";

export interface ReceiptData {
  id?: string;
  vendor_name: string;
  vendor_handle: string;
  memo: string;
  items: InvoiceItem[];
  amount: string;
  payout: string;
  status?: "open" | "paid" | "draft";
  expires_at?: string;
  signed?: boolean;
  network?: "mainnet" | "testnet";
}

function initials(name: string) {
  return (
    name
      .split(/\s+/)
      .filter(Boolean)
      .slice(0, 2)
      .map((w) => w[0]?.toUpperCase())
      .join("") || "?"
  );
}

/** Multiplies base units by a quantity without floating point. */
export function lineTotal(it: InvoiceItem): string {
  if (!/^\d+$/.test(it.unit_price)) return "0";
  return (BigInt(it.unit_price) * BigInt(Math.max(0, Math.floor(it.quantity || 0)))).toString();
}

export function Receipt({ data }: { data: ReceiptData }) {
  const status = data.status ?? "draft";
  return (
    <div className="receipt" aria-label="Invoice">
      {status === "paid" && <span className="rstamp paid">PAID</span>}
      {status === "open" && <span className="rstamp open">UNPAID</span>}
      <div className="vendor">
        <span className="initials" aria-hidden>
          {initials(data.vendor_name)}
        </span>
        <div>
          <b>{data.vendor_name || "Your shop"}</b>
          <span>{data.vendor_handle || "@yourshop"}</span>
        </div>
      </div>
      <div className="meta">
        <span>{data.id ? `Invoice ${shortKey(data.id, 7)}` : "Invoice preview"}</span>
        <span>
          {data.expires_at ? `Due ${new Date(data.expires_at).toLocaleDateString(undefined, { day: "numeric", month: "short" })}` : data.memo}
        </span>
      </div>
      {data.memo && data.expires_at && <div style={{ fontSize: 14, marginBottom: 6 }}>{data.memo}</div>}
      <table>
        <tbody>
          {data.items.map((it, i) => (
            <tr key={i}>
              <td>
                {it.quantity} × {it.name || "Item"}
              </td>
              <td className="num">{formatUSDC(lineTotal(it))} USDC</td>
            </tr>
          ))}
        </tbody>
      </table>
      <div className="total">
        <span>Total</span>
        <b>
          {formatUSDC(data.amount)}
          <small>USDC</small>
        </b>
      </div>
      <div className="payto">
        <span>Pays out to (NEAR {data.network === "mainnet" ? "mainnet" : "testnet"})</span>
        <code>{data.payout ? shortKey(data.payout, 12) : "—"}</code>
      </div>
      {data.signed && (
        <div className="sigline">
          <ShieldCheck size={15} aria-hidden /> Signed invoice: any edit to the payee or amount voids it
        </div>
      )}
    </div>
  );
}
