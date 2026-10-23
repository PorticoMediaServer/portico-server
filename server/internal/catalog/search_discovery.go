package catalog

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
)

type SearchGenre struct {
	PosterURLs        []string `json:"posterUrls"`
	Value             string   `json:"value"`
	Label             string   `json:"label"`
	Count             int      `json:"count"`
	CountIsLowerBound bool     `json:"countIsLowerBound"`
	BackdropURL       string   `json:"backdropUrl,omitempty"`
}
type SearchDiscovery struct {
	Scope            ContentScope    `json:"scope"`
	Revision         ContentRevision `json:"revision"`
	Genres           []SearchGenre   `json:"genres"`
	GenresNextCursor string          `json:"genresNextCursor"`
	OnNow            []ContentEntry  `json:"onNow"`
}

func (s *Service) SearchDiscovery(viewer Viewer, server, cursor string, limit int) (SearchDiscovery, error) {
	out := SearchDiscovery{Scope: ContentScope{ServerID: server, ViewerFence: viewer.Fence, LibraryKind: "mixed", View: "search-discovery"}, Genres: []SearchGenre{}, OnNow: []ContentEntry{}}
	if limit < 1 || limit > 24 {
		return out, ErrSearchQuery
	}
	if dbwork.Snapshot(s.Context()) == nil {
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var e error
			out, e = s.WithContext(ctx).SearchDiscovery(viewer, server, cursor, limit)
			return e
		})
		return out, err
	}
	if err := s.prepareViewer(viewer); err != nil {
		return out, err
	}
	rows, err := s.read().Query(`SELECT c.library_id FROM catalog_libraries c JOIN libraries l ON l.id=c.library_id WHERE c.retired=0 AND l.kind IN('movie','tv','anime') AND c.library_id IN(SELECT value FROM json_each(?)) ORDER BY c.library_id`, viewer.librariesJSON())
	if err != nil {
		return out, err
	}
	libraries := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			break
		}
		libraries = append(libraries, id)
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	viewer.Libraries = libraries
	r := SearchRequest{Viewer: viewer, Profile: viewer.Profile, ViewerFence: viewer.Fence, Libraries: libraries}
	rev, stamp, err := searchRevision(s.Context(), s.read(), r, viewer.librariesJSON())
	if err != nil {
		return out, err
	}
	out.Revision = rev
	scope := cursorScope{Profile: viewer.Profile, Viewer: viewer.Fence, Library: stamp, View: "search-discovery", Sort: "genre", Limit: limit}
	offset := 0
	if cursor != "" {
		c, e := s.decodeSearchCursor(cursor, scope, rev)
		if e != nil {
			return out, e
		}
		offset, e = strconv.Atoi(c.ID)
		if e != nil || offset < 0 {
			return out, ErrCursor
		}
	}
	page, err := s.FacetsAcross(viewer, FacetRequest{Field: "genre", Pivot: "titles", Limit: limit + 1, Offset: offset, Sort: "title:asc"})
	if err != nil {
		return out, err
	}
	values := page.Values
	if len(values) > limit {
		values = values[:limit]
		out.GenresNextCursor, err = s.encodeRevisionCursor(cursorValue{Scope: scope, ID: strconv.Itoa(offset + limit), Expires: time.Now().Add(30 * time.Minute).Unix()}, rev)
		if err != nil {
			return out, err
		}
	}
	if len(values) == 0 {
		return out, nil
	}
	names := []string{}
	for _, v := range values {
		names = append(names, v.Value)
		out.Genres = append(out.Genres, SearchGenre{Value: v.Value, Label: v.Label, Count: v.Count, CountIsLowerBound: v.CountIsLowerBound, PosterURLs: []string{}})
	}
	selection, args, _, err := s.browseMeasureScope(viewer, FacetRequest{Pivot: "titles"})
	if err != nil {
		return out, err
	}
	visible, bound := viewer.itemVisibilitySQL("member.item_id")
	args = append(args, bound...)
	raw, _ := json.Marshal(names)
	args = append(args, string(raw))
	// Start each sample from an indexed returned-genre posting, then check
	// its actual visible member. Do not materialize every title or hydrate a
	// detail per tile. Prefer a real backdrop; no image is invented.
	rows, err = s.read().Query(`WITH targets AS (`+selection+`), samples AS (
 SELECT requested.value label,(SELECT json_group_array(id) FROM (SELECT DISTINCT target.entity_id id,shape.backdrop FROM (
 SELECT ts.entity_id FROM catalog_terms term INDEXED BY catalog_terms_search_label
 JOIN catalog_term_sources ts INDEXED BY catalog_term_sources_term ON ts.term_id=term.id
 WHERE term.vocab=1 AND term.label=requested.value AND ts.label_override IS NULL
 UNION SELECT ts.entity_id FROM catalog_term_sources ts INDEXED BY catalog_term_sources_overrides
 JOIN catalog_terms term ON term.id=ts.term_id AND term.vocab=1
 WHERE ts.label_override IS NOT NULL AND ts.label_override=requested.value
 ) genre_source CROSS JOIN json_each((SELECT json_group_array(id) FROM (
 SELECT genre_source.entity_id id UNION SELECT bm.entity_id FROM catalog_browse_memberships bm INDEXED BY catalog_browse_membership_item JOIN catalog_browse_rows holder ON holder.entity_id=bm.entity_id AND holder.kind IN(1,2) WHERE bm.item_id=genre_source.entity_id))) candidate
 JOIN targets target ON target.entity_id=candidate.value JOIN catalog_browse_rows shape ON shape.entity_id=target.entity_id
 WHERE EXISTS(SELECT 1 FROM catalog_browse_memberships member
 JOIN catalog_term_sources ts ON ts.entity_id IN(member.item_id,(SELECT ep.show_id FROM catalog_episodes ep WHERE ep.entity_id=member.item_id))
 JOIN catalog_terms term ON term.id=ts.term_id AND term.vocab=1
 WHERE member.entity_id=target.entity_id AND `+visible+` AND COALESCE(ts.label_override,term.label)=requested.value)
 ORDER BY shape.backdrop DESC,target.entity_id LIMIT 2)) ids
 FROM json_each(?) requested
 ) SELECT sample.label,pid(e.public_id),k.name FROM samples sample CROSS JOIN json_each(sample.ids) picked JOIN catalog_entities e ON e.id=picked.value JOIN catalog_kinds k ON k.id=e.kind`, args...)
	if err != nil {
		return out, err
	}
	targets := []artworkTarget{}
	byLabel := map[string][]artworkTarget{}
	for rows.Next() {
		var label, id, kind string
		if err = rows.Scan(&label, &id, &kind); err != nil {
			break
		}
		if isItemEntity(kind) {
			kind = "item"
		}
		t := artworkTarget{kind, id}
		targets = append(targets, t)
		byLabel[label] = append(byLabel[label], t)
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	art, err := s.resolveArtwork(targets)
	if err != nil {
		return out, err
	}
	for i := range out.Genres {
		for _, target := range byLabel[out.Genres[i].Value] {
			image := art[target]
			if out.Genres[i].BackdropURL == "" {
				out.Genres[i].BackdropURL = image.BackdropURL
			}
			if image.PosterURL != "" {
				out.Genres[i].PosterURLs = append(out.Genres[i].PosterURLs, image.PosterURL)
			}
		}
	}
	return out, nil
}
