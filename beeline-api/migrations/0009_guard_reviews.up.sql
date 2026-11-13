-- Guard: an AI agent reviews every payment event on an invoice (a refused claim, a confirmed payment, a refund) and
-- writes a verdict and a plain reason. It never decides whether an invoice is paid: that stays with the chain verifier
-- and NEAR Intents. The verdict is advice for the vendor, kept as evidence.
CREATE TABLE guard_reviews (
    id          BIGSERIAL PRIMARY KEY,
    invoice_id  TEXT NOT NULL REFERENCES invoices (id),
    subject     TEXT NOT NULL CHECK (subject IN ('claim_refused', 'payment_confirmed', 'payin_event')),
    ref         TEXT NOT NULL DEFAULT '',         -- transaction hash or deposit address the review is about
    verdict     TEXT NOT NULL CHECK (verdict IN ('clear', 'watch', 'fraud')),
    reason      TEXT NOT NULL,
    model       TEXT NOT NULL,
    evidence    JSONB NOT NULL DEFAULT '{}',      -- the facts the agent was shown (computed by the server, not the customer)
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (invoice_id, subject, ref)
);
CREATE INDEX guard_reviews_invoice_idx ON guard_reviews (invoice_id, id DESC);
