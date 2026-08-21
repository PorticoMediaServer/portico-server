ALTER TABLE cast_bootstraps ADD COLUMN automation_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE cast_receiver_sessions ADD COLUMN automation_json TEXT NOT NULL DEFAULT '{}';
ALTER TABLE cast_receiver_sessions ADD COLUMN automatic_advances INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cast_receiver_sessions ADD COLUMN last_advance_id TEXT NOT NULL DEFAULT '';
ALTER TABLE cast_receiver_sessions ADD COLUMN last_advance_json TEXT NOT NULL DEFAULT '';
