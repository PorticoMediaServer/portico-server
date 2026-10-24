package claimweb

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestLookupCountKeepsAbsenceDistinctFromVerifiedZero(t *testing.T) {
	base := Pending{Purpose: "switch", Code: "code", Name: "Den"}
	raw, _ := json.Marshal(base)
	absent, _ := json.Marshal(LookupPending{Pending: base})
	if string(raw) != string(absent) {
		t.Fatal("absent lookup count changed existing wire")
	}
	zero := 0
	present, _ := json.Marshal(LookupPending{Pending: base, OtherPeopleCount: &zero})
	if !strings.Contains(string(present), `"otherPeopleCount":0`) {
		t.Fatal("verified zero was omitted")
	}
}
