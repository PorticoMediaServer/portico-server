package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"portico.local/server/internal/catalog"
	"strings"
	"testing"
)

func TestHomeDestinationPickerHTTPUsesLiveActorAndBoundedContract(t *testing.T) {
	handler, owner, member := editorHTTPFixture(t)
	body, _ := json.Marshal(catalog.HomeDestinationPickerRequest{Kind: "playlist", Query: "", Limit: 40})
	response := editorRequest(t, handler, "POST", "/v1/home/destination-picker", owner, "application/json", body)
	if response.Code != 200 {
		t.Fatalf("picker read: %d %s", response.Code, response.Body.String())
	}
	tl6AssertSpecResponse(t, "POST", "/v1/home/destination-picker", response)
	var page catalog.HomeDestinationPickerPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) > 40 || page.ViewerFence == "" {
		t.Fatal(page)
	}
	invalid, _ := json.Marshal(catalog.HomeDestinationPickerRequest{Kind: "playlist", Query: "", Limit: 41})
	refused := editorRequest(t, handler, "POST", "/v1/home/destination-picker", member, "application/json", invalid)
	if refused.Code == 200 {
		t.Fatal("oversized picker read accepted")
	}
	unauthenticated := editorRequest(t, handler, "POST", "/v1/home/destination-picker", "", "application/json", body)
	if unauthenticated.Code == 200 {
		t.Fatal("anonymous picker disclosed personal resources")
	}
}

func TestHomeDestinationPickerHidesRestrictedMembershipAndArtwork(t *testing.T) {
	m := buildRestrictionMatrix(t)
	hidden := m.hiddenItems["movie"][0]
	for kind, destination := range map[string]string{"playlist": m.playlist, "collection": m.savedResource} {
		for _, selected := range []string{hidden, absentTwin(hidden)} {
			body := fmt.Sprintf(`{"kind":%q,"limit":40,"itemIds":[%q,%q]}`, kind, selected, m.visibleMovie)
			status, raw := m.call("POST", "/v1/home/destination-picker", body, "restricted")
			if status != http.StatusOK || strings.Contains(raw, hidden) {
				t.Fatalf("%s picker disclosed restricted content: %d %s", kind, status, raw)
			}
			var page catalog.HomeDestinationPickerPage
			if err := json.Unmarshal([]byte(raw), &page); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range page.Rows {
				if row.ID == destination {
					found = true
					if row.AlreadyIn == nil || *row.AlreadyIn != 1 || len(row.Artwork) != 1 || row.Artwork[0].ID != m.visibleMovie {
						t.Fatalf("%s picker did not retain only visible membership/artwork: %+v", kind, row)
					}
				}
			}
			if !found {
				t.Fatalf("%s picker never reached the mixed-content destination", kind)
			}
		}
	}
}
