CREATE TABLE IF NOT EXISTS saved_views (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    pivot TEXT NOT NULL,
    query_json TEXT NOT NULL DEFAULT '',
    sort_json TEXT NOT NULL DEFAULT '[]',
    presentation_json TEXT NOT NULL DEFAULT '[]',
    is_pinned INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_saved_views_user_updated ON saved_views(user_id, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_saved_views_library ON saved_views(library_id, user_id);
