package catalog

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/dbwork"
)

// Pins carry order. The boolean pin remains the record of intent; this file owns
// the viewer's arrangement of what they pinned, both for saved resources and for
// the libraries in the navigation rail.

var ErrPinOrderConflict = errors.New("the pin order changed on another device; reload and try again")
var ErrLibraryNavigationConflict = errors.New("library pins changed on another device; reload and try again")

const maximumPinOrder = 200
const maximumPinnedLibraries = 100

type PinOrderEntry struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}
type PinOrder struct {
	Revision int64           `json:"revision"`
	Order    []PinOrderEntry `json:"order"`
}
type LibraryNavigation struct {
	PinnedLibraryIDs []string `json:"pinnedLibraryIds"`
	Revision         int64    `json:"revision"`
}

func pinOrderRevision(q personalReader, owner string) (int64, error) {
	var revision int64
	e := q.QueryRow(`SELECT COALESCE((SELECT revision FROM saved_pin_order_revisions WHERE owner_key=?),0)`, owner).Scan(&revision)
	return revision, e
}

// PinOrder reads the viewer's arrangement. Pinned resources with no recorded
// position are not invented here; they sort after the arranged ones by name.
func (s *Service) PinOrder(a ResourceActor) (PinOrder, error) {
	out := PinOrder{Order: []PinOrderEntry{}}
	owner := actorKey(a)
	revision, e := pinOrderRevision(s.read(), owner)
	if e != nil {
		return out, e
	}
	out.Revision = revision
	rows, e := s.read().Query(`SELECT kind,resource_id FROM saved_pin_order WHERE owner_key=? ORDER BY position,resource_id`, owner)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var entry PinOrderEntry
		if e = rows.Scan(&entry.Kind, &entry.ID); e != nil {
			return out, e
		}
		out.Order = append(out.Order, entry)
	}
	return out, rows.Err()
}

// SetPinOrder replaces the arrangement in one transaction. Only resources the
// viewer has actually pinned may appear, so an order can never leak the
// existence of something they cannot see.
func (s *Service) SetPinOrder(a ResourceActor, expected int64, order []PinOrderEntry, authorize func(*sql.Tx) error) (PinOrder, error) {
	out := PinOrder{Order: []PinOrderEntry{}}
	if a.Authority == "" || a.AccountID == "" || a.ProfileID == "" {
		return out, errors.New("a complete viewer is required")
	}
	if len(order) > maximumPinOrder {
		return out, errors.New("pin order exceeds 200 entries")
	}
	owner := actorKey(a)
	gated, e := dbwork.Begin(context.Background(), s.db, dbwork.ClassFrom(context.Background(), dbwork.ClassInteractive))
	if e != nil {
		return out, e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if e = authorize(tx); e != nil {
		return out, e
	}
	revision, e := pinOrderRevision(tx, owner)
	if e != nil {
		return out, e
	}
	if revision != expected {
		return out, ErrPinOrderConflict
	}
	seen := map[PinOrderEntry]bool{}
	for _, entry := range order {
		if entry.Kind != "collection" && entry.Kind != "view" && entry.Kind != "playlist" {
			return out, errors.New("pin kind must be collection, view or playlist")
		}
		if seen[entry] {
			return out, errors.New("a pinned resource may appear once in the order")
		}
		seen[entry] = true
		var pinned int
		if e = tx.QueryRow(`SELECT COALESCE((SELECT pinned FROM saved_pins WHERE owner_key=? AND kind=? AND resource_id=?),0)`, owner, entry.Kind, entry.ID).Scan(&pinned); e != nil {
			return out, e
		}
		if pinned != 1 {
			return out, sql.ErrNoRows
		}
	}
	if _, e = tx.Exec(`DELETE FROM saved_pin_order WHERE owner_key=?`, owner); e != nil {
		return out, e
	}
	for index, entry := range order {
		if _, e = tx.Exec(`INSERT INTO saved_pin_order(owner_key,kind,resource_id,position) VALUES(?,?,?,?)`, owner, entry.Kind, entry.ID, index); e != nil {
			return out, e
		}
	}
	revision++
	if _, e = tx.Exec(`INSERT INTO saved_pin_order_revisions(owner_key,revision) VALUES(?,?) ON CONFLICT(owner_key) DO UPDATE SET revision=excluded.revision`, owner, revision); e != nil {
		return out, e
	}
	if e = gated.Commit(); e != nil {
		return out, e
	}
	out.Revision = revision
	out.Order = append(out.Order, order...)
	return out, nil
}

// LibraryNavigation reads the viewer's pinned libraries in their arrangement.
// Libraries that disappeared are dropped rather than reported as holes.
func (s *Service) LibraryNavigation(a ResourceActor, allowed func(string) bool) (LibraryNavigation, error) {
	out := LibraryNavigation{PinnedLibraryIDs: []string{}}
	owner := actorKey(a)
	if e := s.read().QueryRow(`SELECT COALESCE((SELECT revision FROM library_navigation_revisions WHERE owner_key=?),0)`, owner).Scan(&out.Revision); e != nil {
		return out, e
	}
	rows, e := s.read().Query(`SELECT n.library_id FROM library_navigation n JOIN libraries l ON l.id=n.library_id WHERE n.owner_key=? ORDER BY n.position,n.library_id`, owner)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return out, e
		}
		if allowed == nil || allowed(id) {
			out.PinnedLibraryIDs = append(out.PinnedLibraryIDs, id)
		}
	}
	return out, rows.Err()
}

// SetLibraryNavigation replaces the pinned library order. The route's authorize
// callback proves the viewer may see each library inside the same transaction.
func (s *Service) SetLibraryNavigation(a ResourceActor, expected int64, libraries []string, authorize func(*sql.Tx, string) error) (LibraryNavigation, error) {
	out := LibraryNavigation{PinnedLibraryIDs: []string{}}
	if a.Authority == "" || a.AccountID == "" || a.ProfileID == "" {
		return out, errors.New("a complete viewer is required")
	}
	if len(libraries) > maximumPinnedLibraries {
		return out, errors.New("pinned libraries exceed 100 entries")
	}
	owner := actorKey(a)
	gated2, e := dbwork.Begin(context.Background(), s.db, dbwork.ClassFrom(context.Background(), dbwork.ClassInteractive))
	if e != nil {
		return out, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	if e = authorize(tx, ""); e != nil {
		return out, e
	}
	var revision int64
	if e = tx.QueryRow(`SELECT COALESCE((SELECT revision FROM library_navigation_revisions WHERE owner_key=?),0)`, owner).Scan(&revision); e != nil {
		return out, e
	}
	if revision != expected {
		return out, ErrLibraryNavigationConflict
	}
	seen := map[string]bool{}
	for _, library := range libraries {
		if seen[library] {
			return out, errors.New("a library may appear once in the pin order")
		}
		seen[library] = true
		var exists int
		if e = tx.QueryRow(`SELECT 1 FROM libraries WHERE id=?`, library).Scan(&exists); e != nil {
			return out, e
		}
		if e = authorize(tx, library); e != nil {
			return out, e
		}
	}
	if _, e = tx.Exec(`DELETE FROM library_navigation WHERE owner_key=?`, owner); e != nil {
		return out, e
	}
	for index, library := range libraries {
		if _, e = tx.Exec(`INSERT INTO library_navigation(owner_key,library_id,position) VALUES(?,?,?)`, owner, library, index); e != nil {
			return out, e
		}
	}
	revision++
	if _, e = tx.Exec(`INSERT INTO library_navigation_revisions(owner_key,revision) VALUES(?,?) ON CONFLICT(owner_key) DO UPDATE SET revision=excluded.revision`, owner, revision); e != nil {
		return out, e
	}
	if e = gated2.Commit(); e != nil {
		return out, e
	}
	out.Revision = revision
	out.PinnedLibraryIDs = append(out.PinnedLibraryIDs, libraries...)
	return out, nil
}

// LibrariesForViewer returns pinned libraries first, in the viewer's order, each
// flagged so a client never has to re-derive the arrangement.
func (s *Service) LibrariesForViewer(a ResourceActor) ([]Library, error) {
	all, e := s.Libraries()
	if e != nil {
		return nil, e
	}
	navigation, e := s.LibraryNavigation(a, nil)
	if e != nil {
		return nil, e
	}
	byID := map[string]Library{}
	for _, library := range all {
		byID[library.ID] = library
	}
	out := make([]Library, 0, len(all))
	taken := map[string]bool{}
	for _, id := range navigation.PinnedLibraryIDs {
		library, ok := byID[id]
		if !ok || taken[id] {
			continue
		}
		library.Pinned = true
		taken[id] = true
		out = append(out, library)
	}
	for _, library := range all {
		if !taken[library.ID] {
			out = append(out, library)
		}
	}
	return out, nil
}
