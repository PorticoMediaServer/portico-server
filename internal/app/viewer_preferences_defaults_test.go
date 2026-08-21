package app

import "testing"

func TestDefaultProfileDeviceValuesUseExplicitOriginalQuality(t *testing.T) {
	defaults := defaultProfileDeviceValues("mobile")
	for _, network := range []string{"local", "wifi", "cellular", "unknown"} {
		quality, ok := defaults.Playback.Quality[network]
		if !ok {
			t.Fatalf("missing explicit %s quality default", network)
		}
		if quality.Mode != "original" {
			t.Fatalf("%s quality default = %q, want original", network, quality.Mode)
		}
	}
}
