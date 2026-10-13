//go:build devtrust && !release

package oidclogin

import "testing"

func TestDevelopmentAccountOriginIsLoopbackAndProviderFree(t *testing.T) {
	for _, origin := range []string{"http://127.0.0.1:19410", "http://[::1]:19410"} {
		if !developmentAccountOrigin(Config{WebOrigin: origin}) {
			t.Fatalf("local origin refused: %s", origin)
		}
	}
	for _, origin := range []string{"http://localhost:19410", "http://192.168.1.2:19410", "http://127.0.0.1", "http://127.0.0.1:019410", "http://127.0.0.1:19410/", "http://127.0.0.1:19410?", "http://127.0.0.1:19410#x", "http://user@127.0.0.1:19410", "https://127.0.0.1:19410", "http://[::ffff:127.0.0.1]:19410", "http://127.0.0.1:65536"} {
		if developmentAccountOrigin(Config{WebOrigin: origin}) {
			t.Fatalf("unsafe origin admitted: %s", origin)
		}
	}
	for _, config := range []Config{{GoogleClientID: "id"}, {GoogleClientSecret: "secret"}, {AppleClientID: "id"}, {AppleNativeClientID: "id"}, {AppleNativeClientIDs: []string{"id"}}, {AppleNotificationAudiences: []string{"id"}}, {AppleTeamID: "id"}, {AppleKeyID: "id"}, {AppleKeyFile: "file"}} {
		config.WebOrigin = "http://127.0.0.1:19410"
		if developmentAccountOrigin(config) {
			t.Fatal("local origin accepted an external provider configuration")
		}
	}
}
