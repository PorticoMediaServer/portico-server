package compactcatalog

import (
	"context"
	"database/sql"
	"regexp"
	"strings"
)

// ServingTables retain an old relational generation lazily: the first change
// saves the old row (or its absence), without copying a whole library at start.
var ServingTables = []string{
	"catalog_browse_rows", "catalog_browse_blocks", "catalog_browse_buckets",
	"catalog_browse_counted_rows", "catalog_browse_memberships", "catalog_browse_member_metrics",
	"catalog_home_buckets", "catalog_item_availability", "catalog_facet_counts", "catalog_related_facets",
}

var servingNames = func() *strings.Replacer {
	pairs := []string{}
	for _, name := range ServingTables {
		pairs = append(pairs, name, name+"_serving")
	}
	return strings.NewReplacer(pairs...)
}()
var servingIndex = regexp.MustCompile(`\s+INDEXED BY (?:catalog_browse_[a-z_]+|catalog_home_[a-z_]+|catalog_related_[a-z_]+|catalog_facet_[a-z_]+)`)

// ServingSQL is used only by catalogue readers. Workers keep writing the new
// generation; SQL views select the serving generation in the reader's snapshot.
func ServingSQL(query string) string {
	return servingNames.Replace(servingIndex.ReplaceAllString(query, ""))
}

func searchTables(query string, slot int) string {
	if slot == 0 {
		return query
	}
	return strings.NewReplacer("catalog_search_documents", "catalog_search_documents_next", "catalog_search_titles", "catalog_search_titles_next").Replace(query)
}

// SearchServingSQL resolves the active FTS generation in the same snapshot as
// the search. Both generations have independent documents and inverted indexes.
func SearchServingSQL(ctx context.Context, q ReadQuery, query string) (string, error) {
	if !strings.Contains(query, "catalog_search_titles") && !strings.Contains(query, "catalog_search_documents") {
		return query, nil
	}
	var slot int
	err := q.QueryRowContext(ctx, `SELECT COALESCE((SELECT active_slot FROM catalog_derivations WHERE domain=17),0)`).Scan(&slot)
	return searchTables(query, slot), err
}

func beginGeneration(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `UPDATE catalog_generations SET generation=generation+1,retaining=1 WHERE singleton=1 AND retaining=0 AND EXISTS(SELECT 1 FROM catalog_browse_rows)`)
	return err
}

func finishGeneration(ctx context.Context, tx *sql.Tx) error {
	_, err := tx.ExecContext(ctx, `UPDATE catalog_generations SET retaining=0 WHERE singleton=1 AND retaining=1 AND NOT EXISTS(SELECT 1 FROM catalog_derivations WHERE rebuilding=1) AND NOT EXISTS(SELECT 1 FROM catalog_dirty d WHERE domain IN(18,19,20,25,26,28) AND NOT EXISTS(SELECT 1 FROM catalog_derivation_failures f WHERE f.domain=d.domain AND f.entity_id=d.entity_id))`)
	return err
}
