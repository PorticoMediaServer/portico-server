ALTER TABLE playback_sessions ADD COLUMN api_key_id TEXT NOT NULL DEFAULT '';
ALTER TABLE media_download_grants ADD COLUMN api_key_id TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_receivers ADD COLUMN api_key_id TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_receiver_authorizations ADD COLUMN api_key_id TEXT NOT NULL DEFAULT '';

CREATE INDEX idx_playback_sessions_api_key
    ON playback_sessions (api_key_id, ended_at, state);

CREATE INDEX idx_media_download_grants_api_key
    ON media_download_grants (api_key_id, expires_at, consumed_at);

CREATE INDEX idx_playback_receivers_api_key
    ON playback_receivers (api_key_id, expires_at);

CREATE INDEX idx_receiver_authorizations_api_key
    ON playback_receiver_authorizations (api_key_id, expires_at, revoked_at);
