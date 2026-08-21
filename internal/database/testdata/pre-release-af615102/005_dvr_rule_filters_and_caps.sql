ALTER TABLE live_tv_recording_rules ADD COLUMN max_recordings_per_series INTEGER NOT NULL DEFAULT 0;
ALTER TABLE live_tv_recording_rules ADD COLUMN required_keywords TEXT NOT NULL DEFAULT '[]';
ALTER TABLE live_tv_recording_rules ADD COLUMN blocked_keywords TEXT NOT NULL DEFAULT '[]';
ALTER TABLE live_tv_recording_rules ADD COLUMN allowed_channels TEXT NOT NULL DEFAULT '[]';
ALTER TABLE live_tv_recording_rules ADD COLUMN blocked_channels TEXT NOT NULL DEFAULT '[]';
