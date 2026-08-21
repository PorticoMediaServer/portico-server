-- Reserve the migration boundary for profile-owned collaborative, DVR, and
-- Live TV viewer data. These tables are intentionally created by the database
-- compatibility pass after SQL migrations so clean installs and legacy
-- databases share one table-rebuild implementation in database.go.
SELECT 1;
