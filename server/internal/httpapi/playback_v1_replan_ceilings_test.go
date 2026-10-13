package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/playbackv1"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
)

// A member starts a title on the LAN, where no remote cap applies, and is then
// given one. Every way of making that presentation produce new bytes from
// outside the LAN (a burned-in subtitle through the session, through the
// subtitle route by session and by media session, another subtitle, turning it
// off, with headers that claim a local network) is planned at or under the
// member's cap as it is now, and a lower cap takes effect on the next change.
func TestSubtitleReplanRoutesHoldToTheMembersCurrentCeiling(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("planning a burn-in needs an HLS producer, which needs FFmpeg")
	}
	facts, err := decoder.ProbeToolchain(context.Background(), ffmpeg)
	if err != nil {
		t.Skipf("FFmpeg can't be probed: %v", err)
	}
	// The burn is planned, never encoded.
	facts.Filters["ass"] = true
	previous := decoder.CurrentToolchain()
	decoder.ConfigureToolchain(facts)
	defer decoder.RestoreToolchain(previous)

	f := memberLimitsFixture(t, 1)
	member, film := memberWithSettledCatalogue(t, f)
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
	hls, err := playback.NewHLS(context.Background(), f.db, filepath.Join(t.TempDir(), "hls"), ffmpeg)
	if err != nil {
		t.Skipf("no HLS producer here: %v", err)
	}
	player := playback.New(f.db)
	cfg := playback.DefaultDeliveryConfiguration()
	cfg.MaxConversions = 1
	player.ConfigureDelivery(playbackReviewSettings{cfg}, nil, nil, "")
	player.ConfigureSubtitleDelivery(subs)
	player.ConfigureHLS(hls)
	f.v1.Subtitles = subs
	f.deps.Playback, f.deps.Subtitles = player, subs
	f.handler = New(f.deps)
	principal, err := f.id.Authenticate(member)
	if err != nil {
		t.Fatal(err)
	}
	styled := "[Script Info]\nScriptType: v4.00+\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.00,0:00:02.00,Default,,0,0,0,,{\\b1}Hello\n"
	upload := func(operation, title string) string {
		t.Helper()
		receipt, err := subs.Mutate(context.Background(), principal, film, subtitles.Mutation{OperationID: operation, SourceID: f.names["member-film"].Token, Scope: "personal", Format: "ass", Language: "en", Title: title, Rights: "Owner transcript", OffsetUS: "0", Content: &styled, Render: "styled"})
		if err != nil {
			t.Skipf("subtitle upload needs the storage helper here: %v", err)
		}
		return receipt.ResourceID
	}
	first, second := upload("upload-styled-1", "Styled"), upload("upload-styled-2", "Styled again")
	if _, err = f.db.Exec(`UPDATE playback_owner_policy SET transcoding_enabled=1`); err != nil {
		t.Fatal(err)
	}

	const lan, outside = "192.168.1.20:5000", "203.0.113.9:51000"
	claimsLocal := map[string]string{"X-Portico-Transport-Class": "ethernet", "X-Forwarded-For": "192.168.1.5", "X-Real-IP": "192.168.1.5", "Forwarded": "for=192.168.1.5"}
	var s playbackv1.SessionView
	w := f.rawFrom(lan, "POST", "/v1/playback/sessions", member, map[string]string{"Idempotency-Key": "replan-ceiling-00001"}, startBody(film, nil))
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &s) != nil {
		t.Fatalf("LAN start: %d %s", w.Code, w.Body.String())
	}
	planned := func() (string, int, string) {
		t.Helper()
		var media, action string
		var bitrate int
		if err := f.db.QueryRow(`SELECT v.media_session_id,d.video_action,d.video_bitrate FROM playback_v1_sessions v JOIN playback_delivery_plans d ON d.session_id=v.media_session_id WHERE v.id=?`, s.ID).Scan(&media, &action, &bitrate); err != nil {
			t.Fatal(err)
		}
		return media, bitrate, action
	}
	revision := s.Revision
	patch := func(peer string, headers map[string]string, track any) {
		t.Helper()
		all := map[string]string{"If-Match": revision}
		for k, v := range headers {
			all[k] = v
		}
		w := f.rawFrom(peer, "PATCH", "/v1/playback/sessions/"+s.ID, member, all, map[string]any{"subtitles": map[string]any{"trackId": track}})
		if w.Code == 403 {
			t.Skipf("this fixture's source can't be converted: %s", w.Body.String())
		}
		var view playbackv1.SessionView
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &view) != nil {
			t.Fatalf("PATCH subtitles %v from %s: %d %s", track, peer, w.Code, w.Body.String())
		}
		revision = view.Revision
	}
	held := func(what string, ceiling int, burned bool) {
		t.Helper()
		_, bitrate, action := planned()
		if bitrate > ceiling || burned && (action != "convert" || bitrate < 1) {
			t.Fatalf("%s: planned %s at %d bit/s under a member cap of %d", what, action, bitrate, ceiling)
		}
	}
	setMemberLimits(t, f, `{"remoteBitrateKbps":3000}`)

	patch(outside, claimsLocal, first)
	held("burn-in through the session from outside the LAN, claiming a local network", 3_000_000, true)
	patch(outside, nil, second)
	held("another subtitle", 3_000_000, true)
	patch(outside, nil, nil)
	held("turning the subtitle off", 3_000_000, false)

	// The subtitle route, naming the v1 session and then its media session.
	selectBy := func(session, operation, resource string) {
		t.Helper()
		var plan subtitles.PlaybackPlan
		w := f.rawFrom(outside, "GET", "/v1/items/"+film+"/playback/"+session+"/subtitles", member, claimsLocal, nil)
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &plan) != nil {
			t.Fatalf("subtitle plan by %s: %d %s", session, w.Code, w.Body.String())
		}
		body := map[string]any{"operationId": operation, "generation": plan.Generation, "expectedRevision": plan.Revision, "mode": "track", "resourceId": resource, "resourceRevision": 1, "offsetUs": "0", "positionUs": "0"}
		if w = f.rawFrom(outside, "PUT", "/v1/items/"+film+"/playback/"+session+"/subtitles", member, claimsLocal, body); w.Code != 200 {
			t.Fatalf("subtitle selection by %s: %d %s", session, w.Code, w.Body.String())
		}
	}
	selectBy(s.ID, "route-select-1", first)
	held("burn-in through the subtitle route", 3_000_000, true)
	media, _, _ := planned()
	selectBy(media, "route-select-2", second)
	held("burn-in through the subtitle route by media session", 3_000_000, true)

	// A lower cap applies to the next change. Once the owner lifts it, the next
	// change on the home network is planned from the quality the session asked
	// for (the original) under the ceilings that hold now, like any other change
	// to the session.
	if err = f.db.QueryRow(`SELECT revision FROM playback_v1_sessions WHERE id=?`, s.ID).Scan(&revision); err != nil {
		t.Fatal(err)
	}
	setMemberLimits(t, f, `{"remoteBitrateKbps":1000}`)
	patch(outside, nil, first)
	held("burn-in after the cap was lowered", 1_000_000, true)
	setMemberLimits(t, f, `{}`)
	patch(lan, nil, second)
	if _, bitrate, action := planned(); action != "convert" || bitrate <= 1_000_000 {
		t.Fatalf("burn-in after the cap was lifted, back on the LAN: planned %s at %d bit/s", action, bitrate)
	}
}
