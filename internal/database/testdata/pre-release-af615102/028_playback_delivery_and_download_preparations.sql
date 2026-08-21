-- Safe remux decisions require durable, scanner-owned keyframe evidence. A
-- missing timestamp is deliberately not evidence, even if the boolean was
-- populated by an older or partial scan.
ALTER TABLE media_streams ADD COLUMN exact_seek_safe INTEGER NOT NULL DEFAULT 0
    CHECK (exact_seek_safe IN (0, 1));
ALTER TABLE media_streams ADD COLUMN keyframe_evidence_at TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS download_preparations (
    id TEXT PRIMARY KEY,
    server_id TEXT NOT NULL,
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    authorization_revision TEXT NOT NULL,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    quality_profile TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('ready', 'queued', 'running', 'paused', 'failed', 'unavailable', 'cancelled')),
    job_id TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    size_kind TEXT NOT NULL DEFAULT 'unknown' CHECK (size_kind IN ('unknown', 'estimated', 'exact')),
    artifact_expires_at TEXT NOT NULL DEFAULT '',
    progress INTEGER NOT NULL DEFAULT 0,
    error_code TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    cancelled_at TEXT NOT NULL DEFAULT '',
    removed_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_download_preparations_owner
    ON download_preparations(server_id, account_id, profile_id, removed_at, updated_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS idx_download_preparations_active_owner_artifact
    ON download_preparations(server_id, account_id, profile_id, media_id, quality_profile)
    WHERE removed_at = '';

CREATE INDEX IF NOT EXISTS idx_download_preparations_artifact
    ON download_preparations(server_id, media_id, quality_profile, state, removed_at);

CREATE INDEX IF NOT EXISTS idx_download_preparations_job
    ON download_preparations(server_id, job_id)
    WHERE job_id <> '';
