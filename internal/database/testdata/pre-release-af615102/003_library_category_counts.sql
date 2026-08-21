CREATE TABLE IF NOT EXISTS media_category_facets (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    library_id TEXT NOT NULL DEFAULT '',
    facet_type TEXT NOT NULL,
    value TEXT NOT NULL,
    sort_value TEXT NOT NULL,
    updated_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (media_id, facet_type, value)
);

CREATE INDEX IF NOT EXISTS idx_media_category_facets_library ON media_category_facets(library_id, facet_type, sort_value);

UPDATE media_category_facets
SET library_id = COALESCE((SELECT media_items.library_id FROM media_items WHERE media_items.id = media_category_facets.media_id), '')
WHERE EXISTS (
    SELECT 1
    FROM media_items
    WHERE media_items.id = media_category_facets.media_id
        AND COALESCE(media_items.library_id, '') <> ''
        AND media_items.library_id <> media_category_facets.library_id
);

DELETE FROM media_category_facets
WHERE NOT EXISTS (
        SELECT 1
        FROM media_items
        WHERE media_items.id = media_category_facets.media_id
    )
    OR NOT EXISTS (
        SELECT 1
        FROM libraries
        WHERE libraries.id = media_category_facets.library_id
    );

CREATE TABLE IF NOT EXISTS library_category_counts (
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    filter TEXT NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    representative_media_id TEXT NOT NULL DEFAULT '',
    representative_image TEXT NOT NULL DEFAULT '',
    updated_at TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (library_id, filter)
);

CREATE INDEX IF NOT EXISTS idx_library_category_counts_library ON library_category_counts(library_id, count DESC, filter);

INSERT OR REPLACE INTO library_category_counts (library_id, filter, count, representative_media_id, representative_image, updated_at)
SELECT
    f.library_id,
    lower(f.facet_type || ':' || trim(f.value)),
    COUNT(*),
    MIN(f.media_id),
    '',
    strftime('%Y-%m-%dT%H:%M:%fZ','now')
FROM media_category_facets f
JOIN libraries l ON l.id = f.library_id
WHERE trim(f.value) <> ''
GROUP BY f.library_id, f.facet_type, trim(f.value);
