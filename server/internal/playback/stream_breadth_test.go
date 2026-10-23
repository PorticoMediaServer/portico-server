package playback

import (
	"context"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/persistence"
	"testing"
)

func TestVideoBreadthRelationalReadback(t *testing.T) {
	db, e := persistence.Open(filepath.Join(t.TempDir(), "state"))
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	_, _, asset := catalogFixture(t, db, "lib", "/test", compactcatalog.Movie, "film", "Film", compactcatalog.Asset{Path: "/test/film.mkv", Size: 100, ModifiedNS: 1, Container: "matroska", VideoCodec: "hevc", AudioCodec: "truehd", Duration: 10})
	breadth := assets.StreamBreadth{AudioDescription: true, AudioVariant: "truehd_atmos", ChromaSubsampling: "4:2:2", FrameRateNumerator: 24000, FrameRateDenominator: 1001, DolbyVisionRPU: true, DolbyVisionBaseLayer: true, DolbyVisionMetadataKnown: true, DolbyVisionEnhancementType: "fel", DolbyVisionCMVersion: "4.0"}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	e = assets.PersistStreams(tx, asset, 100, 1, []assets.Stream{{Index: 0, Type: "video", Codec: "hevc", Detail: assets.StreamDetail{StreamBreadth: breadth, DolbyVisionProfile: 7}}, {Index: 1, Type: "audio", Codec: "truehd", Channels: 8, Detail: assets.StreamDetail{StreamBreadth: breadth, ObjectAudio: "atmos", SampleRate: 96000, BitsPerSample: 24}}})
	if e != nil {
		tx.Rollback()
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	facts, e := loadDeliverySource(context.Background(), db, asset)
	if e != nil {
		t.Fatal(e)
	}
	if facts.Video.StreamBreadth != breadth || len(facts.Audio) != 1 || !facts.Audio[0].AudioDescription || facts.Audio[0].AudioVariant != "truehd_atmos" {
		t.Fatalf("facts lost: %+v", facts)
	}
	var raw string
	if e = db.QueryRow(`SELECT detail_json FROM asset_streams WHERE asset_id=? AND stream_index=0`, asset).Scan(&raw); e != nil {
		t.Fatal(e)
	}
	var st assets.Stream
	st.DecodeDetail(raw)
	if st.Detail.StreamBreadth != (assets.StreamBreadth{}) {
		t.Fatal("relational facts duplicated in JSON")
	}
	tx, e = db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	if e = assets.PersistStreams(tx, asset, 100, 2, []assets.Stream{{Index: 0, Type: "video", Codec: "h264"}}); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	rows, e := assets.ReadStreamBreadth(context.Background(), db, asset)
	if e != nil || len(rows) != 1 || rows[0] != (assets.StreamBreadth{}) {
		t.Fatal("stale stream facts survived", rows, e)
	}
}
