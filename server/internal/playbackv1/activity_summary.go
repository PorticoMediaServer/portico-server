package playbackv1

import (
	"context"
	"encoding/json"
	"strings"

	"portico.local/server/internal/dbwork"
)

// ActivitySession is a read-only display projection. It never includes the
// presentation URL, file paths, transport controls, or capability tokens.
type ActivitySession struct {
	ID          string        `json:"id"`
	User        ActivityUser  `json:"user"`
	Profile     AdminName     `json:"profile"`
	Device      AdminName     `json:"device"`
	Item        *ActivityItem `json:"item,omitempty"`
	Kind        string        `json:"kind"`
	State       string        `json:"state"`
	StartedAt   string        `json:"startedAt"`
	Problem     bool          `json:"problem"`
	Converting  bool          `json:"converting"`
	PositionMs  int64         `json:"positionMs"`
	DurationMs  int64         `json:"durationMs,omitempty"`
	Decision    Decisions     `json:"decision"`
	Quality     *Quality      `json:"quality,omitempty"`
	VideoHeight int           `json:"videoHeight,omitempty"`
}
type ActivityUser struct {
	ID string `json:"id"`
	// Username is absent when the session authority has no local username.
	// Neither the account ID nor its profile is a substitute.
	Username string `json:"username,omitempty"`
}

type ActivityItem struct {
	ID               string        `json:"id"`
	Title            string        `json:"title"`
	Kind             string        `json:"kind"`
	ShowID           string        `json:"showId,omitempty"`
	EpisodeNumbering string        `json:"episodeNumbering,omitempty"`
	AirDate          string        `json:"airDate,omitempty"`
	ShowTitle        string        `json:"showTitle,omitempty"`
	SeasonNumber     *int          `json:"seasonNumber,omitempty"`
	EpisodeNumber    *int          `json:"episodeNumber,omitempty"`
	Year             int           `json:"year,omitempty"`
	Artist           string        `json:"artist,omitempty"`
	Album            string        `json:"album,omitempty"`
	Art              []ActivityArt `json:"art"`
}
type ActivityArt struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Role     string `json:"role"`
	Revision string `json:"revision"`
}
type ActivitySessions struct {
	Count           int64                   `json:"count"`
	ConvertingCount int64                   `json:"convertingCount"`
	Overflow        ActivitySessionOverflow `json:"overflow"`
	Items           []ActivitySession       `json:"items"`
}

type ActivitySessionOverflow struct {
	Count           int64 `json:"count"`
	ConvertingCount int64 `json:"convertingCount"`
}

// Only committed presentation decisions establish conversion. Invalid or absent
// JSON cannot turn a requested quality ceiling into a conversion fact.
const activityConverting = `CASE WHEN json_valid(v.presentation) THEN CASE WHEN
 json_extract(v.presentation,'$.decision.video.action') IN ('transcode','burn') OR
 json_extract(v.presentation,'$.decision.audio.action')='transcode' OR
 json_extract(v.presentation,'$.decision.subtitles.action')='burn'
 THEN 1 ELSE 0 END ELSE 0 END`
const activityProblem = `v.state IN ('error','buffering')`

// ActivitySummarySessions ranks the entire indexed active set before capping
// at three. Totals and visible selection share one snapshot; only the chosen
// page receives duration/catalogue/artwork enrichment.
func (s *Service) ActivitySummarySessions(ctx context.Context) (ActivitySessions, error) {
	out := ActivitySessions{Items: []ActivitySession{}}
	tx, done, err := dbwork.BeginRead(ctx, s.DB)
	if err != nil {
		return out, err
	}
	defer done()
	now := s.now().UnixMilli()
	if err = tx.QueryRowContext(ctx, `SELECT count(*),COALESCE(SUM(`+activityConverting+`),0)
 FROM playback_v1_sessions v INDEXED BY playback_v1_sessions_active WHERE `+adminActiveV1, now).Scan(&out.Count, &out.ConvertingCount); err != nil {
		return out, err
	}
	rows, err := tx.QueryContext(ctx, `WITH chosen AS MATERIALIZED (
 SELECT v.id,v.created_ms,CASE WHEN `+activityProblem+` THEN 0 WHEN `+activityConverting+`=1 THEN 1 ELSE 2 END priority
 FROM playback_v1_sessions v INDEXED BY playback_v1_sessions_active WHERE `+adminActiveV1+`
 ORDER BY priority,v.created_ms DESC,v.id LIMIT 3
 ) `+adminV1Projection+` FROM chosen CROSS JOIN playback_v1_sessions v ON v.id=chosen.id`+adminV1Joins+`
 ORDER BY chosen.priority,chosen.created_ms DESC,chosen.id`, now)
	if err != nil {
		return out, err
	}
	ids := []string{}
	lengths := []durationKey{}
	for rows.Next() {
		a, length, _, e := readAdminSession(rows, false)
		if e != nil {
			rows.Close()
			return out, e
		}
		user := ActivityUser{ID: a.User.ID}
		if strings.TrimSpace(a.User.Name) != "" {
			user.Username = a.User.Name
		}
		v := ActivitySession{ID: a.ID, User: user, Profile: a.Profile, Device: a.Device, Kind: a.Kind, State: a.State,
			StartedAt: a.StartedAt, Problem: a.State == "error" || a.State == "buffering",
			PositionMs: a.PositionMs, Decision: a.Decision, Quality: a.Quality, VideoHeight: a.VideoHeight}
		v.Converting = a.Decision.Video != nil && (a.Decision.Video.Action == "transcode" || a.Decision.Video.Action == "burn") ||
			a.Decision.Audio != nil && a.Decision.Audio.Action == "transcode" ||
			a.Decision.Subtitles != nil && a.Decision.Subtitles.Action == "burn"
		if a.Item != nil {
			v.Item = &ActivityItem{ID: a.Item.ID, Title: a.Item.Title, Kind: a.Kind, Art: []ActivityArt{}}
			if a.Kind != "channel" {
				ids = append(ids, a.Item.ID)
			}
		}
		out.Items = append(out.Items, v)
		lengths = append(lengths, length)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return out, err
	}
	durations, err := durationsOf(ctx, tx, lengths)
	if err != nil {
		return out, err
	}
	out.Overflow.Count = out.Count - int64(len(out.Items))
	out.Overflow.ConvertingCount = out.ConvertingCount
	for i := range out.Items {
		out.Items[i].DurationMs = durations[lengths[i]]
		if out.Items[i].Converting {
			out.Overflow.ConvertingCount--
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	raw, _ := json.Marshal(ids)
	rows, err = tx.QueryContext(ctx, `SELECT pid(i.public_id),k.name,i.year,COALESCE(pid(sh.public_id),''),COALESCE(sh.title,''),se.number,ep.number,COALESCE(ep.numbering,''),COALESCE(strftime('%Y-%m-%d',ep.air_date*86400,'unixepoch'),''),COALESCE(ar.title,''),COALESCE(al.title,'')
 FROM json_each(?) wanted CROSS JOIN catalog_entities i ON i.public_id=pid_blob(wanted.value)
 CROSS JOIN catalog_kinds k ON k.id=i.kind
 LEFT JOIN catalog_episodes ep ON ep.entity_id=i.id LEFT JOIN catalog_entities sh ON sh.id=ep.show_id
 LEFT JOIN catalog_seasons se ON se.entity_id=ep.season_id
 LEFT JOIN catalog_songs so ON so.entity_id=i.id LEFT JOIN catalog_entities al ON al.id=so.album_id
 LEFT JOIN catalog_albums ad ON ad.entity_id=al.id LEFT JOIN catalog_entities ar ON ar.id=ad.artist_id`, string(raw))
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var id, kind, showID, show, numbering, airDate, artist, album string
		var year int
		var season, episode *int
		if err = rows.Scan(&id, &kind, &year, &showID, &show, &season, &episode, &numbering, &airDate, &artist, &album); err != nil {
			break
		}
		for n := range out.Items {
			if it := out.Items[n].Item; it != nil && it.ID == id {
				it.Kind, it.ShowID, it.EpisodeNumbering, it.AirDate = kind, showID, numbering, airDate
				it.Year, it.ShowTitle, it.SeasonNumber, it.EpisodeNumber, it.Artist, it.Album = year, show, season, episode, artist, album
			}
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return out, err
	}
	// Keyed entity/parent joins and ready-object checks keep artwork work bounded
	// by this page, including an episode's show and a song's album fallback.
	rows, err = tx.QueryContext(ctx, `WITH requested AS MATERIALIZED (
 SELECT i.id,pid(i.public_id) public_id FROM json_each(?) wanted CROSS JOIN catalog_entities i ON i.public_id=pid_blob(wanted.value)
 ), candidates AS (
 SELECT r.public_id,'item' kind,r.id entity_id,0 priority FROM requested r
 UNION ALL SELECT r.public_id,'show',ep.show_id,1 FROM requested r CROSS JOIN catalog_episodes ep ON ep.entity_id=r.id
 UNION ALL SELECT r.public_id,'album',so.album_id,1 FROM requested r CROSS JOIN catalog_songs so ON so.entity_id=r.id
 ), ranked AS (
 SELECT c.public_id,a.kind,pid(e.public_id) art_id,a.role,a.digest,
 row_number() OVER(PARTITION BY c.public_id,CASE a.role WHEN 'backdrop' THEN 'backdrop' WHEN 'cover' THEN 'square' WHEN 'square' THEN 'square' ELSE 'poster' END ORDER BY c.priority,a.role) rank
 FROM candidates c CROSS JOIN artwork_selections a ON a.kind=c.kind AND a.entity_id=c.entity_id AND a.subject='' AND a.role IN('poster','backdrop','cover','square')
 CROSS JOIN artwork_objects o ON o.digest=a.digest AND o.status='ready' CROSS JOIN catalog_entities e ON e.id=a.entity_id
 ) SELECT public_id,kind,art_id,role,digest FROM ranked WHERE rank=1 ORDER BY public_id,role`, string(raw))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var a ActivityArt
		if err = rows.Scan(&id, &a.Kind, &a.ID, &a.Role, &a.Revision); err != nil {
			return out, err
		}
		for n := range out.Items {
			if it := out.Items[n].Item; it != nil && it.ID == id {
				it.Art = append(it.Art, a)
			}
		}
	}
	return out, rows.Err()
}
