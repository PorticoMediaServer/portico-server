package catalog

import (
	"database/sql"
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/operations"
)

// Title-page rows on the recommendation engine: "More like X" (X's provider
// similar titles and X's own facets, ranked by the viewer's taste when signed
// in), "Starring" its top-billed actor, "From" a show's creator, and "Viewers
// also watched" (opted-in members who finished X, when the owner allows
// community activity). Each is a ranking over the same bounded reads as Home.

const (
	recTitleFacets  = 12 // X's strongest facets read for More like X
	recTitleHead    = 100
	recCoViewersMin = 3  // distinct other viewers before Viewers also watched shows
	recCoRecent     = 50 // each co-viewer's recent finishes read
	recCoViewers    = 50 // the most recent finishers of a title read
)

// recTitleSession is a session whose "taste" is the title's own facets
// (rarity weighting does the rest), with the viewer's taste as a lighter
// second voice, and the title as the only similar-title seed.
func (s *Service) recTitleSession(base *recSession, work int64) (*recSession, error) {
	copied := *base
	x := &copied
	// The title replaces taste and seeds, but immutable catalogue inputs can
	// be shared with the viewer's rows inside this same read snapshot. Edges
	// are keyed by exact seeds/weights and each session keeps its own IDF map.
	if base.hydration == nil || base.hydration.snapshot == nil || base.hydration.snapshot != dbwork.Snapshot(s.Context()) {
		x.hydration = &recHydration{snapshot: dbwork.Snapshot(s.Context())}
	}
	x.taste.seeds = nil
	x.idf = map[string]float64{}
	x.strongest = nil
	facets, err := compactcatalog.RecWorkFacets(s.Context(), s.read(), work)
	if err != nil {
		return nil, err
	}
	personal := x.taste.values
	norm := 0.0
	for _, v := range personal {
		if v > 0 {
			norm = max(norm, v)
		}
	}
	values := map[string]float64{}
	for _, f := range facets {
		values[f] = 1
	}
	if norm > 0 {
		for f, v := range personal {
			values[f] += 0.3 * v / norm
		}
	}
	x.taste.values = values
	x.taste.seeds = []recSeed{{work, 1}}
	keys := make([]string, 0, len(values))
	for f := range values {
		if recFacetWeight(f) > 0 {
			keys = append(keys, f)
		}
	}
	if x.idf, err = x.recRarity(keys); err != nil {
		return nil, err
	}
	for _, f := range facets {
		if recFacetWeight(f) > 0 && !strings.HasPrefix(f, "e:") {
			x.strongest = append(x.strongest, f)
		}
	}
	sort.Slice(x.strongest, func(i, j int) bool {
		if a, b := x.strength(x.strongest[i]), x.strength(x.strongest[j]); a != b {
			return a > b
		}
		return x.strongest[i] < x.strongest[j]
	})
	if len(x.strongest) > recTitleFacets {
		x.strongest = x.strongest[:recTitleFacets]
	}
	x.key += "\x00title:" + itoa(work)
	return x, nil
}

// recMoreLike ranks the titles most like work for the viewer. Titles the
// viewer has already seen stay (they are like it) but follow the rest.
func (s *Service) recMoreLike(base *recSession, work int64) ([]recCandidate, error) {
	x, err := s.recTitleSession(base, work)
	if err != nil {
		return nil, err
	}
	ranked, err := x.memo("more_like", func() ([]recCandidate, error) {
		return x.rank(recOptions{head: recTitleHead, noFill: true, includeEngaged: true, diversify: true,
			keep: func(c *recScored) bool { return c.work != work }})
	})
	if err != nil {
		return nil, err
	}
	if err := s.recLoadSignals(x.r.Profile, &x.taste, recCandidateWorks(ranked)); err != nil {
		return nil, err
	}
	engaged := x.taste.engaged
	sort.SliceStable(ranked, func(i, j int) bool {
		a, _ := strconv.ParseInt(ranked[i].Work, 10, 64)
		b, _ := strconv.ParseInt(ranked[j].Work, 10, 64)
		return !engaged[a] && engaged[b]
	})
	return ranked, nil
}

// recFacetTitles ranks one facet's titles (a person's, a creator's) for the
// viewer, the title itself left out.
func (s *Service) recFacetTitles(x *recSession, work int64, facet string) ([]recCandidate, error) {
	if _, ok := x.idf[facet]; !ok {
		more, err := x.recRarity([]string{facet})
		if err != nil {
			return nil, err
		}
		x.idf[facet] = more[facet]
	}
	o := x.facetRow(facet)
	keep := o.keep
	o.keep = func(c *recScored) bool { return c.work != work && keep(c) }
	o.includeEngaged = true
	return x.memo("facet:"+facet+":"+itoa(work), func() ([]recCandidate, error) { return x.rank(o) })
}

// recViewersAlsoWatched is what other members who finished work also
// finished: only members who share their activity (the owner allows community
// activity, the member shows it and hasn't paused history), and only when at
// least recCoViewersMin of them finished it.
func (s *Service) recViewersAlsoWatched(x *recSession, work int64, now time.Time) ([]recCandidate, error) {
	// Finishers only (a watchlist or a like isn't watching it), the most
	// recent recCoViewers of them: a ranking, and a bounded read on a popular
	// title of a large server.
	rows, err := s.read().Query(`SELECT profile_id FROM rec_profile_signals INDEXED BY rec_profile_signals_work WHERE work_id=? AND finished=1 AND profile_id<>? ORDER BY at DESC LIMIT ?`, work, x.r.Profile, recCoViewers)
	if err != nil {
		return nil, err
	}
	var profiles []string
	for rows.Next() {
		var p string
		if err = rows.Scan(&p); err != nil {
			rows.Close()
			return nil, err
		}
		profiles = append(profiles, p)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, err
	}
	var sharing []string
	for _, p := range profiles {
		ok, err := s.profileSharesActivity(p, now)
		if err != nil {
			return nil, err
		}
		if ok {
			sharing = append(sharing, p)
		}
	}
	if len(sharing) < recCoViewersMin {
		return nil, nil
	}
	counts := map[int64]int{}
	for _, p := range sharing {
		works, err := scanInt64s(s.read().Query(`SELECT work_id FROM rec_profile_signals INDEXED BY rec_profile_signals_recent WHERE profile_id=? AND finished=1 AND hidden=0 AND work_id<>? ORDER BY at DESC LIMIT ?`, p, work, recCoRecent))
		if err != nil {
			return nil, err
		}
		for _, w := range works {
			counts[w]++
		}
	}
	var candidates []int64
	for w, n := range counts {
		if n >= 2 {
			candidates = append(candidates, w)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	ranked, err := x.rank(recOptions{facets: []string{}, works: candidates, noFill: true, noSimilar: true, anyMatch: true, includeEngaged: true})
	if err != nil {
		return nil, err
	}
	// Order by how many of them went on to it, then by the viewer's taste.
	sort.SliceStable(ranked, func(i, j int) bool {
		a, _ := strconv.ParseInt(ranked[i].Work, 10, 64)
		b, _ := strconv.ParseInt(ranked[j].Work, 10, 64)
		return counts[a] > counts[b]
	})
	return ranked, nil
}

// profileSharesActivity reports whether a profile (a personal key) is an
// active member who shows activity to members and hasn't paused history.
func (s *Service) profileSharesActivity(key string, now time.Time) (bool, error) {
	v, ok := viewerFromPersonalKey(key)
	if !ok {
		return false, nil
	}
	var active bool
	var err error
	switch v.Authority {
	case "local":
		err = s.read().QueryRow(`SELECT EXISTS(SELECT 1 FROM direct_profiles p JOIN direct_memberships m ON m.account_id=p.account_id WHERE p.id=? AND p.account_id=? AND p.deleted=0 AND m.disabled=0)`, v.ProfileID, v.AccountID).Scan(&active)
	case "hosted":
		err = s.read().QueryRow(`SELECT EXISTS(SELECT 1 FROM authorization_session_families WHERE authority='hosted' AND account_id=? AND profile_id=? AND revoked=0 AND julianday(authorization_horizon)>julianday(?))`, v.AccountID, v.ProfileID, now.UTC().Format(time.RFC3339)).Scan(&active)
	default:
		return false, nil
	}
	if err != nil || !active {
		return false, err
	}
	prefs, _, _, err := operations.EffectivePreferences(s.read(), v, "")
	if err != nil {
		return false, err
	}
	return prefs.Bool("privacy.showActivityToMembers") && !prefs.Bool("privacy.pauseWatchHistory"), nil
}

// recTitleWork is the work a title page is about: a film, or an episode's
// show; with its library and title.
func (s *Service) recTitleWork(item Item) (work int64, title string, err error) {
	err = s.read().QueryRow(`SELECT COALESCE((SELECT show_id FROM catalog_episodes WHERE entity_id=i.id),i.id) FROM catalog_entities i WHERE i.public_id=pid_blob(?)`, item.ID).Scan(&work)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", nil
	}
	if err != nil {
		return 0, "", err
	}
	err = s.read().QueryRow(`SELECT title FROM catalog_entities WHERE id=?`, work).Scan(&title)
	return work, title, err
}

// recTitleRelations are the engine's rows for a title page, as relations.
func (s *Service) recTitleRelations(viewer Viewer, item Item, community bool) ([]recommendationRelation, error) {
	work, title, err := s.recTitleWork(item)
	if err != nil || work == 0 {
		return nil, err
	}
	var out []recommendationRelation
	add := func(relation, provider, evidence, heading string, ranked []recCandidate) {
		ids := make([]string, 0, itemRecommendationRowLimit)
		for i := 0; i < len(ranked) && i < itemRecommendationRowLimit; i++ {
			ids = append(ids, ranked[i].ID)
		}
		if len(ids) > 0 {
			out = append(out, recommendationRelation{relation, provider, evidence, heading, `SELECT value FROM json_each(?)`, []any{idsJSON(ids)}})
		}
	}
	// One session (the viewer's taste, libraries, rarity) serves every row.
	base, err := s.recSession(HomeRequest{Viewer: viewer, Profile: viewer.Profile, ViewerFence: viewer.Fence, Libraries: []string{item.LibraryID}, Restrictions: s.recRestrictions})
	if err != nil {
		return nil, err
	}
	more, err := s.recMoreLike(base, work)
	if err != nil {
		return nil, err
	}
	add("more_like", "local", item.ID, "More like "+title, more)
	facets, err := compactcatalog.RecWorkFacets(s.Context(), s.read(), work)
	if err != nil {
		return nil, err
	}
	for _, f := range facets {
		if strings.HasPrefix(f, "cd:creator:") {
			ranked, err := s.recFacetTitles(base, work, f)
			if err != nil {
				return nil, err
			}
			name := recTitleCase(strings.TrimPrefix(f, "cd:creator:"))
			add("creator", "local", f, "From "+name, ranked)
			break
		}
	}
	if item.Kind == "movie" {
		var provider, person, name string
		err := s.read().QueryRow(`SELECT c.provider,c.provider_person_id,c.credited_name FROM catalog_entities i JOIN catalog_credits c ON c.entity_id=i.id
		 JOIN catalog_credit_labels d ON d.id=c.department_id WHERE i.id=? AND d.label='Acting' AND c.provider_person_id<>'' ORDER BY c.source_ordinal LIMIT 1`, work).Scan(&provider, &person, &name)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		if err == nil {
			ranked, err := s.recFacetTitles(base, work, "p:"+provider+":"+person)
			if err != nil {
				return nil, err
			}
			add("starring", provider, person, "Starring "+name, ranked)
		}
	}
	if community {
		ranked, err := s.recViewersAlsoWatched(base, work, s.recommendationNow(time.Time{}))
		if err != nil {
			return nil, err
		}
		add("viewers_also_watched", "local", item.ID, "Viewers also watched", ranked)
	}
	return out, nil
}
