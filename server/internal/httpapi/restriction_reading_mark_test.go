package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"portico.local/server/internal/identity"
)

// Both registry entries retain non-vacuous real-target reads/writes, then
// compare concealed title/library refusals with a truly absent target.
func readingMarkRestrictionCheck(t *testing.T) {
	for _, format := range []string{"pdf", "cbz"} {
		t.Run(format, func(t *testing.T) {
			kind := "book"
			if format == "cbz" {
				kind = "comic"
			}
			r := newReadingFixture(t, kind, format)
			h := New(r.d)
			ctx := context.Background()
			owner := readingOwnerLogin(t, r)
			profiles, err := r.f.id.CreateDirectProfile(ctx, owner.AccountToken, "Mark reader", "blue")
			if err != nil {
				t.Fatal(err)
			}
			var child identity.DirectProfile
			for _, p := range profiles.Profiles {
				if !p.Primary && p.Name == "Mark reader" {
					child = p
				}
			}
			if child.ID == "" {
				t.Fatal("no real child profile")
			}
			selected, err := r.f.id.SelectDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectSelection{})
			if err != nil {
				t.Fatal(err)
			}
			token := selected.Session.AccessToken
			path := func(id string) string { return "/v1/items/" + id + "/reading/mark" }
			get := readingHTTP(t, h, "GET", path(r.item), token, "", nil)
			var initial ReadingMarkResponse
			if get.Code != 200 || json.Unmarshal(get.Body.Bytes(), &initial) != nil || initial.ItemID != r.item || initial.ServerID != r.f.id.ID() || initial.ViewerFence == "" || initial.Read {
				t.Fatal("real initial mark", get.Code, get.Body.String())
			}
			opened := readingHTTP(t, h, "POST", "/v1/reading/sessions", token, "", ReadingSessionRequest{r.item, r.source})
			var manifest ReadingSessionResponse
			if opened.Code != 200 || json.Unmarshal(opened.Body.Bytes(), &manifest) != nil || manifest.SessionID == "" {
				t.Fatal("actual Reading session", opened.Code, opened.Body.String())
			}
			page := int64(0)
			saved := readingHTTP(t, h, "PUT", "/v1/reading/sessions/"+manifest.SessionID+"/progress", token, manifest.Revision, ReadingProgressRequest{Location: ReadingLocation{Format: format, PageIndex: &page, Progression: .25}})
			if saved.Code != 200 {
				t.Fatal("actual saved location", saved.Code, saved.Body.String())
			}
			before := readingRestrictionProgress(t, r)
			if before == "null" || before == "[]" {
				t.Fatal("saved-location control was vacuous")
			}
			yes := true
			body := ReadingMarkRequest{OperationID: "matrix-mark-read", ExpectedRevision: initial.Revision, Read: &yes}
			put := readingHTTP(t, h, "PUT", path(r.item), token, "", body)
			var marked ReadingMarkResponse
			if put.Code != 200 || json.Unmarshal(put.Body.Bytes(), &marked) != nil || !marked.Read || marked.ItemID != r.item || marked.OperationID != body.OperationID || marked.Revision <= initial.Revision {
				t.Fatal("real explicit read mark", put.Code, put.Body.String())
			}
			replay := readingHTTP(t, h, "PUT", path(r.item), token, "", body)
			var replayed ReadingMarkResponse
			if replay.Code != 200 || json.Unmarshal(replay.Body.Bytes(), &replayed) != nil || replayed.Revision != marked.Revision || !replayed.Read {
				t.Fatal("immutable operation replay", replay.Code, replay.Body.String())
			}
			if readingRestrictionProgress(t, r) != before {
				t.Fatal("explicit mark changed saved Reading location")
			}
			other := readingHTTP(t, h, "GET", path(r.item), r.f.owner.AccessToken, "", nil)
			var otherMark ReadingMarkResponse
			if other.Code != 200 || json.Unmarshal(other.Body.Bytes(), &otherMark) != nil || otherMark.Read {
				t.Fatal("mark escaped its exact profile", other.Code, other.Body.String())
			}
			for _, method := range []string{"GET", "PUT"} {
				if w := readingHTTP(t, h, method, path(r.item), "", "", body); w.Code != 401 {
					t.Fatal("anonymous mark", w.Code, w.Body.String())
				}
			}
			refuse := func() {
				t.Helper()
				for _, method := range []string{"GET", "PUT"} {
					hidden := readingHTTP(t, h, method, path(r.item), token, "", body)
					absent := readingHTTP(t, h, method, path(absentTwin(r.item)), token, "", body)
					if hidden.Code != 404 || absent.Code != 404 || normalizedAnswer(hidden.Body.String(), r.item) != normalizedAnswer(absent.Body.String(), absentTwin(r.item)) {
						t.Fatal("hidden/absent mark differs", method, hidden.Code, hidden.Body.String(), absent.Code, absent.Body.String())
					}
					for _, marker := range []string{r.item, r.source, r.library, r.path} {
						if marker != "" && strings.Contains(hidden.Body.String(), marker) {
							t.Fatal("hidden mark leaked target", hidden.Body.String())
						}
					}
				}
				if readingRestrictionProgress(t, r) != before {
					t.Fatal("refused mark changed saved Reading location")
				}
			}
			if _, err = r.f.id.SetProfileRestrictions(ctx, owner.AccountToken, child.ID, identity.ProfileRestrictionEdit{ExpectedRevision: 1, AllowUnrated: false}); err != nil {
				t.Fatal(err)
			}
			refuse()
			if _, err = r.f.id.SetProfileRestrictions(ctx, owner.AccountToken, child.ID, identity.ProfileRestrictionEdit{ExpectedRevision: 2, AllowUnrated: true}); err != nil {
				t.Fatal(err)
			}
			if _, err = r.f.id.EditDirectProfile(ctx, owner.AccountToken, child.ID, identity.DirectProfileEdit{ExpectedRevision: child.Revision, Policy: &identity.ProfilePolicy{AllowedLibraries: []string{}}}); err != nil {
				t.Fatal(err)
			}
			refuse()
		})
	}
}
