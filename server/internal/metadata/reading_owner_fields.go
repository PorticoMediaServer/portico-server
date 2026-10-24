package metadata

import (
	"context"
	"database/sql"
	"math"
	"strconv"
	"strings"
)

func readingRepairKind(kind string) bool { return kind == "ebook" || kind == "comic" }
func readingAttributionField(field string) bool {
	return field == "author" || field == "series" || field == "seriesNumber"
}

func readingAttributionSpec(field, label string) RepairFieldSpec {
	spec := textSpec(field, label, "general", true)
	if field != "seriesNumber" {
		spec.MaxLength = 4096
	}
	return spec
}

// Reading attribution is scalar owner data. It never creates a person, an
// audiobook Author/Book, or a provider/archive/artwork permission.
func readReadingOwnerFields(ctx context.Context, tx *sql.Tx, entity int64) (map[string]string, error) {
	out := map[string]string{}
	rows, err := tx.QueryContext(ctx, `SELECT field,value,automatic_value,locked FROM metadata_owner_fields WHERE kind='item' AND entity_id=? AND field IN('author','series','seriesNumber')`, entity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var field, value, automatic string
		var locked bool
		if err = rows.Scan(&field, &value, &automatic, &locked); err != nil {
			return nil, err
		}
		if !locked || strings.TrimSpace(value) == "" {
			value = automatic
		}
		out[field] = value
	}
	return out, rows.Err()
}

func readingAttributionEdit(field string, current RepairField, edit RepairFieldEdit) (RepairFieldEdit, error) {
	if edit.Values != nil {
		return edit, ErrRepairInput
	}
	clear := edit.Automatic || edit.Locked != nil && !*edit.Locked || edit.Value != nil && strings.TrimSpace(*edit.Value) == ""
	if clear {
		value, locked := current.Automatic, false
		return RepairFieldEdit{Value: &value, Locked: &locked}, nil
	}
	if field == "seriesNumber" && edit.Value != nil {
		value := strings.TrimSpace(*edit.Value)
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || strings.ContainsAny(value, "xXpP") {
			return edit, ErrRepairInput
		}
		edit.Value = &value
	}
	return edit, nil
}

func readingAttributionChanged(ctx context.Context, tx *sql.Tx, entity int64) error {
	result, err := tx.ExecContext(ctx, `UPDATE library_revisions SET revision=revision+1 WHERE library_id=(SELECT cl.library_id FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.id=?)`, entity)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return sql.ErrNoRows
	}
	return err
}
