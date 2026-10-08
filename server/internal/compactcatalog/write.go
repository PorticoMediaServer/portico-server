package compactcatalog

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
)

// The catalogue write API. Every writer of catalogue facts calls these inside
// its own dbwork transaction: they resolve identity through the ledger, apply
// owner locks, and queue the derived work the change causes. Nothing else
// writes the fact tables (catalog_entities, the kind side tables, assets and
// links, terms, credits, attributes, collections). Side effects owned by other
// subsystems (library revisions, provider job invalidation, artwork dirt) are
// triggers on the fact tables, so they happen whoever writes.

// Source says who is writing a field: an automatic writer (scanner, provider
// metadata) never overwrites a field the owner locked; the owner always writes.
type Source int

const (
	Automatic Source = iota
	Owner
)

// ErrIdentityConflict: an identity key already names an entity of another
// library or kind (two libraries sharing a folder, or a key reused by a kind).
var ErrIdentityConflict = errors.New("catalogue identity belongs to another library or kind")

// LibraryTx returns the catalogue handle of a library. A trigger on libraries
// keeps the handle, so this is one indexed read; the write is only a fallback.
// Library kinds are their own enum (movie 1, tv 2, anime 3, music 4, audiobook 5).
func LibraryTx(ctx context.Context, tx *sql.Tx, library string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM catalog_libraries WHERE library_id=?`, library).Scan(&id)
	if !errors.Is(err, sql.ErrNoRows) {
		return id, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO catalog_libraries(library_id,root,name,kind)
 SELECT l.id,l.root,l.name,CASE l.kind WHEN 'movie' THEN 1 WHEN 'tv' THEN 2 WHEN 'anime' THEN 3 WHEN 'music' THEN 4 WHEN 'audiobook' THEN 5 ELSE 99 END FROM libraries l WHERE l.id=?
 RETURNING id`, library).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, fmt.Errorf("library %q does not exist", library)
	}
	return id, err
}

// LibraryHandle returns an existing library's handle (0 when it has none yet).
func LibraryHandle(ctx context.Context, q ReadQuery, library string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM catalog_libraries WHERE library_id=?`, library).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// Entity is a catalogue entity's backbone facts.
type Entity struct {
	Library   int64  // catalog_libraries.id
	Kind      Kind   //
	Parent    int64  // 0: none (a season's show, an episode's season, an album's artist, a file's book)
	Key       string // identity source key (see ItemKey and friends)
	Title     string //
	SortTitle string //
	Language  string // metadata language, for sorting
	Year      int    //
	Added     string // RFC 3339 time the entity was added, or "" (containers)
}

// Identity keys: stable names of an entity inside its library's root folder,
// so a rescan or a re-added library finds the same public id. A new kind
// brings its own prefix.
func ItemKey(root, path string, part int) string {
	rel := path
	if r, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(r, "..") {
		rel = filepath.ToSlash(r)
	}
	if part > 0 {
		return fmt.Sprintf("file:%s#%d", rel, part)
	}
	return "file:" + rel
}
func ShowKey(localKey string) string { return "show:" + localKey }
func SeasonKey(showLocalKey string, number int) string {
	return fmt.Sprintf("season:%s/%d", showLocalKey, number)
}
func EpisodeKey(showLocalKey, numbering string, season, number int) string {
	return fmt.Sprintf("episode:%s/%s/%d/%d", showLocalKey, numbering, season, number)
}
func ArtistKey(localKey string) string    { return "artist:" + localKey }
func AlbumKey(localKey string) string     { return "album:" + localKey }
func BookKey(localKey string) string      { return "book:" + localKey }
func CollectionKey(nameKey string) string { return "collection:" + nameKey }
func AuthorKey(name string) string        { return dbwork.AuthorIdentityKey(name) }
func ExtraKey(root, path string) string {
	return "extra:" + strings.TrimPrefix(ItemKey(root, path, 0), "file:")
}
func RecordingKey(recording string) string { return "recording:" + recording }

// libraryRoot is the folder identity keys are relative to: the library's
// identity root, fixed when it was created.
func libraryRoot(ctx context.Context, tx *sql.Tx, library int64) (string, error) {
	var root string
	err := tx.QueryRowContext(ctx, `SELECT root FROM catalog_libraries WHERE id=?`, library).Scan(&root)
	return root, err
}

// LibraryRootTx returns a library handle's root folder (for building keys).
func LibraryRootTx(ctx context.Context, tx *sql.Tx, library int64) (string, error) {
	return libraryRoot(ctx, tx, library)
}

// FindEntityTx returns the live entity an identity key names in a library, or 0.
func FindEntityTx(ctx context.Context, tx *sql.Tx, library int64, key string) (int64, error) {
	root, err := libraryRoot(ctx, tx, library)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT e.id FROM catalog_identities i JOIN catalog_entities e ON e.public_id=i.public_id WHERE i.root=? AND i.source_key=?`, root, key).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// EnsureEntityTx finds or creates the entity its identity key names. An
// existing entity keeps its facts (a scanner's filename title never replaces a
// provider's); it is only made live again and re-parented if that changed.
func EnsureEntityTx(ctx context.Context, tx *sql.Tx, e Entity) (id int64, created bool, err error) {
	return upsertEntity(ctx, tx, e, false)
}

// UpsertEntityTx finds or creates the entity its identity key names and brings
// its backbone facts up to date (unlocked fields only, for an existing entity).
// A retired entity that reappears is live again.
func UpsertEntityTx(ctx context.Context, tx *sql.Tx, e Entity) (id int64, created bool, err error) {
	return upsertEntity(ctx, tx, e, true)
}

func upsertEntity(ctx context.Context, tx *sql.Tx, e Entity, update bool) (id int64, created bool, err error) {
	if e.Library == 0 || e.Kind < 1 || e.Key == "" {
		return 0, false, fmt.Errorf("catalogue entity needs a library, a kind and an identity key")
	}
	root, err := libraryRoot(ctx, tx, e.Library)
	if err != nil {
		return 0, false, err
	}
	var public []byte
	var existing sql.NullInt64
	var library, kind sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT i.public_id,e.id,e.library_id,e.kind FROM catalog_identities i LEFT JOIN catalog_entities e ON e.public_id=i.public_id WHERE i.root=? AND i.source_key=?`, root, e.Key).Scan(&public, &existing, &library, &kind)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		public = entityid.New()
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_identities(public_id,root,source_key,kind) VALUES(?,?,?,?)`, public, root, e.Key, int(e.Kind)); err != nil {
			return 0, false, err
		}
	case err != nil:
		return 0, false, err
	}
	var parent any
	if e.Parent != 0 {
		parent = e.Parent
	}
	if existing.Valid {
		if library.Int64 != e.Library || kind.Int64 != int64(e.Kind) {
			return 0, false, fmt.Errorf("%w: %s", ErrIdentityConflict, e.Key)
		}
		id = existing.Int64
		res, err := tx.ExecContext(ctx, `UPDATE catalog_entities SET parent_id=?,retired=0 WHERE id=? AND (parent_id IS NOT ? OR retired<>0)`, parent, id, parent)
		if err != nil {
			return 0, false, err
		}
		if !update {
			if n, _ := res.RowsAffected(); n > 0 {
				return id, false, touchEntity(ctx, tx, id, e.Kind)
			}
			return id, false, nil
		}
		fields := map[string]any{"title": e.Title, "year": e.Year}
		if e.SortTitle != "" {
			fields["sort_title"] = e.SortTitle
		}
		if e.Language != "" {
			fields["metadata_language"] = e.Language
		}
		if err = setFields(ctx, tx, id, Automatic, fields, true); err != nil {
			return 0, false, err
		}
		return id, false, nil
	}
	added := addedMillis(e.Added)
	err = tx.QueryRowContext(ctx, `INSERT INTO catalog_entities(public_id,library_id,kind,parent_id,title,sort_title,metadata_language,sort_key,sort_head,year,added_ms)
 VALUES(?1,?2,?3,?4,?5,?6,?7,portico_sort_title(?5,?6,?7),substr(portico_sort_title(?5,?6,?7),1,1),?8,?9) RETURNING id`,
		public, e.Library, int(e.Kind), parent, e.Title, e.SortTitle, e.Language, e.Year, added).Scan(&id)
	if err != nil {
		return 0, false, err
	}
	// Every playable item has a details row, as every old items row had these
	// columns: readers join catalog_item_details directly.
	if e.Added != "" {
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_item_details(entity_id,added_text) VALUES(?,?) ON CONFLICT(entity_id) DO UPDATE SET added_text=excluded.added_text`, id, e.Added); err != nil {
			return 0, false, err
		}
	} else if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO catalog_item_details(entity_id) SELECT ? FROM catalog_kinds WHERE id=? AND playable=1`, id, int(e.Kind)); err != nil {
		return 0, false, err
	}
	return id, true, touchEntity(ctx, tx, id, e.Kind)
}

// touchEntity queues the derived work any change to an entity's facts causes.
func touchEntity(ctx context.Context, tx *sql.Tx, id int64, kind Kind) error {
	for _, domain := range []int{DomainSearch, DomainBrowseRows, DomainBrowseEdges} {
		if err := TouchTx(ctx, tx, domain, id); err != nil {
			return err
		}
	}
	if kind == Movie {
		for _, domain := range []int{DomainCategories, DomainRelated} {
			if err := TouchTx(ctx, tx, domain, id); err != nil {
				return err
			}
		}
	}
	// Rows derived from a parent's facts follow it (each bounded by the
	// container): a season's row carries its show's title and year, a song's
	// edges its album's artist, a book file's edges its book's author — and the
	// author a file belongs to now re-derives, so a retired or re-authored book
	// releases it.
	switch kind {
	case Show:
		return touchDependents(ctx, tx, id, map[int]string{DomainBrowseRows: `SELECT ?1,entity_id,1 FROM catalog_seasons WHERE show_id=?2`})
	case Album:
		return touchDependents(ctx, tx, id, map[int]string{DomainBrowseEdges: `SELECT ?1,entity_id,1 FROM catalog_songs INDEXED BY catalog_songs_album_order WHERE album_id=?2`})
	case Book:
		return touchDependents(ctx, tx, id, map[int]string{
			DomainBrowseEdges: `SELECT ?1,entity_id,1 FROM catalog_book_files INDEXED BY catalog_book_files_order WHERE book_id=?2`,
			DomainBrowseRows: `SELECT ?1,m.entity_id,1 FROM catalog_book_files f INDEXED BY catalog_book_files_order
			 CROSS JOIN catalog_browse_memberships m INDEXED BY catalog_browse_membership_item ON m.item_id=f.entity_id WHERE f.book_id=?2 AND m.source=7`,
		})
	}
	return nil
}

// touchDependents queues, per domain, the ids a query selects as (domain, id, 1).
func touchDependents(ctx context.Context, tx *sql.Tx, id int64, queries map[int]string) error {
	for domain, query := range queries {
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_dirty(domain,entity_id,revision) `+query+` ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1`, domain, id); err != nil {
			return err
		}
	}
	return nil
}

// TouchEntityTx queues an entity's derived data after a writer changed facts
// the API doesn't know about (a trigger-less side table).
func TouchEntityTx(ctx context.Context, tx *sql.Tx, id int64) error {
	var kind Kind
	if err := tx.QueryRowContext(ctx, `SELECT kind FROM catalog_entities WHERE id=?`, id).Scan(&kind); err != nil {
		return err
	}
	return touchEntity(ctx, tx, id, kind)
}

// Where each field lives. A field is written in the kind's side table when
// that table has the column, else in the backbone, else in the item details.
var backboneFields = map[string]bool{"title": true, "sort_title": true, "metadata_language": true, "year": true}

var detailFields = setOf("added_text", "overview", "poster_url", "backdrop_url", "original_title", "edition", "tagline", "release_date", "content_rating", "studio", "network", "country", "tags", "labels")

var sideTables = map[Kind]struct {
	table  string
	fields map[string]bool
}{
	Show:       {"catalog_shows", setOf("local_key", "provider_match_status", "original_title", "tagline", "overview", "content_rating", "studio", "network", "country", "tags", "labels")},
	Season:     {"catalog_seasons", setOf("show_id", "number", "title", "overview")},
	Episode:    {"catalog_episodes", setOf("show_id", "season_id", "numbering", "number", "local_identity_status", "ordering_basis", "air_date")},
	Artist:     {"catalog_artists", setOf("local_key", "overview", "tags")},
	Album:      {"catalog_albums", setOf("artist_id", "local_key", "edition_key", "provider_match_status", "overview", "label", "tags", "release_id", "release_group_id", "release_date", "release_country", "provider_observed_at")},
	Track:      {"catalog_songs", setOf("album_id", "disc_number", "track_number", "release_status", "recording_id", "release_id", "release_group_id", "track_id", "provider_observed_at", "provider_title", "provider_artist")},
	Book:       {"catalog_books", setOf("library_id", "local_key", "author", "narrator", "provider_match_status", "overview", "series", "series_position", "tags", "labels", "local_metadata_payload")},
	Part:       {"catalog_book_files", setOf("book_id", "disc_number", "part_number")},
	Collection: {"catalog_collections", setOf("library_id", "name_key", "created_at")},
}

func setOf(names ...string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		out[n] = true
	}
	return out
}

// OwnerNamespace is the metadata_owner_fields kind of an entity kind: the
// editor's field set ('item' for every playable item).
func OwnerNamespace(k Kind) string {
	switch k {
	case Show:
		return "show"
	case Season:
		return "season"
	case Artist:
		return "artist"
	case Album:
		return "album"
	case Book:
		return "book"
	case Collection:
		return "collection"
	case Author:
		return "author"
	}
	return "item"
}

// SetFieldsTx writes named fields of one entity wherever they live. An
// Automatic write to a field the owner locked records the incoming value as
// the field's automatic_value and keeps the owner's value; an Owner write (or
// an unlocked field) is written. Queues the entity's derived data when
// anything changed.
func SetFieldsTx(ctx context.Context, tx *sql.Tx, id int64, source Source, fields map[string]any) error {
	return setFields(ctx, tx, id, source, fields, false)
}

// setFields routes fields to their tables; backboneFirst sends the backbone
// names (title, year, …) to catalog_entities even where the kind's side table
// has a column of that name (a season's title override).
func setFields(ctx context.Context, tx *sql.Tx, id int64, source Source, fields map[string]any, backboneFirst bool) error {
	if len(fields) == 0 {
		return nil
	}
	var kind Kind
	if err := tx.QueryRowContext(ctx, `SELECT kind FROM catalog_entities WHERE id=?`, id).Scan(&kind); err != nil {
		return err
	}
	if source == Automatic {
		locked, err := lockedFields(ctx, tx, OwnerNamespace(kind), id, kind)
		if err != nil {
			return err
		}
		for name, value := range fields {
			owner, ok := locked[name]
			if !ok {
				continue
			}
			if _, err = tx.ExecContext(ctx, `UPDATE metadata_owner_fields SET automatic_value=? WHERE kind=? AND entity_id=? AND field=?`, fmt.Sprint(value), OwnerNamespace(kind), id, owner); err != nil {
				return err
			}
			delete(fields, name)
		}
		if len(fields) == 0 {
			return nil
		}
	}
	groups := map[string]map[string]any{}
	side, hasSide := sideTables[kind]
	for name, value := range fields {
		table := ""
		switch {
		case backboneFirst && backboneFields[name]:
			table = "catalog_entities"
		case hasSide && side.fields[name]:
			table = side.table
		case backboneFields[name]:
			table = "catalog_entities"
		case detailFields[name]:
			table = "catalog_item_details"
		default:
			return fmt.Errorf("catalogue field %q is not a field of kind %d", name, kind)
		}
		if groups[table] == nil {
			groups[table] = map[string]any{}
		}
		groups[table][name] = value
	}
	// When an item was first catalogued is stored twice: as text beside its
	// details (browse and "recently added" order by it) and as milliseconds on
	// the entity (the watched watermark compares it). A write of one is a
	// write of both, so they never disagree.
	if added, ok := groups["catalog_item_details"]["added_text"]; ok {
		if groups["catalog_entities"] == nil {
			groups["catalog_entities"] = map[string]any{}
		}
		text, _ := added.(string)
		groups["catalog_entities"]["added_ms"] = addedMillis(text)
	}
	changed := false
	for table, values := range groups {
		n, err := writeFacts(ctx, tx, table, id, values)
		if err != nil {
			return err
		}
		changed = changed || n > 0
	}
	if changed {
		return touchEntity(ctx, tx, id, kind)
	}
	return nil
}

// lockedFields maps each catalogue field the owner locked on the entity to the
// name the lock is recorded under. The metadata editor records locks under
// its own field names (sortTitle, contentRating, description); a lock recorded
// under the catalogue field itself (title, overview) maps to itself.
func lockedFields(ctx context.Context, tx *sql.Tx, namespace string, id int64, kind Kind) (map[string]string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT field FROM metadata_owner_fields WHERE kind=? AND entity_id=? AND locked=1`, namespace, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var f string
		if err = rows.Scan(&f); err != nil {
			return nil, err
		}
		out[f] = f
		if column, ok := OwnerFieldColumn(kind, f); ok {
			out[column] = f
		}
	}
	return out, rows.Err()
}

// OwnerFieldColumn maps a metadata-editor field to the catalogue field the
// write API routes to its table (backbone, kind side table or details), for an
// entity of kind. The editor records owner values and locks under its own
// names; this is the one translation both sides use.
func OwnerFieldColumn(kind Kind, field string) (string, bool) {
	switch field {
	case "title":
		return "title", true
	case "description":
		return "overview", true
	case "sortTitle":
		return "sort_title", true
	case "metadataLanguage":
		return "metadata_language", true
	case "originalTitle":
		return "original_title", true
	case "releaseDate":
		if kind == Episode {
			return "air_date", true
		}
		return "release_date", true
	case "contentRating":
		return "content_rating", true
	case "seriesIndex":
		return "series_position", true
	case "episodeNumber", "number":
		return "number", true
	case "trackNumber":
		return "track_number", true
	case "discNumber":
		return "disc_number", true
	case "partNumber":
		return "part_number", true
	case "tags", "labels", "label", "author", "narrator", "series", "studio", "network", "country", "edition", "tagline", "year":
		return field, true
	}
	return "", false
}

// writeFacts upserts columns of one entity's row in table and reports whether
// a value changed. The backbone recomputes its sort key when a sort input
// changed.
// addedMillis is a first-catalogued time as the entity stores it: RFC 3339 text
// to milliseconds, or NULL when there is no (valid) time.
func addedMillis(text string) any {
	if text == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		return nil
	}
	return t.UnixMilli()
}

func writeFacts(ctx context.Context, tx *sql.Tx, table string, id int64, values map[string]any) (int64, error) {
	names := make([]string, 0, len(values))
	for n := range values {
		names = append(names, n)
	}
	sort.Strings(names)
	args := make([]any, 0, len(names)*2+1)
	var set, differ []string
	for _, n := range names {
		set = append(set, n+"=?")
		args = append(args, values[n])
	}
	for _, n := range names {
		differ = append(differ, n+" IS NOT ?")
		args = append(args, values[n])
	}
	if table == "catalog_entities" {
		args = append(args, id)
		res, err := tx.ExecContext(ctx, `UPDATE catalog_entities SET `+strings.Join(set, ",")+` WHERE (`+strings.Join(differ, " OR ")+`) AND id=?`, args...)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		if n > 0 {
			_, err = tx.ExecContext(ctx, `UPDATE catalog_entities SET sort_key=portico_sort_title(title,sort_title,metadata_language),sort_head=substr(portico_sort_title(title,sort_title,metadata_language),1,1) WHERE id=?`, id)
		}
		return n, err
	}
	// An existing side row is updated in place. It cannot be an upsert: SQLite
	// checks NOT NULL on the proposed insert row before ON CONFLICT, so a partial
	// write to a row whose table has a required column (an album's artist) would
	// fail even though the row exists.
	args = append(args, id)
	res, err := tx.ExecContext(ctx, `UPDATE `+table+` SET `+strings.Join(set, ",")+` WHERE (`+strings.Join(differ, " OR ")+`) AND entity_id=?`, args...)
	if err != nil {
		return 0, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return n, nil
	}
	var exists bool
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE entity_id=?)`, id).Scan(&exists); err != nil || exists {
		return 0, err
	}
	// Side rows are created on first write with defaults for the rest.
	insertArgs := append([]any{id}, valuesInOrder(values, names)...)
	res, err = tx.ExecContext(ctx, `INSERT INTO `+table+`(entity_id,`+strings.Join(names, ",")+`) VALUES(?`+strings.Repeat(",?", len(names))+`)`, insertArgs...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func valuesInOrder(values map[string]any, names []string) []any {
	out := make([]any, len(names))
	for i, n := range names {
		out[i] = values[n]
	}
	return out
}

// SetFactsTx writes an entity's side-table row (creating it) for an automatic
// writer: locked fields are skipped as in SetFieldsTx. Use it right after
// UpsertEntityTx for the kind's own facts.
func SetFactsTx(ctx context.Context, tx *sql.Tx, id int64, fields map[string]any) error {
	return SetFieldsTx(ctx, tx, id, Automatic, fields)
}

// RetireEntityTx retires an entity (a container that lost its last member):
// it keeps its identity, personal state and history, and leaves browse and
// search.
func RetireEntityTx(ctx context.Context, tx *sql.Tx, id int64) error {
	res, err := tx.ExecContext(ctx, `UPDATE catalog_entities SET retired=1 WHERE id=? AND retired=0`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	return TouchEntityTx(ctx, tx, id)
}

// DeleteEntityTx deletes an entity and everything that cascades from it. Its
// identity stays in the ledger, so the same file or key gets the same public
// id if it comes back. Derived rows that don't cascade (counted rows, bucket
// contributions, the title index) are dropped by the derivations, which see
// the entity gone.
func DeleteEntityTx(ctx context.Context, tx *sql.Tx, id int64) error {
	// The containers it belonged to re-derive their rows.
	if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_dirty(domain,entity_id,revision)
 SELECT ?,entity_id,1 FROM catalog_browse_memberships INDEXED BY catalog_browse_membership_item WHERE item_id=? AND entity_id<>item_id
 ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1`, DomainBrowseRows, id); err != nil {
		return err
	}
	// The external-content title index needs the old title to remove a row.
	if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_search_titles(catalog_search_titles,rowid,title,kind) SELECT 'delete',entity_id,title,kind FROM catalog_search_documents WHERE entity_id=?`, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_search_documents WHERE entity_id=?`, id); err != nil {
		return err
	}
	for _, domain := range []int{DomainBrowseRows, DomainItemMetrics, DomainCategories} {
		if err := TouchTx(ctx, tx, domain, id); err != nil {
			return err
		}
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM catalog_entities WHERE id=?`, id)
	return err
}

// Asset is one physical file's evidence.
type Asset struct {
	Path                              string
	Size, ModifiedNS                  int64
	Container, VideoCodec, AudioCodec string
	Width, Height                     int
	Duration                          float64
}

// NewAssetToken is a fresh public asset id (the asset id clients see).
func NewAssetToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// newPersonToken is a fresh public person id (32 hex digits, as before).
func newPersonToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// UpsertAssetTx records a file's evidence (by path) and marks it available.
// It returns the asset's integer id and its public token.
func UpsertAssetTx(ctx context.Context, tx *sql.Tx, a Asset) (int64, string, error) {
	var id int64
	var token string
	// An upsert makes the file available; one that was unavailable changes its
	// items' availability, which re-derives from the links.
	var wasUnavailable bool
	if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_assets WHERE path=? AND available=0)`, a.Path).Scan(&wasUnavailable); err != nil {
		return 0, "", err
	}
	err := tx.QueryRowContext(ctx, `INSERT INTO catalog_assets(token,path,size,modified_ns,container,video_codec,audio_codec,width,height,duration,available)
 VALUES(?,?,?,?,?,?,?,?,?,?,1)
 ON CONFLICT(path) DO UPDATE SET size=excluded.size,modified_ns=excluded.modified_ns,container=excluded.container,video_codec=excluded.video_codec,audio_codec=excluded.audio_codec,width=excluded.width,height=excluded.height,duration=excluded.duration,available=1
 RETURNING id,token`, NewAssetToken(), a.Path, a.Size, a.ModifiedNS, a.Container, a.VideoCodec, a.AudioCodec, a.Width, a.Height, a.Duration).Scan(&id, &token)
	if err != nil {
		return 0, "", err
	}
	if wasUnavailable {
		return id, token, touchAssetItems(ctx, tx, id)
	}
	return id, token, TouchTx(ctx, tx, DomainAssetMetrics, id)
}

// CreateAssetTx records a new file under a public token the caller chose
// (inventory keeps an asset's token across moves). It returns the integer id.
func CreateAssetTx(ctx context.Context, tx *sql.Tx, token string, a Asset, available bool) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `INSERT INTO catalog_assets(token,path,size,modified_ns,container,video_codec,audio_codec,width,height,duration,available)
 VALUES(?,?,?,?,?,?,?,?,?,?,?) RETURNING id`, token, a.Path, a.Size, a.ModifiedNS, a.Container, a.VideoCodec, a.AudioCodec, a.Width, a.Height, a.Duration, available).Scan(&id)
	if err != nil {
		return 0, err
	}
	return id, TouchTx(ctx, tx, DomainAssetMetrics, id)
}

var assetColumns = setOf("path", "size", "modified_ns", "container", "video_codec", "audio_codec", "width", "height", "duration", "available")

// SetAssetTx updates a file's evidence columns (path, size, modified_ns,
// container, codecs, dimensions, duration, available) when they differ, and
// queues the items it backs.
func SetAssetTx(ctx context.Context, tx *sql.Tx, asset int64, values map[string]any) error {
	names := make([]string, 0, len(values))
	for n := range values {
		if !assetColumns[n] {
			return fmt.Errorf("catalogue asset has no column %q", n)
		}
		names = append(names, n)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	var set, differ []string
	args := make([]any, 0, 2*len(names)+1)
	for _, n := range names {
		set = append(set, n+"=?")
		args = append(args, values[n])
	}
	for _, n := range names {
		differ = append(differ, n+" IS NOT ?")
		args = append(args, values[n])
	}
	args = append(args, asset)
	res, err := tx.ExecContext(ctx, `UPDATE catalog_assets SET `+strings.Join(set, ",")+` WHERE (`+strings.Join(differ, " OR ")+`) AND id=?`, args...)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	return touchAssetItems(ctx, tx, asset)
}

// AssetByPathTx returns the asset recorded at path (0 when none).
func AssetByPathTx(ctx context.Context, tx *sql.Tx, path string) (int64, string, error) {
	var id int64
	var token string
	err := tx.QueryRowContext(ctx, `SELECT id,token FROM catalog_assets WHERE path=?`, path).Scan(&id, &token)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	return id, token, err
}

// AssetByTokenTx returns the integer id of a public asset id (0 when none).
func AssetByTokenTx(ctx context.Context, q ReadQuery, token string) (int64, error) {
	var id int64
	err := q.QueryRowContext(ctx, `SELECT id FROM catalog_assets WHERE token=?`, token).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

// SetAssetAvailableTx records whether a file is present (inventory, trash,
// remote sources) and queues the availability of the items it backs.
func SetAssetAvailableTx(ctx context.Context, tx *sql.Tx, asset int64, available bool) error {
	res, err := tx.ExecContext(ctx, `UPDATE catalog_assets SET available=? WHERE id=? AND available<>?`, available, asset, available)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	return touchAssetItems(ctx, tx, asset)
}

func touchAssetItems(ctx context.Context, tx *sql.Tx, asset int64) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_dirty(domain,entity_id,revision)
 SELECT ?,entity_id,1 FROM catalog_asset_links INDEXED BY catalog_asset_links_asset WHERE asset_id=?
 ON CONFLICT(domain,entity_id) DO UPDATE SET revision=revision+1`, DomainAvailability, asset); err != nil {
		return err
	}
	return TouchTx(ctx, tx, DomainAssetMetrics, asset)
}

// DeleteAssetTx forgets a file: its links go with it, and the items it backed
// re-derive their availability (an item left with no file is the caller's to
// delete).
func DeleteAssetTx(ctx context.Context, tx *sql.Tx, asset int64) error {
	if err := touchAssetItems(ctx, tx, asset); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `DELETE FROM catalog_assets WHERE id=?`, asset)
	return err
}

// Link is one of an item's physical parts.
type Link struct {
	Part       int
	Start      float64
	End        *float64
	HasSegment bool
}

// LinkAssetTx attaches a file to an item (idempotent).
func LinkAssetTx(ctx context.Context, tx *sql.Tx, item, asset int64, l Link) error {
	var end any
	if l.End != nil {
		end = *l.End
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO catalog_asset_links(entity_id,asset_id,part_index,available,start_seconds,end_seconds)
 SELECT ?1,?2,?3,a.available,?4,?5 FROM catalog_assets a WHERE a.id=?2
 ON CONFLICT(entity_id,asset_id) DO UPDATE SET part_index=excluded.part_index,start_seconds=excluded.start_seconds,end_seconds=excluded.end_seconds
 WHERE catalog_asset_links.part_index IS NOT excluded.part_index OR catalog_asset_links.start_seconds IS NOT excluded.start_seconds OR catalog_asset_links.end_seconds IS NOT excluded.end_seconds`, item, asset, l.Part, l.Start, end)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	return touchLinked(ctx, tx, item)
}

// UnlinkAssetTx detaches a file from an item.
func UnlinkAssetTx(ctx context.Context, tx *sql.Tx, item, asset int64) error {
	res, err := tx.ExecContext(ctx, `DELETE FROM catalog_asset_links WHERE entity_id=? AND asset_id=?`, item, asset)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	return touchLinked(ctx, tx, item)
}

func touchLinked(ctx context.Context, tx *sql.Tx, item int64) error {
	for _, domain := range []int{DomainAvailability, DomainItemMetrics, DomainBrowseRows} {
		if err := TouchTx(ctx, tx, domain, item); err != nil {
			return err
		}
	}
	return nil
}

// Term is one provider's term of a vocabulary for an entity.
type Term struct {
	SourceID string // the provider's id for it ('' → the key)
	Name     string // display label
	Key      string // normalized key ('' → lower(Name))
}

// SetTermsTx replaces an entity's terms of one vocabulary from one provider, in
// the provider's order (the first term is the primary one).
func SetTermsTx(ctx context.Context, tx *sql.Tx, entity int64, vocab Vocab, provider string, terms []Term) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_term_sources WHERE entity_id=? AND provider=? AND (SELECT t.vocab FROM catalog_terms t WHERE t.id=catalog_term_sources.term_id)=?`, entity, provider, int(vocab)); err != nil {
		return err
	}
	for ordinal, t := range terms {
		key := t.Key
		if key == "" {
			key = strings.ToLower(strings.TrimSpace(t.Name))
		}
		if key == "" {
			continue
		}
		source := t.SourceID
		if source == "" {
			source = key
		}
		var term int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO catalog_terms(vocab,key,label) VALUES(?,?,?) ON CONFLICT(vocab,key) DO UPDATE SET label=label RETURNING id`, int(vocab), key, t.Name).Scan(&term); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO catalog_entity_terms(entity_id,term_id) VALUES(?,?)`, entity, term); err != nil {
			return err
		}
		var override any
		if t.Name != "" {
			override = t.Name
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_term_sources(entity_id,term_id,provider,source_id,label_override,source_name,ordinal) VALUES(?,?,?,?,?,?,?)
 ON CONFLICT(entity_id,provider,source_id) DO UPDATE SET term_id=excluded.term_id,label_override=excluded.label_override,source_name=excluded.source_name,ordinal=excluded.ordinal`, entity, term, provider, source, override, t.Name, ordinal); err != nil {
			return err
		}
	}
	// A term no provider names any more leaves the entity.
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_entity_terms WHERE entity_id=?1 AND (SELECT t.vocab FROM catalog_terms t WHERE t.id=catalog_entity_terms.term_id)=?2
 AND NOT EXISTS(SELECT 1 FROM catalog_term_sources s INDEXED BY catalog_term_source_membership WHERE s.entity_id=?1 AND s.term_id=catalog_entity_terms.term_id)`, entity, int(vocab)); err != nil {
		return err
	}
	return TouchEntityTx(ctx, tx, entity)
}

// Credit is one person's credit on an entity from one provider.
type Credit struct {
	PersonKey        string // people identity key ('' for an unlinked credit)
	PersonName       string
	PersonSortName   string
	ProviderPersonID string
	CreditID         string
	CreditedName     string
	Role, Department string
	Ordinal          int
}

// SetCreditsTx replaces an entity's credits from one provider, creating the
// people they name.
func SetCreditsTx(ctx context.Context, tx *sql.Tx, entity int64, provider string, credits []Credit) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_credits WHERE entity_id=? AND provider=?`, entity, provider); err != nil {
		return err
	}
	var next int
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(max(ord),-1)+1 FROM catalog_credits WHERE entity_id=?`, entity).Scan(&next); err != nil {
		return err
	}
	for _, c := range credits {
		var person any
		if c.PersonKey != "" {
			var id int64
			// The people directory orders by sort_name on its index (binary
			// collation), so the stored form is folded: "de Niro" before "Zeta".
			sortName := c.PersonSortName
			if sortName == "" {
				sortName = c.PersonName
			}
			sortName = strings.ToLower(sortName)
			if err := tx.QueryRowContext(ctx, `INSERT INTO catalog_people(token,identity_key,name,sort_name) VALUES(?,?,?,?)
 ON CONFLICT(identity_key) DO UPDATE SET name=name RETURNING id`, newPersonToken(), c.PersonKey, c.PersonName, sortName).Scan(&id); err != nil {
				return err
			}
			person = id
		}
		role, err := creditLabel(ctx, tx, c.Role)
		if err != nil {
			return err
		}
		department, err := creditLabel(ctx, tx, c.Department)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO catalog_credits(entity_id,ord,person_id,role_id,department_id,credited_name,provider,credit_id,provider_person_id,source_ordinal) VALUES(?,?,?,?,?,?,?,?,?,?)`,
			entity, next, person, role, department, c.CreditedName, provider, c.CreditID, c.ProviderPersonID, c.Ordinal); err != nil {
			return err
		}
		next++
	}
	return TouchEntityTx(ctx, tx, entity)
}

func creditLabel(ctx context.Context, tx *sql.Tx, label string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM catalog_credit_labels WHERE label=?`, label).Scan(&id)
	if !errors.Is(err, sql.ErrNoRows) {
		return id, err
	}
	err = tx.QueryRowContext(ctx, `INSERT INTO catalog_credit_labels(label) VALUES(?) RETURNING id`, label).Scan(&id)
	return id, err
}

// SetAttributesTx replaces an item's values of one attribute field (content
// rating, label, studio, tag, …). A new field is a new catalog_attribute_fields row.
func SetAttributesTx(ctx context.Context, tx *sql.Tx, item int64, field string, values []string) error {
	var fieldID int64
	if err := tx.QueryRowContext(ctx, `INSERT INTO catalog_attribute_fields(field) VALUES(?) ON CONFLICT(field) DO UPDATE SET field=field RETURNING id`, field).Scan(&fieldID); err != nil {
		return err
	}
	// Driven by the item's own edges: a field's terms are server-wide (a studio
	// or tag field can hold hundreds of thousands).
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_item_attribute_edges WHERE item_id=?1 AND (SELECT t.field_id FROM catalog_attribute_terms t WHERE t.id=term_id)=?2`, item, fieldID); err != nil {
		return err
	}
	for i, v := range values {
		key := strings.ToLower(strings.TrimSpace(v))
		if key == "" {
			continue
		}
		var term int64
		if err := tx.QueryRowContext(ctx, `INSERT INTO catalog_attribute_terms(field_id,value_key) VALUES(?,?) ON CONFLICT(field_id,value_key) DO UPDATE SET value_key=value_key RETURNING id`, fieldID, key).Scan(&term); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO catalog_item_attribute_edges(item_id,term_id,source_value,source_rowid) VALUES(?,?,?,?)`, item, term, v, i); err != nil {
			return err
		}
	}
	return TouchEntityTx(ctx, tx, item)
}

// SetCollectionMemberTx adds (present) or removes an item of a collection.
func SetCollectionMemberTx(ctx context.Context, tx *sql.Tx, collection, item int64, orderKey string, present bool) error {
	var res sql.Result
	var err error
	if present {
		res, err = tx.ExecContext(ctx, `INSERT INTO catalog_collection_members(collection_id,item_id,order_key) VALUES(?,?,?) ON CONFLICT(collection_id,item_id) DO UPDATE SET order_key=excluded.order_key WHERE catalog_collection_members.order_key IS NOT excluded.order_key`, collection, item, orderKey)
	} else {
		res, err = tx.ExecContext(ctx, `DELETE FROM catalog_collection_members WHERE collection_id=? AND item_id=?`, collection, item)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil
	}
	// member_count follows the member rows by trigger (feeds.sql), cascades
	// included.
	if err = TouchTx(ctx, tx, DomainBrowseEdges, item); err != nil {
		return err
	}
	return TouchTx(ctx, tx, DomainBrowseRows, collection)
}

// SetSongArtistsTx replaces a song's credited artists.
func SetSongArtistsTx(ctx context.Context, tx *sql.Tx, song int64, artists []int64) error {
	// NOT EXISTS, not NOT IN: a nil list encodes as JSON null, whose one NULL
	// row made NOT IN false for every artist, so clearing kept them all.
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_song_artists WHERE song_id=? AND NOT EXISTS(SELECT 1 FROM json_each(?) j WHERE j.value=artist_id)`, song, mustJSON(artists)); err != nil {
		return err
	}
	for _, a := range artists {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO catalog_song_artists(song_id,artist_id) VALUES(?,?)`, song, a); err != nil {
			return err
		}
	}
	return TouchTx(ctx, tx, DomainBrowseEdges, song)
}

// Chapter is one chapter of an audiobook file.
type Chapter struct {
	Title      string
	Start, End float64
}

// SetBookChaptersTx replaces an audiobook file's chapters.
func SetBookChaptersTx(ctx context.Context, tx *sql.Tx, file int64, chapters []Chapter) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM catalog_book_chapters WHERE file_id=?`, file); err != nil {
		return err
	}
	for i, c := range chapters {
		if _, err := tx.ExecContext(ctx, `INSERT INTO catalog_book_chapters(file_id,chapter_index,title,start_seconds,end_seconds) VALUES(?,?,?,?,?)`, file, i, c.Title, c.Start, c.End); err != nil {
			return err
		}
	}
	return nil
}
