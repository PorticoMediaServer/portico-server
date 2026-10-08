package catalog

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const relatedCandidateLimit = 96
const relatedEntryLimit = 12

type RelatedMovieRow struct {
	ID         string `json:"id"`
	Relation   string `json:"relation"`
	Provider   string `json:"provider"`
	EvidenceID string `json:"evidenceId"`
	Heading    string `json:"heading"`
	// HeadingText is Heading as {code, params, fallback} (CON-19).
	HeadingText ServerText     `json:"headingText"`
	Entries     []ContentEntry `json:"entries"`
}
type RelatedMovies struct {
	Version     int               `json:"version"`
	LibraryID   string            `json:"libraryId"`
	LibraryName string            `json:"libraryName"`
	Rows        []RelatedMovieRow `json:"rows"`
}

// The materialized first stage enforces the candidate budget before any
// availability/identity checks or proximity sort. This is a deterministic
// shortlist, not an exhaustive best-match search through an entire library.
const relatedCandidatesSQL = `WITH shortlist AS MATERIALIZED (
 SELECT entity_id,year FROM catalog_related_facets INDEXED BY catalog_related_lookup
 WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?)
 AND relation=CASE ? WHEN 'genre' THEN 1 WHEN 'person' THEN 2 END AND provider=? AND facet_id=?
 ORDER BY year DESC,entity_id LIMIT 96
) SELECT pid(i.public_id),c.year,i.title,COALESCE(d.provider_id,'') FROM shortlist c
 JOIN catalog_entities i ON i.id=c.entity_id JOIN catalog_libraries l ON l.id=i.library_id
 LEFT JOIN metadata_details d ON d.item_id=i.id AND d.provider=?
 WHERE i.public_id<>pid_blob(?) AND l.library_id=? AND i.kind=1
 AND EXISTS(SELECT 1 FROM catalog_asset_links x JOIN catalog_assets a ON a.id=x.asset_id WHERE x.entity_id=i.id AND x.available=1)
 AND NOT EXISTS(SELECT 1 FROM catalog_entities seed JOIN catalog_asset_links original ON original.entity_id=seed.id JOIN catalog_asset_links other ON other.asset_id=original.asset_id WHERE seed.public_id=pid_blob(?) AND other.entity_id=i.id)
 AND NOT EXISTS(SELECT 1 FROM metadata_details original JOIN metadata_details other ON other.provider=original.provider AND other.provider_id=original.provider_id WHERE original.item_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND other.item_id=i.id AND original.provider_id<>'' AND original.provider IN('tmdb','tvdb','anilist','musicbrainz'))`

type relatedCandidate struct {
	id, title, providerID string
	year                  int
}

func (s *Service) movieRecommendations(profile string, item Item, genres []Genre) (*RelatedMovies, error) {
	if err := s.compactProjectionReady(21); err != nil {
		return nil, err
	}
	var libraryName string
	var kind int
	if e := s.read().QueryRow(`SELECT name,kind FROM catalog_libraries WHERE library_id=? AND retired=0`, item.LibraryID).Scan(&libraryName, &kind); e != nil {
		return nil, e
	}
	if item.Kind != "movie" || kind != 1 {
		return nil, nil
	}
	out := &RelatedMovies{Version: 1, LibraryID: item.LibraryID, LibraryName: libraryName, Rows: []RelatedMovieRow{}}
	relations := []RelatedMovieRow{}
	for _, genre := range genres {
		if len(relations) == 2 {
			break
		}
		relations = append(relations, RelatedMovieRow{ID: "genre:" + genre.Provider + ":" + genre.ID, Relation: "genre", Provider: genre.Provider, EvidenceID: genre.ID, Heading: "More " + genre.Name + " movies"})
	}
	var actorProvider, actorID, actorName string
	e := s.read().QueryRow(`SELECT c.provider,c.provider_person_id,c.credited_name
 FROM catalog_entities i JOIN catalog_credits c ON c.entity_id=i.id
 JOIN catalog_credit_labels d ON d.id=c.department_id
 WHERE i.public_id=pid_blob(?) AND c.provider_person_id<>'' AND d.label='Acting'
 ORDER BY c.source_ordinal,c.provider,c.provider_person_id LIMIT 1`, item.ID).Scan(&actorProvider, &actorID, &actorName)
	if e != nil && e != sql.ErrNoRows {
		return nil, e
	}
	if e == nil {
		relations = append(relations, RelatedMovieRow{ID: "person:" + actorProvider + ":" + actorID, Relation: "person", Provider: actorProvider, EvidenceID: actorID, Heading: "With " + actorName})
	}
	for _, relation := range relations {
		query := relatedCandidatesSQL
		args := []any{item.LibraryID, relation.Relation, relation.Provider, relation.EvidenceID, relation.Provider, item.ID, item.LibraryID, item.ID, item.ID}
		if relation.Relation == "genre" {
			name := ""
			for _, g := range genres {
				if g.Provider == relation.Provider && g.ID == relation.EvidenceID {
					name = g.Name
					break
				}
			}
			// IDs for manual and NFO genres are local tokens, not global facets.
			// Normalize their text alongside provider genres before shortlisting.
			tail := strings.SplitN(relatedCandidatesSQL, ") SELECT pid(i.public_id)", 2)[1]
			restriction, bind := ItemRestrictionSQL("i.id", s.recRestrictions)
			// The 96 visible films nearest in year: two seeks on the
			// compact folded-name order index, one upward and one downward.
			side := func(compare, order string) string {
				return `SELECT f.entity_id,f.year FROM catalog_related_facets f INDEXED BY catalog_related_lookup CROSS JOIN catalog_entities i ON i.id=f.entity_id
				 WHERE f.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND f.relation=1 AND f.provider='name' AND f.facet_id=? AND f.year` + compare + `? AND ` + recommendationVisible("i.id") + ` AND ` + restriction + `
				 ORDER BY f.year ` + order + `,f.entity_id LIMIT 96`
			}
			query = `WITH shortlist AS MATERIALIZED (SELECT entity_id,year FROM (SELECT * FROM (` + side(">=", "ASC") + `) UNION ALL SELECT * FROM (` + side("<", "DESC") + `)) ORDER BY abs(year-?),entity_id LIMIT 96) SELECT pid(i.public_id)` + tail
			key := normalizeFacetText(name)
			args = []any{item.LibraryID, key, item.Year}
			args = append(args, bind...)
			args = append(args, item.LibraryID, key, item.Year)
			args = append(args, bind...)
			args = append(args, item.Year, relation.Provider, item.ID, item.LibraryID, item.ID, item.ID)
		}
		rows, e := s.read().Query(query, args...)
		if e != nil {
			return nil, e
		}
		candidates := []relatedCandidate{}
		for rows.Next() {
			var c relatedCandidate
			if e = rows.Scan(&c.id, &c.year, &c.title, &c.providerID); e != nil {
				rows.Close()
				return nil, e
			}
			candidates = append(candidates, c)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return nil, e
		}
		if len(candidates) > relatedCandidateLimit {
			return nil, fmt.Errorf("recommendation candidate bound exceeded")
		}
		distance := func(year int) int {
			if item.Year <= 0 || year <= 0 {
				return 100000
			}
			v := year - item.Year
			if v < 0 {
				return -v
			}
			return v
		}
		sort.Slice(candidates, func(i, j int) bool {
			a, b := candidates[i], candidates[j]
			if distance(a.year) != distance(b.year) {
				return distance(a.year) < distance(b.year)
			}
			if a.year != b.year {
				return a.year > b.year
			}
			if strings.ToLower(a.title) != strings.ToLower(b.title) {
				return strings.ToLower(a.title) < strings.ToLower(b.title)
			}
			return a.id < b.id
		})
		// The candidate list is loaded in one pair of round trips rather than two
		// statements per candidate. The loop below is unchanged: same order, same
		// filtering, same limit — only the source of the rows differs, and a
		// candidate the batch did not return still falls back to the single read
		// so its error reaches the caller the way it always did.
		candidateIDs := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			candidateIDs = append(candidateIDs, candidate.id)
		}
		loaded, e := s.mediaPageIndex(profile, candidateIDs)
		if e != nil {
			return nil, e
		}
		seen := map[string]bool{}
		relation.Entries = []ContentEntry{}
		for _, candidate := range candidates {
			key := candidate.id
			if candidate.providerID != "" {
				key = relation.Provider + ":" + candidate.providerID
			}
			if seen[key] {
				continue
			}
			seen[key] = true
			target, ok := loaded[candidate.id]
			if !ok {
				var e error
				if target, e = s.Get(profile, candidate.id); e != nil {
					return nil, e
				}
			}
			if target.LibraryID != item.LibraryID || target.Kind != "movie" || !target.Available {
				continue
			}
			entry := contentItem(target)
			entry.Overview = ""
			entry.BackdropURL = ""
			entry.Playback = nil
			if target.Year > 0 {
				entry.Subtitle = strconv.Itoa(target.Year)
			}
			relation.Entries = append(relation.Entries, entry)
			if len(relation.Entries) == relatedEntryLimit {
				break
			}
		}
		if len(relation.Entries) > 0 {
			out.Rows = append(out.Rows, relation)
		}
	}
	return out, nil
}
