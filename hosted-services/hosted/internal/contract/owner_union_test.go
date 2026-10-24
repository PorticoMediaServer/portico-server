package contract_test

import (
	"encoding/json"
	"strings"
	"testing"

	"portico.local/hosted/claimweb"
	"portico.local/hosted/internal/serverreturn"
)

func TestConformOneOfDiscriminators(t *testing.T) {
	schema := map[string]any{"oneOf": []any{
		map[string]any{"type": "string", "const": "switch"},
		map[string]any{"type": "string", "enum": []any{"sign-in", "setup"}},
	}}
	for _, value := range []any{"switch", "sign-in", "setup"} {
		if problems := conform(schema, value, "$"); len(problems) != 0 {
			t.Fatalf("valid discriminator refused: %v", problems)
		}
	}
	for _, value := range []any{"change-owner", "", nil, json.Number("1")} {
		if len(conform(schema, value, "$")) == 0 {
			t.Fatal("invalid discriminator accepted")
		}
	}
	schema["oneOf"] = append(schema["oneOf"].([]any), map[string]any{"type": "string", "const": "switch"})
	if len(conform(schema, "switch", "$")) == 0 {
		t.Fatal("oneOf accepted two matching alternatives")
	}
	schema["type"] = "boolean"
	if len(conform(schema, "sign-in", "$")) == 0 {
		t.Fatal("oneOf bypassed its sibling type constraint")
	}
}

// These are the schemas of checked pure transport types, not a claim that the
// owner handlers or their authentication policy have reached the active API.
func TestConformOwnerTransportVariants(t *testing.T) {
	decode := func(value any) any {
		raw, e := json.Marshal(value)
		if e != nil {
			t.Fatal(e)
		}
		var out any
		d := json.NewDecoder(strings.NewReader(string(raw)))
		d.UseNumber()
		if e := d.Decode(&out); e != nil {
			t.Fatal(e)
		}
		return out
	}
	schema := func(value map[string]any) map[string]any { return decode(value).(map[string]any) }
	// Public synthetic transport strings only; signature/PKCE/authority checks
	// are exercised by claimweb and serverreturn's own protocol tests.
	signed := claimweb.Signed{Payload: "public-fixture-payload", Signature: "public-fixture-signature"}
	legacy := claimweb.PendingRequest{Payload: signed.Payload, Signature: signed.Signature}
	bound := legacy
	bound.OwnerStart = &signed
	pendingSchema := schema((claimweb.PendingRequest{}).JSONSchema())
	for _, valid := range []any{legacy, bound} {
		if problems := conform(pendingSchema, decode(valid), "$"); len(problems) != 0 {
			t.Fatalf("valid pending variant refused: %v", problems)
		}
	}
	for _, mutate := range []func(map[string]any){
		func(v map[string]any) { v["ownerStart"] = nil },
		func(v map[string]any) { v["ownerStart"].(map[string]any)["unknown"] = true },
		func(v map[string]any) { delete(v["ownerStart"].(map[string]any), "signature") },
		func(v map[string]any) { v["claimHash"] = "digest-is-not-a-proof" },
	} {
		v := decode(bound).(map[string]any)
		mutate(v)
		if len(conform(pendingSchema, v, "$")) == 0 {
			t.Fatal("incomplete or unknown pending proof accepted")
		}
	}
	owner := serverreturn.OwnerExchange{TransactionID: "owner_transaction", State: strings.Repeat("s", 43), Verifier: strings.Repeat("v", 43), RegistrationID: "ret_actual", InstallationID: strings.Repeat("i", 43), ServerChallenge: strings.Repeat("c", 43), Purpose: "switch"}
	normal := serverreturn.Exchange{Code: strings.Repeat("a", 43), TransactionID: "normal_transaction", Verifier: owner.Verifier, RegistrationID: owner.RegistrationID, InstallationID: owner.InstallationID, ServerChallenge: owner.ServerChallenge, Purpose: "sign-in"}
	exchangeSchema := schema((serverreturn.ExchangeRequest{}).JSONSchema())
	setup := serverreturn.SetupExchange(owner)
	setup.Purpose = "setup"
	for _, valid := range []any{owner, normal, setup} {
		if problems := conform(exchangeSchema, decode(valid), "$"); len(problems) != 0 {
			t.Fatalf("valid exchange variant refused: %v", problems)
		}
	}
	for _, mutate := range []func(map[string]any){
		func(v map[string]any) { v["code"] = normal.Code },
		func(v map[string]any) { v["unknown"] = true },
		func(v map[string]any) { delete(v, "state") },
		func(v map[string]any) { v["state"] = nil },
		func(v map[string]any) { v["purpose"] = "change-owner" },
		func(v map[string]any) { v["purpose"] = "sign-in" },
	} {
		v := decode(owner).(map[string]any)
		mutate(v)
		if len(conform(exchangeSchema, v, "$")) == 0 {
			t.Fatal("mixed or invalid owner exchange accepted")
		}
	}
}

func TestSetupTransportCannotConformToOwnerPurpose(t *testing.T) {
	owner := serverreturn.OwnerExchange{TransactionID: "setup_transaction", State: strings.Repeat("s", 43), Verifier: strings.Repeat("v", 43), RegistrationID: "ret_actual", InstallationID: strings.Repeat("i", 43), ServerChallenge: strings.Repeat("c", 43), Purpose: "setup"}
	raw, _ := json.Marshal(owner)
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	shapeRaw, _ := json.Marshal((serverreturn.OwnerExchange{}).JSONSchema())
	var shape map[string]any
	if err := json.Unmarshal(shapeRaw, &shape); err != nil {
		t.Fatal(err)
	}
	if len(conform(shape, value, "$")) == 0 {
		t.Fatal("setup conformed to switch-only schema")
	}
}
