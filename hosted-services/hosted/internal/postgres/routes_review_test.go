package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"strings"
	"testing"
)

// Normal PostgreSQL fixture only. Marking proof facts below is fixture setup;
// this test does not contact or claim to prove a real external endpoint.
func TestW2I02RouteHistoryDeduplicatesAndCannotResurrectMismatch(t *testing.T) {
	f := currentClaimJourney(t)
	keys := &security.Keys{PrivateKey: ed25519.NewKeyFromSeed(bytes.Repeat([]byte{1}, 32)), KeyID: "review", SealKey: bytes.Repeat([]byte{2}, 32)}
	service := directory.NewWithOptions(f.store, keys, directory.Options{})
	root := (&httpapi.Handler{Claims: f.handler, Identity: identity.New(f.store), Directory: service}).Routes()
	f.handler = root
	approval := journeyApproval(t, f)
	raw, sig := journeyProof(t, f, approval, claims.Finalize)
	var commit claims.Commit
	claimHTTP(t, root, "/v1/server-claims/finalize", "", proofEnvelope(raw, sig), &commit, 200)
	raw, sig = journeyProof(t, f, approval, claims.Result)
	var result claims.ResultValue
	claimHTTP(t, root, "/v1/server-claims/result", "", proofEnvelope(raw, sig), &result, 200)
	claimHTTP(t, root, "/v1/server-claims/ack", result.ServerCredential, commit, nil, 200)
	ctx := context.Background()
	authority, e := service.CaptureServerAuthority(ctx, result.ServerCredential)
	if e != nil {
		t.Fatal(e)
	}
	list, e := service.List(ctx, f.expected.AccountID)
	if e != nil || len(list) != 1 || list[0].Routes != nil {
		t.Fatal("unobserved server produced generation-zero document", e)
	}
	a, b := "https://a.example.com", "https://b.example.com"
	if _, e = service.ObserveRoutes(ctx, authority, directory.RouteObservation{Generation: 1, RemoteEnabled: true, CGNAT: true, Candidates: []directory.RouteCandidate{{BaseURL: a, Class: "public"}, {BaseURL: b, Class: "public"}}}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.Exec(`UPDATE networking_route_candidates SET state='verified',verified_at=clock_timestamp() WHERE server_id=$1`, commit.ServerID); e != nil {
		t.Fatal(e)
	}
	// BE-HOSTED-08: a route's quality says whether the route itself was
	// verified, never whether the server "seems up" from its check-in lease:
	// with the lease long expired, the verified current route is still
	// reachable-quality, and nothing is claimed about liveness.
	if _, e = f.store.DB.Exec(`UPDATE servers SET presence_expires_at=now()-interval '30 days' WHERE id=$1`, commit.ServerID); e != nil {
		t.Fatal(e)
	}
	verified, e := f.store.RouteSets(ctx, f.expected.AccountID)
	if e != nil {
		t.Fatal(e)
	}
	if len(verified[commit.ServerID].Candidates) != 2 {
		t.Fatalf("verified candidates: %+v", verified[commit.ServerID])
	}
	for _, candidate := range verified[commit.ServerID].Candidates {
		if candidate.Quality != "reachable" {
			t.Fatalf("a verified current route read as %q from an expired check-in lease", candidate.Quality)
		}
	}
	if _, e = service.ObserveRoutes(ctx, authority, directory.RouteObservation{Generation: 2, RemoteEnabled: true, Candidates: []directory.RouteCandidate{{BaseURL: a, Class: "public"}}}); e != nil {
		t.Fatal(e)
	}
	sets, e := f.store.RouteSets(ctx, f.expected.AccountID)
	if e != nil {
		t.Fatal(e)
	}
	set := sets[commit.ServerID]
	if len(set.Candidates) != 2 {
		t.Fatal("duplicate origins in current/previous sets", set)
	}
	seen := map[string]int64{}
	for _, candidate := range set.Candidates {
		if _, ok := seen[candidate.BaseURL]; ok {
			t.Fatal("duplicate signed origin")
		}
		seen[candidate.BaseURL] = candidate.Generation
	}
	if seen[a] != 2 || seen[b] != 1 {
		t.Fatal("wrong generation won", seen)
	}

	if e = f.store.RecordRouteFailure(ctx, authority, 2, a, "identity_mismatch"); e != nil {
		t.Fatal(e)
	}
	sets, e = f.store.RouteSets(ctx, f.expected.AccountID)
	if e != nil {
		t.Fatal(e)
	}
	set = sets[commit.ServerID]
	if len(set.Candidates) != 1 || set.Candidates[0].BaseURL != b {
		t.Fatal("previous verified address resurrected current mismatch", set)
	}
	lan := "http://192.168.1.8:32500"
	if _, e = service.ObserveRoutes(ctx, authority, directory.RouteObservation{Generation: 3, RemoteEnabled: false, Candidates: []directory.RouteCandidate{{BaseURL: lan, Class: "lan"}}}); e != nil {
		t.Fatal(e)
	}
	sets, e = f.store.RouteSets(ctx, f.expected.AccountID)
	if e != nil {
		t.Fatal(e)
	}
	set = sets[commit.ServerID]
	if len(set.Candidates) != 1 || set.Candidates[0].BaseURL != lan {
		t.Fatal("remote opt-out leaked public history", set)
	}
	var metrics strings.Builder
	if e = f.store.Metrics(ctx, &metrics); e != nil || !strings.Contains(metrics.String(), "portico_cgnat_reports_total 3\n") || !strings.Contains(metrics.String(), "portico_cgnat_shared_reports_total 1\n") {
		t.Fatal("anonymous CGNAT count", metrics.String(), e)
	}
	assertion, signature := cancelAssertion(t, f, commit.ClaimGeneration)
	claimHTTP(t, root, "/v1/server-claims/cancel", "", proofEnvelope(assertion, signature), nil, 200)
	if _, e = service.ObserveRoutes(ctx, authority, directory.RouteObservation{Generation: 4, RemoteEnabled: true, Candidates: []directory.RouteCandidate{{BaseURL: a, Class: "public"}}}); e == nil {
		t.Fatal("revoked captured authority republished routes")
	}
}
