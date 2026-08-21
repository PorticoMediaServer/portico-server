ALTER TABLE devices ADD COLUMN installation_id TEXT NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_user_installation
    ON devices(user_id, installation_id) WHERE installation_id <> '';

ALTER TABLE quick_connect_requests ADD COLUMN installation_id TEXT NOT NULL DEFAULT '';
ALTER TABLE tv_setup_sessions ADD COLUMN installation_id TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS native_refresh_tokens (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    auth_provider TEXT NOT NULL DEFAULT 'local',
    token_hash TEXT NOT NULL UNIQUE,
    replaced_by_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT '',
    revoked_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_native_refresh_tokens_family
    ON native_refresh_tokens(family_id, created_at);
CREATE INDEX IF NOT EXISTS idx_native_refresh_tokens_user
    ON native_refresh_tokens(user_id, revoked_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_native_refresh_tokens_device
    ON native_refresh_tokens(device_id, revoked_at, expires_at);
CREATE INDEX IF NOT EXISTS idx_native_refresh_tokens_replacement
    ON native_refresh_tokens(replaced_by_id);
