package config

import "testing"

func TestDefaultConfigNeverEmbedsAcoustIDAPIKey(t *testing.T) {
	t.Setenv("PORTICO_ACOUSTID_API_KEY", "")
	if got := Load().AcoustIDAPIKey; got != "" {
		t.Fatalf("default AcoustID API key = %q, expected empty", got)
	}
}
