package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"portico.local/server/internal/livechannels"
	"strings"
	"testing"
	"time"
)

func assertSearchCursorNoPrivateAnchors(t *testing.T, cursor string, private ...string) {
	t.Helper()
	queue := []string{cursor}
	for len(queue) > 0 {
		v := queue[0]
		queue = queue[1:]
		for _, secret := range private {
			if strings.Contains(v, secret) {
				t.Fatalf("cursor exposes private anchor %q", secret)
			}
		}
		payload, _, _ := strings.Cut(v, ".")
		decoded, err := base64.RawURLEncoding.DecodeString(payload)
		if err != nil {
			continue
		}
		for _, secret := range private {
			if strings.Contains(string(decoded), secret) {
				t.Fatalf("decoded cursor exposes %q: %s", secret, decoded)
			}
		}
		var object map[string]any
		if json.Unmarshal(decoded, &object) == nil {
			for _, child := range object {
				if s, ok := child.(string); ok && s != v && len(s) > 20 {
					queue = append(queue, s)
				}
			}
		}
	}
}
func TestSearchProgrammeCursorHidesBothLayersAndHiddenAiringAnchor(t *testing.T) {
	f := newSearchPrivacyFixture(t)
	p, err := f.d.Identity.Authenticate(f.token)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	var guide strings.Builder
	guide.WriteString("<tv>")
	for i := 0; i < 129; i++ {
		fmt.Fprintf(&guide, `<programme channel="news" start="%s" stop="%s"><title>A Private Repeated Programme</title><rating system="MPAA"><value>R</value></rating></programme>`, now.Add(time.Duration(i)*time.Minute).Format("20060102150405 -0700"), now.Add(time.Duration(i+1)*time.Minute).Format("20060102150405 -0700"))
	}
	fmt.Fprintf(&guide, `<programme channel="news" start="%s" stop="%s"><title>Z Public Programme</title><rating system="MPAA"><value>G</value></rating></programme></tv>`, now.Add(130*time.Minute).Format("20060102150405 -0700"), now.Add(131*time.Minute).Format("20060102150405 -0700"))
	_, err = f.d.LiveChannels.Save(context.Background(), f.d.liveAuthority(p), livechannels.SourceInput{ID: strings.Repeat("ab", 24), RequestID: strings.Repeat("ef", 24), Name: "News", ExpectedRevision: 1, TunerCount: 1, Playlist: "#EXTM3U\n#EXTINF:-1 tvg-id=\"news\",Northwind News\nhttps://provider.invalid/news\n", Guide: guide.String()})
	if err != nil {
		t.Fatal(err)
	}
	f.c.Exec(`INSERT INTO live_source_settings(source_id,viewer_access) VALUES(?,'server-members') ON CONFLICT(source_id) DO UPDATE SET viewer_access=excluded.viewer_access`, strings.Repeat("ab", 24))
	f.restrictLiveTV(true)
	var hiddenID string
	if err = f.db.QueryRow(`SELECT id FROM live_programmes WHERE title='A Private Repeated Programme' ORDER BY start_utc,id LIMIT 1 OFFSET 127`).Scan(&hiddenID); err != nil {
		t.Fatal(err)
	}
	path := "/v1/search?q=Programme&group=programmes&limit=1"
	cursor := ""
	found := 0
	first := ""
	for i := 0; ; i++ {
		request := path
		if cursor != "" {
			request += "&cursor=" + url.QueryEscape(cursor)
		}
		page := readSearchPrivacyResponse(t, f.h, request, f.kids.AccessToken)
		if len(page.Groups) != 1 || page.Groups[0].Status != "success" {
			t.Fatal(page)
		}
		g := page.Groups[0]
		if i == 0 {
			if len(g.Items) != 0 || g.NextCursor == "" {
				t.Fatal("hidden 128-airing window falsely exhausted", g)
			}
			first = g.NextCursor
		}
		for _, e := range g.Items {
			if e.Title != "Z Public Programme" {
				t.Fatal("hidden programme identity", e)
			}
			found++
		}
		if g.NextCursor == "" {
			break
		}
		assertSearchCursorNoPrivateAnchors(t, g.NextCursor, "a private repeated programme", "A Private Repeated Programme", hiddenID, "WithinID", "WithinStart")
		if i > 10 {
			t.Fatal("did not exhaust mixed guide groups")
		}
		cursor = g.NextCursor
	}
	if found != 1 {
		t.Fatal("missing allowed control", found)
	}
	f.restrictLiveTV(false)
	w := logoutHTTPRequest(f.h, "GET", path+"&cursor="+url.QueryEscape(first), f.kids.AccessToken)
	if (w.Code != 400 && w.Code != 409) || strings.Contains(w.Body.String(), "Programme") || strings.Contains(w.Body.String(), hiddenID) {
		t.Fatal("revoked cursor disclosed guide identity", w.Code, w.Body.String())
	}
}
