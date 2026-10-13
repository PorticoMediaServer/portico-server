package catalog

import (
	"context"
	"portico.local/server/internal/catalogtest"
	"testing"
)

func TestCompleteScanAvoidsUnconsumedPassesButCustomKeepsThem(t *testing.T) {
	c := catalogtest.Open(t)
	c.Library("film", "Films", "movie", "/films")
	// A stored Complete profile is read through its named profile semantics.
	c.Exec(`UPDATE library_scan_policies SET tier='complete',operations_json='["probe","checksum","fingerprint"]' WHERE library_id='film'`)
	p, err := New(c.DB).ScanPolicy(context.Background(), "film")
	if err != nil {
		t.Fatal(err)
	}
	if p.Allows("checksum") || p.Allows("fingerprint") || !p.Allows("trickplay") || !p.Allows("loudness") {
		t.Fatal(p)
	}
	custom, err := ValidateScanPolicy(ScanPolicy{Tier: "custom", Operations: []string{"probe", "checksum", "fingerprint"}})
	if err != nil || !custom.Allows("checksum") || !custom.Allows("fingerprint") {
		t.Fatal(custom, err)
	}
}
