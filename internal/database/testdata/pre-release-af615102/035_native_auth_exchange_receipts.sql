-- A bounded, proof-bound receipt lets clients recover the exact native
-- credential family when a one-time profile/Hosted exchange committed but its
-- success response was lost. The proof hash is derived from the server-issued
-- one-time secret plus immutable request context; installationId is excluded
-- because it is optional, untrusted continuity metadata.
CREATE TABLE native_auth_exchange_receipts (
    kind TEXT NOT NULL,
    proof_hash TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    native_refresh_token_id TEXT NOT NULL REFERENCES native_refresh_tokens(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY (kind, proof_hash)
);

CREATE UNIQUE INDEX idx_native_auth_exchange_receipts_refresh
    ON native_auth_exchange_receipts(native_refresh_token_id);

CREATE INDEX idx_native_auth_exchange_receipts_expiry
    ON native_auth_exchange_receipts(expires_at);
