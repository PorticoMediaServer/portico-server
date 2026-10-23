package claimweb

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestPendingRequestRetainsBothSignedProofs(t *testing.T) {
	key, claim, start, now := ownerStartFixture(t)
	original, signedStart := signOwnerFixture(t, key, claim), signOwnerFixture(t, key, start)
	for _, owner := range []*Signed{nil, &signedStart} {
		in := PendingRequest{Payload: original.Payload, Signature: original.Signature, OwnerStart: owner}
		raw, e := json.Marshal(in)
		var got PendingRequest
		if e != nil || json.Unmarshal(raw, &got) != nil || got.Original() != original {
			t.Fatal("original signed claim was not retained", e)
		}
		if owner == nil {
			legacy, _ := json.Marshal(original)
			if string(raw) != string(legacy) || got.OwnerStart != nil {
				t.Fatal("ordinary pending bytes changed")
			}
		} else if got.OwnerStart == nil || *got.OwnerStart != signedStart {
			t.Fatal("full owner-start proof was not retained")
		} else if _, _, e := VerifyOwnerStart(got.Original(), *got.OwnerStart, claim.Audience, now); e != nil {
			t.Fatal("retained proof pair does not verify", e)
		}
		if fmt.Sprintf("%v %#v", in, in) != "[pending claim redacted] [pending claim redacted]" {
			t.Fatal("pending proof leaked through formatting")
		}
	}
}

func TestPendingRequestRejectsAmbiguousProofTransport(t *testing.T) {
	base := `"payload":"claim","signature":"signature"`
	cases := []string{
		`null`, `[]`, `{}`, `{` + base + `} {}`,
		`{` + base + `,"payload":"replacement"}`,
		`{` + base + `,"Payload":"replacement"}`,
		`{` + base + `,"claimHash":"digest-only"}`,
		`{` + base + `,"ownerStart":null}`,
		`{` + base + `,"ownerStart":{}}`,
		`{` + base + `,"ownerStart":{"payload":"start"}}`,
		`{` + base + `,"ownerStart":{"payload":null,"signature":"signature"}}`,
		`{` + base + `,"ownerStart":{"payload":"start","signature":"signature","signature":"other"}}`,
		`{` + base + `,"ownerStart":{"payload":"start","signature":"signature","unknown":true}}`,
		`{"payload":null,"signature":"signature"}`,
		`{"payload":"claim","signature":12}`,
		`{"payload":"` + strings.Repeat("a", 32<<10) + `","signature":"signature"}`,
	}
	for i, raw := range cases {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			q := PendingRequest{Payload: "old", Signature: "old", OwnerStart: &Signed{Payload: "old", Signature: "old"}}
			if q.UnmarshalJSON([]byte(raw)) == nil {
				t.Fatal("ambiguous proof transport accepted")
			}
			if q.Payload != "" || q.Signature != "" || q.OwnerStart != nil {
				t.Fatal("failed decode retained predecessor proof")
			}
		})
	}
}

func TestPendingRequestSchemaRequiresNonNullFullStart(t *testing.T) {
	variants := (PendingRequest{}).JSONSchema()["oneOf"].([]any)
	if len(variants) != 2 {
		t.Fatal("pending variants are not distinct")
	}
	for _, v := range variants {
		if v.(map[string]any)["additionalProperties"] != false {
			t.Fatal("pending variant is not closed")
		}
	}
	owner := variants[1].(map[string]any)
	if len(owner["required"].([]string)) != 3 || owner["properties"].(map[string]any)["ownerStart"].(map[string]any)["type"] != "object" {
		t.Fatal("owner start can be omitted or normalized from null")
	}
}
