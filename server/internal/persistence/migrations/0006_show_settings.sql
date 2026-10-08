-- 0006: how one show's episodes are presented (Spec — Title Pages §4), set by the owner from the
-- show's page: hide the season switcher, number episodes across seasons, split a long list into
-- ranges of a hundred, and list the newest first. A show without a row has the defaults (all off).
CREATE TABLE catalog_show_settings(
 entity_id INTEGER PRIMARY KEY REFERENCES catalog_entities(id) ON DELETE CASCADE,
 hide_seasons INTEGER NOT NULL DEFAULT 0 CHECK(hide_seasons IN(0,1)),
 absolute_numbering INTEGER NOT NULL DEFAULT 0 CHECK(absolute_numbering IN(0,1)),
 ranges INTEGER NOT NULL DEFAULT 0 CHECK(ranges IN(0,1)),
 newest_first INTEGER NOT NULL DEFAULT 0 CHECK(newest_first IN(0,1)),
 revision INTEGER NOT NULL DEFAULT 1) STRICT;
