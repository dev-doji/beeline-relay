-- Quotes record which configured route they belong to, the refund address sent to the
-- provider, and the exact provider request, so the binding can be re-checked later.
ALTER TABLE quotes
    ADD COLUMN route_id              TEXT NOT NULL,
    ADD COLUMN refund_address        TEXT NOT NULL,
    ADD COLUMN time_estimate_seconds INTEGER,
    ADD COLUMN provider_request      JSONB NOT NULL;

-- Sandbox workspaces hold simulated payments only (test double, never the live provider).
ALTER TABLE workspaces
    ADD COLUMN is_sandbox BOOLEAN NOT NULL DEFAULT false;
