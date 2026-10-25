package catalog

import (
	"encoding/json"
	"io"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/requestinput"
	"strconv"
	"strings"
)

// pageLimit applies the published page limits (BE-API-04): an absent limit
// is the default page, and a limit above the published maximum is clamped to
// it rather than reset to the default.
func pageLimit(limit int) int {
	if limit < 1 {
		return BrowseDefaultLimit
	}
	return min(limit, BrowseMaximumLimit)
}

func (s *Service) audioCursorID(id string) (int64, error) {
	var entityID int64
	if err := s.read().QueryRow(`SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`, id).Scan(&entityID); err != nil {
		return 0, ErrCursor
	}
	return entityID, nil
}

func (s *Service) Artists(library, cursor string, limit int) ([]Artist, string, error) {
	limit = pageLimit(limit)
	after := int64(0)
	if cursor != "" {
		var err error
		after, err = s.audioCursorID(cursor)
		if err != nil {
			return nil, "", err
		}
	}
	rows, err := s.read().Query(`SELECT pid(e.public_id),l.library_id,e.title FROM catalog_entities e
 JOIN catalog_artists d ON d.entity_id=e.id JOIN catalog_libraries l ON l.id=e.library_id
 WHERE l.library_id=? AND l.retired=0 AND e.kind=5 AND e.retired=0 AND e.id>?
 AND (EXISTS(SELECT 1 FROM catalog_song_artists sa JOIN catalog_songs song ON song.entity_id=sa.song_id WHERE sa.artist_id=e.id)
 OR EXISTS(SELECT 1 FROM catalog_albums a JOIN catalog_songs song ON song.album_id=a.entity_id WHERE a.artist_id=e.id))
 ORDER BY e.id LIMIT ?`, library, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Artist{}
	for rows.Next() {
		var row Artist
		if err = rows.Scan(&row.ID, &row.LibraryID, &row.Name); err != nil {
			return nil, "", err
		}
		out = append(out, row)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].ID
	}
	return out, next, rows.Err()
}

func (s *Service) Albums(artist, cursor string, limit int) ([]Album, string, error) {
	limit = pageLimit(limit)
	after := int64(0)
	if cursor != "" {
		var err error
		after, err = s.audioCursorID(cursor)
		if err != nil {
			return nil, "", err
		}
	}
	rows, err := s.read().Query(`SELECT pid(e.public_id),l.library_id,e.title,pid(ar.public_id),ar.title,e.year,a.provider_match_status,
 COALESCE((SELECT pid(item.public_id) FROM catalog_songs s JOIN catalog_entities item ON item.id=s.entity_id JOIN catalog_item_details d ON d.entity_id=item.id WHERE s.album_id=e.id AND d.poster_url<>'' AND item.retired=0 LIMIT 1),''),
 a.release_id,a.release_group_id,a.release_date,a.release_country,a.provider_observed_at,`+albumRoleSQL("e", "ar")+`
 FROM catalog_entities e JOIN catalog_albums a ON a.entity_id=e.id
 JOIN catalog_entities ar ON ar.id=a.artist_id
 JOIN catalog_libraries l ON l.id=e.library_id
 WHERE e.kind=6 AND e.retired=0 AND ar.retired=0 AND l.retired=0
 AND (ar.public_id=pid_blob(?) OR EXISTS(SELECT 1 FROM catalog_songs s JOIN catalog_song_artists sa ON sa.song_id=s.entity_id WHERE s.album_id=e.id AND sa.artist_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?))))
 AND e.id>? ORDER BY e.id LIMIT ?`, artist, artist, artist, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Album{}
	for rows.Next() {
		var row Album
		var artItem, role string
		if err = rows.Scan(&row.ID, &row.LibraryID, &row.Title, &row.AlbumArtistID, &row.AlbumArtist, &row.Year, &row.ProviderMatchStatus, &artItem, &row.ProviderReleaseID, &row.ProviderReleaseGroupID, &row.ProviderReleaseDate, &row.ProviderReleaseCountry, &row.ProviderObservedAt, &role); err != nil {
			return nil, "", err
		}
		row.Role = AlbumRole(role)
		if artItem != "" {
			row.PosterURL = "/v1/items/" + artItem + "/art/poster"
		}
		out = append(out, row)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].ID
	}
	return out, next, rows.Err()
}

func (s *Service) Books(profile, library, cursor string, limit int) ([]Book, string, error) {
	limit = pageLimit(limit)
	after := int64(0)
	if cursor != "" {
		var err error
		after, err = s.audioCursorID(cursor)
		if err != nil {
			return nil, "", err
		}
	}
	rows, err := s.read().Query(`SELECT pid(e.public_id),l.library_id,e.title,b.author,b.narrator,b.provider_match_status,
 COALESCE((SELECT pid(item.public_id) FROM catalog_book_files f JOIN catalog_entities item ON item.id=f.entity_id JOIN catalog_item_details d ON d.entity_id=item.id WHERE f.book_id=e.id AND d.poster_url<>'' AND item.retired=0 LIMIT 1),''),
 COALESCE(pid(resume_item.public_id),''),COALESCE(r.position,0)
 FROM catalog_entities e JOIN catalog_books b ON b.entity_id=e.id JOIN catalog_libraries l ON l.id=e.library_id
 LEFT JOIN book_resume r ON r.book_id=e.id AND r.profile_id=?
 LEFT JOIN catalog_entities resume_item ON resume_item.id=r.item_id
 WHERE l.library_id=? AND l.retired=0 AND e.kind=8 AND e.retired=0
 AND e.id>? AND EXISTS(SELECT 1 FROM catalog_book_files f WHERE f.book_id=e.id)
 ORDER BY e.id LIMIT ?`, profile, library, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []Book{}
	for rows.Next() {
		var row Book
		var artItem, resumeItem string
		var position int64
		if err = rows.Scan(&row.ID, &row.LibraryID, &row.Title, &row.Author, &row.Narrator, &row.ProviderMatchStatus, &artItem, &resumeItem, &position); err != nil {
			return nil, "", err
		}
		if artItem != "" {
			row.PosterURL = "/v1/items/" + artItem + "/art/poster"
		}
		if resumeItem != "" {
			row.Resume = &BookResume{resumeItem, float64(position) / 1000}
		}
		out = append(out, row)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].ID
	}
	return out, next, rows.Err()
}

func (s *Service) LibraryForAudioEntity(kind, id string) (string, error) {
	var query string
	switch kind {
	case "artist":
		query = `catalog_artists`
	case "album":
		query = `catalog_albums`
	case "book":
		query = `catalog_books`
	default:
		return "", requestinput.New("invalid audio entity")
	}
	var library string
	err := s.read().QueryRow(`SELECT l.library_id FROM catalog_entities e JOIN `+query+` d ON d.entity_id=e.id JOIN catalog_libraries l ON l.id=e.library_id WHERE e.public_id=pid_blob(?) AND e.retired=0 AND l.retired=0`, id).Scan(&library)
	return library, err
}

func (s *Service) AudioItems(profile, parent, kind, cursor string, limit int) ([]Item, string, error) {
	table, parentColumn, number := "catalog_songs", "album_id", "track_number"
	if kind == "book" {
		table, parentColumn, number = "catalog_book_files", "book_id", "part_number"
	}
	limit = pageLimit(limit)
	disc, track := 0, 0
	afterName := ""
	var after int64
	if cursor != "" {
		if err := s.read().QueryRow(`SELECT COALESCE(d.disc_number,1),COALESCE(d.`+number+`,0),item.sort_key,item.id FROM `+table+` d JOIN catalog_entities item ON item.id=d.entity_id JOIN catalog_entities p ON p.id=d.`+parentColumn+` WHERE p.public_id=pid_blob(?) AND item.public_id=pid_blob(?)`, parent, cursor).Scan(&disc, &track, &afterName, &after); err != nil {
			return nil, "", requestinput.New("invalid audio cursor")
		}
	}
	rows, err := s.read().Query(`SELECT pid(item.public_id) FROM `+table+` d JOIN catalog_entities item ON item.id=d.entity_id JOIN catalog_entities p ON p.id=d.`+parentColumn+`
 WHERE p.public_id=pid_blob(?) AND p.retired=0 AND item.retired=0
 AND (COALESCE(d.disc_number,1),COALESCE(d.`+number+`,0),item.sort_key,item.id)>(?,?,?,?)
 ORDER BY COALESCE(d.disc_number,1),COALESCE(d.`+number+`,0),item.sort_key,item.id LIMIT ?`, parent, disc, track, afterName, after, limit+1)
	if err != nil {
		return nil, "", err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, "", err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, "", err
	}
	next := ""
	if len(ids) > limit {
		ids = ids[:limit]
		next = ids[len(ids)-1]
	}
	out := []Item{}
	for _, id := range ids {
		item, err := s.Get(profile, id)
		if err != nil {
			return nil, "", err
		}
		out = append(out, item)
	}
	return out, next, nil
}

func (s *Service) BookChapters(book, cursor string, limit int) ([]BookChapter, string, error) {
	limit = pageLimit(limit)
	disc, part, index := 0, 0, 0
	file := ""
	fileName := ""
	var fileID int64
	if cursor != "" {
		colon := strings.LastIndexByte(cursor, ':')
		if colon <= 0 {
			return nil, "", requestinput.New("invalid chapter cursor")
		}
		parsed, err := strconv.Atoi(cursor[colon+1:])
		if err != nil || parsed < 0 {
			return nil, "", requestinput.New("invalid chapter cursor")
		}
		file, index = cursor[:colon], parsed
		if err = s.read().QueryRow(`SELECT COALESCE(f.disc_number,1),COALESCE(f.part_number,0),item.sort_key,item.id FROM catalog_book_chapters c JOIN catalog_book_files f ON f.entity_id=c.file_id JOIN catalog_entities item ON item.id=f.entity_id JOIN catalog_entities b ON b.id=f.book_id WHERE b.public_id=pid_blob(?) AND item.public_id=pid_blob(?) AND c.chapter_index=?`, book, file, index).Scan(&disc, &part, &fileName, &fileID); err != nil {
			return nil, "", requestinput.New("invalid chapter cursor")
		}
	}
	rows, err := s.read().Query(`SELECT pid(item.public_id)||':'||CAST(c.chapter_index AS TEXT),pid(item.public_id),c.chapter_index,c.title,c.start_seconds,c.end_seconds
 FROM catalog_book_chapters c JOIN catalog_book_files f ON f.entity_id=c.file_id JOIN catalog_entities item ON item.id=f.entity_id JOIN catalog_entities b ON b.id=f.book_id
 WHERE b.public_id=pid_blob(?) AND b.retired=0 AND item.retired=0
 AND (COALESCE(f.disc_number,1),COALESCE(f.part_number,0),item.sort_key,item.id,c.chapter_index)>(?,?,?,?,?)
 ORDER BY COALESCE(f.disc_number,1),COALESCE(f.part_number,0),item.sort_key,item.id,c.chapter_index LIMIT ?`, book, disc, part, fileName, fileID, index, limit+1)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	out := []BookChapter{}
	for rows.Next() {
		var row BookChapter
		if err = rows.Scan(&row.ID, &row.ItemID, &row.Index, &row.Title, &row.StartSeconds, &row.EndSeconds); err != nil {
			return nil, "", err
		}
		out = append(out, row)
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		next = out[len(out)-1].ID
	}
	return out, next, rows.Err()
}

// artistSongsPage is the bounded section cursor a page was issued at. The enclosing content
// cursor is signed and scoped by the handler, and re-reads the revisions around it, so this value
// carries only the exact ordering position it stopped at.
type artistSongsPage struct {
	Plays    int64  `json:"p"`
	SortKey  string `json:"k"`
	EntityID int64  `json:"i"`
}

func decodeArtistSongsPage(raw string) (artistSongsPage, bool) {
	var value artistSongsPage
	if len(raw) > 512 || raw == "" {
		return value, false
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil || value.Plays < 0 || value.EntityID <= 0 || len(value.SortKey) > 256 {
		return artistSongsPage{}, false
	}
	// A trailing value (a stray '}' included) is not the end of the cursor.
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return artistSongsPage{}, false
	}
	return value, true
}

// ArtistTopSongs lists the songs this library admits to this artist, ranked by the profile's own
// play count with a stable sort-key/entity tie and bounded by pageLimit. Membership, availability
// and restrictions are the library's existing predicates; the count is that whole scope.
func (s *Service) ArtistTopSongs(profile, library, artist, cursor string, limit int, restrictions identity.ContentRestrictions) ([]ContentEntry, string, int, error) {
	limit = pageLimit(limit)
	restriction, bound := ItemRestrictionSQL("item.id", restrictions)
	ranked := `WITH ranked AS (
 SELECT item.id AS entity_id, item.public_id AS public_id, item.sort_key AS sort_key,
        COALESCE(personal.play_count,0) AS plays
 FROM catalog_song_artists sa
 JOIN catalog_entities artist ON artist.id=sa.artist_id
 JOIN catalog_entities item ON item.id=sa.song_id
 JOIN catalog_libraries lib ON lib.id=item.library_id AND lib.library_id=? AND lib.retired=0
 LEFT JOIN catalog_personal_sort_rows personal ON personal.profile_id=? AND personal.entity_id=item.id
 WHERE artist.public_id=pid_blob(?) AND artist.retired=0
   AND artist.library_id=item.library_id
   AND ` + homeVisible("item.id") + ` AND ` + restriction + `
)`
	base := append([]any{library, profile, artist}, bound...)
	var count int
	if err := s.read().QueryRow(ranked+` SELECT count(*) FROM ranked`, base...).Scan(&count); err != nil {
		return nil, "", 0, err
	}
	page, hasPage := artistSongsPage{Plays: -1}, false
	if cursor != "" {
		var ok bool
		if page, ok = decodeArtistSongsPage(cursor); !ok {
			return nil, "", 0, requestinput.New("invalid artist songs cursor")
		}
		hasPage = true
	}
	rows, err := s.read().Query(ranked+`
 SELECT pid(public_id) FROM ranked
 WHERE (? = 0 OR plays < ? OR (plays = ? AND (sort_key > ? OR (sort_key = ? AND entity_id > ?))))
 ORDER BY plays DESC, sort_key, entity_id LIMIT ?`,
		append(base, boolInt(hasPage), page.Plays, page.Plays, page.SortKey, page.SortKey, page.EntityID, limit+1)...)
	if err != nil {
		return nil, "", 0, err
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return nil, "", 0, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, "", 0, err
	}
	next := ""
	if len(ids) > limit {
		ids = ids[:limit]
		var plays, entityID int64
		var sortKey string
		if err = s.read().QueryRow(`SELECT COALESCE((SELECT play_count FROM catalog_personal_sort_rows p WHERE p.profile_id=? AND p.entity_id=e.id),0),e.sort_key,e.id FROM catalog_entities e WHERE e.public_id=pid_blob(?)`, profile, ids[len(ids)-1]).Scan(&plays, &sortKey, &entityID); err != nil {
			return nil, "", 0, err
		}
		encoded, err := json.Marshal(artistSongsPage{Plays: plays, SortKey: sortKey, EntityID: entityID})
		if err != nil {
			return nil, "", 0, err
		}
		next = string(encoded)
	}
	out, err := s.homeEntries(profile, ids)
	if err != nil {
		return nil, "", 0, err
	}
	return out, next, count, nil
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
