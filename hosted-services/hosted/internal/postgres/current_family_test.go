package postgres_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/sessions"
	"strings"
	"testing"
	"time"
)

type currentFamilyFixture struct {
	store    *postgres.Store
	expected identity.CurrentFamilyExpectation
}

// Unlike setup, this fixture creates only accounts/families/access rows. It
// performs no enrollment, provider, HTTP, scanner or filesystem-media operation.
func currentFamilySetup(t *testing.T) currentFamilyFixture {
	t.Helper()
	s := database(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, id := range []string{"strict-owner", "strict-other"} {
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO accounts(id,username,display_name,password_hash) VALUES($1,$1,'Fixture',$2)`, id, []byte("not-used-by-these-authorization-tests")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO session_families(id,account_id,mode,current_refresh_hash,account_epoch,absolute_expires_at,idle_expires_at) VALUES($1,$2,'native',$3,0,clock_timestamp()+interval '1 hour',clock_timestamp()+interval '30 minutes')`, id+"-family", id, security.Hash(id+"-refresh")); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.ExecContext(ctx, `INSERT INTO account_sessions(token_hash,account_id,family_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '15 minutes')`, security.Hash(id+"-access"), id, id+"-family"); err != nil {
			t.Fatal(err)
		}
	}
	return currentFamilyFixture{s, identity.CurrentFamilyExpectation{TokenHash: security.Hash("strict-owner-access"), AccountID: "strict-owner", FamilyID: "strict-owner-family", AuthEpoch: 0, FamilyGeneration: 0, Mode: "native"}}
}

func strictTx(t *testing.T, s *postgres.Store) (context.Context, *sql.Tx) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tx.Rollback() })
	return ctx, tx
}

func TestCurrentFamilyAuthorizationBindingsAndExpiry(t *testing.T) {
	f := currentFamilySetup(t)
	cases := []struct {
		name, mutation string
		change         func(*identity.CurrentFamilyExpectation)
		valid          bool
		storageReject  bool
	}{
		{name: "epoch zero native", valid: true},
		{name: "browser", mutation: `UPDATE session_families SET mode='browser' WHERE id='strict-owner-family'`, change: func(e *identity.CurrentFamilyExpectation) { e.Mode = "browser" }, valid: true},
		{name: "wrong account", change: func(e *identity.CurrentFamilyExpectation) { e.AccountID = "strict-other" }},
		{name: "wrong family", change: func(e *identity.CurrentFamilyExpectation) { e.FamilyID = "strict-other-family" }},
		{name: "wrong mode", change: func(e *identity.CurrentFamilyExpectation) { e.Mode = "browser" }},
		{name: "wrong epoch", change: func(e *identity.CurrentFamilyExpectation) { e.AuthEpoch = 1 }},
		{name: "wrong generation", change: func(e *identity.CurrentFamilyExpectation) { e.FamilyGeneration = 1 }},
		{name: "unknown account", change: func(e *identity.CurrentFamilyExpectation) { e.AccountID = "unknown" }},
		{name: "unknown family", change: func(e *identity.CurrentFamilyExpectation) { e.FamilyID = "unknown" }},
		{name: "wrong access", change: func(e *identity.CurrentFamilyExpectation) { e.TokenHash = security.Hash("strict-other-access") }},
		{name: "deleted access", mutation: `DELETE FROM account_sessions WHERE account_id='strict-owner'`},
		{name: "corrupt cross-account access", storageReject: true, mutation: `UPDATE account_sessions SET account_id='strict-other' WHERE account_id='strict-owner'`},
		{name: "corrupt cross-account family", storageReject: true, mutation: `UPDATE session_families SET account_id='strict-other' WHERE id='strict-owner-family'`},
		{name: "family epoch changed", mutation: `UPDATE session_families SET account_epoch=1 WHERE id='strict-owner-family'`},
		{name: "revoked family", mutation: `UPDATE session_families SET revoked_at=clock_timestamp() WHERE id='strict-owner-family'`},
		{name: "access expired", mutation: `UPDATE account_sessions SET expires_at=clock_timestamp()-interval '1 second' WHERE account_id='strict-owner'`},
		{name: "absolute expired", mutation: `UPDATE session_families SET absolute_expires_at=clock_timestamp()-interval '1 second' WHERE id='strict-owner-family'`},
		{name: "idle expired", mutation: `UPDATE session_families SET idle_expires_at=clock_timestamp()-interval '1 second' WHERE id='strict-owner-family'`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, tx := strictTx(t, f.store)
			e := f.expected
			if c.change != nil {
				c.change(&e)
			}
			if c.mutation != "" {
				_, err := tx.ExecContext(ctx, c.mutation)
				if c.storageReject {
					requireSQLState(t, err, "23503")
					return
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			got, err := postgres.AuthorizeCurrentFamilyTx(ctx, tx, e)
			if !c.valid {
				if !errors.Is(err, identity.ErrCredentials) || got != (identity.CurrentFamilyFacts{}) {
					t.Fatal("stale authority not rejected", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.AccountID != e.AccountID || got.FamilyID != e.FamilyID || got.Mode != e.Mode || got.AuthEpoch != 0 || got.FamilyGeneration != 0 || got.CheckedAt.IsZero() || !got.UnexpiredAt(got.CheckedAt) {
				t.Fatal("wrong authority facts")
			}
			if got.UnexpiredAt(got.AccessExpiresAt) || got.UnexpiredAt(got.AbsoluteExpiresAt) || got.UnexpiredAt(got.IdleExpiresAt) {
				t.Fatal("expiry equality accepted")
			}
		})
	}
}

func TestCurrentFamilyMalformedInputAndRequiredFamily(t *testing.T) {
	f := currentFamilySetup(t)
	ctx, tx := strictTx(t, f.store)
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	// A finished transaction makes any accidental database query observable.
	bad := []func(*identity.CurrentFamilyExpectation){
		func(e *identity.CurrentFamilyExpectation) { e.TokenHash = nil },
		func(e *identity.CurrentFamilyExpectation) { e.TokenHash = make([]byte, 33) },
		func(e *identity.CurrentFamilyExpectation) { e.AccountID = "" },
		func(e *identity.CurrentFamilyExpectation) { e.FamilyID = "contains space" },
		func(e *identity.CurrentFamilyExpectation) { e.AccountID = "nonascii-é" },
		func(e *identity.CurrentFamilyExpectation) { e.FamilyID = string(make([]byte, 129)) },
		func(e *identity.CurrentFamilyExpectation) { e.AuthEpoch = -1 },
		func(e *identity.CurrentFamilyExpectation) { e.FamilyGeneration = -1 },
		func(e *identity.CurrentFamilyExpectation) { e.Mode = "unknown" },
	}
	for i, change := range bad {
		e := f.expected
		change(&e)
		if _, err := postgres.AuthorizeCurrentFamilyTx(ctx, tx, e); !errors.Is(err, identity.ErrCredentials) {
			t.Fatalf("malformed case %d reached database: %v", i, err)
		}
	}
	if _, err := postgres.AuthorizeCurrentFamilyTx(ctx, tx, f.expected); !errors.Is(err, sql.ErrTxDone) {
		t.Fatal("valid input did not reach finished transaction", err)
	}
	if _, err := f.store.DB.ExecContext(ctx, `UPDATE account_sessions SET family_id=NULL WHERE account_id='strict-owner'`); err == nil {
		t.Fatal("current schema allowed an unbound session")
	}
	if viewer, err := f.store.Authenticate(ctx, f.expected.TokenHash); err != nil || viewer.Account.ID != f.expected.AccountID {
		t.Fatal("current bound session authentication failed", err)
	}
	ctx2, tx2 := strictTx(t, f.store)
	if _, err := postgres.AuthorizeCurrentFamilyTx(ctx2, tx2, f.expected); err != nil {
		t.Fatal("current bound family rejected", err)
	}
}

type strictOutcome struct {
	facts identity.CurrentFamilyFacts
	err   error
}

func waitStrictBlock(t *testing.T, db *sql.DB, waiter, blocker int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var blocked bool
		if err := db.QueryRowContext(ctx, `SELECT $2=ANY(pg_blocking_pids($1))`, waiter, blocker).Scan(&blocked); err != nil {
			t.Fatal("block observation", err)
		}
		if blocked {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("expected transaction did not block")
		case <-tick.C:
		}
	}
}
func strictPID(t *testing.T, ctx context.Context, tx *sql.Tx) int {
	t.Helper()
	var pid int
	if err := tx.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatal(err)
	}
	return pid
}
func strictResult(t *testing.T, ch <-chan strictOutcome) strictOutcome {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(3 * time.Second):
		t.Fatal("authorization did not settle")
		return strictOutcome{}
	}
}

func TestCurrentFamilyLockWaitUsesFreshClockAndCancellation(t *testing.T) {
	for _, cancelWait := range []bool{false, true} {
		name := "expired after transaction start"
		if cancelWait {
			name = "cancelled lock wait"
		}
		t.Run(name, func(t *testing.T) {
			f := currentFamilySetup(t)
			lockCtx, blocker := strictTx(t, f.store)
			blockerPID := strictPID(t, lockCtx, blocker)
			if _, err := blocker.ExecContext(lockCtx, `SELECT id FROM accounts WHERE id='strict-owner' FOR UPDATE`); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			waiter, err := f.store.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer waiter.Rollback()
			waiterPID := strictPID(t, ctx, waiter)
			var began time.Time
			if err = waiter.QueryRowContext(ctx, `SELECT transaction_timestamp()`).Scan(&began); err != nil {
				t.Fatal(err)
			}
			ch := make(chan strictOutcome, 1)
			go func() {
				facts, err := postgres.AuthorizeCurrentFamilyTx(ctx, waiter, f.expected)
				ch <- strictOutcome{facts, err}
			}()
			waitStrictBlock(t, f.store.DB, waiterPID, blockerPID)
			if cancelWait {
				cancel()
				r := strictResult(t, ch)
				if !errors.Is(r.err, context.Canceled) {
					t.Fatal("cancel not propagated", r.err)
				}
				return
			}
			// This expiry is newer than waiter's transaction timestamp, but is already
			// past at release. A query using now() would incorrectly authorize it.
			if _, err = blocker.ExecContext(lockCtx, `UPDATE account_sessions SET expires_at=$1::timestamptz+interval '1 microsecond' WHERE account_id='strict-owner'`, began); err != nil {
				t.Fatal(err)
			}
			var alreadyPast bool
			if err = blocker.QueryRowContext(lockCtx, `SELECT clock_timestamp()>$1::timestamptz+interval '1 microsecond'`, began).Scan(&alreadyPast); err != nil || !alreadyPast {
				t.Fatal("invalid expiry schedule", err)
			}
			if err = blocker.Commit(); err != nil {
				t.Fatal(err)
			}
			r := strictResult(t, ch)
			if !errors.Is(r.err, identity.ErrCredentials) {
				t.Fatal("lock wait used stale clock", r.err)
			}
		})
	}
}

func TestCurrentFamilyGenerationAndRevocationSerialization(t *testing.T) {
	f := currentFamilySetup(t)
	ctx, tx := strictTx(t, f.store)
	if _, err := postgres.AuthorizeCurrentFamilyTx(ctx, tx, f.expected); err != nil {
		t.Fatal(err)
	}
	blockerPID := strictPID(t, ctx, tx)
	otherCtx, other := strictTx(t, f.store)
	otherPID := strictPID(t, otherCtx, other)
	changed := make(chan strictOutcome, 1)
	go func() {
		_, err := other.ExecContext(otherCtx, `SELECT id FROM accounts WHERE id='strict-owner' FOR SHARE`)
		if err == nil {
			_, err = other.ExecContext(otherCtx, `UPDATE session_families SET generation=generation+1 WHERE id='strict-owner-family'`)
		}
		if err == nil {
			err = other.Commit()
		}
		changed <- strictOutcome{err: err}
	}()
	waitStrictBlock(t, f.store.DB, otherPID, blockerPID)
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if r := strictResult(t, changed); r.err != nil {
		t.Fatal(r.err)
	}
	// The old access row remains ordinarily valid after rotation; only the
	// generation-bound sensitive approval is stale.
	if _, err := f.store.Authenticate(ctx, f.expected.TokenHash); err != nil {
		t.Fatal("ordinary access invalidated", err)
	}
	ctx2, tx2 := strictTx(t, f.store)
	if _, err := postgres.AuthorizeCurrentFamilyTx(ctx2, tx2, f.expected); !errors.Is(err, identity.ErrCredentials) {
		t.Fatal("old generation survived", err)
	}
	if err := tx2.Rollback(); err != nil {
		t.Fatal(err)
	}
	fresh := f.expected
	fresh.FamilyGeneration = 1
	ctx3, tx3 := strictTx(t, f.store)
	if _, err := postgres.AuthorizeCurrentFamilyTx(ctx3, tx3, fresh); err != nil {
		t.Fatal("explicit current expectation rejected", err)
	}
	if err := tx3.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := f.store.LogoutAll(ctx, f.expected.AccountID); err != nil {
		t.Fatal(err)
	}
	ctx4, tx4 := strictTx(t, f.store)
	if _, err := postgres.AuthorizeCurrentFamilyTx(ctx4, tx4, fresh); !errors.Is(err, identity.ErrCredentials) {
		t.Fatal("epoch/logout survived", err)
	}
}

func TestCurrentFamilyActualRefreshRetainsAccessButInvalidatesApproval(t *testing.T) {
	f := currentFamilySetup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := f.store.DB.ExecContext(ctx, `INSERT INTO refresh_tokens(token_hash,family_id,generation) VALUES($1,$2,0)`, security.Hash("strict-owner-refresh"), f.expected.FamilyID); err != nil {
		t.Fatal(err)
	}
	_, err := f.store.RotateRefresh(ctx, sessions.Rotation{FamilyID: f.expected.FamilyID, Mode: "native", RequestID: "strict-refresh-operation", InputHash: security.Hash("strict-owner-refresh"), OutputHash: security.Hash("strict-new-refresh"), AccessHash: security.Hash("strict-new-access"), Cipher: []byte("test-only-opaque-repository-receipt"), ExpiresAt: time.Now().Add(5 * time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.Authenticate(ctx, f.expected.TokenHash); err != nil {
		t.Fatal("actual refresh removed ordinary old access", err)
	}
	checkCtx, tx := strictTx(t, f.store)
	if _, err = postgres.AuthorizeCurrentFamilyTx(checkCtx, tx, f.expected); !errors.Is(err, identity.ErrCredentials) {
		t.Fatal("stored approval survived actual refresh", err)
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	current := f.expected
	current.FamilyGeneration = 1
	current.TokenHash = security.Hash("strict-new-access")
	freshCtx, freshTx := strictTx(t, f.store)
	if _, err = postgres.AuthorizeCurrentFamilyTx(freshCtx, freshTx, current); err != nil {
		t.Fatal("fresh explicit authority rejected", err)
	}
}

func TestCurrentFamilyAuthorizationUsesBoundedPrimaryKeyReads(t *testing.T) {
	f := currentFamilySetup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	// Populate enough rows for realistic planner choices. All rows remain in this
	// disposable schema; no query borrows actual accounts or reads their values.
	for _, query := range []string{
		`INSERT INTO accounts(id,username,display_name,password_hash) SELECT 'plan-account-'||n,'plan-user-'||n,'Fixture',decode(repeat('01',32),'hex') FROM generate_series(1,2000)n`,
		`INSERT INTO session_families(id,account_id,mode,current_refresh_hash,account_epoch,absolute_expires_at,idle_expires_at) SELECT 'plan-family-'||n,'plan-account-'||n,'native',decode(lpad(to_hex(n),64,'0'),'hex'),0,clock_timestamp()+interval '1 hour',clock_timestamp()+interval '30 minutes' FROM generate_series(1,2000)n`,
		`INSERT INTO account_sessions(token_hash,account_id,family_id,expires_at) SELECT decode(lpad(to_hex(n),64,'0'),'hex'),'plan-account-'||n,'plan-family-'||n,clock_timestamp()+interval '15 minutes' FROM generate_series(1,2000)n`,
		`ANALYZE accounts`, `ANALYZE session_families`, `ANALYZE account_sessions`,
	} {
		if _, err := f.store.DB.ExecContext(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := f.store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	// A fixture table is small enough that a local planner may prefer a sequential
	// scan even where a key exists. The property under test is that each read is
	// keyed and bounded, so make the keyed plan the only one available and then
	// require it: if the key were missing, planning fails instead of passing.
	if _, err = tx.ExecContext(ctx, `SET LOCAL enable_seqscan=off; SET LOCAL enable_bitmapscan=off`); err != nil {
		t.Fatal(err)
	}
	// Several unique indexes can serve the same keyed predicate, and which one a
	// given server version costs lowest is not the property under test. Each read
	// must reach its row through a unique index whose leading column is the key it
	// was given, and must never scan the table.
	plans := []struct {
		query, table, key string
		arg               any
	}{
		{`SELECT id,auth_epoch FROM accounts WHERE id=$1 FOR UPDATE`, "accounts", "id", f.expected.AccountID},
		{`SELECT id,account_id,account_epoch,generation,mode,absolute_expires_at,idle_expires_at,revoked_at FROM session_families WHERE id=$1 FOR SHARE`, "session_families", "id", f.expected.FamilyID},
		{`SELECT account_id,family_id,expires_at FROM account_sessions WHERE token_hash=$1 FOR SHARE`, "account_sessions", "token_hash", f.expected.TokenHash},
	}
	type planNode struct {
		Node  string            `json:"Node Type"`
		Index string            `json:"Index Name"`
		Rows  float64           `json:"Actual Rows"`
		Loops float64           `json:"Actual Loops"`
		Plans []json.RawMessage `json:"Plans"`
	}
	for _, p := range plans {
		var keyed []string
		rows, e := tx.QueryContext(ctx, `SELECT c.relname FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid JOIN pg_class t ON t.oid=i.indrelid JOIN pg_namespace n ON n.oid=t.relnamespace WHERE n.nspname=current_schema() AND t.relname=$1 AND i.indisunique AND i.indkey[0]=(SELECT attnum FROM pg_attribute WHERE attrelid=t.oid AND attname=$2)`, p.table, p.key)
		if e != nil {
			t.Fatal(e)
		}
		for rows.Next() {
			var name string
			if e = rows.Scan(&name); e != nil {
				rows.Close()
				t.Fatal(e)
			}
			keyed = append(keyed, name)
		}
		rows.Close()
		if e = rows.Err(); e != nil {
			t.Fatal(e)
		}
		if len(keyed) == 0 {
			t.Fatal("no unique key leads on", p.table, p.key)
		}
		var raw []byte
		if err = tx.QueryRowContext(ctx, `EXPLAIN (ANALYZE,FORMAT JSON) `+p.query, p.arg).Scan(&raw); err != nil {
			t.Fatal(err)
		}
		var envelope []struct {
			Plan json.RawMessage `json:"Plan"`
		}
		if err = json.Unmarshal(raw, &envelope); err != nil || len(envelope) != 1 {
			t.Fatal("invalid plan", err)
		}
		found := false
		var walk func(json.RawMessage)
		walk = func(raw json.RawMessage) {
			var node planNode
			if e := json.Unmarshal(raw, &node); e != nil {
				t.Fatal(e)
			}
			if node.Rows > 1 || node.Loops > 1 {
				t.Fatal("unbounded keyed query plan")
			}
			if strings.Contains(node.Node, "Seq Scan") {
				t.Fatal("keyed read scanned the table", p.table)
			}
			for _, name := range keyed {
				if node.Index == name {
					found = true
				}
			}
			for _, child := range node.Plans {
				walk(child)
			}
		}
		walk(envelope[0].Plan)
		if !found {
			t.Fatal("keyed read did not use a unique key on", p.table, p.key, keyed)
		}
	}
}
