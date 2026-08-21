package app

import (
	"reflect"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/browsecontract"
	"github.com/PorticoMediaServer/portico-server/internal/catalogkind"
)

func TestCanonicalKindsStayConsistentAcrossProductBrowseAndSearch(t *testing.T) {
	contract := canonicalProductContract()
	if want := catalogkind.PublicKinds(); !reflect.DeepEqual(contract.EntityKinds, want) {
		t.Fatalf("product entity kinds = %#v, want canonical vocabulary %#v", contract.EntityKinds, want)
	}

	browseField, ok := browsecontract.FieldByID("entityKind")
	if !ok {
		t.Fatal("browse entityKind field is missing")
	}
	for _, kind := range browseField.AllowedValues {
		if !catalogkind.IsPublic(kind) || kind == string(catalogkind.Unsupported) {
			t.Errorf("browse exposes non-canonical selectable kind %q", kind)
		}
	}

	for _, mapping := range contract.Search.ResultSemantics.KindMappings {
		if want := string(catalogkind.Public(mapping.ResultKind)); mapping.EntityKind != want {
			t.Errorf("search result %q maps to %q, want %q", mapping.ResultKind, mapping.EntityKind, want)
		}
		if !catalogkind.IsPublic(mapping.EntityKind) {
			t.Errorf("search maps %q to non-canonical kind %q", mapping.ResultKind, mapping.EntityKind)
		}
	}
}
