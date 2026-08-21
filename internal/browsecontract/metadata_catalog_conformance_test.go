package browsecontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMetadataCatalogClaimedBrowseFieldsExist(t *testing.T) {
	type fixtureField struct {
		Source, Normalized, Shape              string
		Browse, Search, Detail, Recommendation bool
	}
	var fixture struct {
		Revision string
		Fields   []fixtureField
	}
	path := filepath.Join("..", "..", "api", "openapi", "fixtures", "metadata-catalog-conformance.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Revision == "" || len(fixture.Fields) == 0 {
		t.Fatal("metadata catalog fixture is empty")
	}
	seen := map[string]string{}
	for _, field := range fixture.Fields {
		if field.Source == "" || field.Normalized == "" || field.Shape == "" {
			t.Fatalf("incomplete fixture field: %#v", field)
		}
		if prior := seen[field.Source]; prior != "" {
			t.Fatalf("source %q mapped twice (%s, %s)", field.Source, prior, field.Normalized)
		}
		seen[field.Source] = field.Normalized
		if field.Browse {
			if _, ok := FieldByID(field.Normalized); !ok {
				t.Errorf("%s claims browse support but normalized field %q is absent", field.Source, field.Normalized)
			}
		}
	}
}

func TestRichMetadataFieldsUseCanonicalOperatorFamilies(t *testing.T) {
	for _, id := range []string{"alternateTitle", "originalTitle", "status", "studio", "company", "network", "country", "creator", "credit", "author", "narrator", "series", "regionalCertification"} {
		field, ok := FieldByID(id)
		if !ok || field.ValueType != ValueString {
			t.Errorf("%s string contract=%#v present=%v", id, field, ok)
		}
	}
	for _, id := range []string{"language", "keyword", "collection", "franchise", "acceptedProviderIdentity"} {
		field, ok := FieldByID(id)
		if !ok || field.ValueType != ValueIdentitySet {
			t.Errorf("%s identity-set contract=%#v present=%v", id, field, ok)
		}
	}
	for _, id := range []string{"communityRating", "criticRating", "audienceRating"} {
		field, ok := FieldByID(id)
		if !ok || field.ValueType != ValueNumber {
			t.Errorf("%s numeric contract=%#v present=%v", id, field, ok)
		}
	}
}
