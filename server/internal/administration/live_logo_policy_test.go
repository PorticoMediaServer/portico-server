package administration

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"

	"portico.local/server/internal/livechannels"
)

func liveLogoPolicyFixture(t *testing.T) (*Service, *sql.DB, string, string, []byte, []byte) {
	t.Helper()
	s, db, _ := newService(t)
	store, err := livechannels.New(db)
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.Save(context.Background(), func(context.Context, *sql.Tx, bool) (string, func(string, string) bool, error) {
		return "owner", func(string, string) bool { return true }, nil
	}, livechannels.SourceInput{ID: strings.Repeat("5a", 24), Name: "Logo policy fixture", RequestID: strings.Repeat("4b", 24), TunerCount: 1,
		Playlist: "#EXTM3U\n#EXTINF:-1 tvg-id=\"one\" tvg-logo=\"https://fixture.invalid/one.png\",One\nhttps://fixture.invalid/stream\n"})
	if err != nil {
		t.Fatal(err)
	}
	var channel string
	if err = db.QueryRow(`SELECT channel_id FROM live_channel_versions WHERE generation_id=?`, source.Generation).Scan(&channel); err != nil {
		t.Fatal(err)
	}
	encoded := func(c color.RGBA) []byte {
		img := image.NewRGBA(image.Rect(0, 0, 16, 16))
		img.SetRGBA(0, 0, c)
		var out bytes.Buffer
		if err := png.Encode(&out, img); err != nil {
			t.Fatal(err)
		}
		return out.Bytes()
	}
	return s, db, source.ID, channel, encoded(color.RGBA{R: 255, A: 255}), encoded(color.RGBA{B: 255, A: 255})
}

func TestLiveSourceLogoModesProtectOwnerArtworkAndResolveServerChoice(t *testing.T) {
	s, _, source, channel, owner, provider := liveLogoPolicyFixture(t)
	ctx := context.Background()
	fullID := "live:" + source + ":" + channel
	for i, tc := range []struct {
		mode            string
		server, replace bool
	}{
		{"follow", false, false}, {"follow", true, false},
		{"import", false, false}, {"import", true, false},
		{"replace", false, true}, {"replace", true, true},
		{"none", false, false}, {"none", true, false},
	} {
		global, err := s.LiveDefaultsDocument(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		global.Settings.LogoImport = tc.server
		if _, err = s.SaveLiveDefaults(ctx, nil, Change[LiveDefaults]{ExpectedRevision: global.Revision, OperationID: fmt.Sprintf("logo-global-%d", i), Settings: global.Settings}); err != nil {
			t.Fatal(err)
		}
		config, err := s.LiveSourceConfiguration(ctx, nil, source)
		if err != nil {
			t.Fatal(err)
		}
		config.Settings.Logos.Mode = tc.mode
		if _, err = s.SaveLiveSourceConfiguration(ctx, nil, source, Change[LiveSourceSettings]{ExpectedRevision: config.Revision, OperationID: fmt.Sprintf("logo-source-%d", i), Settings: config.Settings}); err != nil {
			t.Fatal(err)
		}
		if _, err = s.UploadChannelLogo(ctx, nil, fullID, owner, nil, fmt.Sprintf("owner-logo-%d", i)); err != nil {
			t.Fatal(err)
		}
		calls := 0
		s.LogoFetch = func(context.Context, string) ([]byte, error) { calls++; return provider, nil }
		result, err := s.ImportLogos(ctx, nil, source, fmt.Sprintf("logo-import-%d", i))
		if err != nil {
			t.Fatal(err)
		}
		want, count := owner, 0
		if tc.replace {
			want, count = provider, 1
		}
		got, _, _, err := s.ChannelLogoBytes(ctx, fullID)
		if err != nil || !bytes.Equal(got, want) || calls != count || result.Imported != count {
			t.Fatalf("mode=%s global=%t calls=%d result=%+v owner bytes retained=%t err=%v", tc.mode, tc.server, calls, result, bytes.Equal(got, owner), err)
		}
	}
}

func TestLiveSourceStopsInstallingWhenLogoChoiceChangesDuringFetch(t *testing.T) {
	s, db, source, _, _, provider := liveLogoPolicyFixture(t)
	ctx := context.Background()
	s.LogoFetch = func(context.Context, string) ([]byte, error) {
		config, err := s.LiveSourceConfiguration(ctx, nil, source)
		if err != nil {
			return nil, err
		}
		config.Settings.Logos.Mode = "none"
		_, err = s.SaveLiveSourceConfiguration(ctx, nil, source, Change[LiveSourceSettings]{ExpectedRevision: config.Revision, OperationID: "logo-off-during-fetch", Settings: config.Settings})
		return provider, err
	}
	result, err := s.ImportLogos(ctx, nil, source, "logo-fetch-policy-race")
	if err != nil || result.Requested != 1 || result.Imported != 0 || result.Skipped != 1 {
		t.Fatalf("changed policy ignored: %+v %v", result, err)
	}
	var stored, failures int
	if err = db.QueryRow(`SELECT count(*) FROM admin_channel_logos`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM live_logo_failures`).Scan(&failures); err != nil || stored != 0 || failures != 0 {
		t.Fatalf("revoked import stored=%d provider failures=%d err=%v", stored, failures, err)
	}
}
