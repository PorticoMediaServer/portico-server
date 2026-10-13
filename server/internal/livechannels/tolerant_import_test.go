package livechannels

import (
	"compress/gzip"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"portico.local/server/internal/remotemedia"
	"strings"
	"testing"
	"time"
)

func TestTolerantXMLTVKeepsUsableRowsAndInfersStop(t *testing.T) {
	in := testInput()
	in.Guide = `<?xml version="1.0"?><!DOCTYPE tv SYSTEM "xmltv.dtd"><tv><provider-extra><nested/></provider-extra><programme channel="one" start="202601011200"><title>First
 programme</title><new/><previously-shown/></programme><programme channel="one" start="202601011300" stop="202601011410"><title>Second</title></programme><programme channel="one" start="202601011400" stop="202601011500"><title>Third</title></programme><programme channel="one" start="bad"><title>Odd</title></programme><programme channel="two" start="20260101" stop="20260102"><title>Other</title></programme></tv>`
	data, e := parse(in)
	if e != nil {
		t.Fatal(e)
	}
	if len(data.programmes) != 4 || data.programmes[0].title != "First programme" || !data.programmes[0].end.Equal(data.programmes[1].start) || !data.programmes[1].end.Equal(data.programmes[2].start) {
		t.Fatalf("%+v", data.programmes)
	}
	if data.diagnostics.SkippedProgrammes != 1 || data.diagnostics.AdjustedProgrammes != 3 {
		t.Fatalf("%+v", data.diagnostics)
	}
	in.Guide = `<tv><provider-extra><x><!DOCTYPE tv [<!ENTITY x SYSTEM "file:///etc/passwd">]></x></provider-extra></tv>`
	if _, e = parse(in); e == nil {
		t.Fatal("hidden entity declaration accepted")
	}
	in.Guide = `<?xml version="1.0" encoding="ISO-8859-1"?><tv><programme channel="one" start="20260101" stop="20260102"><title>Caf` + string([]byte{0xe9}) + `</title></programme></tv>`
	data, e = parse(in)
	if e != nil || data.programmes[0].title != "Café" {
		t.Fatalf("legacy encoding: %+v %v", data, e)
	}
}
func TestTolerantPlaylistFallbackIdentityAndChannelSelection(t *testing.T) {
	in := testInput()
	in.Guide = ""
	in.Playlist = "\ufeff#EXTM3U\n#EXTINF:-1 tvg-name=\"Fallback\" group-title=\"Keep\" catchup=\"append\" catchup-days=\"7\",\nhttps://provider.invalid/one?token=old\n#EXTINF:-1 tvg-id=\"duplicate\" group-title=\"Keep\",HD\nhttps://provider.invalid/hd\n#EXTINF:-1 tvg-id=\"duplicate\" group-title=\"Keep\",SD\nhttps://provider.invalid/sd\n#EXTINF:-1 tvg-id=\"bad\",Bad\nfile:///private\n#EXTINF:-1 tvg-id=\"other\" group-title=\"Drop\",Other\nhttps://provider.invalid/other\n"
	in.EnabledGroups = []string{"Keep"}
	data, e := parse(in)
	if e != nil {
		t.Fatal(e)
	}
	if len(data.channels) != 3 || data.channels[0].catchup != "append" || data.diagnostics.SkippedChannels != 1 {
		t.Fatalf("%+v", data)
	}
	key := data.channels[0].key
	in.Playlist = strings.Replace(in.Playlist, "token=old", "token=new", 1)
	data, e = parse(in)
	if e != nil || data.channels[0].key != key {
		t.Fatal("rotating query token changed fallback identity")
	}
}
func TestLargeLineupAndOwnerLimitsKeepGoodChannels(t *testing.T) {
	in := testInput()
	in.Guide = ""
	var playlist strings.Builder
	playlist.WriteString("#EXTM3U\n")
	for i := 0; i < 3000; i++ {
		fmt.Fprintf(&playlist, "#EXTINF:-1 tvg-id=\"%d\",Channel %d\nhttps://provider.invalid/%d\n", i, i, i)
	}
	in.Playlist = playlist.String()
	data, e := parse(in)
	if e != nil || len(data.channels) != 3000 {
		t.Fatalf("large lineup: %d %v", len(data.channels), e)
	}
	in.Limits.Channels = 200
	data, e = parse(in)
	if e != nil || len(data.channels) != 200 || data.diagnostics.SkippedChannels != 2800 {
		t.Fatalf("owner limit: %d %+v %v", len(data.channels), data.diagnostics, e)
	}
}
func TestGzipGuideMagicWithoutContentEncoding(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g := gzip.NewWriter(w)
		_, _ = g.Write([]byte("<tv/>"))
		_ = g.Close()
	}))
	defer server.Close()
	ctx := context.Background()
	approval, e := (remotemedia.Policy{}).Approve(ctx, server.URL+"/")
	if e != nil {
		t.Fatal(e)
	}
	data, e := ReadSource(ctx, server.URL+"/guide.xml.gz", remotemedia.Policy{Approvals: []remotemedia.Approval{approval}}, 4096)
	if e != nil || string(data) != "<tv/>" {
		t.Fatalf("%q %v", data, e)
	}
}
func TestChannelSharingCountsOneTunerAndReportsEveryReader(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	a := testAuthority("owner", true, true)
	source, e := store.Save(ctx, a, testInput())
	if e != nil {
		t.Fatal(e)
	}
	owner := Owner{Authority: "local", AccountID: "account", ProfileID: "profile"}
	for i, kind := range []string{"live", "live", "recording"} {
		e = store.transaction(ctx, a, true, func(tx *sql.Tx) error {
			_, err := AcquireTx(ctx, tx, Allocation{SourceID: source.ID, ChannelID: "one", ResourceID: fmt.Sprint(i), Kind: kind, Owner: owner}, time.Now())
			return err
		})
		if e != nil {
			t.Fatalf("reader %d: %v", i, e)
		}
	}
	e = store.transaction(ctx, a, true, func(tx *sql.Tx) error {
		_, err := AcquireTx(ctx, tx, Allocation{SourceID: source.ID, ChannelID: "two", ResourceID: "different", Kind: "live", Owner: owner}, time.Now())
		return err
	})
	if !errors.Is(e, ErrCapacity) {
		t.Fatalf("another channel exceeded capacity: %v", e)
	}
	var count int
	if e = store.db.QueryRow(`SELECT count(*) FROM live_allocations WHERE state='active'`).Scan(&count); e != nil || count != 3 {
		t.Fatalf("readers: %d %v", count, e)
	}
}

func TestLargeProgrammeGuideUsesDiskSpoolAndBoundedPreview(t *testing.T) {
	in := testInput()
	var guide strings.Builder
	guide.WriteString("<tv>")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < programmeMemoryRows+2; i++ {
		a := start.Add(time.Duration(i) * time.Minute)
		fmt.Fprintf(&guide, `<programme channel="one" start="%s" stop="%s"><title>Episode %d</title></programme>`, a.Format("20060102150405 -0700"), a.Add(time.Minute).Format("20060102150405 -0700"), i)
	}
	guide.WriteString("</tv>")
	in.Guide = guide.String()
	data, e := parse(in)
	if e != nil {
		t.Fatal(e)
	}
	defer data.close()
	if data.spool == nil || len(data.programmes) != 0 || data.programmeCount() != programmeMemoryRows+2 {
		t.Fatalf("spool=%v memory=%d rows=%d", data.spool != nil, len(data.programmes), data.programmeCount())
	}
	preview, e := previewParsed(data)
	if e != nil || preview.Programmes != programmeMemoryRows+2 {
		t.Fatalf("%+v %v", preview, e)
	}
	count := 0
	e = data.batches(context.Background(), 128, func(batch []parsedProgramme) error {
		if len(batch) > 128 {
			t.Fatal("unbounded batch")
		}
		count += len(batch)
		return nil
	})
	if e != nil || count != preview.Programmes {
		t.Fatalf("%d %v", count, e)
	}
}
