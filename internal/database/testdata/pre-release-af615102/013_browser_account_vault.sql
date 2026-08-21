ALTER TABLE users ADD COLUMN disabled_at TEXT NOT NULL DEFAULT '';
ALTER TABLE sessions ADD COLUMN browser_vault_id TEXT NOT NULL DEFAULT '';

CREATE TABLE browser_account_vaults (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    device_id TEXT NOT NULL,
    active_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    automatic_sign_in INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX idx_browser_account_vaults_active
    ON browser_account_vaults(token_hash, revoked_at, expires_at);
CREATE INDEX idx_browser_account_vaults_device
    ON browser_account_vaults(device_id, revoked_at, expires_at);

CREATE TABLE browser_account_entries (
    vault_id TEXT NOT NULL REFERENCES browser_account_vaults(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    auth_provider TEXT NOT NULL CHECK (auth_provider IN ('local', 'portico')),
    profile_identity_id TEXT NOT NULL REFERENCES profile_identities(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    added_at TEXT NOT NULL,
    last_used_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (vault_id, user_id)
);

CREATE INDEX idx_browser_account_entries_user
    ON browser_account_entries(user_id, revoked_at, expires_at);
CREATE INDEX idx_browser_account_entries_device
    ON browser_account_entries(device_id, revoked_at, expires_at);
