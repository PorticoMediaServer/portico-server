package administration

import (
	"os"
	"strings"
	"testing"
)

// The clients word storage categories and day presets from the catalogue (packages/i18n,
// server.storage.category.* and server.maintenance.cadence.*), in the viewer's language; the
// server's US English must stay the catalogue's, or the two would disagree.
func TestMaintenanceWordsMatchTheCatalogue(t *testing.T) {
	raw, err := os.ReadFile("../../../packages/i18n/src/catalog/en-US/owner-console.ts")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	has := func(id, text string) {
		t.Helper()
		if !strings.Contains(source, "'"+id+"': '"+text+"'") {
			t.Errorf("%s differs from the catalogue (or is missing): %q", id, text)
		}
	}
	for _, c := range StorageCategories() {
		has("server.storage.category."+c.ID, c.Name)
		has("server.storage.category."+c.ID+".description", c.Description)
	}
	for _, c := range MaintenanceCadences() {
		has("server.maintenance.cadence."+c.ID, c.Name)
	}
}
