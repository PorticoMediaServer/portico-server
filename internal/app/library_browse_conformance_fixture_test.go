package app

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestBrowseConformanceFixtureTracksCanonicalProductContract(t *testing.T) {
	bytes, err := os.ReadFile("../../api/openapi/fixtures/library-browse-conformance.json")
	if err != nil {
		t.Fatalf("read conformance fixture: %v", err)
	}
	var fixture struct {
		APIVersion     string `json:"apiVersion"`
		ActionRevision string `json:"actionRevision"`
		LibraryKinds   []struct {
			ID     string   `json:"id"`
			Pivots []string `json:"pivots"`
		} `json:"libraryKinds"`
		InvalidRequests      []map[string]any `json:"invalidRequests"`
		PermissionReductions []map[string]any `json:"permissionReductions"`
		ActionRules          struct {
			AbsenceIsAuthoritative    bool   `json:"absenceIsAuthoritative"`
			APIBulkExecution          string `json:"apiBulkExecution"`
			ClientFlowsOwnSelectionUI bool   `json:"clientFlowsOwnSelectionUI"`
		} `json:"actionRules"`
		NavigationOwnership struct {
			PrimaryDestinationsAndVisualPlacement string `json:"primaryDestinationsAndVisualPlacement"`
			InSectionTabsAndComposition           string `json:"inSectionTabsAndComposition"`
			ServerDrivenPrimaryNavigationManifest bool   `json:"serverDrivenPrimaryNavigationManifest"`
			RequiredSectionTabsAreHideable        bool   `json:"requiredSectionTabsAreHideable"`
		} `json:"navigationOwnership"`
	}
	if err := json.Unmarshal(bytes, &fixture); err != nil {
		t.Fatalf("decode conformance fixture: %v", err)
	}
	if fixture.APIVersion != productContractRevision {
		t.Fatalf("fixture API version=%q contract=%q", fixture.APIVersion, productContractRevision)
	}
	if fixture.ActionRevision != mediaActionRevision {
		t.Fatalf("fixture action revision=%q contract=%q", fixture.ActionRevision, mediaActionRevision)
	}
	kinds := productLibraryKinds()
	if len(fixture.LibraryKinds) != len(kinds) {
		t.Fatalf("fixture kinds=%d contract kinds=%d", len(fixture.LibraryKinds), len(kinds))
	}
	for index, kind := range kinds {
		pivots := make([]string, 0, len(kind.Pivots))
		for _, pivot := range kind.Pivots {
			pivots = append(pivots, pivot.ID)
		}
		if fixture.LibraryKinds[index].ID != kind.ID || !reflect.DeepEqual(fixture.LibraryKinds[index].Pivots, pivots) {
			t.Fatalf("fixture kind %d = %#v, contract=%s/%#v", index, fixture.LibraryKinds[index], kind.ID, pivots)
		}
	}
	if len(fixture.InvalidRequests) < 9 || len(fixture.PermissionReductions) < 3 || !fixture.ActionRules.AbsenceIsAuthoritative || fixture.ActionRules.APIBulkExecution != "per-item" || !fixture.ActionRules.ClientFlowsOwnSelectionUI {
		t.Fatalf("fixture omitted validation, permission, or action rules: %#v", fixture)
	}
	if fixture.NavigationOwnership.PrimaryDestinationsAndVisualPlacement != "application" || fixture.NavigationOwnership.InSectionTabsAndComposition != "server" || fixture.NavigationOwnership.ServerDrivenPrimaryNavigationManifest || fixture.NavigationOwnership.RequiredSectionTabsAreHideable {
		t.Fatalf("fixture omitted the application primary navigation and server-owned section composition boundary: %#v", fixture.NavigationOwnership)
	}
}
