-- Persistent, cross-process source capacity ownership shared by Live playback
-- and DVR workers. Leases are recoverable after crashes and provider stream
-- keys permit intentional shared-stream semantics where supported.
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

CREATE INDEX idx_live_tv_tuner_allocations_source
    ON live_tv_tuner_allocations(source_id, allocation_key, allocation_kind, heartbeat_at);

CREATE INDEX idx_live_tv_tuner_allocations_expiry
    ON live_tv_tuner_allocations(source_id, expires_at, heartbeat_at);
