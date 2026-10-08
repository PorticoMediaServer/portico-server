package downloads

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/identity"
)

// ProfileAllowsDownloads reads the current profile switch in the same
// transaction as preparation, grant and receipt authority. A revoked hosted
// profile also loses offline authorization on the next server contact.
func ProfileAllowsDownloads(tx *sql.Tx, viewer identity.Viewer) (bool, error) {
	if viewer.Authority == "hosted" {
		var revoked int
		e := tx.QueryRow(`SELECT revoked FROM restrictions WHERE profile_id=?`, viewer.ProfileID).Scan(&revoked)
		if e != nil && !errors.Is(e, sql.ErrNoRows) {
			return false, e
		}
		if e == nil && revoked != 0 {
			return false, nil
		}
	}
	restrictions, err := identity.RestrictionsForViewerTx(context.Background(), tx, viewer)
	if err != nil {
		return false, err
	}
	return restrictions.AllowDownloads, nil
}

// AccountDisabled reports an account the server has switched off. A disabled
// account keeps no offline authorization: its receipts stop revalidating on the
// next contact, which is the only moment a server gets to say so.
func AccountDisabled(tx *sql.Tx, viewer identity.Viewer) (bool, error) {
	if viewer.Authority != "hosted" {
		return false, nil
	}
	var revoked int
	e := tx.QueryRow(`SELECT revoked FROM restrictions WHERE profile_id=?`, viewer.ProfileID).Scan(&revoked)
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	return revoked != 0, nil
}
