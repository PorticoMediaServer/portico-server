package httpapi

import (
	"strconv"

	"portico.local/server/internal/identity"
)

// APILevel is this server's newest API major. Clients already in the field
// read it; newer clients negotiate from APIMajors and Capabilities instead
// (O-159). Bump it by hand only for a breaking change, in the same commit, and
// keep serving the old major; packages/contracts/src/api-level.ts carries the
// same numbers (TestAPILevelMatchesContracts).
const APILevel = 1

// APIMinor rises with each additive release of the newest major. It is for
// support and diagnostics only: clients gate features on Capabilities.
const APIMinor = 0

// APIMajors lists every major this server still serves, oldest first. Its
// first entry is the oldest client major the server accepts.
var APIMajors = []int{1}

// Capabilities names what this server can do beyond the 1.0 floor: one stable
// ID per feature added after 1.0 that a client may gate on. IDs are never
// renamed or reused. At 1.0 the set is empty: everything shipped by 1.0 is
// the floor every client may assume.
var Capabilities = []string{}

// SystemInfo is public and contains only the facts needed before sign-in.
type SystemInfo struct {
	ID     string               `json:"id"`
	Name   string               `json:"name"`
	SignIn identity.SignInKind  `json:"signIn"`
	Move   *identity.PublicMove `json:"move"`
	// MoveEndsAt is when the old sign-in offered during a move to Portico Accounts stops
	// working (RFC 3339, UTC); null when there is no such offer.
	MoveEndsAt       *string `json:"moveEndsAt"`
	SetupRequired    bool    `json:"setupRequired"`
	HostedConfigured bool    `json:"hostedConfigured"`
	HostedAttached   bool    `json:"hostedAttached"`
	Version          string  `json:"version"`
	APILevel         int     `json:"apiLevel"`
	// APIVersion is "major.minor" of the newest major (O-159).
	APIVersion string `json:"apiVersion"`
	// APIMajors lists the API majors this server serves, oldest first.
	APIMajors []int `json:"apiMajors"`
	// Capabilities lists features added after 1.0, sorted; empty at 1.0.
	Capabilities []string `json:"capabilities"`
}

// systemAPI returns the negotiation facts every GET /v1/system carries.
func systemAPI(info SystemInfo) SystemInfo {
	info.APILevel = APILevel
	info.APIVersion = strconv.Itoa(APILevel) + "." + strconv.Itoa(APIMinor)
	info.APIMajors = append([]int(nil), APIMajors...)
	info.Capabilities = append([]string{}, Capabilities...)
	return info
}
