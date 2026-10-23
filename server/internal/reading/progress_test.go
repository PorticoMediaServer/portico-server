package reading

import (
	"math"
	"testing"
)

func TestLocationFormatBoundaries(t *testing.T) {
	zero := int64(0)
	negative := int64(-1)
	for _, tc := range []struct {
		location Location
		valid    bool
	}{{Location{Format: "epub", CFI: "epubcfi(/6/2!/4/2:0)", Progression: .2}, true}, {Location{Format: "pdf", PageIndex: &zero, Progression: 0}, true}, {Location{Format: "cbz", PageIndex: &zero, Progression: 1}, true}, {Location{Format: "pdf", CFI: "epubcfi(/6/2)", PageIndex: &zero}, false}, {Location{Format: "epub", CFI: "epubcfi(/6/2)", PageIndex: &zero}, false}, {Location{Format: "cbz", PageIndex: &negative}, false}, {Location{Format: "pdf", PageIndex: &zero, Progression: math.NaN()}, false}, {Location{Format: "pdf", PageIndex: &zero, Progression: math.Inf(1)}, false}, {Location{Format: "epub", CFI: "fake"}, false}, {Location{Format: "epub", CFI: "epubcfi(/6/2)", Section: "../other"}, false}} {
		if err := tc.location.Validate(); (err == nil) != tc.valid {
			t.Fatalf("%+v => %v", tc.location, err)
		}
	}
}
