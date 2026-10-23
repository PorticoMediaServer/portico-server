package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/metadata"
	"portico.local/server/internal/playbackv1"
)

// This exercises real Cast pairing and two independently authenticated device
// profiles through the live HTTP routes. The sender offers no version or decode
// facts in the load: the receiver's own profile selects its version.
func TestReceiverPrerequisitesUseReceiverCapabilitiesAndVersionFacts(t *testing.T) {
	f := socialHTTP(t)
	c := catalogtest.New(t, f.db)
	item := c.ID(f.item)
	// Remove the fixture's preexisting unprobed file so these two observed
	// variants are the complete version set, through the catalogue writer API.
	original := tl6AssetID(t, f.db, f.item)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.UnlinkAssetTx(ctx, tx, item, original)
	})
	versions := map[int]string{}
	for _, height := range []int{1080, 360} {
		path := filepath.Join(f.root, map[int]string{1080: "receiver-hd.mp4", 360: "receiver-sd.mp4"}[height])
		if err := os.WriteFile(path, []byte("direct media fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			asset, token, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: path, Size: info.Size(), ModifiedNS: info.ModTime().UnixNano(), Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Width: height * 16 / 9, Height: height, Duration: 600})
			if err != nil {
				return err
			}
			versions[height] = token
			return compactcatalog.LinkAssetTx(ctx, tx, item, asset, compactcatalog.Link{})
		})
	}
	c.Drain()
	phone := webCapabilities()
	f.callAs(f.token, "PUT", "/v1/me/devices/current/capabilities", nil, phone, 204, nil)
	var phoneOptions playbackv1.Options
	f.callAs(f.token, "GET", "/v1/items/"+f.item+"/playback-options", nil, nil, 200, &phoneOptions)
	if phoneOptions.Preferred == nil || phoneOptions.Preferred.VersionID != versions[1080] {
		t.Fatalf("phone version: %+v", phoneOptions.Preferred)
	}
	var local playbackv1.SessionView
	f.callAs(f.token, "POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "receiver-source-local-001"}, tl6StartBody(f.item, nil), 201, &local)

	screen := pairCastTarget(t, f, f.token, "receiver-capability-proof")
	cast := webCapabilities()
	cast["client"] = map[string]any{"family": "cast", "form": "cast", "platform": "cast", "app": "portico-cast"}
	cast["display"] = map[string]any{"width": 640, "height": 480, "dynamicRanges": []string{"sdr"}}
	nativeEngine(cast)["name"] = "caf-shaka"
	nativeEngine(cast)["video"] = []map[string]any{{"codec": "h264", "bitDepths": []int{8}, "maxWidth": 640, "maxHeight": 480, "maxFrameRate": 30, "dynamicRanges": []string{"sdr"}}}
	f.callAs(screen.Session.AccessToken, "PUT", "/v1/me/devices/current/capabilities", nil, cast, 204, nil)
	// A Cast target starts playback directly; it has no catalogue/options
	// browsing grant. Keep that narrower authority intact.
	if w := f.as(screen.Session.AccessToken, "GET", "/v1/items/"+f.item+"/playback-options", nil); w.Code != 403 {
		t.Fatalf("receiver gained options browsing: %d", w.Code)
	}
	if len(phoneOptions.Versions) != 2 {
		t.Fatalf("version set: %+v", phoneOptions.Versions)
	}
	for _, version := range phoneOptions.Versions {
		if version.ID != versions[1080] && version.ID != versions[360] {
			continue
		}
		if version.DurationMs != 600000 || version.SizeBytes != int64(len("direct media fixture")) || version.Container != "mp4" || version.Label == "" {
			t.Fatalf("version lost actual media facts: %+v", version)
		}
		// Asset facts do not supply stream analysis or measured bitrate. Those
		// optional fields stay empty rather than being invented by the DTO.
		if len(version.Video) != 0 || version.BitrateKbps != 0 {
			t.Fatalf("invented optional stream facts: %+v", version)
		}
	}
	var remote playbackv1.SessionView
	w := f.callAs(screen.Session.AccessToken, "POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "receiver-own-session-001"}, tl6StartBody(f.item, nil), 201, &remote)
	assertSpecResponse(t, "POST", "/v1/playback/sessions", w)
	if remote.VersionID != versions[360] {
		t.Fatalf("receiver start inherited the sender's version: %+v", remote)
	}
	// Pairing and starting a different device are not a verified handoff. They
	// must not retire the phone's local playback or create a transfer signal.
	var stillLocal playbackv1.SessionView
	f.callAs(f.token, "GET", "/v1/playback/sessions/"+local.ID, nil, nil, 200, &stillLocal)
	if stillLocal.State != "playing" || stillLocal.End != nil {
		t.Fatalf("unacknowledged transfer stopped local playback: %+v", stillLocal)
	}
	var after playbackv1.Options
	f.callAs(f.token, "GET", "/v1/items/"+f.item+"/playback-options", nil, nil, 200, &after)
	if after.Preferred == nil || after.Preferred.VersionID != versions[1080] {
		t.Fatal("receiver profile overwrote sender profile")
	}
	// A sender cannot update the TV's profile by addressing another device.
	if w := f.as(f.token, "PUT", "/v1/me/devices/"+screen.Device.DeviceID+"/capabilities", cast); w.Code != 404 {
		t.Fatalf("cross-device profile endpoint: %d", w.Code)
	}
	if w := f.as(screen.DeviceToken, "GET", "/v1/items/"+f.item+"/playback-options", nil); w.Code != 401 {
		t.Fatalf("device renewal token became a bearer: %d", w.Code)
	}
	if w := f.as(screen.Session.AccessToken, "GET", "/v1/libraries", nil); w.Code != 403 {
		t.Fatalf("receiver gained catalogue authority: %d", w.Code)
	}
	f.callAs(screen.Session.AccessToken, "DELETE", "/v1/playback/sessions/"+remote.ID, nil, nil, 204, nil)
	f.callAs(f.token, "DELETE", "/v1/playback/sessions/"+local.ID, nil, nil, 204, nil)
}

func TestReceiverPrerequisitesArtworkRetainsViewerAuthority(t *testing.T) {
	f := socialHTTP(t)
	service := metadata.New(f.db, "test-token")
	if err := service.SetArtworkDirectory(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	f.d.Metadata = service
	f.h = New(f.d)
	state := editorState(t, f.h, f.token, f.item)
	data, contentType := editorUpload(t, state.Revision, editorPNG(t, 600, 900))
	if w := editorRequest(t, f.h, "POST", "/v1/metadata/item/"+f.item+"/art/poster/upload", f.token, contentType, data); w.Code != 200 {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	settleCompactCatalogue(t, f.db)
	screen := pairCastTarget(t, f, f.token, "receiver-artwork-proof")
	// Actual protected artwork is fetched with the receiver's bearer, without
	// acquiring the general title/details capability or a sender credential.
	for _, bearer := range []string{f.token, screen.Session.AccessToken} {
		w := f.as(bearer, "GET", "/v1/items/"+f.item+"/art/poster?w=400", nil)
		if w.Code != 200 {
			t.Fatalf("allowed artwork: %d %s", w.Code, w.Body.String())
		}
		cfg, _, err := image.DecodeConfig(bytes.NewReader(w.Body.Bytes()))
		if err != nil || max(cfg.Width, cfg.Height) != 400 {
			t.Fatalf("artwork bytes: %+v %v", cfg, err)
		}
	}
	for _, path := range []string{"/v1/items/" + f.item, "/v1/metadata/item/" + f.item} {
		if w := f.as(screen.Session.AccessToken, "GET", path, nil); w.Code != 403 {
			t.Fatalf("artwork grant widened to %s: %d", path, w.Code)
		}
	}
	other := pairCastTarget(t, f, f.outside, "receiver-hidden-artwork-proof")
	w := f.as(other.Session.AccessToken, "GET", "/v1/items/"+f.item+"/art/poster", nil)
	if w.Code != 404 {
		t.Fatalf("hidden artwork: %d %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["title"]; ok {
		t.Fatal("hidden artwork disclosed identity")
	}
}
