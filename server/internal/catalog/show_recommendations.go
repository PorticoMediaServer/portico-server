package catalog

import (
	"database/sql"
)

// ShowRecommendationRows publishes the library-local "More like this" row for
// a show page (Spec — Page Content §2): the same bounded engine as movies
// (genres, people, keywords), seeded by the show's first visible episode. An
// episode's entity is its show, so any visible episode seeds the whole show's
// facet set, and the seed's own work is excluded from the results.
func (s *Service) ShowRecommendationRows(viewer Viewer, show string, limit int) ([]HomeRow, ContentRevision, error) {
	library, err := s.LibraryForShow(show)
	if err != nil {
		return nil, ContentRevision{}, err
	}
	if !viewer.AllowsLibrary(library) {
		return nil, ContentRevision{}, sql.ErrNoRows
	}
	if err := s.prepareViewer(viewer); err != nil {
		return nil, ContentRevision{}, err
	}
	// Published compact facts only: the show, its episodes and their
	// asset links (the old tables are the projector's input, not a read).
	restriction, bound := ItemRestrictionSQL("item.id", viewer.EffectiveRestrictions())
	args := append([]any{show}, bound...)
	var seed string
	if err := s.read().QueryRow(`SELECT pid(item.public_id) FROM catalog_entities sh JOIN catalog_episodes ep ON ep.show_id=sh.id JOIN catalog_entities item ON item.id=ep.entity_id
  WHERE sh.public_id=pid_blob(?) AND EXISTS(SELECT 1 FROM catalog_asset_links a WHERE a.entity_id=item.id) AND `+restriction+` ORDER BY item.id LIMIT 1`, args...).Scan(&seed); err != nil {
		return nil, ContentRevision{}, err
	}
	var title string
	if err := s.read().QueryRow(`SELECT title FROM catalog_entities WHERE public_id=pid_blob(?)`, show).Scan(&title); err != nil {
		return nil, ContentRevision{}, err
	}
	before, err := s.ContentRevision(library, viewer.Profile)
	if err != nil {
		return nil, before, err
	}
	loaded, err := s.Get(viewer.Profile, seed)
	if err != nil {
		return nil, before, err
	}
	genres, err := s.itemGenreList(seed)
	if err != nil {
		return nil, before, err
	}
	rows, err := s.itemRecommendations(viewer, loaded, genres, limit, false, false)
	if err != nil {
		return nil, before, err
	}
	for i := range rows {
		if rows[i].Relation == "more_like" {
			rows[i].Title = "More like " + title
			rows[i].Endpoint = "/v1/shows/" + show + "/recommendations"
		}
	}
	after, err := s.ContentRevision(library, viewer.Profile)
	if err != nil {
		return nil, before, err
	}
	if before != after {
		return nil, before, ErrStaleContinuation
	}
	return rows, before, nil
}
