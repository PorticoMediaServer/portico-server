package postgres

// CurrentClaimServerDeclaration is the proposed one-current-format replacement
// for the servers declaration. Root must compose this into fresh initialization
// and bump the current schema marker atomically with all required consumers.
// It is deliberately not executed or appended as an ALTER migration here.
const CurrentClaimServerDeclaration = `CREATE TABLE servers(
 id text PRIMARY KEY CHECK(id ~ '^srv_[A-Za-z0-9_-]{43}$'),
 owner_account_id text REFERENCES accounts(id),
 name text NOT NULL,
 base_url text,
 public_key text NOT NULL CHECK(public_key ~ '^[A-Za-z0-9_-]{43}$'),
 token_hash bytea UNIQUE CHECK(token_hash IS NULL OR octet_length(token_hash)=32),
 token_ciphertext bytea CHECK(token_ciphertext IS NULL OR octet_length(token_ciphertext) BETWEEN 28 AND 4096),
 claim_generation bigint NOT NULL CHECK(claim_generation>0),
 credential_generation bigint NOT NULL CHECK(credential_generation>0),
 revoked_at timestamptz,
 policy_revision bigint NOT NULL DEFAULT 1 CHECK(policy_revision>0),
 local_revision bigint NOT NULL DEFAULT 0 CHECK(local_revision>=0),
 policy_issued_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 last_heartbeat_at timestamptz,
 last_seen_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 retired_reason text NOT NULL DEFAULT '', replaced_by text,
 presence_expires_at timestamptz,
 endpoint_verified boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
 CHECK((revoked_at IS NULL AND owner_account_id IS NOT NULL AND token_hash IS NOT NULL AND token_ciphertext IS NOT NULL) OR
       (revoked_at IS NOT NULL AND owner_account_id IS NULL AND token_hash IS NULL AND token_ciphertext IS NULL AND base_url IS NULL AND NOT endpoint_verified)),
 CHECK(NOT endpoint_verified OR (base_url IS NOT NULL AND base_url LIKE 'https://%')),
 UNIQUE(id,claim_generation,credential_generation)
);`
