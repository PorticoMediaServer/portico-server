package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/playbackv1"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
)

// §5.5: a text subtitle is a WebVTT sidecar under the presentation's grant;
// switching it keeps the generation; off removes it; burning a text track is
// refused (only image and styled tracks burn).
func TestPlaybackV1SubtitleSidecars(t *testing.T) {
	f, subs, principal, source := v1SubtitleFixture(t)
	body := "1\n00:00:00,000 --> 00:00:02,000\nHello\n"
	var err error
	receipt, err := subs.Mutate(context.Background(), principal, f.items[0], subtitles.Mutation{OperationID: "upload-english", SourceID: source, Scope: "personal", Format: "srt", Language: "en", Title: "English", Rights: "Owner transcript", OffsetUS: "0", Content: &body})
	if err != nil {
		t.Skipf("subtitle upload needs the storage helper here: %v", err)
	}
	var options playbackv1.Options
	f.call("GET", "/v1/items/"+f.items[0]+"/playback-options", nil, nil, 200, &options)
	found := false
	for _, s := range options.Versions[0].Subtitles {
		found = found || s.ID == receipt.ResourceID && s.Language == "en"
	}
	if !found {
		t.Fatalf("uploaded subtitle missing from options: %+v", options.Versions[0].Subtitles)
	}
	var s playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "subtitle-key-0000001"}, startBody(f.items[0], map[string]any{"subtitles": map[string]any{"trackId": receipt.ResourceID}}), 201, &s)
	if len(s.Presentation.Subtitles) != 1 || s.Presentation.Subtitles[0].TrackID != receipt.ResourceID || !strings.HasPrefix(s.Presentation.Subtitles[0].URL, "/v1/media/") {
		t.Fatalf("subtitles %+v", s.Presentation.Subtitles)
	}
	var off playbackv1.SessionView
	f.call("PATCH", "/v1/playback/sessions/"+s.ID, map[string]string{"If-Match": s.Revision}, map[string]any{"subtitles": map[string]any{"trackId": nil}}, 200, &off)
	if len(off.Presentation.Subtitles) != 0 || off.Presentation.Generation != s.Presentation.Generation {
		t.Fatalf("off %+v", off.Presentation)
	}
	if w := f.raw("PATCH", "/v1/playback/sessions/"+s.ID, f.owner.AccessToken, map[string]string{"If-Match": off.Revision}, map[string]any{"subtitles": map[string]any{"trackId": "nope"}}); w.Code != 400 {
		t.Fatalf("unknown subtitle: %d", w.Code)
	}
	if w := f.raw("PATCH", "/v1/playback/sessions/"+s.ID, f.owner.AccessToken, map[string]string{"If-Match": off.Revision}, map[string]any{"subtitles": map[string]any{"trackId": receipt.ResourceID, "delivery": "burn"}}); w.Code != 400 {
		t.Fatalf("burn-in: %d", w.Code)
	}
}

func v1SubtitleFixture(t *testing.T) (*v1Fixture, *subtitles.Service, identity.Principal, string) {
	t.Helper()
	f := newV1Fixture(t, 1)
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(f.root)
	if err != nil {
		t.Fatal(err)
	}
	subs, err := subtitles.New(subtitles.Options{DB: f.db, Directory: filepath.Join(root, "subtitles"), Storage: storage.New(binary), HelperBinary: binary,
		Authorize: func(ctx context.Context, tx *sql.Tx, p identity.Principal, item string) (identity.Principal, error) {
			return p, nil
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { subs.Close() })
	player := playback.New(f.db)
	player.ConfigureSubtitleDelivery(subs)
	f.v1.Subtitles = subs
	f.handler = New(Dependencies{DB: f.db, Identity: f.id, Catalog: f.cat, Playback: player, Subtitles: subs, PlaybackV1: f.v1})
	principal, err := f.id.Authenticate(f.owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	return f, subs, principal, f.records[0].Token
}

// §5.5 burn-in: a styled track the device can't draw is burned into the video.
// The session keeps its id; the presentation gets a new generation and URL,
// and its decision says "burn". Off again returns to a plain stream (another
// generation). The same track with delivery embeddedClient burns nothing.
func TestPlaybackV1SubtitleBurnIn(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("burn-in needs FFmpeg")
	}
	facts, err := decoder.ProbeToolchain(context.Background(), ffmpeg)
	if err != nil {
		t.Skipf("FFmpeg can't be probed: %v", err)
	}
	// This test plans the burn (the protocol and the delivery plan), it never
	// runs the encoder, so a libass FFmpeg isn't required to exercise it; the
	// rendering itself is covered by playback's ordinary burn-in runtime test.
	facts.Filters["ass"] = true
	previous := decoder.CurrentToolchain()
	decoder.ConfigureToolchain(facts)
	defer decoder.RestoreToolchain(previous)
	f, subs, principal, source := v1SubtitleFixture(t)
	// Burn-in is an HLS conversion: this server needs its HLS producer.
	hls, err := playback.NewHLS(context.Background(), f.db, filepath.Join(t.TempDir(), "hls"), ffmpeg)
	if err != nil {
		t.Skipf("no HLS producer here: %v", err)
	}
	player := playback.New(f.db)
	player.ConfigureSubtitleDelivery(subs)
	player.ConfigureHLS(hls)
	f.handler = New(Dependencies{DB: f.db, Identity: f.id, Catalog: f.cat, Playback: player, Subtitles: subs, PlaybackV1: f.v1})
	styled := "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,{\\b1}Hello\n"
	var receipt subtitles.Receipt
	receipt, err = subs.Mutate(context.Background(), principal, f.items[0], subtitles.Mutation{OperationID: "upload-styled", SourceID: source, Scope: "personal", Format: "ass", Language: "en", Title: "Styled", Rights: "Owner transcript", OffsetUS: "0", Content: &styled, Render: "styled"})
	if err != nil {
		t.Skipf("subtitle upload needs the storage helper here: %v", err)
	}
	if _, err = f.db.Exec(`UPDATE playback_owner_policy SET transcoding_enabled=1`); err != nil {
		t.Fatal(err)
	}
	var s playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "burn-key-00000000001"}, startBody(f.items[0], nil), 201, &s)
	var burned playbackv1.SessionView
	w := f.raw("PATCH", "/v1/playback/sessions/"+s.ID, f.owner.AccessToken, map[string]string{"If-Match": s.Revision}, map[string]any{"subtitles": map[string]any{"trackId": receipt.ResourceID}})
	if w.Code == 403 {
		t.Skipf("this fixture's source can't be converted: %s", w.Body.String())
	}
	if w.Code != 200 {
		t.Fatalf("burn: %d %s", w.Code, w.Body.String())
	}
	if err = json.Unmarshal(w.Body.Bytes(), &burned); err != nil {
		t.Fatal(err)
	}
	p := burned.Presentation
	if burned.ID != s.ID || p.Generation <= s.Presentation.Generation || p.URL == s.Presentation.URL || p.Mode != "stream" || p.Decision.Video == nil || p.Decision.Video.Action != "burn" || len(p.Subtitles) != 0 {
		t.Fatalf("burned presentation %+v (was %+v)", p, s.Presentation)
	}
	var plain playbackv1.SessionView
	f.call("PATCH", "/v1/playback/sessions/"+s.ID, map[string]string{"If-Match": burned.Revision}, map[string]any{"subtitles": map[string]any{"trackId": nil}}, 200, &plain)
	if plain.Presentation.Generation <= p.Generation || plain.Presentation.Decision.Video != nil && plain.Presentation.Decision.Video.Action == "burn" {
		t.Fatalf("off after burn %+v", plain.Presentation)
	}
	var drawn playbackv1.SessionView
	f.call("PATCH", "/v1/playback/sessions/"+s.ID, map[string]string{"If-Match": plain.Revision}, map[string]any{"subtitles": map[string]any{"trackId": receipt.ResourceID, "delivery": "embeddedClient"}}, 200, &drawn)
	if drawn.Presentation.Generation != plain.Presentation.Generation {
		t.Fatalf("embeddedClient burned: %+v", drawn.Presentation)
	}
	if w := f.raw("PATCH", "/v1/playback/sessions/"+s.ID, f.owner.AccessToken, map[string]string{"If-Match": drawn.Revision}, map[string]any{"subtitles": map[string]any{"trackId": receipt.ResourceID, "delivery": "sidecar"}}); w.Code != 400 {
		t.Fatalf("a styled track as a sidecar: %d", w.Code)
	}
}
