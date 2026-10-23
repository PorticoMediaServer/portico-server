package httpapi

import (
	"portico.local/server/internal/playback"
	"portico.local/server/internal/playbackv1"
	"testing"
)

func TestMusicQualityR28HTTPSourceAndSession(t *testing.T) {
	f := musicFixture(t, 1)
	headers := map[string]string{playback.TransportClassHeader: "cellular", DeviceClassHeader: "mobile"}
	read := func(want playbackv1.Quality) {
		t.Helper()
		var out playbackv1.Options
		f.call("GET", "/v1/items/"+f.items[0]+"/playback-options", headers, nil, 200, &out)
		if out.Preferred == nil || out.Preferred.Quality != want || len(out.Qualities) != 4 {
			t.Fatal(out)
		}
	}
	read(playbackv1.Quality{Mode: playbackv1.QualityLimit, MaxAudioBitrateKbps: 320})
	f.call("PATCH", "/v1/preferences", nil, map[string]any{"scope": "profile-device-class", "deviceClass": "mobile", "expectedRevision": 1, "idempotencyKey": "music-original-http", "values": map[string]any{"music.qualityMobile": "original"}}, 200, nil)
	read(playbackv1.Quality{Mode: playbackv1.QualityOriginal})
	headers["Idempotency-Key"] = "music-start-original-http"
	var session playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", headers, startBody(f.items[0], nil), 201, &session)
	if session.Presentation.Quality == nil || session.Presentation.Quality.Mode != playbackv1.QualityOriginal || session.Presentation.AudioRender == nil || session.Presentation.AudioRender.Mode != "direct" {
		t.Fatal(session)
	}
}
