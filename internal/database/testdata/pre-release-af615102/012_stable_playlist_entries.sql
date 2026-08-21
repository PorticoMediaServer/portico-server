CREATE TABLE playlist_items_next (
    entry_id TEXT PRIMARY KEY NOT NULL DEFAULT ('pentry_' || lower(hex(randomblob(16)))),
    playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    sort_order INTEGER NOT NULL DEFAULT 0,
    added_at TEXT NOT NULL
);

INSERT INTO playlist_items_next (entry_id, playlist_id, media_id, sort_order, added_at)
SELECT
    'pentry_' || lower(hex(randomblob(16))),
    playlist_id,
    media_id,
    ROW_NUMBER() OVER (
        PARTITION BY playlist_id
        ORDER BY sort_order ASC, added_at ASC, media_id ASC
    ),
    added_at
FROM playlist_items;

DROP TABLE playlist_items;
ALTER TABLE playlist_items_next RENAME TO playlist_items;

CREATE INDEX idx_playlist_items_order ON playlist_items(playlist_id, sort_order, entry_id);
CREATE INDEX idx_playlist_items_media ON playlist_items(playlist_id, media_id);
