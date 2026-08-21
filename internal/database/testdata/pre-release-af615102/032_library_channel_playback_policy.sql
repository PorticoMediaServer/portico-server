-- Library Channel delivery binds its Live HLS extension to the canonical
-- playback session. Session/channel deletion cascades and short retention
-- prevents stale grants from becoming durable policy records.
CREATE TABLE library_channel_playback_policies (
    playback_session_id TEXT PRIMARY KEY REFERENCES playback_sessions(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES library_channels(id) ON DELETE CASCADE,
    policy_json TEXT NOT NULL,
    resource_revision INTEGER NOT NULL CHECK (resource_revision > 0),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);

CREATE INDEX idx_library_channel_playback_policies_expiry
    ON library_channel_playback_policies(expires_at, channel_id);

CREATE INDEX idx_library_channel_playback_policies_channel
    ON library_channel_playback_policies(channel_id, created_at);
