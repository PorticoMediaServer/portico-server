package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"portico.local/server/internal/identity"
)

func retireProfileInHTTPTest(t *testing.T, s *identity.Service, ctx context.Context, bearer, id string, expected int64, deleteSaved bool) error {
	t.Helper()
	preview, err := s.PreviewDirectProfileDeletion(ctx, bearer, id)
	if err != nil {
		return err
	}
	if _, err = s.RetireDirectProfile(ctx, bearer, id, identity.ProfileRetirementRequest{ExpectedRevision: expected, Lifetime: preview.Lifetime, DeleteSavedResources: deleteSaved}); err != nil {
		return err
	}
	for i := 0; i < 20000; i++ {
		worked, err := s.PurgeProfileStep(ctx)
		if err != nil {
			return err
		}
		if !worked {
			return nil
		}
	}
	t.Fatal("profile purge failed to converge")
	return nil
}

// Exercise the shipped typed routes, including lost-response retry and the
// physical completion boundary. Transport success alone never means Removed.
func TestProfileRetirementHTTPRoutesKeepExactLifetimeUntilPhysicalCompletion(t *testing.T) {
	f := administrationFixtureNew(t)
	doc, err := f.deps.Identity.CreateDirectProfile(t.Context(), f.owner, "Kids", "mint")
	if err != nil {
		t.Fatal(err)
	}
	var kid identity.DirectProfile
	for _, p := range doc.Profiles {
		if !p.Primary {
			kid = p
		}
	}
	path := "/v1/direct/profiles/" + kid.ID
	w := f.call("GET", path+"/deletion-preview", f.owner, "")
	if w.Code != http.StatusOK {
		t.Fatalf("preview: %d %s", w.Code, w.Body)
	}
	var preview identity.ProfileDeletionPreview
	if err = json.Unmarshal(w.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if len(preview.Lifetime) != 32 {
		t.Fatalf("missing lifetime: %+v", preview)
	}
	body := fmt.Sprintf(`{"expectedRevision":%d,"lifetime":%q,"deleteSavedResources":true}`, preview.Revision, preview.Lifetime)
	assertState := func(method, url, body, want string) {
		t.Helper()
		r := f.call(method, url, f.owner, body)
		var state identity.ProfileRetirement
		if r.Code != http.StatusOK || json.Unmarshal(r.Body.Bytes(), &state) != nil || state.ProfileID != kid.ID || state.Lifetime != preview.Lifetime || state.State != want {
			t.Fatalf("%s %s: %d %s", method, url, r.Code, r.Body)
		}
	}
	assertState("DELETE", path, body, "removing")
	assertState("DELETE", path, body, "removing")
	status := path + "/retirement?lifetime=" + preview.Lifetime
	assertState("GET", status, "", "removing")
	wrong := f.call("GET", path+"/retirement?lifetime="+strings.Repeat("0", 32), f.owner, "")
	if wrong.Code != http.StatusConflict {
		t.Fatalf("wrong lifetime: %d %s", wrong.Code, wrong.Body)
	}
	if r := f.call("GET", status, f.member, ""); r.Code == http.StatusOK {
		t.Fatalf("member learned retirement: %s", r.Body)
	}
	for i := 0; i < 20000; i++ {
		worked, e := f.deps.Identity.PurgeProfileStep(t.Context())
		if e != nil {
			t.Fatal(e)
		}
		if !worked {
			break
		}
		if i == 19999 {
			t.Fatal("purge did not converge")
		}
	}
	assertState("GET", status, "", "removed")
	assertState("DELETE", path, body, "removed")
	var deleted bool
	var name, art string
	var pin, libraries any
	if err = f.deps.DB.QueryRow(`SELECT deleted,name,art,pin_hash,allowed_libraries FROM direct_profiles WHERE id=?`, kid.ID).Scan(&deleted, &name, &art, &pin, &libraries); err != nil || !deleted || name != "Kids" || art != "slate" || pin != nil || libraries != nil {
		t.Fatalf("unsanitized history label: %v %q %q %v %v %v", deleted, name, art, pin, libraries, err)
	}
}
