CREATE TABLE IF NOT EXISTS user_library_navigation (
    user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    pinned INTEGER NOT NULL DEFAULT 1 CHECK (pinned IN (0, 1)),
    sort_order INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (user_id, library_id)
);

CREATE INDEX IF NOT EXISTS idx_user_library_navigation_order
ON user_library_navigation(user_id, pinned DESC, sort_order, library_id);

INSERT INTO user_library_navigation (user_id, library_id, pinned, sort_order, created_at, updated_at)
SELECT
    users.id,
    libraries.id,
    1,
    libraries.sort_order,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM users
CROSS JOIN libraries
WHERE users.role IN ('owner', 'admin')
ON CONFLICT(user_id, library_id) DO NOTHING;
