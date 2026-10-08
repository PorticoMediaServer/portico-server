package directdns

import (
	"bytes"
	"strings"
	"testing"
)

var testEndpointMACKey = bytes.Repeat([]byte{0x31}, 32)
var testPreviousEndpointMACKey = bytes.Repeat([]byte{0x32}, 32)

func TestACMEChallengeTarget(t *testing.T) {
	target, err := ACMEChallengeTarget("_acme-challenge.ptc-eeeeeeeeeeeeeeeeeeee.direct.getportico.tv.", "direct.getportico.tv", "acme.getportico.tv")
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	if target != "ptc-eeeeeeeeeeeeeeeeeeee.acme.getportico.tv" {
		t.Fatalf("target = %q", target)
	}
	for _, name := range []string{
		"_acme-challenge.direct.getportico.tv",
		"_acme-challenge.bad_token.direct.getportico.tv",
		"_acme-challenge.extra.ptc-token.direct.getportico.tv",
	} {
		if _, err := ACMEChallengeTarget(name, "direct.getportico.tv", "acme.getportico.tv"); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}

func TestIndependentNamespacesAndUntrustedInterpretation(t *testing.T) {
	seen := map[string]bool{}
	for range 64 {
		ns, e := NewNamespace()
		if e != nil || !ValidNamespace(ns) || seen[ns] {
			t.Fatal(ns, e)
		}
		seen[ns] = true
	}
	for _, ns := range []string{"ptc-" + strings.Repeat("b", 20), "pca-" + strings.Repeat("a", 52), "PTC-" + strings.Repeat("a", 20)} {
		if ValidNamespace(ns) {
			t.Fatal("noncanonical namespace")
		}
	}
}
