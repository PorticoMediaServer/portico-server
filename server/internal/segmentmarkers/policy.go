package segmentmarkers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ConfidencePolicy changes confidence floors only. Approval, measurement and
// ambiguity remain independent conditions and cannot be supplied by a number.
type ConfidencePolicy struct{ ViewerPercent, AutomaticPercent int }

func DefaultConfidencePolicy() ConfidencePolicy { return ConfidencePolicy{50, 85} }
func (p ConfidencePolicy) AutomaticSafe(approved bool, confidence float64, provenance string) bool {
	return approved || confidence >= float64(p.AutomaticPercent)/100 && MeasuredProvenance(provenance) && !strings.Contains(provenance, "semantic_ambiguous")
}
func (p ConfidencePolicy) ViewerVisible(approved bool, confidence float64, provenance string) bool {
	return approved || confidence >= float64(p.ViewerPercent)/100 && !strings.Contains(provenance, "semantic_ambiguous")
}
func (p ConfidencePolicy) Valid() bool {
	return (p.ViewerPercent == 30 || p.ViewerPercent == 50 || p.ViewerPercent == 70) && (p.AutomaticPercent == 70 || p.AutomaticPercent == 85 || p.AutomaticPercent == 95)
}

// ReadConfidencePolicy reads once in the caller's existing bounded snapshot.
// An absent value uses the pre-existing policy; a broken schema fails closed.
func ReadConfidencePolicy(ctx context.Context, tx *sql.Tx) (ConfidencePolicy, error) {
	p := DefaultConfidencePolicy()
	var viewer, automatic sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT s."segments.viewerConfidencePercent",s."segments.automaticConfidencePercent" FROM server_settings AS s WHERE s.singleton=1`).Scan(&viewer, &automatic)
	if errors.Is(err, sql.ErrNoRows) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if viewer.Valid {
		p.ViewerPercent = int(viewer.Int64)
	}
	if automatic.Valid {
		p.AutomaticPercent = int(automatic.Int64)
	}
	if !p.Valid() {
		return p, fmt.Errorf("invalid segment confidence policy")
	}
	return p, nil
}
