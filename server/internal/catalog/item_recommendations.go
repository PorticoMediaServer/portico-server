package catalog

import (
	"database/sql"
	"errors"
	"strings"
)

// One recommendation engine serves per-item rows, detail.related and the
// viewer's suggestions. Every candidate comes from persisted library identity
// (genres, credited people, collections, albums, books), never a title guess,
// and every relation is bounded before media hydration.

const itemRecommendationRowLimit = 12
const itemRecommendationRows = 6

type recommendationRelation struct {
	relation, provider, evidence, heading string
	query                                 string
	args                                  []any
}

func recommendationVisible(entityID string) string {
	return `EXISTS(SELECT 1 FROM catalog_asset_links link JOIN catalog_assets asset ON asset.id=link.asset_id WHERE link.entity_id=` + entityID + ` AND link.available=1)
 AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=` + entityID + `)`
}

func (s *Service) recommendationLibrary(id string) (string, error) {
	var name string
	e := s.read().QueryRow(`SELECT name FROM catalog_libraries WHERE library_id=? AND retired=0`, id).Scan(&name)
	return name, e
}

// extendedRelations are the rows the per-item endpoint adds on top of the
// published detail relations: overall facet overlap, the director, and the
// owning show. Movie genre and person rows keep their own identity-aware
// candidate projection and are not rebuilt here.
// extendedRelations adds the director to the per-item endpoint's rows (the
// engine's rows come from recTitleRelations).
func (s *Service) extendedRelations(item Item, genres []Genre) ([]recommendationRelation, error) {
	visible := recommendationVisible("i.id")
	out := []recommendationRelation{}
	var directorProvider, directorID, directorName string
	e := s.read().QueryRow(`SELECT c.provider,c.provider_person_id,c.credited_name
 FROM catalog_entities i JOIN catalog_credits c ON c.entity_id=i.id
 JOIN catalog_credit_labels department ON department.id=c.department_id
 WHERE i.public_id=pid_blob(?) AND c.provider_person_id<>'' AND department.label='Directing'
 ORDER BY c.source_ordinal,c.provider,c.provider_person_id LIMIT 1`, item.ID).Scan(&directorProvider, &directorID, &directorName)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return nil, e
	}
	if e == nil {
		out = append(out, recommendationRelation{"director", directorProvider, directorID, "Directed by " + directorName,
			s.creditQuery("Directing", visible), []any{directorProvider, directorID, item.LibraryID, item.Kind, item.ID, itemRecommendationRowLimit}})
	}
	return out, nil
}

func (s *Service) episodicRelations(item Item, genres []Genre) ([]recommendationRelation, error) {
	var show string
	if item.Kind != "episode" {
		return nil, nil
	}
	if err := s.read().QueryRow(`SELECT pid(sh.public_id) FROM catalog_entities i JOIN catalog_episodes ep ON ep.entity_id=i.id JOIN catalog_entities sh ON sh.id=ep.show_id WHERE i.public_id=pid_blob(?)`, item.ID).Scan(&show); err != nil {
		return nil, err
	}
	candidates, err := s.recommendationCandidates(HomeRequest{Profile: "", Libraries: []string{item.LibraryID}, Restrictions: s.recRestrictions}, "related", item.ID)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for _, c := range candidates {
		ids = append(ids, c.ID)
		if len(ids) == 12 {
			break
		}
	}
	// Related shows share accepted facets; episode lists remain on show detail.
	return []recommendationRelation{{"show", "local", show, "More like " + item.Title, `SELECT value FROM json_each(?)`, []any{idsJSON(ids)}}}, nil
}

func (s *Service) creditQuery(department, visible string) string {
	return `SELECT pid(i.public_id) FROM catalog_credits c JOIN catalog_entities i ON i.id=c.entity_id JOIN catalog_libraries l ON l.id=i.library_id JOIN catalog_credit_labels d ON d.id=c.department_id` +
		` WHERE c.provider=? AND c.provider_person_id=? AND d.label='` + department + `' AND l.library_id=? AND i.kind=CASE ? WHEN 'movie' THEN 1 ELSE -1 END AND i.public_id<>pid_blob(?) AND ` + visible + ` ORDER BY i.year DESC,i.id LIMIT ?`
}

func (s *Service) collectionRelations(item Item) ([]recommendationRelation, error) {
	rows, e := s.read().Query(`SELECT pid(e.public_id),e.title FROM catalog_entities item JOIN catalog_collection_members m ON m.item_id=item.id JOIN catalog_entities e ON e.id=m.collection_id JOIN catalog_collections c ON c.entity_id=e.id WHERE item.public_id=pid_blob(?) AND `+collectionHeadFence+` ORDER BY c.name_key,e.id LIMIT 2`, item.ID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []recommendationRelation{}
	for rows.Next() {
		var id, name string
		if e = rows.Scan(&id, &name); e != nil {
			return nil, e
		}
		out = append(out, recommendationRelation{"collection", "local", id, "More from " + name,
			`SELECT pid(i.public_id) FROM catalog_entities co JOIN catalog_collection_members m ON m.collection_id=co.id JOIN catalog_entities i ON i.id=m.item_id WHERE co.public_id=pid_blob(?) AND i.public_id<>pid_blob(?) AND ` + recommendationVisible("i.id") + ` ORDER BY i.year,i.title COLLATE NOCASE,i.id LIMIT ?`,
			[]any{id, item.ID, itemRecommendationRowLimit}})
	}
	return out, rows.Err()
}

func recommendationRowShape(kind string) string {
	switch kind {
	case "song", "album":
		return "square"
	case "episode":
		return "landscape"
	}
	return "poster"
}

// ItemRecommendations publishes related rows for any item kind: the extended
// relations the per-item endpoint adds, then the published relations that also
// feed detail.related, then collection membership.
func (s *Service) ItemRecommendations(viewer Viewer, item Item, genres []Genre, limit int, community bool) ([]HomeRow, error) {
	return s.itemRecommendations(viewer, item, genres, limit, true, community)
}

// itemRecommendations: the engine's rows first (More like X, Starring, From
// its creator, Viewers also watched when community is on), then the director,
// the published genre and person rows, and collections.
func (s *Service) itemRecommendations(viewer Viewer, item Item, genres []Genre, limit int, full, community bool) ([]HomeRow, error) {
	profile := viewer.Profile
	if limit <= 0 || limit > itemRecommendationRowLimit {
		limit = itemRecommendationRowLimit
	}
	out := []HomeRow{}
	var e error
	listening := item.Kind == "song" || item.Kind == "audiobook_file"
	if item.Kind == "movie" || item.Kind == "episode" || item.Kind == "show" {
		relations, e := s.recTitleRelations(viewer, item, full && community)
		if e != nil {
			return nil, e
		}
		for _, relation := range relations {
			row, ok, e := s.recommendationRow(profile, item, relation, limit)
			if e != nil {
				return nil, e
			}
			if ok {
				out = append(out, row)
			}
		}
	}
	if full && item.Kind == "movie" {
		extended, e := s.extendedRelations(item, genres)
		if e != nil {
			return nil, e
		}
		for _, relation := range extended {
			row, ok, e := s.recommendationRow(profile, item, relation, limit)
			if e != nil {
				return nil, e
			}
			if ok {
				out = append(out, row)
			}
		}
	}
	switch {
	case listening:
		projection, e := s.listeningRecommendations(profile, item)
		if e != nil {
			return nil, e
		}
		if projection != nil {
			for _, row := range projection.Rows {
				entries := row.Entries
				if len(entries) > limit {
					entries = entries[:limit]
				}
				out = append(out, relatedRowAsHome(item, row, entries))
			}
		}
	case item.Kind == "movie":
		projection, e := s.movieRecommendations(profile, item, genres)
		if e != nil {
			return nil, e
		}
		if projection != nil {
			for _, row := range projection.Rows {
				entries := row.Entries
				if len(entries) > limit {
					entries = entries[:limit]
				}
				out = append(out, relatedRowAsHome(item, row, entries))
			}
		}
	}
	if full {
		collections, e := s.collectionRelations(item)
		if e != nil {
			return nil, e
		}
		for _, relation := range collections {
			if len(out) >= itemRecommendationRows {
				break
			}
			row, ok, e := s.recommendationRow(profile, item, relation, limit)
			if e != nil {
				return nil, e
			}
			if ok {
				out = append(out, row)
			}
		}
	}
	if len(out) > itemRecommendationRows {
		out = out[:itemRecommendationRows]
	}
	out, e = s.filterRecommendationRows(viewer, out)
	if e != nil {
		return nil, e
	}
	for index := range out {
		out[index].Priority = index * 10
	}
	return out, nil
}

func (s *Service) recommendationRow(profile string, item Item, relation recommendationRelation, limit int) (HomeRow, bool, error) {
	ids, e := s.recommendationIDs(relation)
	if e != nil {
		return HomeRow{}, false, e
	}
	if len(ids) > limit {
		ids = ids[:limit]
	}
	entries, e := s.homeEngineEntries(profile, ids)
	if e != nil {
		return HomeRow{}, false, e
	}
	trimmed := []ContentEntry{}
	for _, entry := range entries {
		entry.Overview = ""
		entry.BackdropURL = ""
		entry.Playback = nil
		trimmed = append(trimmed, entry)
	}
	if len(trimmed) == 0 {
		return HomeRow{}, false, nil
	}
	row := RelatedMovieRow{ID: relation.relation + ":" + relation.provider + ":" + relation.evidence, Relation: relation.relation,
		Provider: relation.provider, EvidenceID: relation.evidence, Heading: relation.heading, HeadingText: relatedRowText(relation.relation, relation.heading), Entries: trimmed}
	return relatedRowAsHome(item, row, trimmed), true, nil
}

func relatedRowAsHome(item Item, row RelatedMovieRow, entries []ContentEntry) HomeRow {
	kind := item.Kind
	if len(entries) > 0 {
		kind = entries[0].Kind
	}
	return HomeRow{ID: row.ID, Title: row.Heading, TitleText: relatedRowText(row.Relation, row.Heading), Kind: "related", ArtworkShape: recommendationRowShape(kind),
		Endpoint: "/v1/items/" + item.ID + "/recommendations", LibraryID: item.LibraryID, PrivacySensitivity: "catalog",
		PolicyState: "available", Relation: row.Relation, Provider: row.Provider, EvidenceID: row.EvidenceID,
		CacheTTLSeconds: 300, Hideable: true, Reorderable: false, Entries: entries, Total: len(entries), Limit: len(entries)}
}

func (s *Service) recommendationIDs(relation recommendationRelation) ([]string, error) {
	rows, e := s.read().Query(relation.query, relation.args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// relatedProjection keeps detail.related on its published shape while drawing
// its rows from the unified engine. Movie detail keeps the two-genre and
// one-person budget that clients already validate.
func (s *Service) relatedProjection(item Item, rows []HomeRow) (*RelatedMovies, error) {
	name, e := s.recommendationLibrary(item.LibraryID)
	if e != nil {
		return nil, e
	}
	out := &RelatedMovies{Version: 1, LibraryID: item.LibraryID, LibraryName: name, Rows: []RelatedMovieRow{}}
	allowed := map[string]bool{"genre": true, "person": true}
	if item.Kind == "song" {
		allowed = map[string]bool{"album": true, "artist": true, "genre": true}
	} else if item.Kind == "audiobook_file" {
		allowed = map[string]bool{"book": true, "author": true, "genre": true}
	} else if item.Kind != "movie" {
		allowed = map[string]bool{"genre": true, "person": true, "show": true}
	}
	genres, people := 0, 0
	for _, row := range rows {
		if !allowed[row.Relation] || len(row.Entries) == 0 || len(out.Rows) == 3 {
			continue
		}
		if row.Relation == "genre" {
			if genres == 2 {
				continue
			}
			genres++
		}
		if row.Relation == "person" {
			if people == 1 {
				continue
			}
			people++
		}
		entries := row.Entries
		if len(entries) > relatedEntryLimit {
			entries = entries[:relatedEntryLimit]
		}
		out.Rows = append(out.Rows, RelatedMovieRow{ID: row.ID, Relation: row.Relation, Provider: row.Provider,
			EvidenceID: row.EvidenceID, Heading: row.Title, HeadingText: relatedRowText(row.Relation, row.Title), Entries: entries})
	}
	return out, nil
}

// --- viewer suggestions ----------------------------------------------------

type Suggestion struct {
	Entry  ContentEntry `json:"entry"`
	Reason string       `json:"reason"`
	Source string       `json:"source"`
	Score  float64      `json:"score"`
}

type Suggestions struct {
	Items       []Suggestion    `json:"items"`
	Total       int             `json:"total"`
	Revision    ContentRevision `json:"revision"`
	GeneratedAt string          `json:"generatedAt"`
}

// Suggestions ranks unwatched candidates for the viewer from the same facet
// overlap that drives the recommended home row, then tops up from the watchlist.
func (s *Service) Suggestions(r HomeRequest) (Suggestions, error) {
	if r.Profile != "" && r.Profile != r.Viewer.Profile || r.ViewerFence != "" && r.ViewerFence != r.Viewer.Fence {
		return Suggestions{}, ErrCursor
	}
	r = r.scoped()
	if err := s.prepareViewer(r.Viewer); err != nil {
		return Suggestions{}, err
	}
	r.Libraries = homeUnique(r.Libraries)
	limit := homeLimit(r.Limit)
	out := Suggestions{Items: []Suggestion{}}
	before, e := s.homeRevision(r.Libraries, r.Profile)
	if e != nil {
		return out, e
	}
	out.Revision = before
	candidates, e := s.recommendationCandidates(r, "recommended", "")
	if e != nil {
		return out, e
	}
	ids := []string{}
	for _, c := range candidates {
		if len(ids) == limit {
			break
		}
		ids = append(ids, c.ID)
	}
	entries, e := s.homeEngineEntries(r.Profile, ids)
	if e != nil {
		return out, e
	}
	for _, entry := range entries {
		out.Items = append(out.Items, Suggestion{Entry: entry, Reason: "Recommended for you", Source: "because_you_watched", Score: 0.6})
	}
	out.Total = len(out.Items)
	after, e := s.homeRevision(r.Libraries, r.Profile)
	if e != nil {
		return out, e
	}
	if before != after {
		return out, ErrStaleContinuation
	}
	out.GeneratedAt = r.now().UTC().Format("2006-01-02T15:04:05.000Z")
	return out, nil
}

// ItemRecommendationRows is the endpoint projection: it loads the item, its
// genres and the related rows under one revision fence.
// ItemRecommendationRows are a title page's rows; community is the owner's
// community-activity setting (Viewers also watched).
func (s *Service) ItemRecommendationRows(viewer Viewer, item string, limit int, community bool) ([]HomeRow, ContentRevision, error) {
	if e := s.VisibleItem(s.Context(), viewer, item); e != nil {
		return nil, ContentRevision{}, e
	}
	profile := viewer.Profile
	library, e := s.LibraryForItem(item)
	if e != nil {
		return nil, ContentRevision{}, e
	}
	before, e := s.ContentRevision(library, profile)
	if e != nil {
		return nil, before, e
	}
	loaded, e := s.Get(profile, item)
	if e != nil {
		return nil, before, e
	}
	genres, e := s.itemGenreList(item)
	if e != nil {
		return nil, before, e
	}
	out, e := s.ItemRecommendations(viewer, loaded, genres, limit, community)
	if e != nil {
		return nil, before, e
	}
	after, e := s.ContentRevision(library, profile)
	if e != nil {
		return nil, before, e
	}
	if before != after {
		return nil, before, ErrStaleContinuation
	}
	return out, before, nil
}

// itemGenres is one item's genre evidence for the recommendation engine.
// itemGenreList is an item's published genres (compact term sources), in
// the order the recommendation seed uses.
func (s *Service) itemGenreList(item string) ([]Genre, error) {
	rows, e := s.itemGenres(item, "")
	if e != nil {
		return nil, e
	}
	genres := []Genre{}
	for rows.Next() {
		var genre Genre
		if e = rows.Scan(&genre.ID, &genre.Name, &genre.Provider); e != nil {
			rows.Close()
			return nil, e
		}
		genres = append(genres, genre)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return nil, e
	}
	return genres, nil
}

// Final relation boundary also protects legacy facet-specific rows. The same
// work map collapses editions, rejects negatives, and requires a visible child.
func (s *Service) filterRecommendationRows(viewer Viewer, rows []HomeRow) ([]HomeRow, error) {
	profile := viewer.Profile
	r := HomeRequest{Profile: profile, Libraries: viewer.Libraries, Restrictions: viewer.EffectiveRestrictions()}
	// Only the listed entries (and a bounded sample of each listed show's,
	// album's or book's members) are resolved; never the viewer's libraries.
	members, err := s.recommendationRowMembers(rows)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 {
		return []HomeRow{}, nil
	}
	base, args := recBase(r, members)
	// Entries are named by public id: both the member and its work's
	// canonical entity are returned as public ids (members carry the row's
	// public_id; a work's id is an integer entity id).
	query := base + ` SELECT pid(m.public_id),(SELECT pid(we.public_id) FROM catalog_entities we WHERE we.id=w.id),w.work FROM members m JOIN works w ON w.work=m.work JOIN personal p ON p.work=w.work WHERE p.negative=0
 UNION SELECT (SELECT pid(we.public_id) FROM catalog_entities we WHERE we.id=w.id),(SELECT pid(we.public_id) FROM catalog_entities we WHERE we.id=w.id),w.work FROM works w JOIN personal p ON p.work=w.work WHERE p.negative=0`
	found, err := s.read().Query(query, args...)
	if err != nil {
		return nil, err
	}
	canonical, work := map[string]string{}, map[string]string{}
	for found.Next() {
		var id, target, key string
		if err = found.Scan(&id, &target, &key); err != nil {
			found.Close()
			return nil, err
		}
		canonical[id] = target
		work[id] = key
	}
	err = found.Err()
	found.Close()
	if err != nil {
		return nil, err
	}
	out := []HomeRow{}
	for _, row := range rows {
		ids := []string{}
		seen := map[string]bool{}
		for _, entry := range row.Entries {
			target := canonical[entry.ID]
			if target == "" || seen[work[entry.ID]] {
				continue
			}
			seen[work[entry.ID]] = true
			ids = append(ids, target)
		}
		if len(ids) == 0 {
			continue
		}
		entries, e := s.homeEngineEntries(profile, ids)
		if e != nil {
			return nil, e
		}
		for i := range entries {
			entries[i].Playback = nil
			entries[i].Overview = ""
			entries[i].BackdropURL = ""
		}
		row.Entries = entries
		row.Total = len(entries)
		row.Limit = len(entries)
		out = append(out, row)
	}
	return out, nil
}

// recommendationRowMembers is the item ids behind a set of recommendation
// rows: item entries themselves, and up to five member items of each entity
// entry (a show's episodes, an album's songs, a book's parts), by index.
func (s *Service) recommendationRowMembers(rows []HomeRow) ([]string, error) {
	if len(rows) == 0 {
		return []string{}, nil
	}
	out, seen := []string{}, map[string]bool{}
	for _, row := range rows {
		for _, entry := range row.Entries {
			if seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			found, err := s.read().Query(`SELECT pid(e.public_id) FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind WHERE e.public_id=pid_blob(?) AND k.playable=1 AND e.kind<>11
			 UNION ALL SELECT pid(public_id) FROM (SELECT item.public_id FROM catalog_entities parent JOIN catalog_episodes child ON child.show_id=parent.id JOIN catalog_entities item ON item.id=child.entity_id WHERE parent.public_id=pid_blob(?) LIMIT 5)
			 UNION ALL SELECT pid(public_id) FROM (SELECT item.public_id FROM catalog_entities parent JOIN catalog_songs child ON child.album_id=parent.id JOIN catalog_entities item ON item.id=child.entity_id WHERE parent.public_id=pid_blob(?) LIMIT 5)
			 UNION ALL SELECT pid(public_id) FROM (SELECT item.public_id FROM catalog_entities parent JOIN catalog_book_files child ON child.book_id=parent.id JOIN catalog_entities item ON item.id=child.entity_id WHERE parent.public_id=pid_blob(?) LIMIT 5)`, entry.ID, entry.ID, entry.ID, entry.ID)
			if err != nil {
				return nil, err
			}
			for found.Next() {
				var id string
				if err = found.Scan(&id); err != nil {
					found.Close()
					return nil, err
				}
				if !seen["item:"+id] {
					seen["item:"+id] = true
					out = append(out, id)
					if len(out) == relatedPoolLimit {
						found.Close()
						return out, nil
					}
				}
			}
			err = found.Err()
			found.Close()
			if err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

// relatedRowHeadings are the catalogue ids of the per-item row headings, with
// the fixed text before the one parameter (CON-19).
var relatedRowHeadings = map[string]struct{ code, prefix, param string }{
	"because_you_watched":  {"home.row.becauseYouWatched", "Because you watched ", "title"},
	"more_like":            {"home.row.moreLike", "More like ", "title"},
	"starring":             {"home.row.starring", "Starring ", "name"},
	"creator":              {"home.row.fromCreator", "From ", "name"},
	"viewers_also_watched": {"home.row.viewersAlsoWatched", "Viewers also watched", ""},
	"director":             {"home.row.directedBy", "Directed by ", "name"},
	"show":                 {"home.row.moreLike", "More like ", "title"},
	"collection":           {"home.row.moreFrom", "More from ", "name"},
}

// relatedRowText is a per-item row heading as {code, params, fallback}. A
// heading that doesn't carry its relation's fixed text keeps only the fallback.
func relatedRowText(relation, heading string) ServerText {
	spec, ok := relatedRowHeadings[relation]
	if !ok || !strings.HasPrefix(heading, spec.prefix) {
		return ServerText{Fallback: heading}
	}
	if spec.param == "" {
		return ServerText{Code: spec.code, Fallback: heading}
	}
	return ServerText{Code: spec.code, Params: map[string]string{spec.param: strings.TrimPrefix(heading, spec.prefix)}, Fallback: heading}
}
