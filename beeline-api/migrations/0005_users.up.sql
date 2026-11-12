-- Signed-in users (identified by their Privy DID), their profile, invoice ownership and chat history.
-- No passwords and no private keys are stored: Privy handles sign-in and the user's wallet.
CREATE TABLE users (
    id           TEXT PRIMARY KEY,                 -- Privy DID, e.g. did:privy:...
    shop_name    TEXT NOT NULL DEFAULT '',
    near_account TEXT NOT NULL DEFAULT '',         -- payout account the user chose (their Privy wallet by default)
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE invoices ADD COLUMN user_id TEXT REFERENCES users (id);
CREATE INDEX invoices_user_idx ON invoices (user_id, created_at DESC) WHERE user_id IS NOT NULL;

CREATE TABLE chat_messages (
    id         BIGSERIAL PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users (id),
    role       TEXT NOT NULL CHECK (role IN ('user', 'assistant')),
    body       TEXT NOT NULL,
    invoice_id TEXT REFERENCES invoices (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX chat_messages_user_idx ON chat_messages (user_id, id DESC);
