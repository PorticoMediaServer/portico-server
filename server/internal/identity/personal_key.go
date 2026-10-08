package identity

import "portico.local/server/internal/persistence"

// PersonalKey is the lossless authority-qualified owner tuple inside this one
// server's database. Keep Principal.ProfileID unchanged for authorization.
func PersonalKey(v Viewer) string {
	return persistence.PersonalOwnerKey(v.Authority, v.AccountID, v.ProfileID)
}
