package networking

import (
	"encoding/json"
	"strconv"

	"portico.local/server/internal/wirejson"
)

// ClaimSetupStartRequest binds protected browser state to the exact pending
// setup, not to an owner session. The setup-token HTTP edge must compare its
// request/revision with the actual SetupClaimAuthority before any mutation.
type ClaimSetupStartRequest struct {
	SetupRequestID string `json:"setupRequestId"`
	SetupRevision  int64  `json:"setupRevision,string"`
	State          string `json:"state"`
	CodeChallenge  string `json:"codeChallenge"`
	InstallationID string `json:"installationId"`
}

func (q *ClaimSetupStartRequest) UnmarshalJSON(raw []byte) error {
	*q = ClaimSetupStartRequest{}
	fields, err := wirejson.Object(raw, "setupRequestId", "setupRevision", "state", "codeChallenge", "installationId")
	if err != nil || len(fields) != 5 {
		return ErrInvalid
	}
	type plain ClaimSetupStartRequest
	var next plain
	if json.Unmarshal(raw, &next) != nil || !validID(next.SetupRequestID) || next.SetupRevision <= 0 || !ownerRandom32(next.State) || !ownerRandom32(next.CodeChallenge) || len(next.InstallationID) < 32 || !validID(next.InstallationID) {
		return ErrInvalid
	}
	var revision string
	if json.Unmarshal(fields["setupRevision"], &revision) != nil || strconv.FormatInt(next.SetupRevision, 10) != revision {
		return ErrInvalid
	}
	*q = ClaimSetupStartRequest(next)
	return nil
}

func (ClaimSetupStartRequest) String() string   { return "[setup start request redacted]" }
func (ClaimSetupStartRequest) GoString() string { return "[setup start request redacted]" }
