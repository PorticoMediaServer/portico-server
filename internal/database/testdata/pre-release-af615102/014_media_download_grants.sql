CREATE TABLE IF NOT EXISTS media_download_grants (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    server_id TEXT NOT NULL,
    principal_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    version_kind TEXT NOT NULL CHECK (version_kind IN ('source', 'optimized')),
    version_id TEXT NOT NULL,
    version_fingerprint TEXT NOT NULL,
    profile TEXT NOT NULL,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_media_download_grants_principal
    ON media_download_grants(principal_user_id, consumed_at, expires_at);

CREATE INDEX IF NOT EXISTS idx_media_download_grants_media
    ON media_download_grants(media_id, consumed_at, expires_at);
