package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/facettext"
)

// SearchQueueQuery names a complete song search, never the currently loaded
// cards. Durable queue capture streams its keys on the retained transaction.
type SearchQueueQuery struct {
	Q          string   `json:"q"`
	Group      string   `json:"group"`
	Sort       string   `json:"sort,omitempty"`
	Direction  string   `json:"direction,omitempty"`
	LibraryIDs []string `json:"libraryIds,omitempty"`
}

func (q SearchQueueQuery) Validate() error {
	if q.Group != "songs" || len(q.LibraryIDs) > 64 {
		return ErrSearchQuery
	}
	if _, _, err := NormalizeSearch(q.Q); err != nil {
		return err
	}
	if q.Sort != "" && !containsString(catalogSorts, q.Sort) {
		return ErrSearchQuery
	}
	if q.Direction != "" && q.Direction != "asc" && q.Direction != "desc" {
		return ErrSearchQuery
	}
	return nil
}
func (s *Service) SearchQueueKeysTx(ctx context.Context, tx *sql.Tx, viewer Viewer, q SearchQueueQuery) (QueueSelection, error) {
	if err := q.Validate(); err != nil {
		return QueueSelection{}, err
	}
	clone := *s.WithContext(ctx)
	clone.readTx = tx
	s = &clone
	if err := s.prepareViewer(viewer); err != nil {
		return QueueSelection{}, err
	}
	normalized, match, err := NormalizeSearch(q.Q)
	if err != nil {
		return QueueSelection{}, err
	}
	libraries := viewer.Libraries
	if len(q.LibraryIDs) > 0 {
		libraries = []string{}
		seen := map[string]bool{}
		for _, id := range q.LibraryIDs {
			if !viewer.AllowsLibrary(id) || seen[id] {
				return QueueSelection{}, ErrSearchQuery
			}
			seen[id] = true
			libraries = append(libraries, id)
		}
	}
	r := SearchRequest{Viewer: viewer, Profile: viewer.Profile, ViewerFence: viewer.Fence, Libraries: libraries, Q: normalized, Sort: q.Sort, Direction: q.Direction, Restrictions: viewer.Restrictions}
	if r.Sort == "" {
		r.Sort = "relevance"
	}
	if r.Direction == "" {
		r.Direction = searchDefaultDirection(r.Sort)
	}
	tokens, err := s.searchLibraryTokens(ctx, libraries)
	if err != nil {
		return QueueSelection{}, err
	}
	if len(tokens) == 0 {
		tokens = []string{"lnone"}
	}
	match = `kind : k7 AND kind : (` + strings.Join(tokens, " OR ") + `) AND search : (` + match + `)`
	order := catalogOrder(r.Sort, "song", normalized)
	tie := "entity_id"
	if r.Sort == "relevance" {
		tie = "public_id"
	}
	from := `(SELECT rowid entity_id,-rank relevance FROM catalog_search_titles WHERE catalog_search_titles MATCH ?) m CROSS JOIN catalog_search_documents d ON d.entity_id=m.entity_id CROSS JOIN catalog_entities e ON e.id=m.entity_id JOIN catalog_libraries l ON l.id=e.library_id`
	predicate, visibleArgs := viewer.itemVisibilitySQL("e.id")
	where := `e.kind=7 AND l.library_id IN(SELECT value FROM json_each(?)) AND ` + predicate + ` AND EXISTS(SELECT 1 FROM catalog_songs song JOIN catalog_entities album ON album.id=song.album_id WHERE song.entity_id=e.id AND album.library_id=e.library_id AND album.retired=0) AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=e.id)`
	args := append(append([]any{}, order.args...), match, idsJSON(libraries))
	args = append(args, visibleArgs...)
	if clause, values := searchCJKPredicate("d.normalized", normalized); clause != "" {
		where += clause
		args = append(args, values...)
	}
	// Prefix ranking is the same maintained title/context tier as Search cards.
	// Freeze the same least-common prefix lead inside this retained snapshot.
	if terms, ok := facettext.IndexedSearchTerms(normalized); ok && r.Sort == "relevance" && len(libraries) <= 64 {
		lead, total := "", int64(^uint64(0)>>1)
		for _, term := range terms {
			ids, e := s.prefixTermIDs(ctx, term, int(compactcatalog.Track), libraries)
			if e != nil {
				return QueueSelection{}, e
			}
			raw, _ := json.Marshal(ids)
			var n int64
			if e = s.read().QueryRowContext(ctx, `SELECT COALESCE(sum(total),0) FROM catalog_search_prefix_counts WHERE term_id IN(SELECT value FROM json_each(?)) AND kind=7 AND container=-1 AND library_id IN(SELECT id FROM catalog_libraries WHERE library_id IN(SELECT value FROM json_each(?)))`, string(raw), idsJSON(libraries)).Scan(&n); e != nil {
				return QueueSelection{}, e
			}
			if n < total {
				lead, total = term, n
			}
		}
		if lead != "" {
			tie = "entity_id"
			order = searchOrder{expr: `CASE WHEN d.title_fold=? THEN 3 ELSE COALESCE((SELECT max(p.tier) FROM catalog_search_prefix_rows p INDEXED BY catalog_search_prefix_rows_entity WHERE p.entity_id=d.entity_id AND p.term_id=(SELECT id FROM catalog_search_prefix_terms WHERE prefix=?)),0) END`}
			args = append([]any{facettext.SearchFold(normalized), lead}, args[len(catalogOrder(r.Sort, "song", normalized).args):]...)
		}
	}
	// SELECT's ORDER BY parameters follow FROM/WHERE parameters, so put the
	// ranking projection before FROM to preserve the producer's binding order.
	query := `SELECT public_id FROM (SELECT pid(e.public_id) public_id,e.id entity_id,` + order.expr + ` ord FROM ` + from + ` WHERE ` + where + `) ORDER BY ord ` + r.Direction + `,` + tie + ` ` + r.Direction
	rev, stamp, err := searchRevision(ctx, tx, r, idsJSON(libraries))
	if err != nil {
		return QueueSelection{}, err
	}
	served, err := compactcatalog.SearchServingSQL(ctx, tx, compactcatalog.ServingSQL(query))
	// A lost retained read can resume only under the identical selection authority.
	// Personal/catalogue revisions alone do not change when parental access does.
	selected := append([]string(nil), libraries...)
	sort.Strings(selected)
	binding, _ := json.Marshal([]any{stamp, rev.Viewer, viewer.Profile, viewer.Fence, RestrictionFence(viewer.Restrictions), selected, normalized, r.Sort, r.Direction})
	digest := sha256.Sum256(binding)
	return QueueSelection{SQL: served, Args: args, Revision: "search_" + hex.EncodeToString(digest[:]), Label: normalized}, err
}
