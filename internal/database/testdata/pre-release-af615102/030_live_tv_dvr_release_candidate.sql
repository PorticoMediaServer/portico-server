ALTER TABLE live_tv_sources ADD COLUMN tuner_count INTEGER NOT NULL DEFAULT 1 CHECK (tuner_count BETWEEN 1 AND 64);
ALTER TABLE live_tv_sources ADD COLUMN discovered_tuner_count INTEGER NOT NULL DEFAULT 0 CHECK (discovered_tuner_count BETWEEN 0 AND 64);
ALTER TABLE live_tv_sources ADD COLUMN tuner_count_mode TEXT NOT NULL DEFAULT 'default' CHECK (tuner_count_mode IN ('default', 'discovered', 'overridden'));

ALTER TABLE live_tv_recording_rules ADD COLUMN priority INTEGER NOT NULL DEFAULT 50 CHECK (priority BETWEEN 0 AND 100);
ALTER TABLE live_tv_recording_rules ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0);

ALTER TABLE live_tv_recordings ADD COLUMN priority INTEGER NOT NULL DEFAULT 50 CHECK (priority BETWEEN 0 AND 100);
ALTER TABLE live_tv_recordings ADD COLUMN failure_code TEXT NOT NULL DEFAULT '';
ALTER TABLE live_tv_recordings ADD COLUMN revision INTEGER NOT NULL DEFAULT 1 CHECK (revision > 0);

CREATE INDEX IF NOT EXISTS idx_recording_rules_priority
    ON live_tv_recording_rules(source_id, enabled, priority DESC, updated_at DESC, id);

CREATE INDEX IF NOT EXISTS idx_recordings_allocation
    ON live_tv_recordings(source_id, status, starts_at, ends_at, priority DESC, id);
