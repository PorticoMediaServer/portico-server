CREATE TABLE IF NOT EXISTS profile_search_history (
    profile_id TEXT NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
    normalized_query TEXT NOT NULL,
    query TEXT NOT NULL,
    use_count INTEGER NOT NULL DEFAULT 1,
    last_used_at TEXT NOT NULL,
    PRIMARY KEY (profile_id, normalized_query)
);

CREATE INDEX IF NOT EXISTS idx_profile_search_history_recent
    ON profile_search_history(profile_id, last_used_at DESC);

CREATE INDEX IF NOT EXISTS idx_media_people_normalized_name
    ON media_people(lower(trim(name)), sort_order, media_id);

CREATE INDEX IF NOT EXISTS idx_media_browse_personal_rating
    ON user_media_state(profile_id, rating, media_id);

ALTER TABLE media_people
    ADD COLUMN canonical_person_key TEXT NOT NULL DEFAULT '';

UPDATE media_people
SET canonical_person_key = (
    SELECT 'provider:' || lower(hex(lower(trim(provider.key)) || char(31) || trim(CAST(provider.value AS TEXT))))
     FROM json_each(CASE WHEN json_valid(media_people.provider_ids_json) THEN media_people.provider_ids_json ELSE '{}' END) provider
     WHERE trim(provider.key) <> '' AND trim(CAST(provider.value AS TEXT)) <> ''
     ORDER BY lower(trim(provider.key)), trim(CAST(provider.value AS TEXT)) LIMIT 1
)
WHERE trim(canonical_person_key) = ''
  AND trim(name) <> ''
  AND trim(role) <> ''
  AND EXISTS (
      SELECT 1
      FROM json_each(CASE WHEN json_valid(media_people.provider_ids_json) THEN media_people.provider_ids_json ELSE '{}' END) provider
      WHERE trim(provider.key) <> '' AND trim(CAST(provider.value AS TEXT)) <> ''
  );

CREATE INDEX IF NOT EXISTS idx_media_people_canonical_person
    ON media_people(canonical_person_key, media_id)
    WHERE canonical_person_key <> '';

-- Audiobook authors and series are first-class browse entities. Their public
-- identifiers are deliberately random and durable: display names and scanner
-- ordering can change without invalidating saved URLs. A strong metadata key
-- may reconcile several books; weak metadata remains one local entity per
-- durable media member so identical names can never silently merge namesakes.
CREATE TABLE IF NOT EXISTS audiobook_browse_entities (
    id TEXT PRIMARY KEY,
    library_id TEXT NOT NULL REFERENCES libraries(id) ON DELETE CASCADE,
    entity_kind TEXT NOT NULL CHECK (entity_kind IN ('author', 'audiobook-series')),
    identity_key TEXT NOT NULL,
    display_name TEXT NOT NULL,
    normalized_name TEXT NOT NULL,
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    UNIQUE (library_id, entity_kind, identity_key)
);

CREATE INDEX IF NOT EXISTS idx_audiobook_browse_entities_page
    ON audiobook_browse_entities(library_id, entity_kind, normalized_name, id);

CREATE TABLE IF NOT EXISTS audiobook_browse_entity_members (
    entity_id TEXT NOT NULL REFERENCES audiobook_browse_entities(id) ON DELETE CASCADE,
    entity_kind TEXT NOT NULL CHECK (entity_kind IN ('author', 'audiobook-series')),
    media_id TEXT NOT NULL REFERENCES media_items(id) ON DELETE CASCADE,
    evidence_key TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL,
    PRIMARY KEY (entity_id, media_id),
    UNIQUE (entity_kind, media_id)
);

CREATE INDEX IF NOT EXISTS idx_audiobook_browse_members_entity
    ON audiobook_browse_entity_members(entity_kind, entity_id, media_id);
