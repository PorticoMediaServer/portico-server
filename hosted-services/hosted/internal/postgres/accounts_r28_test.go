package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"portico.local/dns/directdns"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
)

// Waiting accounts have no directory membership yet. Hosted proves their
// identity; the server validates its fresh challenge and decides admission.
func TestWaitingIdentityHandlerNeedsNoDirectoryMembership(t *testing.T) {
	f := setup(t)
	if _, e := f.store.DB.Exec(`DELETE FROM server_members WHERE account_id=$1`, f.member.Account.ID); e != nil {
		t.Fatal(e)
	}
	const email = "waiting@example.test"
	if _, e := f.store.DB.Exec(`INSERT INTO account_contact_emails(id,account_id,address,verified_at,source,is_primary) VALUES('waiting-contact',$1,$2,clock_timestamp(),'challenge',true)`, f.member.Account.ID, email); e != nil {
		t.Fatal(e)
	}
	if current, e := f.ids.IdentityEmail(context.Background(), f.member.Account.ID); e != nil || current != email {
		t.Fatal("verified primary identity email", current, e)
	}
	server, challenge := f.server, testChallenge
	if path := os.Getenv("PORTICO_IDENTITY_CONSUMER_INPUT"); path != "" {
		raw, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		var q struct{ ServerID, Challenge string }
		if json.Unmarshal(raw, &q) != nil {
			t.Fatal("bad private consumer input")
		}
		server, challenge = q.ServerID, q.Challenge
	}
	h := (&httpapi.Handler{Identity: f.ids, Directory: f.dir}).Routes()
	request := func(token string, body any, want int) []byte {
		raw, _ := json.Marshal(body)
		r := httptest.NewRequest("POST", "/v1/servers/"+server+"/identity", bytes.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		out := httptest.NewRecorder()
		h.ServeHTTP(out, r)
		if out.Code != want {
			t.Fatalf("identity status %d want %d: %s", out.Code, want, out.Body.String())
		}
		return out.Body.Bytes()
	}
	body := map[string]string{"challenge": challenge}
	raw := request(f.member.AccessToken, body, 200)
	var signed directory.SignedPolicy
	if e := json.Unmarshal(raw, &signed); e != nil {
		t.Fatal(e)
	}
	payload, e := base64.RawURLEncoding.Strict().DecodeString(signed.Payload)
	if e != nil {
		t.Fatal(e)
	}
	var c directory.IdentityClaims
	if e = json.Unmarshal(payload, &c); e != nil {
		t.Fatal(e)
	}
	if _, e = signed.Verify(f.keys.RootPublicKey, "documents", c.IssuedAt); e != nil {
		t.Fatal(e)
	}
	canonical, _ := json.Marshal(c)
	if !bytes.Equal(canonical, payload) || c.AccountID != f.member.Account.ID || c.Username != "member" || c.DisplayName != "Member" || c.Email != email || c.ServerID != server || c.Challenge != challenge || c.Kind != directory.IdentityKind {
		t.Fatalf("wrong identity/order: %s", payload)
	}
	request("", body, 401)
	request(f.member.AccessToken, map[string]string{"challenge": "short"}, 400)
	var count int
	if e = f.store.DB.QueryRow(`SELECT count(*) FROM server_members WHERE account_id=$1`, f.member.Account.ID).Scan(&count); e != nil || count != 0 {
		t.Fatal("identity invented directory membership", count, e)
	}
	if path := os.Getenv("PORTICO_IDENTITY_PRODUCER_JSON"); path != "" {
		artifact, _ := json.Marshal(map[string]any{"assertion": json.RawMessage(raw), "root": base64.RawURLEncoding.EncodeToString(f.keys.RootPublicKey), "serverId": server, "challenge": challenge})
		if e = os.WriteFile(path, artifact, 0600); e != nil {
			t.Fatal(e)
		}
	}
}

func TestAccountDeletionRefusesOwnershipAndOnlySignalsMemberDeparture(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, e := f.ids.PrepareDeletion(ctx, f.owner.Account, "StrongPass9"); !errors.Is(e, identity.ErrOwnedServers) {
		t.Fatal("owner deletion was not refused", e)
	}
	p, e := f.ids.PrepareDeletion(ctx, f.member.Account, "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	r, e := f.ids.AcceptDeletion(ctx, p.Token, "")
	if e != nil || !r.AccountDeleted || r.Status != "complete" || len(r.PendingServers) != 0 {
		t.Fatal(r, e)
	}
	a, e := f.store.CaptureServerAuthority(ctx, security.Hash(f.token))
	if e != nil {
		t.Fatal(e)
	}
	check, e := f.dir.ServerHeartbeat(ctx, a, directory.CheckInReport{})
	if e != nil || len(check.Departures) != 1 || check.Departures[0].AccountID != f.member.Account.ID {
		t.Fatal("departure missing", check, e)
	}
	for _, name := range []string{"profiles", "memberships", "restrictions", "invitations", "profile_server_policies", "profile_trusted_selections", "profile_erasure_jobs", "account_erasure_jobs", "account_preferences", "hosted_schema_migrations"} {
		var exists bool
		if e = f.store.DB.QueryRow(`SELECT to_regclass(current_schema()||'.'||$1) IS NOT NULL`, name).Scan(&exists); e != nil || exists {
			t.Fatal("retired Hosted authority remains", name, e)
		}
	}
	// A claim acquired after preview must also block acceptance under the same
	// account lock used by finalization, rather than silently retiring the server.
	g := currentClaimJourney(t)
	if _, e = g.store.DB.Exec(`UPDATE accounts SET password_hash=$1 WHERE id=$2`, passwordHashR28(t, "StrongPass9"), g.expected.AccountID); e != nil {
		t.Fatal(e)
	}
	ids := identity.New(g.store)
	aOwner, _, e := g.store.Password(ctx, "strict-owner")
	if e != nil {
		t.Fatal(e)
	}
	preview, e := ids.PrepareDeletion(ctx, aOwner, "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	approval := journeyApproval(t, g)
	raw, sig := journeyProof(t, g, approval, claims.Finalize)
	if _, e = g.service.Finalize(ctx, raw, sig); e != nil {
		t.Fatal(e)
	}
	if _, e = ids.AcceptDeletion(ctx, preview.Token, ""); !errors.Is(e, identity.ErrOwnedServers) {
		t.Fatal("newly owned server deletion", e)
	}
}

func TestSwitchRequiresExplicitCurrentCollisionReservation(t *testing.T) {
	f := currentClaimJourney(t)
	ctx := context.Background()
	f.input.Purpose = "switch" // no prior Hosted registration
	approval := journeyApproval(t, f)
	if approval.TargetUsername != "strict-owner" {
		t.Fatal("switch omitted approved username", approval.TargetUsername)
	}
	raw, sig := journeyProof(t, f, approval, claims.Finalize)
	var w claims.Wire
	if e := json.Unmarshal(raw, &w); e != nil {
		t.Fatal(e)
	}
	for _, change := range []func(*claims.Wire){
		func(q *claims.Wire) { q.ReservationAccountID = "strict-other" },
		func(q *claims.Wire) { q.ReservationUsername = "collision" },
		func(q *claims.Wire) { q.ReservationGeneration = "1" },
		func(q *claims.Wire) { q.ReservationOperationID = "another_operation" },
		func(q *claims.Wire) {
			q.ReservationAccountID = ""
			q.ReservationUsername = ""
			q.ReservationGeneration = ""
			q.ReservationOperationID = ""
		},
	} {
		bad := w
		change(&bad)
		payload, _ := json.Marshal(bad)
		if _, e := f.service.Finalize(ctx, payload, ed25519.Sign(f.key, payload)); !errors.Is(e, claims.ErrStale) {
			t.Fatal("bad reservation finalized", bad, e)
		}
	}
	var committed int
	if e := f.store.DB.QueryRow(`SELECT count(*) FROM servers`).Scan(&committed); e != nil || committed != 0 {
		t.Fatal("collision created Hosted transfer", committed, e)
	}
	if _, e := f.service.Finalize(ctx, raw, sig); e != nil {
		t.Fatal("reserved never-claimed switch", e)
	}
}

func finalizedR28(t *testing.T, f claimJourney) claims.Commit {
	t.Helper()
	a := journeyApproval(t, f)
	raw, sig := journeyProof(t, f, a, claims.Finalize)
	c, e := f.service.Finalize(context.Background(), raw, sig)
	if e != nil {
		t.Fatal(e)
	}
	return c
}

func TestOfflineLeavePurgesRegistrationAndFeedsLateDNSReaders(t *testing.T) {
	f := currentClaimJourney(t)
	ctx := context.Background()
	c := finalizedR28(t, f)
	const ns = "ptc-aaaaaaaaaaaaaaaaaaaa"
	const label = "0123456789abcdef0123456789abcdef"
	if _, e := f.store.DB.Exec(`INSERT INTO certificate_namespaces(operation_id,namespace,member_label,server_identity) VALUES($1,$2,$3,$4)`, c.OperationID, ns, label, c.ServerID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.DB.Exec(`INSERT INTO direct_addresses(namespace,server_id,ipv4,lan_ipv4,sequence) VALUES($1,$2,'8.8.8.8','192.168.1.3',nextval('direct_address_sequence'))`, ns, c.ServerID); e != nil {
		t.Fatal(e)
	}
	before, e := f.store.DirectAddressChanges(ctx, 0, 1)
	if e != nil || len(before) != 1 {
		t.Fatal(before, e)
	}
	dns, e := directdns.OpenTable("")
	if e != nil {
		t.Fatal(e)
	}
	feed := func(after int64) []directdns.FeedRow {
		t.Helper()
		rows, e := f.store.DirectAddressChanges(ctx, after, 1)
		if e != nil {
			t.Fatal(e)
		}
		out := []directdns.FeedRow{}
		for _, r := range rows {
			out = append(out, directdns.FeedRow{Sequence: r.Sequence, Namespace: r.Namespace, Label: r.Label, IPv4: r.IPv4, IPv6: r.IPv6, LANIPv4: r.LANIPv4, LANIPv6: r.LANIPv6, PreviousLabel: r.PreviousLabel, PreviousLabelUntil: r.PreviousLabelUntil})
		}
		return out
	}
	if e = dns.Apply(feed(0)); e != nil {
		t.Fatal(e)
	}
	if _, ok := dns.Lookup(ns); !ok {
		t.Fatal("initial route missing")
	}
	w := claims.LeaveWire{Kind: "portico.server.leave", Version: "1", Audience: "https://hosted.example", OperationID: c.OperationID, ServerID: c.ServerID, PublicKey: f.input.PublicKey, ClaimGeneration: c.ClaimGeneration, CredentialGeneration: c.CredentialGeneration, NoticeID: "durable_notice", Reason: "moving_to_local"}
	raw, _ := json.Marshal(w)
	signature := ed25519.Sign(f.key, raw)
	bad := w
	bad.CredentialGeneration = "999"
	badRaw, _ := json.Marshal(bad)
	if e = f.service.Leave(ctx, badRaw, ed25519.Sign(f.key, badRaw)); !errors.Is(e, claims.ErrStale) {
		t.Fatal("stale leave accepted", e)
	}
	// The notice is saved bytes with no expiry, delivered after the local switch.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- f.service.Leave(ctx, raw, signature) }()
	}
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatal("notice retry", e)
		}
	}
	for _, table := range []string{"servers", "claim_operations", "claim_approvals", "claim_nonces", "server_members", "certificate_namespaces", "certificate_orders", "direct_addresses", "direct_address_events", "networking_route_state", "current_server_endpoints"} {
		var n int
		if e = f.store.DB.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); e != nil || n != 0 {
			t.Fatal("server data retained", table, n, e)
		}
	}
	var columns int
	if e = f.store.DB.QueryRow(`SELECT count(*) FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='direct_address_withdrawals' AND column_name NOT IN('namespace','member_label','previous_member_label','sequence')`).Scan(&columns); e != nil || columns != 0 {
		t.Fatal("identity stored in withdrawal", columns, e)
	}
	rows := feed(before[0].Sequence)
	if len(rows) != 1 || rows[0].IPv4 != "" || rows[0].LANIPv4 != "" {
		t.Fatal("late DNS reader missed withdrawal", rows)
	}
	if e = dns.Apply(rows); e != nil {
		t.Fatal(e)
	}
	if _, ok := dns.Lookup(ns); ok {
		t.Fatal("DNS route survived leave")
	}
	if e = dns.Apply(feed(0)); e != nil {
		t.Fatal(e)
	}
	if _, ok := dns.Lookup(ns); ok {
		t.Fatal("stale feed resurrected route")
	}
	// Re-register with exactly the same installation/operation IDs. The global
	// claim sequence still prevents an old tuple from matching the successor.
	successor := finalizedR28(t, f)
	if successor.ClaimGeneration == c.ClaimGeneration {
		t.Fatal("deleted registration reused generation")
	}
	w.NoticeID = "late_different_notice"
	payload, _ := json.Marshal(w)
	if e = f.service.Leave(ctx, payload, ed25519.Sign(f.key, payload)); !errors.Is(e, claims.ErrStale) {
		t.Fatal("old generation removed successor", e)
	}
	if e = f.service.Leave(ctx, raw, signature); e != nil {
		t.Fatal("exact replay receipt", e)
	}
	var live int
	if e = f.store.DB.QueryRow(`SELECT count(*) FROM servers WHERE claim_generation=$1::bigint`, successor.ClaimGeneration).Scan(&live); e != nil || live != 1 {
		t.Fatal("successor lost", live, e)
	}
}

func TestLeaveHTTPWaitsForNamespaceProducerAndReturnsEmpty204(t *testing.T) {
	f := currentClaimJourney(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	a := journeyApproval(t, f)
	raw, sig := journeyProof(t, f, a, claims.Finalize)
	c, e := f.service.Finalize(ctx, raw, sig)
	if e != nil {
		t.Fatal(e)
	}
	raw, sig = journeyProof(t, f, a, claims.Result)
	result, e := f.service.Result(ctx, raw, sig)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.service.Acknowledge(ctx, result.ServerCredential, c); e != nil {
		t.Fatal(e)
	}
	authority, e := f.store.CaptureServerAuthority(ctx, security.Hash(result.ServerCredential))
	if e != nil {
		t.Fatal(e)
	}
	w := claims.LeaveWire{Kind: "portico.server.leave", Version: "1", Audience: "https://hosted.example", OperationID: c.OperationID, ServerID: c.ServerID, PublicKey: f.input.PublicKey, ClaimGeneration: c.ClaimGeneration, CredentialGeneration: c.CredentialGeneration, NoticeID: "concurrent_notice", Reason: "start_over_local"}
	raw, _ = json.Marshal(w)
	envelope := httpapi.ClaimSignedRequest{Payload: base64.RawURLEncoding.EncodeToString(raw), Signature: base64.RawURLEncoding.EncodeToString(ed25519.Sign(f.key, raw))}
	request := func(q httpapi.ClaimSignedRequest) *httptest.ResponseRecorder {
		body, _ := json.Marshal(q)
		r := httptest.NewRequest("POST", "/v1/servers/leave", bytes.NewReader(body)).WithContext(ctx)
		r.Header.Set("Content-Type", "application/json")
		out := httptest.NewRecorder()
		f.handler.ServeHTTP(out, r)
		return out
	}
	for _, q := range []httpapi.ClaimSignedRequest{
		{Payload: envelope.Payload + "=", Signature: envelope.Signature},
		{Payload: envelope.Payload, Signature: envelope.Signature + "=="},
		{Payload: envelope.Payload + "\n", Signature: envelope.Signature},
	} {
		if out := request(q); out.Code != 400 {
			t.Fatal("noncanonical envelope accepted", out.Code)
		}
	}
	ready, release := make(chan string, 1), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	producer := make(chan error, 1)
	go func() {
		producer <- f.store.WithDirectNamespace(ctx, authority, func(ns string) error {
			ready <- ns
			select {
			case <-release:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	var ns string
	select {
	case ns = <-ready:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	left := make(chan *httptest.ResponseRecorder, 1)
	go func() { left <- request(envelope) }()
	// Observe the real account lock held by the namespace producer. The leave
	// must see its committed public name before deleting the registration.
	waiting := false
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		if e = f.store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT id FROM accounts%')`).Scan(&waiting); e != nil {
			t.Fatal(e)
		}
		if waiting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !waiting {
		t.Fatal("leave did not serialize with namespace registration")
	}
	releaseOnce.Do(func() { close(release) })
	if e = <-producer; e != nil {
		t.Fatal(e)
	}
	select {
	case out := <-left:
		if out.Code != 204 || out.Body.Len() != 0 {
			t.Fatal("leave response", out.Code, out.Body.String())
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if out := request(envelope); out.Code != 204 || out.Body.Len() != 0 {
		t.Fatal("replay response", out.Code, out.Body.String())
	}
	var live, withdrawal int
	if e = f.store.DB.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM certificate_namespaces),(SELECT count(*) FROM direct_address_withdrawals WHERE namespace=$1)`, ns).Scan(&live, &withdrawal); e != nil || live != 0 || withdrawal != 1 {
		t.Fatal("late producer survived leave", live, withdrawal, e)
	}
	if e = f.store.WithDirectNamespace(ctx, authority, func(string) error { t.Error("old authority reached namespace producer"); return nil }); e == nil {
		t.Fatal("old authority resurrected registration")
	}
}

func TestTVReviewAndDecisionUseTheSamePorticoAccount(t *testing.T) {
	store, svc, ids, a, _ := activation(t)
	ctx := context.Background()
	other, e := ids.Register(ctx, "other-tv", "Other", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	auth := startTV(t, svc, 99)
	review, e := svc.Review(ctx, a.Account.ID, "192.0.2.2", auth.UserCode)
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.Decide(ctx, other.AccessToken, "192.0.2.3", review.RequestID, auth.UserCode, "approve"); e == nil {
		t.Fatal("another account approved stale review")
	}
	if _, e = svc.Review(ctx, other.Account.ID, "192.0.2.3", auth.UserCode); e == nil {
		t.Fatal("another account took reviewed consent")
	}
	if e = svc.Decide(ctx, a.AccessToken, "192.0.2.2", review.RequestID, auth.UserCode, "approve"); e != nil {
		t.Fatal(e)
	}
	due(t, store)
	grant, e := svc.Poll(ctx, auth.DeviceCode)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal(grant)
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil {
		t.Fatal("grant")
	}
	if _, ok := body["profiles"]; ok {
		t.Fatal("Hosted grant carried server profiles")
	}
	if grant.Account.ID != a.Account.ID {
		t.Fatal("wrong TV account")
	}
}

func TestPrivacyRetentionPrunesAddressesAtTwoDaysAndAuditAtSeven(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	for _, age := range []string{"1 day", "3 days"} {
		if _, e := f.store.DB.Exec(`INSERT INTO direct_address_events(server_id,namespace,address,observed_at) VALUES($1,'ptc-aaaaaaaaaaaaaaaaaaaa','8.8.8.8',clock_timestamp()-$2::interval)`, f.server, age); e != nil {
			t.Fatal(e)
		}
	}
	for _, age := range []string{"6 days", "8 days"} {
		if _, e := f.store.DB.Exec(`INSERT INTO security_audit(actor_id,action,target_id,created_at) VALUES('retention','retention','retention',clock_timestamp()-$1::interval)`, age); e != nil {
			t.Fatal(e)
		}
	}
	if e := f.store.PruneExpired(ctx, 100); e != nil {
		t.Fatal(e)
	}
	var addresses, audits int
	if e := f.store.DB.QueryRow(`SELECT count(*) FROM direct_address_events`).Scan(&addresses); e != nil || addresses != 1 {
		t.Fatal(addresses, e)
	}
	if e := f.store.DB.QueryRow(`SELECT count(*) FROM security_audit WHERE action='retention'`).Scan(&audits); e != nil || audits != 1 {
		t.Fatal(audits, e)
	}
}

func passwordHashR28(t *testing.T, password string) []byte {
	t.Helper()
	h, e := bcrypt.GenerateFromPassword([]byte(password), identity.PasswordCost)
	if e != nil {
		t.Fatal(e)
	}
	return h
}
