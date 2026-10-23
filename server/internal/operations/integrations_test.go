package operations

import "testing"

func TestIntegrationPolicyOptionsAreClosed(t *testing.T) {
	if fields := validateIntegrations(DefaultSettings()); len(fields) > 0 {
		t.Fatal(fields)
	}
	for name, change := range map[string]func(*Settings){
		"retries":  func(v *Settings) { v.WebhookRetries = "forever" },
		"timeout":  func(v *Settings) { v.WebhookTimeoutSeconds = 16 },
		"queue":    func(v *Settings) { v.WebhookQueueSize = 1001 },
		"log":      func(v *Settings) { v.WebhookLogSize = 201 },
		"lifetime": func(v *Settings) { v.CredentialMaxDays = 31 },
		"scope":    func(v *Settings) { v.MemberTokenScope = "admin" },
		"tokens":   func(v *Settings) { v.TokensPerMember = 26 },
		"keys":     func(v *Settings) { v.APIKeysMax = 101 },
	} {
		v := DefaultSettings()
		change(&v)
		if len(validateIntegrations(v)) != 1 {
			t.Fatalf("%s: invalid choice accepted or unrelated values changed", name)
		}
	}
	for _, days := range []int{0, 30, 90, 365} {
		v := DefaultSettings()
		v.CredentialMaxDays = days
		if fields := validateIntegrations(v); len(fields) > 0 {
			t.Fatal(fields)
		}
	}
}
