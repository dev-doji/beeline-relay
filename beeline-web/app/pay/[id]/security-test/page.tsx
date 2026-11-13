import { PayShell } from "@/components/pay/PayShell";
import { SecurityTest } from "@/components/pay/SecurityTest";

export default function SecurityTestPage({ params }: { params: Promise<{ id: string }> }) {
  return (
    <PayShell>
      <SecurityTest params={params} />
    </PayShell>
  );
}
