package httpapi

import (
	"bytes"
	"encoding/json"
	"image"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"portico.local/server/internal/metadata"
)

func TestArtworkWidthIncludesLargerStorageBucket(t *testing.T) {
	for requested, want := range map[int]int{1: 400, 400: 400, 401: 800, 800: 800, 801: 1920, 1920: 1920, 1921: 2560, 2560: 2560, 9000: 2560} {
		if got := artworkWidth(httptest.NewRequest("GET", "/art?w="+strconv.Itoa(requested), nil)); got != want {
			t.Fatal(requested, got, want)
		}
	}
}

func TestArtworkSettingsUploadAndServingUseActualCurrentSize(t *testing.T) {
	h, owner, member, db := editorHTTPFixtureDB(t)
	item := editorItemPublic(t, db, "Alpha")
	for _, choice := range []struct{ setting, source, stored, ceiling int }{
		{1280, 1400, 1280, 500000},
		{1920, 1400, 1400, 500000},
		{2560, 2700, 2560, 888888},
	} {
		t.Run(strconv.Itoa(choice.setting), func(t *testing.T) {
			read := editorRequest(t, h, "GET", "/v1/admin/metadata/settings", owner, "", nil)
			if read.Code != 200 {
				t.Fatal(read.Code, read.Body.String())
			}
			tl6AssertSpecResponse(t, "GET", "/v1/admin/metadata/settings", read)
			var current metadata.MetadataSettings
			if err := json.Unmarshal(read.Body.Bytes(), &current); err != nil {
				t.Fatal(err)
			}
			body, err := json.Marshal(metadata.MetadataSettingsUpdate{ExpectedRevision: current.Revision, LocalArtworkLimitBytes: current.LocalArtworkLimitBytes, IncludeAdult: current.IncludeAdult, ArtworkMaxEdge: &choice.setting})
			if err != nil {
				t.Fatal(err)
			}
			saved := editorRequest(t, h, "PUT", "/v1/admin/metadata/settings", owner, "application/json", body)
			if saved.Code != 200 {
				t.Fatal(saved.Code, saved.Body.String())
			}
			tl6AssertSpecResponse(t, "PUT", "/v1/admin/metadata/settings", saved)
			state := editorState(t, h, owner, item)
			body, contentType := editorUpload(t, state.Revision, editorPNG(t, choice.source, choice.source/2))
			uploaded := editorRequest(t, h, "POST", "/v1/metadata/item/"+item+"/art/poster/upload", owner, contentType, body)
			if uploaded.Code != 200 {
				t.Fatal(uploaded.Code, uploaded.Body.String())
			}
			settleCompactCatalogue(t, db)
			base := "/v1/items/" + item + "/art/poster"
			full := editorRequest(t, h, "GET", base+"?w=2560", owner, "", nil)
			if full.Code != 200 || full.Body.Len() > choice.ceiling {
				t.Fatal("stored picture", full.Code, full.Body.Len())
			}
			version := strings.Trim(full.Header().Get("ETag"), `"`)
			if version == "" {
				t.Fatal("stored picture has no validator")
			}
			for _, width := range []int{400, 800, 1920, 2560} {
				path := base + "?v=" + version + "&w=" + strconv.Itoa(width) + "&size=thumbnail"
				response := editorRequest(t, h, "GET", path, owner, "", nil)
				cfg, _, err := image.DecodeConfig(bytes.NewReader(response.Body.Bytes()))
				if response.Code != 200 || err != nil || max(cfg.Width, cfg.Height) != min(width, choice.stored) {
					t.Fatal("served picture", width, response.Code, cfg, err)
				}
				if width >= choice.stored && !bytes.Equal(response.Body.Bytes(), full.Body.Bytes()) {
					t.Fatal("at or above stored edge did not return stored bytes", width)
				}
				if response.Header().Get("Cache-Control") != "private, max-age=31536000, immutable" {
					t.Fatal("versioned cache policy", response.Header())
				}
				tag := response.Header().Get("ETag")
				cached := editorRequestWithHeader(t, h, "GET", path, owner, map[string]string{"If-None-Match": tag})
				if cached.Code != 304 || cached.Body.Len() != 0 {
					t.Fatal("conditional artwork", width, cached.Code)
				}
				denied := editorRequestWithHeader(t, h, "GET", path, member, map[string]string{"If-None-Match": tag})
				if denied.Code != 404 {
					t.Fatal("private conditional artwork", width, denied.Code)
				}
			}
		})
	}
}
