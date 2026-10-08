package catalog

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// The server's play history (migration 0007): the owner's record of what was
// played here, by whom, on which device and when. A viewer's own History
// (personal_history) is a different thing — theirs to pause and clear — and this
// record does neither: an owner who wants less keeps less (playHistoryDays).

// recordPlay writes or updates the history row of one playback, in the playback
// transaction. A row begins once the play has really started (or finished), so a
// title opened and closed again leaves nothing behind. counted reports that this
// report is the one that completed the play: the caller keeps the viewer's own
// count from it.
func recordPlay(tx *sql.Tx, v identity.Viewer, playbackID string, entityID, kind int64, now time.Time, positionMs int64, duration float64, started, completed bool) (counted bool, err error) {
	nowMs := now.UnixMilli()
	var id int64
	var was bool
	err = tx.QueryRow(`SELECT id,completed FROM play_history WHERE playback_id=?`, playbackID).Scan(&id, &was)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if !started && !completed {
			return false, nil
		}
		if err = insertPlay(tx, v, playbackID, entityID, kind, nowMs, positionMs, duration, completed); err != nil {
			return false, err
		}
	case err != nil:
		return false, err
	default:
		if _, err = tx.Exec(`UPDATE play_history SET updated_ms=?,position_ms=max(position_ms,?),completed=max(completed,?) WHERE id=?`, nowMs, positionMs, completed, id); err != nil {
			return false, err
		}
	}
	if !completed || was {
		return false, nil
	}
	_, err = tx.Exec(`INSERT INTO play_counts(item_id,plays,last_played_ms) VALUES(?,1,?) ON CONFLICT(item_id) DO UPDATE SET plays=plays+1,last_played_ms=excluded.last_played_ms`, entityID, nowMs)
	return err == nil, err
}

// insertPlay copies in every name the row will be read by. It runs once per
// playback, so the lookups are a handful of primary-key reads.
func insertPlay(tx *sql.Tx, v identity.Viewer, playbackID string, entityID, kind int64, nowMs, positionMs int64, duration float64, completed bool) error {
	var title, library, parent string
	var season, episode sql.NullInt64
	parentSQL := `''`
	switch kind {
	case int64(compactcatalog.Episode):
		parentSQL = `COALESCE((SELECT show.title FROM catalog_episodes ep JOIN catalog_entities show ON show.id=ep.show_id WHERE ep.entity_id=e.id),'')`
	case int64(compactcatalog.Track):
		parentSQL = `COALESCE((SELECT artist.title FROM catalog_songs song JOIN catalog_albums album ON album.entity_id=song.album_id JOIN catalog_entities artist ON artist.id=album.artist_id WHERE song.entity_id=e.id),'')`
	case int64(compactcatalog.Part):
		parentSQL = `COALESCE((SELECT book.title FROM catalog_book_files file JOIN catalog_entities book ON book.id=file.book_id WHERE file.entity_id=e.id),'')`
	}
	if err := tx.QueryRow(`SELECT e.title,cl.library_id,`+parentSQL+`,
	 (SELECT se.number FROM catalog_episodes ep JOIN catalog_seasons se ON se.entity_id=ep.season_id WHERE ep.entity_id=e.id),
	 (SELECT ep.number FROM catalog_episodes ep WHERE ep.entity_id=e.id)
	 FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE e.id=?`, entityID).Scan(&title, &library, &parent, &season, &episode); err != nil {
		return err
	}
	// Who: the account's name and, for any profile but its first, the profile's.
	user, profile := v.AccountID, ""
	if v.Authority == "local" {
		var name sql.NullString
		var profileName sql.NullString
		var primary sql.NullBool
		if err := tx.QueryRow(`SELECT a.username,p.name,p.is_primary FROM accounts a LEFT JOIN direct_profiles p ON p.account_id=a.id AND p.id=? WHERE a.id=?`, v.ProfileID, v.AccountID).Scan(&name, &profileName, &primary); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if name.String != "" {
			user = name.String
		}
		if profileName.Valid && !primary.Bool {
			profile = profileName.String
		}
	}
	// Where: the device the playing session was signed in on.
	var device, deviceName, platform string
	if err := tx.QueryRow(`SELECT d.id,d.name,d.platform FROM playback_sessions ps JOIN authorization_family_tokens t ON t.token_hash=ps.session_hash
	 JOIN identity_device_families f ON f.family_id=t.family_id JOIN identity_devices d ON d.id=f.device_id WHERE ps.id=?`, playbackID).Scan(&device, &deviceName, &platform); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	// How it was delivered: whether the server converted the picture or the
	// sound for this play, read from the session's stored decision while the
	// session still exists (sessions end after thirty days; the history stays).
	var converted bool
	if err := tx.QueryRow(`SELECT COALESCE((SELECT CASE WHEN json_valid(v.presentation) AND (json_extract(v.presentation,'$.audioRender.mode')='converted'
	 OR json_extract(v.presentation,'$.decision.video.action')='transcode' OR json_extract(v.presentation,'$.decision.audio.action')='transcode') THEN 1 ELSE 0 END
	 FROM playback_v1_sessions v WHERE v.media_session_id=? LIMIT 1),0)`, playbackID).Scan(&converted); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO play_history(playback_id,authority,account_id,profile_id,user_name,profile_name,item_id,library_id,kind,title,parent_title,season,episode,device_id,device_name,platform,started_ms,updated_ms,position_ms,duration_ms,completed,converted)
	 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		playbackID, v.Authority, v.AccountID, v.ProfileID, user, profile, entityID, library, kind, title, parent, season, episode, device, deviceName, platform, nowMs, nowMs, positionMs, positionMillis(duration), completed, converted)
	return err
}

// countPersonalPlay adds one to the viewer's own count of a title.
func countPersonalPlay(tx *sql.Tx, profile string, entityID int64) error {
	_, err := tx.Exec(`INSERT INTO personal_play_counts(profile_id,item_id,plays) VALUES(?,?,1) ON CONFLICT(profile_id,item_id) DO UPDATE SET plays=plays+1`, profile, entityID)
	return err
}

// PlayHistoryEntry is one play, as the owner reads it.
type PlayHistoryEntry struct {
	ID      string `json:"id"`
	User    string `json:"user"`
	Profile string `json:"profile,omitempty"`
	// AccountID lets a client filter to "this person" from a row.
	AccountID string `json:"accountId"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	// ParentTitle is the show of an episode, the artist of a song, the book of an
	// audiobook file; Season and Episode are an episode's numbers.
	ParentTitle string `json:"parentTitle,omitempty"`
	Season      *int64 `json:"season,omitempty"`
	Episode     *int64 `json:"episode,omitempty"`
	// ItemID is the title's id while it is still in the catalogue.
	ItemID          string  `json:"itemId,omitempty"`
	LibraryID       string  `json:"libraryId"`
	Device          string  `json:"device,omitempty"`
	Platform        string  `json:"platform,omitempty"`
	StartedAt       string  `json:"startedAt"`
	PositionSeconds float64 `json:"positionSeconds"`
	DurationSeconds float64 `json:"durationSeconds"`
	Completed       bool    `json:"completed"`
}

type PlayHistoryPage struct {
	Entries []PlayHistoryEntry `json:"entries"`
	// Total counts the plays the filters match; sent with the first page only.
	Total      *int64 `json:"total,omitempty"`
	NextCursor string `json:"nextCursor"`
}

// PlayHistoryQuery filters the history. Every field is optional.
type PlayHistoryQuery struct {
	AccountID string
	LibraryID string
	// Period is one of HistoryPeriods' names ("24h", "7d", "30d", "90d", "all").
	Period string
	Cursor string
	Limit  int
}

// PlayHistoryMax is the most rows one page lists.
const PlayHistoryMax = 200

// PlayHistory lists plays newest first. Each filter has an index that yields its
// rows already in order, so a page reads a page of rows however long the history
// is; the total is one count over the same index range, asked for once.
func (s *Service) PlayHistory(ctx context.Context, q PlayHistoryQuery) (PlayHistoryPage, error) {
	out := PlayHistoryPage{Entries: []PlayHistoryEntry{}}
	if q.Limit == 0 {
		q.Limit = 50
	}
	if q.Period == "" {
		q.Period = "all"
	}
	window, known := HistoryPeriods[q.Period]
	if q.Limit < 1 || q.Limit > PlayHistoryMax || !known || len(q.AccountID) > 128 || len(q.LibraryID) > 128 {
		return out, ErrCursor
	}
	where, args := []string{}, []any{}
	if q.AccountID != "" {
		where, args = append(where, `h.account_id=?`), append(args, q.AccountID)
	}
	if q.LibraryID != "" {
		where, args = append(where, `h.library_id=?`), append(args, q.LibraryID)
	}
	if window > 0 {
		where, args = append(where, `h.started_ms>=?`), append(args, time.Now().Add(-window).UnixMilli())
	}
	filter := ""
	if len(where) > 0 {
		filter = ` WHERE ` + strings.Join(where, ` AND `)
	}
	tx, done, err := dbwork.BeginRead(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer done()
	page, pageArgs := filter, append([]any{}, args...)
	if q.Cursor == "" {
		var total int64
		if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM play_history h`+filter, args...).Scan(&total); err != nil {
			return out, err
		}
		out.Total = &total
	} else {
		started, id, ok := strings.Cut(q.Cursor, ":")
		startedMs, startedErr := strconv.ParseInt(started, 10, 64)
		rowID, idErr := strconv.ParseInt(id, 10, 64)
		if !ok || startedErr != nil || idErr != nil {
			return out, ErrCursor
		}
		if page == "" {
			page = ` WHERE `
		} else {
			page += ` AND `
		}
		page += `(h.started_ms,h.id)<(?,?)`
		pageArgs = append(pageArgs, startedMs, rowID)
	}
	rows, err := tx.QueryContext(ctx, `SELECT h.id,h.user_name,h.profile_name,h.account_id,h.kind,h.title,h.parent_title,h.season,h.episode,
	 COALESCE((SELECT pid(e.public_id) FROM catalog_entities e WHERE e.id=h.item_id AND e.retired=0),''),
	 h.library_id,h.device_name,h.platform,h.started_ms,h.position_ms,h.duration_ms,h.completed
	 FROM play_history h`+page+` ORDER BY h.started_ms DESC,h.id DESC LIMIT ?`, append(pageArgs, q.Limit+1)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	var lastStarted, lastID int64
	for rows.Next() {
		var row PlayHistoryEntry
		var id, kind, started, position, duration int64
		var season, episode sql.NullInt64
		if err = rows.Scan(&id, &row.User, &row.Profile, &row.AccountID, &kind, &row.Title, &row.ParentTitle, &season, &episode, &row.ItemID, &row.LibraryID, &row.Device, &row.Platform, &started, &position, &duration, &row.Completed); err != nil {
			return out, err
		}
		if len(out.Entries) == q.Limit {
			out.NextCursor = strconv.FormatInt(lastStarted, 10) + ":" + strconv.FormatInt(lastID, 10)
			break
		}
		lastStarted, lastID = started, id
		row.ID = strconv.FormatInt(id, 10)
		row.Kind, _ = compactcatalog.Kind(kind).Name()
		if season.Valid {
			row.Season = &season.Int64
		}
		if episode.Valid {
			row.Episode = &episode.Int64
		}
		row.StartedAt = time.UnixMilli(started).UTC().Format("2006-01-02T15:04:05.000Z")
		row.PositionSeconds, row.DurationSeconds = float64(position)/1000, float64(duration)/1000
		out.Entries = append(out.Entries, row)
	}
	return out, rows.Err()
}

// PlaySummary is the owner's viewing statistics for a period, read from the
// play history: the same plays the Play history page lists, so the two always
// agree, and kept for as long as the history is (forever unless the owner sets
// playHistoryDays).
type PlaySummary struct {
	Period string `json:"period"`
	Plays  int64  `json:"plays"`
	People int64  `json:"people"`
	// WatchedSeconds is the time between each play's first and last report.
	WatchedSeconds int64 `json:"watchedSeconds"`
	// Converted counts the plays the server converted (picture or sound).
	Converted  int64       `json:"converted"`
	MostPlayed []PlayCount `json:"mostPlayed"`
	MostActive []PlayCount `json:"mostActive"`
}

// PlayCount is a name and its plays in the period.
type PlayCount struct {
	Name  string `json:"name"`
	Plays int64  `json:"plays"`
}

// PlaySummary totals the plays of a period ("24h", "7d", "30d", "90d", "all").
// A bounded period is a range over the started index; "all" reads the history
// once, which an owner asks for and the server does not do on its own.
func (s *Service) PlaySummary(ctx context.Context, period string) (PlaySummary, error) {
	out := PlaySummary{Period: period, MostPlayed: []PlayCount{}, MostActive: []PlayCount{}}
	window, known := HistoryPeriods[period]
	if !known {
		return out, ErrCursor
	}
	since := int64(0)
	if window > 0 {
		since = time.Now().Add(-window).UnixMilli()
	}
	tx, done, err := dbwork.BeginRead(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer done()
	if err = tx.QueryRowContext(ctx, `SELECT count(*),count(DISTINCT authority||'/'||account_id||'/'||profile_id),COALESCE(sum(max(0,updated_ms-started_ms)),0)/1000,COALESCE(sum(converted),0)
	 FROM play_history INDEXED BY play_history_recent WHERE started_ms>=?`, since).Scan(&out.Plays, &out.People, &out.WatchedSeconds, &out.Converted); err != nil {
		return out, err
	}
	top := func(name string) ([]PlayCount, error) {
		rows, err := tx.QueryContext(ctx, `SELECT `+name+` AS name,count(*) AS plays FROM play_history INDEXED BY play_history_recent WHERE started_ms>=? GROUP BY name ORDER BY plays DESC,name COLLATE NOCASE LIMIT 5`, since)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		list := []PlayCount{}
		for rows.Next() {
			var row PlayCount
			if err = rows.Scan(&row.Name, &row.Plays); err != nil {
				return nil, err
			}
			if row.Name != "" {
				list = append(list, row)
			}
		}
		return list, rows.Err()
	}
	// An episode counts for its show and an audiobook file for its book; a film or a song is itself.
	if out.MostPlayed, err = top(`CASE WHEN kind IN(` + strconv.Itoa(int(compactcatalog.Episode)) + `,` + strconv.Itoa(int(compactcatalog.Part)) + `) AND parent_title<>'' THEN parent_title ELSE title END`); err != nil {
		return out, err
	}
	out.MostActive, err = top(`CASE WHEN profile_name<>'' THEN user_name||' · '||profile_name ELSE user_name END`)
	return out, err
}

// countEntryPlays says how many times the viewer has played each song on a page
// of entries to the end (the Songs table's Plays column). One query for the
// page; a song never played carries no count.
func (s *Service) countEntryPlays(profile string, entries []ContentEntry) error {
	ids := []string{}
	for _, entry := range entries {
		if entry.Kind == "song" {
			ids = append(ids, entry.ID)
		}
	}
	if len(ids) == 0 || profile == "" {
		return nil
	}
	rows, err := s.read().Query(`SELECT pid(e.public_id),c.plays FROM (SELECT DISTINCT value FROM json_each(?)) requested
	 CROSS JOIN catalog_entities e ON e.public_id=pid_blob(requested.value)
	 CROSS JOIN personal_play_counts c ON c.profile_id=? AND c.item_id=e.id`, idsJSON(ids), profile)
	if err != nil {
		return err
	}
	defer rows.Close()
	plays := map[string]int{}
	for rows.Next() {
		var id string
		var n int
		if err = rows.Scan(&id, &n); err != nil {
			return err
		}
		plays[id] = n
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for index := range entries {
		if n, ok := plays[entries[index].ID]; ok && entries[index].Kind == "song" {
			count := n
			entries[index].Plays = &count
		}
	}
	return nil
}

// serverWidePlays reports whether the owner has turned on activity shared across
// profiles (home.communityActivityEnabled): the one setting that decides whether
// any viewer sees what the whole server plays.
func (s *Service) serverWidePlays() bool {
	var on sql.NullBool
	if err := s.read().QueryRow(`SELECT json_extract(body,'$."home.communityActivityEnabled"') FROM console_documents WHERE scope='server'`).Scan(&on); err != nil {
		return false
	}
	return on.Valid && on.Bool
}
