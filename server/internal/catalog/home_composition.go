package catalog

import (
	"database/sql"
	"errors"
	"sort"
	"strings"
	"time"

	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/personalstate"
)

// HomeReason records the actual source of a selection, separately from its
// localized wording. Source is absent for non-causal recommendations.
type HomeReason struct {
	Kind      string            `json:"kind"`
	Text      ServerText        `json:"text"`
	Source    *ContentEntityRef `json:"source,omitempty"`
	LibraryID string            `json:"libraryId,omitempty"`
}
type HomeSpotlight struct {
	Entry  ContentEntry `json:"entry"`
	Reason HomeReason   `json:"reason"`
	// ResumeTarget is a show's actual next episode, including its own position.
	ResumeTarget *ContentEntry `json:"resumeTarget,omitempty"`
}
type HomeResume struct {
	EntryID string       `json:"entryId"`
	Target  ContentEntry `json:"target"`
}
type HomePeriod struct {
	Days   int        `json:"days"`
	Start  string     `json:"start"`
	End    string     `json:"end"`
	Metric string     `json:"metric"`
	Text   ServerText `json:"text"`
}
type HomeCollection struct {
	Collection  Collection     `json:"collection"`
	Members     []ContentEntry `json:"members"`
	BackdropURL string         `json:"backdropUrl,omitempty"`
	// These facts describe the whole collection only when its complete visible
	// membership fits the preview. Otherwise the exact count stands alone.
	FirstYear *int   `json:"firstYear,omitempty"`
	LastTitle string `json:"lastTitle,omitempty"`
}
type HomeChannel struct {
	Channel livechannels.Channel     `json:"channel"`
	Source  livechannels.GuideSource `json:"source"`
}
type HomeLive struct {
	Revision   string                         `json:"revision"`
	ObservedAt string                         `json:"observedAt"`
	ExpiresAt  string                         `json:"expiresAt"`
	Sort       string                         `json:"sort"`
	Group      string                         `json:"group"`
	Groups     []livechannels.GuideGroupCount `json:"groups"`
	Channels   []HomeChannel                  `json:"channels"`
}

const homeNewWindow = 14 * 24 * time.Hour
const homeCompositionMaximum = 100

// homeMergedRecent merges maintained, already restricted recent-work walks.
// Each library costs <=4096 candidates, each contributes <=100 distinct works;
// the Home row itself is exactly the newest 100 of that bounded union.
func (s *Service) homeMergedRecent(r HomeRequest, audio bool) ([][2]string, error) {
	libraries, err := s.homeLibraries(r.Libraries)
	if err != nil {
		return nil, err
	}
	all := [][2]string{}
	for _, library := range libraries {
		isAudio := library.Kind == "music" || library.Kind == "audiobook"
		if !audio && library.Kind != "movie" && library.Kind != "tv" && library.Kind != "anime" {
			continue
		}
		if isAudio != audio {
			continue
		}
		values, err := s.homeRecentWorks(r, library.ID)
		if err != nil {
			return nil, err
		}
		all = append(all, values...)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i][1] != all[j][1] {
			return all[i][1] > all[j][1]
		}
		return all[i][0] > all[j][0]
	})
	out := make([][2]string, 0, min(len(all), homeCompositionMaximum))
	seen := map[string]bool{}
	for _, v := range all {
		if !seen[v[0]] {
			seen[v[0]] = true
			out = append(out, v)
		}
		if len(out) == homeCompositionMaximum {
			break
		}
	}
	return out, nil
}

// The newest completed, still-accessible video among the last 64 history
// events is the causal anchor. A watched mark alone never claims a viewing.
func (s *Service) homeBecause(r HomeRequest) (*ContentEntityRef, []recCandidate, error) {
	restriction, args := r.Viewer.itemVisibilitySQL("i.id")
	bind := append([]any{r.Profile, idsJSON(r.Libraries), r.now().UTC().Format(time.RFC3339Nano)}, args...)
	rows, err := s.read().Query(`SELECT pid(i.public_id),i.title FROM
 (SELECT item_id,completed,updated_at,id FROM personal_history INDEXED BY personal_history_page WHERE profile_id=? ORDER BY updated_at DESC,id DESC LIMIT 64) h
 CROSS JOIN catalog_entities i ON i.id=h.item_id JOIN catalog_libraries l ON l.id=i.library_id
 WHERE h.completed=1 AND i.kind IN(1,4) AND l.library_id IN(SELECT value FROM json_each(?)) AND julianday(h.updated_at)<=julianday(?) AND `+homeVisible("i.id")+` AND `+restriction+` ORDER BY h.updated_at DESC,h.id DESC LIMIT 1`, bind...)
	if err != nil {
		return nil, nil, err
	}
	var source ContentEntityRef
	if rows.Next() {
		err = rows.Scan(&source.ID, &source.Name)
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return nil, nil, err
	}
	if source.ID == "" {
		return nil, []recCandidate{}, nil
	}
	ranked, err := s.recommendationCandidates(r, "related", source.ID)
	if err != nil {
		return nil, nil, err
	}
	out := []recCandidate{}
	for _, c := range ranked {
		if c.Kind == "movie" || c.Kind == "show" {
			out = append(out, c)
		}
		if len(out) == homeCompositionMaximum {
			break
		}
	}
	return &source, out, nil
}

func (s *Service) homeAudioSource(r HomeRequest, booksOnly bool) (homeSource, error) {
	// Preserve the existing bounded continuation membership and ordering; group
	// its files into the real album/book containers before selecting additions.
	base := homeContinueBase("continue_listening")
	restriction, args := r.Viewer.itemVisibilitySQL("candidate.entity")
	if booksOnly {
		restriction = "(" + restriction + ") AND i.kind=9"
	}
	bind := append([]any{r.Profile, r.Profile, idsJSON(r.Libraries)}, args...)
	rows, err := s.read().Query(`SELECT candidate.id,candidate.ord,CASE i.kind WHEN 9 THEN pid(book.public_id) WHEN 7 THEN pid(album.public_id) END,i.kind
 FROM (`+base+`) candidate CROSS JOIN catalog_entities i ON i.public_id=pid_blob(candidate.id)
 LEFT JOIN catalog_book_files bf ON bf.entity_id=i.id LEFT JOIN catalog_entities book ON book.id=bf.book_id
 LEFT JOIN catalog_songs sg ON sg.entity_id=i.id LEFT JOIN catalog_entities album ON album.id=sg.album_id
 WHERE `+restriction+` ORDER BY candidate.ord DESC,candidate.id DESC LIMIT 40`, bind...)
	if err != nil {
		return homeSource{}, err
	}
	candidates := []recCandidate{}
	seen := map[string]bool{}
	for rows.Next() {
		var file, at, parent string
		var kind int
		if err = rows.Scan(&file, &at, &parent, &kind); err != nil {
			break
		}
		if !booksOnly || kind == 9 {
			if !seen[parent] {
				seen[parent] = true
				candidates = append(candidates, recCandidate{ID: parent})
			}
		}
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return homeSource{}, err
	}
	if !booksOnly {
		recent, err := s.homeMergedRecent(r, true)
		if err != nil {
			return homeSource{}, err
		}
		// Recent works retain server newest-first order, with resumes leading.
		for _, v := range recent {
			if !seen[v[0]] {
				seen[v[0]] = true
				candidates = append(candidates, recCandidate{ID: v[0]})
			}
			if len(candidates) == homeCompositionMaximum {
				break
			}
		}
	}
	return candidateSource(candidates), nil
}

func (s *Service) homeCollection(r HomeRequest) (*HomeCollection, error) {
	libraries, err := s.homeLibraries(r.Libraries)
	if err != nil {
		return nil, err
	}
	for _, library := range libraries {
		if library.Kind != "movie" {
			continue
		}
		collections, _, err := s.Collections(library.ID, r.Viewer, "", 4)
		if err != nil {
			return nil, err
		}
		for _, collection := range collections {
			if collection.ItemCount == 0 {
				continue
			}
			restriction, args := r.Viewer.itemVisibilitySQL("i.id")
			bind := append([]any{collection.ID}, args...)
			rows, err := s.read().Query(`SELECT pid(i.public_id) FROM catalog_entities co CROSS JOIN catalog_collection_members m ON m.collection_id=co.id CROSS JOIN catalog_entities i ON i.id=m.item_id WHERE co.public_id=pid_blob(?) AND `+homeVisible("i.id")+` AND `+restriction+` ORDER BY m.order_key,m.item_id LIMIT 4`, bind...)
			if err != nil {
				return nil, err
			}
			ids := []string{}
			for rows.Next() {
				var id string
				if err = rows.Scan(&id); err != nil {
					break
				}
				ids = append(ids, id)
			}
			if e := rows.Err(); err == nil {
				err = e
			}
			rows.Close()
			if err != nil {
				return nil, err
			}
			if len(ids) == 0 {
				continue
			}
			members, err := s.homeEntries(r.Profile, ids)
			if err != nil {
				return nil, err
			}
			if err = s.homeEnrich(r.Profile, members); err != nil {
				return nil, err
			}
			out := &HomeCollection{Collection: collection, Members: members}
			for _, member := range members {
				if out.BackdropURL == "" {
					out.BackdropURL = member.BackdropURL
				}
			}
			if collection.ItemCount == len(members) {
				chronological := append([]ContentEntry{}, members...)
				sort.SliceStable(chronological, func(i, j int) bool {
					a, b := chronological[i].Year, chronological[j].Year
					return a != nil && (b == nil || *a < *b)
				})
				if chronological[0].Year != nil && chronological[len(chronological)-1].Year != nil {
					out.FirstYear = chronological[0].Year
					out.LastTitle = chronological[len(chronological)-1].Title
				}
			}
			return out, nil
		}
	}
	return nil, nil
}

// homeSpotlight chooses, in order: newest recent video, first related pick
// from an actual completed history event, newest continuation, then ranked
// recommendations and recent additions until five different works are filled.
// Membership is independent of UI timers. A sparse library yields fewer real
// works, never duplicate slides or manufactured titles/reasons.
func (s *Service) homeSpotlight(r HomeRequest) ([]HomeSpotlight, error) {
	recent, err := s.homeMergedRecent(r, false)
	if err != nil {
		return nil, err
	}
	source, related, err := s.homeBecause(r)
	if err != nil {
		return nil, err
	}
	continuation, err := s.homeRowFor(r, homeRowSpec{ID: "continue", Direction: "desc"}, ContentRevision{}, HomeRowPage{Limit: 5}, false)
	if err != nil {
		return nil, err
	}
	recommended, err := s.homeEngineSource(r, homeRowSpec{ID: "recommended"})
	if err != nil {
		return nil, err
	}
	type pick struct {
		id     string
		reason HomeReason
	}
	picks := []pick{}
	seen := map[string]bool{}
	add := func(id string, reason HomeReason) error {
		if len(picks) == 5 {
			return nil
		}
		var work string
		err := s.read().QueryRow(`SELECT CASE i.kind WHEN 4 THEN pid(sh.public_id) WHEN 3 THEN pid(parent.public_id) ELSE pid(i.public_id) END FROM catalog_entities i LEFT JOIN catalog_episodes ep ON ep.entity_id=i.id LEFT JOIN catalog_entities sh ON sh.id=ep.show_id LEFT JOIN catalog_seasons sn ON sn.entity_id=i.id LEFT JOIN catalog_entities parent ON parent.id=sn.show_id WHERE i.public_id=pid_blob(?) AND i.retired=0`, id).Scan(&work)
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if work != "" && !seen[work] {
			seen[work] = true
			picks = append(picks, pick{work, reason})
		}
		return nil
	}
	recentReason := func(v [2]string) HomeReason {
		added := strings.Split(v[1], "\x1f")[0]
		at, _ := time.Parse(time.RFC3339Nano, added)
		kind, code, fallback := "recent", "next.home.reason.recentIn", "Recently added in {library}"
		if !at.IsZero() && !at.After(r.now()) && r.now().Sub(at) <= homeNewWindow {
			kind, code, fallback = "new", "next.home.reason.newIn", "New in {library}"
		}
		return HomeReason{Kind: kind, Text: ServerText{Code: code, Fallback: fallback}}
	}
	if len(recent) > 0 {
		if err = add(recent[0][0], recentReason(recent[0])); err != nil {
			return nil, err
		}
	}
	if source != nil && len(related) > 0 {
		if err = add(related[0].ID, HomeReason{Kind: "because_you_watched", Source: source, Text: ServerText{Code: "next.home.becauseYouWatched", Params: map[string]string{"title": source.Name}, Fallback: "Because you watched " + source.Name}}); err != nil {
			return nil, err
		}
	}
	if len(continuation.Entries) > 0 {
		if err = add(continuation.Entries[0].ID, HomeReason{Kind: "continue", Text: ServerText{Code: "next.home.continueWatching", Fallback: "Continue Watching"}}); err != nil {
			return nil, err
		}
	}
	for _, c := range recommended.candidates {
		if c.Kind == "movie" || c.Kind == "show" {
			if err = add(c.ID, HomeReason{Kind: "recommended", Text: ServerText{Code: "next.home.reason.recommended", Fallback: "Recommended for you"}}); err != nil {
				return nil, err
			}
		}
		if len(picks) == 5 {
			break
		}
	}
	for _, v := range recent {
		if err = add(v[0], recentReason(v)); err != nil {
			return nil, err
		}
		if len(picks) == 5 {
			break
		}
	}
	ids := []string{}
	for _, p := range picks {
		ids = append(ids, p.id)
	}
	entries, err := s.homeEngineEntries(r.Profile, ids)
	if err != nil {
		return nil, err
	}
	if err = s.homeEnrich(r.Profile, entries); err != nil {
		return nil, err
	}
	targets := []artworkTarget{}
	for _, e := range entries {
		targets = append(targets, artworkTarget{e.Kind, e.ID})
	}
	art, err := s.resolveArtwork(targets)
	if err != nil {
		return nil, err
	}
	out := []HomeSpotlight{}
	for i, entry := range entries {
		var overview, library string
		err = s.read().QueryRow(`SELECT substr(COALESCE(sh.overview,d.overview,''),1,300),l.name FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id JOIN libraries l ON l.id=cl.library_id LEFT JOIN catalog_shows sh ON sh.entity_id=e.id LEFT JOIN catalog_item_details d ON d.entity_id=e.id WHERE e.public_id=pid_blob(?)`, entry.ID).Scan(&overview, &library)
		if err != nil {
			return nil, err
		}
		entry.Overview = overview
		artwork := art[artworkTarget{entry.Kind, entry.ID}]
		if artwork.BackdropURL != "" {
			entry.BackdropURL = artwork.BackdropURL
		}
		p := HomeSpotlight{Entry: entry, Reason: picks[i].reason}
		if p.Reason.Kind == "new" {
			unwatched, e := s.homeEntryUnwatched(r, entry)
			if e != nil {
				return nil, e
			}
			if !unwatched {
				p.Reason.Kind = "recent"
				p.Reason.Text = ServerText{Code: "next.home.reason.recentIn", Fallback: "Recently added in {library}"}
			}
		}
		if p.Reason.Kind == "new" || p.Reason.Kind == "recent" {
			p.Reason.LibraryID = entry.LibraryID
			p.Reason.Text.Params = map[string]string{"library": library}
			p.Reason.Text.Fallback = strings.ReplaceAll(p.Reason.Text.Fallback, "{library}", library)
		}
		if entry.Kind == "show" {
			next, _, e := s.workspaceNextUp(ShowWorkspaceRequest{Viewer: r.Viewer, Profile: r.Profile, Library: entry.LibraryID}, entry.ID)
			if e != nil {
				return nil, e
			}
			p.ResumeTarget = next
			one := []ContentEntry{p.Entry}
			if e = s.homeSpotlightSeasonCounts(r, one); e != nil {
				return nil, e
			}
			p.Entry = one[0]
		}
		out = append(out, p)
	}
	return out, nil
}

func (s *Service) homeNewBadges(r HomeRequest, entries []ContentEntry, src homeSource) error {
	rows, err := s.read().Query(`SELECT id,ord FROM (`+src.base+`)`, src.args...)
	if err != nil {
		return err
	}
	added := map[string]string{}
	for rows.Next() {
		var id, ord string
		if err = rows.Scan(&id, &ord); err != nil {
			break
		}
		added[id] = strings.Split(ord, "\x1f")[0]
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return err
	}
	for i := range entries {
		entry := &entries[i]
		stamp := added[entry.ID]
		entry.AddedAt = &stamp
		at, e := time.Parse(time.RFC3339Nano, stamp)
		if e != nil || at.After(r.now()) || r.now().Sub(at) > homeNewWindow {
			continue
		}
		unwatched, e := s.homeEntryUnwatched(r, *entry)
		if e != nil {
			return e
		}
		entry.New = unwatched
	}
	return nil
}

// New eligibility is shared by the row badge and the spotlight's truthful reason.
func (s *Service) homeEntryUnwatched(r HomeRequest, entry ContentEntry) (bool, error) {
	var unwatched bool
	restriction, args := r.Viewer.itemVisibilitySQL("m.item_id")
	bind := append([]any{r.Profile, entry.ID}, args...)
	err := s.read().QueryRow(`WITH scope AS(SELECT ? AS profile),target AS(SELECT * FROM catalog_entities WHERE public_id=pid_blob(?)) SELECT CASE WHEN e.kind IN(2,3) THEN EXISTS(SELECT 1 FROM catalog_browse_memberships m WHERE m.entity_id=e.id AND `+restriction+` AND `+personalstate.CompactSQL("scope.profile", "m.item_id")+`=0) ELSE `+personalstate.CompactSQL("scope.profile", "e.id")+`=0 END FROM scope CROSS JOIN target e`, bind...).Scan(&unwatched)
	return unwatched, err
}

// Selected-page audiobook resumes retain the real playable part position.
// Whole-book totals are exact only when all <=512 visible parts have a length.
func (s *Service) homeAudioResume(r HomeRequest, entries []ContentEntry) ([]HomeResume, error) {
	if len(entries) == 0 {
		return []HomeResume{}, nil
	}
	ids := []string{}
	for _, e := range entries {
		ids = append(ids, e.ID)
	}
	restriction, args := r.Viewer.itemVisibilitySQL("candidate.entity")
	base := homeContinueBase("continue_listening")
	rows, err := s.read().Query(`SELECT pid(parent.public_id),candidate.id FROM (`+base+`) candidate
 CROSS JOIN catalog_entities i ON i.public_id=pid_blob(candidate.id)
 LEFT JOIN catalog_book_files bf ON bf.entity_id=i.id LEFT JOIN catalog_songs sg ON sg.entity_id=i.id
 JOIN catalog_entities parent ON parent.id=CASE i.kind WHEN 9 THEN bf.book_id WHEN 7 THEN sg.album_id END
 WHERE parent.public_id IN(SELECT pid_blob(value) FROM json_each(?)) AND `+restriction+` ORDER BY candidate.ord DESC,candidate.id DESC LIMIT 40`, append([]any{r.Profile, r.Profile, idsJSON(r.Libraries), idsJSON(ids)}, args...)...)
	if err != nil {
		return nil, err
	}
	fileParents := map[string]string{}
	parentFiles := map[string]string{}
	files := []string{}
	for rows.Next() {
		var parent, file string
		if err = rows.Scan(&parent, &file); err != nil {
			break
		}
		if parentFiles[parent] == "" {
			parentFiles[parent] = file
			fileParents[file] = parent
			files = append(files, file)
		}
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	targets, err := s.homeEntries(r.Profile, files)
	if err != nil {
		return nil, err
	}
	out := []HomeResume{}
	type resume struct {
		item     string
		position float64
	}
	resumes := map[string]resume{}
	for _, target := range targets {
		parent := fileParents[target.ID]
		out = append(out, HomeResume{EntryID: parent, Target: target})
		position := 0.0
		if target.ProgressSeconds != nil {
			position = *target.ProgressSeconds
		}
		resumes[parent] = resume{target.ID, position}
	}
	for index := range entries {
		entry := &entries[index]
		v, ok := resumes[entry.ID]
		if !ok {
			continue
		}
		entry.Playback = &ContentPlayback{ItemID: v.item, StartSeconds: &v.position}
		if entry.Kind != "book" {
			continue
		}
		restriction, args := r.Viewer.itemVisibilitySQL("file.id")
		rows, err := s.read().Query(`SELECT pid(file.public_id),(SELECT max(NULLIF(COALESCE(link.end_seconds,a.duration)-link.start_seconds,0)) FROM catalog_asset_links link JOIN catalog_assets a ON a.id=link.asset_id WHERE link.entity_id=file.id AND link.available=1 AND a.available=1) FROM catalog_entities book CROSS JOIN catalog_book_files f ON f.book_id=book.id CROSS JOIN catalog_entities file ON file.id=f.entity_id WHERE book.public_id=pid_blob(?) AND `+homeVisible("file.id")+` AND `+restriction+` ORDER BY COALESCE(f.disc_number,1),COALESCE(f.part_number,0),file.sort_key,file.id LIMIT 513`, append([]any{entry.ID}, args...)...)
		if err != nil {
			return nil, err
		}
		total, position := 0.0, 0.0
		known, found := true, false
		n := 0
		for rows.Next() {
			var id string
			var duration sql.NullFloat64
			if err = rows.Scan(&id, &duration); err != nil {
				break
			}
			n++
			if !duration.Valid || duration.Float64 <= 0 {
				known = false
			}
			if id == v.item {
				position = total + v.position
				found = true
			}
			total += duration.Float64
		}
		if e := rows.Err(); err == nil {
			err = e
		}
		rows.Close()
		if err != nil {
			return nil, err
		}
		if known && found && n <= 512 {
			entry.Duration = &total
			entry.ProgressSeconds = &position
		}
	}
	return out, nil
}

func (s *Service) homeSpotlightSeasonCounts(r HomeRequest, entries []ContentEntry) error {
	for i := range entries {
		if entries[i].Kind != "show" {
			continue
		}
		restriction, args := r.Viewer.itemVisibilitySQL("ep.entity_id")
		var count int
		err := s.read().QueryRow(`SELECT count(*) FROM catalog_entities sh CROSS JOIN catalog_seasons sn ON sn.show_id=sh.id WHERE sh.public_id=pid_blob(?) AND sn.number>0 AND EXISTS(SELECT 1 FROM catalog_episodes ep WHERE ep.season_id=sn.entity_id AND `+homeVisible("ep.entity_id")+` AND `+restriction+`)`, append([]any{entries[i].ID}, args...)...).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			entries[i].SeasonCount = &count
		}
	}
	return nil
}
