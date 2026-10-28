package metadata

import (
	"fmt"
	"portico.local/server/internal/metadataprovider"
)

// The original indivisible candidate/timing tests are retained on
// phase2/P2-52b-candidate at6380440fe. Production acceptance now exercises
// staged construction and the complete final writer in mb_staging_test.go.
func mbAccountingLargeRelease(side int) metadataprovider.ReleaseLookup {
	id := fmt.Sprintf("%08x-0000-0000-0000-000000000000", side)
	r := metadataprovider.ReleaseLookup{RequestedID: id, Release: metadataprovider.Release{
		ID: id, ReleaseGroup: metadataprovider.ReleaseGroup{ID: mbLaneGroup},
		Media: []metadataprovider.Medium{{Position: 1}},
	}}
	for n := 0; n < mbEvidenceTracks; n++ {
		r.Release.Media[0].Tracks = append(r.Release.Media[0].Tracks, metadataprovider.ReleaseTrack{
			ID: fmt.Sprintf("%08x-0000-0000-0000-%012x", side+100, n), Position: n + 1,
			Recording: metadataprovider.Recording{ID: fmt.Sprintf("%08x-0000-0000-0000-%012x", side+200, n)},
		})
	}
	return r
}
