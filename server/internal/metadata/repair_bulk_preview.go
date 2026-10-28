package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

type BulkPreviewValue struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}
type BulkPreviewField struct {
	State         string             `json:"state"`
	DistinctCount int64              `json:"distinctCount"`
	Value         *string            `json:"value,omitempty"`
	Values        []BulkPreviewValue `json:"values"`
	Entries       []BulkPreviewValue `json:"entries"`
	Locked        int64              `json:"locked"`
	Truncated     bool               `json:"truncated"`
}
type BulkPreviewTarget struct {
	Kind             string `json:"kind"`
	ID               string `json:"id"`
	LibraryID        string `json:"libraryId"`
	ExpectedRevision string `json:"expectedRevision"`
}
type BulkMetadataPreview struct {
	Tabs              []string                    `json:"tabs"`
	ServerID          string                      `json:"serverId"`
	ViewerFence       string                      `json:"viewerFence"`
	ContentKind       string                      `json:"contentKind"`
	Kind              string                      `json:"kind"`
	Total             int64                       `json:"total"`
	SelectionRevision string                      `json:"selectionRevision,omitempty"`
	Schema            []RepairFieldSpec           `json:"schema"`
	SingleItemFields  []RepairFieldSpec           `json:"singleItemFields"`
	ExcludedFields    []RepairFieldSpec           `json:"excludedFields"`
	Fields            map[string]BulkPreviewField `json:"fields"`
	Targets           []BulkPreviewTarget         `json:"targets"`
}

const bulkPreviewValues = 64

type bulkPreviewVector struct {
	Backbone    map[string]string `json:"backbone"`
	Details     map[string]string `json:"details"`
	Side        map[string]string `json:"side"`
	Locks       map[string]int    `json:"locks"`
	GenreLocked int               `json:"genreLocked"`
	Genres      []string          `json:"genres"`
}

func bulkPreviewGenres(values []string) []string {
	unique := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if !unique[value] {
			unique[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

// BulkPreviewSource rebuilds the live selection and its catalogue/metadata
// fence inside each bounded snapshot. It must be repeatable at After, including
// at exhaustion, so the final validation reads current authority and revisions.
type BulkPreviewSource func(context.Context, *sql.Tx, int64) (catalog.BulkMetadataSelectionPage, error)
type bulkPreviewObservation struct {
	field, value string
	entries      []string
	locked       bool
	count        int64
}
type bulkPreviewPage struct {
	preview      BulkMetadataPreview
	observations []bulkPreviewObservation
}

func previewSchema(contentKind string) (BulkMetadataPreview, error) {
	out := BulkMetadataPreview{ContentKind: contentKind, Schema: []RepairFieldSpec{}, SingleItemFields: []RepairFieldSpec{}, ExcludedFields: []RepairFieldSpec{}, Fields: map[string]BulkPreviewField{}, Targets: []BulkPreviewTarget{}}
	kind, err := compactcatalog.ParseKind(contentKind)
	if err != nil {
		return out, err
	}
	out.Kind = compactcatalog.OwnerNamespace(kind)
	out.SingleItemFields = repairSchema(out.Kind, contentKind)
	for _, spec := range out.SingleItemFields {
		if spec.Bulk {
			out.Schema = append(out.Schema, spec)
		} else {
			out.ExcludedFields = append(out.ExcludedFields, spec)
		}
	}
	if out.Kind == "item" && (contentKind == "movie" || contentKind == "song") {
		out.Schema = append(out.Schema, listSpec("genres", "Genres"))
	}
	out.Tabs = repairSchemaTabs(out.Schema)
	if len(out.Schema) == 0 {
		return out, ErrRepairInput
	}
	return out, nil
}

// BulkPreview releases the catalogue snapshot after every candidate page. The
// exact DISTINCT/count sets live in a private disk-backed accumulator, not a
// catalogue TEMP table, a long snapshot, or a slice of the selection's IDs.
// Nothing is returned until a final fresh snapshot validates all three fences.
func (s *Service) BulkPreview(ctx context.Context, source BulkPreviewSource, authorize func(context.Context, *sql.Tx) error) (out BulkMetadataPreview, err error) {
	if source == nil || authorize == nil || dbwork.Snapshot(ctx) != nil {
		return out, ErrRepairInput
	}
	spool, err := newBulkPreviewSpool(ctx, s.cacheRoot)
	if err != nil {
		return out, err
	}
	defer func() {
		if closeErr := spool.close(); closeErr != nil {
			out = BulkMetadataPreview{}
			err = errors.Join(err, closeErr)
		}
	}()
	authority := dbwork.AuthorityGeneration()
	var fence, revision string
	initialized := false
	var after int64
	read := func(snapshot context.Context, final bool) (page bulkPreviewPage, next catalog.BulkMetadataSelectionPage, err error) {
		tx := dbwork.Snapshot(snapshot)
		if err = authorize(snapshot, tx); err != nil {
			return
		}
		if dbwork.AuthorityGeneration() != authority {
			err = catalog.ErrStaleContinuation
			return
		}
		next, err = source(snapshot, tx, after)
		if initialized && (next.Fence != "" && next.Fence != fence || errors.Is(err, sql.ErrNoRows)) {
			err = catalog.ErrStaleContinuation
		}
		if err != nil {
			return
		}
		if !initialized {
			fence, revision = next.Fence, next.Selection.Revision
			initialized = true
		}
		if next.Fence != fence || next.Selection.Revision != revision {
			err = catalog.ErrStaleContinuation
			return
		}
		if !final && strings.TrimSpace(next.Selection.SQL) != "" {
			page, err = s.bulkPreviewPageTx(snapshot, tx, next.Selection)
			if err != nil {
				return
			}
		}
		// Rebuild the fence even inside the page; a source may also bind external
		// authority. Database revisions themselves share the page's WAL snapshot.
		checked, e := source(snapshot, tx, after)
		if e != nil {
			err = e
			return
		}
		if checked.Fence != fence || checked.Selection.Revision != revision {
			err = catalog.ErrStaleContinuation
			return
		}
		if err = authorize(snapshot, tx); err != nil {
			return
		}
		if dbwork.AuthorityGeneration() != authority {
			err = catalog.ErrStaleContinuation
		}
		return
	}
	for {
		var page bulkPreviewPage
		var next catalog.BulkMetadataSelectionPage
		err = dbwork.WithReadSnapshot(ctx, s.db, func(snapshot context.Context) error { var e error; page, next, e = read(snapshot, false); return e })
		if err != nil {
			return BulkMetadataPreview{}, err
		}
		if dbwork.AuthorityGeneration() != authority {
			return BulkMetadataPreview{}, catalog.ErrStaleContinuation
		}
		if page.preview.Total > 0 {
			if out.Total == 0 {
				out = page.preview
				out.Total = 0
				out.Targets = []BulkPreviewTarget{}
			}
			if out.ContentKind != page.preview.ContentKind {
				return BulkMetadataPreview{}, ErrRepairInput
			}
			if err = spool.add(ctx, page.observations); err != nil {
				return BulkMetadataPreview{}, err
			}
			out.Total += page.preview.Total
			out.Targets = append(out.Targets, page.preview.Targets...)
		}
		if next.Done {
			break
		}
		if next.After <= after {
			return BulkMetadataPreview{}, ErrRepairInput
		}
		after = next.After
	}
	if out.Total == 0 {
		return BulkMetadataPreview{}, ErrRepairInput
	}
	out.SelectionRevision = revision
	if out.Fields, err = spool.fields(ctx, out.Schema); err != nil {
		return BulkMetadataPreview{}, err
	}
	// Finalize the disk aggregate BEFORE the last fresh catalogue snapshot: even
	// a slow spill/count cannot hide a late metadata mutation or revocation.
	err = dbwork.WithReadSnapshot(ctx, s.db, func(snapshot context.Context) error { _, _, e := read(snapshot, true); return e })
	if err != nil {
		return BulkMetadataPreview{}, err
	}
	if dbwork.AuthorityGeneration() != authority {
		return BulkMetadataPreview{}, catalog.ErrStaleContinuation
	}
	return out, nil
}

func (s *Service) bulkPreviewPageTx(ctx context.Context, tx *sql.Tx, selection catalog.BulkMetadataSelection) (page bulkPreviewPage, err error) {
	if tx == nil || strings.TrimSpace(selection.SQL) == "" || len(selection.Targets) > catalog.BulkMetadataPageSize {
		return page, ErrRepairInput
	}
	rows, err := tx.QueryContext(ctx, `SELECT k.name,count(*) FROM (`+selection.SQL+`) selected JOIN catalog_entities i ON i.id=selected.id JOIN catalog_kinds k ON k.id=i.kind GROUP BY k.name LIMIT 2`, selection.Args...)
	if err != nil {
		return page, err
	}
	var contentKind string
	var total int64
	kinds := 0
	for rows.Next() {
		kinds++
		if err = rows.Scan(&contentKind, &total); err != nil {
			break
		}
	}
	if err == nil {
		err = rows.Err()
	}
	closeErr := rows.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return page, err
	}
	if kinds == 0 {
		return page, nil
	}
	if kinds != 1 || total > catalog.BulkMetadataPageSize {
		return page, ErrRepairInput
	}
	out, err := previewSchema(contentKind)
	if err != nil {
		return page, err
	}
	out.Total = total
	explicit := len(selection.Targets) > 0
	targets := selection.Targets
	if !explicit && readingRepairKind(out.ContentKind) {
		// Reading attribution is resolved from the current primary publication/path
		// and locked owner overlay. Reuse its editor reader, rather than fabricating
		// empty author/series values from the generic media vector.
		rows, e := tx.QueryContext(ctx, `SELECT pid(i.public_id),l.library_id FROM (`+selection.SQL+`) selected JOIN catalog_entities i ON i.id=selected.id JOIN catalog_libraries l ON l.id=i.library_id`, selection.Args...)
		if e != nil {
			return page, e
		}
		targets = []catalog.BulkMetadataTarget{}
		for rows.Next() {
			target := catalog.BulkMetadataTarget{Kind: out.Kind, ContentKind: out.ContentKind}
			if err = rows.Scan(&target.ID, &target.LibraryID); err != nil {
				break
			}
			targets = append(targets, target)
		}
		if err == nil {
			err = rows.Err()
		}
		closeErr = rows.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return page, err
		}
	}
	if len(targets) > 0 {
		for _, target := range targets {
			repair := RepairTarget{Kind: target.Kind, ID: target.ID}
			snapshot, entity, e := readRepairSnapshot(ctx, tx, repair)
			if e != nil {
				return page, e
			}

			if entity.library != target.LibraryID || entity.itemKind != out.ContentKind {
				return page, ErrRepairInput
			}
			if explicit {
				revision, e := repairRevision(ctx, tx, repair, snapshot, entity)
				if e != nil {
					return page, e
				}
				out.Targets = append(out.Targets, BulkPreviewTarget{target.Kind, target.ID, target.LibraryID, revision})
			}
			for _, spec := range out.Schema {
				observation := bulkPreviewObservation{field: spec.Field, count: 1}
				if spec.Field == "genres" {
					genres := []string{}
					for _, relationship := range snapshot.Relationships {
						if relationship.Kind == "genre" {
							genres = append(genres, relationship.Label)
						}
					}
					observation.entries = bulkPreviewGenres(genres)
					observation.value = encodeList(observation.entries)
					observation.locked = snapshot.RelationshipLocks["genre"]
				} else {
					field, ok := snapshot.Fields[spec.Field]
					if !ok {
						return page, ErrRepairInput
					}
					observation.value, observation.entries, observation.locked = field.Value, field.Values, field.Locked
				}
				page.observations = append(page.observations, observation)
			}
		}
	} else {
		vector := `json_object('backbone',json_object('metadata_language',i.metadata_language,'year',CAST(i.year AS TEXT)),
   'details',json_object('edition',COALESCE(d.edition,''),'content_rating',COALESCE(d.content_rating,''),'studio',COALESCE(d.studio,''),'network',COALESCE(d.network,''),'country',COALESCE(d.country,''),'tags',COALESCE(d.tags,''),'labels',COALESCE(d.labels,'')),
   'side',json(CASE k.name WHEN 'show' THEN json_object('content_rating',COALESCE(sh.content_rating,''),'studio',COALESCE(sh.studio,''),'network',COALESCE(sh.network,''),'country',COALESCE(sh.country,''),'tags',COALESCE(sh.tags,''),'labels',COALESCE(sh.labels,''))
   WHEN 'artist' THEN json_object('tags',COALESCE(ar.tags,'')) WHEN 'album' THEN json_object('label',COALESCE(al.label,''),'tags',COALESCE(al.tags,'')) WHEN 'book' THEN json_object('author',COALESCE(bk.author,''),'narrator',COALESCE(bk.narrator,''),'series',COALESCE(bk.series,''),'tags',COALESCE(bk.tags,''),'labels',COALESCE(bk.labels,'')) ELSE json_object() END),
   'locks',json(COALESCE((SELECT json_group_object(field,locked) FROM metadata_owner_fields owner WHERE owner.entity_id=i.id AND owner.kind=?),'{}')),
   'genreLocked',COALESCE((SELECT locked FROM metadata_relationship_decisions lock WHERE lock.entity_id=i.id AND lock.kind=? AND lock.relationship='genre'),0),
   'genres',json(COALESCE((SELECT json_group_array(label) FROM (SELECT DISTINCT COALESCE(ts.label_override,t.label) AS label FROM catalog_entity_terms et JOIN catalog_terms t ON t.id=et.term_id JOIN catalog_term_sources ts ON ts.entity_id=et.entity_id AND ts.term_id=et.term_id WHERE et.entity_id=i.id AND t.vocab=1 ORDER BY label)),'[]')))`
		args := append([]any{}, selection.Args...)
		args = append(args, out.Kind, out.Kind)
		vectors := `WITH selected AS MATERIALIZED (` + selection.SQL + `) SELECT ` + vector + ` AS vector FROM selected JOIN catalog_entities i ON i.id=selected.id JOIN catalog_kinds k ON k.id=i.kind LEFT JOIN catalog_item_details d ON d.entity_id=i.id LEFT JOIN catalog_shows sh ON sh.entity_id=i.id LEFT JOIN catalog_artists ar ON ar.entity_id=i.id LEFT JOIN catalog_albums al ON al.entity_id=i.id LEFT JOIN catalog_books bk ON bk.entity_id=i.id`
		grouped, e := tx.QueryContext(ctx, `SELECT vector,count(*) FROM (`+vectors+`) GROUP BY vector`, args...)
		if e != nil {
			return page, e
		}
		for grouped.Next() {
			var raw string
			var count int64
			if err = grouped.Scan(&raw, &count); err != nil {
				break
			}
			var value bulkPreviewVector
			if err = json.Unmarshal([]byte(raw), &value); err != nil {
				break
			}
			for _, spec := range out.Schema {
				observation := bulkPreviewObservation{field: spec.Field, count: count}
				if spec.Field == "genres" {
					observation.entries = bulkPreviewGenres(value.Genres)
					observation.value = encodeList(observation.entries)
					observation.locked = value.GenreLocked != 0
				} else {
					observation.value = repairFieldValue(out.Kind, out.ContentKind, spec.Field, value.Backbone, value.Details, value.Side, sql.NullInt64{})
					if spec.Type == "list" {
						observation.entries = decodeList(observation.value)
						observation.value = encodeList(observation.entries)
					}
					observation.locked = value.Locks[spec.Field] != 0
				}
				page.observations = append(page.observations, observation)
			}
		}
		if err == nil {
			err = grouped.Err()
		}
		closeErr = grouped.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return page, err
		}
	}
	page.preview = out
	return page, nil
}
