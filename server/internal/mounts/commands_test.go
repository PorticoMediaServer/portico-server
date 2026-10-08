package mounts

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func operation(n int) string {
	return fmt.Sprintf("%013d-00000000-0000-4000-8000-%012d", time.Now().UnixMilli(), n)
}
func allowCommand(*sql.Tx) error { return nil }
func TestRemotePathValidationLiteralAndControlCharacters(t *testing.T) {
	config := "[cloud]\ntype=http\nurl=http://fixture.invalid\n"
	for _, path := range []string{"cloud:", "cloud:root", "cloud:2026", "cloud:Movies and music/season01", "cloud:作品"} {
		if e := validateConfig(config, path); e != nil {
			t.Fatal("valid path rejected", path, e)
		}
	}
	for _, path := range []string{"cloud:media\nabc", "cloud:media\rabc", "cloud:\x00", "cloud:\x7f", "cloud:\u0085", strings.Repeat("a", 2049), "cloud:" + string([]byte{255})} {
		if validateConfig(config, path) == nil {
			t.Fatal("hostile path accepted")
		}
	}
}
func TestCommandReceiptsReplayCASAndRemovalRecovery(t *testing.T) {
	s, input := fixture(t)
	ctx := context.Background()
	actor := "local:owner:profile"
	create := Command{OperationID: operation(1), Action: "create", Name: input.Name, Executable: input.Executable, Remote: input.Remote, Config: input.Config}
	receipt, e := s.Command(ctx, actor, "", create, allowCommand)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Command(ctx, actor, "", create, allowCommand)
	if e != nil || again.MountID != receipt.MountID {
		t.Fatal("create replay", again, e)
	}
	rows, _ := s.List()
	if len(rows) != 1 || rows[0].ControlRevision != 1 {
		t.Fatal(rows)
	}
	changed := create
	changed.Config += "\n# changed"
	if _, e = s.Command(ctx, actor, "", changed, allowCommand); !errors.Is(e, ErrCommandConflict) {
		t.Fatal("changed credentials admitted", e)
	}
	if _, e = s.Command(ctx, "other", "", create, allowCommand); !errors.Is(e, ErrCommandConflict) {
		t.Fatal("other principal replay", e)
	}
	denied := errors.New("revoked")
	if _, e = s.Command(ctx, actor, "", create, func(*sql.Tx) error { return denied }); !errors.Is(e, denied) {
		t.Fatal("revoked receipt", e)
	}
	start := Command{OperationID: operation(2), Action: "start", ExpectedRevision: 1}
	r, e := s.Command(ctx, actor, receipt.MountID, start, allowCommand)
	if e != nil || r.ControlRevision != 2 || r.Mount.DesiredState != "running" {
		t.Fatal(r, e)
	}
	restart := Command{OperationID: operation(3), Action: "restart", ExpectedRevision: 2}
	r, e = s.Command(ctx, actor, receipt.MountID, restart, allowCommand)
	if e != nil {
		t.Fatal(e)
	}
	var generation int
	if e = s.db.QueryRow(`SELECT restart_generation FROM mount_controls WHERE mount_id=?`, receipt.MountID).Scan(&generation); e != nil || generation != 1 {
		t.Fatal(generation, e)
	}
	for n := 0; n < 3; n++ {
		if _, e = s.Command(ctx, actor, receipt.MountID, restart, allowCommand); e != nil {
			t.Fatal(e)
		}
	}
	s.db.QueryRow(`SELECT restart_generation FROM mount_controls WHERE mount_id=?`, receipt.MountID).Scan(&generation)
	if generation != 1 {
		t.Fatal("restart duplicated")
	}
	if _, e = s.Command(ctx, actor, receipt.MountID, Command{OperationID: operation(4), Action: "stop", ExpectedRevision: 2}, allowCommand); !errors.Is(e, ErrCommandConflict) {
		t.Fatal("stale CAS", e)
	}
	stop := Command{OperationID: operation(5), Action: "stop", ExpectedRevision: 3}
	if _, e = s.Command(ctx, actor, receipt.MountID, stop, allowCommand); e != nil {
		t.Fatal(e)
	}
	remove := Command{OperationID: operation(6), Action: "delete", ExpectedRevision: 4}
	r, e = s.Command(ctx, actor, receipt.MountID, remove, allowCommand)
	if e != nil || !r.Deleted || !r.Mount.RemovalPending || len(r.Mount.Actions) != 0 {
		t.Fatal("removal acceptance", r, e)
	}
	// Simulate server restart after accepted removal but before filesystem cleanup.
	newer, e := New(s.db, filepath.Dir(s.private), s.root, s.helper, s.storage)
	if e != nil {
		t.Fatal(e)
	}
	newer.observe = func(context.Context, string) (bool, error) { return false, nil }
	newer.reconcile(ctx)
	if _, e = newer.get(receipt.MountID); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("removal did not recover", e)
	}
	if _, e = os.Stat(filepath.Join(s.private, receipt.MountID+".conf")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("config not cleaned")
	}
	if _, e = newer.Command(ctx, actor, receipt.MountID, remove, allowCommand); e != nil {
		t.Fatal("delete replay after removal", e)
	}
	// Permanent old-ID freshness fence remains after receipt payload retention.
	old := create
	old.OperationID = fmt.Sprintf("%013d-00000000-0000-4000-8000-000000000007", time.Now().Add(-31*24*time.Hour).UnixMilli())
	if _, e = newer.Command(ctx, actor, "", old, allowCommand); !errors.Is(e, ErrCommandExpired) {
		t.Fatal("expired create reapplied", e)
	}
	var raw string
	s.db.QueryRow(`SELECT response||fingerprint FROM mount_operations WHERE operation_id=?`, create.OperationID).Scan(&raw)
	if strings.Contains(raw, "leaky-secret") || strings.Contains(raw, "private.example") {
		t.Fatal("credential receipt leak")
	}
}
func TestConcurrentCreateOneEffectAndRetainedRetryBeyondFreshWindow(t *testing.T) {
	s, input := fixture(t)
	c := Command{OperationID: operation(20), Action: "create", Name: input.Name, Executable: input.Executable, Remote: input.Remote, Config: input.Config}
	var wg sync.WaitGroup
	ids := make(chan string, 4)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, e := s.Command(context.Background(), "owner", "", c, allowCommand)
			if e != nil {
				t.Error(e)
			}
			ids <- r.MountID
		}()
	}
	wg.Wait()
	close(ids)
	id := ""
	for value := range ids {
		if id != "" && value != id {
			t.Fatal("duplicate create")
		}
		id = value
	}
	list, _ := s.List()
	if len(list) != 1 {
		t.Fatal(list)
	}
	// Existing receipt is checked before fresh-ID admission, so aged legitimate retry remains exact.
	oldID := fmt.Sprintf("%013d-00000000-0000-4000-8000-000000000021", time.Now().Add(-time.Hour).UnixMilli())
	old := c
	old.OperationID = oldID
	hash := s.fingerprint("owner", "", old)
	_, e := s.db.Exec(`UPDATE mount_operations SET operation_id=?,fingerprint=?,response=json_set(response,'$.operationId',?) WHERE operation_id=?`, oldID, hash, oldID, c.OperationID)
	if e != nil {
		t.Fatal(e)
	}
	r, e := s.Command(context.Background(), "owner", "", old, allowCommand)
	if e != nil || r.MountID != id {
		t.Fatal("retained receipt window", r, e)
	}
}

func TestReceiptRestartUsesOneOwnedReplacement(t *testing.T) {
	s, input := fixture(t)
	ctx := context.Background()
	actor := "owner"
	r, e := s.Command(ctx, actor, "", Command{OperationID: operation(30), Action: "create", Name: input.Name, Executable: input.Executable, Remote: input.Remote, Config: input.Config}, allowCommand)
	if e != nil {
		t.Fatal(e)
	}
	id := r.MountID
	s.observe = func(context.Context, string) (bool, error) { return len(s.active) > 0, nil }
	s.observeIdentity = func(context.Context, string) (string, error) { return "fixture-filesystem", nil }
	if _, e = s.Command(ctx, actor, id, Command{OperationID: operation(31), Action: "start", ExpectedRevision: 1}, allowCommand); e != nil {
		t.Fatal(e)
	}
	pidPath := r.Mount.MountPath + ".fixture-pid"
	waitPID := func(old string) string {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			s.reconcile(ctx)
			raw, err := os.ReadFile(pidPath)
			if err == nil && string(raw) != old {
				return string(raw)
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatal("fixture owned replacement unavailable")
		return ""
	}
	first := waitPID("")
	s.reconcile(ctx)
	firstLease, leaseErr := s.LeaseFor(r.Mount.MountPath)
	if leaseErr != nil {
		t.Fatal(leaseErr)
	}
	restart := Command{OperationID: operation(32), Action: "restart", ExpectedRevision: 2}
	if _, e = s.Command(ctx, actor, id, restart, allowCommand); e != nil {
		t.Fatal(e)
	}
	if firstLease.Lifetime.Err() == nil {
		t.Fatal("restart retained old runtime authority")
	}
	second := waitPID(first)
	s.reconcile(ctx)
	secondLease, leaseErr := s.LeaseFor(r.Mount.MountPath)
	if leaseErr != nil || secondLease.ID == firstLease.ID {
		t.Fatal("replacement did not receive independent runtime identity", leaseErr)
	}
	if second == first {
		t.Fatal("replacement absent")
	}
	for i := 0; i < 3; i++ {
		if _, e = s.Command(ctx, actor, id, restart, allowCommand); e != nil {
			t.Fatal(e)
		}
		s.reconcile(ctx)
	}
	raw, _ := os.ReadFile(pidPath)
	if string(raw) != second || s.active[id].stopping {
		t.Fatal("exact retry restarted child")
	}
	if _, e = s.Command(ctx, actor, id, Command{OperationID: operation(33), Action: "delete", ExpectedRevision: 3}, allowCommand); e == nil {
		t.Fatal("active delete admitted")
	}
	if _, e = s.Command(ctx, actor, id, Command{OperationID: operation(34), Action: "stop", ExpectedRevision: 3}, allowCommand); e != nil {
		t.Fatal(e)
	}
	deadline := time.Now().Add(5 * time.Second)
	for len(s.active) > 0 && time.Now().Before(deadline) {
		s.reconcile(ctx)
		time.Sleep(10 * time.Millisecond)
	}
	if len(s.active) > 0 {
		t.Fatal("owned child not reaped")
	}
}
func TestPendingRemovalCrashAfterDirectoryRemoval(t *testing.T) {
	s, input := fixture(t)
	r, e := s.Command(context.Background(), "owner", "", Command{OperationID: operation(40), Action: "create", Name: input.Name, Executable: input.Executable, Remote: input.Remote, Config: input.Config}, allowCommand)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Command(context.Background(), "owner", r.MountID, Command{OperationID: operation(41), Action: "delete", ExpectedRevision: 1}, allowCommand); e != nil {
		t.Fatal(e)
	}
	if e = os.Remove(r.Mount.MountPath); e != nil {
		t.Fatal(e)
	}
	s.reconcile(context.Background())
	if _, e = s.get(r.MountID); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("missing directory crash recovery stuck", e)
	}
}

func TestCommandReceiptCapacityAndRetentionCleanup(t *testing.T) {
	s, _ := fixture(t)
	_, e := s.db.Exec(`WITH RECURSIVE n(x) AS (VALUES(1) UNION ALL SELECT x+1 FROM n WHERE x<4096) INSERT INTO mount_operations SELECT 'capacity-'||x,'owner','hash','mount','{}',? FROM n`, time.Now().Unix())
	if e != nil {
		t.Fatal(e)
	}
	command := Command{OperationID: operation(900), Action: "stop", ExpectedRevision: 1}
	if _, e = s.Command(context.Background(), "owner", "absent", command, allowCommand); !errors.Is(e, ErrCommandCapacity) {
		t.Fatal("capacity not enforced", e)
	}
	if _, e = s.db.Exec(`UPDATE mount_operations SET created_at=?`, time.Now().Add(-31*24*time.Hour).Unix()); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Command(context.Background(), "owner", "absent", command, allowCommand); !errors.Is(e, sql.ErrNoRows) {
		t.Fatal("retention not cleared", e)
	}
	var n int
	s.db.QueryRow(`SELECT count(*) FROM mount_operations`).Scan(&n)
	if n != 0 {
		t.Fatal("old receipts retained", n)
	}
}
