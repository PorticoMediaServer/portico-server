package main

import "testing"

func TestProductionHostedOriginCannotUseDevelopmentTrust(t *testing.T) {
	for _, raw := range []string{"https://web.getportico.tv", "HTTPS://WEB.GETPORTICO.TV/claim", "https://web.getportico.tv:443/device"} {
		if !productionHostedAddress(raw) {
			t.Fatal(raw)
		}
	}
	for _, raw := range []string{"", "http://127.0.0.1:19410", "https://other.example.test"} {
		if productionHostedAddress(raw) {
			t.Fatal(raw)
		}
	}
}
