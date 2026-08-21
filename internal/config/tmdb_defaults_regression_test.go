package config

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestTMDBCredentialsUsePorticoDefaultsWithoutOwnerConfiguration(t *testing.T) {
	t.Setenv("PORTICO_TMDB_READ_ACCESS_TOKEN", "")
	t.Setenv("PORTICO_TMDB_API_KEY", "")

	cfg := Load()
	if cfg.TMDBReadAccessToken != DefaultTMDBReadAccessToken || cfg.TMDBAPIKey != DefaultTMDBAPIKey {
		t.Fatal("Portico TMDB defaults are not active")
	}
}

func TestDistributedTMDBCredentialFingerprintsDoNotDrift(t *testing.T) {
	for name, value := range map[string]struct {
		credential string
		sha256     string
	}{
		"read access token": {DefaultTMDBReadAccessToken, "fbf57ff9fe80973fcaee2261ab6dcd9657c4ae1d8ef13a25c3f310a3071f9940"}, // gitleaks:allow -- non-secret fingerprint
		"API key":           {DefaultTMDBAPIKey, "ff477f1608adb52a5852d1ffaf2a9d04f0a3940f67197610a812e197a5c3abf1"},          // gitleaks:allow -- non-secret fingerprint
	} {
		digest := sha256.Sum256([]byte(value.credential))
		if got := hex.EncodeToString(digest[:]); got != value.sha256 {
			t.Fatalf("%s fingerprint = %s, want %s", name, got, value.sha256)
		}
	}
}

func TestTMDBCredentialsCanBeSuppliedByOwnerAtRuntime(t *testing.T) {
	t.Setenv("PORTICO_TMDB_READ_ACCESS_TOKEN", "  owner-read-token  ")
	t.Setenv("PORTICO_TMDB_API_KEY", "  owner-api-key  ")

	cfg := Load()
	if cfg.TMDBReadAccessToken != "owner-read-token" {
		t.Fatalf("TMDB read access token = %q", cfg.TMDBReadAccessToken)
	}
	if cfg.TMDBAPIKey != "owner-api-key" {
		t.Fatalf("TMDB API key = %q", cfg.TMDBAPIKey)
	}
}

func TestExplicitTMDBAPIKeyDoesNotEnableBearerAuth(t *testing.T) {
	t.Setenv("PORTICO_TMDB_READ_ACCESS_TOKEN", "")
	t.Setenv("PORTICO_TMDB_API_KEY", "owner-api-key")

	cfg := Load()
	if cfg.TMDBReadAccessToken != "" {
		t.Fatal("TMDB read access token must remain unset for API-key-only configuration")
	}
	if cfg.TMDBAPIKey != "owner-api-key" {
		t.Fatalf("TMDB API key = %q", cfg.TMDBAPIKey)
	}
}
