import { PayShell } from "@/components/pay/PayShell";
import { PayContent } from "@/components/pay/PayContent";

export default function PayPage({ params }: { params: Promise<{ id: string }> }) {
  return (
    <PayShell>
      <PayContent params={params} />
    </PayShell>
  );
}
