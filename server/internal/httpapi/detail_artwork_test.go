package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/metadata"
)

// Run the real HTTP request lifecycle, including cancellable upload admission.
func TestDetailAndPersonArtworkDescriptorsServeSelectedBytes(t *testing.T) {
	h, owner, member, db := editorHTTPFixtureDB(t)
	item := editorItemPublic(t, db, "Alpha")
	c := catalogtest.New(t, db)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		var entity int64
		if err := tx.QueryRow(`SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&entity); err != nil {
			return err
		}
		return compactcatalog.SetCreditsTx(ctx, tx, entity, "tmdb", []compactcatalog.Credit{{PersonKey: "tmdb:42", PersonName: "Actor", PersonSortName: "Actor", ProviderPersonID: "42", CreditID: "actor-42", CreditedName: "Actor", Role: "Lead", Department: "Acting"}})
	})
	c.Drain()
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	request := func(method, path, token, contentType string, body []byte) []byte {
		t.Helper()
		r, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		r.Header.Set("Authorization", "Bearer "+token)
		if contentType != "" {
			r.Header.Set("Content-Type", contentType)
		}
		response, err := server.Client().Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(response.Body, 12<<20))
		if err != nil {
			t.Fatal(err)
		}
		want := 200
		if token == member {
			want = 404
		}
		if response.StatusCode != want {
			t.Fatalf("%s %s: %d, want %d: %s", method, path, response.StatusCode, want, raw)
		}
		if strings.HasPrefix(path, "/v1/metadata/item/") && method == "GET" && strings.Contains(path, "?v=") && token == owner {
			if !strings.HasPrefix(response.Header.Get("Content-Type"), "image/") || len(raw) == 0 || response.Header.Get("Cache-Control") != "private, max-age=31536000, immutable" {
				t.Fatal("descriptor did not serve immutable image bytes", response.Header)
			}
		}
		return raw
	}
	selected := map[string]string{}
	for _, role := range []string{"poster", "backdrop"} {
		var before metadata.RepairState
		if err := json.Unmarshal(request("GET", "/v1/metadata/item/"+item, owner, "", nil), &before); err != nil {
			t.Fatal(err)
		}
		var body bytes.Buffer
		form := multipart.NewWriter(&body)
		fields := map[string]string{"expectedRevision": before.Revision}
		for name, value := range fields {
			if err := form.WriteField(name, value); err != nil {
				t.Fatal(err)
			}
		}
		file, err := form.CreateFormFile("file", "picture.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = file.Write(editorPNG(t, 960, 1440)); err != nil {
			t.Fatal(err)
		}
		if err = form.Close(); err != nil {
			t.Fatal(err)
		}
		var saved metadata.RepairState
		if err = json.Unmarshal(request("POST", "/v1/metadata/item/"+item+"/art/"+role+"/upload", owner, form.FormDataContentType(), body.Bytes()), &saved); err != nil {
			t.Fatal(err)
		}
		for _, art := range saved.Snapshot.Artwork {
			if art.Role == role {
				if art.Digest == art.Thumbnail {
					t.Fatal("fixture must distinguish selected and thumbnail versions")
				}
				selected[role] = art.Digest
			}
		}
		settleCompactCatalogue(t, db)
	}
	// Cast portraits are provider-owned, not an owner-upload control on a
	// movie. Bind this read fixture to real full/thumbnail objects installed
	// above, preserving the production role restriction. This proves the
	// catalogue descriptor and serving path, not provider acquisition.
	c.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,subject,provider,image_id,origin,attribution,source_fence,observed_at)
 SELECT 'portrait-fixture',a.kind,a.entity_id,'portrait','tmdb:42','fixture',candidate.image_id,candidate.origin,candidate.attribution,candidate.source_fence,candidate.observed_at
 FROM artwork_selections a JOIN artwork_candidates candidate ON candidate.id=a.candidate_id
 WHERE a.kind='item' AND a.entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND a.role='poster' AND a.subject=''`, item)
	c.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,subject,candidate_id,digest,thumbnail_digest,locked,revision,actor,observed_at)
 SELECT kind,entity_id,'portrait','tmdb:42','portrait-fixture',digest,thumbnail_digest,0,1,'fixture','now'
 FROM artwork_selections WHERE kind='item' AND entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND role='poster' AND subject=''`, item)
	var portraitDigest string
	if err := db.QueryRow(`SELECT digest FROM artwork_selections WHERE kind='item' AND entity_id=(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)) AND role='portrait' AND subject='tmdb:42'`, item).Scan(&portraitDigest); err != nil {
		t.Fatal(err)
	}
	selected["portrait"] = portraitDigest
	c.Drain()
	var detail catalog.Detail
	if err := json.Unmarshal(request("GET", "/v1/items/"+item+"/detail", owner, "", nil), &detail); err != nil {
		t.Fatal(err)
	}
	prefix := "/v1/metadata/item/" + item + "/art/"
	wantPoster := prefix + "poster?v=" + selected["poster"] + "&w=400"
	wantBackdrop := prefix + "backdrop?v=" + selected["backdrop"] + "&w=1920"
	wantPortrait := prefix + "portrait?v=" + selected["portrait"] + "&w=400&subject=tmdb%3A42"
	if detail.Item.PosterURL != wantPoster || detail.Item.BackdropURL != wantBackdrop || len(detail.Metadata.Credits) != 1 || detail.Metadata.Credits[0].PortraitURL != wantPortrait {
		t.Fatal("detail descriptors", detail.Item, detail.Metadata.Credits)
	}
	var search catalog.SearchEnvelope
	if err := json.Unmarshal(request("GET", "/v1/search?q=Alpha", owner, "", nil), &search); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, group := range search.Groups {
		for _, entry := range group.Items {
			if entry.ID == item {
				found = true
				if entry.PosterURL != detail.Item.PosterURL || entry.BackdropURL != detail.Item.BackdropURL {
					t.Fatal("search and detail disagree", entry, detail.Item)
				}
			}
		}
	}
	if !found {
		t.Fatal("actual search omitted fixture title", search)
	}
	personID := detail.Metadata.Credits[0].PersonID
	var person catalog.PersonPage
	if err := json.Unmarshal(request("GET", "/v1/people/"+personID, owner, "", nil), &person); err != nil || person.Person.PortraitURL != wantPortrait {
		t.Fatal("person descriptor", person.Person, err)
	}
	var directory catalog.PeopleDirectory
	if err := json.Unmarshal(request("GET", "/v1/people?q=Actor", owner, "", nil), &directory); err != nil || len(directory.People) != 1 || directory.People[0].PortraitURL != wantPortrait {
		t.Fatal("directory descriptor", directory, err)
	}
	emitted, err := json.Marshal([]string{detail.Item.PosterURL, detail.Item.BackdropURL, detail.Metadata.Credits[0].PortraitURL, person.Person.PortraitURL, directory.People[0].PortraitURL})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("selected artwork descriptors: %s", emitted)
	for _, path := range []string{wantPoster, wantBackdrop, wantPortrait} {
		request("GET", path, owner, "", nil)
		request("GET", path, member, "", nil)
	}
	request("GET", "/v1/items/"+item+"/detail", member, "", nil)
}
