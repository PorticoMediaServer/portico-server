package catalog

import (
	"context"
	"time"

	"portico.local/server/internal/dbwork"
)

// BrowseReadingCarriedTotal validates the complete ordinary Reading cursor path
// before exposing its signed count. It issues no count query, even for legacy
// cursors with absent totals. The bool means a validated total was present;
// otherwise the caller must prepare the exact count and discard the probe page.
// This is an opt-in adapter seam; existing foreground routing is unchanged.
func (s *Service) BrowseReadingCarriedTotal(viewer Viewer, request BrowseRequest) (BrowseResult, bool, error) {
	if request.Cursor == "" {
		return BrowseResult{}, false, nil
	}
	if dbwork.Snapshot(s.Context()) == nil {
		var page BrowseResult
		var present bool
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var e error
			page, present, e = s.WithContext(ctx).BrowseReadingCarriedTotal(viewer, request)
			return e
		})
		return page, present, err
	}
	signed, err := s.readSignedBrowseCursor(request.Cursor)
	if err != nil {
		return BrowseResult{}, false, err
	}
	if signed.Expires <= time.Now().Unix() {
		return BrowseResult{}, false, ErrReadingProjectionChanged
	}
	if signed.Total < 0 {
		return BrowseResult{}, false, ErrCursor
	}
	witness, err := s.browseExactWitness(request.Library, viewer.Profile, viewer.EffectiveRestrictions())
	if err != nil {
		return BrowseResult{}, false, err
	}
	decoded, err := s.decodeCursor(request.Cursor, signed.Scope)
	if err != nil {
		return BrowseResult{}, false, err
	}
	present := decoded.TotalPresent || decoded.Total > 0
	count := ReadingCountResult{Library: request.Library, Profile: viewer.Profile, ViewerFence: viewer.Fence, Witness: witness, QueryFingerprint: queryFingerprint(request.Query), RawTotal: decoded.Total, Filtered: request.Query != nil, Threshold: browseTotalThreshold}
	if browseRelative(request.Query) {
		count.EvaluationAt = signed.EvaluationAt
	}
	// An absent count gets a private zero only to perform the existing signature,
	// scope, fingerprint and ready-anchor checks without invoking any count SQL.
	// Its projected page is discarded below and can never become a response.
	page, err := s.BrowseReadingWithPreparedCount(viewer, request, count)
	if err != nil {
		return BrowseResult{}, false, err
	}
	if !present {
		return BrowseResult{}, false, nil
	}
	return page, true, nil
}
