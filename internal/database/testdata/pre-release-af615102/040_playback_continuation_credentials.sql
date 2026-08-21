-- Native background/PiP playback continuity is a separate credential family.
-- Only a digest is persisted; the bearer is returned once and must use the
-- PorticoPlayback Authorization scheme. The previous digest is retained for a
-- short rotation overlap so a lost response can be retried safely.
CREATE TABLE IF NOT EXISTS playback_session_continuation_credentials (
    playback_session_id TEXT PRIMARY KEY REFERENCES playback_sessions(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    previous_token_hash TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    client_instance_id TEXT NOT NULL DEFAULT '',
    generation INTEGER NOT NULL DEFAULT 1,
    origin TEXT NOT NULL,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    absolute_expires_at TEXT NOT NULL,
    previous_valid_until TEXT NOT NULL DEFAULT '',
    last_used_at TEXT NOT NULL DEFAULT '',
    last_rotation_request_id TEXT NOT NULL DEFAULT '',
    last_rotation_fingerprint TEXT NOT NULL DEFAULT '',
    revoked_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_playback_continuation_expiry
    ON playback_session_continuation_credentials(expires_at, revoked_at);
CREATE INDEX IF NOT EXISTS idx_playback_continuation_scope
    ON playback_session_continuation_credentials(user_id, profile_id, generation, revoked_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_playback_continuation_previous_token
    ON playback_session_continuation_credentials(previous_token_hash)
    WHERE previous_token_hash <> '';
ALTER TABLE playback_sessions ADD COLUMN last_renegotiation_fingerprint TEXT NOT NULL DEFAULT '';
