package postgres

import (
	"portico.local/hosted/internal/serverreturn"
	"time"
)

// recorded is the database's minimum original intent/confirmation expiry.
// Neither issuance nor accepting action may outlive the sealed server authority.
func securityReturnDeadline(original serverreturn.SecurityContext, recorded time.Time) time.Time {
	if original.ExpiresAt.IsZero() || original.AuthorityExpiresAt.IsZero() || recorded.IsZero() {
		return time.Time{}
	}
	deadline := recorded
	if original.ExpiresAt.Before(deadline) {
		deadline = original.ExpiresAt
	}
	if original.AuthorityExpiresAt.Before(deadline) {
		deadline = original.AuthorityExpiresAt
	}
	return deadline
}
