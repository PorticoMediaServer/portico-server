package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/livechannels"
)

// Real current-viewer HTTP over the real XMLTV import and guide fact producer.
// This is deliberately not a DTO/parser fixture.
func TestSearchProgrammesRequireLiveTV(t *testing.T) {
	f := newSearchPrivacyFixture(t)
	ctx := context.Background()
	p, err := f.d.Identity.Authenticate(f.token)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	var guide strings.Builder
	guide.WriteString("<tv>")
	for i := 0; i < 3; i++ {
		fmt.Fprintf(&guide, `<programme channel="news" start="%s" stop="%s"><title>Northwind Programme %d</title><sub-title>Family edition</sub-title><rating system="MPAA"><value>G</value></rating><live/></programme>`, now.Add(time.Duration(i)*time.Hour).Format("20060102150405 -0700"), now.Add(time.Duration(i+1)*time.Hour).Format("20060102150405 -0700"), i)
	}
	fmt.Fprintf(&guide, `<programme channel="news24" start="%s" stop="%s"><title>Northwind Adult Programme</title><rating system="MPAA"><value>R</value></rating></programme>`, now.Format("20060102150405 -0700"), now.Add(time.Hour).Format("20060102150405 -0700"))
	guide.WriteString("</tv>")
	_, err = f.d.LiveChannels.Save(ctx, f.d.liveAuthority(p), livechannels.SourceInput{ID: strings.Repeat("ab", 24), RequestID: strings.Repeat("ef", 24), Name: "News", ExpectedRevision: 1, TunerCount: 1, Playlist: "#EXTM3U\n#EXTINF:-1 tvg-id=\"news\",Northwind News\nhttps://provider.invalid/news\n#EXTINF:-1 tvg-id=\"news24\",Northwind News 24\nhttps://provider.invalid/news24\n", Guide: guide.String()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`INSERT INTO live_source_settings(source_id,viewer_access) VALUES(?,'server-members') ON CONFLICT(source_id) DO UPDATE SET viewer_access=excluded.viewer_access`, strings.Repeat("ab", 24)); err != nil {
		t.Fatal(err)
	}
	if _, err = f.d.LiveChannels.SearchProgrammes(ctx, f.d.guideDirectoryAuthority(p, "fixture"), nil, "Northwind", 1, "", nil); err != nil {
		t.Fatal("guide programme lookup", err)
	}
	f.restrictLiveTV(true)
	path := "/v1/search?q=Northwind&group=programmes&limit=1"
	control := readSearchPrivacyResponse(t, f.h, path, f.kids.AccessToken)
	for len(control.Groups) == 1 && control.Groups[0].Status == "success" && len(control.Groups[0].Items) == 0 && control.Groups[0].NextCursor != "" {
		control = readSearchPrivacyResponse(t, f.h, path+"&cursor="+url.QueryEscape(control.Groups[0].NextCursor), f.kids.AccessToken)
	}
	if len(control.Groups) != 1 || control.Groups[0].Status != "success" || len(control.Groups[0].Items) != 1 || control.Groups[0].NextCursor == "" {
		t.Fatalf("allowed programme control: %+v", control)
	}
	normalized := readSearchPrivacyResponse(t, f.h, "/v1/search?q=%20%20Family%20%20%20edition%20&group=programmes", f.kids.AccessToken)
	if len(normalized.Groups) != 1 || len(normalized.Groups[0].Items) != 3 || normalized.Query.Q != "Family edition" {
		t.Fatalf("normalized programme subtitle Search: %+v", normalized.Groups)
	}
	item := control.Groups[0].Items[0]
	if item.Programme == nil || item.Channel == nil || !item.Programme.Live || item.Programme.Subtitle != "Family edition" || item.Channel.SourceID == "" {
		t.Fatalf("guide facts missing: %+v", item)
	}
	replay := path + "&cursor=" + url.QueryEscape(control.Groups[0].NextCursor)
	readSearchPrivacyResponse(t, f.h, replay, f.kids.AccessToken)
	all := readSearchPrivacyResponse(t, f.h, "/v1/search?q=Northwind", f.kids.AccessToken)
	for _, g := range all.Groups {
		for _, e := range g.Items {
			if strings.Contains(e.Title, "Adult") || strings.Contains(e.Title, "Private") {
				t.Fatal("parental/library identity leak", e)
			}
		}
	}
	if all.Top != nil && (all.Top.Entry.Kind == "programme" || all.Top.Entry.Kind == "channel") {
		t.Fatal("live identity became Top")
	}
	// Change the current source policy without changing the child's token.
	if _, err = f.db.Exec(`UPDATE live_source_settings SET viewer_access='owner-only' WHERE source_id=?`, strings.Repeat("ab", 24)); err != nil {
		t.Fatal(err)
	}
	deniedSource := readSearchPrivacyResponse(t, f.h, "/v1/search?q=Northwind&groups=live-tv,programmes", f.kids.AccessToken)
	for _, g := range deniedSource.Groups {
		if len(g.Items) > 0 || g.TotalCount != 0 {
			t.Fatal("owner-only source leaked", g)
		}
	}
	if w := logoutHTTPRequest(f.h, "GET", replay, f.kids.AccessToken); w.Code != http.StatusConflict && w.Code != http.StatusBadRequest {
		t.Fatalf("source policy cursor replay: %d %s", w.Code, w.Body.String())
	}
	f.restrictLiveTV(false)
	for _, suffix := range []string{"", "&groups=live-tv,programmes", "&groups=programmes", "&group=movies"} {
		out := readSearchPrivacyResponse(t, f.h, "/v1/search?q=Northwind"+suffix, f.kids.AccessToken)
		assertHTTPWithholdsLiveTV(t, out)
		for _, g := range out.Groups {
			if g.ID == "programmes" || g.EntityKind == "programme" {
				t.Fatal("denied programme count/group", g)
			}
		}
		for _, c := range out.Capabilities.Groups {
			if c.ID == "programmes" {
				t.Fatal("denied programme capability", c)
			}
		}
		if out.Top != nil && (out.Top.Entry.Channel != nil || out.Top.Entry.Programme != nil) {
			t.Fatal("denied Top identity")
		}
	}
	for _, suffix := range []string{"&group=live-tv", "&group=programmes", "&group=programmes&cursor=" + url.QueryEscape(control.Groups[0].NextCursor)} {
		w := logoutHTTPRequest(f.h, "GET", "/v1/search?q=Northwind"+suffix, f.kids.AccessToken)
		if w.Code != 400 && w.Code != 409 {
			t.Fatalf("explicit denied programme %d %s", w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "Northwind") {
			t.Fatal("denial disclosed identity")
		}
	}
	deniedDiscovery := logoutHTTPRequest(f.h, "GET", "/v1/search/discovery", f.kids.AccessToken)
	if deniedDiscovery.Code != 200 || strings.Contains(deniedDiscovery.Body.String(), "Northwind News") || strings.Contains(deniedDiscovery.Body.String(), "Northwind Programme") {
		t.Fatalf("denied empty-state On now: %d %s", deniedDiscovery.Code, deniedDiscovery.Body.String())
	}
	ownerDiscovery := logoutHTTPRequest(f.h, "GET", "/v1/search/discovery", f.token)
	if ownerDiscovery.Code != 200 || !strings.Contains(ownerDiscovery.Body.String(), "Northwind News") || !strings.Contains(ownerDiscovery.Body.String(), "Northwind Programme") {
		t.Fatalf("allowed On now control: %d %s", ownerDiscovery.Code, ownerDiscovery.Body.String())
	}
	// The owner control still resolves all four stored guide titles.
	owner := readSearchPrivacyResponse(t, f.h, "/v1/search?q=Northwind&group=programmes", f.token)
	if len(owner.Groups) != 1 || len(owner.Groups[0].Items) != 4 {
		t.Fatalf("owner guide facts lost: %+v", owner.Groups)
	}
}
