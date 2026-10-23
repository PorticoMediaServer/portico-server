package httpapi

import "portico.local/server/internal/identity"

// SystemInfo is public and contains only the facts needed before sign-in.
type SystemInfo struct {
	ID               string              `json:"id"`
	Name             string              `json:"name"`
	SignIn           identity.SignInKind `json:"signIn"`
	SetupRequired    bool                `json:"setupRequired"`
	HostedConfigured bool                `json:"hostedConfigured"`
	HostedAttached   bool                `json:"hostedAttached"`
	Version          string              `json:"version"`
}
