package catalog

import (
	"context"
	"encoding/json"
	"net/url"
)

type artworkTarget struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// StillURL is an episode's own still (the still or thumbnail role on the item
// itself), never inherited: an episode without one has none, and clients draw
// a placeholder rather than repeat the show's art (Spec — Page Content §0.2).
type resolvedArtwork struct{ PosterURL, BackdropURL, LogoURL, StillURL string }

// resolveArtwork resolves a page, including container inheritance, in one read.
// Targets have already passed the caller's visibility checks; no library scan
// or per-item lookup is hidden inside the resolver.
func (s *Service) resolveArtwork(targets []artworkTarget) (map[artworkTarget]resolvedArtwork, error) {
	return s.resolveArtworkContext(s.Context(), targets)
}

func (s *Service) resolveArtworkContext(ctx context.Context, targets []artworkTarget) (map[artworkTarget]resolvedArtwork, error) {
	out := map[artworkTarget]resolvedArtwork{}
	if len(targets) == 0 {
		return out, nil
	}
	raw, _ := json.Marshal(targets)
	rows, err := s.read().QueryContext(ctx, `WITH requested AS MATERIALIZED (
 SELECT json_extract(value,'$.kind') kind,json_extract(value,'$.id') id FROM json_each(?)
 ), resolved AS MATERIALIZED (
 SELECT r.kind kind,r.id id,e.id entity_id FROM requested r CROSS JOIN catalog_entities e ON e.public_id=pid_blob(r.id)
 ), candidates AS (
 -- Every branch starts from the requested entities and reaches their parents
 -- by key; left to the planner, a branch scanned all episodes or songs.
 SELECT kind,id,kind target_kind,entity_id target_id,0 priority FROM resolved
 UNION ALL SELECT r.kind,r.id,'season',parent.id,1 FROM resolved r CROSS JOIN catalog_episodes ep NOT INDEXED ON r.kind='item' AND ep.entity_id=r.entity_id CROSS JOIN catalog_entities parent ON parent.id=ep.season_id
 UNION ALL SELECT r.kind,r.id,'show',parent.id,2 FROM resolved r CROSS JOIN catalog_episodes ep NOT INDEXED ON r.kind='item' AND ep.entity_id=r.entity_id CROSS JOIN catalog_entities parent ON parent.id=ep.show_id
 UNION ALL SELECT r.kind,r.id,'show',parent.id,1 FROM resolved r CROSS JOIN catalog_seasons se ON r.kind='season' AND se.entity_id=r.entity_id CROSS JOIN catalog_entities parent ON parent.id=se.show_id
 UNION ALL SELECT r.kind,r.id,'album',parent.id,1 FROM resolved r CROSS JOIN catalog_songs x ON r.kind='item' AND x.entity_id=r.entity_id CROSS JOIN catalog_entities parent ON parent.id=x.album_id
 UNION ALL SELECT r.kind,r.id,'book',parent.id,1 FROM resolved r CROSS JOIN catalog_book_files x ON r.kind='item' AND x.entity_id=r.entity_id CROSS JOIN catalog_entities parent ON parent.id=x.book_id
 ), ranked AS (
 SELECT c.kind,c.id,a.kind target_kind,a.entity_id target_id,a.role,
 a.digest digest,
 row_number() OVER(PARTITION BY c.kind,c.id,CASE a.role WHEN 'backdrop' THEN 'backdrop' WHEN 'logo' THEN 'logo' WHEN 'still' THEN 'still' WHEN 'thumbnail' THEN 'still' ELSE 'poster' END ORDER BY c.priority,CASE a.role WHEN 'poster' THEN 0 WHEN 'still' THEN 0 ELSE 1 END,a.role) rank
 FROM candidates c CROSS JOIN artwork_selections a ON a.kind=c.target_kind AND a.entity_id=c.target_id AND a.subject='' AND a.role IN('poster','cover','portrait','square','backdrop','logo','still','thumbnail') AND (a.role NOT IN('still','thumbnail') OR c.priority=0)
 JOIN artwork_objects o ON o.digest=a.digest AND o.status='ready'
 ) SELECT r.kind,r.id,r.target_kind,pid(t.public_id),r.role,r.digest FROM ranked r CROSS JOIN catalog_entities t ON t.id=r.target_id WHERE r.rank=1`, string(raw))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var t artworkTarget
		var kind, id, role, digest string
		if err = rows.Scan(&t.Kind, &t.ID, &kind, &id, &role, &digest); err != nil {
			return nil, err
		}
		width := "400"
		if role == "backdrop" {
			width = "1920"
		} else if role == "logo" || role == "still" || role == "thumbnail" {
			width = "800"
		}
		path := "/v1/metadata/" + url.PathEscape(kind) + "/" + url.PathEscape(id) + "/art/" + role + "?v=" + url.QueryEscape(digest) + "&w=" + width
		art := out[t]
		if role == "backdrop" {
			art.BackdropURL = path
		} else if role == "still" || role == "thumbnail" {
			art.StillURL = path
		} else if role == "logo" {
			art.LogoURL = path
		} else {
			art.PosterURL = path
		}
		out[t] = art
	}
	return out, rows.Err()
}
