-- Server-owned, profile-independent Library Channel schedules. All instants
-- are Unix seconds so lease and range comparisons are numeric and DB-owned.

CREATE TABLE IF NOT EXISTS library_channels (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    description TEXT NOT NULL DEFAULT '' CHECK (length(description) <= 1000),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    sort_order INTEGER NOT NULL DEFAULT 0,
    timezone TEXT NOT NULL CHECK (length(timezone) BETWEEN 1 AND 128),
    seed TEXT NOT NULL CHECK (length(seed) BETWEEN 1 AND 256),
    default_rule_id TEXT,
    quality_profile TEXT NOT NULL DEFAULT 'automatic' CHECK (length(quality_profile) BETWEEN 1 AND 64),
    logo_source TEXT NOT NULL DEFAULT 'none' CHECK (logo_source IN ('none', 'built_in', 'custom')),
    logo_ref TEXT NOT NULL DEFAULT '' CHECK (length(logo_ref) <= 128 AND instr(logo_ref, '/') = 0 AND instr(logo_ref, '\\') = 0 AND instr(logo_ref, '..') = 0),
    logo_mime_type TEXT NOT NULL DEFAULT '' CHECK (logo_mime_type IN ('', 'image/svg+xml', 'image/png', 'image/webp')),
    bug_enabled INTEGER NOT NULL DEFAULT 0 CHECK (bug_enabled IN (0, 1)),
    bug_overhead_accepted INTEGER NOT NULL DEFAULT 0 CHECK (bug_overhead_accepted IN (0, 1)),
    bug_corner TEXT NOT NULL DEFAULT 'top_right' CHECK (bug_corner IN ('top_left', 'top_right', 'bottom_left', 'bottom_right')),
    bug_width_percent REAL NOT NULL DEFAULT 8.0 CHECK (bug_width_percent >= 2.0 AND bug_width_percent <= 20.0),
    bug_inset_percent REAL NOT NULL DEFAULT 2.5 CHECK (bug_inset_percent >= 0.0 AND bug_inset_percent <= 10.0),
    bug_treatment TEXT NOT NULL DEFAULT 'color' CHECK (bug_treatment IN ('color', 'white', 'black')),
    template_key TEXT NOT NULL DEFAULT '',
    template_version INTEGER NOT NULL DEFAULT 0 CHECK (template_version >= 0),
    config_revision INTEGER NOT NULL DEFAULT 1 CHECK (config_revision > 0),
    active_generation_id TEXT,
    generated_through INTEGER,
    health_state TEXT NOT NULL DEFAULT 'pending' CHECK (health_state IN ('pending', 'healthy', 'warning', 'error', 'disabled')),
    health_message TEXT NOT NULL DEFAULT '' CHECK (length(health_message) <= 2000),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
	FOREIGN KEY (id, default_rule_id) REFERENCES library_channel_rules(channel_id, id) DEFERRABLE INITIALLY DEFERRED,
    FOREIGN KEY (id, active_generation_id) REFERENCES library_channel_schedule_generations(channel_id, id) DEFERRABLE INITIALLY DEFERRED
);

CREATE INDEX IF NOT EXISTS idx_library_channels_enabled_sort ON library_channels(enabled, sort_order, name, id);
-- A built-in template may be restored at most once. This is a database
-- invariant (not a list-then-insert convention), so concurrent and repeated
-- restore requests remain idempotent.
CREATE UNIQUE INDEX IF NOT EXISTS idx_library_channels_template ON library_channels(template_key) WHERE template_key <> '';

-- Schedule work is durable and coalesced by channel. Configuration writes
-- enqueue the exact revision in the same transaction; the bounded background
-- maintainer retries with backoff and a later revision always wins.
CREATE TABLE IF NOT EXISTS library_channel_generation_queue (
    channel_id TEXT PRIMARY KEY REFERENCES library_channels(id) ON DELETE CASCADE,
    requested_revision INTEGER NOT NULL CHECK (requested_revision > 0),
    requested_at INTEGER NOT NULL CHECK (requested_at >= 0),
    not_before INTEGER NOT NULL CHECK (not_before >= 0),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 1000),
    last_error TEXT NOT NULL DEFAULT '' CHECK (length(last_error) <= 2000)
);

CREATE INDEX IF NOT EXISTS idx_library_channel_generation_queue_due
ON library_channel_generation_queue(not_before, requested_at, channel_id);

CREATE TABLE IF NOT EXISTS library_channel_rules (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES library_channels(id) ON DELETE CASCADE,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    sort_order INTEGER NOT NULL DEFAULT 0,
    query_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(query_json) AND json_type(query_json) = 'object' AND length(query_json) <= 65536),
    selection_mode TEXT NOT NULL DEFAULT 'shuffle_bag' CHECK (selection_mode IN ('sequential', 'shuffle_bag', 'weighted_random')),
    episode_mode TEXT NOT NULL DEFAULT 'none' CHECK (episode_mode IN ('none', 'in_order', 'marathon', 'randomized')),
    exhaustion_mode TEXT NOT NULL DEFAULT 'loop' CHECK (exhaustion_mode IN ('loop', 'slate')),
    dedupe_window INTEGER NOT NULL DEFAULT 0 CHECK (dedupe_window BETWEEN 0 AND 1000),
    max_consecutive INTEGER NOT NULL DEFAULT 0 CHECK (max_consecutive BETWEEN 0 AND 1000),
    config_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(config_json) AND json_type(config_json) = 'object' AND length(config_json) <= 65536),
    template_key TEXT NOT NULL DEFAULT '',
    template_version INTEGER NOT NULL DEFAULT 0 CHECK (template_version >= 0),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
    UNIQUE(channel_id, id)
);

CREATE INDEX IF NOT EXISTS idx_library_channel_rules_channel ON library_channel_rules(channel_id, enabled, sort_order, id);

CREATE TABLE IF NOT EXISTS library_channel_blocks (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES library_channels(id) ON DELETE CASCADE,
    rule_id TEXT NOT NULL,
    fallback_rule_id TEXT,
    name TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    weekday_mask INTEGER NOT NULL DEFAULT 127 CHECK (weekday_mask BETWEEN 1 AND 127),
    start_minute INTEGER NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
    end_minute INTEGER NOT NULL CHECK (end_minute BETWEEN 0 AND 1439),
    priority INTEGER NOT NULL DEFAULT 0,
    anchored INTEGER NOT NULL DEFAULT 0 CHECK (anchored IN (0, 1)),
    allow_overrun INTEGER NOT NULL DEFAULT 0 CHECK (allow_overrun IN (0, 1)),
    sort_order INTEGER NOT NULL DEFAULT 0,
    template_key TEXT NOT NULL DEFAULT '',
    template_version INTEGER NOT NULL DEFAULT 0 CHECK (template_version >= 0),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    updated_at INTEGER NOT NULL CHECK (updated_at >= 0),
    FOREIGN KEY (channel_id, rule_id) REFERENCES library_channel_rules(channel_id, id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
	FOREIGN KEY (fallback_rule_id) REFERENCES library_channel_rules(id) ON DELETE SET NULL DEFERRABLE INITIALLY DEFERRED
);

CREATE TRIGGER IF NOT EXISTS trg_library_channel_blocks_fallback_insert
BEFORE INSERT ON library_channel_blocks
WHEN NEW.fallback_rule_id IS NOT NULL
AND NOT EXISTS (SELECT 1 FROM library_channel_rules r WHERE r.id=NEW.fallback_rule_id AND r.channel_id=NEW.channel_id)
BEGIN
	SELECT RAISE(ABORT, 'library channel fallback rule belongs to another channel');
END;

CREATE TRIGGER IF NOT EXISTS trg_library_channel_blocks_fallback_update
BEFORE UPDATE OF channel_id, fallback_rule_id ON library_channel_blocks
WHEN NEW.fallback_rule_id IS NOT NULL
AND NOT EXISTS (SELECT 1 FROM library_channel_rules r WHERE r.id=NEW.fallback_rule_id AND r.channel_id=NEW.channel_id)
BEGIN
	SELECT RAISE(ABORT, 'library channel fallback rule belongs to another channel');
END;

CREATE INDEX IF NOT EXISTS idx_library_channel_blocks_channel ON library_channel_blocks(channel_id, enabled, priority DESC, sort_order, id);
CREATE INDEX IF NOT EXISTS idx_library_channel_blocks_rule ON library_channel_blocks(channel_id, rule_id);

CREATE TABLE IF NOT EXISTS library_channel_schedule_generations (
    id TEXT PRIMARY KEY,
    channel_id TEXT NOT NULL REFERENCES library_channels(id) ON DELETE CASCADE,
    config_revision INTEGER NOT NULL CHECK (config_revision > 0),
    status TEXT NOT NULL DEFAULT 'building' CHECK (status IN ('building', 'active', 'superseded', 'failed')),
    horizon_start INTEGER NOT NULL CHECK (horizon_start >= 0),
    horizon_end INTEGER NOT NULL CHECK (horizon_end > horizon_start),
    deterministic_seed TEXT NOT NULL CHECK (length(deterministic_seed) = 64),
    candidate_hash TEXT NOT NULL CHECK (length(candidate_hash) = 64),
    initial_cursor_hash TEXT NOT NULL CHECK (length(initial_cursor_hash) = 64),
    cursor_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(cursor_json) AND json_type(cursor_json) = 'object'),
    warnings_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(warnings_json) AND json_type(warnings_json) = 'array'),
    error_message TEXT NOT NULL DEFAULT '' CHECK (length(error_message) <= 2000),
    lease_token_hash TEXT NOT NULL DEFAULT '' CHECK (lease_token_hash = '' OR length(lease_token_hash) = 64),
    lease_expires_at INTEGER,
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
    completed_at INTEGER,
    UNIQUE(channel_id, id)
);

CREATE INDEX IF NOT EXISTS idx_library_channel_generations_channel_status ON library_channel_schedule_generations(channel_id, status, created_at DESC);
CREATE UNIQUE INDEX IF NOT EXISTS idx_library_channel_one_active_generation ON library_channel_schedule_generations(channel_id) WHERE status = 'active';
CREATE UNIQUE INDEX IF NOT EXISTS idx_library_channel_one_building_generation ON library_channel_schedule_generations(channel_id) WHERE status = 'building';
CREATE INDEX IF NOT EXISTS idx_library_channel_building_lease ON library_channel_schedule_generations(status, lease_expires_at) WHERE status = 'building';

CREATE TABLE IF NOT EXISTS library_channel_schedule_entries (
    id TEXT NOT NULL,
    generation_id TEXT NOT NULL,
    channel_id TEXT NOT NULL,
    rule_id TEXT NOT NULL DEFAULT '',
    block_id TEXT NOT NULL DEFAULT '',
    media_id TEXT REFERENCES media_items(id) ON DELETE SET NULL,
    entry_kind TEXT NOT NULL DEFAULT 'media' CHECK (entry_kind IN ('media', 'slate', 'unavailable')),
    starts_at INTEGER NOT NULL CHECK (starts_at >= 0),
    ends_at INTEGER NOT NULL CHECK (ends_at > starts_at),
    media_offset_seconds INTEGER NOT NULL DEFAULT 0 CHECK (media_offset_seconds >= 0),
    source_duration_seconds INTEGER NOT NULL DEFAULT 0 CHECK (source_duration_seconds >= 0),
    cycle_number INTEGER NOT NULL DEFAULT 0 CHECK (cycle_number >= 0),
    selection_index INTEGER NOT NULL DEFAULT 0 CHECK (selection_index >= 0),
    title TEXT NOT NULL DEFAULT '', subtitle TEXT NOT NULL DEFAULT '', summary TEXT NOT NULL DEFAULT '', content_rating TEXT NOT NULL DEFAULT '',
    artwork_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(artwork_json)),
    availability TEXT NOT NULL DEFAULT 'available' CHECK (availability IN ('available', 'missing', 'restricted', 'unavailable')),
    selection_metadata_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(selection_metadata_json)),
    reason_code TEXT NOT NULL DEFAULT '',
    playout_source_json TEXT NOT NULL CHECK (json_valid(playout_source_json) AND json_type(playout_source_json) = 'object'),
    cursor_after_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(cursor_after_json) AND json_type(cursor_after_json) = 'object'),
    created_at INTEGER NOT NULL CHECK (created_at >= 0),
	PRIMARY KEY (generation_id, id),
    FOREIGN KEY (generation_id, channel_id) REFERENCES library_channel_schedule_generations(id, channel_id) ON DELETE CASCADE,
    UNIQUE(generation_id, starts_at),
    CHECK ((entry_kind = 'media' AND media_id IS NOT NULL) OR (entry_kind IN ('slate', 'unavailable') AND media_id IS NULL))
);

CREATE INDEX IF NOT EXISTS idx_library_channel_entries_guide ON library_channel_schedule_entries(channel_id, starts_at, ends_at);
CREATE INDEX IF NOT EXISTS idx_library_channel_entries_generation ON library_channel_schedule_entries(generation_id, starts_at, id);
CREATE INDEX IF NOT EXISTS idx_library_channel_entries_media ON library_channel_schedule_entries(media_id) WHERE media_id IS NOT NULL;

-- Keep the linear timeline, but delete all private metadata with the source.
-- Cursor identities are one-way SHA-256 tokens, so checkpoints remain useful
-- without retaining the deleted media identifier.
CREATE TRIGGER IF NOT EXISTS trg_library_channel_entries_media_delete
BEFORE DELETE ON media_items
BEGIN
	UPDATE library_channels
	SET health_state = 'pending', health_message = 'library-channel.health-regeneration-required', updated_at = unixepoch()
	WHERE id IN (SELECT channel_id FROM library_channel_schedule_entries WHERE media_id = OLD.id);
	INSERT INTO library_channel_generation_queue(channel_id,requested_revision,requested_at,not_before,attempts,last_error)
	SELECT id,config_revision,unixepoch(),unixepoch(),0,''
	FROM library_channels
	WHERE id IN (SELECT channel_id FROM library_channel_schedule_entries WHERE media_id = OLD.id)
	ON CONFLICT(channel_id) DO UPDATE SET
		requested_revision=excluded.requested_revision,
		requested_at=excluded.requested_at,
		not_before=excluded.not_before,
		attempts=0,
		last_error='';
    UPDATE library_channel_schedule_entries
    SET media_id = NULL, entry_kind = 'unavailable', availability = 'missing',
        title = '', subtitle = '', summary = '', content_rating = '', artwork_json = '{}',
        selection_metadata_json = '{}', reason_code = 'library-channel.program-unavailable',
        playout_source_json = json_object('kind','unavailable','durationSeconds',ends_at-starts_at)
    WHERE media_id = OLD.id;
END;
