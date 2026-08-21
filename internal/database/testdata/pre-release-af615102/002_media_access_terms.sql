CREATE TABLE IF NOT EXISTS media_access_tags (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    normalized_tag TEXT NOT NULL,
    PRIMARY KEY (media_id, normalized_tag)
);

CREATE INDEX IF NOT EXISTS idx_media_access_tags_tag ON media_access_tags(normalized_tag, media_id);

CREATE TABLE IF NOT EXISTS media_access_labels (
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    normalized_label TEXT NOT NULL,
    PRIMARY KEY (media_id, normalized_label)
);

CREATE INDEX IF NOT EXISTS idx_media_access_labels_label ON media_access_labels(normalized_label, media_id);

INSERT OR IGNORE INTO media_access_tags (media_id, normalized_tag)
SELECT m.id, lower(trim(tag.value))
FROM media_items m,
     json_each(CASE WHEN json_valid(m.tags_json) THEN m.tags_json ELSE '[]' END) tag
WHERE lower(trim(tag.value)) <> '';

INSERT OR IGNORE INTO media_access_labels (media_id, normalized_label)
SELECT m.id, lower(trim(label.value))
FROM media_items m,
     json_each(CASE WHEN json_valid(m.labels_json) THEN m.labels_json ELSE '[]' END) label
WHERE lower(trim(label.value)) <> '';

CREATE TRIGGER IF NOT EXISTS media_access_terms_after_insert
AFTER INSERT ON media_items
BEGIN
    DELETE FROM media_access_tags WHERE media_id = NEW.id;
    INSERT OR IGNORE INTO media_access_tags (media_id, normalized_tag)
    SELECT NEW.id, lower(trim(tag.value))
    FROM json_each(CASE WHEN json_valid(NEW.tags_json) THEN NEW.tags_json ELSE '[]' END) tag
    WHERE lower(trim(tag.value)) <> '';

    DELETE FROM media_access_labels WHERE media_id = NEW.id;
    INSERT OR IGNORE INTO media_access_labels (media_id, normalized_label)
    SELECT NEW.id, lower(trim(label.value))
    FROM json_each(CASE WHEN json_valid(NEW.labels_json) THEN NEW.labels_json ELSE '[]' END) label
    WHERE lower(trim(label.value)) <> '';
END;

CREATE TRIGGER IF NOT EXISTS media_access_terms_after_update
AFTER UPDATE OF tags_json, labels_json ON media_items
BEGIN
    DELETE FROM media_access_tags WHERE media_id = NEW.id;
    INSERT OR IGNORE INTO media_access_tags (media_id, normalized_tag)
    SELECT NEW.id, lower(trim(tag.value))
    FROM json_each(CASE WHEN json_valid(NEW.tags_json) THEN NEW.tags_json ELSE '[]' END) tag
    WHERE lower(trim(tag.value)) <> '';

    DELETE FROM media_access_labels WHERE media_id = NEW.id;
    INSERT OR IGNORE INTO media_access_labels (media_id, normalized_label)
    SELECT NEW.id, lower(trim(label.value))
    FROM json_each(CASE WHEN json_valid(NEW.labels_json) THEN NEW.labels_json ELSE '[]' END) label
    WHERE lower(trim(label.value)) <> '';
END;
