-- "Pay from anywhere": a customer sends crypto from any chain or exchange to a one-time NEAR Intents deposit
-- address, and NEAR Intents delivers the invoice's USDC to the vendor on NEAR. One row per quote.
CREATE TABLE payin_quotes (
    id              BIGSERIAL PRIMARY KEY,
    invoice_id      TEXT NOT NULL REFERENCES invoices (id),
    origin_asset    TEXT NOT NULL,            -- NEAR Intents asset id of what the customer pays with
    origin_chain    TEXT NOT NULL,
    origin_symbol   TEXT NOT NULL,
    origin_decimals INT  NOT NULL,
    amount_in       TEXT NOT NULL CHECK (amount_in ~ '^[1-9][0-9]*$'),   -- base units the customer must send
    deposit_address TEXT NOT NULL UNIQUE,     -- one-time address from the signed quote
    deposit_memo    TEXT NOT NULL DEFAULT '',
    refund_to       TEXT NOT NULL,            -- the customer's own address on the origin chain
    deadline        TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'waiting'
                    CHECK (status IN ('waiting', 'processing', 'confirming', 'paid', 'incomplete', 'refunded', 'failed', 'expired')),
    provider_status TEXT NOT NULL DEFAULT '', -- NEAR Intents' own word, kept separately from ours
    delivered_tx    TEXT NOT NULL DEFAULT '', -- destination transaction hash reported by NEAR Intents
    balance_before  TEXT NOT NULL,            -- the vendor's USDC balance on NEAR when the quote was made
    balance_after   TEXT NOT NULL DEFAULT '',
    quote_json      JSONB NOT NULL DEFAULT '{}', -- the provider's signed quote response, kept as evidence
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX payin_quotes_invoice_idx ON payin_quotes (invoice_id, id DESC);
