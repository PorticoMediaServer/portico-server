ALTER TABLE live_tv_programs ADD COLUMN import_generation TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_live_tv_programs_source_generation ON live_tv_programs(source_id, import_generation);
