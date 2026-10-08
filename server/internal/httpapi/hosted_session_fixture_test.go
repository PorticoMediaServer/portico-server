package httpapi

import (
	"context"
	"database/sql"
	"portico.local/server/internal/identity"
	"testing"
	"time"
)

// The calling fixture first applies a signed Hosted policy. Issue the native
// family in a transaction with that policy's exact cached authorization horizon.
func issueHostedFixture(t *testing.T, db *sql.DB, ident *identity.Service, account, profile, role string) (identity.Envelope, error) {
	t.Helper()
	var expiry string
	if err := db.QueryRow(`SELECT expires_at FROM policy WHERE server_id=?`, ident.ID()).Scan(&expiry); err != nil {
		return identity.Envelope{}, err
	}
	horizon, err := time.Parse(time.RFC3339, expiry)
	if err != nil {
		return identity.Envelope{}, err
	}
	ctx := context.Background()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return identity.Envelope{}, err
	}
	defer tx.Rollback()
	out, err := ident.IssueTx(ctx, tx, account, profile, "hosted", role, 1, horizon)
	if err != nil {
		return identity.Envelope{}, err
	}
	if err = tx.Commit(); err != nil {
		return identity.Envelope{}, err
	}
	if _, err = ident.Authenticate(out.AccessToken); err != nil {
		t.Fatalf("hosted fixture issued a non-working device session: %v", err)
	}
	return out, nil
}
