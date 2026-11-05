import type { Metadata } from "next";
import { Chat } from "@/components/chat/Chat";

export const metadata: Metadata = {
  title: "Playground · Beeline",
  description: "Chat with Bee, an AI payment agent, and try to trick it. A NEAR testnet vault decides on-chain.",
};

export default function PlaygroundPage() {
  return <Chat />;
}
