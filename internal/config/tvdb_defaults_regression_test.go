package config

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestTVDBCredentialUsesPorticoDefaultWithoutOwnerConfiguration(t *testing.T) {
	t.Setenv("PORTICO_TVDB_API_KEY", "")
	if got := Load().TVDBAPIKey; got != DefaultTVDBAPIKey {
		t.Fatalf("TheTVDB API key = %q, want Portico default", got)
	}
}

func TestDistributedTVDBCredentialFingerprintDoesNotDrift(t *testing.T) {
	digest := sha256.Sum256([]byte(DefaultTVDBAPIKey))
	if got, want := hex.EncodeToString(digest[:]), "aedb3ce0e9b8d24795588e653f037d88a254d632084929b9ed848eb24bbf284d"; got != want { // gitleaks:allow -- non-secret fingerprint
		t.Fatalf("TheTVDB API key fingerprint = %s, want %s", got, want)
	}
}

func TestTVDBCredentialCanBeSuppliedByOwnerAtRuntime(t *testing.T) {
	t.Setenv("PORTICO_TVDB_API_KEY", "  owner-tvdb-key  ")
	if got := Load().TVDBAPIKey; got != "owner-tvdb-key" {
		t.Fatalf("TheTVDB API key = %q", got)
	}
}
