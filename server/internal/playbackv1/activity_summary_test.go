package playbackv1

import (
	"context"
	"encoding/json"
	"fmt"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/playback"
	"strings"
	"testing"
	"time"
)

func TestActivitySummarySessionsExactTotalAndRealDisplay(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("tv", "TV Shows", "tv", "/private")
	show := c.Show(lib, "A show", 2025)
	season := c.Season(show, 2)
	episode := c.Episode(show, season, 3, "/private/episode.mkv")
	c.Drain()
	digest := strings.Repeat("a", 64)
	c.Exec(`INSERT INTO artwork_objects VALUES(?,'image/png',400,600,100,'now','ready')`, digest)
	c.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,subject,provider,image_id,origin,attribution,source_fence,observed_at) VALUES('art','show',?,'poster','','fixture','one','local','fixture','fence','now')`, show.ID)
	c.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,thumbnail_digest,locked,revision,actor,observed_at) VALUES('show',?,'poster','','art',?,?,0,1,'fixture','now')`, show.ID, digest, digest)
	now := time.Now()
	s := &Service{DB: c.DB, Now: func() time.Time { return now }}
	for n := 0; n < 7; n++ {
		insertPausedSession(t, c.DB, fmt.Sprint(n), "vod", "playing", now.UnixMilli()+int64(n), 0)
		if _, e := c.DB.Exec(`UPDATE playback_v1_sessions SET item_id=?,presentation=?,position_ms=1234 WHERE id=?`, episode.ID, `{"url":"/private-capability-token","decision":{"video":{"action":"transcode"}},"quality":{"mode":"fixed","id":"1080p-low"},"videoHeight":720}`, fmt.Sprint(n)); e != nil {
			t.Fatal(e)
		}
	}
	// Expired and ended sessions never inflate the count.
	if _, e := c.DB.Exec(`UPDATE playback_v1_sessions SET ended_ms=1 WHERE id='0';UPDATE playback_v1_sessions SET lease_expires_ms=1 WHERE id='1'`); e != nil {
		t.Fatal(e)
	}
	out, e := s.ActivitySummarySessions(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if out.Count != 5 || len(out.Items) != 3 || out.ConvertingCount != 5 || out.Overflow.Count != 2 || out.Overflow.ConvertingCount != 2 {
		t.Fatalf("cap/total: %+v", out)
	}
	for _, v := range out.Items {
		if v.Item == nil || v.Item.Title != "Episode 3" || v.Item.ShowTitle != "A show" || v.Item.ShowID != show.Public || v.Item.Kind != "episode" || v.Item.EpisodeNumbering != "seasonal" || len(v.Item.Art) != 1 || v.Item.Art[0].ID != show.Public || v.Item.Art[0].Kind != "show" || v.Item.SeasonNumber == nil || *v.Item.SeasonNumber != 2 || v.Item.EpisodeNumber == nil || *v.Item.EpisodeNumber != 3 || v.Quality == nil || v.Quality.ID != "1080p-low" || v.Quality.MaxHeight != 0 || v.VideoHeight != 720 || v.Kind != "vod" || v.Decision.Video == nil || v.Decision.Video.Action != "transcode" || v.PositionMs != 1234 {
			t.Fatalf("wrong real facts: %+v %+v", v, v.Item)
		}
	}
	raw, _ := json.Marshal(out)
	if strings.Contains(string(raw), "private") || strings.Contains(string(raw), "capability") {
		t.Fatal("session capability/path leaked", string(raw))
	}
	if _, e = c.DB.Exec(`UPDATE playback_v1_sessions SET presentation='{"decision":{"video":{"action":"transcode"}}}'`); e != nil {
		t.Fatal(e)
	}
	out, e = s.ActivitySummarySessions(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	for _, v := range out.Items {
		if v.Quality != nil || v.VideoHeight != 0 {
			t.Fatal("unknown conversion height was fabricated")
		}
	}
}
func TestActivitySummarySessionsAudioAlbumArtist(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("music", "Music", "music", "/music")
	artist := c.Artist(lib, "Artist")
	album := c.Album(artist, "Album", 2024)
	song := c.Song(album, 1, "/music/song.mp3", "Song")
	c.Drain()
	now := time.Now()
	insertPausedSession(t, c.DB, "audio", "audio", "paused", now.UnixMilli(), 0)
	if _, e := c.DB.Exec(`UPDATE playback_v1_sessions SET item_id=? WHERE id='audio'`, song.ID); e != nil {
		t.Fatal(e)
	}
	out, e := (&Service{DB: c.DB}).ActivitySummarySessions(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if len(out.Items) != 1 || out.Items[0].Item.Artist != "Artist" || out.Items[0].Item.Album != "Album" || out.Items[0].State != "paused" {
		t.Fatalf("audio display: %+v", out)
	}
}

// Exercise the production projection after SessionPlan, including replacement
// of a previous height with unknown geometry or a copied picture.
func TestActivitySummarySessionsCommittedVideoHeight(t *testing.T) {
	for _, tc := range []struct {
		name    string
		quality Quality
		plan    playback.DeliveryPlan
		want    int
	}{
		{"fixed capped below requested ladder", Quality{Mode: QualityFixed, ID: "1080p-low"}, playback.DeliveryPlan{Mode: "hls", SourceVideoCodec: "h264", VideoAction: "convert", TargetHeight: 720, Trace: &playback.DecisionTrace{Video: &playback.TraceVideo{Width: 1920, Height: 1080}}}, 720},
		{"automatic on a real rung", Quality{Mode: QualityAutomatic, ID: "1080p-low"}, playback.DeliveryPlan{Mode: "hls", SourceVideoCodec: "h264", VideoAction: "convert", TargetHeight: 480, Trace: &playback.DecisionTrace{Video: &playback.TraceVideo{Width: 1920, Height: 1080}}}, 480},
		{"numeric limit still subject to committed cap", Quality{Mode: QualityLimit, MaxHeight: 1080}, playback.DeliveryPlan{Mode: "hls", SourceVideoCodec: "h264", VideoAction: "convert", TargetHeight: 720, Trace: &playback.DecisionTrace{Video: &playback.TraceVideo{Width: 1920, Height: 1080}}}, 720},
		{"unknown geometry", Quality{Mode: QualityFixed, ID: "1080p-low"}, playback.DeliveryPlan{Mode: "hls", SourceVideoCodec: "h264", VideoAction: "convert", TargetHeight: 720}, 0},
		{"audio-only conversion", Quality{Mode: QualityAutomatic}, playback.DeliveryPlan{Mode: "hls", SourceVideoCodec: "h264", VideoAction: "copy", AudioAction: "convert", Trace: &playback.DecisionTrace{Video: &playback.TraceVideo{Width: 1920, Height: 1080}}}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if e := tc.quality.Validate("quality"); e != nil {
				t.Fatal(e)
			}
			c := catalogtest.Open(t)
			now := time.Now()
			s := &Service{DB: c.DB, Now: func() time.Time { return now }}
			view := Presentation{Quality: &tc.quality, URL: "/private-capability", VideoHeight: 2160}
			s.describe(&view, tc.plan, playback.V1Choice{})
			if view.Decision.Video == nil {
				t.Fatal("video decision missing from committed presentation")
			}
			if view.VideoHeight != tc.want {
				t.Fatalf("committed height: %d, want %d", view.VideoHeight, tc.want)
			}
			raw, e := json.Marshal(view)
			if e != nil {
				t.Fatal(e)
			}
			insertPausedSession(t, c.DB, "session", "vod", "playing", now.UnixMilli(), 0)
			c.Exec(`UPDATE playback_v1_sessions SET presentation=? WHERE id='session'`, string(raw))
			out, e := s.ActivitySummarySessions(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			if len(out.Items) != 1 || out.Items[0].VideoHeight != tc.want || out.Items[0].Quality == nil || *out.Items[0].Quality != tc.quality || out.Items[0].Kind != "vod" {
				t.Fatalf("committed presentation lost: %+v", out)
			}
			wire, e := json.Marshal(out)
			if e != nil {
				t.Fatal(e)
			}
			if strings.Contains(string(wire), "private-capability") || tc.want == 0 && strings.Contains(string(wire), "videoHeight") {
				t.Fatalf("unknown/private fact escaped: %s", wire)
			}
		})
	}
}

func TestActivitySummaryPrioritizesFullActiveSetBeforeThreeAndCountsOverflow(t *testing.T) {
	c := catalogtest.Open(t)
	now := time.UnixMilli(1_790_000_000_000)
	s := &Service{DB: c.DB, Now: func() time.Time { return now }}
	// Four ordinary newer sessions would fill the old pre-priority cap.
	for n := 0; n < 4; n++ {
		insertPausedSession(t, c.DB, fmt.Sprintf("new-%d", n), "vod", "playing", now.UnixMilli()+int64(n), 0)
	}
	for _, v := range []struct {
		id, state, presentation string
		at                      int64
	}{
		{"old-problem", "buffering", `{"decision":{"video":{"action":"direct","reasons":[]}}}`, 10},
		{"old-video", "playing", `{"decision":{"video":{"action":"transcode","reasons":[]}}}`, 20},
		{"old-audio", "playing", `{"decision":{"audio":{"action":"transcode","reasons":[]}}}`, 30},
		{"old-burn", "playing", `{"decision":{"subtitles":{"action":"burn","reasons":[]}}}`, 0},
	} {
		insertPausedSession(t, c.DB, v.id, "vod", v.state, now.UnixMilli()-100+v.at, 0)
		c.Exec(`UPDATE playback_v1_sessions SET presentation=? WHERE id=?`, v.presentation, v.id)
	}
	// A requested 2160 ceiling is no committed conversion. Malformed JSON is unknown.
	c.Exec(`UPDATE playback_v1_sessions SET presentation='{"quality":{"mode":"limit","maxHeight":2160},"decision":{"video":{"action":"copy","reasons":[]}}}' WHERE id='new-3'`)
	c.Exec(`UPDATE playback_v1_sessions SET presentation='not-json' WHERE id='new-2'`)
	insertPausedSession(t, c.DB, "ended", "vod", "error", now.UnixMilli()+100, 0)
	insertPausedSession(t, c.DB, "expired", "vod", "error", now.UnixMilli()+100, 0)
	c.Exec(`UPDATE playback_v1_sessions SET ended_ms=1 WHERE id='ended'`)
	c.Exec(`UPDATE playback_v1_sessions SET lease_expires_ms=? WHERE id='expired'`, now.UnixMilli())
	out, err := s.ActivitySummarySessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 8 || out.ConvertingCount != 3 || out.Overflow.Count != 5 || out.Overflow.ConvertingCount != 1 || len(out.Items) != 3 {
		t.Fatalf("full-set totals: %+v", out)
	}
	for n, id := range []string{"old-problem", "old-audio", "old-video"} {
		if out.Items[n].ID != id {
			t.Fatalf("priority before cap: %+v", out.Items)
		}
	}
	if !out.Items[0].Problem || out.Items[0].Converting || out.Items[1].Problem || !out.Items[1].Converting {
		t.Fatalf("actual state/decisions: %+v", out.Items)
	}
	// The legacy endpoint remains newest-first and keeps its four-row request.
	page, err := s.AdminSessions(context.Background(), 4, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) != 4 || page.Items[0].ID != "new-3" {
		t.Fatalf("legacy pagination changed: %+v", page)
	}
}

func TestActivitySummaryRealUsernameAbsentHostedAndStableRecentOrder(t *testing.T) {
	c := catalogtest.Open(t)
	c.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id) VALUES('owner','real_username',X'00','primary')`)
	c.Exec(`UPDATE direct_profiles SET name='Different profile' WHERE id='primary'`)
	now := time.UnixMilli(1_790_000_000_000)
	for _, id := range []string{"b", "a", "c", "d"} {
		insertPausedSession(t, c.DB, id, "audio", "paused", now.UnixMilli(), 0)
	}
	c.Exec(`UPDATE playback_v1_sessions SET authority='hosted' WHERE id='a'`)
	c.Exec(`UPDATE playback_v1_sessions SET account_id='missing-account',profile_id='missing-profile',device_id='missing-device' WHERE id='b'`)
	out, err := (&Service{DB: c.DB, Now: func() time.Time { return now }}).ActivitySummarySessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Count != 4 || out.ConvertingCount != 0 || out.Overflow.Count != 1 || out.Overflow.ConvertingCount != 0 {
		t.Fatalf("ordinary totals: %+v", out)
	}
	for n, id := range []string{"a", "b", "c"} {
		if out.Items[n].ID != id || out.Items[n].StartedAt != now.UTC().Format(time.RFC3339Nano) {
			t.Fatalf("stable real start: %+v", out.Items)
		}
	}
	if out.Items[0].User.Username != "" || out.Items[1].User.Username != "" || out.Items[2].User.Username != "real_username" || out.Items[2].Profile.Name != "Different profile" {
		t.Fatalf("username provenance: %+v", out.Items)
	}
	if out.Items[0].Profile.Name != "" || out.Items[1].Profile.Name != "" || out.Items[1].Device.Name != "" {
		t.Fatalf("missing names replaced by identifiers: %+v", out.Items)
	}
	wire, _ := json.Marshal(out.Items[0])
	if strings.Contains(string(wire), "username") || strings.Contains(string(wire), `"user":{"id":"owner","name"`) {
		t.Fatalf("unavailable username invented: %s", wire)
	}
	// Revoking/renaming the actual local account is reflected in the next snapshot.
	c.Exec(`UPDATE accounts SET username='renamed_username' WHERE id='owner'`)
	out, err = (&Service{DB: c.DB, Now: func() time.Time { return now }}).ActivitySummarySessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.Items[2].User.Username != "renamed_username" {
		t.Fatal("cached identity escaped source change")
	}
}
