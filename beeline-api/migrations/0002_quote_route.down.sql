ALTER TABLE workspaces DROP COLUMN IF EXISTS is_sandbox;
ALTER TABLE quotes
    DROP COLUMN IF EXISTS provider_request,
    DROP COLUMN IF EXISTS time_estimate_seconds,
    DROP COLUMN IF EXISTS refund_address,
    DROP COLUMN IF EXISTS route_id;
