package catalog

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/identity"
)

type Collection struct {
	ID        string `json:"id"`
	LibraryID string `json:"libraryId"`
	Name      string `json:"name"`
	ItemCount int    `json:"itemCount"`
}

func collectionName(name string) (string, string, error) {
	name = strings.TrimSpace(name)
	if len(name) == 0 || len(name) > 100 {
		return "", "", errors.New("collection name must contain 1–100 bytes")
	}
	for _, c := range name {
		if unicode.IsControl(c) {
			return "", "", errors.New("collection name contains a control character")
		}
	}
	return name, strings.ToLower(strings.Join(strings.Fields(name), " ")), nil
}

const collectionColumns = `pid(e.public_id),l.library_id,e.title`

func (s *Service) Collection(id string) (Collection, error) {
	var c Collection
	err := s.read().QueryRow(`SELECT `+collectionColumns+`,c.member_count FROM catalog_entities e JOIN catalog_collections c ON c.entity_id=e.id JOIN catalog_libraries l ON l.id=c.library_id WHERE e.public_id=pid_blob(?) AND `+collectionHeadFence, id).Scan(&c.ID, &c.LibraryID, &c.Name, &c.ItemCount)
	return c, err
}

// collectionTx resolves a collection's public id to its entity and library.
func collectionTx(ctx context.Context, tx *sql.Tx, id string) (int64, int64, error) {
	var entity, library int64
	// A malformed id is nobody's collection: not found, before it reaches SQL.
	if _, ok := entityid.Decode(id); !ok {
		return 0, 0, sql.ErrNoRows
	}
	err := tx.QueryRowContext(ctx, `SELECT e.id,e.library_id FROM catalog_entities e JOIN catalog_collections c ON c.entity_id=e.id WHERE e.public_id=pid_blob(?) AND e.retired=0`, id).Scan(&entity, &library)
	return entity, library, err
}

func (s *Service) CreateCollection(library, name string) (Collection, error) {
	if _, err := s.movieLibrary(library); err != nil {
		return Collection{}, err
	}
	name, key, err := collectionName(name)
	if err != nil {
		return Collection{}, err
	}
	var id string
	ctx := s.Context()
	err = dbwork.WithWriteTx(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive), func(tx *sql.Tx) error {
		handle, err := compactcatalog.LibraryTx(ctx, tx, library)
		if err != nil {
			return err
		}
		// A collection is made by its owner, not found in a folder: its identity
		// key is its own, so renaming one never hands its id to a new one.
		entity, _, err := compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: compactcatalog.Collection, Key: compactcatalog.CollectionKey(identity.Token()), Title: name})
		if err != nil {
			return err
		}
		if err = compactcatalog.SetFactsTx(ctx, tx, entity, map[string]any{"library_id": handle, "name_key": key, "created_at": time.Now().UTC().Format("2006-01-02T15:04:05.000Z")}); err != nil {
			return err
		}
		id, err = entityid.Public(ctx, tx, entity)
		return err
	})
	if err != nil {
		var coder interface{ Code() int }
		if errors.As(err, &coder) && coder.Code()&255 == 19 {
			return Collection{}, ErrCollectionConflict
		}
		return Collection{}, err
	}
	return s.Collection(id)
}
func (s *Service) RenameCollection(id, name string) (Collection, error) {
	name, key, err := collectionName(name)
	if err != nil {
		return Collection{}, err
	}
	ctx := s.Context()
	err = dbwork.WithWriteTx(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive), func(tx *sql.Tx) error {
		entity, _, err := collectionTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = compactcatalog.SetFieldsTx(ctx, tx, entity, compactcatalog.Owner, map[string]any{"title": name}); err != nil {
			return err
		}
		return compactcatalog.SetFieldsTx(ctx, tx, entity, compactcatalog.Owner, map[string]any{"name_key": key})
	})
	if err != nil {
		var coder interface{ Code() int }
		if errors.As(err, &coder) && coder.Code()&255 == 19 {
			return Collection{}, ErrCollectionConflict
		}
		return Collection{}, err
	}
	return s.Collection(id)
}
func (s *Service) DeleteCollection(id string) error {
	ctx := s.Context()
	return dbwork.WithWriteTx(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive), func(tx *sql.Tx) error {
		entity, _, err := collectionTx(ctx, tx, id)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		return compactcatalog.DeleteEntityTx(ctx, tx, entity)
	})
}

// collectionMovieTx resolves a movie of the collection's library (0 when the
// item is not a movie of that library).
func collectionMovieTx(ctx context.Context, tx *sql.Tx, library int64, item string) (int64, error) {
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM catalog_entities WHERE public_id=pid_blob(?) AND library_id=? AND kind=?`, item, library, compactcatalog.Movie).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	return id, err
}

func (s *Service) SetCollectionItem(collection, item string, present bool) error {
	ctx := s.Context()
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if err != nil {
		return err
	}
	tx := gated.Tx()
	defer gated.Rollback()
	entity, library, err := collectionTx(ctx, tx, collection)
	if err != nil {
		return err
	}
	movie, err := collectionMovieTx(ctx, tx, library, item)
	if err != nil {
		return err
	}
	if movie == 0 {
		return sql.ErrNoRows
	}
	if err = compactcatalog.SetCollectionMemberTx(ctx, tx, entity, movie, "", present); err != nil {
		return err
	}
	return gated.Commit()
}

// EntryOutcomes reports one membership batch item by item. Library collections
// and viewer-owned saved collections publish the same shape so a client renders
// one result view for both.
type EntryOutcome struct {
	ItemID string `json:"itemId"`
	Code   string `json:"code"`
}
type EntryOutcomes struct {
	Added     []string       `json:"added"`
	Removed   []string       `json:"removed"`
	Unchanged []string       `json:"unchanged"`
	Failed    []EntryOutcome `json:"failed"`
}
type CollectionBatch struct {
	Collection Collection    `json:"collection"`
	Entries    EntryOutcomes `json:"entries"`
}

const MaxCollectionBatch = 200

func newEntryOutcomes() EntryOutcomes {
	return EntryOutcomes{Added: []string{}, Removed: []string{}, Unchanged: []string{}, Failed: []EntryOutcome{}}
}
func (o *EntryOutcomes) fail(id, code string) {
	o.Failed = append(o.Failed, EntryOutcome{ItemID: id, Code: code})
}

// SetCollectionItems applies a library collection membership batch atomically
// in storage while reporting each item independently. An item outside the
// collection's library, of the wrong kind, or already absent is one outcome,
// not a failed request.
func (s *Service) SetCollectionItems(collection string, add, remove []string) (CollectionBatch, error) {
	out := CollectionBatch{Entries: newEntryOutcomes()}
	if len(add)+len(remove) == 0 || len(add) > MaxCollectionBatch || len(remove) > MaxCollectionBatch {
		return out, errors.New("a collection membership batch carries 1 to 200 additions and 200 removals")
	}
	seen := map[string]bool{}
	for _, id := range append(append([]string{}, add...), remove...) {
		if id == "" || len(id) > 256 || seen[id] {
			return out, errors.New("each batch item must name a distinct itemId")
		}
		seen[id] = true
	}
	ctx := s.Context()
	gated2, err := dbwork.Begin(ctx, s.db, dbwork.ClassFrom(ctx, dbwork.ClassInteractive))
	if err != nil {
		return out, err
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	entity, library, err := collectionTx(ctx, tx, collection)
	if err != nil {
		return out, err
	}
	for _, item := range add {
		movie, err := collectionMovieTx(ctx, tx, library, item)
		if err != nil {
			return out, err
		}
		if movie == 0 {
			out.Entries.fail(item, "not_found")
			continue
		}
		var member bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_collection_members WHERE collection_id=? AND item_id=?)`, entity, movie).Scan(&member); err != nil {
			return out, err
		}
		if member {
			out.Entries.Unchanged = append(out.Entries.Unchanged, item)
			continue
		}
		if err = compactcatalog.SetCollectionMemberTx(ctx, tx, entity, movie, "", true); err != nil {
			return out, err
		}
		out.Entries.Added = append(out.Entries.Added, item)
	}
	for _, item := range remove {
		movie, err := collectionMovieTx(ctx, tx, library, item)
		if err != nil {
			return out, err
		}
		var member bool
		if movie != 0 {
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_collection_members WHERE collection_id=? AND item_id=?)`, entity, movie).Scan(&member); err != nil {
				return out, err
			}
		}
		if !member {
			out.Entries.fail(item, "not_found")
			continue
		}
		if err = compactcatalog.SetCollectionMemberTx(ctx, tx, entity, movie, "", false); err != nil {
			return out, err
		}
		out.Entries.Removed = append(out.Entries.Removed, item)
	}
	if err = gated2.Commit(); err != nil {
		return out, err
	}
	out.Collection, err = s.Collection(collection)
	return out, err
}

// VisibleCollection is Collection as one viewer may see it; a hidden or unknown
// collection is sql.ErrNoRows.
// CollectionClassesReady answers ErrVisibilityBuilding while any of a
// restricted viewer's libraries is still building its restriction class, for
// every collection id alike. A hidden collection must answer exactly as an
// absent one (SEC-02), so readiness is decided before the id is looked up:
// otherwise a hidden id would answer "building" where an absent one answers
// "not found". Unrestricted viewers are always ready. One indexed probe per
// library.
func (s *Service) CollectionClassesReady(v Viewer) error {
	restrictions := v.EffectiveRestrictions()
	if !restrictions.Active() {
		return nil
	}
	for _, library := range v.Libraries {
		if _, _, err := s.collectionClassReady(library, restrictions); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) VisibleCollection(ctx context.Context, v Viewer, id string) (Collection, error) {
	if err := s.CollectionClassesReady(v); err != nil {
		return Collection{}, err
	}
	if !v.EffectiveRestrictions().Active() {
		var c Collection
		err := s.read().QueryRowContext(ctx, `SELECT `+collectionColumns+`,c.member_count FROM catalog_entities e JOIN catalog_collections c ON c.entity_id=e.id JOIN catalog_libraries l ON l.id=c.library_id WHERE e.public_id=pid_blob(?) AND `+collectionHeadFence, id).Scan(&c.ID, &c.LibraryID, &c.Name, &c.ItemCount)
		if err == nil && !v.AllowsLibrary(c.LibraryID) {
			return Collection{}, sql.ErrNoRows
		}
		return c, err
	}
	var library string
	err := s.read().QueryRowContext(ctx, `SELECT l.library_id FROM catalog_entities e JOIN catalog_collections c ON c.entity_id=e.id JOIN catalog_libraries l ON l.id=c.library_id WHERE e.public_id=pid_blob(?) AND `+collectionHeadFence, id).Scan(&library)
	if err != nil {
		return Collection{}, err
	}
	if !v.AllowsLibrary(library) {
		return Collection{}, sql.ErrNoRows
	}
	class, generation, err := s.collectionClassReady(library, v.EffectiveRestrictions())
	if err != nil {
		return Collection{}, err
	}
	var c Collection
	err = s.read().QueryRowContext(ctx, `SELECT `+collectionColumns+`,COALESCE(vc.total,0) FROM catalog_entities e JOIN catalog_collections c ON c.entity_id=e.id JOIN catalog_libraries l ON l.id=c.library_id LEFT JOIN catalog_collection_visible_counts vc ON vc.class_id=? AND vc.generation=? AND vc.collection_id=c.entity_id WHERE e.public_id=pid_blob(?) AND `+collectionHeadFence+` AND (c.member_count=0 OR COALESCE(vc.total,0)>0)`, class, generation, id).Scan(&c.ID, &c.LibraryID, &c.Name, &c.ItemCount)
	return c, err
}

// Collections lists a library's collections as the viewer may see them.
func (s *Service) Collections(library string, v Viewer, cursor string, limit int) ([]Collection, string, error) {
	if _, err := s.movieLibrary(library); err != nil {
		return nil, "", err
	}
	if !v.AllowsLibrary(library) {
		return []Collection{}, "", nil
	}
	limit = pageLimit(limit)
	scope := cursorScope{Library: library, Viewer: v.Profile + ":" + v.Fence + ":" + RestrictionFence(v.EffectiveRestrictions()), Sort: "collection_name", Direction: "asc"}
	query := `SELECT e.id,` + collectionColumns + `,c.name_key,c.member_count FROM catalog_collections c JOIN catalog_entities e ON e.id=c.entity_id JOIN catalog_libraries l ON l.id=c.library_id WHERE l.library_id=? AND ` + collectionHeadFence
	args := []any{library}
	if v.EffectiveRestrictions().Active() {
		class, generation, err := s.collectionClassReady(library, v.EffectiveRestrictions())
		if err != nil {
			return nil, "", err
		}
		query = `SELECT e.id,` + collectionColumns + `,c.name_key,COALESCE(vc.total,0) FROM catalog_collections c JOIN catalog_entities e ON e.id=c.entity_id JOIN catalog_libraries l ON l.id=c.library_id LEFT JOIN catalog_collection_visible_counts vc ON vc.class_id=? AND vc.generation=? AND vc.collection_id=c.entity_id WHERE l.library_id=? AND ` + collectionHeadFence + ` AND (c.member_count=0 OR COALESCE(vc.total,0)>0)`
		args = []any{class, generation, library}
	}
	expires := time.Now().Add(30 * time.Minute).Unix()
	if cursor != "" {
		c, err := s.decodeCursor(cursor, scope)
		if err != nil {
			return nil, "", err
		}
		after, err := strconv.ParseInt(c.ID, 10, 64)
		if err != nil {
			return nil, "", ErrCursor
		}
		query += ` AND (c.name_key,e.id)>(?,?)`
		args = append(args, c.Value, after)
		expires = c.Expires
	}
	query += ` ORDER BY c.name_key,e.id LIMIT ?`
	args = append(args, limit+1)
	rows, err := s.read().Query(query, args...)
	if err != nil {
		return nil, "", err
	}
	out := []Collection{}
	keys := []string{}
	internal := []int64{}
	for rows.Next() {
		var row Collection
		var key string
		var entity int64
		if err = rows.Scan(&entity, &row.ID, &row.LibraryID, &row.Name, &key, &row.ItemCount); err != nil {
			rows.Close()
			return nil, "", err
		}
		out = append(out, row)
		keys = append(keys, key)
		internal = append(internal, entity)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next, err = s.encodeCursor(cursorValue{Scope: scope, Value: keys[limit-1], ID: strconv.FormatInt(internal[limit-1], 10), Expires: expires})
	}
	return out, next, err
}
