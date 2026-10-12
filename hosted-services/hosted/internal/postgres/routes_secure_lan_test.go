package postgres_test

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"testing"

	"portico.local/dns/directdns"
	"portico.local/hosted/internal/directory"
)

func TestSecureLANRouteDocument(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	const namespace = "ptc-aaaaaaaaaaaaaaaaaaaa"
	const label = "0123456789abcdef0123456789abcdef"
	if _, err := f.store.DB.Exec("INSERT INTO certificate_namespaces(operation_id,namespace,member_label,server_identity) VALUES('fixture_installed_claim',$1,$2,$3)", namespace, label, f.server); err != nil {
		t.Fatal(err)
	}
	authority, err := f.dir.CaptureServerAuthority(ctx, f.token)
	if err != nil {
		t.Fatal(err)
	}
	observation := directory.RouteObservation{Generation: 1, RemoteEnabled: true, Candidates: []directory.RouteCandidate{
		{BaseURL: "https://192.168.1.8:32500", Class: "lan"},
		{BaseURL: "https://192.168.1.9:32500", Class: "lan"},
		{BaseURL: "https://[fd00::8]:32500", Class: "lan"},
		{BaseURL: "https://192.168.1.8:32501", Class: "lan"},
		{BaseURL: "http://192.168.1.8:32500", Class: "lan"},
		{BaseURL: "https://remote.example:32500", Class: "public"},
	}}
	if _, err = f.dir.ObserveRoutes(ctx, authority, observation); err != nil {
		t.Fatal(err)
	}
	signed, err := f.dir.Routes(ctx, f.owner.Account.ID, f.server)
	if err != nil || signed == nil {
		t.Fatalf("signed routes: %+v %v", signed, err)
	}
	raw, err := base64.RawURLEncoding.Strict().DecodeString(signed.Payload)
	if err != nil {
		t.Fatal(err)
	}
	signature, err := base64.RawURLEncoding.Strict().DecodeString(signed.Signature)
	if err != nil || !ed25519.Verify(f.keys.PrivateKey.Public().(ed25519.PublicKey), raw, signature) {
		t.Fatal("invalid route signature", err)
	}
	var document directory.RouteDocument
	if err = json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if document.ServerID != f.server || document.Audience != f.owner.Account.ID || document.Generation != 1 {
		t.Fatalf("wrong signed authority: %+v", document)
	}
	alias := "https://" + directdns.LANHostname(namespace, "direct.getportico.tv", label)
	want := map[string]string{
		alias + ":32500":               "lan",
		alias + ":32501":               "lan",
		"http://192.168.1.8:32500":     "lan",
		"https://remote.example:32500": "public",
	}
	if len(document.Candidates) != len(want) {
		t.Fatalf("duplicate signed candidates: %+v", document.Candidates)
	}
	for _, candidate := range document.Candidates {
		class, ok := want[candidate.BaseURL]
		if !ok || candidate.Class != class || candidate.Generation != 1 || candidate.Quality != "probe_required" {
			t.Fatalf("unexpected signed candidate: %+v", candidate)
		}
		delete(want, candidate.BaseURL)
	}
	var ipv4, ipv6 string
	if err = f.store.DB.QueryRow("SELECT lan_ipv4,lan_ipv6 FROM direct_addresses WHERE namespace=$1", namespace).Scan(&ipv4, &ipv6); err != nil || ipv4 != "192.168.1.8" || ipv6 != "fd00::8" {
		t.Fatalf("LAN address set = %q, %q (%v)", ipv4, ipv6, err)
	}
}
