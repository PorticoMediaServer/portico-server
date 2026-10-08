package catalog

import (
	"context"
	"database/sql"
	"encoding/json"
	"portico.local/server/internal/dbwork"
	"sort"
	"strings"
	"time"

	"portico.local/server/internal/identity"
)

type HomeRequest struct {
	Viewer                         Viewer
	ServerID, Profile, ViewerFence string
	Libraries                      []string
	RowOrder                       []string
	HiddenRowIDs                   []string
	LayoutRevision                 int64
	CommunityActivity              bool
	Limit                          int
	Now                            time.Time
	// Restrictions is applied to every row by homeSource; see restrictions.go.
	Restrictions identity.ContentRestrictions
}

func (r HomeRequest) scoped() HomeRequest {
	r.Profile, r.ViewerFence, r.Libraries, r.Restrictions = r.Viewer.Profile, r.Viewer.Fence, r.Viewer.Libraries, r.Viewer.EffectiveRestrictions()
	return r
}

type homeCandidate struct{ id, order string }

// HomeRevision is the catalogue and viewer revision the home surface is
// composed against. It is exported so the HTTP layer can fold it into a response
// validator and answer a conditional request before composing anything.
func (s *Service) HomeRevision(libraries []string, profile string) (ContentRevision, error) {
	return s.homeRevision(homeUnique(libraries), profile)
}

func (s *Service) homeRevision(libraries []string, profile string) (ContentRevision, error) {
	raw, _ := json.Marshal(libraries)
	var revision ContentRevision
	e := s.read().QueryRow(`SELECT COALESCE(sum(r.revision),0),COALESCE(sum(v.revision),0) FROM library_revisions r LEFT JOIN viewer_revisions v ON v.library_id=r.library_id AND v.profile_id=? WHERE r.library_id IN(SELECT value FROM json_each(?))`, profile, string(raw)).Scan(&revision.Catalog, &revision.Viewer)
	return revision, e
}
func mergeHome(a, b []homeCandidate, limit int) []homeCandidate {
	a = append(a, b...)
	sort.Slice(a, func(i, j int) bool {
		if a[i].order != a[j].order {
			return a[i].order > a[j].order
		}
		return a[i].id > a[j].id
	})
	if len(a) > limit {
		a = a[:limit]
	}
	return a
}
func (s *Service) homeCandidates(query string, args ...any) ([]homeCandidate, error) {
	rows, e := s.read().Query(query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []homeCandidate{}
	for rows.Next() {
		var row homeCandidate
		if e = rows.Scan(&row.id, &row.order); e != nil {
			return nil, e
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
func homeContinueSQL(mode string) string {
	kind := `k.playable=1 AND k.listening=0 AND i.kind<>11`
	if mode == "continue_listening" {
		kind = `(k.playable=1 AND k.listening=1 AND (i.kind<>9 OR EXISTS(SELECT 1 FROM catalog_book_files f JOIN book_resume b ON b.book_id=f.book_id AND b.profile_id=a.profile_id WHERE f.entity_id=i.id AND b.item_id=i.id)))`
	}
	// Progress positions are milliseconds (unit 0); asset durations are seconds.
	return ` FROM progress_activity a JOIN progress p ON p.profile_id=a.profile_id AND p.item_id=a.item_id JOIN catalog_entities i ON i.id=a.item_id JOIN catalog_kinds k ON k.id=i.kind WHERE a.profile_id=? AND a.library_id=? AND i.retired=0 AND EXISTS(SELECT 1 FROM catalog_libraries cl WHERE cl.id=i.library_id AND cl.library_id=a.library_id) AND ` + kind + ` AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=i.id) AND a.state!='ended' AND NOT EXISTS(SELECT 1 FROM continue_dismissals d WHERE d.profile_id=p.profile_id AND d.item_id=p.item_id AND d.playback_id=p.playback_id) AND p.position>0 AND p.position<(SELECT max(s.duration)*1000-CASE WHEN i.kind=9 THEN 0 ELSE 3000 END FROM catalog_asset_links l JOIN catalog_assets s ON s.id=l.asset_id WHERE l.entity_id=i.id AND s.available=1 AND l.available=1)`
}
func (s *Service) homeContinueCount(library, profile, mode string) (int, error) {
	revision, e := s.ContentRevision(library, profile)
	if e != nil {
		return 0, e
	}
	key := countKey{library, profile, revision, mode}
	if cached, ok := s.cachedCount(key); ok {
		return cached, nil
	}
	var n int
	e = s.read().QueryRow(`SELECT count(*)`+homeContinueSQL(mode), profile, library).Scan(&n)
	if e != nil {
		return 0, e
	}
	after, e := s.ContentRevision(library, profile)
	if e != nil {
		return 0, e
	}
	if after != revision {
		return 0, ErrStaleContinuation
	}
	s.storeCount(key, n)
	return n, nil
}
func (s *Service) homeOnce(r HomeRequest) (ContentEnvelope, error) {
	unique := map[string]bool{}
	ids := []string{}
	for _, id := range r.Libraries {
		if !unique[id] {
			ids = append(ids, id)
			unique[id] = true
		}
	}
	sort.Strings(ids)
	r.Libraries = ids
	out := ContentEnvelope{Heading: ContentHeading{Key: "home.title", Fallback: "Home"}, Scope: ContentScope{r.ServerID, "", "mixed", "home", "", r.ViewerFence}, Navigation: []ContentTab{}, Query: ContentQuery{"server", "asc", "", "", 12, "none"}, Sorts: []ContentSort{}, Filters: []ContentFilter{}, Sections: []ContentSection{}}
	before, e := s.homeRevision(r.Libraries, r.Profile)
	if e != nil {
		return out, e
	}
	out.Revision = before
	selected := map[string][]homeCandidate{"continue_watching": {}, "continue_listening": {}, "recently_added": {}}
	counts := map[string]int{}
	// One query for every library's kind, not one per library inside the loop:
	// homeLibraries already reads exactly this.
	kinds := map[string]string{}
	known, e := s.homeLibraries(r.Libraries)
	if e != nil {
		return out, e
	}
	for _, library := range known {
		kinds[library.ID] = library.Kind
	}
	// SEC-02: every candidate query carries the viewer's content restriction,
	// and a restricted viewer's section totals count only what it may see.
	restricted := r.Restrictions.Active()
	visible := func(item string) (string, []any) {
		clause, args := ItemRestrictionSQL(item, r.Restrictions)
		return ` AND ` + clause + recordingsClause(item, r.Restrictions), args
	}
	for _, library := range r.Libraries {
		kind, ok := kinds[library]
		if !ok {
			return out, sql.ErrNoRows
		}
		modes := []string{"continue_watching"}
		if kind == "music" || kind == "audiobook" {
			modes = []string{"continue_listening"}
		}
		for _, mode := range modes {
			clause, clauseArgs := visible("i.id")
			rows, e := s.homeCandidates(`SELECT pid(i.public_id),a.updated_at`+homeContinueSQL(mode)+clause+` ORDER BY a.updated_at DESC,a.item_id DESC LIMIT 12`, append([]any{r.Profile, library}, clauseArgs...)...)
			if e != nil {
				return out, e
			}
			selected[mode] = mergeHome(selected[mode], rows, 12)
			var n int
			if restricted {
				// One person's in-progress set: small, and not cacheable across
				// restriction changes.
				e = s.read().QueryRow(`SELECT count(*)`+homeContinueSQL(mode)+clause, append([]any{r.Profile, library}, clauseArgs...)...).Scan(&n)
			} else {
				n, e = s.homeContinueCount(library, r.Profile, mode)
			}
			if e != nil {
				return out, e
			}
			counts[mode] += n
		}
		clause, clauseArgs := visible("br.entity_id")
		class, e := s.homeClass(library, r)
		if e != nil {
			return out, e
		}
		// The newest dated items: a restricted viewer's walk reads its class's
		// rows, newest first per kind and merged, so it reads only what it may
		// see (rechecked against the current restriction) however little that
		// is; everyone else walks the library's index.
		var rows []homeCandidate
		if class != nil {
			arms, armArgs := s.homeClassArms(class, library, clause, clauseArgs, `ce.retired=0 AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=br.entity_id) AND EXISTS(SELECT 1 FROM catalog_browse_rows row WHERE row.entity_id=br.entity_id AND row.item_id IS NOT NULL)`, class.kinds)
			rows, e = s.homeCandidates(`SELECT pid(ce.public_id),added FROM (`+arms+` ORDER BY 2 DESC,1 DESC LIMIT 12) page CROSS JOIN catalog_entities ce ON ce.id=page.entity_id ORDER BY added DESC,page.entity_id DESC`, armArgs...)
		} else {
			rows, e = s.homeCandidates(`SELECT pid(ce.public_id),COALESCE(br.added_text,'') FROM catalog_browse_rows br INDEXED BY catalog_browse_recent_dated
		 CROSS JOIN catalog_entities ce ON ce.id=br.entity_id WHERE br.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND br.item_id IS NOT NULL AND br.added_text IS NOT NULL
		 AND ce.retired=0 AND NOT EXISTS(SELECT 1 FROM dvr_catalog_retirements gone WHERE gone.item_id=br.entity_id)`+clause+` ORDER BY COALESCE(br.added_text,'') DESC,br.entity_id DESC LIMIT 12`, append([]any{library}, clauseArgs...)...)
		}
		if e != nil {
			return out, e
		}
		selected["recently_added"] = mergeHome(selected["recently_added"], rows, 12)
		var n int
		if restricted {
			n, e = s.homeRecentTotal(library, r.Restrictions)
		} else {
			e = s.read().QueryRow(`SELECT COALESCE(sum(total),0) FROM catalog_home_buckets WHERE library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND dated=1 AND retired=0`, library).Scan(&n)
		}
		if e != nil {
			return out, e
		}
		counts["recently_added"] += n
	}
	selectedIDs := []string{}
	seenIDs := map[string]bool{}
	for _, mode := range []string{"continue_watching", "continue_listening", "recently_added"} {
		for _, candidate := range selected[mode] {
			if !seenIDs[candidate.id] {
				selectedIDs = append(selectedIDs, candidate.id)
				seenIDs[candidate.id] = true
			}
		}
	}
	items, e := s.mediaPage(r.Profile, selectedIDs, false)
	if e != nil {
		return out, e
	}
	loaded := map[string]ContentEntry{}
	for _, item := range items {
		row := contentItem(item)
		if !item.Available {
			row.Playback = nil
		}
		loaded[item.ID] = row
	}
	entry := func(id string) (ContentEntry, error) {
		row, ok := loaded[id]
		if !ok {
			return ContentEntry{}, ErrStaleContinuation
		}
		return row, nil
	}

	for _, section := range []struct{ id, title string }{{"continue_watching", "Continue Watching"}, {"continue_listening", "Continue Listening"}, {"recently_added", "Recently Added"}} {
		if len(selected[section.id]) == 0 {
			continue
		}
		entries := []ContentEntry{}
		for _, candidate := range selected[section.id] {
			row, e := entry(candidate.id)
			if e != nil {
				return out, e
			}
			entries = append(entries, row)
		}
		out.Sections = append(out.Sections, contentSection(section.id, "rail", section.title, entries, counts[section.id], ""))
	}
	after, e := s.homeRevision(r.Libraries, r.Profile)
	if e != nil {
		return out, e
	}
	if before != after {
		return out, ErrStaleContinuation
	}
	if len(out.Sections) == 0 {
		out.Empty = &ContentHeading{Key: "home.empty", Fallback: "No content is available in your libraries yet."}
	}
	return out, nil
}

// Home has no continuation, so a read that raced a publication simply retries.
func (s *Service) Home(r HomeRequest) (ContentEnvelope, error) {
	if dbwork.Snapshot(s.Context()) == nil {
		var out ContentEnvelope
		err := dbwork.WithReadSnapshot(s.Context(), s.db, func(ctx context.Context) error {
			var readErr error
			out, readErr = s.WithContext(ctx).Home(r)
			return readErr
		})
		return out, err
	}
	if err := s.compactProjectionReady(18, 20); err != nil {
		return ContentEnvelope{}, err
	}
	r = r.scoped()
	if err := s.prepareViewer(r.Viewer); err != nil {
		return ContentEnvelope{}, err
	}
	return consistentRead(true, func() (ContentEnvelope, error) { return s.homeOnce(r) })
}

// homeClassWalk is a restricted viewer's published class and the playable
// kinds its dated items can be.
type homeClassWalk struct {
	id, generation int64
	kinds          []int
}

func (s *Service) homeClass(library string, r HomeRequest) (*homeClassWalk, error) {
	if !r.Restrictions.Active() {
		return nil, nil
	}
	key, generation, ready, err := s.publishedVisibilityClass(library, r.Restrictions)
	if err != nil || !ready {
		return nil, err
	}
	out := &homeClassWalk{generation: generation}
	if err = s.read().QueryRow(`SELECT id FROM compact_visibility_classes WHERE class_key=?`, key).Scan(&out.id); err != nil {
		return nil, err
	}
	rows, err := s.read().Query(`SELECT id FROM catalog_kinds WHERE playable=1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind int
		if err = rows.Scan(&kind); err != nil {
			return nil, err
		}
		out.kinds = append(out.kinds, kind)
	}
	return out, rows.Err()
}

// homeClassArms is one index-ordered arm per kind over the class's dated rows,
// newest first (compact_visibility_added); a compound ORDER BY merges them
// and stops at its limit. filter and clause apply to br (the class row) and ce.
func (s *Service) homeClassArms(class *homeClassWalk, library, clause string, clauseArgs []any, filter string, kinds []int) (string, []any) {
	arms := []string{}
	args := []any{}
	for _, kind := range kinds {
		arms = append(arms, `SELECT br.entity_id,br.added FROM compact_visibility_rows br INDEXED BY compact_visibility_added CROSS JOIN catalog_entities ce ON ce.id=br.entity_id
		 WHERE br.class_id=? AND br.generation=? AND br.library_id=(SELECT id FROM catalog_libraries WHERE library_id=?) AND br.kind=? AND br.added<>'' AND `+filter+clause)
		args = append(append(args, class.id, class.generation, library, kind), clauseArgs...)
	}
	return strings.Join(arms, " UNION ALL "), args
}
