package livechannels

import (
	"context"
	"database/sql"
)

// ChannelMarksTx enriches a bounded selected set using the same complete
// authorized source collision domain as the guide. It joins the caller's read
// transaction; it neither opens another snapshot nor widens directory authority.
// Unknown, hidden or unauthorized references receive no mark.
func (s *Store) ChannelMarksTx(ctx context.Context, tx *sql.Tx, authorize DirectoryAuthority, channels []Channel) error {
	clearMarks := func() {
		for i := range channels {
			channels[i].Mark = nil
		}
	}
	clearMarks()
	if tx == nil || authorize == nil || len(channels) > 200 {
		return ErrDenied
	}
	if len(channels) == 0 {
		return nil
	}
	scope, err := authorize(ctx, tx)
	if err != nil {
		return err
	}
	if scope.Fence == "" || len(scope.AllowedLiveSources) > MaxSources || len(scope.LibraryRows) > 64 {
		return ErrDenied
	}
	wanted := map[string]bool{}
	for _, c := range channels {
		if c.SourceID != "" && c.ID != "" {
			wanted[c.SourceID] = true
		}
	}
	// Restrict work to requested sources, retaining EVERY authorized channel in
	// each such source. Page/name/search/favorite filters cannot alter a mark.
	selected := scope
	selected.AllowedLiveSources = nil
	selected.LibraryRows = nil
	for _, source := range scope.AllowedLiveSources {
		if wanted[source] {
			selected.AllowedLiveSources = append(selected.AllowedLiveSources, source)
		}
	}
	for _, row := range scope.LibraryRows {
		if wanted[row.Channel.SourceID] {
			selected.LibraryRows = append(selected.LibraryRows, row)
		}
	}
	if err = directoryMarks(ctx, tx, selected, DirectoryQuery{}, channels); err != nil {
		clearMarks()
		return err
	}
	current, err := authorize(ctx, tx)
	if err != nil || current.Fence != scope.Fence {
		clearMarks()
		if err != nil {
			return err
		}
		return ErrDenied
	}
	return nil
}
