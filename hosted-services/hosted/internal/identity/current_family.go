package identity

import "time"

// CurrentFamilyExpectation binds a sensitive operation to previously captured
// authority. It is repository input, never a client-controlled authority DTO.
// Generation equality is intentionally stricter than ordinary access validity:
// a refresh requires a new proof or explicit approval for these operations.
type CurrentFamilyExpectation struct {
	TokenHash        []byte
	AccountID        string
	FamilyID         string
	AuthEpoch        int64
	FamilyGeneration int64
	Mode             string
}

// CurrentFamilyFacts is valid only within the transaction that checked it.
// Consumers must check all expiries again using database time immediately before
// their final decision, along with their own proof/operation expiry and revision.
type CurrentFamilyFacts struct {
	AccountID         string
	FamilyID          string
	AuthEpoch         int64
	FamilyGeneration  int64
	Mode              string
	AccessExpiresAt   time.Time
	AbsoluteExpiresAt time.Time
	IdleExpiresAt     time.Time
	CheckedAt         time.Time
}

// ValidCurrentFamilyExpectation rejects malformed inputs before database locks.
func ValidCurrentFamilyExpectation(e CurrentFamilyExpectation) bool {
	return len(e.TokenHash) == 32 && currentFamilyID(e.AccountID) && currentFamilyID(e.FamilyID) &&
		e.AuthEpoch >= 0 && e.FamilyGeneration >= 0 && ValidMode(e.Mode)
}

func currentFamilyID(id string) bool {
	if len(id) == 0 || len(id) > 128 {
		return false
	}
	for i := range len(id) {
		if id[i] < 33 || id[i] > 126 {
			return false
		}
	}
	return true
}

// UnexpiredAt needs fresh database time supplied by the transaction's caller.
// It does not independently authorize a later transaction or domain mutation.
func (f CurrentFamilyFacts) UnexpiredAt(now time.Time) bool {
	return !now.IsZero() && f.AccessExpiresAt.After(now) && f.AbsoluteExpiresAt.After(now) && f.IdleExpiresAt.After(now)
}
