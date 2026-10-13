package tableretention

// Permanent replay, revocation and removal ledgers must not be age-pruned:
// deleting them would let a delayed command or credential become new again.
// The baseline counts new primary keys in constant time and refuses growth at
// these resource ceilings; updates and exact retries still work at the ceiling.
// These limits are safety bounds, not owner history-retention preferences.
var PermanentBounds = map[string]int{
	"saved_operation_fences":          10_000_000,
	"playback_personal_intents":       10_000_000,
	"playback_personal_claims":        10_000_000,
	"hosted_signing_revocations":      100_000,
	"hosted_profile_erasure_receipts": 1_000_000,
	"hosted_profile_revisions":        1_000_000,
	"download_receipt_keys":           10_000,
	"networking_claim_intents":        100_000,
	"networking_claim_cancellations":  100_000,
	"hosted_claim_cleanup_receipts":   100_000,
	"live_source_identities":          100_000,
	"live_source_removals":            100_000,
	"live_preference_receipts":        1_000_000,
	"live_enable_receipts":            1_000_000,
	"lc_receipts":                     1_000_000,
	"lc_channels":                     100_000,
	"dvr_rules":                       1_000_000,
	"dvr_recordings":                  1_000_000,
	"remote_sources":                  100_000,
}
