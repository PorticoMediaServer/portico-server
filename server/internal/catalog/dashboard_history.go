package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"portico.local/server/internal/dbwork"
	"strconv"
	"strings"
	"time"
)

const dashboardKind = `CASE h.kind WHEN 1 THEN 'movies' WHEN 4 THEN 'tv' WHEN 7 THEN 'music' WHEN 9 THEN 'audiobooks' WHEN 0 THEN 'live' ELSE 'other' END`

// This filter is shared with the full history table. Person is always the
// server account id, never a name or a profile id.
func historyFilter(q PlayHistoryQuery, now time.Time) (string, []any, error) {
	if q.Period == "" {
		q.Period = "all"
	}
	span, ok := HistoryPeriods[q.Period]
	if !ok || len(q.AccountID) > 128 || len(q.LibraryID) > 128 || len(q.Search) > 200 {
		return "", nil, ErrCursor
	}
	where, args := []string{"h.started_ms<=?"}, []any{now.UnixMilli()}
	if span > 0 {
		where = append(where, "h.started_ms>=?")
		args = append(args, now.Add(-span).UnixMilli())
	}
	for _, f := range []struct{ key, value string }{{"h.account_id", q.AccountID}, {"h.library_id", q.LibraryID}} {
		if f.value != "" {
			where = append(where, f.key+"=?")
			args = append(args, f.value)
		}
	}
	if q.Media != "" && q.Media != "all" {
		if !strings.Contains("|movies|tv|music|audiobooks|live|other|", "|"+q.Media+"|") {
			return "", nil, ErrCursor
		}
		where = append(where, "("+dashboardKind+")=?")
		args = append(args, q.Media)
	}
	if q.Search != "" {
		where = append(where, `(instr(lower(h.title),lower(?))>0 OR instr(lower(h.user_name),lower(?))>0 OR instr(lower(h.device_name),lower(?))>0)`)
		args = append(args, q.Search, q.Search, q.Search)
	}
	return " WHERE " + strings.Join(where, " AND "), args, nil
}

type DashboardKindTime struct {
	Kind      string `json:"kind"`
	Plays     int64  `json:"plays"`
	WatchedMs int64  `json:"watchedMs"`
}
type DashboardPerson struct {
	AccountID string              `json:"accountId"`
	Username  string              `json:"username"`
	Profiles  []string            `json:"profiles"`
	Plays     int64               `json:"plays"`
	WatchedMs int64               `json:"watchedMs"`
	Kinds     []DashboardKindTime `json:"kinds"`
}
type DashboardPeople struct {
	Items      []DashboardPerson `json:"items"`
	NextCursor string            `json:"nextCursor"`
}
type DashboardBucket struct {
	Start   string              `json:"start"`
	End     string              `json:"end"`
	Partial bool                `json:"partial"`
	Kinds   []DashboardKindTime `json:"kinds"`
}
type DashboardTop struct {
	ItemID           string `json:"itemId"`
	Title            string `json:"title"`
	Plays            int64  `json:"plays"`
	People           int64  `json:"people"`
	CountUnit        string `json:"countUnit"`
	PosterRevision   string `json:"posterRevision,omitempty"`
	BackdropRevision string `json:"backdropRevision,omitempty"`
}
type DashboardHistory struct {
	Period   string            `json:"period"`
	From     string            `json:"from"`
	Until    string            `json:"until"`
	TimeZone string            `json:"timeZone"`
	Buckets  []DashboardBucket `json:"buckets"`
	// Hours Monday..Sunday, 0..23, actual played milliseconds.
	BusiestTimes      [][]int64      `json:"busiestTimes"`
	QuietestStartHour *int           `json:"quietestStartHour"`
	Top               []DashboardTop `json:"top"`
	TopKind           string         `json:"topKind"`
}

func (s *Service) DashboardPeople(ctx context.Context, q PlayHistoryQuery) (DashboardPeople, error) {
	out := DashboardPeople{Items: []DashboardPerson{}}
	if q.Limit == 0 {
		q.Limit = 12
	}
	if q.Limit < 1 || q.Limit > 100 {
		return out, ErrCursor
	}
	filter, args, e := historyFilter(q, s.recommendationNow(time.Time{}))
	if e != nil {
		return out, e
	}
	afterTime, afterID := int64(1<<62), ""
	if q.Cursor != "" {
		t, id, ok := strings.Cut(q.Cursor, ":")
		var e error
		afterTime, e = strconv.ParseInt(t, 10, 64)
		if !ok || e != nil || afterTime < 0 || len(id) > 128 {
			return out, ErrCursor
		}
		afterID = id
	}
	tx, done, e := dbwork.BeginRead(ctx, s.db)
	if e != nil {
		return out, e
	}
	defer done()
	rows, e := tx.QueryContext(ctx, `WITH ranked AS (
 SELECT account_id,COALESCE((SELECT a.username FROM accounts a WHERE a.id=h.account_id),max(user_name)) username,count(*) plays,sum(watched_ms) watched FROM play_history h`+filter+` GROUP BY account_id
 ) SELECT account_id,username,plays,watched FROM ranked WHERE watched<? OR watched=? AND account_id>? ORDER BY watched DESC,account_id LIMIT ?`, append(args, afterTime, afterTime, afterID, q.Limit+1)...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var p DashboardPerson
		if e = rows.Scan(&p.AccountID, &p.Username, &p.Plays, &p.WatchedMs); e != nil {
			rows.Close()
			return out, e
		}
		p.Profiles = []string{}
		p.Kinds = []DashboardKindTime{}
		out.Items = append(out.Items, p)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	if len(out.Items) > q.Limit {
		out.Items = out.Items[:q.Limit]
		p := out.Items[len(out.Items)-1]
		out.NextCursor = fmt.Sprintf("%d:%s", p.WatchedMs, p.AccountID)
	}
	// Bounded account page. Kinds/profiles are read only for those accounts.
	for i := range out.Items {
		p := &out.Items[i]
		rows, e = tx.QueryContext(ctx, `SELECT `+dashboardKind+`,count(*),sum(watched_ms) FROM play_history h`+filter+` AND account_id=? GROUP BY `+dashboardKind, append(append([]any{}, args...), p.AccountID)...)
		if e != nil {
			return out, e
		}
		for rows.Next() {
			var k DashboardKindTime
			if e = rows.Scan(&k.Kind, &k.Plays, &k.WatchedMs); e != nil {
				rows.Close()
				return out, e
			}
			p.Kinds = append(p.Kinds, k)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return out, e
		}
		rows, e = tx.QueryContext(ctx, `SELECT DISTINCT profile_name FROM play_history h`+filter+` AND account_id=? AND profile_name<>'' AND profile_name<>user_name ORDER BY profile_name LIMIT 8`, append(append([]any{}, args...), p.AccountID)...)
		if e != nil {
			return out, e
		}
		for rows.Next() {
			var name string
			if e = rows.Scan(&name); e != nil {
				rows.Close()
				return out, e
			}
			p.Profiles = append(p.Profiles, name)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return out, e
		}
	}
	return out, nil
}

func (s *Service) DashboardHistory(ctx context.Context, q PlayHistoryQuery, topKind string) (DashboardHistory, error) {
	now := s.recommendationNow(time.Time{})
	out := DashboardHistory{Period: q.Period, Until: now.Format(time.RFC3339Nano), TimeZone: now.Location().String(), Buckets: []DashboardBucket{}, Top: []DashboardTop{}, TopKind: topKind, BusiestTimes: make([][]int64, 7)}
	for i := range out.BusiestTimes {
		out.BusiestTimes[i] = make([]int64, 24)
	}
	if topKind == "" {
		topKind = "movies"
		out.TopKind = topKind
	}
	if topKind != "movies" && topKind != "tv" && topKind != "music" {
		return out, ErrCursor
	}
	filter, args, e := historyFilter(q, now)
	if e != nil {
		return out, e
	}
	tx, done, e := dbwork.BeginRead(ctx, s.db)
	if e != nil {
		return out, e
	}
	defer done()
	from := now
	var earliest sql.NullInt64
	if e = tx.QueryRowContext(ctx, `SELECT min(started_ms) FROM play_history h`+filter, args...).Scan(&earliest); e != nil {
		return out, e
	}
	if earliest.Valid {
		from = time.UnixMilli(earliest.Int64).In(now.Location())
	}
	span := HistoryPeriods[q.Period]
	if span > 0 {
		from = now.Add(-span)
	}
	out.From = from.Format(time.RFC3339Nano)
	// Calendar boundaries: a Monday week, or first day of the month. Bucket
	// edges are clipped to the requested range and to "now", with partial=true.
	bucketStart := func(t time.Time) time.Time {
		d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
		switch q.Period {
		case "30d", "90d":
			return d.AddDate(0, 0, -(int(d.Weekday())+6)%7)
		case "1y", "all":
			return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location())
		}
		return d
	}
	next := func(t time.Time) time.Time {
		switch q.Period {
		case "30d", "90d":
			return t.AddDate(0, 0, 7)
		case "1y", "all":
			return t.AddDate(0, 1, 0)
		}
		return t.AddDate(0, 0, 1)
	}
	lookup := map[int64]int{}
	for at := bucketStart(from); at.Before(now) && len(out.Buckets) < 1201; at = next(at) {
		end := next(at)
		a, b := at, end
		if a.Before(from) {
			a = from
		}
		if b.After(now) {
			b = now
		}
		lookup[at.UnixMilli()] = len(out.Buckets)
		out.Buckets = append(out.Buckets, DashboardBucket{Start: a.Format(time.RFC3339Nano), End: b.Format(time.RFC3339Nano), Partial: !a.Equal(at) || !b.Equal(end), Kinds: []DashboardKindTime{}})
	}
	// Read real hourly facts. Absent reports remain absent; no elapsed-session
	// lifetime or completion percentage is substituted for watched time.
	rows, e := tx.QueryContext(ctx, `SELECT t.hour_ms,`+dashboardKind+`,sum(t.watched_ms) FROM play_history_time t CROSS JOIN play_history h ON h.id=t.history_id`+filter+` AND t.hour_ms>=? AND t.hour_ms<? GROUP BY t.hour_ms,`+dashboardKind+` ORDER BY t.hour_ms`, append(append([]any{}, args...), from.UnixMilli()-3600000, now.UnixMilli())...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var at, ms int64
		var kind string
		if e = rows.Scan(&at, &kind, &ms); e != nil {
			rows.Close()
			return out, e
		}
		t := time.UnixMilli(at).In(now.Location())
		index, ok := lookup[bucketStart(t).UnixMilli()]
		if !ok {
			continue
		}
		kinds := &out.Buckets[index].Kinds
		found := false
		for i := range *kinds {
			if (*kinds)[i].Kind == kind {
				(*kinds)[i].WatchedMs += ms
				found = true
			}
		}
		if !found {
			*kinds = append(*kinds, DashboardKindTime{Kind: kind, WatchedMs: ms})
		}
		out.BusiestTimes[(int(t.Weekday())+6)%7][t.Hour()] += ms
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	rows, e = tx.QueryContext(ctx, `SELECT (h.started_ms/86400000)*86400000,`+dashboardKind+`,count(*) FROM play_history h`+filter+` GROUP BY h.started_ms/86400000,`+dashboardKind, args...)
	if e != nil {
		return out, e
	}
	for rows.Next() {
		var at, plays int64
		var kind string
		if e = rows.Scan(&at, &kind, &plays); e != nil {
			rows.Close()
			return out, e
		}
		index, ok := lookup[bucketStart(time.UnixMilli(at).In(now.Location())).UnixMilli()]
		if !ok {
			continue
		}
		found := false
		kinds := &out.Buckets[index].Kinds
		for i := range *kinds {
			if (*kinds)[i].Kind == kind {
				(*kinds)[i].Plays += plays
				found = true
			}
		}
		if !found {
			*kinds = append(*kinds, DashboardKindTime{Kind: kind, Plays: plays})
		}
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	var totalHeat int64
	best := int64(1 << 62)
	quiet := 0
	for h := 0; h <= 21; h++ {
		var sum int64
		for _, day := range out.BusiestTimes {
			for k := h; k < h+3; k++ {
				sum += day[k]
			}
		}
		totalHeat += sum
		if sum < best {
			best = sum
			quiet = h
		}
	}
	if totalHeat > 0 {
		out.QuietestStartHour = &quiet
	}
	// Only the top six are joined to catalogue artwork. Music ranks albums,
	// counting track plays; TV ranks shows, counting episode plays.
	target := `h.item_id`
	unit := "movie_plays"
	if topKind == "tv" {
		target = `(SELECT ep.show_id FROM catalog_episodes ep WHERE ep.entity_id=h.item_id)`
		unit = "episode_plays"
	}
	if topKind == "music" {
		target = `(SELECT song.album_id FROM catalog_songs song WHERE song.entity_id=h.item_id)`
		unit = "track_plays"
	}
	rows, e = tx.QueryContext(ctx, `WITH ranked AS (
 SELECT `+target+` target,count(*) plays,count(DISTINCT h.account_id) people FROM play_history h`+filter+` AND (`+dashboardKind+`)=? GROUP BY target ORDER BY plays DESC,people DESC,target LIMIT 6)
 SELECT COALESCE(pid(i.public_id),''),COALESCE(i.title,''),r.plays,r.people,
 COALESCE((SELECT a.digest FROM artwork_selections a WHERE a.entity_id=i.id AND a.role IN('poster','cover','square') AND a.kind=? AND a.subject='' LIMIT 1),''),
 COALESCE((SELECT a.digest FROM artwork_selections a WHERE a.entity_id=i.id AND a.role='backdrop' AND a.kind=? AND a.subject='' LIMIT 1),'')
 FROM ranked r JOIN catalog_entities i ON i.id=r.target ORDER BY r.plays DESC,r.people DESC,r.target`, append(append([]any{}, args...), topKind, map[string]string{"movies": "item", "tv": "show", "music": "album"}[topKind], map[string]string{"movies": "item", "tv": "show", "music": "album"}[topKind])...)
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var v DashboardTop
		if e = rows.Scan(&v.ItemID, &v.Title, &v.Plays, &v.People, &v.PosterRevision, &v.BackdropRevision); e != nil {
			return out, e
		}
		v.CountUnit = unit
		out.Top = append(out.Top, v)
	}
	return out, rows.Err()
}
