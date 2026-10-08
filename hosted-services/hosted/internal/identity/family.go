package identity

import (
	"portico.local/hosted/internal/security"
	"time"
)

type FamilySeed struct {
	ID               string
	RefreshHash      []byte
	Mode             string
	RefreshExpiresAt time.Time
}

// SignInWindow is the rolling sign-in lifetime: any refresh inside the window renews it for
// another full window, and nothing else ends a sign-in but revocation. A user server is the
// enforcement point for removed members, so this credential only proves account identity.
const SignInWindow = 90 * 24 * time.Hour

func NewFamily(mode string) (FamilySeed, string) {
	secret := security.RandomID("hrt_")
	return FamilySeed{ID: security.RandomID("fam_"), RefreshHash: security.Hash(secret), Mode: mode, RefreshExpiresAt: time.Now().UTC().Add(SignInWindow)}, secret
}
func ValidMode(mode string) bool { return mode == "native" || mode == "browser" }
