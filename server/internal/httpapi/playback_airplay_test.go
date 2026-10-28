package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"portico.local/server/internal/playbackv1"
	"portico.local/server/internal/subtitles"
)

func TestAirPlayReceiverPreparesExactCurrentChoicesBeforeHandoff(t *testing.T) {
	f, drawn := v1BurnInFixture(t)
	if _, err := f.db.Exec(`UPDATE playback_owner_policy SET transcoding_enabled=1`); err != nil {
		t.Fatal(err)
	}
	asset := f.records[0].Token
	if _, err := f.db.Exec(`INSERT INTO asset_stream_facts(asset_id,revision,size,modified_ns,fingerprint,detail_version) SELECT token,1,size,modified_ns,'receiver-facts',99 FROM catalog_assets WHERE token=?`, asset); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.Exec(`INSERT INTO asset_streams(asset_id,stream_index,type,codec,language,title,channels,channel_layout,is_default,is_forced,detail_json) VALUES(?,0,'video','h264','','',0,'',1,0,'{"width":1920,"height":1080,"bitDepth":8,"profile":"High","level":40,"dynamicRange":"sdr"}'),(?,1,'audio','aac','eng','Default',2,'stereo',1,0,'{"sampleRate":48000,"profile":"LC"}'),(?,2,'audio','aac','fra','Selected',2,'stereo',0,0,'{"sampleRate":48000,"profile":"LC"}')`, asset, asset, asset); err != nil {
		t.Fatal(err)
	}
	principal, err := f.id.Authenticate(f.owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	tracks := []string{}
	for i, lang := range []string{"en", "fr"} {
		content := "1\n00:00:00,000 --> 00:00:02,000\n" + lang + "\n"
		receipt, e := f.v1.Subtitles.Mutate(context.Background(), principal, f.items[0], subtitles.Mutation{OperationID: []string{"receiver-text-one", "receiver-text-two"}[i], SourceID: f.records[0].Token, Scope: "personal", Format: "srt", Language: lang, Title: lang, Rights: "Own text", OffsetUS: "500", Content: &content})
		if e != nil {
			t.Fatal(e)
		}
		tracks = append(tracks, receipt.ResourceID)
	}
	var local playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "airplay-local-start-1"}, startBody(f.items[0], map[string]any{"subtitles": map[string]any{"trackId": tracks[0], "delivery": "sidecar"}}), 201, &local)
	if local.Presentation.Mode != "direct" {
		t.Fatalf("expected direct local text: %+v", local.Presentation)
	}
	var lastURL string
	for index, track := range []string{tracks[0], "", tracks[1], drawn, "", tracks[0]} {
		if index > 0 {
			var value any = track
			delivery := "sidecar"
			if track == "" {
				value = nil
			}
			if track == drawn {
				delivery = "embeddedClient"
			}
			var next playbackv1.SessionView
			change := map[string]any{"subtitles": map[string]any{"trackId": value, "delivery": delivery}}
			if track == tracks[1] {
				change["audio"] = map[string]any{"trackId": "a2"}
			}
			f.call("PATCH", "/v1/playback/sessions/"+local.ID, map[string]string{"If-Match": local.Revision}, change, 200, &next)
			local = next
		}
		var prepared playbackv1.AirPlayPresentation
		endpoint := "/v1/playback/sessions/" + local.ID + "/receivers/airplay"
		body := playbackv1.AirPlayPrepare{Generation: local.Presentation.Generation, ChoiceRevision: local.Presentation.ChoiceRevision, PositionMs: 17000}
		f.call("POST", endpoint, nil, body, 200, &prepared)
		if prepared.Presentation.AudioTrackID != local.Presentation.AudioTrackID {
			t.Fatalf("receiver changed audio: %s -> %s", local.Presentation.AudioTrackID, prepared.Presentation.AudioTrackID)
		}
		if prepared.Presentation.Mode != "stream" || prepared.Presentation.URL == local.Presentation.URL || !strings.HasSuffix(prepared.Presentation.URL, "/master.m3u8") {
			t.Fatal(prepared)
		}
		if track == "" {
			if prepared.Presentation.Subtitle != nil || len(prepared.Presentation.Subtitles) != 0 {
				t.Fatal(prepared)
			}
		} else {
			if prepared.Presentation.Subtitle == nil || prepared.Presentation.Subtitle.TrackID != track {
				t.Fatal(prepared)
			}
			delivery := "sidecar"
			if track == drawn {
				delivery = "burn"
			}
			if prepared.Presentation.Subtitle.Delivery != delivery {
				t.Fatal(prepared)
			}
		}
		var again playbackv1.AirPlayPresentation
		f.call("POST", endpoint, nil, body, 200, &again)
		if again.Presentation.URL != prepared.Presentation.URL {
			t.Fatal("preparation was not idempotent")
		}
		var unchanged playbackv1.SessionView
		f.call("GET", "/v1/playback/sessions/"+local.ID, nil, nil, 200, &unchanged)
		if unchanged.Presentation.URL != local.Presentation.URL || unchanged.Presentation.Generation != local.Presentation.Generation || unchanged.Presentation.ChoiceRevision != local.Presentation.ChoiceRevision {
			t.Fatal("preparation changed the local picture")
		}
		grant := strings.TrimSuffix(strings.TrimPrefix(prepared.Presentation.URL, "/v1/media/"), "/master.m3u8")
		_, _, _, e := f.v1.Playback.ResolveGrantContext(context.Background(), grant)
		if e != nil {
			t.Fatalf("receiver grant: %v", e)
		}
		var media string
		if e = f.db.QueryRow(`SELECT media_session_id FROM playback_airplay_presentations WHERE session_id=?`, local.ID).Scan(&media); e != nil {
			t.Fatal(e)
		}
		plan, e := f.v1.Playback.SessionPlan(context.Background(), media)
		if e != nil || plan == nil || !plan.AirPlay || len(plan.Renditions) != 0 || plan.Mode != "hls" {
			t.Fatal(plan, e)
		}
		if track == drawn {
			if plan.BurnIn == nil || plan.BurnIn.ResourceID != drawn {
				t.Fatal("receiver lost drawn caption", plan)
			}
		} else if plan.BurnIn != nil {
			t.Fatal("Off/text inherited another track burn", plan)
		}
		if lastURL != "" {
			old := strings.TrimSuffix(strings.TrimPrefix(lastURL, "/v1/media/"), "/master.m3u8")
			if _, _, _, e = f.v1.Playback.ResolveGrantContext(context.Background(), old); e == nil {
				t.Fatal("old choices kept a receiver grant")
			}
		}
		stale := body
		stale.ChoiceRevision++
		w := f.raw("POST", endpoint, f.owner.AccessToken, nil, stale)
		if w.Code != 412 {
			t.Fatalf("stale prep %d %s", w.Code, w.Body.String())
		}
		lastURL = prepared.Presentation.URL
	}
	f.call("DELETE", "/v1/playback/sessions/"+local.ID, nil, nil, 204, nil)
	grant := strings.TrimSuffix(strings.TrimPrefix(lastURL, "/v1/media/"), "/master.m3u8")
	if _, _, _, e := f.v1.Playback.ResolveGrantContext(context.Background(), grant); e == nil {
		t.Fatal("receiver outlived session")
	}
}

func TestAirPlayReceiverBurnRefusalNeverPublishesAnUncaptionedAlternative(t *testing.T) {
	f, track := v1BurnInFixture(t)
	var local playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "airplay-refusal-start"}, startBody(f.items[0], map[string]any{"subtitles": map[string]any{"trackId": track, "delivery": "embeddedClient"}}), 201, &local)
	if _, err := f.db.Exec(`UPDATE playback_owner_policy SET transcoding_enabled=0`); err != nil {
		t.Fatal(err)
	}
	w := f.raw("POST", "/v1/playback/sessions/"+local.ID+"/receivers/airplay", f.owner.AccessToken, nil, playbackv1.AirPlayPrepare{Generation: local.Presentation.Generation, ChoiceRevision: local.Presentation.ChoiceRevision})
	if w.Code < 400 {
		t.Fatalf("refused burn advertised receiver: %s", w.Body.String())
	}
	var count int
	if err := f.db.QueryRow(`SELECT count(*) FROM playback_airplay_presentations`).Scan(&count); err != nil || count != 0 {
		t.Fatal(count, err)
	}
	var after playbackv1.SessionView
	f.call("GET", "/v1/playback/sessions/"+local.ID, nil, nil, 200, &after)
	before, _ := json.Marshal(local.Presentation)
	now, _ := json.Marshal(after.Presentation)
	if string(before) != string(now) {
		t.Fatal("receiver refusal changed local captions")
	}
}
