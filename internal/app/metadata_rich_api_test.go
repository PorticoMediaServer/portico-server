package app

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestMediaDetailExposesOnlyCurrentAcceptedRichMetadata(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	const mediaID = "movie_meridian"
	var baseRevision int
	if err := server.db.QueryRow(`SELECT metadata_revision FROM media_items WHERE id = ?`, mediaID).Scan(&baseRevision); err != nil {
		t.Fatal(err)
	}
	currentRevision := baseRevision + 1
	if _, err := server.db.Exec(`
		INSERT INTO media_metadata_revisions
			(id, media_id, revision, base_revision, state, trigger_kind, provider, locale, started_at, completed_at)
		VALUES ('rev_rich_api_current', ?, ?, ?, 'applied', 'provider', 'tmdb', 'fr-CA', '2026-08-05T00:00:00Z', '2026-08-05T00:00:01Z')`,
		mediaID, currentRevision, baseRevision); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET metadata_revision = ? WHERE id = ?`, currentRevision, mediaID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_metadata_field_values
			(id, media_id, revision_id, field_key, ordinal, locale, value_json, normalized_value, source_kind, provider, confidence, decision, created_at)
		VALUES
			('field_rich_api_accepted', ?, 'rev_rich_api_current', 'alternateTitle', 0, 'fr-CA', '"Le Méridien"', 'le meridien', 'provider', 'tmdb', 0.95, 'accepted', '2026-08-05T00:00:01Z'),
			('field_rich_api_candidate', ?, 'rev_rich_api_current', 'privateCandidate', 0, '', '"do-not-publish"', '', 'provider', 'tmdb', 0.5, 'candidate', '2026-08-05T00:00:01Z')`,
		mediaID, mediaID); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_metadata_relationships
			(id, media_id, revision_id, relationship_type, target_kind, target_key, display_value, target_provider, target_external_id, language, country, role, ordinal, attributes_json, source_kind, source, provider, confidence, decision, created_at)
		VALUES
			('rel_rich_api_locked', ?, 'rev_rich_api_current', 'person', 'person', 'person:42', 'Ari Vega', 'tmdb', '42', 'fr-CA', 'CA', 'director', 2, '{"internalPath":"/private/provider.json"}', 'provider', '/private/provider.json', 'tmdb', 0.9, 'locked', '2026-08-05T00:00:01Z'),
			('rel_rich_api_rejected', ?, 'rev_rich_api_current', 'keyword', 'keyword', 'secret', 'do-not-publish', '', '', '', '', '', 0, '{}', 'provider', '', 'tmdb', 0.2, 'rejected', '2026-08-05T00:00:01Z')`,
		mediaID, mediaID); err != nil {
		t.Fatal(err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	var detail MediaItem
	status, body := doJSON(t, client, http.MethodGet, serverURL+"/api/media/"+mediaID, nil, &detail)
	if status != http.StatusOK {
		t.Fatalf("detail status=%d body=%s", status, body)
	}
	if detail.MetadataEvidence == nil || detail.MetadataEvidence.Revision != currentRevision {
		t.Fatalf("metadata evidence=%#v, want revision %d", detail.MetadataEvidence, currentRevision)
	}
	if len(detail.MetadataEvidence.Values) != 1 || detail.MetadataEvidence.Values[0].Locale != "fr-CA" || string(detail.MetadataEvidence.Values[0].Value) != `"Le Méridien"` {
		t.Fatalf("values=%#v", detail.MetadataEvidence.Values)
	}
	if len(detail.MetadataEvidence.Relationships) != 1 {
		t.Fatalf("relationships=%#v", detail.MetadataEvidence.Relationships)
	}
	relation := detail.MetadataEvidence.Relationships[0]
	if relation.Type != "person" || relation.Name != "Ari Vega" || relation.Provider != "tmdb" || relation.ExternalProvider != "tmdb" || relation.ExternalID != "42" || relation.Locale != "fr-CA" || relation.Country != "CA" || relation.Role != "director" || relation.Order != 2 {
		t.Fatalf("relationship provenance=%#v", relation)
	}
	for _, forbidden := range []string{"do-not-publish", "/private/provider.json", "internalPath", "attributes", "snapshot"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("detail leaked %q: %s", forbidden, body)
		}
	}

	listItem, err := server.getMediaListItem("", mediaID)
	if err != nil {
		t.Fatal(err)
	}
	leanJSON, err := json.Marshal(listItem)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(leanJSON), "metadataEvidence") {
		t.Fatalf("list projection included rich metadata: %s", leanJSON)
	}
}
