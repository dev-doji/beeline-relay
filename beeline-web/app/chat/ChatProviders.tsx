"use client";

import { PrivyProvider } from "@privy-io/react-auth";

const APP_ID = process.env.NEXT_PUBLIC_PRIVY_APP_ID ?? "";

/**
 * Privy handles email sign-in (a one-time code) and the user's wallet. Beeline never sees a
 * password or a private key. Without an app ID the page says sign-in is not switched on.
 */
export function ChatProviders({ children }: { children: React.ReactNode }) {
  if (!APP_ID) {
    return (
      <div className="chat-center">
        <div className="panel" style={{ maxWidth: 460 }}>
          <h2>Sign-in is not switched on</h2>
          <p className="help">Set NEXT_PUBLIC_PRIVY_APP_ID for this site (see docs/DEPLOY.md). Nothing else is affected: payment links at /vendor still work without an account.</p>
        </div>
      </div>
    );
  }
  return (
    <PrivyProvider
      appId={APP_ID}
      config={{
        loginMethods: ["email"],
        appearance: { theme: "light", accentColor: "#6c4fd8" },
      }}
    >
      {children}
    </PrivyProvider>
  );
}
