package postgres

// Fresh-format normalized rows; never persist a plaintext code, verifier or
// Hosted credential. The captured access hash uses the existing family fence.
const serverReturnSchema = `
CREATE TABLE server_returns(
 id text PRIMARY KEY,
 account_id text NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
 family_id text NOT NULL REFERENCES session_families(id) ON DELETE CASCADE,
 access_hash bytea NOT NULL CHECK(octet_length(access_hash)=32),
 account_epoch bigint NOT NULL, family_generation bigint NOT NULL,
 session_mode text NOT NULL CHECK(session_mode IN('browser','native')),
 server_id text NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
 operation_id text NOT NULL REFERENCES claim_operations(id) ON DELETE CASCADE,
 claim_generation bigint NOT NULL,credential_generation bigint NOT NULL,
 route_generation bigint NOT NULL,registration_id text NOT NULL,origin text NOT NULL,
 purpose text NOT NULL CHECK(purpose IN('setup','switch','change-owner')),
 current_owner_account_id text NOT NULL,previous_owner_role text NOT NULL,
 state text NOT NULL,code_challenge text NOT NULL,server_challenge text NOT NULL,installation_id text NOT NULL,
 code_hash bytea UNIQUE CHECK(code_hash IS NULL OR octet_length(code_hash)=32),
 code_expires_at timestamptz,consumed_at timestamptz,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 expires_at timestamptz NOT NULL DEFAULT clock_timestamp()+interval '5 minutes',
 CHECK((code_hash IS NULL)=(code_expires_at IS NULL)),
 CHECK(code_expires_at IS NULL OR code_expires_at<=expires_at)
);
CREATE INDEX server_return_account ON server_returns(account_id,created_at);
CREATE INDEX server_return_expiry ON server_returns(expires_at);
`
