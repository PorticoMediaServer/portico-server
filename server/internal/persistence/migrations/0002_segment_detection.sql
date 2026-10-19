-- Intro and credits detection (mediaanalysis segments). New tables only, so it
-- applies to any database at the baseline, after any other migration.

-- One row per media file the detector has measured or must measure; the
-- measurement itself is an analysis result ('segment_evidence') whose artifact
-- holds the fingerprints. show_id groups a show's episodes for comparison (0 for
-- films); match_digest names the evidence a file's season markers were last
-- made from.
CREATE TABLE segment_files (
 object_id TEXT PRIMARY KEY, source_revision TEXT NOT NULL, library_id TEXT NOT NULL,
 show_id INTEGER NOT NULL DEFAULT 0, mode TEXT NOT NULL CHECK(mode IN('standard','thorough')),
 state TEXT NOT NULL CHECK(state IN('pending','complete','failed','unsupported')),
 attempt INTEGER NOT NULL DEFAULT 0, next_ms INTEGER NOT NULL DEFAULT 0, error_code TEXT NOT NULL DEFAULT '',
 result_id TEXT NOT NULL DEFAULT '', match_digest TEXT NOT NULL DEFAULT '', updated_ms INTEGER NOT NULL
) STRICT;
-- A show whose episodes' evidence changed since it was last compared.
CREATE TABLE segment_shows (
 show_id INTEGER PRIMARY KEY, dirty INTEGER NOT NULL DEFAULT 1 CHECK(dirty IN(0,1)),
 matched_ms INTEGER NOT NULL DEFAULT 0, updated_ms INTEGER NOT NULL
) STRICT;
-- How far the worker has looked: new and changed files arrive through
-- inventory_changes (change_sequence); a full pass over the inventory (cursor)
-- runs when the detection modes change (modes is their digest) and daily.
CREATE TABLE segment_catchup (
 singleton INTEGER PRIMARY KEY CHECK(singleton=1), cursor TEXT NOT NULL DEFAULT '',
 settled INTEGER NOT NULL DEFAULT 0 CHECK(settled IN(0,1)), pass_ms INTEGER NOT NULL DEFAULT 0,
 change_sequence INTEGER NOT NULL DEFAULT 0, modes TEXT NOT NULL DEFAULT ''
) STRICT;
CREATE INDEX segment_files_work ON segment_files(state,next_ms,show_id,object_id);
CREATE INDEX segment_files_show ON segment_files(show_id,state);
CREATE INDEX segment_shows_dirty ON segment_shows(dirty,show_id) WHERE dirty=1;
