package compactcatalog

import (
	"context"
	"database/sql"
)

const searchLiveSQL = `e.retired=0 AND EXISTS(SELECT 1 FROM catalog_kinds k WHERE k.id=e.kind AND k.searchable=1)`

// Each context lookup is keyed by entity. Provider aliases are already accepted
// evidence; malformed provider JSON contributes no aliases. Context is bounded
// independently of the number of credits or aliases a provider supplies.
const searchContentSQL = `e.title||' '||e.sort_title||' '||CASE WHEN e.year>0 THEN e.year ELSE '' END||' '||
 COALESCE(d.original_title,'')||' '||COALESCE(sh.original_title,'')||' '||
 COALESCE((SELECT group_concat(credited_name,' ') FROM (SELECT credited_name FROM catalog_credits WHERE entity_id=e.id ORDER BY ord LIMIT 256)),'')||' '||
 COALESCE((SELECT group_concat(alias,' ') FROM (SELECT CAST(j.value AS TEXT) alias FROM screen_metadata_work w JOIN screen_metadata_publications p ON p.id=w.accepted_publication,json_each(CASE WHEN json_valid(p.payload) THEN p.payload ELSE '{}' END,'$.aliases') j WHERE w.target_id=e.id AND j.type='text' LIMIT 128)),'')||' '||
 COALESCE(b.author,'')||' '||COALESCE(b.narrator,'')||' '||COALESCE(b.series,'')||' '||
 COALESCE((SELECT group_concat(title,' ') FROM (SELECT a.title FROM catalog_song_artists sa JOIN catalog_entities a ON a.id=sa.artist_id AND a.library_id=e.library_id AND a.retired=0 WHERE sa.song_id=e.id LIMIT 64)),'')||' '||
 COALESCE((SELECT a.title||' '||ar.title FROM catalog_songs so JOIN catalog_entities a ON a.id=so.album_id JOIN catalog_albums al ON al.entity_id=a.id JOIN catalog_entities ar ON ar.id=al.artist_id WHERE so.entity_id=e.id AND a.library_id=e.library_id AND ar.library_id=e.library_id),'')||' '||
 COALESCE((SELECT ar.title FROM catalog_albums al JOIN catalog_entities ar ON ar.id=al.artist_id WHERE al.entity_id=e.id AND ar.library_id=e.library_id),'')||' '||
 COALESCE((SELECT sh.title FROM catalog_episodes ep JOIN catalog_entities sh ON sh.id=ep.show_id WHERE ep.entity_id=e.id AND sh.library_id=e.library_id),'')||' '||
 COALESCE((SELECT group_concat(title,' ') FROM (SELECT ce.title FROM catalog_collection_members cm JOIN catalog_entities ce ON ce.id=cm.collection_id AND ce.library_id=e.library_id AND ce.retired=0 WHERE cm.item_id=e.id LIMIT 64)),'')||' '||
 COALESCE((SELECT group_concat(alias,' ') FROM (SELECT CAST(j.value AS TEXT) alias FROM provider_evidence pe,json_each(CASE WHEN json_valid(pe.payload) THEN pe.payload ELSE '{}' END,'$.aliases') j WHERE pe.item_id=e.id AND j.type='text' LIMIT 128)),'')`

func deriveSearch(ctx context.Context, tx *sql.Tx, ids string) error {
	for _, query := range []string{
		`INSERT INTO catalog_search_titles(catalog_search_titles,rowid,search,kind)
 SELECT 'delete',d.entity_id,d.search,d.kind FROM catalog_search_documents d WHERE d.entity_id IN(SELECT value FROM json_each(?))`,
		`DELETE FROM catalog_search_documents WHERE entity_id IN(SELECT value FROM json_each(?))`,
		`INSERT INTO catalog_search_documents(entity_id,title,search,normalized,kind)
 SELECT e.id,e.title,portico_search_text(` + searchContentSQL + `),portico_search_fold(substr(` + searchContentSQL + `,1,65536)),'k'||e.kind||' l'||e.library_id
 FROM json_each(?) j JOIN catalog_entities e ON e.id=j.value
 LEFT JOIN catalog_item_details d ON d.entity_id=e.id LEFT JOIN catalog_shows sh ON sh.entity_id=e.id LEFT JOIN catalog_books b ON b.entity_id=e.id
 WHERE ` + searchLiveSQL,
		`INSERT INTO catalog_search_titles(rowid,search,kind)
 SELECT d.entity_id,d.search,d.kind FROM catalog_search_documents d WHERE d.entity_id IN(SELECT value FROM json_each(?))`,
	} {
		if _, err := tx.ExecContext(ctx, query, ids); err != nil {
			return err
		}
	}
	return nil
}

func init() {
	Register(DomainSearch, Derivation{
		Version: 3,
		Drain: func(ctx context.Context, tx *sql.Tx, keys []Key, limit int) ([]int64, error) {
			ids := keyIDs(keys)
			return ids, deriveSearch(ctx, tx, mustJSON(ids))
		},
		Backfill: func(ctx context.Context, tx *sql.Tx, after int64, limit int) ([]int64, error) {
			return scanIDs(ctx, tx, `SELECT id FROM catalog_entities WHERE id>? ORDER BY id LIMIT ?`, after, limit)
		},
	})
}
