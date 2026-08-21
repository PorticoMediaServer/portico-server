CREATE TABLE IF NOT EXISTS playback_media_grants (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    playback_session_id TEXT NOT NULL REFERENCES playback_sessions(id) ON DELETE CASCADE,
    principal_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    resource_kind TEXT NOT NULL CHECK (resource_kind IN ('media', 'live_channel')),
    resource_id TEXT NOT NULL,
    operation_classes_json TEXT NOT NULL,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    last_authorized_at TEXT NOT NULL DEFAULT '',
    revoked_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_playback_media_grants_session
    ON playback_media_grants(playback_session_id, revoked_at, expires_at);

CREATE INDEX IF NOT EXISTS idx_playback_media_grants_principal
    ON playback_media_grants(principal_user_id, revoked_at, expires_at);
