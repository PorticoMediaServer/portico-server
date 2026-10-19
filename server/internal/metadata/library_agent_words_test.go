package metadata

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The clients word the two sources from the catalogue (packages/i18n, web.metadataSource.*), in the
// viewer's language; the server's US English must stay the catalogue's, or the two would disagree.
func TestLibraryAgentWordsMatchTheCatalogue(t *testing.T) {
	raw, err := os.ReadFile("../../../packages/i18n/src/catalog/en-US/web.ts")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	has := func(id, text string) {
		t.Helper()
		key, _ := json.Marshal(id)
		value, _ := json.Marshal(text)
		quoted := "'" + strings.Trim(string(key), `"`) + "': '" + strings.ReplaceAll(strings.Trim(string(value), `"`), `\"`, `"`) + "'"
		if !strings.Contains(source, quoted) {
			t.Errorf("%s differs from the catalogue: %q", id, text)
		}
	}
	for _, kind := range []string{"movie", "tv", "anime", "audiobook", "music"} {
		for _, option := range LibraryAgentOptions(kind) {
			switch option.ID {
			case AgentOnline:
				has("web.metadataSource.onlineName", option.Name)
				has("web.metadataSource.onlineDescription."+kind, option.Description)
			case AgentLocal:
				has("web.metadataSource.localName", option.Name)
				has("web.metadataSource.localDescription", option.Description)
			default:
				t.Errorf("source %s has no catalogue words; add them to web.metadataSource and metadataAgentWords", option.ID)
			}
		}
	}
}
