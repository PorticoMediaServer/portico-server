-- 0007: the server's play history, and the counts kept from it.
--
-- play_history is the owner's record of what was played on this server: who, what, on which
-- device, when and how far. It is separate from personal_history, which is a viewer's own list
-- (they may pause or clear it) and feeds their recommendations. A row here is written once a
-- play has really started, is updated as the play goes on, and is kept until the owner's
-- retention setting (playHistoryDays; 0 keeps everything) removes it.
--
-- The names are copied in, and item_id has no foreign key: a row still reads "Family Guy ·
-- S7 E13 · Stew-Roids, on Rat vision" after the title, the account or the device is gone.
CREATE TABLE play_history(
 id INTEGER PRIMARY KEY AUTOINCREMENT,
 playback_id TEXT NOT NULL UNIQUE,
 authority TEXT NOT NULL,
 account_id TEXT NOT NULL,
 profile_id TEXT NOT NULL,
 user_name TEXT NOT NULL,
 profile_name TEXT NOT NULL DEFAULT '',
 item_id INTEGER NOT NULL,
 library_id TEXT NOT NULL,
 kind INTEGER NOT NULL,
 title TEXT NOT NULL,
 parent_title TEXT NOT NULL DEFAULT '',
 season INTEGER,
 episode INTEGER,
 device_id TEXT NOT NULL DEFAULT '',
 device_name TEXT NOT NULL DEFAULT '',
 platform TEXT NOT NULL DEFAULT '',
 started_ms INTEGER NOT NULL,
 updated_ms INTEGER NOT NULL,
 position_ms INTEGER NOT NULL DEFAULT 0,
 duration_ms INTEGER NOT NULL DEFAULT 0,
 completed INTEGER NOT NULL DEFAULT 0 CHECK(completed IN(0,1))) STRICT;
-- Newest first, whole server; then the same walk for one account and for one library.
CREATE INDEX play_history_recent ON play_history(started_ms DESC,id DESC);
CREATE INDEX play_history_account ON play_history(account_id,started_ms DESC,id DESC);
CREATE INDEX play_history_library ON play_history(library_id,started_ms DESC,id DESC);

-- How many times a title has been played to the end: on this server, and by each viewer.
-- A count is an aggregate, so it outlives the history rows the retention setting removes.
CREATE TABLE play_counts(
 item_id INTEGER PRIMARY KEY REFERENCES catalog_entities(id) ON DELETE CASCADE,
 plays INTEGER NOT NULL,
 last_played_ms INTEGER NOT NULL) STRICT;
CREATE TABLE personal_play_counts(
 profile_id TEXT NOT NULL,
 item_id INTEGER NOT NULL REFERENCES catalog_entities(id) ON DELETE CASCADE,
 plays INTEGER NOT NULL,
 PRIMARY KEY(profile_id,item_id)) STRICT, WITHOUT ROWID;
CREATE INDEX personal_play_counts_item ON personal_play_counts(item_id);

-- Seed the counts from the history viewers already have, so "Plays" and "Popular" do not start
-- from nothing. The server-wide log itself starts now: the older rows never recorded a device.
INSERT INTO play_counts(item_id,plays,last_played_ms)
 SELECT item_id,count(*),COALESCE(CAST(strftime('%s',max(updated_at)) AS INTEGER),0)*1000 FROM personal_history WHERE completed=1 GROUP BY item_id;
INSERT INTO personal_play_counts(profile_id,item_id,plays)
 SELECT profile_id,item_id,count(*) FROM personal_history WHERE completed=1 GROUP BY profile_id,item_id;

-- Home's "Featured" row is gone (the hero is what the viewer was last in the middle of), and
-- this index served nothing else.
DROP INDEX IF EXISTS catalog_browse_feature;
