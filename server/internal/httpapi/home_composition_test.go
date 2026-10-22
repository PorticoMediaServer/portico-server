package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/livechannels"
	"strings"
	"testing"
	"time"
)

func TestHomeOnNowHTTPComposesCurrentProgrammeAndFencesPages(t *testing.T) {
	f := newSearchPrivacyFixture(t)
	parent, err := f.d.Identity.Authenticate(f.token)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	start := now.Add(-time.Hour).Format("20060102150405 -0700")
	end := now.Add(time.Hour).Format("20060102150405 -0700")
	source := strings.Repeat("ef", 24)
	_, err = f.d.LiveChannels.Save(context.Background(), f.d.liveAuthority(parent), livechannels.SourceInput{ID: source, RequestID: strings.Repeat("fa", 24), Name: "A visible source", Playlist: "#EXTM3U\n#EXTINF:-1 tvg-id=\"sports\" tvg-chno=\"201\" group-title=\"Sports\",Summit Sports\nhttps://provider.invalid/sports\n#EXTINF:-1 tvg-id=\"extra\" tvg-chno=\"202\" group-title=\"Sports\",Summit Sports Extra\nhttps://provider.invalid/extra\n", Guide: `<tv><programme channel="sports" start="` + start + `" stop="` + end + `"><title>Actual sports programme</title><live/></programme><programme channel="extra" start="` + start + `" stop="` + end + `"><title>Actual repeat</title><previously-shown/></programme></tv>`})
	if err != nil {
		t.Fatal(err)
	}
	f.c.Exec(`INSERT INTO live_source_settings(source_id,viewer_access)VALUES(?,'server-members')`, source)
	f.c.Exec(`INSERT INTO live_source_settings(source_id,viewer_access)VALUES(?,'server-members')`, strings.Repeat("ab", 24))
	f.h = New(f.d)
	read := func(path, token string) catalog.HomeRow {
		t.Helper()
		w := logoutHTTPRequest(f.h, "GET", path, token)
		if w.Code != 200 {
			t.Fatalf("row %d %s", w.Code, w.Body)
		}
		var row catalog.HomeRow
		if err := json.Unmarshal(w.Body.Bytes(), &row); err != nil {
			t.Fatal(err)
		}
		return row
	}
	first := read("/v1/home/rows/on_now?limit=1&group=Sports", f.token)
	if first.Kind != "live" || first.ArtworkShape != "landscape" || first.Total != 2 || first.Live == nil || len(first.Live.Channels) != 1 || first.NextCursor == "" || first.Live.Sort != "favorites" {
		t.Fatalf("on now %+v", first)
	}
	card := first.Live.Channels[0]
	if card.Channel.Name != "Summit Sports" || card.Source.ID != source || card.Channel.Mark == nil || card.Channel.Mark.Text != "SS" || len(card.Channel.Programmes) != 1 || card.Channel.Programmes[0].Title != "Actual sports programme" || !card.Channel.Programmes[0].Flags.Live {
		t.Fatalf("actual programme/identity %+v", card)
	}
	second := read("/v1/home/rows/on_now?limit=1&group=Sports&cursor="+url.QueryEscape(first.NextCursor), f.token)
	if second.Start != 1 || second.Live.Channels[0].Channel.Mark.Text != "SSE" || second.Live.Channels[0].Channel.Programmes[0].Flags.Live {
		t.Fatalf("page order/marks/real flags %+v", second)
	}
	f.c.Exec(`INSERT INTO live_channel_preferences(authority,account_id,profile_id,source_id,channel_id,favorite,hidden,revision)VALUES(?,?,?,?,?,1,0,1)`, parent.Authority, parent.AccountID, parent.ProfileID, source, second.Live.Channels[0].Channel.ID)
	stale := logoutHTTPRequest(f.h, "GET", "/v1/home/rows/on_now?limit=1&group=Sports&cursor="+url.QueryEscape(first.NextCursor), f.token)
	if stale.Code != 409 {
		t.Fatalf("favorite revision not fenced: %d %s", stale.Code, stale.Body)
	}
	reordered := read("/v1/home/rows/on_now?limit=1&group=Sports", f.token)
	if reordered.Live.Channels[0].Channel.ID != second.Live.Channels[0].Channel.ID {
		t.Fatal("favorite outside first page not promoted")
	}
	cursor, err := f.d.decodeHomeLiveCursor(reordered.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	cursor.Until = time.Now().Add(-time.Second).Unix()
	expired := f.d.encodeHomeLiveCursor(cursor)
	w := logoutHTTPRequest(f.h, "GET", "/v1/home/rows/on_now?limit=1&group=Sports&cursor="+url.QueryEscape(expired), f.token)
	if w.Code != 409 {
		t.Fatal("time-dependent stale page accepted", w.Code, w.Body.String())
	}
	f.restrictLiveTV(false)
	denied := logoutHTTPRequest(f.h, "GET", "/v1/home", f.kids.AccessToken)
	if denied.Code != 200 || strings.Contains(denied.Body.String(), `"on_now"`) || strings.Contains(denied.Body.String(), "Summit") || strings.Contains(denied.Body.String(), "Northwind News") {
		t.Fatalf("mixed Home leaked denied live metadata %d %s", denied.Code, denied.Body)
	}
	for _, suffix := range []string{"", "?limit=1&group=Sports", "?limit=1&group=Sports&cursor=" + url.QueryEscape(first.NextCursor)} {
		w := logoutHTTPRequest(f.h, "GET", "/v1/home/rows/on_now"+suffix, f.kids.AccessToken)
		if w.Code != 404 || strings.Contains(w.Body.String(), "Summit") {
			t.Fatalf("denied direct row %d %s", w.Code, w.Body)
		}
	}
	// Search's first accepted privacy fix remains intact.
	assertHTTPWithholdsLiveTV(t, readSearchPrivacyResponse(t, f.h, "/v1/search?q=Northwind", f.kids.AccessToken))
	if w := logoutHTTPRequest(f.h, "GET", "/v1/home/rows/on_now", ""); w.Code != http.StatusUnauthorized {
		t.Fatal("anonymous Home live", w.Code)
	}
}
