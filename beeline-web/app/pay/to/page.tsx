import type { Metadata } from "next";
import { OpenPay } from "./OpenPay";

type SP = Promise<{ name?: string; to?: string; note?: string }>;

export async function generateMetadata({ searchParams }: { searchParams: SP }): Promise<Metadata> {
  const { name } = await searchParams;
  const shop = (name ?? "").trim().slice(0, 60);
  const title = shop ? `Pay ${shop} | Beeline` : "Pay with Beeline";
  return {
    title,
    description: "Choose an amount. Beeline signs a one-time invoice for you and a NEAR vault pays only the exact payee.",
    openGraph: { title: shop ? `Pay ${shop}` : "Pay with Beeline", type: "website" },
    twitter: { card: "summary", title: shop ? `Pay ${shop}` : "Pay with Beeline" },
  };
}

export default async function Page({ searchParams }: { searchParams: SP }) {
  const { name, to, note } = await searchParams;
  return <OpenPay name={(name ?? "").slice(0, 60)} to={(to ?? "").slice(0, 64)} note={(note ?? "").slice(0, 200)} />;
}
