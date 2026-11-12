import type { Metadata } from "next";
import { ChatProviders } from "./ChatProviders";

export const metadata: Metadata = {
  title: "Beeline: your payment assistant",
  description: "Sign in with your email, ask for an invoice in a chat, and share the pay link anywhere. A NEAR vault blocks anything that does not match the signed invoice.",
};

export default function ChatLayout({ children }: { children: React.ReactNode }) {
  return <ChatProviders>{children}</ChatProviders>;
}
