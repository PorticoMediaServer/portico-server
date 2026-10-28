package catalog

import (
	"context"
	"fmt"
	"time"

	"portico.local/server/internal/dbwork"
)

// ReadingCountResult is a typed INTERNAL preparation payload. It is not an API
// response, grant, cursor, or client input. Only completed reducer output may be
// put in the shared memory store; public ready responses remain BrowseResult.
type ReadingCountResult struct {
	Library          string
	Profile          string
	ViewerFence      string
	Witness          string
	QueryFingerprint string
	EvaluationAt     string
	RawTotal         int
	Filtered         bool
	Threshold        int
}

func (r *ReadingCountReduction) Result() (ReadingCountResult, bool) {
	if r == nil || !r.done {
		return ReadingCountResult{}, false
	}
	return ReadingCountResult{Library: r.library, Profile: r.profile, ViewerFence: r.viewerFence, Witness: r.witness, QueryFingerprint: r.queryFingerprint, EvaluationAt: r.evaluationAt, RawTotal: r.count, Filtered: r.filtered, Threshold: r.threshold}, true
}

// BrowseReadingWithPreparedCount composes the original page using a completed,
// typed server-owned memory result. The caller still supplies current authority
// and personal-state admission. This opt-in hook changes no foreground route.
func (s *Service) BrowseReadingWithPreparedCount(viewer Viewer, request BrowseRequest, count ReadingCountResult) (BrowseResult, error) {
	if request.Library != count.Library || viewer.Profile != count.Profile || viewer.Fence != count.ViewerFence || !viewer.AllowsLibrary(request.Library) || count.Witness == "" || count.RawTotal < 0 || count.Threshold != browseTotalThreshold || count.Filtered != (request.Query != nil) || count.QueryFingerprint != queryFingerprint(request.Query) {
		return BrowseResult{}, ErrReadingProjectionChanged
	}
	if browseRelative(request.Query) {
		at, err := time.Parse(time.RFC3339Nano, count.EvaluationAt)
		if err != nil {
			return BrowseResult{}, ErrReadingProjectionChanged
		}
		clone := *s
		clone.browseInstant = at
		s = &clone
	} else if count.EvaluationAt != "" {
		return BrowseResult{}, ErrReadingProjectionChanged
	}
	if dbwork.Snapshot(s.Context()) == nil {
		if dbwork.InWriteTx(s.Context()) || s.readTx != nil {
			return BrowseResult{}, fmt.Errorf("prepared Reading page requires a read snapshot")
		}
		var out BrowseResult
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var e error
			out, e = s.WithContext(ctx).BrowseReadingWithPreparedCount(viewer, request, count)
			return e
		})
		return out, err
	}
	witness, err := s.browseExactWitness(request.Library, viewer.Profile, viewer.EffectiveRestrictions())
	if err != nil {
		return BrowseResult{}, err
	}
	if witness != count.Witness {
		return BrowseResult{}, ErrReadingProjectionChanged
	}
	// Query identity and full witness have been proved in this same snapshot;
	// BrowseEntities still checks its normal scope, cursor, expiry and anchor.
	request.preparedTotal = &count
	return s.BrowseEntities(viewer, request)
}
