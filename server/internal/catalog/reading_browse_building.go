package catalog

import (
	"context"
	"portico.local/server/internal/dbwork"
)

// ReadingBrowseBuilding answers only a cold image. A published image follows
// the ordinary bounded count preparation; an absent image is an honest empty
// lower bound, without allocating a doomed count job or returning 412.
func (s *Service) ReadingBrowseBuilding(viewer Viewer, request BrowseRequest) (BrowseResult, bool, error) {
	if dbwork.Snapshot(s.Context()) == nil {
		var page BrowseResult
		var handled bool
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var e error
			page, handled, e = s.WithContext(ctx).ReadingBrowseBuilding(viewer, request)
			return e
		})
		return page, handled, err
	}
	if !viewer.AllowsLibrary(request.Library) {
		return BrowseResult{}, false, nil
	}
	scope, err := s.readingBrowseScope(request.Library)
	if err != nil {
		return BrowseResult{}, false, err
	}
	if scope == nil || scope.generation.Ready > 0 {
		return BrowseResult{}, false, nil
	}
	page, err := s.BrowseEntities(viewer, request)
	return page, true, err
}
