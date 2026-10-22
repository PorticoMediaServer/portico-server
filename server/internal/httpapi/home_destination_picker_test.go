package httpapi

import (
	"encoding/json"
	"portico.local/server/internal/catalog"
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
