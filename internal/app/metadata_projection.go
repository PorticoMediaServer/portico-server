package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// replaceMediaCategoryFacetsTx replaces the facet projection without crossing
// the caller's transaction boundary. revision and source describe the
// canonical metadata snapshot from which the rows were derived.
func replaceMediaCategoryFacetsTx(ctx context.Context, tx *sql.Tx, mediaID string, revision int, source string) error {
	return replaceMediaCategoryFacetRowsTx(ctx, tx, mediaID, revision, source, true)
}

// replaceMediaCategoryFacetRowsTx owns the shared row projection used by both
// ordinary one-item metadata updates and whole-library repair. Ordinary
// updates rebuild the library aggregate inside the same transaction so readers
// never observe mismatched rows and counts. Library repair deliberately passes
// rebuildCounts=false for its bounded item batches and rebuilds the aggregate
// once, after every item row has converged; rebuilding the whole aggregate for
// every repaired item is quadratic on large libraries.
func replaceMediaCategoryFacetRowsTx(ctx context.Context, tx *sql.Tx, mediaID string, revision int, source string, rebuildCounts bool) error {
	if tx == nil {
		return errors.New("replace media category facets: nil transaction")
	}
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return nil
	}
	if revision < 0 {
		return errors.New("replace media category facets: negative metadata revision")
	}
	source = strings.TrimSpace(source)
	now := time.Now().UTC().Format(time.RFC3339)
	var libraryID string
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(library_id, '') FROM media_items WHERE id = ?`, mediaID).Scan(&libraryID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_category_facets WHERE media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("clear media category facets: %w", err)
	}
	_, err := tx.ExecContext(ctx, `
		INSERT OR IGNORE INTO media_category_facets
			(media_id, library_id, facet_type, value, sort_value, updated_at, metadata_revision, source, ordinal)
		SELECT media_id, library_id, facet_type, value, lower(value), ?, ?, ?, ordinal
		FROM (
			SELECT m.id media_id, COALESCE(m.library_id, '') library_id, 'genre' facet_type, trim(j.value) value, CAST(j.key AS INTEGER) ordinal
			FROM media_items m, json_each(CASE WHEN json_valid(m.genres_json) THEN m.genres_json ELSE '[]' END) j WHERE m.id = ?
			UNION ALL SELECT m.id, COALESCE(m.library_id, ''), 'tag', trim(j.value), CAST(j.key AS INTEGER)
			FROM media_items m, json_each(CASE WHEN json_valid(m.tags_json) THEN m.tags_json ELSE '[]' END) j WHERE m.id = ?
			UNION ALL SELECT m.id, COALESCE(m.library_id, ''), 'accessLabel', trim(j.value), CAST(j.key AS INTEGER)
			FROM media_items m, json_each(CASE WHEN json_valid(m.labels_json) THEN m.labels_json ELSE '[]' END) j WHERE m.id = ?
			UNION ALL SELECT id, COALESCE(library_id, ''), 'artist', trim(COALESCE(NULLIF(json_extract(typed_metadata_json, '$.trackArtist'), ''), NULLIF(json_extract(typed_metadata_json, '$.albumArtist'), ''), NULLIF(json_extract(typed_metadata_json, '$.artist'), ''))), 0
			FROM media_items WHERE id = ? AND lower(type) IN ('audio', 'track', 'song', 'album', 'music')
			UNION ALL SELECT id, COALESCE(library_id, ''), 'albumArtist', trim(COALESCE(NULLIF(json_extract(typed_metadata_json, '$.albumArtist'), ''), NULLIF(json_extract(typed_metadata_json, '$.artist'), ''))), 0
			FROM media_items WHERE id = ? AND lower(type) IN ('audio', 'track', 'song', 'album', 'music')
			UNION ALL SELECT id, COALESCE(library_id, ''), 'author', trim(COALESCE(json_extract(typed_metadata_json, '$.author'), '')), 0
			FROM media_items WHERE id = ? AND lower(type) IN ('audiobook', 'book')
			UNION ALL SELECT id, COALESCE(library_id, ''), 'narrator', trim(COALESCE(json_extract(typed_metadata_json, '$.narrator'), '')), 0
			FROM media_items WHERE id = ? AND lower(type) IN ('audiobook', 'book')
			UNION ALL SELECT id, COALESCE(library_id, ''), 'series', trim(COALESCE(json_extract(typed_metadata_json, '$.series'), '')), 0
			FROM media_items WHERE id = ? AND lower(type) IN ('audiobook', 'book')
			UNION ALL SELECT id, COALESCE(library_id, ''), 'label', trim(COALESCE(json_extract(typed_metadata_json, '$.label'), '')), 0 FROM media_items WHERE id = ?
			UNION ALL SELECT id, COALESCE(library_id, ''), 'network', trim(COALESCE(NULLIF(json_extract(typed_metadata_json, '$.network'), ''), network, '')), 0
			FROM media_items WHERE id = ? AND lower(type) IN ('show', 'anime', 'season', 'episode')
			UNION ALL SELECT id, COALESCE(library_id, ''), 'studio', trim(COALESCE(NULLIF(json_extract(typed_metadata_json, '$.studio'), ''), studio, '')), 0
			FROM media_items WHERE id = ? AND lower(type) IN ('movie', 'film', 'video')
			UNION ALL SELECT id, COALESCE(library_id, ''), 'show', trim(title), 0 FROM media_items WHERE id = ? AND lower(type) IN ('show', 'anime')
			UNION ALL SELECT c.id, COALESCE(c.library_id, ''), 'show', trim(p.title), 0 FROM media_items c JOIN media_items p ON p.id = c.parent_id WHERE c.id = ? AND lower(c.type) = 'season'
			UNION ALL SELECT e.id, COALESCE(e.library_id, ''), 'show', trim(g.title), 0 FROM media_items e JOIN media_items p ON p.id = e.parent_id JOIN media_items g ON g.id = p.parent_id WHERE e.id = ? AND lower(e.type) = 'episode'
			UNION ALL SELECT id, COALESCE(library_id, ''), 'season', trim(title), 0 FROM media_items WHERE id = ? AND lower(type) = 'season'
			UNION ALL SELECT e.id, COALESCE(e.library_id, ''), 'season', trim(p.title), 0 FROM media_items e JOIN media_items p ON p.id = e.parent_id WHERE e.id = ? AND lower(e.type) = 'episode'
			UNION ALL SELECT rel.media_id, COALESCE(m.library_id, ''),
				CASE rel.relationship_type
					WHEN 'keyword' THEN 'keyword' WHEN 'country' THEN 'country' WHEN 'language' THEN 'language'
					WHEN 'spokenLanguage' THEN 'language'
					WHEN 'network' THEN 'network' WHEN 'studio' THEN 'studio' WHEN 'company' THEN 'company'
					WHEN 'collection' THEN 'collection' WHEN 'franchise' THEN 'franchise'
					WHEN 'creator' THEN 'creator' WHEN 'label' THEN 'label' WHEN 'format' THEN 'format'
					WHEN 'certification' THEN 'contentRating' WHEN 'contentRating' THEN 'contentRating' ELSE rel.relationship_type END,
				trim(CASE
					WHEN rel.display_value <> '' THEN rel.display_value
					WHEN rel.relationship_type IN ('language','spokenLanguage') THEN rel.language
					WHEN rel.relationship_type = 'country' THEN rel.country
					ELSE rel.target_external_id END), rel.ordinal
			FROM media_metadata_relationships rel
			JOIN media_metadata_revisions rev ON rev.id = rel.revision_id
			JOIN media_items m ON m.id = rel.media_id
			WHERE rel.media_id = ? AND rev.revision = ? AND rel.decision IN ('accepted','locked')
				AND rel.relationship_type IN ('keyword','country','language','spokenLanguage','network','studio','company','collection','franchise','creator','label','format','certification','contentRating')
		) WHERE value <> ''`, now, revision, source,
		mediaID, mediaID, mediaID, mediaID, mediaID, mediaID, mediaID, mediaID,
		mediaID, mediaID, mediaID, mediaID, mediaID, mediaID, mediaID, mediaID, mediaID, revision)
	if err != nil {
		return fmt.Errorf("replace media category facets: %w", err)
	}
	if err := normalizeAccessLabelFacetSortValuesTx(tx, "media_id", mediaID); err != nil {
		return err
	}
	if rebuildCounts && libraryID != "" {
		// A one-item projection changes a library-wide materialized view. Rebuild
		// it before the caller commits so readers can never observe a partially
		// invalidated category cache, and rollback restores both projections.
		if err := rebuildLibraryCategoryCountsTx(tx, libraryID, now); err != nil {
			return fmt.Errorf("rebuild library category counts: %w", err)
		}
	}
	return nil
}

// replaceMediaSearchTx rebuilds one FTS document from a single coherent
// canonical revision while remaining inside the caller's transaction.
func replaceMediaSearchTx(ctx context.Context, tx *sql.Tx, mediaID string, revision int, source string) error {
	if tx == nil {
		return errors.New("replace media search: nil transaction")
	}
	mediaID = strings.TrimSpace(mediaID)
	if mediaID == "" {
		return nil
	}
	if revision < 0 {
		return errors.New("replace media search: negative metadata revision")
	}
	_ = strings.TrimSpace(source) // retained as part of the common projection contract
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_search WHERE media_id = ?`, mediaID); err != nil {
		return fmt.Errorf("clear media search: %w", err)
	}
	// Exact revision predicates prevent a projection from mixing evidence
	// written by two refresh attempts. Canonical media_items fields are updated
	// by the caller in this same transaction before this helper runs.
	_, err := tx.ExecContext(ctx, `
		INSERT INTO media_search
			(media_id, title, summary, genres, alternate_titles, people, relationships, identifiers, keywords)
		SELECT m.id,
			trim(m.title || ' ' || m.sort_title || ' ' || m.original_title || ' ' || m.edition),
			trim(m.summary || ' ' || m.tagline),
			trim(COALESCE((SELECT group_concat(value, ' ') FROM json_each(CASE WHEN json_valid(m.genres_json) THEN m.genres_json ELSE '[]' END)), '')),
			trim(COALESCE((SELECT group_concat(r.display_value, ' ') FROM media_metadata_relationships r JOIN media_metadata_revisions v ON v.id = r.revision_id WHERE r.media_id = m.id AND v.revision = ? AND r.decision IN ('accepted','locked') AND r.relationship_type = 'alternate_title'), '') || ' ' || COALESCE((SELECT group_concat(json_extract(f.value_json, '$'), ' ') FROM media_metadata_field_values f JOIN media_metadata_revisions v ON v.id = f.revision_id WHERE f.media_id = m.id AND v.revision = ? AND f.decision IN ('accepted','locked') AND lower(f.field_key) IN ('alternatetitle','alias','title')), '')),
			trim(COALESCE((SELECT group_concat(p.name || ' ' || p.role || ' ' || p.character, ' ') FROM media_people p WHERE p.media_id = m.id AND p.metadata_revision = ?), '')),
			trim(COALESCE((SELECT group_concat(r.display_value || ' ' || r.relationship_type || ' ' || r.role, ' ') FROM media_metadata_relationships r JOIN media_metadata_revisions v ON v.id = r.revision_id WHERE r.media_id = m.id AND v.revision = ? AND r.decision IN ('accepted','locked') AND r.relationship_type NOT IN ('alternate_title','keyword','external_id')), '')),
			trim(COALESCE((SELECT group_concat(i.provider || ' ' || i.external_type || ' ' || i.external_id, ' ') FROM media_provider_ids i WHERE i.media_id = m.id AND i.evidence_revision = ? AND i.status = 'accepted'), '') || ' ' || COALESCE((SELECT group_concat(r.target_provider || ' ' || r.target_external_id || ' ' || r.display_value, ' ') FROM media_metadata_relationships r JOIN media_metadata_revisions v ON v.id = r.revision_id WHERE r.media_id = m.id AND v.revision = ? AND r.decision IN ('accepted','locked') AND r.relationship_type = 'external_id'), '')),
			trim(m.type || ' ' || m.content_rating || ' ' || m.studio || ' ' || m.network || ' ' || m.country || ' ' || CAST(m.year AS TEXT) || ' ' || COALESCE((SELECT group_concat(value, ' ') FROM json_each(CASE WHEN json_valid(m.tags_json) THEN m.tags_json ELSE '[]' END)), '') || ' ' || COALESCE((SELECT group_concat(r.display_value, ' ') FROM media_metadata_relationships r JOIN media_metadata_revisions v ON v.id = r.revision_id WHERE r.media_id = m.id AND v.revision = ? AND r.decision IN ('accepted','locked') AND r.relationship_type = 'keyword'), ''))
		FROM media_items m WHERE m.id = ?`, revision, revision, revision, revision, revision, revision, revision, mediaID)
	if err != nil {
		return fmt.Errorf("replace media search: %w", err)
	}
	return nil
}

func (s *Server) replaceMediaSearch(mediaID string, revision int, source string) error {
	return s.withBackgroundTxTagged(context.Background(), []string{"search"}, func(tx *sql.Tx) error {
		return replaceMediaSearchTx(context.Background(), tx, mediaID, revision, source)
	})
}
