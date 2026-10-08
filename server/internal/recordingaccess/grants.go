package recordingaccess

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/livechannels"
)


type Grant struct {
	Owner           livechannels.Owner `json:"owner"`
	Enabled         bool               `json:"enabled"`
	InheritProfiles bool               `json:"inheritProfiles"`
	Revision        int64              `json:"revision"`
}

// An explicit profile denial wins over an inherited account grant. Account role,
// including server ownership, never establishes recording permission.
func GrantedTx(ctx context.Context, tx *sql.Tx, o livechannels.Owner) error {
	if !o.Valid() {
		return livechannels.ErrDenied
	}
	var enabled bool
	err := tx.QueryRowContext(ctx, `SELECT enabled FROM dvr_recording_grants WHERE authority=? AND account_id=? AND profile_id=?`, o.Authority, o.AccountID, o.ProfileID).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM dvr_recording_grants WHERE authority=? AND account_id=? AND enabled=1 AND inherit_profiles=1)`, o.Authority, o.AccountID).Scan(&enabled)
	}
	if err != nil {
		return err
	}
	if !enabled {
		return livechannels.ErrDenied
	}
	return nil
}
func ReadGrantsTx(ctx context.Context, tx *sql.Tx) ([]Grant, error) {
	out := []Grant{}
	rows, err := tx.QueryContext(ctx, `SELECT authority,account_id,profile_id,enabled,inherit_profiles,revision FROM dvr_recording_grants ORDER BY authority,account_id,profile_id`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var g Grant
		if err = rows.Scan(&g.Owner.Authority, &g.Owner.AccountID, &g.Owner.ProfileID, &g.Enabled, &g.InheritProfiles, &g.Revision); err != nil {
			return out, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}
func SaveGrantTx(ctx context.Context, tx *sql.Tx, g Grant) (Grant, error) {
	if !g.Owner.Valid() || g.Revision < 0 {
		return g, livechannels.ErrInvalid
	}
	if g.Revision == 0 {
		_, err := tx.ExecContext(ctx, `INSERT INTO dvr_recording_grants VALUES(?,?,?,?,?,1)`, g.Owner.Authority, g.Owner.AccountID, g.Owner.ProfileID, g.Enabled, g.InheritProfiles)
		if err != nil {
			return g, livechannels.ErrConflict
		}
	} else {
		r, err := tx.ExecContext(ctx, `UPDATE dvr_recording_grants SET enabled=?,inherit_profiles=?,revision=revision+1 WHERE authority=? AND account_id=? AND profile_id=? AND revision=?`, g.Enabled, g.InheritProfiles, g.Owner.Authority, g.Owner.AccountID, g.Owner.ProfileID, g.Revision)
		if err != nil {
			return g, err
		}
		n, _ := r.RowsAffected()
		if n != 1 {
			return g, livechannels.ErrConflict
		}
	}
	g.Revision++
	return g, nil
}
