package httpapi

import "portico.local/server/internal/identity"

// APILevel is this server's API generation. A client opens a server only when
// it speaks the same level: the web app at web.getportico.tv talks to servers
// of every version at once, and before release the rule is an exact match.
// Bump it by hand, in the same commit, with any route or document change an
// older client cannot read; packages/contracts/src/api-level.ts carries the
// same number (TestAPILevelMatchesContracts).
const APILevel = 1

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
}
