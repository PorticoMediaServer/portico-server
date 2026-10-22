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
		if _, e := c.DB.Exec(`UPDATE playback_v1_sessions SET item_id=?,presentation=?,position_ms=1234 WHERE id=?`, episode.ID, `{"url":"/private-capability-token","decision":{"video":{"action":"transcode"}},"quality":{"mode":"fixed","id":"1080p"},"videoHeight":720}`, fmt.Sprint(n)); e != nil {
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
	if out.Count != 5 || len(out.Items) != 4 {
		t.Fatalf("cap/total: %+v", out)
	}
	for _, v := range out.Items {
		if v.Item == nil || v.Item.Title != "Episode 3" || v.Item.ShowTitle != "A show" || v.Item.ShowID != show.Public || v.Item.Kind != "episode" || v.Item.EpisodeNumbering != "seasonal" || len(v.Item.Art) != 1 || v.Item.Art[0].ID != show.Public || v.Item.Art[0].Kind != "show" || v.Item.SeasonNumber == nil || *v.Item.SeasonNumber != 2 || v.Item.EpisodeNumber == nil || *v.Item.EpisodeNumber != 3 || v.Quality == nil || v.Quality.ID != "1080p" || v.Quality.MaxHeight != 0 || v.VideoHeight != 720 || v.Kind != "vod" || v.Decision.Video == nil || v.Decision.Video.Action != "transcode" || v.PositionMs != 1234 {
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
		{"fixed capped below requested ladder", Quality{Mode: QualityFixed, ID: "1080p"}, playback.DeliveryPlan{Mode: "hls", SourceVideoCodec: "h264", VideoAction: "convert", TargetHeight: 720, Trace: &playback.DecisionTrace{Video: &playback.TraceVideo{Width: 1920, Height: 1080}}}, 720},
		{"automatic on a real rung", Quality{Mode: QualityAutomatic, ID: "1080p"}, playback.DeliveryPlan{Mode: "hls", SourceVideoCodec: "h264", VideoAction: "convert", TargetHeight: 480, Trace: &playback.DecisionTrace{Video: &playback.TraceVideo{Width: 1920, Height: 1080}}}, 480},
		{"numeric limit still subject to committed cap", Quality{Mode: QualityLimit, MaxHeight: 1080}, playback.DeliveryPlan{Mode: "hls", SourceVideoCodec: "h264", VideoAction: "convert", TargetHeight: 720, Trace: &playback.DecisionTrace{Video: &playback.TraceVideo{Width: 1920, Height: 1080}}}, 720},
		{"unknown geometry", Quality{Mode: QualityFixed, ID: "1080p"}, playback.DeliveryPlan{Mode: "hls", SourceVideoCodec: "h264", VideoAction: "convert", TargetHeight: 720}, 0},
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
