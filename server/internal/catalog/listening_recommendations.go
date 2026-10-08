package catalog

import "strings"

// Listening relations reuse the same permitted work/facet projection as Home.
// A song or part is evidence for its album/book, never a recommendation card.
func (s *Service) listeningRecommendations(profile string, item Item) (*RelatedMovies, error) {
	name, err := s.recommendationLibrary(item.LibraryID)
	if err != nil {
		return nil, err
	}
	out := &RelatedMovies{Version: 1, LibraryID: item.LibraryID, LibraryName: name, Rows: []RelatedMovieRow{}}
	r := HomeRequest{Profile: profile, Libraries: []string{item.LibraryID}, Restrictions: s.recRestrictions}
	candidates, err := s.recommendationCandidates(r, "related", item.ID)
	if err != nil {
		return nil, err
	}
	// Only the seed supplies the headings; candidate facets were already scored.
	base, args := recBase(r, []string{item.ID})
	args = append(args, item.ID)
	rows, err := s.read().Query(base+` SELECT f.f FROM facets f JOIN members m ON m.work=f.work WHERE m.public_id=pid_blob(?) ORDER BY f.f`, args...)
	if err != nil {
		return nil, err
	}
	facets := []string{}
	for rows.Next() {
		var f string
		if err = rows.Scan(&f); err != nil {
			rows.Close()
			return nil, err
		}
		facets = append(facets, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, f := range facets {
		prefix, value, _ := strings.Cut(f, ":")
		relation, heading := "", ""
		switch prefix {
		case "a":
			relation = "artist"
			var artist string
			if err = s.read().QueryRow(`SELECT e.title FROM catalog_entities e JOIN catalog_artists a ON a.entity_id=e.id WHERE e.public_id=pid_blob(?) AND e.retired=0`, value).Scan(&artist); err != nil {
				return nil, err
			}
			heading = "More releases by " + artist
		case "b":
			relation = "author"
			heading = "More by " + value
		case "g":
			relation = "genre"
			heading = "More " + value
		case "k":
			relation = "book"
			heading = "More from " + value
		default:
			continue
		}
		ids := []string{}
		for _, c := range candidates {
			if containsString(c.Facets, f) {
				ids = append(ids, c.ID)
				if len(ids) == 12 {
					break
				}
			}
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
		}
		out.Rows = append(out.Rows, RelatedMovieRow{ID: relation + ":local:" + value, Relation: relation, Provider: "local", EvidenceID: value, Heading: heading, Entries: entries})
		if len(out.Rows) == 3 {
			break
		}
	}
	return out, nil
}
