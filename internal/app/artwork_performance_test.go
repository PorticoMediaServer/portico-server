package app

import "testing"

func TestCanonicalArtworkResizeBoundsVariantCardinality(t *testing.T) {
	tests := []struct {
		name                  string
		width, height         int
		wantWidth, wantHeight int
		wantOK                bool
	}{
		{name: "web poster", width: 160, height: 240, wantWidth: 160, wantHeight: 240, wantOK: true},
		{name: "near poster", width: 300, height: 450, wantWidth: 320, wantHeight: 480, wantOK: true},
		{name: "profile", width: 96, height: 96, wantWidth: 96, wantHeight: 96, wantOK: true},
		{name: "oversized square normalizes", width: 1000, height: 1000, wantWidth: 384, wantHeight: 384, wantOK: true},
		{name: "single dimension", width: 300, wantWidth: 320, wantOK: true},
		{name: "noncanonical aspect", width: 500, height: 300, wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			width, height, ok := canonicalArtworkResize(test.width, test.height)
			if ok != test.wantOK || width != test.wantWidth || height != test.wantHeight {
				t.Fatalf("canonicalArtworkResize(%d, %d) = (%d, %d, %v), want (%d, %d, %v)", test.width, test.height, width, height, ok, test.wantWidth, test.wantHeight, test.wantOK)
			}
		})
	}
}
