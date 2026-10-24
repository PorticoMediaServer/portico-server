package serverreturn

import (
	"encoding/json"
	"testing"
)

func TestSetupExchangeIsDistinctAndCannotWidenSwitch(t *testing.T) {
	owner := ownerExchangeFixture()
	setup := SetupExchange(owner)
	setup.Purpose = "setup"
	raw, err := json.Marshal(ExchangeRequest{Setup: &setup})
	var got ExchangeRequest
	if err != nil || json.Unmarshal(raw, &got) != nil || got.Setup == nil || got.Owner != nil || got.Registered != nil || *got.Setup != setup {
		t.Fatal("setup not kept separate", err)
	}
	var switchOnly OwnerExchange
	if json.Unmarshal(raw, &switchOnly) == nil {
		t.Fatal("switch boundary accepted setup")
	}
	if _, err = json.Marshal(ExchangeRequest{Setup: &setup, Owner: &owner}); err == nil {
		t.Fatal("mixed purposes marshaled")
	}
	if json.Unmarshal([]byte(`{"state":null}`), &got) == nil || got.Setup != nil || got.Owner != nil || got.Registered != nil {
		t.Fatal("failed parse retained old authority")
	}
}
