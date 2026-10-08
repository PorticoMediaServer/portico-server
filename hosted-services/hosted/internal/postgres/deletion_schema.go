package postgres

const deletionSchema = `
CREATE TABLE IF NOT EXISTS account_deletions(id text PRIMARY KEY,token_hash bytea UNIQUE NOT NULL,account_id text NOT NULL,password_hash bytea,snapshot_hash bytea,expires_at timestamptz NOT NULL,accepted_at timestamptz,completed_at timestamptz,disposition text,receipt_expires_at timestamptz);
CREATE INDEX IF NOT EXISTS account_deletions_account ON account_deletions(account_id);
CREATE INDEX IF NOT EXISTS account_deletions_expiry ON account_deletions(expires_at) WHERE accepted_at IS NULL;
CREATE INDEX IF NOT EXISTS account_deletions_receipt_expiry ON account_deletions(receipt_expires_at) WHERE accepted_at IS NOT NULL;
CREATE TABLE IF NOT EXISTS account_erasure_jobs(id text PRIMARY KEY,deletion_id text NOT NULL REFERENCES account_deletions(id),server_id text NOT NULL REFERENCES servers(id),account_id text NOT NULL,profile_ids text[] NOT NULL,revision bigint NOT NULL DEFAULT 1,disposition text NOT NULL,created_at timestamptz NOT NULL DEFAULT now(),acknowledged_at timestamptz,UNIQUE(deletion_id,server_id));
CREATE INDEX IF NOT EXISTS account_erasure_pending ON account_erasure_jobs(server_id,id) WHERE acknowledged_at IS NULL;
CREATE INDEX IF NOT EXISTS account_erasure_deletion ON account_erasure_jobs(deletion_id);
`
