import { PayContent } from "@/components/pay/PayContent";

// The customer's checkout is a single flat card with no app navigation (see components/pay/PayContent.tsx).
export default function PayPage({ params }: { params: Promise<{ id: string }> }) {
  return (
    <div className="ck-page">
      <PayContent params={params} />
    </div>
  );
}
