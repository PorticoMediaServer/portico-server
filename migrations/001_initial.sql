-- Portico Server release baseline.
-- Pre-release database histories are intentionally unsupported; future schema changes append migrations.
PRAGMA foreign_keys = ON;
CREATE TABLE api_keys (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE,
    last_four TEXT NOT NULL DEFAULT '',
    scopes_json TEXT NOT NULL DEFAULT '[]',
    created_at TEXT NOT NULL,
    last_used_at TEXT NOT NULL DEFAULT '',
    revoked_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE audio_fingerprints (
    media_id TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
    path TEXT NOT NULL DEFAULT '',
    fingerprint TEXT NOT NULL DEFAULT '',
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    acoustid_id TEXT NOT NULL DEFAULT '',
    recording_id TEXT NOT NULL DEFAULT '',
    score REAL NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);
CREATE TABLE audio_normalization (
			media_id TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
			track_gain_db REAL NOT NULL DEFAULT 0,
			track_peak REAL NOT NULL DEFAULT 0,
			album_gain_db REAL NOT NULL DEFAULT 0,
			album_peak REAL NOT NULL DEFAULT 0,
			integrated_lufs REAL NOT NULL DEFAULT 0,
			source TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);
CREATE TABLE audiobook_browse_entities (
    id TEXT PRIMARY KEY,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    entity_kind TEXT NOT NULL CHECK (entity_kind IN ('author', 'audiobook-series')),
    identity_key TEXT NOT NULL,
    display_name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (library_id, entity_kind, identity_key)
);
CREATE TABLE audiobook_browse_entity_members (
    entity_id TEXT NOT NULL REFERENCES audiobook_browse_entities(id) ON DELETE CASCADE,
    entity_kind TEXT NOT NULL CHECK (entity_kind IN ('author', 'audiobook-series')),
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    evidence_key TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (entity_id, media_id),
    UNIQUE (entity_kind, media_id)
);
CREATE TABLE audit_events (
    id TEXT PRIMARY KEY,
    actor_user_id TEXT NOT NULL DEFAULT '',
    actor_email TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL DEFAULT 'info',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    client_ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE TABLE automatic_profile_selection_trusts (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id TEXT NOT NULL CHECK (server_id <> ''),
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    installation_id TEXT NOT NULL CHECK (installation_id <> ''),
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    pin_revision INTEGER NOT NULL CHECK (pin_revision >= 0),
    expires_at TEXT NOT NULL,
    revoked_at TEXT NOT NULL DEFAULT '',
    last_used_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (authority, account_id, server_id, device_id, installation_id, profile_id)
);
CREATE TABLE browser_account_entries (
    vault_id TEXT NOT NULL REFERENCES browser_account_vaults(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    auth_provider TEXT NOT NULL CHECK (auth_provider IN ('local', 'portico')),
    profile_identity_id TEXT NOT NULL REFERENCES profile_identities(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    added_at TEXT NOT NULL,
    last_used_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (vault_id, user_id)
);
CREATE TABLE browser_account_vaults (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    device_id TEXT NOT NULL,
    active_user_id TEXT REFERENCES users(id) ON DELETE SET NULL,
    automatic_sign_in INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    revoked_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE cast_bootstraps (
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
, automation_json TEXT NOT NULL DEFAULT '{}', source_playback_session_id TEXT NOT NULL DEFAULT '');
CREATE TABLE cast_receiver_sessions (
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
, last_command_id TEXT NOT NULL DEFAULT '', last_command_json TEXT NOT NULL DEFAULT '', automation_json TEXT NOT NULL DEFAULT '{}', automatic_advances INTEGER NOT NULL DEFAULT 0, last_advance_id TEXT NOT NULL DEFAULT '', last_advance_json TEXT NOT NULL DEFAULT '', source_playback_session_id TEXT NOT NULL DEFAULT '');
CREATE TABLE client_diagnostic_events (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL DEFAULT '',
    device TEXT NOT NULL DEFAULT '',
    app TEXT NOT NULL DEFAULT '',
    origin TEXT NOT NULL DEFAULT '',
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    fields_json TEXT NOT NULL DEFAULT '{}',
    client_time TEXT NOT NULL DEFAULT '',
    byte_size INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE TABLE dashboard_playback_rollups (
			session_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			user_name TEXT NOT NULL DEFAULT '',
			user_role TEXT NOT NULL DEFAULT '',
			media_id TEXT NOT NULL DEFAULT '',
			media_type TEXT NOT NULL DEFAULT '',
			media_group TEXT NOT NULL DEFAULT 'Other',
			title TEXT NOT NULL DEFAULT '',
			art_seed TEXT NOT NULL DEFAULT '',
			library_id TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			ended_at TEXT NOT NULL,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
			location TEXT NOT NULL DEFAULT '',
			bandwidth_mbps REAL NOT NULL DEFAULT 0,
			decision TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL
		);
CREATE TABLE devices (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    app TEXT NOT NULL DEFAULT '',
    platform TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    client_ip TEXT NOT NULL DEFAULT '',
    trusted INTEGER NOT NULL DEFAULT 1,
    revoked_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
, installation_id TEXT NOT NULL DEFAULT '', display_name TEXT NOT NULL DEFAULT '', remote_bitrate_limit_mbps INTEGER NOT NULL DEFAULT 0, options_json TEXT NOT NULL DEFAULT '{}');
CREATE TABLE download_preparations (
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
CREATE TABLE dvr_recording_media (
    recording_id TEXT PRIMARY KEY REFERENCES live_tv_recordings(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL UNIQUE REFERENCES media_items(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL
);
CREATE TABLE hosted_profile_selection_assertion_receipts (
    assertion_id TEXT PRIMARY KEY,
    payload_digest TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    hosted_device_id TEXT NOT NULL,
    local_device_id TEXT NOT NULL,
    installation_id TEXT NOT NULL,
    pin_revision INTEGER NOT NULL CHECK (pin_revision >= 0),
    expires_at TEXT NOT NULL,
    accepted_at TEXT NOT NULL
);
CREATE TABLE hosted_profile_snapshot_state (
    account_id TEXT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    snapshot_id TEXT NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    payload_digest TEXT NOT NULL,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    applied_at TEXT NOT NULL
, checked_at TEXT NOT NULL DEFAULT '', max_age_seconds INTEGER NOT NULL DEFAULT 300, stale_if_error_seconds INTEGER NOT NULL DEFAULT 86400, refresh_retry_at TEXT NOT NULL DEFAULT '');
CREATE TABLE identity_reconciliation_reviews (
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
, subject_id TEXT NOT NULL DEFAULT '', resolution TEXT NOT NULL DEFAULT '', selected_candidate_id TEXT NOT NULL DEFAULT '', resolved_by_user_id TEXT NOT NULL DEFAULT '', resolution_note TEXT NOT NULL DEFAULT '');
CREATE TABLE jobs (
    id TEXT PRIMARY KEY,
    type TEXT NOT NULL,
    status TEXT NOT NULL,
    progress INTEGER NOT NULL DEFAULT 0,
    message TEXT NOT NULL DEFAULT '',
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    attempt_count INTEGER NOT NULL DEFAULT 0,
    next_run_at TEXT NOT NULL DEFAULT '',
    leased_by TEXT NOT NULL DEFAULT '',
    lease_expires_at TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    failure_kind TEXT NOT NULL DEFAULT '',
    deferred_until TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, parent_operation_id TEXT NOT NULL DEFAULT '', idempotency_key TEXT NOT NULL DEFAULT '', active_key TEXT NOT NULL DEFAULT '', priority TEXT NOT NULL DEFAULT 'normal', phase TEXT NOT NULL DEFAULT '', progress_current INTEGER NOT NULL DEFAULT 0, progress_total INTEGER NOT NULL DEFAULT 0, result_reference TEXT NOT NULL DEFAULT '', error_code TEXT NOT NULL DEFAULT '', retry_eligible INTEGER NOT NULL DEFAULT 0, cancellation_requested_at TEXT NOT NULL DEFAULT '', worker_acknowledged_at TEXT NOT NULL DEFAULT '', interrupted_at TEXT NOT NULL DEFAULT '', retention_until TEXT NOT NULL DEFAULT '');
CREATE TABLE libraries (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    sort_order INTEGER NOT NULL DEFAULT 0,
    path TEXT NOT NULL DEFAULT '',
    settings_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL
);
CREATE TABLE storage_sources (
    id TEXT PRIMARY KEY,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    configured_path TEXT NOT NULL,
    resolved_path TEXT NOT NULL DEFAULT '',
    classification TEXT NOT NULL DEFAULT 'unknown' CHECK (classification IN ('local', 'network', 'fuse', 'unknown')),
    classification_source TEXT NOT NULL DEFAULT 'detected' CHECK (classification_source IN ('detected', 'owner')),
    health_state TEXT NOT NULL DEFAULT 'unknown' CHECK (health_state IN ('unknown', 'healthy', 'degraded', 'offline', 'stalled')),
    circuit_state TEXT NOT NULL DEFAULT 'closed' CHECK (circuit_state IN ('closed', 'open', 'half_open')),
    error_class TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '' CHECK (length(error_message) <= 2000),
    latency_ms INTEGER NOT NULL DEFAULT 0 CHECK (latency_ms >= 0),
    consecutive_failures INTEGER NOT NULL DEFAULT 0 CHECK (consecutive_failures >= 0),
    last_progress_at TEXT NOT NULL DEFAULT '',
    last_success_at TEXT NOT NULL DEFAULT '',
    last_failure_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(library_id, configured_path)
);
CREATE TABLE library_scan_runs (
    id TEXT PRIMARY KEY,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    job_id TEXT NOT NULL DEFAULT '',
    mode TEXT NOT NULL CHECK (mode IN ('targeted', 'quick', 'reconcile', 'force_full', 'remove_missing')),
    status TEXT NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'healthy', 'degraded', 'failed', 'cancelled')),
    phase TEXT NOT NULL DEFAULT 'admission',
    files_indexed INTEGER NOT NULL DEFAULT 0 CHECK (files_indexed >= 0),
    files_unchanged INTEGER NOT NULL DEFAULT 0 CHECK (files_unchanged >= 0),
    files_skipped INTEGER NOT NULL DEFAULT 0 CHECK (files_skipped >= 0),
    missing_marked INTEGER NOT NULL DEFAULT 0 CHECK (missing_marked >= 0),
    metadata_queued INTEGER NOT NULL DEFAULT 0 CHECK (metadata_queued >= 0),
    analysis_queued INTEGER NOT NULL DEFAULT 0 CHECK (analysis_queued >= 0),
    absence_authoritative INTEGER NOT NULL DEFAULT 0 CHECK (absence_authoritative IN (0, 1)),
    cleanup_allowed INTEGER NOT NULL DEFAULT 0 CHECK (cleanup_allowed IN (0, 1)),
    warnings_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(warnings_json) AND json_type(warnings_json) = 'array'),
    started_at TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL
);
CREATE TABLE library_scan_run_roots (
    run_id TEXT NOT NULL REFERENCES library_scan_runs(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES storage_sources(id) ON DELETE CASCADE,
    configured_path TEXT NOT NULL,
    resolved_path TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'healthy', 'degraded', 'offline', 'stalled', 'cancelled')),
    error_class TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '' CHECK (length(error_message) <= 2000),
    directories_seen INTEGER NOT NULL DEFAULT 0 CHECK (directories_seen >= 0),
    files_seen INTEGER NOT NULL DEFAULT 0 CHECK (files_seen >= 0),
    started_at TEXT NOT NULL DEFAULT '',
    completed_at TEXT NOT NULL DEFAULT '',
    last_progress_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (run_id, source_id)
);
CREATE TABLE library_scan_continuations (
    library_id TEXT PRIMARY KEY REFERENCES libraries(id) ON DELETE CASCADE,
    mode TEXT NOT NULL CHECK (mode IN ('targeted', 'quick', 'reconcile', 'force_full', 'remove_missing')),
    scan_generation TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE library_scan_continuation_directories (
    library_id TEXT NOT NULL REFERENCES library_scan_continuations(library_id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    signature TEXT NOT NULL,
    media_file_count INTEGER NOT NULL DEFAULT 0 CHECK (media_file_count >= 0),
    changed INTEGER NOT NULL DEFAULT 1 CHECK (changed IN (0, 1)),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (library_id, path)
);
CREATE TABLE scanner_backlog (
    id TEXT PRIMARY KEY,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('metadata', 'analysis')),
    source_revision TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'claimed', 'complete', 'failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_run_at TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '' CHECK (length(last_error) <= 2000),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE(kind, media_id, source_revision)
);
CREATE TABLE library_category_counts (
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    filter TEXT NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    representative_media_id TEXT NOT NULL DEFAULT '',
    representative_image TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (library_id, filter)
);
CREATE TABLE library_channel_blocks (
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
CREATE TABLE library_channel_generation_queue (
    channel_id TEXT PRIMARY KEY REFERENCES library_channels(id) ON DELETE CASCADE,
    requested_revision INTEGER NOT NULL CHECK (requested_revision > 0),
    requested_at INTEGER NOT NULL CHECK (requested_at >= 0),
    not_before INTEGER NOT NULL CHECK (not_before >= 0),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts BETWEEN 0 AND 1000),
    last_error TEXT NOT NULL DEFAULT '' CHECK (length(last_error) <= 2000)
);
CREATE TABLE library_channel_playback_policies (
    playback_session_id TEXT PRIMARY KEY REFERENCES playback_sessions(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES library_channels(id) ON DELETE CASCADE,
    policy_json TEXT NOT NULL,
    resource_revision INTEGER NOT NULL CHECK (resource_revision > 0),
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE TABLE library_channel_rules (
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
CREATE TABLE library_channel_schedule_entries (
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
CREATE TABLE library_channel_schedule_generations (
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
CREATE TABLE library_channels (
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
CREATE TABLE library_item_counts (
			library_id TEXT PRIMARY KEY REFERENCES libraries(id) ON DELETE CASCADE,
			root_item_count INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT ''
		);
CREATE TABLE library_paths (
			id TEXT PRIMARY KEY,
			library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
			path TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		);
CREATE TABLE library_scan_directories (
			library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
			path TEXT NOT NULL,
			signature TEXT NOT NULL,
			media_file_count INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (library_id, path)
		);
CREATE TABLE library_source_groups (
			library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
			kind TEXT NOT NULL,
			path TEXT NOT NULL,
			label TEXT NOT NULL,
			source_type TEXT NOT NULL,
			filter TEXT NOT NULL,
			item_count INTEGER NOT NULL DEFAULT 0,
			file_count INTEGER NOT NULL DEFAULT 0,
			missing_file_count INTEGER NOT NULL DEFAULT 0,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (library_id, kind, path)
		);
CREATE TABLE live_tv_channel_locators (
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
CREATE TABLE live_tv_channel_mappings (
    source_id TEXT NOT NULL REFERENCES live_tv_sources(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL REFERENCES live_tv_channels(id) ON DELETE CASCADE,
    guide_channel_ref TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (source_id, guide_channel_ref),
    UNIQUE(channel_id)
);
CREATE TABLE live_tv_channel_profile_state (
			profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			channel_id TEXT NOT NULL REFERENCES live_tv_channels(id) ON DELETE CASCADE,
			favorite INTEGER NOT NULL DEFAULT 0 CHECK (favorite IN (0, 1)),
			hidden INTEGER NOT NULL DEFAULT 0 CHECK (hidden IN (0, 1)),
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (profile_id, channel_id)
		);
CREATE VIRTUAL TABLE live_tv_channel_search USING fts5(
    channel_id UNINDEXED,
    source_id UNINDEXED,
    name,
    number,
    tvg_id,
    group_title,
    country,
    source_name
);
CREATE TABLE live_tv_channels (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES live_tv_sources(id) ON DELETE CASCADE,
    number TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    stream_url TEXT NOT NULL,
    logo_url TEXT NOT NULL DEFAULT '',
    tvg_id TEXT NOT NULL DEFAULT '',
    group_title TEXT NOT NULL DEFAULT '',
    country TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 1,
    favorite INTEGER NOT NULL DEFAULT 0,
    hidden INTEGER NOT NULL DEFAULT 0,
    sort_order INTEGER NOT NULL DEFAULT 0,
    last_seen_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    import_generation TEXT NOT NULL DEFAULT ''
);
CREATE TABLE live_tv_programs (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES live_tv_sources(id) ON DELETE CASCADE,
    channel_id TEXT REFERENCES live_tv_channels(id) ON DELETE CASCADE,
    channel_ref TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    subtitle TEXT NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    start_at TEXT NOT NULL,
    end_at TEXT NOT NULL,
    episode_num TEXT NOT NULL DEFAULT '',
    is_new INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
, import_generation TEXT NOT NULL DEFAULT '');
CREATE TABLE live_tv_recording_rules (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES live_tv_sources(id) ON DELETE CASCADE,
    channel_id TEXT REFERENCES live_tv_channels(id) ON DELETE SET NULL,
    program_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    match_type TEXT NOT NULL DEFAULT 'single',
    start_padding_minutes INTEGER NOT NULL DEFAULT 2,
    end_padding_minutes INTEGER NOT NULL DEFAULT 5,
    retention_days INTEGER NOT NULL DEFAULT 30,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, max_recordings_per_series INTEGER NOT NULL DEFAULT 0, required_keywords TEXT NOT NULL DEFAULT '[]', blocked_keywords TEXT NOT NULL DEFAULT '[]', allowed_channels TEXT NOT NULL DEFAULT '[]', blocked_channels TEXT NOT NULL DEFAULT '[]', priority INTEGER NOT NULL DEFAULT 50 CHECK (priority BETWEEN 0 AND 100), revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0), folder TEXT NOT NULL DEFAULT '', profile_id TEXT NOT NULL DEFAULT '', guide_generation TEXT NOT NULL DEFAULT '');
CREATE TABLE live_tv_recordings (
    id TEXT PRIMARY KEY,
    rule_id TEXT REFERENCES live_tv_recording_rules(id) ON DELETE SET NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_id TEXT NOT NULL REFERENCES live_tv_sources(id) ON DELETE CASCADE,
    channel_id TEXT REFERENCES live_tv_channels(id) ON DELETE SET NULL,
    program_id TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'scheduled',
    starts_at TEXT NOT NULL,
    ends_at TEXT NOT NULL,
    path TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, priority INTEGER NOT NULL DEFAULT 50 CHECK (priority BETWEEN 0 AND 100), failure_code TEXT NOT NULL DEFAULT '', revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0), folder TEXT NOT NULL DEFAULT '', profile_id TEXT NOT NULL DEFAULT '', guide_generation TEXT NOT NULL DEFAULT '');
CREATE TABLE live_tv_sources (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    m3u_url TEXT NOT NULL DEFAULT '',
    m3u_text TEXT NOT NULL DEFAULT '',
    epg_url TEXT NOT NULL DEFAULT '',
    epg_text TEXT NOT NULL DEFAULT '',
    xtream_base_url TEXT NOT NULL DEFAULT '',
    xtream_username TEXT NOT NULL DEFAULT '',
    xtream_password TEXT NOT NULL DEFAULT '',
    hdhomerun_base_url TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    stream_buffer_seconds INTEGER NOT NULL DEFAULT 18,
    max_retry_seconds INTEGER NOT NULL DEFAULT 45,
    refresh_interval_hours INTEGER NOT NULL DEFAULT 12,
    filter_categories TEXT NOT NULL DEFAULT '[]',
    filter_countries TEXT NOT NULL DEFAULT '[]',
    filter_require_epg INTEGER NOT NULL DEFAULT 0,
    keyword_allow TEXT NOT NULL DEFAULT '[]',
    keyword_deny TEXT NOT NULL DEFAULT '[]',
    sort_order INTEGER NOT NULL DEFAULT 100,
    channel_count INTEGER NOT NULL DEFAULT 0,
    program_count INTEGER NOT NULL DEFAULT 0,
    logo_count INTEGER NOT NULL DEFAULT 0,
    hidden_channel_count INTEGER NOT NULL DEFAULT 0,
    favorite_channel_count INTEGER NOT NULL DEFAULT 0,
    summary_updated_at TEXT NOT NULL DEFAULT '',
    active_import_generation TEXT NOT NULL DEFAULT '',
    last_refreshed_at TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, tuner_count INTEGER NOT NULL DEFAULT 1 CHECK (tuner_count BETWEEN 1 AND 64), discovered_tuner_count INTEGER NOT NULL DEFAULT 0 CHECK (discovered_tuner_count BETWEEN 0 AND 64), tuner_count_mode TEXT NOT NULL DEFAULT 'default' CHECK (tuner_count_mode IN ('default', 'discovered', 'overridden')));
CREATE TRIGGER bind_live_tv_recording_rule_guide_generation
AFTER INSERT ON live_tv_recording_rules
WHEN NEW.guide_generation = ''
BEGIN
    UPDATE live_tv_recording_rules
    SET guide_generation = COALESCE((SELECT active_import_generation FROM live_tv_sources WHERE id = NEW.source_id), '')
    WHERE id = NEW.id AND guide_generation = '';
END;
CREATE TRIGGER rebind_live_tv_recording_rule_source_generation
AFTER UPDATE OF source_id ON live_tv_recording_rules
WHEN NEW.source_id <> OLD.source_id
BEGIN
    UPDATE live_tv_recording_rules
    SET guide_generation = COALESCE((SELECT active_import_generation FROM live_tv_sources WHERE id = NEW.source_id), '')
    WHERE id = NEW.id;
END;
CREATE TRIGGER bind_live_tv_recording_guide_generation
AFTER INSERT ON live_tv_recordings
WHEN NEW.guide_generation = ''
BEGIN
    UPDATE live_tv_recordings
    SET guide_generation = COALESCE((SELECT active_import_generation FROM live_tv_sources WHERE id = NEW.source_id), '')
    WHERE id = NEW.id AND guide_generation = '';
END;
CREATE TRIGGER rebind_live_tv_recording_source_generation
AFTER UPDATE OF source_id ON live_tv_recordings
WHEN NEW.source_id <> OLD.source_id
BEGIN
    UPDATE live_tv_recordings
    SET guide_generation = COALESCE((SELECT active_import_generation FROM live_tv_sources WHERE id = NEW.source_id), '')
    WHERE id = NEW.id;
END;
CREATE TABLE live_tv_tuner_allocations (
    id TEXT PRIMARY KEY,
    source_id TEXT NOT NULL REFERENCES live_tv_sources(id) ON DELETE CASCADE,
    channel_id TEXT NOT NULL DEFAULT '',
    allocation_kind TEXT NOT NULL CHECK (allocation_kind IN ('live_session', 'dvr_recording')),
    consumer_id TEXT NOT NULL,
    allocation_key TEXT NOT NULL,
    lease_token TEXT NOT NULL,
    acquired_at TEXT NOT NULL,
    heartbeat_at TEXT NOT NULL,
    expires_at TEXT NOT NULL DEFAULT '',
    UNIQUE(allocation_kind, consumer_id)
);
CREATE TABLE local_credentials (
			id TEXT PRIMARY KEY,
			profile_identity_id TEXT NOT NULL REFERENCES profile_identities(id) ON DELETE CASCADE,
			password_hash TEXT NOT NULL,
			recovery_enabled INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			revoked_at TEXT NOT NULL DEFAULT ''
		);
CREATE TABLE local_profile_admin_proofs (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    primary_profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    pin_revision INTEGER NOT NULL CHECK (pin_revision >= 0),
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL
);
CREATE TABLE local_profile_pin_credentials (
    profile_id TEXT PRIMARY KEY REFERENCES profiles(id) ON DELETE CASCADE,
    pin_hash TEXT NOT NULL CHECK (length(pin_hash) = 60 AND pin_hash GLOB '$2[aby]$10$*'),
    failed_attempts INTEGER NOT NULL DEFAULT 0 CHECK (failed_attempts >= 0),
    locked_until TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, next_attempt_at TEXT NOT NULL DEFAULT '');
CREATE TABLE localization_options (
			kind TEXT NOT NULL,
			id TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			labels_json TEXT NOT NULL DEFAULT '{}',
			sort_order INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (kind, id)
		);
CREATE TABLE localization_rating_systems (
			country TEXT NOT NULL,
			system TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			labels_json TEXT NOT NULL DEFAULT '{}',
			sort_order INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (country, system)
		);
CREATE TABLE localization_rating_values (
			country TEXT NOT NULL,
			system TEXT NOT NULL,
			rating TEXT NOT NULL,
			label TEXT NOT NULL DEFAULT '',
			labels_json TEXT NOT NULL DEFAULT '{}',
			rank INTEGER NOT NULL DEFAULT 0,
			minimum_age INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (country, system, rating),
			FOREIGN KEY (country, system) REFERENCES localization_rating_systems(country, system) ON DELETE CASCADE
		);
CREATE TABLE media_access_labels (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    normalized_label TEXT NOT NULL,
    PRIMARY KEY (media_id, normalized_label)
);
CREATE TABLE media_access_tags (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    normalized_tag TEXT NOT NULL,
    PRIMARY KEY (media_id, normalized_tag)
);
CREATE TABLE media_attachments (
			id TEXT PRIMARY KEY,
			media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
			stream_id TEXT NOT NULL DEFAULT '',
			filename TEXT NOT NULL DEFAULT '',
			mime_type TEXT NOT NULL DEFAULT '',
			codec TEXT NOT NULL DEFAULT '',
			path TEXT NOT NULL DEFAULT '',
			size_bytes INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			UNIQUE(media_id, stream_id, filename)
		);
CREATE TABLE media_availability (
			media_id TEXT PRIMARY KEY REFERENCES media_items(id) ON DELETE CASCADE,
			file_count INTEGER NOT NULL DEFAULT 0,
			available_file_count INTEGER NOT NULL DEFAULT 0,
			missing_file_count INTEGER NOT NULL DEFAULT 0,
			has_local_source INTEGER NOT NULL DEFAULT 0,
			has_remote_source INTEGER NOT NULL DEFAULT 0,
			has_hdr_source INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL DEFAULT ''
		);
CREATE TABLE media_category_facets (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    library_id TEXT NOT NULL DEFAULT '',
    facet_type TEXT NOT NULL,
    value TEXT NOT NULL,
    sort_value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT '',
    metadata_revision INTEGER NOT NULL DEFAULT 0 CHECK (metadata_revision >= 0),
    source TEXT NOT NULL DEFAULT '',
    ordinal INTEGER NOT NULL DEFAULT 0 CHECK (ordinal >= 0),
    PRIMARY KEY (media_id, facet_type, sort_value)
);
CREATE TABLE media_chapters (
			id TEXT PRIMARY KEY,
			media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
			title TEXT NOT NULL DEFAULT '',
			start_seconds INTEGER NOT NULL DEFAULT 0,
			end_seconds INTEGER NOT NULL DEFAULT 0,
			sort_order INTEGER NOT NULL DEFAULT 0
		);
CREATE TABLE media_download_grants (
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
, profile_id TEXT NOT NULL DEFAULT '', authorization_revision TEXT NOT NULL DEFAULT '', preparation_id TEXT NOT NULL DEFAULT '');
CREATE TABLE media_files (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    quality TEXT NOT NULL DEFAULT '',
    container TEXT NOT NULL DEFAULT '',
    version_label TEXT NOT NULL DEFAULT '',
    resolution TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    source_type TEXT NOT NULL DEFAULT 'local',
    video_codec TEXT NOT NULL DEFAULT '',
    audio_codec TEXT NOT NULL DEFAULT '',
    dynamic_range TEXT NOT NULL DEFAULT '',
    release_group TEXT NOT NULL DEFAULT '',
    three_d INTEGER NOT NULL DEFAULT 0,
    version_group TEXT NOT NULL DEFAULT '',
    quality_rank INTEGER NOT NULL DEFAULT 0,
    size_bytes INTEGER NOT NULL DEFAULT 0,
    mod_time TEXT NOT NULL DEFAULT '',
    available INTEGER NOT NULL DEFAULT 1,
    missing_since TEXT NOT NULL DEFAULT '',
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    scan_generation TEXT NOT NULL DEFAULT ''
, content_fingerprint TEXT NOT NULL DEFAULT '', identity_evidence TEXT NOT NULL DEFAULT '', "aspect_ratio" TEXT NOT NULL DEFAULT '', "audio_bitrate" INTEGER NOT NULL DEFAULT 0, "audio_channel_layout" TEXT NOT NULL DEFAULT '', "audio_channels" INTEGER NOT NULL DEFAULT 0, "audio_sample_rate" INTEGER NOT NULL DEFAULT 0, "bit_depth" INTEGER NOT NULL DEFAULT 0, "bitrate" INTEGER NOT NULL DEFAULT 0, "chroma_location" TEXT NOT NULL DEFAULT '', "color_primaries" TEXT NOT NULL DEFAULT '', "color_space" TEXT NOT NULL DEFAULT '', "color_transfer" TEXT NOT NULL DEFAULT '', "duration_seconds" INTEGER NOT NULL DEFAULT 0, "frame_rate" REAL NOT NULL DEFAULT 0, "height" INTEGER NOT NULL DEFAULT 0, "pixel_format" TEXT NOT NULL DEFAULT '', "video_level" INTEGER NOT NULL DEFAULT 0, "video_profile" TEXT NOT NULL DEFAULT '', "width" INTEGER NOT NULL DEFAULT 0, directory_path TEXT NOT NULL DEFAULT '');
CREATE TABLE media_identity_evidence (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    field TEXT NOT NULL,
    value TEXT NOT NULL DEFAULT '',
    confidence REAL NOT NULL DEFAULT 0,
    path TEXT NOT NULL DEFAULT '',
    raw_json TEXT NOT NULL DEFAULT '{}',
    updated_at TEXT NOT NULL,
    UNIQUE(media_id, source, field, value, path)
);
CREATE TABLE media_images (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    image_type TEXT NOT NULL,
    source TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '',
    remote_url TEXT NOT NULL DEFAULT '',
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    language TEXT NOT NULL DEFAULT '',
    rating REAL NOT NULL DEFAULT 0,
    preferred INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
, sort_order INTEGER NOT NULL DEFAULT 0, discovery_scope TEXT NOT NULL DEFAULT '', provider_image_id TEXT NOT NULL DEFAULT '', content_hash TEXT NOT NULL DEFAULT '', selection_state TEXT NOT NULL DEFAULT 'accepted' CHECK (selection_state IN ('candidate', 'accepted', 'rejected', 'superseded')), confidence REAL NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 1), observed_at TEXT NOT NULL DEFAULT '', provider_updated_at TEXT NOT NULL DEFAULT '', snapshot_id TEXT NOT NULL DEFAULT '', metadata_revision INTEGER NOT NULL DEFAULT 0 CHECK (metadata_revision >= 0));
CREATE TABLE media_items (
    id TEXT PRIMARY KEY,
    library_id TEXT REFERENCES libraries(id) ON DELETE SET NULL,
    parent_id TEXT REFERENCES media_items(id) ON DELETE CASCADE,
    type TEXT NOT NULL,
    title TEXT NOT NULL,
    sort_title TEXT NOT NULL,
    original_title TEXT NOT NULL DEFAULT '',
    edition TEXT NOT NULL DEFAULT '',
    year INTEGER NOT NULL DEFAULT 0,
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    summary TEXT NOT NULL DEFAULT '',
    tagline TEXT NOT NULL DEFAULT '',
    content_rating TEXT NOT NULL DEFAULT '',
    community_rating REAL NOT NULL DEFAULT 0,
    critic_rating INTEGER NOT NULL DEFAULT 0,
    studio TEXT NOT NULL DEFAULT '',
    network TEXT NOT NULL DEFAULT '',
    country TEXT NOT NULL DEFAULT '',
    genres_json TEXT NOT NULL DEFAULT '[]',
    tags_json TEXT NOT NULL DEFAULT '[]',
    labels_json TEXT NOT NULL DEFAULT '[]',
    added_at TEXT NOT NULL,
    metadata_refreshed_at TEXT NOT NULL DEFAULT '',
    season_number INTEGER NOT NULL DEFAULT 0,
    episode_number INTEGER NOT NULL DEFAULT 0,
    index_number INTEGER NOT NULL DEFAULT 0,
    art_seed TEXT NOT NULL DEFAULT '',
    artwork_json TEXT NOT NULL DEFAULT '{}',
    source_url TEXT NOT NULL DEFAULT '',
    typed_metadata_json TEXT NOT NULL DEFAULT '{}',
    random_key TEXT NOT NULL DEFAULT '',
    metadata_revision INTEGER NOT NULL DEFAULT 0 CHECK (metadata_revision >= 0),
    metadata_etag TEXT NOT NULL DEFAULT ''
, sort_artist_key TEXT NOT NULL DEFAULT '', sort_album_artist_key TEXT NOT NULL DEFAULT '', sort_track_artist_key TEXT NOT NULL DEFAULT '', sort_author_key TEXT NOT NULL DEFAULT '', sort_narrator_key TEXT NOT NULL DEFAULT '', sort_series_key TEXT NOT NULL DEFAULT '', sort_label_key TEXT NOT NULL DEFAULT '', sort_network_key TEXT NOT NULL DEFAULT '', sort_studio_key TEXT NOT NULL DEFAULT '', filter_artist_key TEXT NOT NULL DEFAULT '', filter_album_artist_key TEXT NOT NULL DEFAULT '', filter_track_artist_key TEXT NOT NULL DEFAULT '', filter_author_key TEXT NOT NULL DEFAULT '', filter_narrator_key TEXT NOT NULL DEFAULT '', filter_series_key TEXT NOT NULL DEFAULT '', filter_label_key TEXT NOT NULL DEFAULT '', filter_network_key TEXT NOT NULL DEFAULT '', filter_studio_key TEXT NOT NULL DEFAULT '');
CREATE TABLE media_lyrics (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    source TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    format TEXT NOT NULL DEFAULT '',
    language TEXT NOT NULL DEFAULT '',
    path TEXT NOT NULL DEFAULT '',
    text TEXT NOT NULL DEFAULT '',
    synced INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    UNIQUE(media_id, source, path, language, format)
);
CREATE TABLE media_match_candidates (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    external_id TEXT NOT NULL DEFAULT '',
    external_type TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    score REAL NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'candidate' CHECK (status IN ('candidate', 'accepted', 'rejected', 'superseded', 'expired')),
    revision_id TEXT REFERENCES media_metadata_revisions(id) ON DELETE SET NULL,
    snapshot_id TEXT REFERENCES media_provider_snapshots(id) ON DELETE SET NULL,
    reason_codes_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(reason_codes_json) AND length(reason_codes_json) <= 65536),
    raw_query TEXT NOT NULL DEFAULT '' CHECK (length(raw_query) <= 4096),
    raw_result_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(raw_result_json) AND length(raw_result_json) <= 262144),
    payload_digest TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE TABLE media_metadata_locks (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    field TEXT NOT NULL,
    value_json TEXT NOT NULL DEFAULT 'null',
    source TEXT NOT NULL DEFAULT '',
    user_id TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    lock_kind TEXT NOT NULL DEFAULT 'scalar' CHECK (lock_kind IN ('scalar', 'relationship', 'artwork', 'credit', 'identity')),
    value_hash TEXT NOT NULL DEFAULT '',
    metadata_revision INTEGER NOT NULL DEFAULT 0 CHECK (metadata_revision >= 0),
    PRIMARY KEY(media_id, field)
);
CREATE TABLE media_people (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    role TEXT NOT NULL DEFAULT '',
    source TEXT NOT NULL DEFAULT '',
    sort_order INTEGER NOT NULL DEFAULT 0,
    provider_ids_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL, canonical_person_key TEXT NOT NULL DEFAULT '', character TEXT NOT NULL DEFAULT '', image_url TEXT NOT NULL DEFAULT '', provider_person_id TEXT NOT NULL DEFAULT '', confidence REAL NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 1), observed_at TEXT NOT NULL DEFAULT '', snapshot_id TEXT NOT NULL DEFAULT '', metadata_revision INTEGER NOT NULL DEFAULT 0 CHECK (metadata_revision >= 0),
    UNIQUE(media_id, name, role, source, sort_order)
);
CREATE TABLE media_provider_ids (
			media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			external_id TEXT NOT NULL,
			external_type TEXT NOT NULL DEFAULT '',
			confidence REAL NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 1),
			source TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'accepted' CHECK (status IN ('candidate', 'accepted', 'rejected', 'superseded')),
			observed_at TEXT NOT NULL DEFAULT '',
			provider_updated_at TEXT NOT NULL DEFAULT '',
			snapshot_id TEXT NOT NULL DEFAULT '',
			evidence_revision INTEGER NOT NULL DEFAULT 0 CHECK (evidence_revision >= 0),
			accepted_at TEXT NOT NULL DEFAULT '',
			accepted_by_user_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			updated_at TEXT NOT NULL,
			PRIMARY KEY (media_id, provider, external_type, external_id)
		);
CREATE TABLE media_metadata_revisions (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    revision INTEGER NOT NULL CHECK (revision > 0),
    base_revision INTEGER NOT NULL CHECK (base_revision >= 0),
    state TEXT NOT NULL CHECK (state IN ('staged', 'applied', 'rejected', 'failed')),
    trigger_kind TEXT NOT NULL CHECK (trigger_kind IN ('manual', 'provider', 'scanner', 'local', 'embedded', 'repair', 'system')),
    actor_user_id TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    external_type TEXT NOT NULL DEFAULT '',
    external_id TEXT NOT NULL DEFAULT '',
    locale TEXT NOT NULL DEFAULT '',
    started_at TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT '',
    error_code TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '' CHECK (length(detail) <= 2000),
    UNIQUE(media_id, revision)
);
CREATE TABLE media_provider_snapshots (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    revision_id TEXT NOT NULL REFERENCES media_metadata_revisions(id) ON DELETE CASCADE,
    provider TEXT NOT NULL,
    external_type TEXT NOT NULL DEFAULT '',
    external_id TEXT NOT NULL,
    locale TEXT NOT NULL DEFAULT '',
    schema_version INTEGER NOT NULL CHECK (schema_version > 0),
    mapping_version TEXT NOT NULL CHECK (length(trim(mapping_version)) > 0 AND length(mapping_version) <= 128),
    payload_json TEXT NOT NULL CHECK (json_valid(payload_json) AND length(payload_json) <= 2097152),
    payload_sha256 TEXT NOT NULL CHECK (length(payload_sha256) = 64),
    byte_length INTEGER NOT NULL CHECK (byte_length >= 0 AND byte_length <= 2097152),
    source_payload_sha256 TEXT NOT NULL CHECK (length(source_payload_sha256) = 64),
    source_byte_length INTEGER NOT NULL CHECK (source_byte_length >= 0),
    truncated INTEGER NOT NULL DEFAULT 0 CHECK (truncated IN (0, 1)),
    result_status TEXT NOT NULL CHECK (result_status IN ('ok', 'not_found', 'degraded', 'error')),
    fetched_at TEXT NOT NULL,
    provider_updated_at TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL DEFAULT '',
    UNIQUE(revision_id, provider, external_type, external_id, locale)
);
CREATE TABLE media_metadata_field_values (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    revision_id TEXT NOT NULL REFERENCES media_metadata_revisions(id) ON DELETE CASCADE,
    field_key TEXT NOT NULL,
    ordinal INTEGER NOT NULL DEFAULT 0 CHECK (ordinal >= 0),
    locale TEXT NOT NULL DEFAULT '',
    value_json TEXT NOT NULL CHECK (json_valid(value_json) AND length(value_json) <= 65536),
    normalized_value TEXT NOT NULL DEFAULT '',
    source_kind TEXT NOT NULL CHECK (source_kind IN ('manual', 'provider', 'embedded', 'nfo', 'file', 'scanner', 'system')),
    provider TEXT NOT NULL DEFAULT '',
    snapshot_id TEXT REFERENCES media_provider_snapshots(id) ON DELETE SET NULL,
    confidence REAL NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 1),
    decision TEXT NOT NULL CHECK (decision IN ('candidate', 'accepted', 'rejected', 'locked')),
    reason_code TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    UNIQUE(revision_id, field_key, ordinal, locale, source_kind, provider)
);
CREATE TABLE media_metadata_relationships (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    revision_id TEXT NOT NULL REFERENCES media_metadata_revisions(id) ON DELETE CASCADE,
    relationship_type TEXT NOT NULL CHECK (relationship_type IN ('alternate_title', 'language', 'country', 'keyword', 'company', 'studio', 'network', 'creator', 'person', 'character', 'collection', 'franchise', 'certification', 'genre', 'tag', 'status', 'external_id', 'release', 'track', 'recording', 'work', 'related_media', 'artwork', 'provider_coverage', 'label', 'medium', 'format')),
    target_kind TEXT NOT NULL DEFAULT '',
    target_key TEXT NOT NULL,
    display_value TEXT NOT NULL,
    target_provider TEXT NOT NULL DEFAULT '',
    target_external_id TEXT NOT NULL DEFAULT '',
    language TEXT NOT NULL DEFAULT '',
    country TEXT NOT NULL DEFAULT '',
    role TEXT NOT NULL DEFAULT '',
    ordinal INTEGER NOT NULL DEFAULT 0 CHECK (ordinal >= 0),
    attributes_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(attributes_json) AND length(attributes_json) <= 65536),
    source_kind TEXT NOT NULL CHECK (source_kind IN ('manual', 'provider', 'embedded', 'nfo', 'file', 'scanner', 'system')),
    source TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    snapshot_id TEXT REFERENCES media_provider_snapshots(id) ON DELETE SET NULL,
    confidence REAL NOT NULL DEFAULT 0 CHECK (confidence >= 0 AND confidence <= 1),
    decision TEXT NOT NULL CHECK (decision IN ('candidate', 'accepted', 'rejected', 'locked')),
    reason_code TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    UNIQUE(revision_id, relationship_type, target_key, source_kind, provider, language, country, role)
);
CREATE TABLE media_metadata_refresh_outcomes (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    revision_id TEXT REFERENCES media_metadata_revisions(id) ON DELETE SET NULL,
    operation_id TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    expected_revision INTEGER NOT NULL CHECK (expected_revision >= 0),
    resulting_revision INTEGER NOT NULL CHECK (resulting_revision >= 0),
    status TEXT NOT NULL CHECK (status IN ('applied', 'conflict', 'failed', 'skipped')),
    error_code TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '' CHECK (length(detail) <= 2000),
    created_at TEXT NOT NULL
);
CREATE TABLE media_rating_evidence (
			media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			country TEXT NOT NULL DEFAULT '',
			rating_system TEXT NOT NULL DEFAULT '',
			raw_rating TEXT NOT NULL DEFAULT '',
			normalized_rating TEXT NOT NULL DEFAULT '',
			normalized_rank INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (media_id, provider, source)
		);
CREATE TABLE media_scanner_hints (
			media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
			source TEXT NOT NULL,
			title TEXT NOT NULL DEFAULT '',
			year INTEGER NOT NULL DEFAULT 0,
			raw_json TEXT NOT NULL DEFAULT '{}',
			updated_at TEXT NOT NULL,
			PRIMARY KEY (media_id, source)
		);
CREATE TABLE media_scanner_identity_aliases (
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    scanner_key TEXT NOT NULL,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED,
    first_seen_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    PRIMARY KEY (library_id, scanner_key)
);
CREATE VIRTUAL TABLE media_search USING fts5(
    media_id UNINDEXED,
    title,
    summary,
    genres,
    alternate_titles,
    people,
    relationships,
    identifiers,
    keywords
);
CREATE TABLE media_segments (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    segment_type TEXT NOT NULL DEFAULT '',
    start_seconds INTEGER NOT NULL DEFAULT 0,
    end_seconds INTEGER NOT NULL DEFAULT 0,
    source TEXT NOT NULL DEFAULT '',
    provider TEXT NOT NULL DEFAULT '',
    confidence REAL NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL, automatic_safe INTEGER NOT NULL DEFAULT 0,
    UNIQUE(media_id, segment_type, start_seconds, end_seconds, source, provider)
);
CREATE TABLE media_streams (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    codec TEXT NOT NULL,
    language TEXT NOT NULL DEFAULT '',
    channels INTEGER NOT NULL DEFAULT 0,
    bitrate INTEGER NOT NULL DEFAULT 0,
    width INTEGER NOT NULL DEFAULT 0,
    height INTEGER NOT NULL DEFAULT 0,
    display_title TEXT NOT NULL,
    source_url TEXT NOT NULL DEFAULT '',
    subtitle_offset_ms INTEGER NOT NULL DEFAULT 0,
    profile TEXT NOT NULL DEFAULT '',
    level INTEGER NOT NULL DEFAULT 0,
    pixel_format TEXT NOT NULL DEFAULT '',
    bit_depth INTEGER NOT NULL DEFAULT 0,
    color_transfer TEXT NOT NULL DEFAULT '',
    color_primaries TEXT NOT NULL DEFAULT '',
    color_space TEXT NOT NULL DEFAULT '',
    chroma_location TEXT NOT NULL DEFAULT '',
    field_order TEXT NOT NULL DEFAULT '',
    dynamic_range TEXT NOT NULL DEFAULT '',
    dolby_vision_profile TEXT NOT NULL DEFAULT ''
, exact_seek_safe INTEGER NOT NULL DEFAULT 0
    CHECK (exact_seek_safe IN (0, 1)), keyframe_evidence_at TEXT NOT NULL DEFAULT '', "aspect_ratio" TEXT NOT NULL DEFAULT '', "channel_layout" TEXT NOT NULL DEFAULT '', "file_id" TEXT NOT NULL DEFAULT '', "frame_rate" REAL NOT NULL DEFAULT 0, "hearing_impaired" INTEGER NOT NULL DEFAULT 0, "is_default" INTEGER NOT NULL DEFAULT 0, "is_forced" INTEGER NOT NULL DEFAULT 0, "sample_rate" INTEGER NOT NULL DEFAULT 0, "source_identity" TEXT NOT NULL DEFAULT '', "source_kind" TEXT NOT NULL DEFAULT 'unknown', "storage_key" TEXT NOT NULL DEFAULT '', "stream_index" INTEGER NOT NULL DEFAULT 0);
CREATE TABLE media_analysis_facts (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    media_file_id TEXT NOT NULL DEFAULT '',
    schema_version INTEGER NOT NULL CHECK (schema_version > 0),
    source_revision TEXT NOT NULL CHECK (length(trim(source_revision)) > 0),
    source_fingerprint TEXT NOT NULL CHECK (length(trim(source_fingerprint)) > 0),
    facts_digest TEXT NOT NULL CHECK (length(trim(facts_digest)) > 0),
    facts_json TEXT NOT NULL CHECK (json_valid(facts_json) AND json_type(facts_json) = 'object' AND length(facts_json) <= 4194304),
    analyzed_at TEXT NOT NULL,
    PRIMARY KEY (media_id, media_file_id)
);
CREATE UNIQUE INDEX idx_media_analysis_facts_digest ON media_analysis_facts(media_id, facts_digest);
CREATE TABLE media_trickplay_sets (
				id TEXT PRIMARY KEY,
				media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
				media_file_id TEXT NOT NULL DEFAULT '',
				width INTEGER NOT NULL DEFAULT 0,
				height INTEGER NOT NULL DEFAULT 0,
				tile_width INTEGER NOT NULL DEFAULT 0,
				tile_height INTEGER NOT NULL DEFAULT 0,
				interval_seconds INTEGER NOT NULL DEFAULT 0,
				duration_seconds INTEGER NOT NULL DEFAULT 0,
				tile_count INTEGER NOT NULL DEFAULT 0,
				path TEXT NOT NULL DEFAULT '',
				stale INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL,
				UNIQUE(media_id, media_file_id, width, height, interval_seconds)
			);
CREATE TABLE media_trickplay_tiles (
				id TEXT PRIMARY KEY,
				set_id TEXT NOT NULL REFERENCES media_trickplay_sets(id) ON DELETE CASCADE,
				tile_index INTEGER NOT NULL DEFAULT 0,
				start_seconds INTEGER NOT NULL DEFAULT 0,
				end_seconds INTEGER NOT NULL DEFAULT 0,
				row INTEGER NOT NULL DEFAULT 0,
				col INTEGER NOT NULL DEFAULT 0,
				path TEXT NOT NULL DEFAULT '',
				created_at TEXT NOT NULL,
				UNIQUE(set_id, tile_index)
			);
CREATE TABLE metadata_health_issues (
    id TEXT PRIMARY KEY,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    title TEXT NOT NULL DEFAULT '',
    type TEXT NOT NULL DEFAULT '',
    library_id TEXT NOT NULL DEFAULT '',
    library_name TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL,
    severity TEXT NOT NULL DEFAULT 'info',
    reason TEXT NOT NULL DEFAULT '',
    detail TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL DEFAULT '',
    issue_updated_at TEXT NOT NULL DEFAULT '',
    generated_at TEXT NOT NULL
);
CREATE TABLE native_auth_exchange_receipts (
    kind TEXT NOT NULL,
    proof_hash TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    native_refresh_token_id TEXT NOT NULL REFERENCES native_refresh_tokens(id) ON DELETE CASCADE,
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    PRIMARY KEY (kind, proof_hash)
);
CREATE TABLE native_refresh_tokens (
    id TEXT PRIMARY KEY,
    family_id TEXT NOT NULL,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    auth_provider TEXT NOT NULL DEFAULT 'local',
    token_hash TEXT NOT NULL UNIQUE,
    replaced_by_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT '',
    revoked_at TEXT NOT NULL DEFAULT ''
, profile_id TEXT NOT NULL DEFAULT '', rotation_key_hash TEXT NOT NULL DEFAULT '');
CREATE TABLE optimized_versions (
			id TEXT PRIMARY KEY,
			media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
			profile TEXT NOT NULL,
			path TEXT NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			container TEXT NOT NULL DEFAULT '',
			video_codec TEXT NOT NULL DEFAULT '',
			audio_codec TEXT NOT NULL DEFAULT '',
			width INTEGER NOT NULL DEFAULT 0,
			height INTEGER NOT NULL DEFAULT 0,
			bitrate INTEGER NOT NULL DEFAULT 0,
			duration_seconds INTEGER NOT NULL DEFAULT 0,
            state TEXT NOT NULL DEFAULT 'ready' CHECK (state IN ('staging', 'validating', 'ready', 'superseded', 'failed', 'deleting')),
            preset_version INTEGER NOT NULL DEFAULT 0 CHECK (preset_version >= 0),
            planner_revision TEXT NOT NULL DEFAULT '',
            source_revision TEXT NOT NULL DEFAULT '',
            source_fingerprint TEXT NOT NULL DEFAULT '',
            source_facts_digest TEXT NOT NULL DEFAULT '',
            plan_digest TEXT NOT NULL DEFAULT '',
            plan_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(plan_json) AND json_type(plan_json) = 'object'),
            output_facts_digest TEXT NOT NULL DEFAULT '',
            output_facts_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(output_facts_json) AND json_type(output_facts_json) = 'object'),
            compatibility_tags_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(compatibility_tags_json) AND json_type(compatibility_tags_json) = 'array'),
            superseded_at TEXT NOT NULL DEFAULT ''
		);
CREATE TABLE person_public_ids (
    public_id TEXT PRIMARY KEY,
    identity_key TEXT NOT NULL UNIQUE,
    created_at TEXT NOT NULL
);
CREATE TABLE playback_media_grants (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    playback_session_id TEXT NOT NULL REFERENCES playback_sessions(id) ON DELETE CASCADE,
    principal_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    resource_kind TEXT NOT NULL CHECK (resource_kind IN ('media', 'live_channel')),
    resource_id TEXT NOT NULL,
    operation_classes_json TEXT NOT NULL,
    issued_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    last_authorized_at TEXT NOT NULL DEFAULT '',
    revoked_at TEXT NOT NULL DEFAULT ''
, profile_id TEXT NOT NULL DEFAULT '', authorization_revision TEXT NOT NULL DEFAULT '', delivery_mode TEXT NOT NULL DEFAULT '', transcode_quality TEXT NOT NULL DEFAULT '', allowed_qualities_json TEXT NOT NULL DEFAULT '[]', plan_digest TEXT NOT NULL DEFAULT '', plan_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(plan_json) AND json_type(plan_json) = 'object'), source_revision TEXT NOT NULL DEFAULT '', playback_generation INTEGER NOT NULL DEFAULT 0 CHECK (playback_generation >= 0));
CREATE TABLE playback_prepared_handoffs (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    source_session_id TEXT NOT NULL REFERENCES playback_sessions(id) ON DELETE CASCADE,
    client_instance_id TEXT NOT NULL DEFAULT '',
    media_id TEXT NOT NULL,
    queue_media_ids_json TEXT NOT NULL DEFAULT '[]',
    source_context_json TEXT NOT NULL DEFAULT '{}',
    queue_revision INTEGER NOT NULL DEFAULT 0,
    playback_revision INTEGER NOT NULL DEFAULT 0,
    state TEXT NOT NULL DEFAULT 'prepared' CHECK (state IN ('prepared', 'committing', 'committed')),
    request_id TEXT NOT NULL DEFAULT '',
    request_fingerprint TEXT NOT NULL DEFAULT '',
    committed_response TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE TABLE playback_receivers (
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
, profile_id TEXT NOT NULL DEFAULT '');
CREATE TABLE playback_session_continuation_credentials (
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
, last_rotation_receipt TEXT NOT NULL DEFAULT '');
CREATE TABLE playback_session_history (
			session_id TEXT NOT NULL REFERENCES playback_sessions(id) ON DELETE CASCADE,
			media_id TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			played_at TEXT NOT NULL,
			PRIMARY KEY (session_id, media_id, sort_order)
		);
CREATE TABLE playback_session_queue (
			session_id TEXT NOT NULL REFERENCES playback_sessions(id) ON DELETE CASCADE,
			media_id TEXT NOT NULL,
			sort_order INTEGER NOT NULL DEFAULT 0,
			added_at TEXT NOT NULL,
			PRIMARY KEY (session_id, media_id)
		);
CREATE TABLE playback_sessions (
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
, profile_id TEXT NOT NULL DEFAULT '', selected_quality_id TEXT NOT NULL DEFAULT '', selected_audio_stream_id TEXT NOT NULL DEFAULT '', selected_subtitle_stream_id TEXT NOT NULL DEFAULT '', selected_subtitle_mode TEXT NOT NULL DEFAULT 'off', selected_version_id TEXT NOT NULL DEFAULT '', progress_authority TEXT NOT NULL DEFAULT 'sender', progress_generation INTEGER NOT NULL DEFAULT 1, renegotiation_revision INTEGER NOT NULL DEFAULT 0, last_renegotiation_request_id TEXT NOT NULL DEFAULT '', last_renegotiation_fingerprint TEXT NOT NULL DEFAULT '', client_profile_json TEXT NOT NULL DEFAULT '{}', playback_intent_json TEXT NOT NULL DEFAULT '{}', command_json TEXT NOT NULL DEFAULT '{}', command_updated_at TEXT NOT NULL DEFAULT '', plan_schema_version INTEGER NOT NULL DEFAULT 0 CHECK (plan_schema_version >= 0), plan_digest TEXT NOT NULL DEFAULT '', plan_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(plan_json) AND json_type(plan_json) = 'object'), source_revision TEXT NOT NULL DEFAULT '', media_facts_digest TEXT NOT NULL DEFAULT '', capability_evidence_id TEXT NOT NULL DEFAULT '', playback_generation INTEGER NOT NULL DEFAULT 0 CHECK (playback_generation >= 0));
CREATE TABLE "playlist_items" (
    entry_id TEXT PRIMARY KEY NOT NULL DEFAULT ('pentry_' || lower(hex(randomblob(16)))),
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    added_at TEXT NOT NULL
);
CREATE TABLE playlist_shares (
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    can_edit INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (playlist_id, user_id)
);
CREATE TABLE playlists (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL DEFAULT 'playlist',
    title TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    visibility TEXT NOT NULL DEFAULT 'private',
    smart_filter_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, profile_id TEXT NOT NULL DEFAULT '');
CREATE TABLE portico_login_requests (
    id TEXT PRIMARY KEY,
    state_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    return_url TEXT NOT NULL,
    server_id TEXT NOT NULL DEFAULT '',
    callback_url TEXT NOT NULL DEFAULT '',
    local_origin TEXT NOT NULL DEFAULT '',
    client_ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    remember_on_browser INTEGER NOT NULL DEFAULT 1
, installation_id TEXT NOT NULL DEFAULT '', exchange_result_json TEXT NOT NULL DEFAULT '');
CREATE TABLE profile_account_authentications (
    id TEXT PRIMARY KEY,
    token_hash TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    auth_provider TEXT NOT NULL CHECK (auth_provider IN ('local', 'portico')),
    purpose TEXT NOT NULL CHECK (purpose IN ('browser', 'native')),
    device_id TEXT NOT NULL CHECK (device_id <> ''),
    installation_id TEXT NOT NULL CHECK (installation_id <> ''),
    browser_binding_hash TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
);
CREATE TABLE profile_erasure_receipts (
    operation_id TEXT PRIMARY KEY,
    target_digest TEXT NOT NULL UNIQUE,
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    erased_at TEXT NOT NULL
);
CREATE TABLE profile_identities (
			id TEXT PRIMARY KEY,
			profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
			provider TEXT NOT NULL,
			subject TEXT NOT NULL,
			email TEXT NOT NULL DEFAULT '',
			display_name TEXT NOT NULL DEFAULT '',
			verified_at TEXT NOT NULL DEFAULT '',
			last_seen_at TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
CREATE TABLE profile_search_history (
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    normalized_query TEXT NOT NULL,
    query TEXT NOT NULL,
    use_count INTEGER NOT NULL DEFAULT 1,
    last_used_at TEXT NOT NULL,
    PRIMARY KEY (profile_id, normalized_query)
);
CREATE TABLE profile_selection_grants (
    id TEXT PRIMARY KEY,
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    auth_provider TEXT NOT NULL CHECK (auth_provider IN ('local', 'portico')),
    device_id TEXT NOT NULL DEFAULT '',
    installation_id TEXT NOT NULL DEFAULT '',
    pin_revision INTEGER NOT NULL DEFAULT 0 CHECK (pin_revision >= 0),
    expires_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL
, purpose TEXT NOT NULL DEFAULT 'native'
    CHECK (purpose IN ('browser', 'native')), account_authentication_id TEXT NOT NULL DEFAULT '', browser_binding_hash TEXT NOT NULL DEFAULT '');
CREATE TABLE profiles (
    id TEXT PRIMARY KEY,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL,
    permissions_json TEXT NOT NULL,
    preferences_json TEXT NOT NULL DEFAULT '{}',
    max_content_rating TEXT NOT NULL DEFAULT '',
    max_active_sessions INTEGER NOT NULL DEFAULT 0,
    remote_bitrate_limit_mbps INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, account_id TEXT NOT NULL DEFAULT '', origin TEXT NOT NULL DEFAULT 'local'
    CHECK (origin IN ('local', 'hosted')), external_profile_id TEXT NOT NULL DEFAULT '', is_primary INTEGER NOT NULL DEFAULT 0
    CHECK (is_primary IN (0, 1)), sort_order INTEGER NOT NULL DEFAULT 0, avatar_url TEXT NOT NULL DEFAULT '', restrictions_json TEXT NOT NULL DEFAULT '{}', pin_required INTEGER NOT NULL DEFAULT 0
    CHECK (pin_required IN (0, 1)), disabled_at TEXT NOT NULL DEFAULT '', pin_revision INTEGER NOT NULL DEFAULT 0
    CHECK (pin_revision >= 0), policy_updated_at TEXT NOT NULL DEFAULT '');
CREATE TABLE public_resource_identity_keys (
			resource_kind TEXT NOT NULL,
			identity_key TEXT NOT NULL,
			resource_id TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY (resource_kind, identity_key),
			UNIQUE (resource_kind, resource_id)
		);
CREATE TABLE quick_connect_requests (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL,
    secret_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL,
    user_id TEXT NOT NULL DEFAULT '',
    device_name TEXT NOT NULL DEFAULT '',
    app TEXT NOT NULL DEFAULT '',
    platform TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    client_ip TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    consumed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, installation_id TEXT NOT NULL DEFAULT '', native_refresh_token_id TEXT NOT NULL DEFAULT '');
CREATE TABLE remote_access_members (
    portico_membership_id TEXT PRIMARY KEY,
    portico_user_id TEXT NOT NULL,
    email TEXT NOT NULL,
    display_name TEXT NOT NULL,
    role TEXT NOT NULL,
    status TEXT NOT NULL,
    local_user_id TEXT NOT NULL DEFAULT '',
    last_synced_at TEXT NOT NULL
, permission_template_json TEXT NOT NULL DEFAULT '{}');
CREATE TABLE saved_views (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    pivot TEXT NOT NULL,
    query_json TEXT NOT NULL DEFAULT '',
    sort_json TEXT NOT NULL DEFAULT '[]',
    presentation_json TEXT NOT NULL DEFAULT '[]',
    is_pinned INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, profile_id TEXT NOT NULL DEFAULT '');
CREATE TABLE security_audit_chain_state (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    anchor_previous_hash TEXT NOT NULL DEFAULT '',
    head_sequence INTEGER NOT NULL DEFAULT 0,
    head_hash TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT ''
);
CREATE TABLE security_audit_events (
    sequence INTEGER PRIMARY KEY AUTOINCREMENT,
    id TEXT NOT NULL UNIQUE,
    previous_hash TEXT NOT NULL DEFAULT '',
    event_hash TEXT NOT NULL,
    actor_user_id TEXT NOT NULL DEFAULT '',
    actor_email TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL DEFAULT '',
    resource_id TEXT NOT NULL DEFAULT '',
    severity TEXT NOT NULL DEFAULT 'info',
    metadata_json TEXT NOT NULL DEFAULT '{}',
    client_ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    byte_size INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE TABLE server_diagnostic_events (
    id TEXT PRIMARY KEY,
    level TEXT NOT NULL,
    message TEXT NOT NULL,
    fields_json TEXT NOT NULL DEFAULT '{}',
    source TEXT NOT NULL DEFAULT 'server',
    byte_size INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL
);
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    device_id TEXT NOT NULL DEFAULT '',
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TEXT NOT NULL,
    created_at TEXT NOT NULL,
    last_seen_at TEXT NOT NULL
, browser_vault_id TEXT NOT NULL DEFAULT '', profile_id TEXT NOT NULL DEFAULT '', profile_identity_id TEXT NOT NULL DEFAULT '', auth_provider TEXT NOT NULL DEFAULT 'local');
CREATE TABLE settings (
    key TEXT PRIMARY KEY,
    value_json TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
CREATE TABLE settings_quarantine (
    id TEXT PRIMARY KEY,
    key TEXT NOT NULL,
    value_json TEXT NOT NULL,
    reason TEXT NOT NULL,
    quarantined_at TEXT NOT NULL
);
CREATE TABLE tmdb_trending_cache (
			media_type TEXT NOT NULL,
			time_window TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			fetched_at TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			PRIMARY KEY (media_type, time_window)
		);
CREATE TABLE tv_setup_sessions (
    id TEXT PRIMARY KEY,
    code TEXT NOT NULL,
    status TEXT NOT NULL,
    user_id TEXT NOT NULL DEFAULT '',
    device_public_key TEXT NOT NULL,
    grant_secret_hash TEXT NOT NULL DEFAULT '',
    encrypted_grant_json TEXT NOT NULL DEFAULT '',
    device_name TEXT NOT NULL DEFAULT '',
    platform TEXT NOT NULL DEFAULT '',
    app_version TEXT NOT NULL DEFAULT '',
    server_hint TEXT NOT NULL DEFAULT '',
    auth_mode_hint TEXT NOT NULL DEFAULT '',
    endpoint_url TEXT NOT NULL DEFAULT '',
    client_ip TEXT NOT NULL DEFAULT '',
    user_agent TEXT NOT NULL DEFAULT '',
    expires_at TEXT NOT NULL,
    redeemed_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, installation_id TEXT NOT NULL DEFAULT '', native_refresh_token_id TEXT NOT NULL DEFAULT '');
CREATE TABLE "user_display_preferences" (
				profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
				user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				client TEXT NOT NULL,
				view TEXT NOT NULL,
				preferences_json TEXT NOT NULL DEFAULT '{}',
				updated_at TEXT NOT NULL,
				PRIMARY KEY (profile_id, client, view)
			);
CREATE TABLE user_library_access (
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
			created_at TEXT NOT NULL,
			PRIMARY KEY (user_id, library_id)
		);
CREATE TABLE "user_library_navigation" (
				profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
				user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
				pinned INTEGER NOT NULL DEFAULT 1 CHECK (pinned IN (0, 1)),
				sort_order INTEGER NOT NULL DEFAULT 0,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL,
				PRIMARY KEY (profile_id, library_id)
			);
CREATE TABLE "user_media_state" (
				profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
				user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
				watchlisted INTEGER NOT NULL DEFAULT 0,
				favorite INTEGER NOT NULL DEFAULT 0,
				liked INTEGER NOT NULL DEFAULT 0,
				watched INTEGER NOT NULL DEFAULT 0,
				progress_seconds INTEGER NOT NULL DEFAULT 0,
				rating INTEGER NOT NULL DEFAULT 0,
				last_played_at TEXT,
				updated_at TEXT NOT NULL,
				progress_session_id TEXT NOT NULL DEFAULT '',
				progress_recorded_at TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (profile_id, media_id)
			);
CREATE TABLE "user_recommendation_cache" (
				profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
				user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
				rank INTEGER NOT NULL,
				score REAL NOT NULL DEFAULT 0,
				reason TEXT NOT NULL DEFAULT '',
				source TEXT NOT NULL DEFAULT '',
				generated_at TEXT NOT NULL,
				expires_at TEXT NOT NULL,
				PRIMARY KEY (profile_id, media_id)
			);
CREATE TABLE users (
    id TEXT PRIMARY KEY,
    username TEXT NOT NULL DEFAULT '',
    email TEXT NOT NULL UNIQUE,
    display_name TEXT NOT NULL,
    password_hash TEXT,
    role TEXT NOT NULL,
    auth_origin TEXT NOT NULL DEFAULT 'local',
    portico_user_id TEXT NOT NULL DEFAULT '',
    portico_membership_id TEXT NOT NULL DEFAULT '',
    permissions_json TEXT NOT NULL,
    preferences_json TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
, disabled_at TEXT NOT NULL DEFAULT '', allow_account_profiles INTEGER NOT NULL DEFAULT 1
    CHECK (allow_account_profiles IN (0, 1)), max_active_streams INTEGER NOT NULL DEFAULT 0
    CHECK (max_active_streams >= 0 AND max_active_streams <= 32), max_content_rating TEXT NOT NULL DEFAULT '', max_active_sessions INTEGER NOT NULL DEFAULT 0, remote_bitrate_limit_mbps INTEGER NOT NULL DEFAULT 0, profile_image_path TEXT NOT NULL DEFAULT '', profile_image_url TEXT NOT NULL DEFAULT '');
CREATE TABLE viewer_feedback (
    id TEXT PRIMARY KEY,
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    kind TEXT NOT NULL CHECK (kind IN ('general', 'playback', 'media', 'quality')),
    category TEXT NOT NULL CHECK (category IN (
        'wont-play', 'buffering', 'playback-stopped', 'wrong-video', 'wrong-audio',
        'wrong-subtitles', 'incorrect-media-information', 'higher-quality-request', 'other'
    )),
    message TEXT NOT NULL,
    media_id TEXT NOT NULL DEFAULT '',
    playback_session_id TEXT NOT NULL DEFAULT '',
    device_class TEXT NOT NULL CHECK (device_class IN ('web', 'mobile', 'television')),
    platform TEXT NOT NULL,
    app_version TEXT NOT NULL,
    error_category TEXT NOT NULL DEFAULT '',
    diagnostics_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(diagnostics_json) AND json_type(diagnostics_json) = 'object'),
    duplicate_hash TEXT NOT NULL,
    duplicate_count INTEGER NOT NULL DEFAULT 0 CHECK (duplicate_count >= 0),
    status TEXT NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'read', 'resolved', 'dismissed')),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    owner_response TEXT NOT NULL DEFAULT '',
    responded_by_account_id TEXT NOT NULL DEFAULT '',
    responded_at TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    expires_at TEXT NOT NULL
);
CREATE TABLE viewer_notification_receipts (
    notification_id TEXT NOT NULL REFERENCES viewer_notifications(id) ON DELETE CASCADE,
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id TEXT NOT NULL CHECK (server_id <> ''),
    profile_id TEXT NOT NULL DEFAULT '',
    audience TEXT NOT NULL CHECK (audience IN ('profile', 'account-admin')),
    read_at TEXT NOT NULL DEFAULT '',
    archived_at TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL,
    CHECK (
        (audience = 'profile' AND profile_id <> '')
        OR (audience = 'account-admin' AND profile_id = '')
    ),
    PRIMARY KEY (notification_id, authority, account_id, server_id, profile_id, audience)
);
CREATE TABLE viewer_notification_revisions (
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id TEXT NOT NULL CHECK (server_id <> ''),
    profile_id TEXT NOT NULL DEFAULT '',
    audience TEXT NOT NULL CHECK (audience IN ('profile', 'account-admin')),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    updated_at TEXT NOT NULL,
    CHECK (
        (audience = 'profile' AND profile_id <> '')
        OR (audience = 'account-admin' AND profile_id = '')
    ),
    PRIMARY KEY (authority, account_id, server_id, profile_id, audience)
);
CREATE TABLE viewer_notifications (
    id TEXT PRIMARY KEY,
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    server_id TEXT NOT NULL CHECK (server_id <> ''),
    profile_id TEXT NOT NULL DEFAULT '',
    audience TEXT NOT NULL CHECK (audience IN ('profile', 'account-admin')),
    kind TEXT NOT NULL CHECK (kind IN (
        'account.security', 'download.ready', 'download.failed', 'dvr.conflict',
        'dvr.recording-failed', 'feedback.received', 'feedback.updated',
        'library-channel.degraded', 'membership.changed', 'server.message'
    )),
    severity TEXT NOT NULL CHECK (severity IN ('informational', 'success', 'warning', 'error')),
    message_id TEXT NOT NULL,
    icon_id TEXT NOT NULL,
    interpolation_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(interpolation_json) AND json_type(interpolation_json) = 'object'),
    content_json TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(content_json) AND json_type(content_json) = 'object'),
    actions_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(actions_json) AND json_type(actions_json) = 'array'),
    source_feedback_id TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    expires_at TEXT NOT NULL,
    CHECK (
        (audience = 'profile' AND profile_id <> '')
        OR (audience = 'account-admin' AND profile_id = '')
    ),
    CHECK (
        (kind IN ('feedback.received', 'feedback.updated') AND source_feedback_id <> '')
        OR (kind NOT IN ('feedback.received', 'feedback.updated') AND source_feedback_id = '')
    )
);
CREATE TABLE viewer_preference_document_quarantine (
    id TEXT PRIMARY KEY,
    document_id TEXT NOT NULL,
    scope_type TEXT NOT NULL,
    authority TEXT NOT NULL,
    account_id TEXT NOT NULL,
    profile_id TEXT NOT NULL,
    server_id TEXT NOT NULL,
    device_class TEXT NOT NULL,
    installation_id TEXT NOT NULL,
    version TEXT NOT NULL,
    revision INTEGER NOT NULL,
    values_json TEXT NOT NULL,
    reason TEXT NOT NULL,
    quarantined_at TEXT NOT NULL
);
CREATE TABLE viewer_preference_documents (
    id TEXT PRIMARY KEY,
    scope_type TEXT NOT NULL CHECK (scope_type IN (
        'profile-server',
        'profile-device-class',
        'account-server-installation'
    )),
    authority TEXT NOT NULL CHECK (authority IN ('local', 'hosted')),
    account_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    profile_id TEXT NOT NULL DEFAULT '',
    server_id TEXT NOT NULL CHECK (server_id <> ''),
    device_class TEXT NOT NULL DEFAULT '' CHECK (device_class IN ('', 'web', 'mobile', 'television')),
    installation_id TEXT NOT NULL DEFAULT '',
    version TEXT NOT NULL CHECK (version = 'v1'),
    revision INTEGER NOT NULL DEFAULT 0 CHECK (revision >= 0),
    values_json TEXT NOT NULL CHECK (json_valid(values_json) AND json_type(values_json) = 'object'),
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    CHECK (
        (scope_type = 'profile-server' AND profile_id <> '' AND device_class = '' AND installation_id = '')
        OR (scope_type = 'profile-device-class' AND profile_id <> '' AND device_class <> '' AND installation_id = '')
        OR (scope_type = 'account-server-installation' AND profile_id = '' AND device_class = '' AND installation_id <> '')
    ),
    UNIQUE (scope_type, authority, account_id, profile_id, server_id, device_class, installation_id)
);
CREATE TABLE watch_with_friends_command_receipts (
    group_id TEXT NOT NULL REFERENCES watch_with_friends_groups(id) ON DELETE CASCADE,
    idempotency_key TEXT NOT NULL,
    request_fingerprint TEXT NOT NULL,
    response_revision INTEGER NOT NULL,
    response_playback_revision INTEGER NOT NULL,
    created_at TEXT NOT NULL,
    PRIMARY KEY (group_id, idempotency_key)
);
CREATE TABLE watch_with_friends_groups (
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
, revision INTEGER NOT NULL DEFAULT 0, playback_revision INTEGER NOT NULL DEFAULT 0, position_updated_at TEXT NOT NULL DEFAULT '', playback_rate REAL NOT NULL DEFAULT 1, reconnect_generation INTEGER NOT NULL DEFAULT 0, last_idempotency_key TEXT NOT NULL DEFAULT '');
CREATE TABLE watch_with_friends_members (
			group_id TEXT NOT NULL REFERENCES watch_with_friends_groups(id) ON DELETE CASCADE,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
			state TEXT NOT NULL DEFAULT 'joined',
			position_seconds INTEGER NOT NULL DEFAULT 0,
			joined_at TEXT NOT NULL,
			last_seen_at TEXT NOT NULL,
			PRIMARY KEY (group_id, profile_id)
		);
CREATE TABLE watch_with_friends_queue (
			group_id TEXT NOT NULL REFERENCES watch_with_friends_groups(id) ON DELETE CASCADE,
			media_id TEXT NOT NULL,
			media_title TEXT NOT NULL DEFAULT '',
			sort_order INTEGER NOT NULL DEFAULT 0,
			added_by_user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			added_by_profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
			added_at TEXT NOT NULL,
			PRIMARY KEY (group_id, media_id)
		);
CREATE INDEX idx_api_keys_token ON api_keys(token_hash, revoked_at);
CREATE INDEX idx_api_keys_user ON api_keys(user_id, revoked_at, created_at);
CREATE INDEX idx_audio_fingerprints_recording ON audio_fingerprints(recording_id);
CREATE INDEX idx_audiobook_browse_entities_page
    ON audiobook_browse_entities(library_id, entity_kind, normalized_name, id);
CREATE INDEX idx_audiobook_browse_members_entity
    ON audiobook_browse_entity_members(entity_kind, entity_id, media_id);
CREATE INDEX idx_audit_events_created ON audit_events(created_at);
CREATE INDEX idx_audit_events_created_cursor ON audit_events(created_at DESC, id DESC);
CREATE INDEX idx_audit_events_resource ON audit_events(resource_type, resource_id);
CREATE INDEX idx_automatic_profile_trust_lookup
    ON automatic_profile_selection_trusts(account_id, server_id, device_id, installation_id, expires_at);
CREATE INDEX idx_automatic_profile_trust_profile
    ON automatic_profile_selection_trusts(profile_id, pin_revision, revoked_at);
CREATE INDEX idx_browser_account_entries_device
    ON browser_account_entries(device_id, revoked_at, expires_at);
CREATE INDEX idx_browser_account_entries_user
    ON browser_account_entries(user_id, revoked_at, expires_at);
CREATE INDEX idx_browser_account_vaults_active
    ON browser_account_vaults(token_hash, revoked_at, expires_at);
CREATE INDEX idx_browser_account_vaults_device
    ON browser_account_vaults(device_id, revoked_at, expires_at);
CREATE INDEX idx_cast_bootstraps_expiry ON cast_bootstraps(expires_at, redeemed_at);
CREATE INDEX idx_cast_receiver_sessions_expiry
    ON cast_receiver_sessions(expires_at, status);
CREATE INDEX idx_cast_receiver_sessions_scope
    ON cast_receiver_sessions(user_id, profile_id, client_instance_id, status, generation);
CREATE INDEX idx_client_diagnostic_events_created
    ON client_diagnostic_events(created_at DESC, id DESC);
CREATE INDEX idx_client_diagnostic_events_principal
    ON client_diagnostic_events(account_id, device, origin, created_at DESC);
CREATE INDEX idx_dashboard_playback_rollups_ended_started ON dashboard_playback_rollups(ended_at, started_at);
CREATE INDEX idx_dashboard_playback_rollups_group_started ON dashboard_playback_rollups(media_group, started_at);
CREATE INDEX idx_dashboard_playback_rollups_last_seen ON dashboard_playback_rollups(last_seen_at, session_id);
CREATE INDEX idx_dashboard_playback_rollups_library_started ON dashboard_playback_rollups(library_id, started_at);
CREATE INDEX idx_dashboard_playback_rollups_started ON dashboard_playback_rollups(started_at);
CREATE INDEX idx_dashboard_playback_rollups_type_started ON dashboard_playback_rollups(media_type, started_at);
CREATE INDEX idx_dashboard_playback_rollups_user_started ON dashboard_playback_rollups(user_id, started_at);
CREATE UNIQUE INDEX idx_devices_user_installation
    ON devices(user_id, installation_id) WHERE installation_id <> '';
CREATE INDEX idx_devices_user_seen ON devices(user_id, last_seen_at);
CREATE UNIQUE INDEX idx_download_preparations_active_owner_artifact
    ON download_preparations(server_id, account_id, profile_id, media_id, quality_profile)
    WHERE removed_at = '';
CREATE INDEX idx_download_preparations_artifact
    ON download_preparations(server_id, media_id, quality_profile, state, removed_at);
CREATE INDEX idx_download_preparations_job
    ON download_preparations(server_id, job_id)
    WHERE job_id <> '';
CREATE INDEX idx_download_preparations_owner
    ON download_preparations(server_id, account_id, profile_id, removed_at, updated_at DESC);
CREATE INDEX idx_hosted_profile_assertion_receipts_expiry
    ON hosted_profile_selection_assertion_receipts(expires_at);
CREATE INDEX idx_identity_reconciliation_reviews_open
ON identity_reconciliation_reviews(domain, status, created_at DESC);
CREATE INDEX idx_identity_reconciliation_reviews_subject
ON identity_reconciliation_reviews(domain, subject_id, status);
CREATE UNIQUE INDEX idx_jobs_active_key
    ON jobs(active_key)
    WHERE active_key <> '' AND status IN ('queued', 'running');
CREATE INDEX idx_jobs_created ON jobs(created_at);
CREATE INDEX idx_jobs_created_cursor ON jobs(created_at DESC, id DESC);
CREATE INDEX idx_jobs_failure_kind ON jobs(status, failure_kind, updated_at);
CREATE INDEX idx_jobs_interrupted_retry
    ON jobs(status, retry_eligible, updated_at DESC, id DESC);
CREATE INDEX idx_jobs_lease ON jobs(status, lease_expires_at);
CREATE INDEX idx_jobs_library_scan_summary ON jobs(type, resource_type, resource_id, updated_at DESC, created_at DESC);
CREATE INDEX idx_jobs_ready ON jobs(status, next_run_at, created_at);
CREATE INDEX idx_jobs_retention ON jobs(status, updated_at DESC, id DESC);
CREATE INDEX idx_jobs_retention_until
    ON jobs(status, retention_until, updated_at DESC, id DESC);
CREATE INDEX idx_library_category_counts_library ON library_category_counts(library_id, count DESC, filter);
CREATE INDEX idx_library_channel_blocks_channel ON library_channel_blocks(channel_id, enabled, priority DESC, sort_order, id);
CREATE INDEX idx_library_channel_blocks_rule ON library_channel_blocks(channel_id, rule_id);
CREATE INDEX idx_library_channel_building_lease ON library_channel_schedule_generations(status, lease_expires_at) WHERE status = 'building';
CREATE INDEX idx_library_channel_entries_generation ON library_channel_schedule_entries(generation_id, starts_at, id);
CREATE INDEX idx_library_channel_entries_guide ON library_channel_schedule_entries(channel_id, starts_at, ends_at);
CREATE INDEX idx_library_channel_entries_media ON library_channel_schedule_entries(media_id) WHERE media_id IS NOT NULL;
CREATE INDEX idx_library_channel_generation_queue_due
ON library_channel_generation_queue(not_before, requested_at, channel_id);
CREATE INDEX idx_library_channel_generations_channel_status ON library_channel_schedule_generations(channel_id, status, created_at DESC);
CREATE UNIQUE INDEX idx_library_channel_one_active_generation ON library_channel_schedule_generations(channel_id) WHERE status = 'active';
CREATE UNIQUE INDEX idx_library_channel_one_building_generation ON library_channel_schedule_generations(channel_id) WHERE status = 'building';
CREATE INDEX idx_library_channel_playback_policies_channel
    ON library_channel_playback_policies(channel_id, created_at);
CREATE INDEX idx_library_channel_playback_policies_expiry
    ON library_channel_playback_policies(expires_at, channel_id);
CREATE INDEX idx_library_channel_rules_channel ON library_channel_rules(channel_id, enabled, sort_order, id);
CREATE INDEX idx_library_channels_enabled_sort ON library_channels(enabled, sort_order, name, id);
CREATE UNIQUE INDEX idx_library_channels_template ON library_channels(template_key) WHERE template_key <> '';
CREATE INDEX idx_library_paths_library ON library_paths(library_id, sort_order);
CREATE INDEX idx_library_scan_directories_library ON library_scan_directories(library_id, path);
CREATE INDEX idx_library_scan_continuation_directories_library ON library_scan_continuation_directories(library_id, path);
CREATE INDEX idx_library_scan_runs_library ON library_scan_runs(library_id, started_at DESC, id DESC);
CREATE INDEX idx_library_scan_runs_status ON library_scan_runs(status, updated_at, id);
CREATE INDEX idx_library_scan_run_roots_source ON library_scan_run_roots(source_id, status, last_progress_at);
CREATE INDEX idx_scanner_backlog_ready ON scanner_backlog(kind, status, next_run_at, created_at, id);
CREATE INDEX idx_scanner_backlog_library ON scanner_backlog(library_id, kind, status, created_at, id);
CREATE INDEX idx_storage_sources_health ON storage_sources(health_state, circuit_state, updated_at, id);
CREATE INDEX idx_library_source_groups_library ON library_source_groups(library_id, item_count DESC, kind, label);
CREATE INDEX idx_live_tv_channel_locators_channel
ON live_tv_channel_locators(channel_id, active, last_seen_at DESC);
CREATE UNIQUE INDEX idx_live_tv_channel_locators_evidence
ON live_tv_channel_locators(
    channel_id,
    provider_kind,
    provider_key,
    stream_url,
    tvg_id,
    channel_number,
    normalized_name
);
CREATE INDEX idx_live_tv_channel_locators_provider
ON live_tv_channel_locators(source_id, provider_kind, provider_key)
WHERE provider_key <> '';
CREATE INDEX idx_live_tv_channel_locators_stream
ON live_tv_channel_locators(source_id, stream_url)
WHERE stream_url <> '';
CREATE INDEX idx_live_tv_channel_locators_tvg
ON live_tv_channel_locators(source_id, tvg_id)
WHERE tvg_id <> '';
CREATE INDEX idx_live_tv_channel_mappings_channel ON live_tv_channel_mappings(channel_id);
CREATE INDEX idx_live_tv_channel_profile_state_account ON live_tv_channel_profile_state(user_id, profile_id, updated_at);
CREATE INDEX idx_live_tv_channel_profile_state_favorite ON live_tv_channel_profile_state(profile_id, favorite, channel_id);
CREATE INDEX idx_live_tv_channel_profile_state_hidden ON live_tv_channel_profile_state(profile_id, hidden, channel_id);
CREATE INDEX idx_live_tv_channels_source ON live_tv_channels(source_id, sort_order, name);
CREATE INDEX idx_live_tv_channels_source_generation ON live_tv_channels(source_id, import_generation, sort_order, name);
CREATE INDEX idx_live_tv_channels_tvg ON live_tv_channels(source_id, tvg_id);
CREATE INDEX idx_live_tv_programs_channel_time ON live_tv_programs(channel_id, start_at, end_at);
CREATE INDEX idx_live_tv_programs_source_generation ON live_tv_programs(source_id, import_generation);
CREATE INDEX idx_live_tv_programs_source_time ON live_tv_programs(source_id, start_at, end_at);
CREATE INDEX idx_live_tv_recording_rules_source_generation ON live_tv_recording_rules(source_id, guide_generation, revision);
CREATE INDEX idx_live_tv_recordings_source_generation ON live_tv_recordings(source_id, guide_generation, status, revision);
CREATE INDEX idx_live_tv_sources_enabled ON live_tv_sources(enabled, sort_order);
CREATE INDEX idx_live_tv_tuner_allocations_expiry
    ON live_tv_tuner_allocations(source_id, expires_at, heartbeat_at);
CREATE INDEX idx_live_tv_tuner_allocations_source
    ON live_tv_tuner_allocations(source_id, allocation_key, allocation_kind, heartbeat_at);
CREATE INDEX idx_local_credentials_identity ON local_credentials(profile_identity_id, revoked_at);
CREATE INDEX idx_local_profile_admin_proofs_account
    ON local_profile_admin_proofs(account_id, expires_at);
CREATE INDEX idx_local_profile_admin_proofs_session
    ON local_profile_admin_proofs(session_id, expires_at);
CREATE INDEX idx_localization_options_kind ON localization_options(kind, sort_order, id);
CREATE INDEX idx_localization_rating_systems_order ON localization_rating_systems(sort_order, country, system);
CREATE INDEX idx_localization_rating_values_order ON localization_rating_values(country, system, sort_order, rating);
CREATE INDEX idx_media_access_labels_label ON media_access_labels(normalized_label, media_id);
CREATE INDEX idx_media_access_tags_tag ON media_access_tags(normalized_tag, media_id);
CREATE INDEX idx_media_attachments_media ON media_attachments(media_id);
CREATE INDEX idx_media_availability_source_flags ON media_availability(has_local_source, has_remote_source, has_hdr_source, media_id);
CREATE INDEX idx_media_browse_library_parent_type_added ON media_items(library_id, parent_id, type, added_at DESC, id);
CREATE INDEX idx_media_browse_library_parent_type_title_nocase ON media_items(library_id, parent_id, type, sort_title COLLATE NOCASE, id);
CREATE INDEX idx_media_category_facets_library ON media_category_facets(library_id, facet_type, sort_value);
CREATE INDEX idx_media_chapters_media ON media_chapters(media_id, sort_order);
CREATE INDEX idx_media_download_grants_media
    ON media_download_grants(media_id, consumed_at, expires_at);
CREATE INDEX idx_media_download_grants_principal
    ON media_download_grants(principal_user_id, consumed_at, expires_at);
CREATE INDEX idx_media_files_identity_fingerprint
ON media_files(library_id, source_type, content_fingerprint)
WHERE content_fingerprint <> '';
CREATE INDEX idx_media_files_library_directory ON media_files(library_id, directory_path, available);
CREATE INDEX idx_media_files_library_missing ON media_files(library_id, available, missing_since);
CREATE INDEX idx_media_files_library_path ON media_files(library_id, path);
CREATE INDEX idx_media_files_library_scan_generation ON media_files(library_id, available, scan_generation);
CREATE INDEX idx_media_files_media ON media_files(media_id, available);
CREATE UNIQUE INDEX idx_media_files_media_path ON media_files(media_id, path);
CREATE INDEX idx_media_files_media_source_profile ON media_files(media_id, source_type, dynamic_range, resolution);
CREATE INDEX idx_media_files_path ON media_files(path);
CREATE INDEX idx_media_identity_evidence_media ON media_identity_evidence(media_id, source, field);
CREATE INDEX idx_media_images_media ON media_images(media_id, image_type, preferred);
CREATE UNIQUE INDEX idx_media_images_unique_source ON media_images(media_id, image_type, source, path, remote_url);
CREATE INDEX idx_media_images_scanner_scope ON media_images(media_id, source, provider, discovery_scope);
CREATE INDEX idx_media_images_selection ON media_images(media_id, image_type, selection_state, preferred, rating DESC);
CREATE INDEX idx_media_library ON media_items(library_id);
CREATE INDEX idx_media_library_parent_added ON media_items(library_id, parent_id, added_at DESC);
CREATE INDEX idx_media_library_parent_filter_album_artist_v3 ON media_items(library_id, parent_id, filter_album_artist_key, sort_title, id);
CREATE INDEX idx_media_library_parent_filter_artist_v3 ON media_items(library_id, parent_id, filter_artist_key, sort_title, id);
CREATE INDEX idx_media_library_parent_filter_author_v3 ON media_items(library_id, parent_id, filter_author_key, sort_title, id);
CREATE INDEX idx_media_library_parent_filter_label_v3 ON media_items(library_id, parent_id, filter_label_key, sort_title, id);
CREATE INDEX idx_media_library_parent_filter_narrator_v3 ON media_items(library_id, parent_id, filter_narrator_key, sort_title, id);
CREATE INDEX idx_media_library_parent_filter_network_v3 ON media_items(library_id, parent_id, filter_network_key, sort_title, id);
CREATE INDEX idx_media_library_parent_filter_series_v3 ON media_items(library_id, parent_id, filter_series_key, sort_title, id);
CREATE INDEX idx_media_library_parent_filter_studio_v3 ON media_items(library_id, parent_id, filter_studio_key, sort_title, id);
CREATE INDEX idx_media_library_parent_filter_track_artist_v3 ON media_items(library_id, parent_id, filter_track_artist_key, sort_title, id);
CREATE INDEX idx_media_library_parent_random ON media_items(library_id, parent_id, random_key, id);
CREATE INDEX idx_media_library_parent_sort ON media_items(library_id, parent_id, sort_title);
CREATE INDEX idx_media_library_parent_sort_album_artist ON media_items(library_id, parent_id, sort_album_artist_key, sort_title, id);
CREATE INDEX idx_media_library_parent_sort_artist ON media_items(library_id, parent_id, sort_artist_key, sort_title, id);
CREATE INDEX idx_media_library_parent_sort_author ON media_items(library_id, parent_id, sort_author_key, sort_title, id);
CREATE INDEX idx_media_library_parent_sort_narrator ON media_items(library_id, parent_id, sort_narrator_key, sort_title, id);
CREATE INDEX idx_media_library_parent_sort_series ON media_items(library_id, parent_id, sort_series_key, sort_title, id);
CREATE INDEX idx_media_library_parent_sort_track_artist ON media_items(library_id, parent_id, sort_track_artist_key, sort_title, id);
CREATE INDEX idx_media_lyrics_media ON media_lyrics(media_id, source, language);
CREATE INDEX idx_media_match_candidates_external ON media_match_candidates(provider, external_id, external_type);
CREATE INDEX idx_media_match_candidates_media ON media_match_candidates(media_id, provider, external_type, created_at);
CREATE INDEX idx_media_metadata_revisions_media ON media_metadata_revisions(media_id, state, revision DESC);
CREATE INDEX idx_media_provider_snapshots_identity ON media_provider_snapshots(media_id, provider, external_type, external_id, fetched_at DESC);
CREATE INDEX idx_media_metadata_field_values_current ON media_metadata_field_values(media_id, field_key, decision, revision_id);
CREATE INDEX idx_media_metadata_relationships_current ON media_metadata_relationships(media_id, relationship_type, decision, ordinal);
CREATE INDEX idx_media_metadata_relationships_reverse ON media_metadata_relationships(relationship_type, target_key, decision, media_id);
CREATE INDEX idx_media_metadata_refresh_media ON media_metadata_refresh_outcomes(media_id, created_at DESC);
CREATE INDEX idx_media_parent ON media_items(parent_id);
CREATE INDEX idx_media_parent_index ON media_items(parent_id, index_number);
CREATE INDEX idx_media_people_canonical_person
    ON media_people(canonical_person_key, media_id)
    WHERE canonical_person_key <> '';
CREATE INDEX idx_media_people_media ON media_people(media_id, role, sort_order);
CREATE INDEX idx_media_people_name_role ON media_people(lower(trim(name)), role, media_id);
CREATE INDEX idx_media_people_normalized_name
    ON media_people(lower(trim(name)), sort_order, media_id);
CREATE INDEX idx_media_provider_ids_external ON media_provider_ids(provider, external_id, external_type);
CREATE UNIQUE INDEX idx_media_provider_ids_one_accepted ON media_provider_ids(media_id, provider, external_type) WHERE status = 'accepted';
CREATE INDEX idx_media_provider_ids_candidates ON media_provider_ids(media_id, provider, external_type, status, confidence DESC, observed_at DESC);
CREATE INDEX idx_media_rating_evidence_media ON media_rating_evidence(media_id, normalized_rank);
CREATE INDEX idx_media_scanner_identity_aliases_media
ON media_scanner_identity_aliases(media_id);
CREATE INDEX idx_media_segments_media ON media_segments(media_id, start_seconds);
CREATE INDEX idx_media_sort ON media_items(sort_title);
CREATE INDEX idx_media_trickplay_sets_media ON media_trickplay_sets(media_id, stale, created_at);
CREATE INDEX idx_media_trickplay_tiles_set ON media_trickplay_tiles(set_id, tile_index);
CREATE INDEX idx_media_type ON media_items(type);
CREATE INDEX idx_media_type_added ON media_items(type, added_at DESC, id);
CREATE INDEX idx_metadata_health_category ON metadata_health_issues(category, severity, title);
CREATE INDEX idx_metadata_health_library_category ON metadata_health_issues(library_id, category, severity, title);
CREATE INDEX idx_metadata_health_media ON metadata_health_issues(media_id);
CREATE INDEX idx_native_auth_exchange_receipts_expiry
    ON native_auth_exchange_receipts(expires_at);
CREATE UNIQUE INDEX idx_native_auth_exchange_receipts_refresh
    ON native_auth_exchange_receipts(native_refresh_token_id);
CREATE INDEX idx_native_refresh_tokens_device
    ON native_refresh_tokens(device_id, revoked_at, expires_at);
CREATE INDEX idx_native_refresh_tokens_family
    ON native_refresh_tokens(family_id, created_at);
CREATE INDEX idx_native_refresh_tokens_profile
    ON native_refresh_tokens(profile_id, revoked_at, expires_at);
CREATE INDEX idx_native_refresh_tokens_replacement
    ON native_refresh_tokens(replaced_by_id);
CREATE INDEX idx_native_refresh_tokens_user
    ON native_refresh_tokens(user_id, revoked_at, expires_at);
CREATE UNIQUE INDEX idx_optimized_versions_ready_media_profile ON optimized_versions(media_id, profile) WHERE state = 'ready';
CREATE INDEX idx_optimized_versions_media_profile_state ON optimized_versions(media_id, profile, state, updated_at DESC);
CREATE INDEX idx_person_public_ids_identity ON person_public_ids(identity_key);
CREATE INDEX idx_playback_continuation_expiry
    ON playback_session_continuation_credentials(expires_at, revoked_at);
CREATE UNIQUE INDEX idx_playback_continuation_previous_token
    ON playback_session_continuation_credentials(previous_token_hash)
    WHERE previous_token_hash <> '';
CREATE INDEX idx_playback_continuation_scope
    ON playback_session_continuation_credentials(user_id, profile_id, generation, revoked_at);
CREATE INDEX idx_playback_media_grants_principal
    ON playback_media_grants(principal_user_id, revoked_at, expires_at);
CREATE INDEX idx_playback_media_grants_session
    ON playback_media_grants(playback_session_id, revoked_at, expires_at);
CREATE INDEX idx_playback_prepared_handoff_scope
    ON playback_prepared_handoffs(user_id, profile_id, source_session_id, expires_at);
CREATE INDEX idx_playback_receivers_profile_seen
    ON playback_receivers(profile_id, last_seen_at);
CREATE INDEX idx_playback_receivers_user_seen ON playback_receivers(user_id, last_seen_at);
CREATE INDEX idx_playback_session_history_order ON playback_session_history(session_id, sort_order);
CREATE INDEX idx_playback_session_queue_order ON playback_session_queue(session_id, sort_order);
CREATE INDEX idx_playback_sessions_live ON playback_sessions(ended_at, last_seen_at);
CREATE INDEX idx_playback_sessions_media_active ON playback_sessions(media_id, ended_at, last_seen_at);
CREATE INDEX idx_playback_sessions_media_started ON playback_sessions(media_id, started_at);
CREATE INDEX idx_playback_sessions_profile_active ON playback_sessions(profile_id, ended_at, last_seen_at);
CREATE INDEX idx_playback_sessions_started ON playback_sessions(started_at);
CREATE INDEX idx_playback_sessions_user_active ON playback_sessions(user_id, ended_at, last_seen_at);
CREATE INDEX idx_playback_sessions_user_client_active ON playback_sessions(user_id, client_instance_id, ended_at, last_seen_at);
CREATE INDEX idx_playback_sessions_user_started ON playback_sessions(user_id, started_at);
CREATE INDEX idx_playlist_items_media ON playlist_items(playlist_id, media_id);
CREATE INDEX idx_playlist_items_order ON playlist_items(playlist_id, sort_order, entry_id);
CREATE INDEX idx_playlist_shares_user ON playlist_shares(user_id, playlist_id);
CREATE INDEX idx_playlists_profile_kind
    ON playlists(profile_id, kind, updated_at);
CREATE INDEX idx_playlists_user_kind ON playlists(user_id, kind, updated_at);
CREATE INDEX idx_portico_access_members_status ON remote_access_members(status, email);
CREATE INDEX idx_portico_login_requests_status ON portico_login_requests(status, expires_at, created_at);
CREATE INDEX idx_portico_login_retryable_state
    ON portico_login_requests(state_hash, status, expires_at);
CREATE INDEX idx_profile_account_authentications_account
    ON profile_account_authentications(account_id, consumed_at, expires_at);
CREATE INDEX idx_profile_account_authentications_lookup
    ON profile_account_authentications(token_hash, consumed_at, expires_at);
CREATE INDEX idx_profile_identities_profile ON profile_identities(profile_id, provider);
CREATE UNIQUE INDEX idx_profile_identities_provider_subject ON profile_identities(provider, subject) WHERE subject <> '';
CREATE INDEX idx_profile_search_history_recent
    ON profile_search_history(profile_id, last_used_at DESC);
CREATE INDEX idx_profile_selection_grants_lookup
    ON profile_selection_grants(token_hash, consumed_at, expires_at);
CREATE INDEX idx_profile_selection_grants_profile
    ON profile_selection_grants(profile_id, consumed_at, expires_at);
CREATE UNIQUE INDEX idx_profiles_account_external
    ON profiles(account_id, external_profile_id) WHERE external_profile_id <> '';
CREATE UNIQUE INDEX idx_profiles_account_primary
    ON profiles(account_id) WHERE is_primary = 1;
CREATE INDEX idx_profiles_account_sort
    ON profiles(account_id, sort_order, created_at, id);
CREATE UNIQUE INDEX idx_quick_connect_active_code
ON quick_connect_requests(code)
WHERE status IN ('pending', 'approved');
CREATE INDEX idx_quick_connect_code ON quick_connect_requests(code, status, expires_at);
CREATE UNIQUE INDEX idx_quick_connect_native_refresh_receipt
    ON quick_connect_requests(native_refresh_token_id)
    WHERE native_refresh_token_id <> '';
CREATE INDEX idx_quick_connect_status ON quick_connect_requests(status, expires_at, created_at);
CREATE INDEX idx_recording_rules_account
    ON live_tv_recording_rules(user_id, enabled, updated_at DESC, id);
CREATE INDEX idx_recording_rules_priority
    ON live_tv_recording_rules(source_id, enabled, priority DESC, updated_at DESC, id);
CREATE INDEX idx_recording_rules_profile_updated ON live_tv_recording_rules(profile_id, updated_at DESC, id);
CREATE INDEX idx_recording_rules_source ON live_tv_recording_rules(source_id, enabled);
CREATE INDEX idx_recordings_account_status
    ON live_tv_recordings(user_id, status, starts_at DESC, id);
CREATE INDEX idx_recordings_allocation
    ON live_tv_recordings(source_id, status, starts_at, ends_at, priority DESC, id);
CREATE INDEX idx_recordings_group_admin ON live_tv_recordings(folder, title, starts_at DESC) WHERE status <> 'scheduled';
CREATE INDEX idx_recordings_group_profile ON live_tv_recordings(profile_id, folder, title, starts_at DESC) WHERE status <> 'scheduled';
CREATE INDEX idx_recordings_group_user ON live_tv_recordings(user_id, folder, title, starts_at DESC) WHERE status <> 'scheduled';
CREATE INDEX idx_recordings_profile_schedule ON live_tv_recordings(profile_id, status, starts_at, id);
CREATE UNIQUE INDEX idx_recordings_rule_program_unique
    ON live_tv_recordings(rule_id, program_id)
    WHERE rule_id IS NOT NULL AND program_id <> '';
CREATE INDEX idx_recordings_schedule ON live_tv_recordings(status, starts_at);
CREATE UNIQUE INDEX idx_remote_access_members_local_user
    ON remote_access_members(local_user_id) WHERE trim(local_user_id) <> '';
CREATE UNIQUE INDEX idx_remote_access_members_single_owner
    ON remote_access_members(role) WHERE role = 'owner';
CREATE INDEX idx_remote_access_members_status ON remote_access_members(status, email);
CREATE INDEX idx_saved_views_library ON saved_views(library_id, user_id);
CREATE INDEX idx_saved_views_profile_updated
    ON saved_views(profile_id, updated_at DESC, id DESC);
CREATE INDEX idx_saved_views_user_updated ON saved_views(user_id, updated_at DESC, id DESC);
CREATE INDEX idx_security_audit_events_created
    ON security_audit_events(created_at DESC, sequence DESC);
CREATE INDEX idx_server_diagnostic_events_created
    ON server_diagnostic_events(created_at DESC, id DESC);
CREATE INDEX idx_sessions_device ON sessions(device_id);
CREATE INDEX idx_sessions_token_hash ON sessions(token_hash);
CREATE INDEX idx_sessions_user ON sessions(user_id);
CREATE INDEX idx_settings_quarantine_key
    ON settings_quarantine(key, quarantined_at);
CREATE INDEX idx_streams_media ON media_streams(media_id);
CREATE INDEX idx_streams_media_kind ON media_streams(media_id, kind, height, dynamic_range);
CREATE INDEX idx_tmdb_trending_cache_expires ON tmdb_trending_cache(expires_at);
CREATE UNIQUE INDEX idx_tv_setup_active_code
ON tv_setup_sessions(code)
WHERE status IN ('pending', 'grant_ready');
CREATE UNIQUE INDEX idx_tv_setup_native_refresh_receipt
    ON tv_setup_sessions(native_refresh_token_id)
    WHERE native_refresh_token_id <> '';
CREATE INDEX idx_tv_setup_sessions_code ON tv_setup_sessions(code, status, expires_at);
CREATE INDEX idx_tv_setup_sessions_status ON tv_setup_sessions(status, expires_at, created_at);
CREATE INDEX idx_user_display_preferences_account ON user_display_preferences(user_id, profile_id);
CREATE INDEX idx_user_display_preferences_user ON user_display_preferences(profile_id, client, view);
CREATE INDEX idx_user_library_access_library ON user_library_access(library_id);
CREATE INDEX idx_user_library_access_user ON user_library_access(user_id);
CREATE INDEX idx_user_library_navigation_account ON user_library_navigation(user_id, profile_id);
CREATE INDEX idx_user_library_navigation_order ON user_library_navigation(profile_id, pinned DESC, sort_order, library_id);
CREATE INDEX idx_user_recommendation_cache_account ON user_recommendation_cache(user_id, profile_id);
CREATE INDEX idx_user_recommendation_cache_user_rank ON user_recommendation_cache(profile_id, rank);
CREATE INDEX idx_user_state_account ON user_media_state(user_id, profile_id);
CREATE INDEX idx_user_state_favorite_updated ON user_media_state(profile_id, favorite, updated_at DESC, media_id);
CREATE INDEX idx_user_state_last_played ON user_media_state(profile_id, last_played_at);
CREATE INDEX idx_user_state_media_recent ON user_media_state(last_played_at, media_id);
CREATE INDEX idx_media_browse_personal_rating ON user_media_state(profile_id, rating, media_id);
CREATE INDEX idx_user_state_resume_recent ON user_media_state(profile_id, watched, last_played_at DESC, updated_at DESC, media_id);
CREATE INDEX idx_user_state_watchlist ON user_media_state(profile_id, watchlisted);
CREATE INDEX idx_user_state_watchlist_updated ON user_media_state(profile_id, watchlisted, updated_at DESC, media_id);
CREATE UNIQUE INDEX idx_users_portico_membership ON users(portico_membership_id) WHERE portico_membership_id <> '';
CREATE UNIQUE INDEX idx_users_portico_user ON users(portico_user_id) WHERE portico_user_id <> '';
CREATE UNIQUE INDEX idx_users_single_owner
    ON users(role) WHERE role = 'owner';
CREATE UNIQUE INDEX idx_users_username_unique ON users(lower(username)) WHERE username <> '';
CREATE INDEX idx_viewer_feedback_duplicate
    ON viewer_feedback(account_id, profile_id, duplicate_hash, created_at DESC);
CREATE INDEX idx_viewer_feedback_expiry ON viewer_feedback(expires_at);
CREATE INDEX idx_viewer_feedback_owner_page
    ON viewer_feedback(status, created_at DESC, id DESC);
CREATE INDEX idx_viewer_feedback_recipient
    ON viewer_feedback(account_id, profile_id, created_at DESC, id DESC);
CREATE INDEX idx_viewer_notification_receipts_recipient
    ON viewer_notification_receipts(authority, account_id, server_id, profile_id, audience, archived_at, read_at);
CREATE INDEX idx_viewer_notifications_admin_page
    ON viewer_notifications(account_id, server_id, audience, created_at DESC, id DESC);
CREATE INDEX idx_viewer_notifications_expiry ON viewer_notifications(expires_at);
CREATE INDEX idx_viewer_notifications_profile_page
    ON viewer_notifications(account_id, server_id, profile_id, audience, created_at DESC, id DESC);
CREATE INDEX idx_viewer_preference_documents_installation
    ON viewer_preference_documents(account_id, server_id, installation_id, device_class);
CREATE INDEX idx_viewer_preference_documents_profile
    ON viewer_preference_documents(account_id, profile_id, server_id, scope_type);
CREATE INDEX idx_viewer_preference_quarantine_document
    ON viewer_preference_document_quarantine(document_id, quarantined_at);
CREATE INDEX idx_watch_with_friends_command_receipts_created
    ON watch_with_friends_command_receipts(group_id, created_at DESC);
CREATE INDEX idx_watch_with_friends_groups_active ON watch_with_friends_groups(ended_at, updated_at);
CREATE INDEX idx_watch_with_friends_groups_owner_profile ON watch_with_friends_groups(owner_profile_id, ended_at, updated_at);
CREATE INDEX idx_watch_with_friends_members_account ON watch_with_friends_members(user_id, last_seen_at);
CREATE INDEX idx_watch_with_friends_members_profile ON watch_with_friends_members(profile_id, last_seen_at);
CREATE INDEX idx_watch_with_friends_queue_order ON watch_with_friends_queue(group_id, sort_order);
CREATE TRIGGER media_access_terms_after_insert
AFTER INSERT ON media_items
BEGIN
    DELETE FROM media_access_tags WHERE media_id = NEW.id;
    INSERT OR IGNORE INTO media_access_tags (media_id, normalized_tag)
    SELECT NEW.id, lower(trim(tag.value))
    FROM json_each(CASE WHEN json_valid(NEW.tags_json) THEN NEW.tags_json ELSE '[]' END) tag
    WHERE lower(trim(tag.value)) <> '';

    DELETE FROM media_access_labels WHERE media_id = NEW.id;
    INSERT OR IGNORE INTO media_access_labels (media_id, normalized_label)
    SELECT NEW.id, lower(trim(label.value))
    FROM json_each(CASE WHEN json_valid(NEW.labels_json) THEN NEW.labels_json ELSE '[]' END) label
    WHERE lower(trim(label.value)) <> '';
END;
CREATE TRIGGER media_access_terms_after_update
AFTER UPDATE OF tags_json, labels_json ON media_items
BEGIN
    DELETE FROM media_access_tags WHERE media_id = NEW.id;
    INSERT OR IGNORE INTO media_access_tags (media_id, normalized_tag)
    SELECT NEW.id, lower(trim(tag.value))
    FROM json_each(CASE WHEN json_valid(NEW.tags_json) THEN NEW.tags_json ELSE '[]' END) tag
    WHERE lower(trim(tag.value)) <> '';

    DELETE FROM media_access_labels WHERE media_id = NEW.id;
    INSERT OR IGNORE INTO media_access_labels (media_id, normalized_label)
    SELECT NEW.id, lower(trim(label.value))
    FROM json_each(CASE WHEN json_valid(NEW.labels_json) THEN NEW.labels_json ELSE '[]' END) label
    WHERE lower(trim(label.value)) <> '';
END;
CREATE TRIGGER media_files_availability_delete
		AFTER DELETE ON media_files
		BEGIN
			UPDATE media_availability
			SET file_count = MAX(file_count - 1, 0),
				available_file_count = MAX(available_file_count - CASE WHEN OLD.available = 1 THEN 1 ELSE 0 END, 0),
				missing_file_count = MAX(missing_file_count - CASE WHEN OLD.available = 0 THEN 1 ELSE 0 END, 0),
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = OLD.media_id;

			UPDATE media_availability
			SET
				has_local_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND lower(trim(COALESCE(f.source_type, 'local'))) IN ('', 'local')
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'magnet:%'
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = OLD.media_id
							AND trim(COALESCE(mi.source_url, '')) <> ''
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'magnet:%'
					)
					THEN 1 ELSE 0 END,
				has_remote_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND (
								lower(trim(COALESCE(f.source_type, ''))) = 'remote'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'magnet:%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = OLD.media_id
							AND (
								lower(trim(COALESCE(mi.source_url, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'magnet:%'
							)
					)
					THEN 1 ELSE 0 END,
				has_hdr_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND (
								lower(COALESCE(f.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(f.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%2160%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%4k%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_streams st
						WHERE st.media_id = OLD.media_id
							AND st.kind = 'video'
							AND (
								COALESCE(st.height, 0) >= 2160
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%dolby%'
							)
					)
					THEN 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = OLD.media_id;

		END;
CREATE TRIGGER media_files_availability_insert
		AFTER INSERT ON media_files
		BEGIN
			INSERT INTO media_availability (media_id, file_count, available_file_count, missing_file_count, updated_at)
			VALUES (NEW.media_id, 1, CASE WHEN NEW.available = 1 THEN 1 ELSE 0 END, CASE WHEN NEW.available = 0 THEN 1 ELSE 0 END, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
			ON CONFLICT(media_id) DO UPDATE SET
				file_count = file_count + 1,
				available_file_count = available_file_count + CASE WHEN NEW.available = 1 THEN 1 ELSE 0 END,
				missing_file_count = missing_file_count + CASE WHEN NEW.available = 0 THEN 1 ELSE 0 END,
				updated_at = excluded.updated_at;

			UPDATE media_availability
			SET
				has_local_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND lower(trim(COALESCE(f.source_type, 'local'))) IN ('', 'local')
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'magnet:%'
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.media_id
							AND trim(COALESCE(mi.source_url, '')) <> ''
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'magnet:%'
					)
					THEN 1 ELSE 0 END,
				has_remote_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND (
								lower(trim(COALESCE(f.source_type, ''))) = 'remote'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'magnet:%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.media_id
							AND (
								lower(trim(COALESCE(mi.source_url, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'magnet:%'
							)
					)
					THEN 1 ELSE 0 END,
				has_hdr_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND (
								lower(COALESCE(f.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(f.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%2160%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%4k%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_streams st
						WHERE st.media_id = NEW.media_id
							AND st.kind = 'video'
							AND (
								COALESCE(st.height, 0) >= 2160
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%dolby%'
							)
					)
					THEN 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = NEW.media_id;

		END;
CREATE TRIGGER media_files_availability_update
		AFTER UPDATE OF media_id, available, path, source, source_type, resolution, dynamic_range ON media_files
		BEGIN
			UPDATE media_availability
			SET file_count = MAX(file_count - 1, 0),
				available_file_count = MAX(available_file_count - CASE WHEN OLD.available = 1 THEN 1 ELSE 0 END, 0),
				missing_file_count = MAX(missing_file_count - CASE WHEN OLD.available = 0 THEN 1 ELSE 0 END, 0),
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = OLD.media_id;
			INSERT INTO media_availability (media_id, file_count, available_file_count, missing_file_count, updated_at)
			VALUES (NEW.media_id, 1, CASE WHEN NEW.available = 1 THEN 1 ELSE 0 END, CASE WHEN NEW.available = 0 THEN 1 ELSE 0 END, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
			ON CONFLICT(media_id) DO UPDATE SET
				file_count = file_count + 1,
				available_file_count = available_file_count + CASE WHEN NEW.available = 1 THEN 1 ELSE 0 END,
				missing_file_count = missing_file_count + CASE WHEN NEW.available = 0 THEN 1 ELSE 0 END,
				updated_at = excluded.updated_at;

			UPDATE media_availability
			SET
				has_local_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND lower(trim(COALESCE(f.source_type, 'local'))) IN ('', 'local')
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'magnet:%'
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = OLD.media_id
							AND trim(COALESCE(mi.source_url, '')) <> ''
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'magnet:%'
					)
					THEN 1 ELSE 0 END,
				has_remote_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND (
								lower(trim(COALESCE(f.source_type, ''))) = 'remote'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'magnet:%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = OLD.media_id
							AND (
								lower(trim(COALESCE(mi.source_url, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'magnet:%'
							)
					)
					THEN 1 ELSE 0 END,
				has_hdr_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND (
								lower(COALESCE(f.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(f.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%2160%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%4k%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_streams st
						WHERE st.media_id = OLD.media_id
							AND st.kind = 'video'
							AND (
								COALESCE(st.height, 0) >= 2160
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%dolby%'
							)
					)
					THEN 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = OLD.media_id;


			UPDATE media_availability
			SET
				has_local_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND lower(trim(COALESCE(f.source_type, 'local'))) IN ('', 'local')
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'magnet:%'
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.media_id
							AND trim(COALESCE(mi.source_url, '')) <> ''
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'magnet:%'
					)
					THEN 1 ELSE 0 END,
				has_remote_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND (
								lower(trim(COALESCE(f.source_type, ''))) = 'remote'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'magnet:%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.media_id
							AND (
								lower(trim(COALESCE(mi.source_url, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'magnet:%'
							)
					)
					THEN 1 ELSE 0 END,
				has_hdr_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND (
								lower(COALESCE(f.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(f.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%2160%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%4k%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_streams st
						WHERE st.media_id = NEW.media_id
							AND st.kind = 'video'
							AND (
								COALESCE(st.height, 0) >= 2160
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%dolby%'
							)
					)
					THEN 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = NEW.media_id;

		END;
CREATE TRIGGER media_items_availability_insert
		AFTER INSERT ON media_items
		BEGIN
			INSERT INTO media_availability (media_id, updated_at)
			VALUES (NEW.id, strftime('%Y-%m-%dT%H:%M:%fZ','now'))
			ON CONFLICT(media_id) DO NOTHING;

			UPDATE media_availability
			SET
				has_local_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.id
							AND lower(trim(COALESCE(f.source_type, 'local'))) IN ('', 'local')
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'magnet:%'
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.id
							AND trim(COALESCE(mi.source_url, '')) <> ''
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'magnet:%'
					)
					THEN 1 ELSE 0 END,
				has_remote_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.id
							AND (
								lower(trim(COALESCE(f.source_type, ''))) = 'remote'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'magnet:%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.id
							AND (
								lower(trim(COALESCE(mi.source_url, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'magnet:%'
							)
					)
					THEN 1 ELSE 0 END,
				has_hdr_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.id
							AND (
								lower(COALESCE(f.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(f.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%2160%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%4k%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_streams st
						WHERE st.media_id = NEW.id
							AND st.kind = 'video'
							AND (
								COALESCE(st.height, 0) >= 2160
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%dolby%'
							)
					)
					THEN 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = NEW.id;

		END;
CREATE TRIGGER media_items_availability_source_update
		AFTER UPDATE OF source_url ON media_items
		BEGIN

			UPDATE media_availability
			SET
				has_local_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.id
							AND lower(trim(COALESCE(f.source_type, 'local'))) IN ('', 'local')
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'magnet:%'
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.id
							AND trim(COALESCE(mi.source_url, '')) <> ''
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'magnet:%'
					)
					THEN 1 ELSE 0 END,
				has_remote_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.id
							AND (
								lower(trim(COALESCE(f.source_type, ''))) = 'remote'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'magnet:%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.id
							AND (
								lower(trim(COALESCE(mi.source_url, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'magnet:%'
							)
					)
					THEN 1 ELSE 0 END,
				has_hdr_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.id
							AND (
								lower(COALESCE(f.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(f.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%2160%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%4k%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_streams st
						WHERE st.media_id = NEW.id
							AND st.kind = 'video'
							AND (
								COALESCE(st.height, 0) >= 2160
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%dolby%'
							)
					)
					THEN 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = NEW.id;

		END;
CREATE TRIGGER media_items_sort_keys_insert
		AFTER INSERT ON media_items
		BEGIN
			UPDATE media_items SET
				sort_artist_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.trackArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.albumArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.artist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.author'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.narrator'), ''), NULLIF(NEW.studio, ''), NEW.sort_title))),
				sort_album_artist_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.albumArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.artist'), ''), NULLIF(NEW.studio, ''), NEW.sort_title))),
				sort_track_artist_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.trackArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.artist'), ''), NULLIF(NEW.studio, ''), NEW.sort_title))),
				sort_author_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.author'), ''), NULLIF(NEW.studio, ''), NEW.sort_title))),
				sort_narrator_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.narrator'), ''), NEW.sort_title))),
				sort_series_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.series'), ''), NEW.sort_title))),
				sort_label_key = lower(trim(COALESCE(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.label'), ''))),
				sort_network_key = lower(trim(COALESCE(NULLIF(NEW.network, ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.network'), ''), ''))),
				sort_studio_key = lower(trim(COALESCE(NULLIF(NEW.studio, ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.studio'), ''), ''))),
				filter_artist_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.trackArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.albumArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.artist'), ''), NULLIF(NEW.studio, ''), ''))),
				filter_album_artist_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.albumArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.artist'), ''), NULLIF(NEW.studio, ''), ''))),
				filter_track_artist_key = lower(trim(COALESCE(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.trackArtist'), ''))),
				filter_author_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.author'), ''), NULLIF(NEW.studio, ''), ''))),
				filter_narrator_key = lower(trim(COALESCE(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.narrator'), ''))),
				filter_series_key = lower(trim(COALESCE(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.series'), ''))),
				filter_label_key = lower(trim(COALESCE(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.label'), ''))),
				filter_network_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.network'), ''), NULLIF(NEW.network, ''), ''))),
				filter_studio_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.studio'), ''), NULLIF(NEW.studio, ''), '')))
			WHERE id = NEW.id;
		END;
CREATE TRIGGER media_items_sort_keys_update
		AFTER UPDATE OF typed_metadata_json, studio, network, sort_title ON media_items
		BEGIN
			UPDATE media_items SET
				sort_artist_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.trackArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.albumArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.artist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.author'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.narrator'), ''), NULLIF(NEW.studio, ''), NEW.sort_title))),
				sort_album_artist_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.albumArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.artist'), ''), NULLIF(NEW.studio, ''), NEW.sort_title))),
				sort_track_artist_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.trackArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.artist'), ''), NULLIF(NEW.studio, ''), NEW.sort_title))),
				sort_author_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.author'), ''), NULLIF(NEW.studio, ''), NEW.sort_title))),
				sort_narrator_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.narrator'), ''), NEW.sort_title))),
				sort_series_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.series'), ''), NEW.sort_title))),
				sort_label_key = lower(trim(COALESCE(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.label'), ''))),
				sort_network_key = lower(trim(COALESCE(NULLIF(NEW.network, ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.network'), ''), ''))),
				sort_studio_key = lower(trim(COALESCE(NULLIF(NEW.studio, ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.studio'), ''), ''))),
				filter_artist_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.trackArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.albumArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.artist'), ''), NULLIF(NEW.studio, ''), ''))),
				filter_album_artist_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.albumArtist'), ''), NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.artist'), ''), NULLIF(NEW.studio, ''), ''))),
				filter_track_artist_key = lower(trim(COALESCE(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.trackArtist'), ''))),
				filter_author_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.author'), ''), NULLIF(NEW.studio, ''), ''))),
				filter_narrator_key = lower(trim(COALESCE(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.narrator'), ''))),
				filter_series_key = lower(trim(COALESCE(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.series'), ''))),
				filter_label_key = lower(trim(COALESCE(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.label'), ''))),
				filter_network_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.network'), ''), NULLIF(NEW.network, ''), ''))),
				filter_studio_key = lower(trim(COALESCE(NULLIF(json_extract(CASE WHEN json_valid(NEW.typed_metadata_json) THEN NEW.typed_metadata_json ELSE '{}' END, '$.studio'), ''), NULLIF(NEW.studio, ''), '')))
			WHERE id = NEW.id;
		END;
CREATE TRIGGER media_streams_availability_delete
		AFTER DELETE ON media_streams
		BEGIN

			UPDATE media_availability
			SET
				has_local_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND lower(trim(COALESCE(f.source_type, 'local'))) IN ('', 'local')
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'magnet:%'
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = OLD.media_id
							AND trim(COALESCE(mi.source_url, '')) <> ''
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'magnet:%'
					)
					THEN 1 ELSE 0 END,
				has_remote_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND (
								lower(trim(COALESCE(f.source_type, ''))) = 'remote'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'magnet:%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = OLD.media_id
							AND (
								lower(trim(COALESCE(mi.source_url, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'magnet:%'
							)
					)
					THEN 1 ELSE 0 END,
				has_hdr_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND (
								lower(COALESCE(f.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(f.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%2160%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%4k%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_streams st
						WHERE st.media_id = OLD.media_id
							AND st.kind = 'video'
							AND (
								COALESCE(st.height, 0) >= 2160
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%dolby%'
							)
					)
					THEN 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = OLD.media_id;

		END;
CREATE TRIGGER media_streams_availability_insert
		AFTER INSERT ON media_streams
		BEGIN

			UPDATE media_availability
			SET
				has_local_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND lower(trim(COALESCE(f.source_type, 'local'))) IN ('', 'local')
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'magnet:%'
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.media_id
							AND trim(COALESCE(mi.source_url, '')) <> ''
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'magnet:%'
					)
					THEN 1 ELSE 0 END,
				has_remote_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND (
								lower(trim(COALESCE(f.source_type, ''))) = 'remote'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'magnet:%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.media_id
							AND (
								lower(trim(COALESCE(mi.source_url, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'magnet:%'
							)
					)
					THEN 1 ELSE 0 END,
				has_hdr_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND (
								lower(COALESCE(f.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(f.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%2160%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%4k%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_streams st
						WHERE st.media_id = NEW.media_id
							AND st.kind = 'video'
							AND (
								COALESCE(st.height, 0) >= 2160
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%dolby%'
							)
					)
					THEN 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = NEW.media_id;

		END;
CREATE TRIGGER media_streams_availability_update
		AFTER UPDATE OF media_id, kind, height, dynamic_range, display_title ON media_streams
		BEGIN

			UPDATE media_availability
			SET
				has_local_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND lower(trim(COALESCE(f.source_type, 'local'))) IN ('', 'local')
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'magnet:%'
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = OLD.media_id
							AND trim(COALESCE(mi.source_url, '')) <> ''
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'magnet:%'
					)
					THEN 1 ELSE 0 END,
				has_remote_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND (
								lower(trim(COALESCE(f.source_type, ''))) = 'remote'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'magnet:%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = OLD.media_id
							AND (
								lower(trim(COALESCE(mi.source_url, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'magnet:%'
							)
					)
					THEN 1 ELSE 0 END,
				has_hdr_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = OLD.media_id
							AND (
								lower(COALESCE(f.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(f.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%2160%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%4k%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_streams st
						WHERE st.media_id = OLD.media_id
							AND st.kind = 'video'
							AND (
								COALESCE(st.height, 0) >= 2160
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%dolby%'
							)
					)
					THEN 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = OLD.media_id;


			UPDATE media_availability
			SET
				has_local_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND lower(trim(COALESCE(f.source_type, 'local'))) IN ('', 'local')
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(f.path, ''))) NOT LIKE 'magnet:%'
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.media_id
							AND trim(COALESCE(mi.source_url, '')) <> ''
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'http://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'https://%'
							AND lower(trim(COALESCE(mi.source_url, ''))) NOT LIKE 'magnet:%'
					)
					THEN 1 ELSE 0 END,
				has_remote_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND (
								lower(trim(COALESCE(f.source_type, ''))) = 'remote'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(f.path, ''))) LIKE 'magnet:%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_items mi
						WHERE mi.id = NEW.media_id
							AND (
								lower(trim(COALESCE(mi.source_url, ''))) LIKE 'http://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'https://%'
								OR lower(trim(COALESCE(mi.source_url, ''))) LIKE 'magnet:%'
							)
					)
					THEN 1 ELSE 0 END,
				has_hdr_source = CASE WHEN
					EXISTS (
						SELECT 1 FROM media_files f
						WHERE f.media_id = NEW.media_id
							AND (
								lower(COALESCE(f.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(f.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%2160%'
								OR lower(COALESCE(f.resolution, '')) LIKE '%4k%'
							)
					)
					OR EXISTS (
						SELECT 1 FROM media_streams st
						WHERE st.media_id = NEW.media_id
							AND st.kind = 'video'
							AND (
								COALESCE(st.height, 0) >= 2160
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.dynamic_range, '')) LIKE '%dolby%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%hdr%'
								OR lower(COALESCE(st.display_title, '')) LIKE '%dolby%'
							)
					)
					THEN 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%fZ','now')
			WHERE media_id = NEW.media_id;

		END;
CREATE TRIGGER trg_automatic_profile_trust_device_insert_guard
BEFORE INSERT ON automatic_profile_selection_trusts
WHEN NOT EXISTS (
    SELECT 1 FROM devices
    WHERE devices.id = NEW.device_id
      AND devices.user_id = NEW.account_id
      AND devices.installation_id = NEW.installation_id
      AND COALESCE(devices.revoked_at, '') = ''
)
BEGIN
    SELECT RAISE(ABORT, 'automatic profile trust device mismatch');
END;
CREATE TRIGGER trg_automatic_profile_trust_device_update_guard
BEFORE UPDATE OF account_id, device_id, installation_id ON automatic_profile_selection_trusts
WHEN NOT EXISTS (
    SELECT 1 FROM devices
    WHERE devices.id = NEW.device_id
      AND devices.user_id = NEW.account_id
      AND devices.installation_id = NEW.installation_id
      AND COALESCE(devices.revoked_at, '') = ''
)
BEGIN
    SELECT RAISE(ABORT, 'automatic profile trust device mismatch');
END;
CREATE TRIGGER trg_automatic_profile_trust_profile_guard
BEFORE INSERT ON automatic_profile_selection_trusts
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
      AND profiles.pin_revision = NEW.pin_revision
)
BEGIN
    SELECT RAISE(ABORT, 'automatic profile trust requires current profile security state');
END;
CREATE TRIGGER trg_automatic_profile_trust_profile_update_guard
BEFORE UPDATE OF account_id, profile_id, pin_revision ON automatic_profile_selection_trusts
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
      AND profiles.pin_revision = NEW.pin_revision
)
BEGIN
    SELECT RAISE(ABORT, 'automatic profile trust requires current profile security state');
END;
CREATE TRIGGER trg_automatic_profile_trust_revoke_security_update
AFTER UPDATE OF pin_revision, disabled_at ON profiles
BEGIN
    UPDATE automatic_profile_selection_trusts
       SET revoked_at = CURRENT_TIMESTAMP,
           updated_at = CURRENT_TIMESTAMP
     WHERE profile_id = NEW.id AND revoked_at = '';
END;
CREATE TRIGGER trg_hosted_primary_external_id_immutable
BEFORE UPDATE OF external_profile_id ON profiles
WHEN OLD.origin = 'hosted' AND OLD.is_primary = 1
    AND OLD.external_profile_id <> ''
    AND NEW.external_profile_id <> OLD.external_profile_id
BEGIN
    SELECT RAISE(ABORT, 'hosted primary profile is immutable');
END;
CREATE TRIGGER trg_hosted_profile_selection_assertion_receipts_profile_account_insert
				BEFORE INSERT ON hosted_profile_selection_assertion_receipts
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.account_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_hosted_profile_selection_assertion_receipts_profile_account_update
				BEFORE UPDATE ON hosted_profile_selection_assertion_receipts
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.account_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_library_channel_blocks_fallback_insert
BEFORE INSERT ON library_channel_blocks
WHEN NEW.fallback_rule_id IS NOT NULL
AND NOT EXISTS (SELECT 1 FROM library_channel_rules r WHERE r.id=NEW.fallback_rule_id AND r.channel_id=NEW.channel_id)
BEGIN
	SELECT RAISE(ABORT, 'library channel fallback rule belongs to another channel');
END;
CREATE TRIGGER trg_library_channel_blocks_fallback_update
BEFORE UPDATE OF channel_id, fallback_rule_id ON library_channel_blocks
WHEN NEW.fallback_rule_id IS NOT NULL
AND NOT EXISTS (SELECT 1 FROM library_channel_rules r WHERE r.id=NEW.fallback_rule_id AND r.channel_id=NEW.channel_id)
BEGIN
	SELECT RAISE(ABORT, 'library channel fallback rule belongs to another channel');
END;
CREATE TRIGGER trg_library_channel_entries_media_delete
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
CREATE TRIGGER trg_library_item_counts_media_delete
		AFTER DELETE ON media_items
		WHEN OLD.parent_id IS NULL AND COALESCE(OLD.library_id, '') <> ''
		BEGIN
			UPDATE library_item_counts
			SET root_item_count = CASE WHEN root_item_count > 0 THEN root_item_count - 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
			WHERE library_id = OLD.library_id;
		END;
CREATE TRIGGER trg_library_item_counts_media_insert
		AFTER INSERT ON media_items
		WHEN NEW.parent_id IS NULL AND COALESCE(NEW.library_id, '') <> ''
		BEGIN
			INSERT INTO library_item_counts (library_id, root_item_count, updated_at)
			VALUES (NEW.library_id, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
			ON CONFLICT(library_id) DO UPDATE SET
				root_item_count = root_item_count + 1,
				updated_at = excluded.updated_at;
		END;
CREATE TRIGGER trg_library_item_counts_media_update_new
		AFTER UPDATE OF library_id, parent_id ON media_items
		WHEN NEW.parent_id IS NULL
			AND COALESCE(NEW.library_id, '') <> ''
			AND (OLD.parent_id IS NOT NULL OR COALESCE(NEW.library_id, '') <> COALESCE(OLD.library_id, ''))
		BEGIN
			INSERT INTO library_item_counts (library_id, root_item_count, updated_at)
			VALUES (NEW.library_id, 1, strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
			ON CONFLICT(library_id) DO UPDATE SET
				root_item_count = root_item_count + 1,
				updated_at = excluded.updated_at;
		END;
CREATE TRIGGER trg_library_item_counts_media_update_old
		AFTER UPDATE OF library_id, parent_id ON media_items
		WHEN OLD.parent_id IS NULL
			AND COALESCE(OLD.library_id, '') <> ''
			AND (NEW.parent_id IS NOT NULL OR COALESCE(NEW.library_id, '') <> COALESCE(OLD.library_id, ''))
		BEGIN
			UPDATE library_item_counts
			SET root_item_count = CASE WHEN root_item_count > 0 THEN root_item_count - 1 ELSE 0 END,
				updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
			WHERE library_id = OLD.library_id;
		END;
CREATE TRIGGER trg_live_tv_channel_profile_state_profile_id_account_insert
				BEFORE INSERT ON live_tv_channel_profile_state
				WHEN (
					NEW.profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_live_tv_channel_profile_state_profile_id_account_update
				BEFORE UPDATE ON live_tv_channel_profile_state
				WHEN (
					NEW.profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_live_tv_channel_profile_state_profile_id_default_insert
				AFTER INSERT ON live_tv_channel_profile_state
				WHEN NEW.profile_id = ''
				BEGIN
					UPDATE live_tv_channel_profile_state
					SET profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_live_tv_channel_profile_state_profile_id_default_update
				AFTER UPDATE ON live_tv_channel_profile_state
				WHEN NEW.profile_id = ''
				BEGIN
					UPDATE live_tv_channel_profile_state
					SET profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_live_tv_recording_rules_profile_id_account_insert
				BEFORE INSERT ON live_tv_recording_rules
				WHEN (
					NEW.profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_live_tv_recording_rules_profile_id_account_update
				BEFORE UPDATE ON live_tv_recording_rules
				WHEN (
					NEW.profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_live_tv_recording_rules_profile_id_default_insert
				AFTER INSERT ON live_tv_recording_rules
				WHEN NEW.profile_id = ''
				BEGIN
					UPDATE live_tv_recording_rules
					SET profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_live_tv_recording_rules_profile_id_default_update
				AFTER UPDATE ON live_tv_recording_rules
				WHEN NEW.profile_id = ''
				BEGIN
					UPDATE live_tv_recording_rules
					SET profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_live_tv_recordings_profile_id_account_insert
				BEFORE INSERT ON live_tv_recordings
				WHEN (
					NEW.profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_live_tv_recordings_profile_id_account_update
				BEFORE UPDATE ON live_tv_recordings
				WHEN (
					NEW.profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_live_tv_recordings_profile_id_default_insert
				AFTER INSERT ON live_tv_recordings
				WHEN NEW.profile_id = ''
				BEGIN
					UPDATE live_tv_recordings
					SET profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_live_tv_recordings_profile_id_default_update
				AFTER UPDATE ON live_tv_recordings
				WHEN NEW.profile_id = ''
				BEGIN
					UPDATE live_tv_recordings
					SET profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_local_child_requires_primary_pin
BEFORE INSERT ON profiles
WHEN NEW.origin = 'local' AND NEW.is_primary = 0
    AND NOT EXISTS (
        SELECT 1
        FROM profiles primary_profile
        JOIN local_profile_pin_credentials credential
          ON credential.profile_id = primary_profile.id
        WHERE primary_profile.account_id = NEW.account_id
          AND primary_profile.is_primary = 1
          AND primary_profile.origin = 'local'
          AND primary_profile.disabled_at = ''
    )
BEGIN
    SELECT RAISE(ABORT, 'primary profile PIN is required before adding profiles');
END;
CREATE TRIGGER trg_local_profile_admin_proof_primary_guard
BEFORE INSERT ON local_profile_admin_proofs
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.primary_profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.is_primary = 1
      AND profiles.origin = 'local'
      AND profiles.disabled_at = ''
      AND profiles.pin_revision = NEW.pin_revision
)
BEGIN
    SELECT RAISE(ABORT, 'profile administration proof requires current local primary profile');
END;
CREATE TRIGGER trg_local_profile_admin_proof_revoke_security_update
AFTER UPDATE OF pin_revision, disabled_at ON profiles
BEGIN
    DELETE FROM local_profile_admin_proofs WHERE account_id = NEW.account_id;
END;
CREATE TRIGGER trg_local_profile_pin_origin_guard
BEFORE INSERT ON local_profile_pin_credentials
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id AND profiles.origin = 'local'
)
BEGIN
    SELECT RAISE(ABORT, 'local profile PIN requires a local profile');
END;
CREATE TRIGGER trg_local_profile_pin_update_origin_guard
BEFORE UPDATE ON local_profile_pin_credentials
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id AND profiles.origin = 'local'
)
BEGIN
    SELECT RAISE(ABORT, 'local profile PIN requires a local profile');
END;
CREATE TRIGGER trg_media_download_grants_profile_account_insert
				BEFORE INSERT ON media_download_grants
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.principal_user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_media_download_grants_profile_account_update
				BEFORE UPDATE ON media_download_grants
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.principal_user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_native_refresh_tokens_profile_account_insert
				BEFORE INSERT ON native_refresh_tokens
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_native_refresh_tokens_profile_account_update
				BEFORE UPDATE ON native_refresh_tokens
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_playback_media_grants_profile_account_insert
				BEFORE INSERT ON playback_media_grants
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.principal_user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_playback_media_grants_profile_account_update
				BEFORE UPDATE ON playback_media_grants
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.principal_user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_playback_receivers_profile_insert_guard
BEFORE INSERT ON playback_receivers
WHEN NEW.profile_id = '' OR NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id AND profiles.account_id = NEW.user_id
)
BEGIN
    SELECT RAISE(ABORT, 'playback receiver profile does not belong to account');
END;
CREATE TRIGGER trg_playback_receivers_profile_update_guard
BEFORE UPDATE OF user_id, profile_id ON playback_receivers
WHEN NEW.profile_id = '' OR NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id AND profiles.account_id = NEW.user_id
)
BEGIN
    SELECT RAISE(ABORT, 'playback receiver profile does not belong to account');
END;
CREATE TRIGGER trg_playback_sessions_profile_account_insert
				BEFORE INSERT ON playback_sessions
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_playback_sessions_profile_account_update
				BEFORE UPDATE ON playback_sessions
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_playlists_profile_account_insert
				BEFORE INSERT ON playlists
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_playlists_profile_account_update
				BEFORE UPDATE ON playlists
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_primary_pin_delete_guard
BEFORE DELETE ON local_profile_pin_credentials
WHEN EXISTS (
        SELECT 1 FROM profiles primary_profile
        WHERE primary_profile.id = OLD.profile_id
          AND primary_profile.is_primary = 1
		  AND primary_profile.origin = 'local'
    )
    AND EXISTS (
        SELECT 1 FROM profiles child_profile
        WHERE child_profile.account_id = (
            SELECT account_id FROM profiles WHERE id = OLD.profile_id
        )
          AND child_profile.is_primary = 0
          AND child_profile.disabled_at = ''
    )
BEGIN
    SELECT RAISE(ABORT, 'primary profile PIN cannot be cleared while child profiles exist');
END;
CREATE TRIGGER trg_profile_account_auth_revoke_profile_delete
AFTER DELETE ON profiles
BEGIN
    DELETE FROM profile_account_authentications WHERE account_id = OLD.account_id;
END;
CREATE TRIGGER trg_profile_account_auth_revoke_profile_insert
AFTER INSERT ON profiles
BEGIN
    DELETE FROM profile_account_authentications WHERE account_id = NEW.account_id;
END;
CREATE TRIGGER trg_profile_account_auth_revoke_profile_security_update
AFTER UPDATE OF pin_revision, policy_updated_at, disabled_at ON profiles
BEGIN
    DELETE FROM profile_account_authentications WHERE account_id = NEW.account_id;
END;
CREATE TRIGGER trg_profile_account_auth_revoke_user_security_update
AFTER UPDATE OF password_hash, disabled_at ON users
BEGIN
    DELETE FROM profile_account_authentications WHERE account_id = NEW.id;
END;
CREATE TRIGGER trg_profile_hosted_pin_cleanup
AFTER UPDATE OF origin ON profiles
WHEN OLD.origin = 'local' AND NEW.origin = 'hosted'
BEGIN
    DELETE FROM local_profile_pin_credentials WHERE profile_id = NEW.id;
END;
CREATE TRIGGER trg_profile_selection_grant_local_source_proof
BEFORE INSERT ON profile_selection_grants
WHEN NEW.auth_provider = 'local' AND NEW.account_authentication_id LIKE 'pauth_%'
BEGIN
    SELECT CASE WHEN NOT EXISTS (
        SELECT 1 FROM profile_account_authentications source
        WHERE source.id = NEW.account_authentication_id
          AND source.account_id = NEW.account_id
          AND source.auth_provider = NEW.auth_provider
          AND source.purpose = NEW.purpose
          AND source.device_id = NEW.device_id
          AND source.installation_id = NEW.installation_id
    ) THEN RAISE(ABORT, 'invalid local profile selection source proof') END;
END;
CREATE TRIGGER trg_profile_selection_grants_profile_account_insert
				BEFORE INSERT ON profile_selection_grants
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.account_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_profile_selection_grants_profile_account_update
				BEFORE UPDATE ON profile_selection_grants
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.account_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_profiles_account_immutable
BEFORE UPDATE OF account_id ON profiles
WHEN NEW.account_id <> OLD.account_id
BEGIN
    SELECT RAISE(ABORT, 'profile account ownership is immutable');
END;
CREATE TRIGGER trg_profiles_account_insert_guard
BEFORE INSERT ON profiles
WHEN NEW.account_id = '' OR NOT EXISTS (SELECT 1 FROM users WHERE id = NEW.account_id)
BEGIN
    SELECT RAISE(ABORT, 'profile account does not exist');
END;
CREATE TRIGGER trg_profiles_account_update_guard
BEFORE UPDATE OF account_id ON profiles
WHEN NEW.account_id = '' OR NOT EXISTS (SELECT 1 FROM users WHERE id = NEW.account_id)
BEGIN
    SELECT RAISE(ABORT, 'profile account does not exist');
END;
CREATE TRIGGER trg_profiles_primary_delete_guard
BEFORE DELETE ON profiles
WHEN OLD.is_primary = 1
    AND EXISTS (SELECT 1 FROM users WHERE users.id = OLD.account_id)
BEGIN
    SELECT RAISE(ABORT, 'primary profile cannot be deleted');
END;
CREATE TRIGGER trg_profiles_primary_role_immutable
BEFORE UPDATE OF is_primary ON profiles
WHEN NEW.is_primary <> OLD.is_primary
BEGIN
    SELECT RAISE(ABORT, 'primary profile role is immutable');
END;
CREATE TRIGGER trg_remote_access_members_role_insert_guard
BEFORE INSERT ON remote_access_members
WHEN NEW.role NOT IN ('owner', 'user')
BEGIN
    SELECT RAISE(ABORT, 'remote membership role must be owner or user');
END;
CREATE TRIGGER trg_remote_access_members_role_update_guard
BEFORE UPDATE OF role ON remote_access_members
WHEN NEW.role NOT IN ('owner', 'user')
BEGIN
    SELECT RAISE(ABORT, 'remote membership role must be owner or user');
END;
CREATE TRIGGER trg_saved_views_profile_account_insert
				BEFORE INSERT ON saved_views
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_saved_views_profile_account_update
				BEFORE UPDATE ON saved_views
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_sessions_profile_account_insert
				BEFORE INSERT ON sessions
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_sessions_profile_account_update
				BEFORE UPDATE ON sessions
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_user_display_preferences_profile_account_insert
				BEFORE INSERT ON user_display_preferences
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_user_display_preferences_profile_account_update
				BEFORE UPDATE ON user_display_preferences
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_user_library_navigation_profile_account_insert
				BEFORE INSERT ON user_library_navigation
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_user_library_navigation_profile_account_update
				BEFORE UPDATE ON user_library_navigation
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_user_media_state_profile_account_insert
				BEFORE INSERT ON user_media_state
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_user_media_state_profile_account_update
				BEFORE UPDATE ON user_media_state
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_user_recommendation_cache_profile_account_insert
				BEFORE INSERT ON user_recommendation_cache
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_user_recommendation_cache_profile_account_update
				BEFORE UPDATE ON user_recommendation_cache
				WHEN NEW.profile_id = '' OR NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_users_owner_delete_guard
BEFORE DELETE ON users
WHEN OLD.role = 'owner'
BEGIN
    SELECT RAISE(ABORT, 'server owner cannot be deleted');
END;
CREATE TRIGGER trg_users_owner_demotion_guard
BEFORE UPDATE OF role ON users
WHEN OLD.role = 'owner' AND NEW.role <> 'owner'
BEGIN
    SELECT RAISE(ABORT, 'server owner cannot be demoted');
END;
CREATE TRIGGER trg_users_profiles_delete
AFTER DELETE ON users
BEGIN
    DELETE FROM profiles WHERE account_id = OLD.id;
END;
CREATE TRIGGER trg_users_profiles_insert
		AFTER INSERT ON users
		BEGIN
			INSERT OR IGNORE INTO profiles (id, account_id, origin, is_primary, display_name, role, permissions_json, preferences_json, restrictions_json, max_content_rating, max_active_sessions, remote_bitrate_limit_mbps, created_at, updated_at)
			VALUES (NEW.id, NEW.id, CASE WHEN NEW.auth_origin = 'portico' THEN 'hosted' ELSE 'local' END, 1, NEW.display_name, NEW.role, NEW.permissions_json, COALESCE(NEW.preferences_json, '{}'), '{"version":"v1","maximumAgeRating":null,"allowUnrated":true,"blockedLabels":[],"allowDownloads":true,"allowLiveTV":true,"allowDvr":true,"allowWatchWithFriends":true,"allowFeedback":true}', COALESCE(NEW.max_content_rating, ''), COALESCE(NEW.max_active_sessions, 0), COALESCE(NEW.remote_bitrate_limit_mbps, 0), NEW.created_at, NEW.updated_at);
			INSERT OR IGNORE INTO profile_identities (id, profile_id, provider, subject, email, display_name, verified_at, last_seen_at, created_at, updated_at)
			SELECT 'pident_local_' || NEW.id, NEW.id, 'local', NEW.id, NEW.email, NEW.display_name, NEW.created_at, '', NEW.created_at, NEW.updated_at
			WHERE COALESCE(NEW.password_hash, '') <> '';
			INSERT OR IGNORE INTO local_credentials (id, profile_identity_id, password_hash, recovery_enabled, created_at, revoked_at)
			SELECT 'cred_local_' || NEW.id, 'pident_local_' || NEW.id, NEW.password_hash, 0, NEW.created_at, ''
			WHERE COALESCE(NEW.password_hash, '') <> '';
			INSERT OR IGNORE INTO profile_identities (id, profile_id, provider, subject, email, display_name, verified_at, last_seen_at, created_at, updated_at)
			SELECT 'pident_portico_' || NEW.id, NEW.id, 'portico', NEW.portico_user_id, NEW.email, NEW.display_name, NEW.created_at, '', NEW.created_at, NEW.updated_at
			WHERE COALESCE(NEW.portico_user_id, '') <> '';
		END;
CREATE TRIGGER trg_users_profiles_update
		AFTER UPDATE ON users
		BEGIN
			UPDATE profiles
			SET account_id = NEW.id,
				origin = CASE WHEN NEW.auth_origin = 'portico' THEN 'hosted' ELSE 'local' END,
				is_primary = 1,
				display_name = NEW.display_name,
				role = NEW.role,
				permissions_json = NEW.permissions_json,
				preferences_json = COALESCE(NEW.preferences_json, '{}'),
				max_content_rating = COALESCE(NEW.max_content_rating, ''),
				max_active_sessions = COALESCE(NEW.max_active_sessions, 0),
				remote_bitrate_limit_mbps = COALESCE(NEW.remote_bitrate_limit_mbps, 0),
				updated_at = NEW.updated_at
			WHERE id = NEW.id;
			INSERT OR IGNORE INTO profile_identities (id, profile_id, provider, subject, email, display_name, verified_at, last_seen_at, created_at, updated_at)
			SELECT 'pident_local_' || NEW.id, NEW.id, 'local', NEW.id, NEW.email, NEW.display_name, NEW.created_at, '', NEW.created_at, NEW.updated_at
			WHERE COALESCE(NEW.password_hash, '') <> '';
			UPDATE profile_identities SET email = NEW.email, display_name = NEW.display_name, updated_at = NEW.updated_at WHERE id = 'pident_local_' || NEW.id;
			INSERT OR IGNORE INTO local_credentials (id, profile_identity_id, password_hash, recovery_enabled, created_at, revoked_at)
			SELECT 'cred_local_' || NEW.id, 'pident_local_' || NEW.id, NEW.password_hash, 0, NEW.created_at, ''
			WHERE COALESCE(NEW.password_hash, '') <> '';
			UPDATE local_credentials SET password_hash = NEW.password_hash, revoked_at = '' WHERE id = 'cred_local_' || NEW.id AND COALESCE(NEW.password_hash, '') <> '';
			UPDATE local_credentials SET revoked_at = NEW.updated_at WHERE id = 'cred_local_' || NEW.id AND COALESCE(NEW.password_hash, '') = '';
			INSERT OR IGNORE INTO profile_identities (id, profile_id, provider, subject, email, display_name, verified_at, last_seen_at, created_at, updated_at)
			SELECT 'pident_portico_' || NEW.id, NEW.id, 'portico', NEW.portico_user_id, NEW.email, NEW.display_name, NEW.created_at, '', NEW.created_at, NEW.updated_at
			WHERE COALESCE(NEW.portico_user_id, '') <> '';
			UPDATE profile_identities SET subject = NEW.portico_user_id, email = NEW.email, display_name = NEW.display_name, updated_at = NEW.updated_at WHERE id = 'pident_portico_' || NEW.id AND COALESCE(NEW.portico_user_id, '') <> '';
		END;
CREATE TRIGGER trg_users_role_insert_guard
BEFORE INSERT ON users
WHEN NEW.role NOT IN ('owner', 'user')
BEGIN
    SELECT RAISE(ABORT, 'user role must be owner or user');
END;
CREATE TRIGGER trg_users_role_update_guard
BEFORE UPDATE OF role ON users
WHEN NEW.role NOT IN ('owner', 'user')
BEGIN
    SELECT RAISE(ABORT, 'user role must be owner or user');
END;
CREATE TRIGGER trg_viewer_feedback_profile_insert_guard
BEFORE INSERT ON viewer_feedback
WHEN NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
)
BEGIN
    SELECT RAISE(ABORT, 'feedback profile does not belong to account');
END;
CREATE TRIGGER trg_viewer_notification_feedback_source_guard
BEFORE INSERT ON viewer_notifications
WHEN NEW.source_feedback_id <> '' AND NOT EXISTS (
    SELECT 1 FROM viewer_feedback f
    WHERE f.id = NEW.source_feedback_id
      AND (
          (
              NEW.kind = 'feedback.received'
              AND NEW.audience = 'account-admin'
              AND EXISTS (
                  SELECT 1 FROM users owner
                  WHERE owner.id = NEW.account_id
                    AND owner.role = 'owner'
                    AND COALESCE(owner.disabled_at, '') = ''
              )
          )
          OR (
              NEW.kind = 'feedback.updated'
              AND NEW.audience = 'profile'
              AND f.account_id = NEW.account_id
              AND f.profile_id = NEW.profile_id
          )
      )
)
BEGIN
    SELECT RAISE(ABORT, 'notification feedback source mismatch');
END;
CREATE TRIGGER trg_viewer_notification_feedback_source_update_guard
BEFORE UPDATE OF source_feedback_id, kind, authority, account_id, profile_id, audience ON viewer_notifications
WHEN NEW.source_feedback_id <> '' AND NOT EXISTS (
    SELECT 1 FROM viewer_feedback f
    WHERE f.id = NEW.source_feedback_id
      AND (
          (
              NEW.kind = 'feedback.received'
              AND NEW.audience = 'account-admin'
              AND EXISTS (
                  SELECT 1 FROM users owner
                  WHERE owner.id = NEW.account_id
                    AND owner.role = 'owner'
                    AND COALESCE(owner.disabled_at, '') = ''
              )
          )
          OR (
              NEW.kind = 'feedback.updated'
              AND NEW.audience = 'profile'
              AND f.account_id = NEW.account_id
              AND f.profile_id = NEW.profile_id
          )
      )
)
BEGIN
    SELECT RAISE(ABORT, 'notification feedback source mismatch');
END;
CREATE TRIGGER trg_viewer_notification_profile_insert_guard
BEFORE INSERT ON viewer_notifications
WHEN NEW.profile_id <> '' AND NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
)
BEGIN
    SELECT RAISE(ABORT, 'notification profile does not belong to account');
END;
CREATE TRIGGER trg_viewer_notification_profile_update_guard
BEFORE UPDATE OF account_id, profile_id, audience ON viewer_notifications
WHEN (
    (NEW.audience = 'profile' AND NOT EXISTS (
        SELECT 1 FROM profiles
        WHERE profiles.id = NEW.profile_id
          AND profiles.account_id = NEW.account_id
          AND profiles.disabled_at = ''
    ))
    OR (NEW.audience = 'account-admin' AND NEW.profile_id <> '')
)
BEGIN
    SELECT RAISE(ABORT, 'notification recipient is invalid');
END;
CREATE TRIGGER trg_viewer_notification_receipt_recipient_guard
BEFORE INSERT ON viewer_notification_receipts
WHEN NOT EXISTS (
    SELECT 1 FROM viewer_notifications n
    WHERE n.id = NEW.notification_id
      AND n.authority = NEW.authority
      AND n.account_id = NEW.account_id
      AND n.server_id = NEW.server_id
      AND n.profile_id = NEW.profile_id
      AND n.audience = NEW.audience
)
BEGIN
    SELECT RAISE(ABORT, 'notification receipt recipient mismatch');
END;
CREATE TRIGGER trg_viewer_notification_receipt_update_recipient_guard
BEFORE UPDATE OF notification_id, authority, account_id, server_id, profile_id, audience ON viewer_notification_receipts
WHEN NOT EXISTS (
    SELECT 1 FROM viewer_notifications n
    WHERE n.id = NEW.notification_id
      AND n.authority = NEW.authority
      AND n.account_id = NEW.account_id
      AND n.server_id = NEW.server_id
      AND n.profile_id = NEW.profile_id
      AND n.audience = NEW.audience
)
BEGIN
    SELECT RAISE(ABORT, 'notification receipt recipient mismatch');
END;
CREATE TRIGGER trg_viewer_preference_profile_cleanup
AFTER DELETE ON profiles
BEGIN
    DELETE FROM viewer_preference_documents WHERE profile_id = OLD.id;
END;
CREATE TRIGGER trg_viewer_preference_profile_disable_cleanup
AFTER UPDATE OF disabled_at ON profiles
WHEN OLD.disabled_at = '' AND NEW.disabled_at <> ''
BEGIN
    DELETE FROM viewer_preference_documents WHERE profile_id = NEW.id;
END;
CREATE TRIGGER trg_viewer_preference_profile_insert_guard
BEFORE INSERT ON viewer_preference_documents
WHEN NEW.profile_id <> '' AND NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
)
BEGIN
    SELECT RAISE(ABORT, 'preference profile does not belong to account');
END;
CREATE TRIGGER trg_viewer_preference_profile_update_guard
BEFORE UPDATE OF account_id, profile_id ON viewer_preference_documents
WHEN NEW.profile_id <> '' AND NOT EXISTS (
    SELECT 1 FROM profiles
    WHERE profiles.id = NEW.profile_id
      AND profiles.account_id = NEW.account_id
      AND profiles.disabled_at = ''
)
BEGIN
    SELECT RAISE(ABORT, 'preference profile does not belong to account');
END;
CREATE TRIGGER trg_watch_with_friends_groups_owner_profile_id_account_insert
				BEFORE INSERT ON watch_with_friends_groups
				WHEN (
					NEW.owner_profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.owner_user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.owner_profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.owner_profile_id
					  AND profiles.account_id = NEW.owner_user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_watch_with_friends_groups_owner_profile_id_account_update
				BEFORE UPDATE ON watch_with_friends_groups
				WHEN (
					NEW.owner_profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.owner_user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.owner_profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.owner_profile_id
					  AND profiles.account_id = NEW.owner_user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_watch_with_friends_groups_owner_profile_id_default_insert
				AFTER INSERT ON watch_with_friends_groups
				WHEN NEW.owner_profile_id = ''
				BEGIN
					UPDATE watch_with_friends_groups
					SET owner_profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.owner_user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_watch_with_friends_groups_owner_profile_id_default_update
				AFTER UPDATE ON watch_with_friends_groups
				WHEN NEW.owner_profile_id = ''
				BEGIN
					UPDATE watch_with_friends_groups
					SET owner_profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.owner_user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_watch_with_friends_members_profile_id_account_insert
				BEFORE INSERT ON watch_with_friends_members
				WHEN (
					NEW.profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_watch_with_friends_members_profile_id_account_update
				BEFORE UPDATE ON watch_with_friends_members
				WHEN (
					NEW.profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.profile_id
					  AND profiles.account_id = NEW.user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_watch_with_friends_members_profile_id_default_insert
				AFTER INSERT ON watch_with_friends_members
				WHEN NEW.profile_id = ''
				BEGIN
					UPDATE watch_with_friends_members
					SET profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_watch_with_friends_members_profile_id_default_update
				AFTER UPDATE ON watch_with_friends_members
				WHEN NEW.profile_id = ''
				BEGIN
					UPDATE watch_with_friends_members
					SET profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_watch_with_friends_queue_added_by_profile_id_account_insert
				BEFORE INSERT ON watch_with_friends_queue
				WHEN (
					NEW.added_by_profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.added_by_user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.added_by_profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.added_by_profile_id
					  AND profiles.account_id = NEW.added_by_user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_watch_with_friends_queue_added_by_profile_id_account_update
				BEFORE UPDATE ON watch_with_friends_queue
				WHEN (
					NEW.added_by_profile_id = '' AND NOT EXISTS (
						SELECT 1 FROM profiles
						WHERE profiles.account_id = NEW.added_by_user_id
						  AND profiles.is_primary = 1
						  AND profiles.disabled_at = ''
					)
				) OR (
					NEW.added_by_profile_id <> '' AND NOT EXISTS (
					SELECT 1 FROM profiles
					WHERE profiles.id = NEW.added_by_profile_id
					  AND profiles.account_id = NEW.added_by_user_id
					  AND profiles.disabled_at = ''
					)
				)
				BEGIN
					SELECT RAISE(ABORT, 'profile does not belong to account');
				END;
CREATE TRIGGER trg_watch_with_friends_queue_added_by_profile_id_default_insert
				AFTER INSERT ON watch_with_friends_queue
				WHEN NEW.added_by_profile_id = ''
				BEGIN
					UPDATE watch_with_friends_queue
					SET added_by_profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.added_by_user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;
CREATE TRIGGER trg_watch_with_friends_queue_added_by_profile_id_default_update
				AFTER UPDATE ON watch_with_friends_queue
				WHEN NEW.added_by_profile_id = ''
				BEGIN
					UPDATE watch_with_friends_queue
					SET added_by_profile_id = (
						SELECT id FROM profiles
						WHERE account_id = NEW.added_by_user_id
						  AND is_primary = 1
						  AND disabled_at = ''
						LIMIT 1
					)
					WHERE rowid = NEW.rowid;
				END;

-- Durable traversal state for restart-safe metadata descendant cascades.
CREATE TABLE metadata_continuation_operations (
    id TEXT PRIMARY KEY,
    root_kind TEXT NOT NULL,
    root_id TEXT NOT NULL,
    provider TEXT NOT NULL,
    policy_revision TEXT NOT NULL,
    provider_revision TEXT NOT NULL,
    traversal_phase TEXT NOT NULL,
    traversal_cursor TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL CHECK (status IN ('running','retry_wait','completed','completed_with_failures','failed','cancelled')),
    processed_count INTEGER NOT NULL DEFAULT 0 CHECK (processed_count >= 0),
    remaining_count INTEGER NOT NULL DEFAULT 0 CHECK (remaining_count >= 0),
    failed_count INTEGER NOT NULL DEFAULT 0 CHECK (failed_count >= 0),
    retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    next_retry_at TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    completed_at TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_metadata_continuation_operations_ready ON metadata_continuation_operations(status,next_retry_at,updated_at);
CREATE TABLE metadata_continuation_cursors (
    operation_id TEXT NOT NULL REFERENCES metadata_continuation_operations(id) ON DELETE CASCADE,
    phase TEXT NOT NULL,
    parent_key TEXT NOT NULL DEFAULT '',
    cursor TEXT NOT NULL DEFAULT '',
    exhausted INTEGER NOT NULL DEFAULT 0 CHECK (exhausted IN (0,1)),
    page_count INTEGER NOT NULL DEFAULT 0 CHECK (page_count >= 0),
    updated_at TEXT NOT NULL,
    PRIMARY KEY (operation_id,phase,parent_key)
);
CREATE TABLE metadata_continuation_pages (
    operation_id TEXT NOT NULL REFERENCES metadata_continuation_operations(id) ON DELETE CASCADE,
    phase TEXT NOT NULL,
    parent_key TEXT NOT NULL DEFAULT '',
    cursor TEXT NOT NULL DEFAULT '',
    next_cursor TEXT NOT NULL DEFAULT '',
    exhausted INTEGER NOT NULL CHECK (exhausted IN (0,1)),
    created_at TEXT NOT NULL,
    PRIMARY KEY (operation_id,phase,parent_key,cursor)
);
CREATE TABLE metadata_continuation_items (
    operation_id TEXT NOT NULL REFERENCES metadata_continuation_operations(id) ON DELETE CASCADE,
    item_key TEXT NOT NULL,
    parent_key TEXT NOT NULL DEFAULT '',
    item_kind TEXT NOT NULL,
    provider_id TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}',
    state TEXT NOT NULL CHECK (state IN ('pending','processing','retry_wait','succeeded','failed')),
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    next_retry_at TEXT NOT NULL DEFAULT '',
    lease_until TEXT NOT NULL DEFAULT '',
    last_error TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (operation_id,item_key)
);
CREATE INDEX idx_metadata_continuation_items_ready ON metadata_continuation_items(operation_id,state,next_retry_at,lease_until,created_at);
