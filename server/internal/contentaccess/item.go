package contentaccess

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"portico.local/server/internal/access"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
)

// VisibleItemTx is the service-level item fence shared by HTTP, playback,
// preparation and downloads. It applies profile restrictions and local member
// ceilings in the caller's authority transaction. Denial reveals no title.
func VisibleItemTx(ctx context.Context, tx *sql.Tx, p identity.Principal, item string) error {
	if tx == nil || item == "" {
		return identity.ErrContentRestricted
	}
	restrictions, err := identity.RestrictionsForViewerTx(ctx, tx, p.Viewer)
	if err != nil {
		return err
	}
	content := restrictions.Content()
	clause, args := catalog.ItemRestrictionSQL("i.id", content)
	query := `SELECT EXISTS(SELECT 1 FROM catalog_entities i WHERE i.public_id=pid_blob(?) AND (` + clause + `))`
	args = append([]any{item}, args...)
	var allowed bool
	if err = tx.QueryRowContext(ctx, query, args...).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return identity.ErrContentRestricted
	}
	// P8: with Recordings off, a published recording is absent even to a
	// caller who knows its id. One primary-key probe.
	if content.BlockRecordings {
		var recording bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dvr_catalog_provenance WHERE item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)))`, item).Scan(&recording); err != nil {
			return err
		}
		if recording {
			return identity.ErrContentRestricted
		}
	}
	if content.Active() && (content.MaximumAge != nil || content.BlockUnrated) {
		var pending bool
		if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities e JOIN catalog_item_attribute_edges a ON a.item_id=e.id JOIN catalog_attribute_terms term ON term.id=a.term_id JOIN content_rating_pending p ON p.value_key=term.value_key WHERE e.public_id=pid_blob(?) AND term.field_id=1)`, item).Scan(&pending); err != nil {
			return err
		}
		if pending {
			return identity.ErrContentRestricted
		}
	}
	enforcer := access.Enforcer{Store: &access.Store{}}
	if err = enforcer.VisibleItem(ctx, tx, p, item); errors.Is(err, access.ErrContentRating) || errors.Is(err, access.ErrLabelDenied) {
		return identity.ErrContentRestricted
	}
	return err
}

// VisibleKnownItemTx is VisibleItemTx for a caller that already holds the item
// (a media grant, a download receipt or preparation, a queue being built).
// Catalogue facts are written synchronously, so there is no pending state to
// wait for: the check is the same fence. It tells the caller nothing new: it
// had the item.
func VisibleKnownItemTx(ctx context.Context, tx *sql.Tx, p identity.Principal, item string) error {
	return VisibleItemTx(ctx, tx, p, item)
}

// VisibleItemsSQL is VisibleItemTx for a set: one SQL predicate over the item id
// expression `item` that admits exactly the items VisibleItemTx admits, and only
// in libraries allowedLibrary accepts. A surface that lists or snapshots many
// items applies it in the statement that reads them (inaccessible ≡ absent),
// instead of one check per item.
// Unrestricted reports whether the caller's item fence (VisibleItemsSQL)
// admits every item: every library is allowed, and no content or member
// restriction applies. Such a caller sees a container exactly as it is, so
// what is derived from the container alone (its size) holds for every such
// caller.
func Unrestricted(ctx context.Context, tx *sql.Tx, p identity.Principal, allowedLibrary func(library string) error) (bool, error) {
	if tx == nil || allowedLibrary == nil {
		return false, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT library_id FROM catalog_libraries WHERE retired=0`)
	if err != nil {
		return false, err
	}
	libraries := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		libraries = append(libraries, id)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return false, err
	}
	for _, id := range libraries {
		if allowedLibrary(id) != nil {
			return false, nil
		}
	}
	restrictions, err := identity.RestrictionsForViewerTx(ctx, tx, p.Viewer)
	if err != nil {
		return false, err
	}
	if clause, _ := catalog.ItemRestrictionSQL("item", restrictions.Content()); clause != "1" {
		return false, nil
	}
	enforcer := access.Enforcer{Store: &access.Store{}}
	member, _, err := enforcer.VisibilityClause(ctx, tx, p, "item")
	if err != nil {
		return false, err
	}
	return member == "", nil
}

// VisibleItemsSQL is the viewer's visibility predicate for the item named by
// item, an SQL expression yielding its integer entity id (catalog_entities.id),
// as every restriction helper takes.
func VisibleItemsSQL(ctx context.Context, tx *sql.Tx, p identity.Principal, item string, allowedLibrary func(library string) error) (string, []any, error) {
	if tx == nil {
		return "", nil, identity.ErrContentRestricted
	}
	clauses, args := []string{`EXISTS(SELECT 1 FROM catalog_entities pub WHERE pub.id=` + item + `)`}, []any{}
	if allowedLibrary != nil {
		rows, err := tx.QueryContext(ctx, `SELECT library_id FROM catalog_libraries WHERE retired=0`)
		if err != nil {
			return "", nil, err
		}
		libraries := []string{}
		for rows.Next() {
			var id string
			if err = rows.Scan(&id); err != nil {
				rows.Close()
				return "", nil, err
			}
			libraries = append(libraries, id)
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return "", nil, err
		}
		allowed := []string{}
		for _, id := range libraries {
			if allowedLibrary(id) == nil {
				allowed = append(allowed, id)
			}
		}
		raw, _ := json.Marshal(allowed)
		clauses = append(clauses, `EXISTS(SELECT 1 FROM catalog_entities visible_item JOIN catalog_libraries visible_library ON visible_library.id=visible_item.library_id WHERE visible_item.id=`+item+` AND visible_library.library_id IN (SELECT value FROM json_each(?)))`)
		args = append(args, string(raw))
	}
	restrictions, err := identity.RestrictionsForViewerTx(ctx, tx, p.Viewer)
	if err != nil {
		return "", nil, err
	}
	content := restrictions.Content()
	clause, values := catalog.ItemRestrictionSQL(item, content)
	clauses, args = append(clauses, clause), append(args, values...)
	if content.BlockRecordings {
		clauses = append(clauses, `NOT EXISTS(SELECT 1 FROM dvr_catalog_provenance recording WHERE recording.item_id=`+item+`)`)
	}
	if content.Active() && (content.MaximumAge != nil || content.BlockUnrated) {
		clauses = append(clauses, `NOT EXISTS(SELECT 1 FROM catalog_entities ci JOIN catalog_item_attribute_edges pending_rating ON pending_rating.item_id=ci.id JOIN catalog_attribute_terms term ON term.id=pending_rating.term_id JOIN content_rating_pending pending ON pending.value_key=term.value_key WHERE ci.id=`+item+` AND term.field_id=1)`)
	}
	enforcer := access.Enforcer{Store: &access.Store{}}
	member, values, err := enforcer.VisibilityClause(ctx, tx, p, item)
	if err != nil {
		return "", nil, err
	}
	if member != "" {
		clauses, args = append(clauses, "("+member+")"), append(args, values...)
	}
	return "(" + strings.Join(clauses, " AND ") + ")", args, nil
}
