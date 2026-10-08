package catalog

import (
	"strings"
)

// Saved views on Home. A viewer adds one by placing "view:<id>" in the Home
// layout's order (Customize Home › Add a saved view); the row is the view's
// browse (its library, pivot, filter and sort) under the view's name, its
// first page on Home, the whole list on the view's own page. A view the
// viewer can no longer read, or that no longer validates, is left out; its id
// stays harmlessly in the layout until the layout is next saved.

const (
	homeViewPrefix = "view:"
	homeViewRow    = 60 // a view row's entries on Home; See all opens the view
)

// homeViewSpecs are the Home rows for the saved views the layout names.
func (s *Service) homeViewSpecs(r HomeRequest) []homeRowSpec {
	var out []homeRowSpec
	actor, ok := homeViewActor(r.Profile)
	if !ok {
		return nil
	}
	for i, id := range r.RowOrder {
		if !strings.HasPrefix(id, homeViewPrefix) {
			continue
		}
		view, err := s.SavedResource(r.ServerID, r.ViewerFence, strings.TrimPrefix(id, homeViewPrefix), actor)
		if err != nil || view.Kind != "view" || view.Definition == nil || view.Status != "ready" || !containsString(r.Libraries, view.Definition.LibraryID) {
			continue
		}
		lib, err := s.library(view.Definition.LibraryID)
		if err != nil {
			continue
		}
		out = append(out, homeRowSpec{ID: id, Title: view.Name, Kind: "saved_view", ArtworkShape: homeArtwork(lib.Kind), LibraryID: view.Definition.LibraryID,
			Priority: 90 + i, CacheTTL: 60, Hideable: true, Reorderable: true, Cursors: true, Privacy: "personal", PolicyState: "available", Direction: "asc"})
	}
	return out
}

// homeViewSource runs the view's browse and lists its first entries in order.
func (s *Service) homeViewSource(r HomeRequest, spec homeRowSpec) (homeSource, error) {
	actor, ok := homeViewActor(r.Profile)
	if !ok {
		return homeSource{}, ErrHomeRowUnknown
	}
	view, err := s.SavedResource(r.ServerID, r.ViewerFence, strings.TrimPrefix(spec.ID, homeViewPrefix), actor)
	if err != nil || view.Definition == nil {
		return homeSource{}, ErrHomeRowUnknown
	}
	d := view.Definition
	page, err := s.BrowseEntities(r.Viewer, BrowseRequest{ServerID: r.ServerID, Library: d.LibraryID, Profile: r.Profile, ViewerFence: r.ViewerFence, Pivot: d.Pivot, Query: d.Query, Sort: d.Sort, Limit: homeViewRow, Restrictions: r.Restrictions})
	if err != nil {
		return homeSource{}, err
	}
	candidates := make([]recCandidate, 0, len(page.Entries))
	for _, e := range page.Entries {
		candidates = append(candidates, recCandidate{ID: e.ID})
	}
	return candidateSource(candidates), nil
}

func homeViewActor(profile string) (ResourceActor, bool) {
	v, ok := viewerFromPersonalKey(profile)
	if !ok {
		return ResourceActor{}, false
	}
	return ResourceActor{Authority: v.Authority, AccountID: v.AccountID, ProfileID: v.ProfileID}, true
}
