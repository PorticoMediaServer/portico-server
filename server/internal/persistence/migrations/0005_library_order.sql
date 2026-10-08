-- 0005: an owner can put the libraries in their own order (the rail, the library switcher,
-- Home's per-library rows). Zero means "not ordered yet": such libraries sort by name, as before.
ALTER TABLE libraries ADD COLUMN position INTEGER NOT NULL DEFAULT 0;
