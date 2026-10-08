package identity

import "time"

// Protocol bounds are mirrored by the dedicated SQL constraints. They do not
// enable providers, SMTP or routes; their consumers require separate review.
const (
	IdentityBindingLifetime   = 24 * time.Hour
	IdentityAttemptLifetime   = 10 * time.Minute
	IdentityExchangeLease     = 30 * time.Second
	IdentityReceiptLifetime   = 5 * time.Minute
	IdentityProofLifetime     = 5 * time.Minute
	IdentityMarkerLifetime    = 15 * time.Minute
	IdentityCleanupBatch      = 100
	IdentityChallengeAttempts = 5
	IdentityDeliveryAttempts  = 5
)
