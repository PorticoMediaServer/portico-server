package networking

import "testing"

func TestHostedMemberRouteNameAndLegacyMigration(t *testing.T) {
	ns := "ptc-aaaaaaaaaaaaaaaaaaaa"
	for _, name := range []string{
		"current." + ns + ".direct.getportico.tv",
		"c-0123456789abcdef0123456789abcdef." + ns + ".direct.getportico.tv",
	} {
		if !validHostedRouteName(name, ns) {
			t.Fatal("valid Hosted route refused", name)
		}
	}
	for _, name := range []string{
		"c-short." + ns + ".direct.getportico.tv",
		"c-0123456789abcdef0123456789abcdeG." + ns + ".direct.getportico.tv",
		"c-0123456789abcdef0123456789abcdef.ptc-bbbbbbbbbbbbbbbbbbbb.direct.getportico.tv",
	} {
		if validHostedRouteName(name, ns) {
			t.Fatal("invalid Hosted route accepted", name)
		}
	}
}
