package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
)

func TestSearchAdmissionIsRealCurrentViewerHTTP(t *testing.T) {
	f := newSearchPrivacyFixture(t)
	f.d.admission = newAdmission()
	p, err := f.d.Identity.Authenticate(f.token)
	if err != nil {
		t.Fatal(err)
	}
	held := []func(){}
	for {
		done, ok := f.d.admission.acquireSearch(identity.PersonalKey(p.Viewer))
		if !ok {
			break
		}
		held = append(held, done)
	}
	defer func() {
		for _, done := range held {
			done()
		}
	}()
	// Drive the real handler with a deliberately occupied admission ledger. No
	// SQL/parser stand-in controls the overload result.
	h := http.HandlerFunc(f.d.searchHandler)
	w := logoutHTTPRequest(h, "GET", "/v1/search?q=Northwind", f.token)
	if w.Code != 429 || w.Header().Get("Retry-After") != "1" {
		t.Fatalf("real admission %d %s", w.Code, w.Body.String())
	}
	var body struct {
		Error struct {
			Code string `json:"code"`
		}
	}
	if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error.Code != "search_busy" {
		t.Fatal("typed search_busy absent", w.Body.String())
	}
	for _, done := range held {
		done()
	}
	held = nil
	if w = logoutHTTPRequest(h, "GET", "/v1/search?q=Northwind", f.token); w.Code != 200 {
		t.Fatalf("admission did not recover %d %s", w.Code, w.Body.String())
	}
}

func TestSearchHTTPWireEvidence(t *testing.T) {
	f := newSearchPrivacyFixture(t)
	ctx := context.Background()
	film := f.c.Movie(f.c.Handle("movies"), "/movies/harbor.mkv", "Harbor", 2025)
	f.c.Attributes(film.ID, "contentRating", "G")
	f.c.Write(func(ctx context.Context, tx *sql.Tx) error {
		return compactcatalog.SetCreditsTx(ctx, tx, film.ID, "fixture", []compactcatalog.Credit{{PersonKey: "fixture:northwind", PersonName: "Northwind Actor", PersonSortName: "Northwind Actor", CreditedName: "Northwind Actor", CreditID: "fixture-credit", Department: "Acting", Role: "Lead"}})
	})
	key := identity.PersonalKey(f.kids.Viewer)
	f.c.Exec(`INSERT INTO personal_items(profile_id,item_id,watchlisted,favorite,revision) VALUES(?,?,1,0,1)`, key, film.ID)
	f.c.Exec(`INSERT INTO progress(profile_id,item_id,position,completed,playback_id) VALUES(?,?,12500,0,'wire-evidence')`, key, film.ID)
	f.c.Drain()
	if _, err := f.d.Catalog.ClassifyPendingRatings(ctx); err != nil {
		t.Fatal(err)
	}
	f.c.Drain()
	f.restrictLiveTV(false)
	path := "/v1/search?q=Northwind&groups=movies,shows,episodes,people,artists,albums,songs,books,live-tv,programmes"
	w := logoutHTTPRequest(f.h, "GET", path, f.kids.AccessToken)
	if w.Code != 200 {
		t.Fatalf("wire %d %s", w.Code, w.Body.String())
	}
	var out catalog.SearchEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, g := range out.Groups {
		for _, e := range g.Items {
			if e.ID == film.Public {
				found = true
				if e.MatchedOn == nil || e.MatchedOn.Field != "credit" || e.MatchedOn.Text != "Northwind Actor" || e.Watchlisted == nil || !*e.Watchlisted {
					t.Fatalf("source facts missing %+v", e)
				}
			}
		}
	}
	if !found {
		t.Fatal("source film absent")
	}
	// Optional evidence export contains only the response body and fixture IDs,
	// never the viewer's token/session/credentials.
	if path := os.Getenv("PORTICO_SEARCH_EVIDENCE"); path != "" {
		raw, err := json.MarshalIndent(map[string]any{"response": json.RawMessage(w.Body.Bytes()), "scope": map[string]string{"serverId": f.d.Identity.ID(), "viewerId": key}, "filmId": film.Public}, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSearchTopUnicodeHTTPWire(t *testing.T) {
	f := newSearchPrivacyFixture(t)
	film := f.c.Movie(f.c.Handle("movies"), "/movies/sigma.mkv", "ΟΣ", 2026)
	f.c.Attributes(film.ID, "contentRating", "G")
	f.c.Drain()
	records := []map[string]any{}
	for _, q := range []string{"οσ", "ος", "ΟΣ"} {
		w := logoutHTTPRequest(f.h, "GET", "/v1/search?q="+q+"&groups=movies", f.kids.AccessToken)
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		var page catalog.SearchEnvelope
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if (page.Top != nil) != (q != "οσ") {
			t.Fatal("noncanonical Top", q, page.Top)
		}
		if len(page.Groups) != 1 || len(page.Groups[0].Items) != 1 || page.Groups[0].Items[0].ID != film.Public {
			t.Fatal("Unicode candidate missing", page)
		}
		records = append(records, map[string]any{"q": q, "response": json.RawMessage(w.Body.Bytes()), "scope": map[string]string{"serverId": f.d.Identity.ID(), "viewerId": identity.PersonalKey(f.kids.Viewer)}})
	}
	if path := os.Getenv("PORTICO_SEARCH_UNICODE_WIRE"); path != "" {
		raw, err := json.MarshalIndent(records, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
}
