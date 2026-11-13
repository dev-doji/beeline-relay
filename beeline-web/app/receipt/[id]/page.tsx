import type { Metadata } from "next";
import { ReceiptPage } from "@/components/pay/ReceiptPage";

export const metadata: Metadata = { title: "Receipt | Beeline", robots: { index: false } };

export default function Page({ params }: { params: Promise<{ id: string }> }) {
  return (
    <div className="ck-page">
      <ReceiptPage params={params} />
    </div>
  );
}
