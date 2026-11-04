-- Core schema for Beeline Relay. See docs/spec/TRD.md section 4 and docs/POSITIONING.md.

CREATE TABLE workspaces (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Agent and console credentials. Only a SHA-256 hash of the key is stored.
CREATE TABLE api_keys (
    id          TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id),
    kind        TEXT NOT NULL CHECK (kind IN ('agent', 'console')),
    label       TEXT NOT NULL,
    key_hash    BYTEA NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at  TIMESTAMPTZ
);

-- Immutable snapshot of a provider quote, including 1Click's signature for dispute resolution.
CREATE TABLE quotes (
    id                    TEXT PRIMARY KEY,
    workspace_id          TEXT NOT NULL REFERENCES workspaces(id),
    fingerprint           TEXT NOT NULL,
    source_network        TEXT NOT NULL,
    source_asset_id       TEXT NOT NULL,
    source_amount         NUMERIC(78, 0) NOT NULL CHECK (source_amount > 0),
    destination_network   TEXT NOT NULL,
    destination_asset_id  TEXT NOT NULL,
    destination_address   TEXT NOT NULL,
    expected_amount_out   NUMERIC(78, 0),
    min_amount_out        NUMERIC(78, 0),
    deposit_address       TEXT,
    deposit_memo          TEXT,
    expires_at            TIMESTAMPTZ NOT NULL,
    provider              TEXT NOT NULL,
    provider_correlation_id TEXT,
    provider_signature    TEXT,
    provider_response     JSONB NOT NULL,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX quotes_workspace_idx ON quotes (workspace_id, created_at DESC);

CREATE TABLE payment_sessions (
    id                  TEXT PRIMARY KEY,
    workspace_id        TEXT NOT NULL REFERENCES workspaces(id),
    quote_id            TEXT NOT NULL REFERENCES quotes(id),
    agent_key_id        TEXT REFERENCES api_keys(id),
    client_reference    TEXT,
    state               TEXT NOT NULL CHECK (state IN (
                            'created', 'quoted', 'awaiting_approval', 'awaiting_authorization',
                            'submitted', 'processing', 'completed', 'failed', 'refunded',
                            'expired', 'recovery_required')),
    fingerprint         TEXT NOT NULL,
    source_tx_hash      TEXT,
    provider_status     TEXT,
    provider_updated_at TIMESTAMPTZ,
    last_reconciled_at  TIMESTAMPTZ,
    is_simulated        BOOLEAN NOT NULL DEFAULT false,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (quote_id)
);
CREATE INDEX payment_sessions_workspace_idx ON payment_sessions (workspace_id, created_at DESC);

CREATE TABLE idempotency_records (
    workspace_id        TEXT NOT NULL REFERENCES workspaces(id),
    idempotency_key     TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    payment_id          TEXT NOT NULL REFERENCES payment_sessions(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at          TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (workspace_id, idempotency_key)
);

-- Append-only at the application level: corrections are new events.
CREATE TABLE payment_events (
    id              TEXT PRIMARY KEY,
    payment_id      TEXT NOT NULL REFERENCES payment_sessions(id),
    sequence        INTEGER NOT NULL,
    type            TEXT NOT NULL,
    source          TEXT NOT NULL CHECK (source IN (
                        'application', 'wallet', 'provider', 'chain_observation',
                        'reconciler', 'policy', 'operator', 'test_harness')),
    state_before    TEXT,
    state_after     TEXT,
    observed_at     TIMESTAMPTZ NOT NULL,
    recorded_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    dedupe_key      TEXT,
    correlation_id  TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (payment_id, sequence),
    UNIQUE (payment_id, dedupe_key)
);

-- Spend policy per workspace (one active row). Amounts are base units of the route's asset.
CREATE TABLE policies (
    workspace_id            TEXT PRIMARY KEY REFERENCES workspaces(id),
    max_amount_per_payment  NUMERIC(78, 0) NOT NULL,
    daily_budget_per_agent  NUMERIC(78, 0) NOT NULL,
    approval_threshold      NUMERIC(78, 0) NOT NULL,
    allowed_recipients      TEXT[] NOT NULL DEFAULT '{}',
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT now()
);
