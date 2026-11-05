import type { Metadata } from "next";
import { Monitor } from "@/components/monitor/Monitor";

export const metadata: Metadata = {
  title: "Monitor · Beeline",
  description: "Watch an AI agent and a NEAR vault sort incoming payment requests into paid and blocked, live on NEAR testnet.",
};

export default function MonitorPage() {
  return <Monitor />;
}
