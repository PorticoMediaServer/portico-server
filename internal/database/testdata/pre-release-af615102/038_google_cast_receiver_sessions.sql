CREATE TABLE IF NOT EXISTS cast_bootstraps (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    receiver_id TEXT NOT NULL,
    receiver_origin TEXT NOT NULL,
    receiver_public_key TEXT NOT NULL,
    receiver_challenge TEXT NOT NULL,
    server_origin TEXT NOT NULL,
    playback_session_id TEXT NOT NULL REFERENCES playback_sessions(id) ON DELETE CASCADE,
    client_instance_id TEXT NOT NULL,
    generation INTEGER NOT NULL DEFAULT 1,
    capabilities_json TEXT NOT NULL DEFAULT '[]',
    expires_at TEXT NOT NULL,
    redeemed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_cast_bootstraps_expiry ON cast_bootstraps(expires_at, redeemed_at);

CREATE TABLE IF NOT EXISTS cast_receiver_sessions (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    receiver_id TEXT NOT NULL,
    receiver_origin TEXT NOT NULL,
    server_origin TEXT NOT NULL,
    playback_session_id TEXT NOT NULL REFERENCES playback_sessions(id) ON DELETE CASCADE,
    client_instance_id TEXT NOT NULL,
    generation INTEGER NOT NULL DEFAULT 1,
    capabilities_json TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'stopped', 'expired', 'revoked')),
    expires_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    stopped_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_cast_receiver_sessions_scope
    ON cast_receiver_sessions(user_id, profile_id, client_instance_id, status, generation);
CREATE INDEX IF NOT EXISTS idx_cast_receiver_sessions_expiry
    ON cast_receiver_sessions(expires_at, status);

ALTER TABLE playback_sessions ADD COLUMN progress_authority TEXT NOT NULL DEFAULT 'sender';
ALTER TABLE playback_sessions ADD COLUMN progress_generation INTEGER NOT NULL DEFAULT 1;
ALTER TABLE playback_sessions ADD COLUMN renegotiation_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE playback_sessions ADD COLUMN last_renegotiation_request_id TEXT NOT NULL DEFAULT '';
ALTER TABLE cast_receiver_sessions ADD COLUMN last_command_id TEXT NOT NULL DEFAULT '';
ALTER TABLE cast_receiver_sessions ADD COLUMN last_command_json TEXT NOT NULL DEFAULT '';
