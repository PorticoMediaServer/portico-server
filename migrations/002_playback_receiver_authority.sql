ALTER TABLE playback_receivers ADD COLUMN receiver_public_key TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_receivers ADD COLUMN receiver_public_key_fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_receivers ADD COLUMN authorization_revision TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_receivers ADD COLUMN expires_at TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_receivers ADD COLUMN client_instance_id TEXT NOT NULL DEFAULT '';

CREATE TABLE playback_receiver_authorizations (
    id TEXT PRIMARY KEY,
    receiver_id TEXT NOT NULL REFERENCES playback_receivers(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    controller_id TEXT NOT NULL,
    controller_public_key TEXT NOT NULL,
    controller_client_instance_id TEXT NOT NULL,
    receiver_public_key_fingerprint TEXT NOT NULL,
    allowed_commands_json TEXT NOT NULL,
    authorization_revision TEXT NOT NULL,
    request_id TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    response_json TEXT NOT NULL,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT NOT NULL DEFAULT '',
    UNIQUE (user_id, profile_id, request_id)
);

CREATE INDEX idx_playback_receiver_authorizations_receiver_active
    ON playback_receiver_authorizations (receiver_id, profile_id, expires_at, revoked_at);
