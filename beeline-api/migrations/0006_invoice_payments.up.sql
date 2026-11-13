-- Payments verified on-chain, and the security events around an invoice.
-- A row in invoice_payments exists only after the server read the transaction from NEAR and it matched the invoice.
CREATE TABLE invoice_payments (
    tx_hash      TEXT PRIMARY KEY,                                -- one transaction can pay at most one invoice, once
    invoice_id   TEXT NOT NULL UNIQUE REFERENCES invoices (id),   -- one verified payment per invoice
    sender       TEXT NOT NULL,
    receiver     TEXT NOT NULL,
    token        TEXT NOT NULL,
    amount       TEXT NOT NULL CHECK (amount ~ '^[1-9][0-9]*$'),
    memo         TEXT NOT NULL,
    block_hash   TEXT NOT NULL,
    block_time   TIMESTAMPTZ NOT NULL,
    verification TEXT NOT NULL DEFAULT 'chain-verified',
    verified_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Things that did not match: a claim with the wrong token, amount, payee, a reused transaction, and so on.
CREATE TABLE invoice_events (
    id         BIGSERIAL PRIMARY KEY,
    invoice_id TEXT NOT NULL REFERENCES invoices (id),
    kind       TEXT NOT NULL,
    reason     TEXT NOT NULL,
    tx_hash    TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX invoice_events_invoice_idx ON invoice_events (invoice_id, id DESC);
