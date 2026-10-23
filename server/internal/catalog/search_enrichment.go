package catalog

import (
	"encoding/json"
	"strings"

	"portico.local/server/internal/facettext"
)

// Enrichment begins after the ranked LIMIT. Every read starts from the returned
// keys, and no result makes a separate detail/API request.
func (s *Service) enrichSearchEntries(r SearchRequest, entries []ContentEntry) error {
	if len(entries) == 0 {
		return nil
	}
	ids, mediaIDs := []string{}, []string{}
	for _, e := range entries {
		ids = append(ids, e.ID)
		if isItemEntity(e.Kind) {
			mediaIDs = append(mediaIDs, e.ID)
		}
	}
	media, err := s.mediaPage(r.Profile, mediaIDs, false)
	if err != nil {
		return err
	}
	byID := map[string]Item{}
	for _, item := range media {
		byID[item.ID] = item
	}
	for i := range entries {
		e := &entries[i]
		if item, ok := byID[e.ID]; ok {
			e.ProgressSeconds = &item.ProgressSeconds
			e.Watched = item.Watched
			e.Missing = item.Missing
			e.StillURL = item.StillURL
			if item.Episode != nil {
				ep := item.Episode
				e.Show = &ContentEntityRef{ID: ep.ShowID, Name: ep.ShowTitle}
				e.SeasonNumber = ep.SeasonNumber
				e.EpisodeNumber = &ep.Number
				e.DateBased = ep.Numbering == "date"
				e.Special = ep.SeasonNumber != nil && *ep.SeasonNumber == 0
				e.AirDate = ep.AirDate
				if ep.Numbering == "absolute" {
					e.AbsoluteNumber = &ep.Number
				}
			}
			if item.Song != nil {
				e.Album = &ContentEntityRef{ID: item.Song.AlbumID, Name: item.Song.AlbumTitle}
				e.DiscNumber = item.Song.DiscNumber
				e.TrackNumber = item.Song.TrackNumber
			}
		}
	}
	rows, err := s.read().Query(`SELECT pid(e.public_id),COALESCE(p.watchlisted,0),COALESCE(pid(ar.public_id),''),COALESCE(ar.title,''),0,e.year,COALESCE(b.author,''),COALESCE(b.narrator,''),COALESCE(b.series,'')
 FROM json_each(?) requested CROSS JOIN catalog_entities e ON e.public_id=pid_blob(requested.value)
 LEFT JOIN personal_items p ON p.profile_id=? AND p.item_id=e.id
 LEFT JOIN catalog_songs song ON song.entity_id=e.id LEFT JOIN catalog_albums a ON a.entity_id=COALESCE(song.album_id,e.id)
 LEFT JOIN catalog_entities ar ON ar.id=a.artist_id AND ar.library_id=e.library_id AND ar.retired=0
 LEFT JOIN catalog_books b ON b.entity_id=e.id
`, idsJSON(ids), r.Profile)
	if err != nil {
		return err
	}
	type context struct {
		watch                                      bool
		artistID, artist, author, narrator, series string
		seasons, year                              int
	}
	contexts := map[string]context{}
	for rows.Next() {
		var id string
		var c context
		if err = rows.Scan(&id, &c.watch, &c.artistID, &c.artist, &c.seasons, &c.year, &c.author, &c.narrator, &c.series); err != nil {
			break
		}
		contexts[id] = c
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return err
	}
	for i := range entries {
		e := &entries[i]
		c := contexts[e.ID]
		e.Watchlisted = &c.watch
		if c.year > 0 {
			e.Year = &c.year
		}
		if c.artistID != "" {
			e.Artist = &ContentEntityRef{ID: c.artistID, Name: c.artist}
		}
		if e.Kind == "book" {
			e.Author = c.author
			if c.narrator != "" {
				e.Narrators = []string{c.narrator}
			}
			if c.series != "" {
				e.Series = []AudioSeries{{Name: c.series}}
			}
		}
		e.Subtitle = ""
	}
	// Season membership uses current parental/container visibility, never a raw
	// show count (which could advertise a hidden season).
	if err = s.searchSeasonCounts(r, entries); err != nil {
		return err
	}
	if err = s.listeningPageFavorites(r.Profile, entries); err != nil {
		return err
	}
	if err = s.countEntryPlays(r.Profile, entries); err != nil {
		return err
	}
	if err = s.searchMatches(r, entries); err != nil {
		return err
	}
	if err = s.searchBookProgress(r, entries); err != nil {
		return err
	}
	if err = s.searchArtistCounts(r, entries); err != nil {
		return err
	}
	return s.searchArtistArtwork(r, entries)
}

// Match evidence is bounded independently of provider credit/alias volume.
// Only values belonging to an already admitted result can be named.
func searchWordsMatch(text, q string) bool {
	words := strings.Fields(facettext.SearchFold(text))
	terms := strings.Fields(facettext.SearchFold(q))
	if len(terms) == 0 {
		return false
	}
	for _, term := range terms {
		found := false
		for _, word := range words {
			if strings.HasPrefix(word, term) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func (s *Service) searchMatches(r SearchRequest, entries []ContentEntry) error {
	ids := []string{}
	for _, e := range entries {
		if !searchWordsMatch(e.Title, r.Q) {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	rows, err := s.read().Query(`SELECT pid(e.public_id),
 COALESCE((SELECT json_group_array(json_object('field','credit','text',name)) FROM (SELECT COALESCE(NULLIF(c.credited_name,''),p.name,'') name FROM catalog_credits c LEFT JOIN catalog_people p ON p.id=c.person_id WHERE c.entity_id=e.id ORDER BY c.ord LIMIT 256)),'[]'),
 COALESCE((SELECT title FROM catalog_episodes ep JOIN catalog_entities sh ON sh.id=ep.show_id AND sh.library_id=e.library_id AND sh.retired=0 WHERE ep.entity_id=e.id),''),
 COALESCE((SELECT a.title FROM catalog_songs song JOIN catalog_entities a ON a.id=song.album_id AND a.library_id=e.library_id AND a.retired=0 WHERE song.entity_id=e.id),'')
 FROM json_each(?) requested CROSS JOIN catalog_entities e ON e.public_id=pid_blob(requested.value)`, idsJSON(ids))
	if err != nil {
		return err
	}
	matches := map[string][]SearchMatch{}
	for rows.Next() {
		var id, raw, show, album string
		if err = rows.Scan(&id, &raw, &show, &album); err != nil {
			break
		}
		var names []SearchMatch
		if err = json.Unmarshal([]byte(raw), &names); err != nil {
			break
		}
		if show != "" {
			names = append(names, SearchMatch{"show", show})
		}
		if album != "" {
			names = append(names, SearchMatch{"album", album})
		}
		matches[id] = names
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return err
	}
	for i := range entries {
		e := &entries[i]
		names := matches[e.ID]
		if e.Artist != nil {
			names = append(names, SearchMatch{"artist", e.Artist.Name})
		}
		if e.Author != "" {
			names = append(names, SearchMatch{"author", e.Author})
		}
		for _, narrator := range e.Narrators {
			names = append(names, SearchMatch{"narrator", narrator})
		}
		for _, series := range e.Series {
			names = append(names, SearchMatch{"series", series.Name})
		}
		for _, n := range names {
			if n.Text != "" && searchEvidenceMatch(e.Title, n.Text, r.Q) {
				m := n
				e.MatchedOn = &m
				break
			}
		}
	}
	return nil
}
func (s *Service) searchSeasonCounts(r SearchRequest, entries []ContentEntry) error {
	ids := []string{}
	for _, e := range entries {
		if e.Kind == "show" {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	visible, args := r.Viewer.entityVisibilitySQL("se.entity_id", "l.library_id")
	bound := append([]any{idsJSON(ids)}, args...)
	rows, err := s.read().Query(`SELECT pid(sh.public_id),count(*) FROM json_each(?) requested CROSS JOIN catalog_entities sh ON sh.public_id=pid_blob(requested.value) CROSS JOIN catalog_seasons se ON se.show_id=sh.id JOIN catalog_libraries l ON l.id=sh.library_id WHERE `+visible+` GROUP BY sh.id`, bound...)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err = rows.Scan(&id, &n); err != nil {
			break
		}
		counts[id] = n
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].Kind == "show" {
			n := counts[entries[i].ID]
			entries[i].SeasonCount = &n
		}
	}
	return nil
}
func (s *Service) enrichSearchPeople(r SearchRequest, entries []ContentEntry) error {
	if len(entries) == 0 {
		return nil
	}
	ids := []string{}
	for i := range entries {
		ids = append(ids, entries[i].ID)
		if entries[i].Subtitle != "" {
			entries[i].Roles = strings.Split(entries[i].Subtitle, " · ")
		}
		entries[i].Subtitle = ""
	}
	visibility, args := personCreditVisibility(r.Viewer)
	bound := append([]any{idsJSON(ids), r.Viewer.librariesJSON()}, args...)
	// One indexed per-person first credit inside a batch statement, rather than
	// reading every person's detail page and eight hydrated known-for cards.
	rows, err := s.read().Query(`SELECT requested.value,pid(i.public_id),i.title FROM json_each(?) requested CROSS JOIN catalog_people person ON person.token=requested.value JOIN catalog_entities i ON i.id=(SELECT pc.entity_id FROM `+personTitleCredits+` pc JOIN catalog_entities i ON i.id=pc.entity_id JOIN catalog_libraries l ON l.id=i.library_id LEFT JOIN catalog_item_details detail ON detail.entity_id=i.id WHERE pc.person_id=person.id AND l.library_id IN(SELECT value FROM json_each(?)) AND `+visibility+` ORDER BY pc.ord,COALESCE(detail.added_text,'') DESC,pc.entity_id LIMIT 1)`, bound...)
	if err != nil {
		return err
	}
	known := map[string]ContentEntityRef{}
	for rows.Next() {
		var id string
		var v ContentEntityRef
		if err = rows.Scan(&id, &v.ID, &v.Name); err != nil {
			break
		}
		known[id] = v
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return err
	}
	for i := range entries {
		if v, ok := known[entries[i].ID]; ok {
			v := v
			entries[i].KnownFor = &v
		}
	}
	return nil
}
func (s *Service) searchArtistArtwork(r SearchRequest, entries []ContentEntry) error {
	ids := []string{}
	for _, e := range entries {
		if e.Kind == "artist" {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	visible, args := r.Viewer.entityVisibilitySQL("a.entity_id", "l.library_id")
	bound := append([]any{idsJSON(ids)}, args...)
	rows, err := s.read().Query(`SELECT requested.value,pid(album.public_id) FROM json_each(?) requested CROSS JOIN catalog_entities artist ON artist.public_id=pid_blob(requested.value) CROSS JOIN json_each((SELECT json_group_array(id) FROM (SELECT a.entity_id id FROM catalog_albums a JOIN catalog_entities album ON album.id=a.entity_id JOIN catalog_libraries l ON l.id=album.library_id WHERE a.artist_id=artist.id AND album.library_id=artist.library_id AND `+visible+` ORDER BY album.year DESC,album.id LIMIT 4))) sample JOIN catalog_entities album ON album.id=sample.value`, bound...)
	if err != nil {
		return err
	}
	samples := map[string][]string{}
	targets := []artworkTarget{}
	for rows.Next() {
		var id, album string
		if err = rows.Scan(&id, &album); err != nil {
			break
		}
		samples[id] = append(samples[id], album)
		targets = append(targets, artworkTarget{"album", album})
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return err
	}
	art, err := s.resolveArtwork(targets)
	if err != nil {
		return err
	}
	for i := range entries {
		e := &entries[i]
		for _, id := range samples[e.ID] {
			if a := art[artworkTarget{"album", id}]; a.PosterURL != "" {
				e.ArtworkPaths = append(e.ArtworkPaths, a.PosterURL)
			}
		}
	}
	return nil
}

func searchEvidenceMatch(title, text, q string) bool {
	titleWords := strings.Fields(facettext.SearchFold(title))
	textWords := strings.Fields(facettext.SearchFold(text))
	for _, term := range strings.Fields(facettext.SearchFold(q)) {
		inTitle := false
		for _, word := range titleWords {
			if strings.HasPrefix(word, term) {
				inTitle = true
				break
			}
		}
		if inTitle {
			continue
		}
		for _, word := range textWords {
			if strings.HasPrefix(word, term) {
				return true
			}
		}
	}
	return false
}
func (s *Service) searchBookProgress(r SearchRequest, entries []ContentEntry) error {
	ids := []string{}
	for _, e := range entries {
		if e.Kind == "book" {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	visible, args := r.Viewer.itemVisibilitySQL("file.id")
	bound := append([]any{idsJSON(ids), r.Profile}, args...)
	rows, err := s.read().Query(`SELECT requested.value,pid(file.public_id),resume.position FROM json_each(?) requested CROSS JOIN catalog_entities book ON book.public_id=pid_blob(requested.value) CROSS JOIN book_resume resume ON resume.profile_id=? AND resume.book_id=book.id CROSS JOIN catalog_entities file ON file.id=resume.item_id JOIN catalog_book_files part ON part.entity_id=file.id AND part.book_id=book.id WHERE resume.unit=0 AND resume.completed=0 AND `+visible, bound...)
	if err != nil {
		return err
	}
	progress := map[string]BookProgress{}
	for rows.Next() {
		var id string
		var p BookProgress
		if err = rows.Scan(&id, &p.ItemID, &p.PartPositionMs); err != nil {
			break
		}
		p.BookID = id
		progress[id] = p
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return err
	}
	for i := range entries {
		if p, ok := progress[entries[i].ID]; ok {
			p := p
			entries[i].BookProgress = &p
		}
	}
	return nil
}
func (s *Service) searchArtistCounts(r SearchRequest, entries []ContentEntry) error {
	ids := []string{}
	for _, e := range entries {
		if e.Kind == "artist" {
			ids = append(ids, e.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	visible, args := r.Viewer.entityVisibilitySQL("a.entity_id", "l.library_id")
	bound := append([]any{idsJSON(ids)}, args...)
	rows, err := s.read().Query(`SELECT requested.value,count(*) FROM json_each(?) requested CROSS JOIN catalog_entities artist ON artist.public_id=pid_blob(requested.value) CROSS JOIN catalog_albums a INDEXED BY catalog_albums_artist ON a.artist_id=artist.id JOIN catalog_entities album ON album.id=a.entity_id JOIN catalog_libraries l ON l.id=album.library_id WHERE album.library_id=artist.library_id AND `+visible+` GROUP BY artist.id`, bound...)
	if err != nil {
		return err
	}
	counts := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err = rows.Scan(&id, &n); err != nil {
			break
		}
		counts[id] = n
	}
	if e := rows.Err(); err == nil {
		err = e
	}
	rows.Close()
	if err != nil {
		return err
	}
	for i := range entries {
		if entries[i].Kind == "artist" {
			n := counts[entries[i].ID]
			entries[i].AlbumCount = &n
		}
	}
	return nil
}
