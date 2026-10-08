package catalog

import (
	"database/sql"
	"errors"
	"strings"

	"portico.local/server/internal/segmentmarkers"
)

func (s *Service) detailOnce(viewer Viewer, server, item string, owner bool) (Detail, error) {
	out := Detail{Markers: []segmentmarkers.Marker{}, Actions: []DetailAction{}, Metadata: DetailMetadata{Status: "unavailable", Sources: []MetadataSource{}, Ratings: []ProviderRating{}, Genres: []Genre{}, Credits: []Credit{}}}
	if e := s.VisibleItem(s.Context(), viewer, item); e != nil {
		return out, e
	}
	profile, fence := viewer.Profile, viewer.Fence
	library, e := s.LibraryForItem(item)
	if e != nil {
		return out, e
	}
	before, e := s.ContentRevision(library, profile)
	if e != nil {
		return out, e
	}
	out.Scope = DetailScope{server, library, item, fence}
	out.Revision = before
	out.Item, e = s.Get(profile, item)
	if e != nil {
		return out, e
	}
	out.Personal, e = readPersonal(s.read(), profile, item)
	if e != nil {
		return out, e
	}
	out.Item.ProgressSeconds = out.Personal.ProgressSeconds
	min, max, step := .5, 5., .5
	out.Actions = []DetailAction{{ID: "watchlist", LabelKey: "action.watchlist", Enabled: true}, {ID: "favorite", LabelKey: "action.favorite", Enabled: true}, {ID: "rating", LabelKey: "action.rating", Enabled: true, Min: &min, Max: &max, Step: &step}}
	out.Actions = append(out.Actions, DetailAction{ID: "watched", LabelKey: "action.watched", Enabled: true})
	if out.Item.Available {
		zero := 0.
		resume := out.Item.ProgressSeconds
		if resume > out.Item.Duration {
			resume = out.Item.Duration
		}
		if out.Item.Kind != "audiobook_file" && resume >= out.Item.Duration-3 {
			resume = 0
		}
		if resume > 0 {
			out.Actions = append(out.Actions, DetailAction{ID: "resume", LabelKey: "action.resume", Enabled: true, Playback: &ContentPlayback{ItemID: item, StartSeconds: &resume}}, DetailAction{ID: "start_over", LabelKey: "action.start_over", Enabled: true, Playback: &ContentPlayback{ItemID: item, StartSeconds: &zero}})
		} else {
			out.Actions = append(out.Actions, DetailAction{ID: "play", LabelKey: "action.play", Enabled: true, Playback: &ContentPlayback{ItemID: item, StartSeconds: &zero}})
		}
	}
	if owner {
		if out.Item.Kind == "audiobook_file" {
			out.Actions = append(out.Actions, DetailAction{ID: "local_metadata_policy", LabelKey: "action.local_metadata_policy", Enabled: true})
		}
		if out.Item.Kind == "song" {
			out.Actions = append(out.Actions, DetailAction{ID: "music_metadata_review", LabelKey: "action.music_metadata_review", Enabled: true})
		}
		if editableMetadataKind(out.Item.Kind) {
			out.Actions = append(out.Actions, DetailAction{ID: "metadata_edit", LabelKey: "action.metadata_edit", Enabled: true})
		}
		out.Actions = append(out.Actions, DetailAction{ID: "delete", LabelKey: "action.unlink_item", Enabled: true})
	}
	if out.Facts, e = s.titleFacts(item); e != nil {
		return out, e
	}
	if out.Files, e = s.titleFiles(item, owner); e != nil {
		return out, e
	}
	var known int
	if e = s.read().QueryRow(`SELECT count(*) FROM metadata_details WHERE item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, item).Scan(&known); e != nil {
		return out, e
	}
	if known > 0 {
		out.Metadata.Status = "available"
	}
	rows, e := s.read().Query(`SELECT provider,value,scale,votes,source_url,observed_at FROM metadata_ratings WHERE item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) ORDER BY provider LIMIT 16`, item)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v ProviderRating
		if e = rows.Scan(&v.Provider, &v.Value, &v.Max, &v.Votes, &v.SourceURL, &v.ObservedAt); e != nil {
			rows.Close()
			return out, e
		}
		out.Metadata.Ratings = append(out.Metadata.Ratings, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = s.itemGenres(item, out.Item.Kind)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var v Genre
		if e = rows.Scan(&v.ID, &v.Name, &v.Provider); e != nil {
			rows.Close()
			return out, e
		}
		out.Metadata.Genres = append(out.Metadata.Genres, v)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	// The cast's first page and the key crew; the rest pages from
	// GET /v1/items/{id}/credits (credits.go).
	credits, totals, e := s.detailCredits(item)
	if e != nil {
		return out, e
	}
	out.Metadata.Credits, out.Metadata.CreditTotals = credits, totals
	rows, e = s.read().Query(`SELECT provider,source_url,observed_at FROM metadata_details WHERE item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) ORDER BY provider LIMIT 16`, item)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var source MetadataSource
		if e = rows.Scan(&source.Provider, &source.SourceURL, &source.ObservedAt); e != nil {
			rows.Close()
			return out, e
		}
		out.Metadata.Sources = append(out.Metadata.Sources, source)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = s.read().Query(`SELECT DISTINCT c.attribution FROM artwork_selections a JOIN artwork_candidates c ON c.id=a.candidate_id WHERE c.attribution<>'' AND ((a.kind='item' AND a.entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))) OR (a.kind='album' AND a.entity_id IN(SELECT album.id FROM catalog_entities source JOIN catalog_songs song ON song.entity_id=source.id JOIN catalog_entities album ON album.id=song.album_id WHERE source.public_id=pid_blob(?))) OR (a.kind='book' AND a.entity_id IN(SELECT book.id FROM catalog_entities source JOIN catalog_book_files part ON part.entity_id=source.id JOIN catalog_entities book ON book.id=part.book_id WHERE source.public_id=pid_blob(?)))) ORDER BY c.attribution LIMIT 12`, item, item, item)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var text string
		if e = rows.Scan(&text); e != nil {
			rows.Close()
			return out, e
		}
		out.Metadata.Attributions = append(out.Metadata.Attributions, text)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	var tvdb int
	if e = s.read().QueryRow(`SELECT count(*) FROM metadata_details WHERE item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND provider='tvdb'`, item).Scan(&tvdb); e != nil {
		return out, e
	}
	if tvdb > 0 {
		seen := false
		for _, text := range out.Metadata.Attributions {
			if strings.Contains(text, "TheTVDB") {
				seen = true
			}
		}
		if !seen && len(out.Metadata.Attributions) < 12 {
			out.Metadata.Attributions = append(out.Metadata.Attributions, "Metadata provided by TheTVDB. https://thetvdb.com")
		}
	}
	// One engine serves detail.related, per-item recommendation rows and
	// suggestions. detail.related keeps its published shape and budgets.
	related, e := s.itemRecommendations(viewer, out.Item, out.Metadata.Genres, relatedEntryLimit, false, false)
	if e != nil {
		return out, e
	}
	if out.Related, e = s.relatedProjection(out.Item, related); e != nil {
		return out, e
	}
	if out.Extras, e = s.itemExtras(profile, out.Item); e != nil {
		return out, e
	}
	after, e := s.ContentRevision(library, profile)
	if e != nil {
		return out, e
	}
	if before != after {
		return out, ErrStaleContinuation
	}
	return out, nil
}

// Detail has no continuation, so a read that raced a publication simply retries.
func (s *Service) Detail(viewer Viewer, server, item string, owner bool) (Detail, error) {
	return consistentRead(true, func() (Detail, error) { return s.detailOnce(viewer, server, item, owner) })
}

// titleFacts reads the item's own facts (the editor's General fields); nil when the item has
// no details row or every field is empty.
func (s *Service) titleFacts(item string) (*TitleFacts, error) {
	var f TitleFacts
	err := s.read().QueryRow(`SELECT COALESCE(original_title,''),COALESCE(edition,''),COALESCE(tagline,''),COALESCE(release_date,''),COALESCE(content_rating,''),COALESCE(studio,''),COALESCE(network,''),COALESCE(country,'')
	 FROM catalog_item_details WHERE entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))`, item).Scan(&f.OriginalTitle, &f.Edition, &f.Tagline, &f.ReleaseDate, &f.ContentRating, &f.Studio, &f.Network, &f.Country)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if f == (TitleFacts{}) {
		return nil, nil
	}
	return &f, nil
}
