package httpapi

import (
	"context"
	"errors"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/livechannels"
)

// This composes the actual 06 guide facts and source authority; a configured
// store is never treated as permission. Search's final scope check also rejects
// a feature revocation while these bounded reads were in flight.
func (d Dependencies) searchLiveLookup(p identity.Principal, fence string, restrictions identity.ContentRestrictions, q, direction string) func(context.Context, string, string, string, int) (catalog.SearchGroupResult, error) {
	return func(ctx context.Context, group, cursor, sort string, limit int) (catalog.SearchGroupResult, error) {
		out := catalog.SearchGroupResult{Items: []catalog.ContentEntry{}}
		if err := d.featureAllowed(ctx, p, "live_tv"); err != nil {
			return out, catalog.ErrCursor
		}
		authority := d.guideDirectoryAuthority(p, fence)
		if group == "programmes" {
			var allowed func(string) bool
			if restrictions.Active() {
				allowed = func(rating string) bool { return catalog.RatingAllowed(restrictions, rating) }
			}
			page, err := d.LiveChannels.SearchProgrammes(ctx, authority, d.airingProjection(p), q, limit, cursor, allowed)
			if errors.Is(err, livechannels.ErrCursor) {
				return out, catalog.ErrStaleContinuation
			}
			if err != nil {
				return out, err
			}
			for _, v := range page.Items {
				a := v.Airing
				programme := a.Programme
				ch := searchChannel(a.Channel)
				id := ch.Provenance + ":" + ch.SourceID + ":" + programme.ID
				out.Items = append(out.Items, catalog.ContentEntry{ID: id, Kind: "programme", Title: programme.Title, Channel: ch, Programme: &catalog.SearchProgramme{ID: programme.ID, Title: programme.Title, Subtitle: programme.Subtitle, Start: programme.Start, End: programme.End, Live: programme.Flags.Live, MoreAirings: v.MoreAirings, MoreAiringsExact: v.MoreAiringsExact}, PosterURL: programme.Image, Navigation: &catalog.ContentNavigation{View: "programme", EntityID: id}})
			}
			out.NextCursor = page.NextCursor
			out.SourceRevision, out.WindowStart, out.WindowEnd = page.Revision, page.Start, page.End
			out.TotalCount = len(out.Items)
			out.TotalCountExact = cursor == "" && page.NextCursor == ""
			return out, nil
		}
		page, err := d.LiveChannels.SearchChannels(ctx, authority, q, sort, direction, limit, cursor)
		if errors.Is(err, livechannels.ErrCursor) {
			return out, catalog.ErrStaleContinuation
		}
		if err != nil {
			return out, err
		}
		if err = d.annotateViewerDirectory(ctx, p, page.Items); err != nil {
			return out, err
		}
		for _, ch := range page.Items {
			out.Items = append(out.Items, catalog.ContentEntry{ID: ch.ID, Kind: "channel", Title: ch.Name, Channel: searchChannel(ch), PosterURL: ch.LogoPath, Navigation: &catalog.ContentNavigation{View: "channel", EntityID: ch.ID}})
		}
		out.TotalCount, out.TotalCountExact, out.NextCursor = page.Total, true, page.NextCursor
		out.SourceRevision = page.Revision
		return out, nil
	}
}
func searchChannel(ch livechannels.Channel) *catalog.SearchChannel {
	return &catalog.SearchChannel{ID: ch.ID, SourceID: ch.SourceID, Generation: ch.Generation, Name: ch.Name, Number: ch.Number, Group: ch.Group, Provenance: string(ch.Provenance)}
}

func (d Dependencies) searchOnNow(ctx context.Context, p identity.Principal, fence string, restrictions identity.ContentRestrictions) ([]catalog.ContentEntry, error) {
	out := []catalog.ContentEntry{}
	if d.LiveChannels == nil {
		return out, nil
	}
	if err := d.featureAllowed(ctx, p, "live_tv"); err != nil {
		return out, err
	}
	read := func(ctx context.Context, q livechannels.GuideQuery) (livechannels.Guide, error) {
		q.Viewer = channelSubject(p)
		if restrictions.Active() {
			q.ProgrammeAllowed = func(rating string) bool { return catalog.RatingAllowed(restrictions, rating) }
		}
		if q.Kind == livechannels.LibraryChannel {
			if d.LibraryChannels == nil {
				return livechannels.Guide{}, livechannels.ErrUnavailable
			}
			return d.LibraryChannels.Guide(ctx, d.libraryAuthority(p), q, d.LiveChannels.ContinuationKey("library-guide-v1"))
		}
		return d.LiveChannels.Guide(ctx, d.liveAuthority(p), q)
	}
	page, err := d.LiveChannels.NowNextPage(ctx, d.guideDirectoryAuthority(p, fence), read, livechannels.GuidePageQuery{Directory: livechannels.DirectoryQuery{Kind: "all", Limit: 4}, OnNow: true})
	if err != nil {
		return out, err
	}
	if err = d.annotateViewerDirectory(ctx, p, page.Channels); err != nil {
		return out, err
	}
	for _, ch := range page.Channels {
		entry := catalog.ContentEntry{ID: ch.ID, Kind: "channel", Title: ch.Name, Channel: searchChannel(ch), PosterURL: ch.LogoPath, Navigation: &catalog.ContentNavigation{View: "channel", EntityID: ch.ID}}
		for _, v := range ch.Programmes {
			if v.Title != livechannels.RestrictedProgrammeTitle {
				entry.Programme = &catalog.SearchProgramme{ID: v.ID, Title: v.Title, Subtitle: v.Subtitle, Start: v.Start, End: v.End, Live: v.Flags.Live, MoreAiringsExact: false}
				break
			}
		}
		out = append(out, entry)
	}
	return out, nil
}
