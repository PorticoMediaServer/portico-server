package catalog

import "portico.local/server/internal/compactcatalog"

// recHydration retains immutable source data for this recSession's read
// snapshot. Candidate selection, profile signals, scoring and eligibility are
// still performed independently for each row. Nothing survives the request.
type recHydration struct {
	works         map[int64]*recWorkMetadata
	postings      map[recPostingKey][]recPosting
	similar       []recPosting
	similarLoaded bool
}

type recWorkMetadata struct {
	quality    float64
	hasQuality bool
	votes      int64
	facets     []string
}

type recPostingKey struct {
	facet   string
	library int64
	limit   int
}

type recPosting struct {
	work  int64
	value float64
}

func (x *recSession) recPostings(facet string, library int64, limit int, each func(int64, float64)) error {
	key := recPostingKey{facet, library, limit}
	if x.hydration.postings == nil {
		x.hydration.postings = map[recPostingKey][]recPosting{}
	}
	items, loaded := x.hydration.postings[key]
	if !loaded {
		if err := x.s.recPostings(facet, library, limit, func(work int64, quality float64) {
			items = append(items, recPosting{work, quality})
		}); err != nil {
			return err
		}
		x.hydration.postings[key] = items
	}
	for _, item := range items {
		each(item.work, item.value)
	}
	return nil
}

func (x *recSession) recSimilar(each func(int64, float64)) error {
	if !x.hydration.similarLoaded {
		var items []recPosting
		if err := x.s.recSimilar(x.taste.seeds, func(work int64, weight float64) {
			items = append(items, recPosting{work, weight})
		}); err != nil {
			return err
		}
		x.hydration.similar, x.hydration.similarLoaded = items, true
	}
	for _, item := range x.hydration.similar {
		each(item.work, item.value)
	}
	return nil
}

func (x *recSession) hydrateScored(scored []*recScored) error {
	if x.hydration.works == nil {
		x.hydration.works = map[int64]*recWorkMetadata{}
	}
	missing := map[int64]*recWorkMetadata{}
	var ids []int64
	for _, candidate := range scored {
		if _, loaded := x.hydration.works[candidate.work]; !loaded && missing[candidate.work] == nil {
			missing[candidate.work] = &recWorkMetadata{}
			ids = append(ids, candidate.work)
		}
	}
	if len(ids) > 0 {
		raw := idsJSON64(ids)
		votes, err := x.s.read().Query(`SELECT j.value,max(r.votes) FROM json_each(?) j CROSS JOIN metadata_ratings r ON r.item_id=j.value GROUP BY j.value`, raw)
		if err != nil {
			return err
		}
		for votes.Next() {
			var work, n int64
			if err = votes.Scan(&work, &n); err != nil {
				votes.Close()
				return err
			}
			missing[work].votes = n
		}
		votes.Close()
		if err = votes.Err(); err != nil {
			return err
		}
		rows, err := x.s.read().Query(`SELECT p.entity_id,p.facet,p.quality FROM json_each(?) j CROSS JOIN catalog_rec_postings p INDEXED BY catalog_rec_postings_entity ON p.entity_id=j.value`, raw)
		if err != nil {
			return err
		}
		for rows.Next() {
			var work int64
			var facet string
			var quality float64
			if err = rows.Scan(&work, &facet, &quality); err != nil {
				rows.Close()
				return err
			}
			data := missing[work]
			data.quality, data.hasQuality = quality, true
			if facet != compactcatalog.RecAllFacet {
				data.facets = append(data.facets, facet)
			}
		}
		rows.Close()
		if err = rows.Err(); err != nil {
			return err
		}
	}
	// Rarity depends on each candidate facet, not on how many candidates share
	// it. The same genre appearing on 400 works is one lookup key.
	unknown := map[string]bool{}
	for _, data := range missing {
		for _, facet := range data.facets {
			if _, loaded := x.idf[facet]; !loaded {
				unknown[facet] = true
			}
		}
	}
	if len(unknown) > 0 {
		keys := make([]string, 0, len(unknown))
		for facet := range unknown {
			keys = append(keys, facet)
		}
		more, err := x.s.recRarity(keys)
		if err != nil {
			return err
		}
		for facet, value := range more {
			x.idf[facet] = value
		}
	}
	for work, data := range missing {
		x.hydration.works[work] = data
	}
	for _, candidate := range scored {
		data := x.hydration.works[candidate.work]
		candidate.votes, candidate.facets = data.votes, data.facets
		if data.hasQuality {
			candidate.quality = data.quality
		}
	}
	return nil
}
