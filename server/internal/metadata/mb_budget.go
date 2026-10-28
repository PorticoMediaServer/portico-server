package metadata

import (
	"context"
	"database/sql"
	"errors"
)

var errMBBudget = errors.New("MusicBrainz evidence retention budget exhausted")

// Admission charges payload bytes conservatively plus every normalized row.
// Accepted catalog evidence is protected authority, outside this cache budget.
// Base accounting triggers maintain the singleton in the source transaction;
// the original aggregate remains the independent test oracle.
// Aliases and live stages remain in the budget and cannot be evicted to admit
// another request. This check runs after construction in the same transaction;
// refusal rolls back the entire prospective revision, including its children.
func checkMBEvidenceBudget(ctx context.Context, tx *sql.Tx) error {
	var releases, recordings, charge int64
	e := tx.QueryRowContext(ctx, `SELECT releases,recordings,charge FROM mb_evidence_totals WHERE singleton=1`).Scan(&releases, &recordings, &charge)
	if e != nil {
		return e
	}
	if releases > 512 || recordings > 20000 || charge > 256<<20 {
		return errMBBudget
	}
	return nil
}
