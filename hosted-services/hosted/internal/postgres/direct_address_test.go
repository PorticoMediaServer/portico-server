package postgres_test

import (
	"context"
	"database/sql"
	"net/netip"
	"testing"

	"portico.local/hosted/internal/postgres"
)

func TestDirectAddressFeedOnlyMovesOnAChange(t *testing.T) {
	store := database(t)
	ctx := context.Background()
	if _, e := store.DB.Exec(`INSERT INTO certificate_namespaces(operation_id,namespace) VALUES('op_a','ptc-aaaaaaaaaaaaaaaaaaaa'),('op_b','ptc-aaaaaaaaaaaaaaaaaaba')`); e != nil {
		t.Fatal(e)
	}
	record := func(ns, ip string) bool {
		t.Helper()
		tx, e := store.DB.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback()
		changed, e := postgres.RecordDirectAddressTxForTest(ctx, tx, "srv_"+ns, ns, netip.MustParseAddr(ip))
		if e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
		return changed
	}
	a, b := "ptc-aaaaaaaaaaaaaaaaaaaa", "ptc-aaaaaaaaaaaaaaaaaaba"
	if !record(a, "156.57.135.50") || record(a, "156.57.135.50") {
		t.Fatal("a repeated report of the same address must write nothing")
	}
	first, e := store.DirectAddressChanges(ctx, 0, 10)
	if e != nil || len(first) != 1 || first[0].IPv4 != "156.57.135.50" {
		t.Fatal(first, e)
	}
	if !record(b, "2606:4700:4700::1111") || !record(a, "156.57.200.9") || !record(a, "2606:4700:4700::1001") {
		t.Fatal("changes were not recorded")
	}
	// A reader resuming from what it applied sees every later change once, oldest first, and a
	// second address family joins the row rather than replacing the first.
	next, e := store.DirectAddressChanges(ctx, first[0].Sequence, 10)
	if e != nil || len(next) != 2 || next[0].Namespace != b || next[1].Namespace != a || next[1].IPv4 != "156.57.200.9" || next[1].IPv6 != "2606:4700:4700::1001" || next[0].Sequence >= next[1].Sequence {
		t.Fatal(next, e)
	}
	if rest, _ := store.DirectAddressChanges(ctx, next[1].Sequence, 10); len(rest) != 0 {
		t.Fatal(rest)
	}
	var events int
	if e = store.DB.QueryRow(`SELECT count(*) FROM direct_address_events WHERE namespace=$1`, a).Scan(&events); e != nil || events != 3 {
		t.Fatal("every WAN address change is logged", events, e)
	}
}

// A withdrawal is a change like any other: it takes the next feed sequence, so the DNS nodes
// learn about it the same way they learn about a new address, and it leaves a tombstone in the
// event log. Before it existed the column simply stayed and the nodes answered it for ever.
func TestWithdrawingAFamilyIsAChangeTheFeedCarries(t *testing.T) {
	store := database(t)
	ctx := context.Background()
	if _, e := store.DB.Exec(`INSERT INTO certificate_namespaces(operation_id,namespace) VALUES('op_a','ptc-aaaaaaaaaaaaaaaaaaaa')`); e != nil {
		t.Fatal(e)
	}
	ns := "ptc-aaaaaaaaaaaaaaaaaaaa"
	inTx := func(apply func(tx *sql.Tx) (bool, error)) bool {
		t.Helper()
		tx, e := store.DB.BeginTx(ctx, nil)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback()
		changed, e := apply(tx)
		if e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(); e != nil {
			t.Fatal(e)
		}
		return changed
	}
	for _, ip := range []string{"156.57.135.50", "2606:4700:4700::1111"} {
		if !inTx(func(tx *sql.Tx) (bool, error) {
			return postgres.RecordDirectAddressTxForTest(ctx, tx, "srv_1", ns, netip.MustParseAddr(ip))
		}) {
			t.Fatal("the address was not recorded", ip)
		}
	}
	if !inTx(func(tx *sql.Tx) (bool, error) {
		return postgres.WithdrawDirectAddressTxForTest(ctx, tx, "srv_1", ns, "ipv6")
	}) {
		t.Fatal("a published family reported nothing to withdraw")
	}
	rows, e := store.DirectAddressChanges(ctx, 0, 10)
	if e != nil || len(rows) != 1 || rows[0].IPv6 != "" || rows[0].IPv4 != "156.57.135.50" {
		t.Fatal("the feed does not carry the withdrawal", rows, e)
	}
	// Idempotent: a repeat spends no sequence and writes no event, so a lost response costs
	// one harmless retry.
	before := rows[0].Sequence
	if inTx(func(tx *sql.Tx) (bool, error) {
		return postgres.WithdrawDirectAddressTxForTest(ctx, tx, "srv_1", ns, "ipv6")
	}) {
		t.Fatal("a repeated withdrawal reported a change")
	}
	if rows, e = store.DirectAddressChanges(ctx, 0, 10); e != nil || len(rows) != 1 || rows[0].Sequence != before {
		t.Fatal("a repeated withdrawal moved the feed", rows, e)
	}
	// The tombstone: an empty address with the previous one recorded, so the WAN-change log
	// says what happened rather than going quiet.
	var previous string
	if e = store.DB.QueryRow(`SELECT previous FROM direct_address_events WHERE namespace=$1 AND address='' ORDER BY id DESC LIMIT 1`, ns).Scan(&previous); e != nil || previous != "2606:4700:4700::1111" {
		t.Fatal("no tombstone was logged", previous, e)
	}
	// Withdrawing the last family leaves a row with nothing in it, which the DNS table reads
	// as "this name no longer exists".
	if !inTx(func(tx *sql.Tx) (bool, error) {
		return postgres.WithdrawDirectAddressTxForTest(ctx, tx, "srv_1", ns, "ipv4")
	}) {
		t.Fatal("the last family could not be withdrawn")
	}
	if rows, e = store.DirectAddressChanges(ctx, 0, 10); e != nil || len(rows) != 1 || rows[0].IPv4 != "" || rows[0].IPv6 != "" {
		t.Fatal(rows, e)
	}
	// Only the two families exist; nothing else reaches a column name.
	if _, e = store.DB.BeginTx(ctx, nil); e != nil {
		t.Fatal(e)
	}
	tx, _ := store.DB.BeginTx(ctx, nil)
	defer tx.Rollback()
	if _, e = postgres.WithdrawDirectAddressTxForTest(ctx, tx, "srv_1", ns, "ipv4; DROP TABLE direct_addresses"); e == nil {
		t.Fatal("an arbitrary family name was accepted")
	}
}
