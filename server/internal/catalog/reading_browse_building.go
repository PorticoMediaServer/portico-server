package catalog

import (
	"context"
	"portico.local/server/internal/dbwork"
	"strings"
)

const readingImportingCursorPrefix = "browse_reading_import_"

func (s *Service) readingImportingPage(scope *readingBrowseScope, request BrowseRequest) (bool, error) {
	if request.Query != nil || request.Range != nil || request.Seek != nil || request.SelectionBasis != "" || request.HomeRowDisplay != "" {
		return false, nil
	}
	if request.Cursor == "" {
		return scope.importing, nil
	}
	signed, err := s.readSignedBrowseCursor(request.Cursor)
	if err != nil {
		return false, err
	}
	return strings.HasPrefix(signed.Fingerprint, readingImportingCursorPrefix), nil
}

// ReadingBrowseBuilding serves a cold image or an importing keyset stream
// without an exact-count preparation. It reuses the signed cursor envelope:
// the opaque fingerprint identifies the stream, and TotalPresent remains false.
// All other queries and conditional ranges retain exact preparation.
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
	if scope == nil {
		return BrowseResult{}, false, nil
	}
	if scope.generation.Ready > 0 {
		lower, e := s.readingImportingPage(scope, request)
		if e != nil {
			return BrowseResult{}, false, e
		}
		if !lower {
			return BrowseResult{}, false, nil
		}
	}
	page, err := s.BrowseEntities(viewer, request)
	return page, true, err
}
