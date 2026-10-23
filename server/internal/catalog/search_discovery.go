package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
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
	scope.Viewer += ":" + RestrictionFence(viewer.Restrictions)
	c := searchDiscoveryCursor{Scope: scope, Revision: rev, Expires: time.Now().Add(30 * time.Minute).Unix()}
	if cursor != "" {
		if e := s.state.searchCursors.Get(cursor, "search-discovery", &c); e != nil {
			return out, ErrStaleContinuation
		}
		if c.Scope != scope || c.Revision != rev {
			return out, ErrStaleContinuation
		}
	}
	if len(c.Genres) == 0 {
		after := c.AfterGenre
		for i := 0; i < limit+1; i++ {
			label, e := s.nextSearchGenre(after)
			if e != nil {
				return out, e
			}
			if label == "" {
				break
			}
			if i == limit {
				c.More = true
				break
			}
			c.Genres = append(c.Genres, SearchGenre{Value: label, Label: label, PosterURLs: []string{}})
			after = label
		}
		if len(c.Genres) == 0 {
			return out, nil
		}
		c.AfterGenre = after
	}
	complete, err := s.walkSearchGenres(viewer, libraries, &c)
	if err != nil {
		return out, err
	}
	if complete {
		targets := []artworkTarget{}
		for _, samples := range c.Samples {
			for _, sample := range samples {
				targets = append(targets, sample.Target)
			}
		}
		art, e := s.resolveArtwork(targets)
		if e != nil {
			return out, e
		}
		for _, g := range c.Genres {
			if g.Count == 0 {
				continue
			}
			for _, sample := range c.Samples[g.Value] {
				image := art[sample.Target]
				if g.BackdropURL == "" {
					g.BackdropURL = image.BackdropURL
				}
				if image.PosterURL != "" {
					g.PosterURLs = append(g.PosterURLs, image.PosterURL)
				}
			}
			out.Genres = append(out.Genres, g)
		}
		if !c.More {
			return out, nil
		}
		c.Genres = nil
		c.Samples = nil
		c.More = false
		c.Library = 0
		c.Kind = 0
		c.Target = 0
		c.Member = 0
		c.CountTarget = 0
		c.Counted = nil
	}
	// Incomplete exact counts stay private. Empty progress windows retain a
	// continuation, never a private vocabulary name or an invented facet count.
	out.GenresNextCursor, err = s.state.searchCursors.Put("search-discovery", c, time.Unix(c.Expires, 0))
	return out, err
}

const searchGenreWindow = 128

type searchGenreSample struct {
	Target   artworkTarget
	Backdrop bool
}
type searchDiscoveryCursor struct {
	Scope                       cursorScope
	Revision                    ContentRevision
	Expires                     int64
	AfterGenre                  string
	Genres                      []SearchGenre
	More                        bool
	Library, Kind               int
	Target, Member, CountTarget int64
	Counted                     map[string]bool
	Samples                     map[string][]searchGenreSample
}

func (s *Service) nextSearchGenre(after string) (string, error) {
	// Each branch seeks one distinct label. Preserve the original NOCASE,label
	// ordering without grouping whole posting lists, including label overrides.
	query := `SELECT label FROM (
 SELECT label FROM (SELECT label FROM catalog_terms INDEXED BY catalog_terms_search_label
 WHERE vocab=1 AND label COLLATE NOCASE=? AND label>? ORDER BY label,id LIMIT 1)
 UNION ALL SELECT label FROM (SELECT label FROM catalog_terms INDEXED BY catalog_terms_search_label
 WHERE vocab=1 AND label COLLATE NOCASE>? ORDER BY label COLLATE NOCASE,label,id LIMIT 1)
 UNION ALL SELECT label FROM (SELECT label_override label FROM catalog_term_sources INDEXED BY catalog_term_sources_search_label
 WHERE label_override IS NOT NULL AND label_override COLLATE NOCASE=? AND label_override>? ORDER BY label_override,entity_id,term_id LIMIT 1)
 UNION ALL SELECT label FROM (SELECT label_override label FROM catalog_term_sources INDEXED BY catalog_term_sources_search_label
 WHERE label_override IS NOT NULL AND label_override COLLATE NOCASE>? ORDER BY label_override COLLATE NOCASE,label_override,entity_id,term_id LIMIT 1)
 ) ORDER BY label COLLATE NOCASE,label LIMIT 1`
	var next string
	err := s.read().QueryRow(query, after, after, after, after, after, after).Scan(&next)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return next, err
}

// Bound title keys before expanding bounded memberships. Even one huge show
// progresses by member key, without materializing its whole visible relation.
const searchGenreSourceWindow = `WITH titles AS MATERIALIZED (
 SELECT id FROM catalog_entities INDEXED BY catalog_entities_library
 WHERE library_id=? AND kind=? AND id>=? ORDER BY id LIMIT 130
) SELECT t.id,COALESCE(m.value,0) FROM titles t LEFT JOIN json_each((
 SELECT json_group_array(item_id) FROM (SELECT DISTINCT item_id
 FROM catalog_browse_memberships WHERE entity_id=t.id AND item_id>CASE WHEN t.id=? THEN ? ELSE 0 END
 ORDER BY item_id LIMIT 129))) m
 WHERE NOT(t.id=? AND ?=-1) ORDER BY t.id,m.value LIMIT ?`

func (s *Service) walkSearchGenres(viewer Viewer, libraries []string, c *searchDiscoveryCursor) (bool, error) {
	if c.Samples == nil {
		c.Samples = map[string][]searchGenreSample{}
	}
	names := []string{}
	for _, g := range c.Genres {
		names = append(names, g.Value)
	}
	namesJSON, _ := json.Marshal(names)
	budget := searchGenreWindow
	for sources := 0; sources < searchGenreWindow && c.Library < len(libraries) && budget > 0; sources++ {
		var library int64
		if err := s.read().QueryRow(`SELECT id FROM catalog_libraries WHERE library_id=? AND retired=0`, libraries[c.Library]).Scan(&library); err != nil {
			return false, err
		}
		kind := 1
		if c.Kind == 1 {
			kind = 2
		}
		rows, err := s.read().Query(searchGenreSourceWindow, library, kind, c.Target, c.Target, c.Member, c.Target, c.Member, budget+1)
		if err != nil {
			return false, err
		}
		pairs := [][2]int64{}
		for rows.Next() {
			var p [2]int64
			if err = rows.Scan(&p[0], &p[1]); err != nil {
				break
			}
			pairs = append(pairs, p)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return false, err
		}
		more := len(pairs) > budget
		if more {
			pairs = pairs[:budget]
		}
		if len(pairs) > 0 {
			raw, _ := json.Marshal(pairs)
			visible, args := viewer.itemVisibilitySQL("p.member")
			query := `WITH picked AS (SELECT json_extract(value,'$[0]') target,json_extract(value,'$[1]') member FROM json_each(?))
 SELECT DISTINCT p.target,genre.value,pid(e.public_id),e.kind,b.backdrop
 FROM picked p CROSS JOIN json_each(?) genre
 JOIN catalog_entities e ON e.id=p.target AND e.retired=0
 JOIN catalog_browse_rows b ON b.entity_id=p.target AND b.kind IN(1,2)
 WHERE p.member>0 AND ` + visible + ` AND EXISTS(
 SELECT 1 FROM catalog_term_sources ts JOIN catalog_terms term ON term.id=ts.term_id AND term.vocab=1
 WHERE ts.entity_id IN(p.member,(SELECT ep.show_id FROM catalog_episodes ep WHERE ep.entity_id=p.member)) AND COALESCE(ts.label_override,term.label)=genre.value)
 ORDER BY p.target,genre.value`
			rows, err = s.read().Query(query, append([]any{string(raw), string(namesJSON)}, args...)...)
			if err != nil {
				return false, err
			}
			type match struct {
				label  string
				sample searchGenreSample
			}
			matches := map[int64][]match{}
			for rows.Next() {
				var target int64
				var label, id string
				var k int
				var backdrop bool
				if err = rows.Scan(&target, &label, &id, &k, &backdrop); err != nil {
					break
				}
				artKind := "item"
				if k == 2 {
					artKind = "show"
				}
				matches[target] = append(matches[target], match{label, searchGenreSample{artworkTarget{artKind, id}, backdrop}})
			}
			if err == nil {
				err = rows.Err()
			}
			rows.Close()
			if err != nil {
				return false, err
			}
			for _, p := range pairs {
				if c.CountTarget != p[0] {
					c.CountTarget = p[0]
					c.Counted = map[string]bool{}
				}
				for _, m := range matches[p[0]] {
					if c.Counted[m.label] {
						continue
					}
					c.Counted[m.label] = true
					for i := range c.Genres {
						if c.Genres[i].Value == m.label {
							c.Genres[i].Count++
							break
						}
					}
					samples := c.Samples[m.label]
					if m.sample.Backdrop {
						samples = append([]searchGenreSample{m.sample}, samples...)
					} else if len(samples) < 2 {
						samples = append(samples, m.sample)
					}
					if len(samples) > 2 {
						samples = samples[:2]
					}
					c.Samples[m.label] = samples
				}
				c.Target, c.Member = p[0], p[1]
				if p[1] == 0 {
					c.Member = -1
				}
			}
		}
		budget -= len(pairs)
		if more {
			return false, nil
		}
		c.Target = 0
		c.Member = 0
		c.CountTarget = 0
		c.Counted = nil
		c.Kind++
		if c.Kind == 2 {
			c.Kind = 0
			c.Library++
		}
	}
	return c.Library == len(libraries), nil
}
