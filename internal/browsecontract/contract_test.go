package browsecontract

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateQueryEnforcesCanonicalFieldOperatorAndValueTypes(t *testing.T) {
	valid := []json.RawMessage{
		json.RawMessage(`{"field":"entityKind","operator":"in","value":["movie","episode"]}`),
		json.RawMessage(`{"field":"dateAdded","operator":"between","value":["2025-01-01","2026-01-01"]}`),
		json.RawMessage(`{"all":[{"field":"genre","operator":"contains","value":"Drama"},{"not":{"field":"favorite","operator":"is","value":true}}]}`),
	}
	for _, query := range valid {
		if err := ValidateQuery(query, ValidationOptions{}); err != nil {
			t.Errorf("canonical query rejected (%s): %v", query, err)
		}
	}

	invalid := []json.RawMessage{
		json.RawMessage(`{"field":"entityKind","operator":"equals","value":"server"}`),
		json.RawMessage(`{"field":"year","operator":"contains","value":"2026"}`),
		json.RawMessage(`{"field":"dateAdded","operator":"equals","value":"yesterday"}`),
		json.RawMessage(`{"field":"favorite","operator":"is","value":"true"}`),
		json.RawMessage(`{"field":"genre","operator":"contains-any","value":"Drama"}`),
		json.RawMessage(`{"field":"genre","operator":"contains","value":["Drama"]}`),
	}
	for _, query := range invalid {
		if err := ValidateQuery(query, ValidationOptions{}); err == nil {
			t.Errorf("invalid query accepted: %s", query)
		}
	}
}

func TestValidateQueryEnforcesStructuralLimitsAndScheduleFieldSubset(t *testing.T) {
	allowed := map[string]struct{}{"entityKind": {}, "genre": {}}
	if err := ValidateQuery(json.RawMessage(`{"field":"favorite","operator":"is","value":true}`), ValidationOptions{AllowedFields: allowed}); err == nil {
		t.Fatal("query field outside the supplied policy subset was accepted")
	}

	tooMany := make([]string, MaximumClauses+1)
	for index := range tooMany {
		tooMany[index] = `{"field":"genre","operator":"contains","value":"Drama"}`
	}
	query := json.RawMessage(`{"all":[` + strings.Join(tooMany, ",") + `]}`)
	if err := ValidateQuery(query, ValidationOptions{}); err == nil {
		t.Fatal("query exceeding the predicate limit was accepted")
	}

	deep := json.RawMessage(`{"not":{"not":{"not":{"not":{"not":{"not":{"field":"genre","operator":"contains","value":"Drama"}}}}}}}`)
	if err := ValidateQuery(deep, ValidationOptions{}); err == nil {
		t.Fatal("query exceeding the nesting limit was accepted")
	}
}

func TestFieldsReturnsDefensiveCopies(t *testing.T) {
	first := Fields()
	first[0].Operators[0] = "mutated"
	first[0].AllowedValues[0] = "mutated"
	second := Fields()
	if second[0].Operators[0] == "mutated" || second[0].AllowedValues[0] == "mutated" {
		t.Fatal("caller mutation changed the canonical browse vocabulary")
	}
}
