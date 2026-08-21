ALTER TABLE media_files ADD COLUMN content_fingerprint TEXT NOT NULL DEFAULT '';
ALTER TABLE media_files ADD COLUMN identity_evidence TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_media_files_identity_fingerprint
ON media_files(library_id, source_type, content_fingerprint)
WHERE content_fingerprint <> '';

CREATE TABLE IF NOT EXISTS identity_reconciliation_reviews (
    id TEXT PRIMARY KEY,
    domain TEXT NOT NULL,
    library_or_source_id TEXT NOT NULL DEFAULT '',
    candidate_locator TEXT NOT NULL DEFAULT '',
    evidence_kind TEXT NOT NULL,
    evidence_value TEXT NOT NULL DEFAULT '',
    candidate_ids_json TEXT NOT NULL DEFAULT '[]',
    status TEXT NOT NULL DEFAULT 'open',
    created_at TEXT NOT NULL,
    resolved_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_identity_reconciliation_reviews_open
ON identity_reconciliation_reviews(domain, status, created_at DESC);

CREATE TABLE IF NOT EXISTS media_scanner_identity_aliases (
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    scanner_key TEXT NOT NULL,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    PRIMARY KEY (library_id, scanner_key)
);

CREATE INDEX IF NOT EXISTS idx_media_scanner_identity_aliases_media
ON media_scanner_identity_aliases(media_id);

CREATE TABLE IF NOT EXISTS live_tv_channel_locators (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES live_tv_channels(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES live_tv_sources(id) ON DELETE CASCADE,
    provider_kind TEXT NOT NULL DEFAULT '',
    provider_key TEXT NOT NULL DEFAULT '',
    stream_url TEXT NOT NULL DEFAULT '',
    tvg_id TEXT NOT NULL DEFAULT '',
    channel_number TEXT NOT NULL DEFAULT '',
    normalized_name TEXT NOT NULL DEFAULT '',
    active INTEGER NOT NULL DEFAULT 1,
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_live_tv_channel_locators_channel
ON live_tv_channel_locators(channel_id);

CREATE INDEX IF NOT EXISTS idx_live_tv_channel_locators_provider
ON live_tv_channel_locators(source_id, provider_kind, provider_key)
WHERE provider_key <> '';

CREATE INDEX IF NOT EXISTS idx_live_tv_channel_locators_stream
ON live_tv_channel_locators(source_id, stream_url)
WHERE stream_url <> '';

CREATE INDEX IF NOT EXISTS idx_live_tv_channel_locators_tvg
ON live_tv_channel_locators(source_id, tvg_id)
WHERE tvg_id <> '';

INSERT INTO live_tv_channel_locators (
    id, channel_id, source_id, provider_kind, provider_key, stream_url, tvg_id,
    channel_number, normalized_name, active, first_seen_at, last_seen_at
)
SELECT
    'ltvloc_' || substr(lower(hex(randomblob(16))), 1, 24),
    id,
    source_id,
    'legacy',
    CASE WHEN tvg_id <> '' THEN tvg_id ELSE number END,
    stream_url,
    tvg_id,
    number,
    lower(trim(name)),
    enabled,
    created_at,
    last_seen_at
FROM live_tv_channels
WHERE NOT EXISTS (
    SELECT 1 FROM live_tv_channel_locators l WHERE l.channel_id = live_tv_channels.id
);
