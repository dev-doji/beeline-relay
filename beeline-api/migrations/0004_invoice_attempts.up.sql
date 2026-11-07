-- Every attempt to pay a playground pay link: honest payments and visitors' attacks, with the
-- vault's on-chain decision. Public testnet demo data: messages are shown in the attack log.
CREATE TABLE invoice_attempts (
    id             TEXT PRIMARY KEY,
    invoice_id     TEXT NOT NULL REFERENCES invoices (id) ON DELETE CASCADE,
    kind           TEXT NOT NULL CHECK (kind IN ('pay', 'attack')),
    message        TEXT NOT NULL DEFAULT '',
    edited_payout  TEXT NOT NULL DEFAULT '',
    edited_amount  TEXT NOT NULL DEFAULT '',
    outcome        TEXT NOT NULL CHECK (outcome IN ('paid', 'blocked', 'reply', 'error')),
    layer          TEXT NOT NULL DEFAULT '',
    reason         TEXT NOT NULL DEFAULT '',
    recipient      TEXT NOT NULL DEFAULT '',  -- whom the agent tried to pay
    amount         TEXT NOT NULL DEFAULT '',  -- base units the agent tried to pay
    tx_hash        TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX invoice_attempts_invoice_idx ON invoice_attempts (invoice_id, created_at DESC);
CREATE INDEX invoice_attempts_created_idx ON invoice_attempts (created_at DESC);
