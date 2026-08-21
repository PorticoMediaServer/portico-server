ALTER TABLE cast_bootstraps ADD COLUMN source_playback_session_id TEXT NOT NULL DEFAULT '';
ALTER TABLE cast_receiver_sessions ADD COLUMN source_playback_session_id TEXT NOT NULL DEFAULT '';

