package postgres_test

import (
	"context"
	"strings"
	"testing"

	"portico.local/hosted/internal/directory"
)

// A20: removing a member (membership revoked or deleted, or a profile
// restricted) gives the server a new members-only label and a new feed
// sequence, so the removed member's name stops resolving. A repeated
// restriction sync does not rotate it again.
func TestMemberRemovalRotatesMemberLabel(t *testing.T) {
	f := setup(t)
	db := f.store.DB
	const ns = "ptc-aaaaaaaaaaaaaaaaaaaa"
	if _, e := db.Exec(`INSERT INTO certificate_namespaces(operation_id,namespace,member_label,server_identity) VALUES('fixture_installed_claim',$1,$2,$3)`, ns, "0123456789abcdef0123456789abcdef", f.server); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO direct_addresses(namespace,server_id,ipv4,sequence) VALUES($1,$2,'8.8.8.8',nextval('direct_address_sequence'))`, ns, f.server); e != nil {
		t.Fatal(e)
	}
	if _, e := db.Exec(`INSERT INTO memberships(server_id,account_id,profile_id,role) VALUES($1,$2,$3,'member') ON CONFLICT DO NOTHING`, f.server, f.member.Account.ID, f.member.Profiles[0].ID); e != nil {
		t.Fatal(e)
	}
	state := func() (string, int64) {
		t.Helper()
		var label string
		var seq int64
		if e := db.QueryRow(`SELECT n.member_label,d.sequence FROM certificate_namespaces n JOIN direct_addresses d ON d.namespace=n.namespace WHERE n.namespace=$1`, ns).Scan(&label, &seq); e != nil {
			t.Fatal(e)
		}
		return label, seq
	}
	label, seq := state()
	rotate := func() {
		t.Helper()
		if _, e := f.store.RotateQueuedMemberLabels(context.Background(), 100); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := db.Exec(`UPDATE memberships SET revoked=true WHERE server_id=$1 AND account_id=$2`, f.server, f.member.Account.ID); e != nil {
		t.Fatal(e)
	}
	// A87: the removal only queues the rotation; the feed lock is taken later,
	// in the rotation's own transaction.
	if queued, _ := state(); queued != label {
		t.Fatal("rotated inside the removal transaction")
	}
	rotate()
	next, nextSeq := state()
	if next == label || len(next) != 32 || nextSeq <= seq {
		t.Fatalf("membership revoke: label %s -> %s, sequence %d -> %d", label, next, seq, nextSeq)
	}
	restrict := `INSERT INTO restrictions(server_id,account_id,profile_id,local_revision,allowed_libraries,revoked) VALUES($1,$2,$3,1,'{}',true) ON CONFLICT(server_id,account_id,profile_id) DO UPDATE SET revoked=EXCLUDED.revoked,local_revision=EXCLUDED.local_revision`
	if _, e := db.Exec(restrict, f.server, f.owner.Account.ID, f.owner.Profiles[0].ID); e != nil {
		t.Fatal(e)
	}
	// The previous label is still in its 24-hour grace: the second rotation
	// waits instead of replacing it early.
	rotate()
	if held, _ := state(); held != next {
		t.Fatal("a second rotation cut the previous label's grace short")
	}
	if _, e := db.Exec(`UPDATE certificate_namespaces SET previous_label_until=clock_timestamp()-interval '1 second' WHERE namespace=$1`, ns); e != nil {
		t.Fatal(e)
	}
	rotate()
	third, _ := state()
	if third == next {
		t.Fatal("restriction did not rotate the label")
	}
	if _, e := db.Exec(restrict, f.server, f.owner.Account.ID, f.owner.Profiles[0].ID); e != nil {
		t.Fatal(e)
	}
	rotate()
	if again, _ := state(); again != third {
		t.Fatal("a repeated restriction sync rotated the label again")
	}
}

// A86: after a rotation the old name keeps resolving for the grace period (the
// DNS feed carries it), the server learns the new label from its presence
// answer, and a remaining member's route read rewrites the server's published
// public name to the new label. The removed member gets no routes at all.
func TestMemberLabelRotationKeepsRemainingMembersResolving(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	db := f.store.DB
	const ns, oldLabel = "ptc-aaaaaaaaaaaaaaaaaaaa", "0123456789abcdef0123456789abcdef"
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO certificate_namespaces(operation_id,namespace,member_label,server_identity) VALUES('fixture_installed_claim',$1,$2,$3)`, []any{ns, oldLabel, f.server}},
		{`INSERT INTO direct_addresses(namespace,server_id,ipv4,sequence) VALUES($1,$2,'8.8.8.8',nextval('direct_address_sequence'))`, []any{ns, f.server}},
		{`INSERT INTO server_members(server_id,account_id,role) VALUES($1,$2,'member'),($1,$3,'admin')`, []any{f.server, f.member.Account.ID, f.owner.Account.ID}},
		{`DELETE FROM memberships WHERE account_id=$1`, []any{f.member.Account.ID}},
		{`INSERT INTO networking_route_state(server_id,operation_id,claim_generation,credential_generation,generation,observation_digest,remote_enabled) VALUES($1,'fixture_installed_claim',1,1,1,$2,true)`, []any{f.server, make([]byte, 32)}},
		{`INSERT INTO networking_route_candidates(server_id,operation_id,generation,origin,class,state) VALUES($1,'fixture_installed_claim',1,$2,'public','reported')`, []any{f.server, "https://c-" + oldLabel + "." + ns + ".direct.getportico.tv:32500"}},
	} {
		if _, e := db.Exec(q.sql, q.args...); e != nil {
			t.Fatal(e)
		}
	}
	// A second member is removed from the server (its push drops the index row).
	if _, e := db.Exec(`DELETE FROM server_members WHERE server_id=$1 AND account_id=$2`, f.server, f.owner.Account.ID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.RotateQueuedMemberLabels(ctx, 100); e != nil {
		t.Fatal(e)
	}
	var newLabel string
	if e := db.QueryRow(`SELECT member_label FROM certificate_namespaces WHERE namespace=$1`, ns).Scan(&newLabel); e != nil || newLabel == oldLabel {
		t.Fatalf("not rotated: %s %v", newLabel, e)
	}
	changes, e := f.store.DirectAddressChanges(ctx, 0, 10)
	if e != nil || len(changes) != 1 || changes[0].Label != newLabel || changes[0].PreviousLabel != oldLabel || !strings.HasSuffix(changes[0].PreviousLabelUntil, "Z") {
		t.Fatalf("feed: %+v %v", changes, e)
	}
	authority, e := f.dir.CaptureServerAuthority(ctx, f.token)
	if e != nil {
		t.Fatal(e)
	}
	// The server learns the new label from its check-in (run on the wake the
	// removal's policy change sends).
	checkIn, e := f.dir.ServerHeartbeat(ctx, authority, directory.CheckInReport{})
	if e != nil || checkIn.RouteLabel != newLabel {
		t.Fatalf("check-in label %q %v", checkIn.RouteLabel, e)
	}
	sets, e := f.store.RouteSets(ctx, f.member.Account.ID)
	if e != nil || len(sets[f.server].Candidates) != 1 {
		t.Fatalf("routes: %+v %v", sets, e)
	}
	if got := sets[f.server].Candidates[0].BaseURL; got != "https://c-"+newLabel+"."+ns+".direct.getportico.tv:32500" {
		t.Fatalf("remaining member got %s", got)
	}
	// After the grace the feed stops carrying the old label.
	if _, e = db.Exec(`UPDATE certificate_namespaces SET previous_label_until=clock_timestamp()-interval '1 second' WHERE namespace=$1`, ns); e != nil {
		t.Fatal(e)
	}
	if changes, e = f.store.DirectAddressChanges(ctx, 0, 10); e != nil || changes[0].PreviousLabel != "" {
		t.Fatalf("grace not ended: %+v %v", changes, e)
	}
}
