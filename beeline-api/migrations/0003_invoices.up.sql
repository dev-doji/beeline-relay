-- Vendor invoices for the public playground ("pay links"). Testnet demo data only.
CREATE TABLE invoices (
    id           TEXT PRIMARY KEY,
    vendor_name  TEXT NOT NULL,
    vendor_handle TEXT NOT NULL DEFAULT '',
    payout       TEXT NOT NULL,
    asset        TEXT NOT NULL,
    amount       TEXT NOT NULL CHECK (amount ~ '^[1-9][0-9]*$'),
    memo         TEXT NOT NULL DEFAULT '',
    items        JSONB NOT NULL DEFAULT '[]',
    payload      TEXT NOT NULL,      -- the exact JSON the issuer signed
    signature    TEXT NOT NULL,      -- hex ed25519 over payload
    status       TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'paid')),
    paid_tx      TEXT,
    paid_at      TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at   TIMESTAMPTZ NOT NULL
);
CREATE INDEX invoices_created_at_idx ON invoices (created_at DESC);
