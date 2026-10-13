package httpapi

import (
	"encoding/json"
	"os/exec"
	"path/filepath"
	"testing"
)

// The consent page's own script, run by Node against a stand-in document.
// Skipped where Node is not installed.
func runConsentPage(t *testing.T, review map[string]any) map[string]any {
	t.Helper()
	node, e := exec.LookPath("node")
	if e != nil {
		t.Skip("node is not installed")
	}
	answer, _ := json.Marshal(review)
	catalogue, _ := filepath.Abs("../../packages/i18n/app-sign-in.en.json")
	raw, e := exec.Command(node, "testdata/authorize_page_harness.js", "appauthassets/authorize.js", catalogue, string(answer)).CombinedOutput()
	if e != nil {
		t.Fatal(e, string(raw))
	}
	var out map[string]any
	if e = json.Unmarshal(raw, &out); e != nil {
		t.Fatal(e, string(raw))
	}
	return out
}

// Continue is pressed before anything about the app is shown. A held key, a
// second press or a double-click must not carry on into "Allow sign-in".
func TestConsentIsNotGivenByARepeatedPress(t *testing.T) {
	out := runConsentPage(t, map[string]any{"id": "app_review", "accountId": "acct", "accountName": "Sam", "serverName": "Den", "appName": "Outside Player", "redirectUri": "my-player://done"})
	if out["focusAfterReview"] != "consent-line" {
		t.Fatalf("focus moved to %v when the consent appeared", out["focusAfterReview"])
	}
	if out["allowDisabledAtFirst"] != true {
		t.Fatal("Allow accepted a press as soon as it was drawn")
	}
	if calls := out["callsAfterRepeat"].([]any); len(calls) != 1 || calls[0] != "/v1/app-authorizations" {
		t.Fatalf("a repeated press decided the request: %v", calls)
	}
	if out["allowDisabledLater"] != false {
		t.Fatal("Allow never became available")
	}
	calls := out["callsAfterDeliberateAllow"].([]any)
	assigned := out["assigned"].([]any)
	if len(calls) != 2 || calls[1] != "/v1/app-authorizations/app_review/decision" || len(assigned) != 1 || assigned[0] != "my-player://done?code=one" {
		t.Fatalf("a deliberate Allow did not complete: %v %v", calls, assigned)
	}
}
