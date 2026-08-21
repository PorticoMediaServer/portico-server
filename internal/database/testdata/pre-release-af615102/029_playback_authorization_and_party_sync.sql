-- These tables historically originated in the compatibility pass that follows
-- SQL migrations. Establish the pre-029 shapes here so the durable additions
-- below work for both clean installs and upgrades.
CREATE TABLE IF NOT EXISTS media_segments (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    segment_type TEXT NOT NULL DEFAULT '',
    start_seconds INTEGER NOT NULL DEFAULT 0,
    end_seconds INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    confidence REAL NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE(media_id, segment_type, start_seconds, end_seconds, source, provider)
);

CREATE TABLE IF NOT EXISTS playback_receivers (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    code TEXT NOT NULL,
    app TEXT NOT NULL DEFAULT '',
    platform TEXT NOT NULL DEFAULT '',
    supported_commands_json TEXT NOT NULL DEFAULT '["load"]',
    command_json TEXT NOT NULL DEFAULT '{}',
    command_updated_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS watch_with_friends_groups (
    id TEXT PRIMARY KEY,
    owner_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    owner_profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    name TEXT NOT NULL DEFAULT '',
    media_id TEXT NOT NULL,
    media_title TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'paused',
    position_seconds INTEGER NOT NULL DEFAULT 0,
    shuffle_enabled INTEGER NOT NULL DEFAULT 0,
    repeat_mode TEXT NOT NULL DEFAULT 'none',
    command_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    ended_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS playback_sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL,
    media_type TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    ended_at TEXT NOT NULL DEFAULT '',
    client_ip TEXT NOT NULL DEFAULT '',
    client_instance_id TEXT NOT NULL DEFAULT '',
    device TEXT NOT NULL DEFAULT '',
    app TEXT NOT NULL DEFAULT '',
    location TEXT NOT NULL DEFAULT '',
    state TEXT NOT NULL DEFAULT 'playing',
    progress INTEGER NOT NULL DEFAULT 0,
    position_seconds INTEGER NOT NULL DEFAULT 0,
    bandwidth_mbps REAL NOT NULL DEFAULT 0,
    decision TEXT NOT NULL DEFAULT '',
    video_decision TEXT NOT NULL DEFAULT '',
    video_source TEXT NOT NULL DEFAULT '',
    video_target TEXT NOT NULL DEFAULT '',
    audio_decision TEXT NOT NULL DEFAULT '',
    audio_source TEXT NOT NULL DEFAULT '',
    audio_target TEXT NOT NULL DEFAULT '',
    subtitle_decision TEXT NOT NULL DEFAULT 'None',
    diagnostics_json TEXT NOT NULL DEFAULT '{}',
    source_context_json TEXT NOT NULL DEFAULT '{}',
    history_paused INTEGER NOT NULL DEFAULT 0,
    last_event_sequence INTEGER NOT NULL DEFAULT 0,
    last_event_recorded_at TEXT NOT NULL DEFAULT '',
    last_event_received_at TEXT NOT NULL DEFAULT '',
    repeat_mode TEXT NOT NULL DEFAULT 'off' CHECK (repeat_mode IN ('off', 'one', 'all')),
    queue_revision INTEGER NOT NULL DEFAULT 0 CHECK (queue_revision >= 0),
    is_live INTEGER NOT NULL DEFAULT 0
);
-- Older databases receive playback_sessions from the compatibility schema
-- without profile_id. Keep the bootstrap table at that historical shape, then
-- add the profile binding exactly once for both clean installs and upgrades.
ALTER TABLE playback_sessions ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_playback_sessions_profile_active ON playback_sessions(profile_id, ended_at, last_seen_at);

ALTER TABLE media_segments ADD COLUMN automatic_safe INTEGER NOT NULL DEFAULT 0;
DELETE FROM media_segments
WHERE source = 'generated' AND provider = 'chapter-markers';

ALTER TABLE playback_receivers ADD COLUMN profile_id TEXT NOT NULL DEFAULT '';
-- Legacy receivers were account-owned and did not carry a viewing profile.
-- Preserve them only when the account has a truthful primary profile
-- relationship; never assume that an account/user id is also a profile id.
UPDATE playback_receivers
SET profile_id = COALESCE((
    SELECT id
    FROM profiles
    WHERE account_id = playback_receivers.user_id AND is_primary = 1
    ORDER BY id
    LIMIT 1
), '')
WHERE profile_id = '';
-- A receiver without a provable account/profile relationship cannot safely be
-- exposed to any profile. Removing the stale pairing is the least-privilege
-- quarantine for legacy rows; clients can pair the receiver again explicitly.
DELETE FROM playback_receivers
WHERE profile_id = ''
   OR NOT EXISTS (
       SELECT 1
       FROM profiles
       WHERE profiles.id = playback_receivers.profile_id
         AND profiles.account_id = playback_receivers.user_id
   );
CREATE INDEX IF NOT EXISTS idx_playback_receivers_profile_seen
    ON playback_receivers(profile_id, last_seen_at);
CREATE TRIGGER IF NOT EXISTS trg_playback_receivers_profile_insert_guard
BEFORE INSERT ON playback_receivers
WHEN NEW.profile_id = '' OR NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id AND profiles.account_id = NEW.user_id
)
BEGIN
    SELECT RAISE(ABORT, 'playback receiver profile does not belong to account');
END;
CREATE TRIGGER IF NOT EXISTS trg_playback_receivers_profile_update_guard
BEFORE UPDATE OF user_id, profile_id ON playback_receivers
WHEN NEW.profile_id = '' OR NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id AND profiles.account_id = NEW.user_id
)
BEGIN
    SELECT RAISE(ABORT, 'playback receiver profile does not belong to account');
END;

ALTER TABLE watch_with_friends_groups ADD COLUMN revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE watch_with_friends_groups ADD COLUMN playback_revision INTEGER NOT NULL DEFAULT 0;
ALTER TABLE watch_with_friends_groups ADD COLUMN position_updated_at TEXT NOT NULL DEFAULT '';
ALTER TABLE watch_with_friends_groups ADD COLUMN playback_rate REAL NOT NULL DEFAULT 1;
ALTER TABLE watch_with_friends_groups ADD COLUMN reconnect_generation INTEGER NOT NULL DEFAULT 0;
ALTER TABLE watch_with_friends_groups ADD COLUMN last_idempotency_key TEXT NOT NULL DEFAULT '';
UPDATE watch_with_friends_groups
SET position_updated_at = updated_at
WHERE position_updated_at = '';

ALTER TABLE playback_media_grants ADD COLUMN authorization_revision TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_media_grants ADD COLUMN delivery_mode TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_media_grants ADD COLUMN transcode_quality TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_media_grants ADD COLUMN allowed_qualities_json TEXT NOT NULL DEFAULT '[]';
ALTER TABLE media_download_grants ADD COLUMN authorization_revision TEXT NOT NULL DEFAULT '';
ALTER TABLE media_download_grants ADD COLUMN preparation_id TEXT NOT NULL DEFAULT '';

ALTER TABLE playback_sessions ADD COLUMN selected_quality_id TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_sessions ADD COLUMN selected_audio_stream_id TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_sessions ADD COLUMN selected_subtitle_stream_id TEXT NOT NULL DEFAULT '';
ALTER TABLE playback_sessions ADD COLUMN selected_subtitle_mode TEXT NOT NULL DEFAULT 'off';
ALTER TABLE playback_sessions ADD COLUMN selected_version_id TEXT NOT NULL DEFAULT '';

CREATE TABLE watch_with_friends_command_receipts (
    group_id TEXT NOT NULL REFERENCES watch_with_friends_groups(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    response_revision INTEGER NOT NULL,
    response_playback_revision INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (group_id, idempotency_key)
);

CREATE INDEX idx_watch_with_friends_command_receipts_created
    ON watch_with_friends_command_receipts(group_id, created_at DESC);
