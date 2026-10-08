// Package buildinfo exposes release metadata without reading application state.
package buildinfo

// Values are embedded by the controlled release builder. Development builds
// deliberately make no claim to a verified source identity.
var Version = "0.1.0-dev"
var BuildID = "development"
var SourceDigest = "unsealed"
var BuiltAt = ""
var Proof = "unsealed"

// Release is the public build identity served at GET /v1/system. Fields are in
// JSON key order, matching Info's encoding byte for byte.
type Release struct {
	BuildID      string `json:"buildId"`
	BuiltAt      string `json:"builtAt"`
	Proof        string `json:"proof"`
	SourceDigest string `json:"sourceDigest"`
	Version      string `json:"version"`
}

// Current is this binary's build identity.
func Current() Release {
	return Release{BuildID: BuildID, BuiltAt: BuiltAt, Proof: Proof, SourceDigest: SourceDigest, Version: Version}
}

func Info() map[string]string {
	return map[string]string{"version": Version, "buildId": BuildID, "sourceDigest": SourceDigest, "builtAt": BuiltAt, "proof": Proof}
}
