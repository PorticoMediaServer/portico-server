package operations

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ExportIncludePaths is an explicit per-export choice, never a saved default.
type ExportIncludePaths bool

func (ExportIncludePaths) JSONSchema() map[string]any { return map[string]any{"type": "boolean"} }
func (v *ExportIncludePaths) UnmarshalJSON(raw []byte) error {
	switch string(bytes.TrimSpace(raw)) {
	case "true":
		*v = true
	case "false":
		*v = false
	default:
		return ErrInvalid
	}
	return nil
}

type ExportPathCategory string

func (ExportPathCategory) JSONSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"library-configured-root", "library-resolved-root", "managed-mount-point"}}
}

type ExportPathFact struct {
	Category  ExportPathCategory `json:"category"`
	Reference string             `json:"reference"`
	Value     string             `json:"value"`
}
type ExportPathFacts struct {
	Limit   int              `json:"limit"`
	HasMore bool             `json:"hasMore"`
	Omitted int              `json:"omitted"`
	Items   []ExportPathFact `json:"items"`
}

const exportPathLimit = 128

// Only model-classified local source roots and recorded managed mount points.
// No filesystem calls, remote locators, labels, identifiers or free text enter
// the result. The caller invokes this only after authorized receipt lookup.
func readExportPathFacts(ctx context.Context, tx *sql.Tx) (*ExportPathFacts, error) {
	rows, err := tx.QueryContext(ctx, `SELECT category,substr(value,1,4097) FROM (
 SELECT 1 AS rank,id AS ordering,'library-configured-root' AS category,configured_root AS value FROM library_sources WHERE kind='local'
 UNION ALL SELECT 2,id,'library-resolved-root',root FROM library_sources WHERE kind='local'
 UNION ALL SELECT 3,id,'managed-mount-point',mount_path FROM managed_mounts
 ) ORDER BY rank,ordering`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &ExportPathFacts{Limit: exportPathLimit, Items: []ExportPathFact{}}
	for rows.Next() {
		var category ExportPathCategory
		var value string
		if err = rows.Scan(&category, &value); err != nil {
			return nil, err
		}
		if len(value) > 4096 || !utf8.ValidString(value) || !filepath.IsAbs(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
			out.Omitted++
			continue
		}
		if len(out.Items) == exportPathLimit {
			out.HasMore = true
			break
		}
		out.Items = append(out.Items, ExportPathFact{category, "path-" + strconv.Itoa(len(out.Items)+1), value})
	}
	return out, rows.Err()
}
