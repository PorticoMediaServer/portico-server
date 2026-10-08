package postgres_test

import (
	"context"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"strings"
	"testing"
	"time"
)

func requireSQLState(t *testing.T, err error, code string) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != code {
		t.Fatalf("wanted SQL state %s, received %v", code, err)
	}
}

// requireForeignKeyViolation accepts the whole integrity-violation family a
// referencing row can raise. PostgreSQL reports a RESTRICT fence as 23001 and a
// NO ACTION one as 23503, and which of the two a given server version picks is
// not the property under test: that the write was refused by a key is.
func requireForeignKeyViolation(t *testing.T, err error) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || (pg.Code != "23503" && pg.Code != "23001") {
		t.Fatalf("wanted a foreign key violation, received %v", err)
	}
}

func TestCurrentIdentitySchemaRequiresCurrentFormatAndRestart(t *testing.T) {
	f := currentFamilySetup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := f.store.Migrate(ctx); err != nil {
		t.Fatal("current-format restart failed", err)
	}
	var enabled bool
	if err := f.store.DB.QueryRowContext(ctx, `SELECT enabled FROM identity_protocol_policy`).Scan(&enabled); err != nil || enabled {
		t.Fatal("provider policy enabled by schema initialization", err)
	}
	if _, err := f.store.DB.ExecContext(ctx, `UPDATE accounts SET password_hash=NULL WHERE id='strict-owner'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB.ExecContext(ctx, `UPDATE accounts SET password_hash=''::bytea WHERE id='strict-owner'`); err == nil {
		t.Fatal("empty credential accepted")
	} else {
		requireSQLState(t, err, "23514")
	}
	if _, err := f.store.DB.ExecContext(ctx, `UPDATE account_sessions SET family_id=NULL WHERE account_id='strict-owner'`); err == nil {
		t.Fatal("unbound current session accepted")
	} else {
		requireSQLState(t, err, "23502")
	}
	// A missing exact marker cannot silently adopt existing relations.
	tx, err := f.store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DROP TABLE hosted_schema_contract`); err != nil {
		t.Fatal(err)
	}
	if err = postgres.InitializeCurrentHostedSchemaTx(ctx, tx); err == nil {
		t.Fatal("unknown schema was adopted")
	}
	if err = tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = f.store.Migrate(ctx); err != nil {
		t.Fatal("rejected initialization changed current schema", err)
	}

}

func seedIdentityAttempt(t *testing.T, f currentFamilyFixture) time.Time {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var at time.Time
	if err := f.store.DB.QueryRowContext(ctx, `SELECT clock_timestamp()`).Scan(&at); err != nil {
		t.Fatal(err)
	}
	statements := []struct {
		q    string
		args []any
	}{
		{`INSERT INTO identity_provider_descriptors(id,provider,revision,issuer,audience,callback_uri) VALUES('fixture-google','google',1,'https://accounts.google.com','fixture-client','https://identity.fixture.invalid/callback')`, nil},
		{`INSERT INTO identity_browser_bindings(binding_hash,created_at,expires_at) VALUES($1,$2,$2::timestamptz+interval '24 hours')`, []any{security.Hash("fixture-binding"), at}},
		{`INSERT INTO identity_transactions(id,binding_hash,binding_generation,state_hash,nonce_hash,descriptor_id,provider,descriptor_revision,policy_revision,marker_key_id,mode,session_mode,created_at,expires_at) VALUES('fixture-attempt',$1,0,$2,$3,'fixture-google','google',1,1,$4,'login','browser',$5,$5::timestamptz+interval '10 minutes')`, []any{security.Hash("fixture-binding"), security.Hash("fixture-state"), security.Hash("fixture-nonce"), strings.Repeat("a", 64), at}},
	}
	for _, s := range statements {
		if _, err := f.store.DB.ExecContext(ctx, s.q, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	return at
}

func TestCurrentIdentitySchemaAuthorityTuplesAndTTL(t *testing.T) {
	f := currentFamilySetup(t)
	at := seedIdentityAttempt(t, f)
	if _, err := f.store.DB.Exec(`INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES('tuple-link','strict-owner','google','https://accounts.google.com','tuple-subject',clock_timestamp())`); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, q, code string
		args          []any
	}{
		{"access family belongs to account", `UPDATE account_sessions SET family_id='strict-other-family' WHERE account_id='strict-owner'`, "23503", nil},
		{"device family belongs to account", `INSERT INTO hosted_devices(id,account_id,name,platform,app_version,session_hash,family_id) VALUES('tuple-device','strict-owner','Fixture','test','1',$1,'strict-other-family')`, "23503", []any{security.Hash("tuple-device")}},
		{"receipt family belongs to account", `INSERT INTO identity_completion_receipts(transaction_id,transaction_expires_at,binding_generation,account_id,family_id,account_epoch,family_generation,ciphertext,created_at,expires_at) SELECT id,expires_at,0,'strict-owner','strict-other-family',0,0,decode(repeat('01',28),'hex'),created_at,created_at+interval '1 minute' FROM identity_transactions`, "23503", nil},
		{"proof family belongs to account", `INSERT INTO account_recent_proofs(proof_hash,account_id,family_id,account_epoch,family_generation,session_mode,captured_access_hash,credential_kind,purpose,intent_digest,created_at,expires_at) VALUES($1,'strict-owner','strict-other-family',0,0,'native',$2,'password','password_set_or_change',$3,$4,$4::timestamptz+interval '1 minute')`, "23503", []any{security.Hash("tuple-proof"), security.Hash("strict-owner-access"), security.Hash("tuple-intent"), at}},
		{"reauth family belongs to account", `UPDATE identity_transactions SET mode='reauthenticate',purpose='password_set_or_change',intent_digest=$1,expected_account_id='strict-owner',expected_family_id='strict-other-family',expected_account_epoch=0,expected_family_generation=0,expected_access_hash=$2,expected_identity_id='tuple-link',expected_identity_revision=1 WHERE id='fixture-attempt'`, "23503", []any{security.Hash("tuple-intent"), security.Hash("strict-owner-access")}},
		{"policy cannot enable without key", `UPDATE identity_protocol_policy SET enabled=true,revision=2`, "23514", nil},
		{"policy revision cannot repeat", `UPDATE identity_protocol_policy SET enabled=false`, "P0001", nil},
		{"policy cannot delete for ABA", `DELETE FROM identity_protocol_policy`, "P0001", nil},
		{"partial reauthentication tuple", `UPDATE identity_transactions SET mode='reauthenticate',expected_account_id='strict-owner' WHERE id='fixture-attempt'`, "23514", nil},
		{"login cannot issue proof", `UPDATE identity_transactions SET status='proof_issued',terminal_at=created_at WHERE id='fixture-attempt'`, "23514", nil},
		{"terminal state needs terminal time", `UPDATE identity_transactions SET status='completed' WHERE id='fixture-attempt'`, "23514", nil},
		{"attempt cannot slide beyond hard expiry", `UPDATE identity_transactions SET expires_at=created_at+interval '11 minutes' WHERE id='fixture-attempt'`, "23514", nil},
		{"claim cannot outlive parent", `INSERT INTO identity_exchange_claims(transaction_id,transaction_expires_at,lease_id,transaction_revision,binding_generation,acquired_at,lease_until) SELECT id,expires_at,'lease-invalid',revision,binding_generation,expires_at-interval '1 second',expires_at+interval '1 second' FROM identity_transactions`, "23514", nil},
		{"claim is bounded thirty seconds", `INSERT INTO identity_exchange_claims(transaction_id,transaction_expires_at,lease_id,transaction_revision,binding_generation,acquired_at,lease_until) SELECT id,expires_at,'lease-long',revision,binding_generation,created_at,created_at+interval '31 seconds' FROM identity_transactions`, "23514", nil},
		{"claim cannot forge parent expiry", `INSERT INTO identity_exchange_claims(transaction_id,transaction_expires_at,lease_id,transaction_revision,binding_generation,acquired_at,lease_until) SELECT id,expires_at+interval '1 second','lease-parent',revision,binding_generation,created_at,created_at+interval '20 seconds' FROM identity_transactions`, "23503", nil},
		{"receipt cipher is bounded", `INSERT INTO identity_completion_receipts(transaction_id,transaction_expires_at,binding_generation,account_id,family_id,account_epoch,family_generation,ciphertext,created_at,expires_at) SELECT id,expires_at,0,'strict-owner','strict-owner-family',0,0,decode(repeat('01',32769),'hex'),created_at,created_at+interval '1 minute' FROM identity_transactions`, "23514", nil},
		{"proof provider tuple is complete", `INSERT INTO account_recent_proofs(proof_hash,account_id,family_id,account_epoch,family_generation,session_mode,captured_access_hash,credential_kind,purpose,intent_digest,created_at,expires_at) VALUES($1,'strict-owner','strict-owner-family',0,0,'native',$2,'provider','password_set_or_change',$3,$4,$4::timestamptz+interval '1 minute')`, "23514", []any{security.Hash("fixture-proof"), security.Hash("strict-owner-access"), security.Hash("fixture-intent"), at}},
		{"marker retention bounded", `INSERT INTO deleted_identity_markers(marker_key_id,subject_digest,deleted_at,expires_at) VALUES($1,$2,$3,$3::timestamptz+interval '16 minutes')`, "23514", []any{strings.Repeat("a", 64), security.Hash("fixture-subject"), at}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ctx, tx := strictTx(t, f.store)
			_, err := tx.ExecContext(ctx, c.q, c.args...)
			requireSQLState(t, err, c.code)
		})
	}
	ctx, tx := strictTx(t, f.store)
	if _, err := tx.ExecContext(ctx, `UPDATE identity_protocol_policy SET revision=revision+1,enabled=true,marker_key_id=$1`, strings.Repeat("b", 64)); err != nil {
		t.Fatal("explicit policy transition rejected", err)
	}
	// The same complete tuple is accepted when each family belongs to the account.
	for _, c := range cases[:5] {
		if _, err := tx.ExecContext(ctx, strings.ReplaceAll(c.q, "strict-other-family", "strict-owner-family"), c.args...); err != nil {
			t.Fatal("coherent family ownership rejected", c.name, err)
		}
	}
	// This validates storage revision mechanics only. No provider consumer or
	// mixed-node key-rotation behavior is claimed by a table test.
}

func TestCurrentIdentitySchemaDeletionOwnershipAndCascades(t *testing.T) {
	f := currentFamilySetup(t)
	at := seedIdentityAttempt(t, f)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := f.store.DB.ExecContext(ctx, `INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES('fixture-link','strict-owner','google','https://accounts.google.com','fixture-subject',$1)`, at); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB.ExecContext(ctx, `UPDATE account_external_identities SET account_id='strict-other',revision=2 WHERE id='fixture-link'`); err == nil {
		t.Fatal("identity ownership transferred")
	} else {
		requireSQLState(t, err, "P0001")
	}
	if _, err := f.store.DB.ExecContext(ctx, `INSERT INTO account_contact_emails(id,account_id,address,verified_at,source,source_identity_id) VALUES('fixture-contact','strict-other','test@example.invalid',$1,'provider','fixture-link')`, at); err == nil {
		t.Fatal("contact crossed identity account")
	} else {
		requireSQLState(t, err, "23503")
	}
	if _, err := f.store.DB.ExecContext(ctx, `INSERT INTO identity_exchange_claims(transaction_id,transaction_expires_at,lease_id,transaction_revision,binding_generation,acquired_at,lease_until) SELECT id,expires_at,'fixture-lease',revision,binding_generation,created_at,created_at+interval '20 seconds' FROM identity_transactions`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.DB.ExecContext(ctx, `INSERT INTO identity_exchange_secrets(transaction_id,transaction_expires_at,sealed_verifier,expires_at) SELECT id,expires_at,decode(repeat('01',28),'hex'),expires_at FROM identity_transactions`); err != nil {
		t.Fatal(err)
	}
	// Explicit binding cancellation/erasure is allowed to remove its own pending
	// material; cleanup itself must later preserve live claims under its contract.
	if _, err := f.store.DB.ExecContext(ctx, `DELETE FROM identity_browser_bindings WHERE binding_hash=$1`, security.Hash("fixture-binding")); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"identity_transactions", "identity_exchange_claims", "identity_exchange_secrets"} {
		var n int
		if err := f.store.DB.QueryRowContext(ctx, `SELECT count(*) FROM `+table).Scan(&n); err != nil || n != 0 {
			t.Fatal("private pending material retained", table, err)
		}
	}
	// Remove unrelated family FKs so the identity erasure fence is the actual
	// remaining reason an unproved account deletion must fail.
	tx, err := f.store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `DELETE FROM account_sessions WHERE account_id='strict-owner'`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM session_families WHERE account_id='strict-owner'`); err != nil {
		t.Fatal(err)
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM accounts WHERE id='strict-owner'`); err == nil {
		t.Fatal("linked subject erased without explicit erasure transition")
	} else {
		requireForeignKeyViolation(t, err)
	}
}

func TestPasswordAbsenceDeniesEveryCurrentPasswordBoundary(t *testing.T) {
	for _, hash := range [][]byte{nil, []byte("malformed")} {
		name := "absent"
		if hash != nil {
			name = "malformed"
		}
		t.Run(name, func(t *testing.T) {
			f := currentFamilySetup(t)
			ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
			defer cancel()
			if _, err := f.store.DB.ExecContext(ctx, `UPDATE accounts SET password_hash=$1 WHERE id='strict-owner'`, hash); err != nil {
				t.Fatal(err)
			}
			svc := identity.New(f.store)
			account := identity.Account{ID: "strict-owner", Username: "strict-owner", DisplayName: "Fixture"}
			if _, err := svc.Login(ctx, account.Username, "CurrentPass9"); !errors.Is(err, identity.ErrCredentials) {
				t.Fatal("password login accepted", err)
			}
			if err := svc.ChangePassword(ctx, account, "CurrentPass9", "ChangedPass8"); !errors.Is(err, identity.ErrCredentials) {
				t.Fatal("password change accepted", err)
			}
			if _, err := svc.PrepareDeletion(ctx, account, "CurrentPass9"); !errors.Is(err, identity.ErrCredentials) {
				t.Fatal("password deletion preview accepted", err)
			}
			next, err := bcrypt.GenerateFromPassword([]byte("ChangedPass8"), bcrypt.DefaultCost)
			if err != nil {
				t.Fatal(err)
			}
			family, _ := identity.NewFamily("native")
			if err = f.store.CreateSession(ctx, account.ID, security.Hash("denied-access"), time.Now().Add(time.Minute), hash, family); !errors.Is(err, identity.ErrCredentials) {
				t.Fatal("repository session accepted absent confirmation", err)
			}
			if err = f.store.ChangePassword(ctx, account.ID, hash, next); !errors.Is(err, identity.ErrConflict) {
				t.Fatal("repository password change accepted", err)
			}
			if _, err = f.store.PrepareDeletion(ctx, account.ID, hash, security.Hash("denied-preview"), time.Now().Add(time.Minute)); !errors.Is(err, identity.ErrDeletionConflict) {
				t.Fatal("repository preview accepted", err)
			}
			if _, err = f.store.DB.ExecContext(ctx, `INSERT INTO account_deletions(id,token_hash,account_id,password_hash,snapshot_hash,expires_at) VALUES('malformed-preview',$1,$2,$3,$4,clock_timestamp()+interval '1 minute')`, security.Hash("crafted-current-preview"), account.ID, hash, security.Hash("fixture-snapshot")); err != nil {
				t.Fatal(err)
			}
			if _, err = f.store.AcceptDeletion(ctx, security.Hash("crafted-current-preview"), ""); !errors.Is(err, identity.ErrDeletionConflict) {
				t.Fatal("pending preview inferred authority from absent credentials", err)
			}
			var epoch int64
			var n int
			if err = f.store.DB.QueryRowContext(ctx, `SELECT auth_epoch FROM accounts WHERE id=$1`, account.ID).Scan(&epoch); err != nil || epoch != 0 {
				t.Fatal("denied request changed account", err)
			}
			if err = f.store.DB.QueryRowContext(ctx, `SELECT count(*) FROM account_sessions WHERE account_id=$1`, account.ID).Scan(&n); err != nil || n != 1 {
				t.Fatal("denied request issued or revoked session", err)
			}
		})
	}
}

func passwordAccountFixture(t *testing.T) (currentFamilyFixture, *identity.Service, identity.Account) {
	t.Helper()
	f := currentFamilySetup(t)
	hash, err := bcrypt.GenerateFromPassword([]byte("CurrentPass9"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.DB.Exec(`UPDATE accounts SET password_hash=$1`, hash); err != nil {
		t.Fatal(err)
	}
	return f, identity.New(f.store), identity.Account{ID: "strict-owner", Username: "strict-owner", DisplayName: "Fixture"}
}

type deletionOutcome struct {
	receipt identity.DeletionReceipt
	err     error
}

func TestCurrentPasswordDeletionAccountFirstAndExactReceiptReplay(t *testing.T) {
	f, svc, account := passwordAccountFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	preview, err := svc.PrepareDeletion(ctx, account, "CurrentPass9")
	if err != nil {
		t.Fatal(err)
	}
	lockCtx, blocker := strictTx(t, f.store)
	if _, err = blocker.ExecContext(lockCtx, `SELECT id FROM accounts WHERE id=$1 FOR UPDATE`, account.ID); err != nil {
		t.Fatal(err)
	}
	pid := strictPID(t, lockCtx, blocker)
	out := make(chan deletionOutcome, 1)
	go func() { r, e := svc.AcceptDeletion(ctx, preview.Token, ""); out <- deletionOutcome{r, e} }()
	observedCtx, observedCancel := context.WithTimeout(ctx, 2*time.Second)
	defer observedCancel()
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		var blocked bool
		if err = f.store.DB.QueryRowContext(observedCtx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, pid).Scan(&blocked); err != nil {
			t.Fatal("accept lock observation", err)
		}
		if blocked {
			break
		}
		select {
		case <-observedCtx.Done():
			t.Fatal("accept did not wait on account")
		case <-tick.C:
		}
	}
	// While acceptance demonstrably waits for the account, its preview must
	// remain lockable. Preview-first acceptance would block this exact query.
	probeCtx, probeCancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer probeCancel()
	probe, err := f.store.DB.BeginTx(probeCtx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Rollback()
	if _, err = probe.ExecContext(probeCtx, `SELECT id FROM account_deletions WHERE token_hash=$1 FOR UPDATE`, security.Hash(preview.Token)); err != nil {
		t.Fatal("accept took preview before account", err)
	}
	if err = probe.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err = blocker.Commit(); err != nil {
		t.Fatal(err)
	}
	var committed identity.DeletionReceipt
	select {
	case result := <-out:
		if result.err != nil {
			t.Fatal(result.err)
		}
		committed = result.receipt
	case <-time.After(3 * time.Second):
		t.Fatal("accept did not settle")
	}
	if committed.Status != "complete" {
		t.Fatal("deletion did not commit")
	}
	// Account absence is now legitimate only because this exact receipt already
	// committed. Replays must not recreate sessions or infer a new authorization.
	repeated := make(chan deletionOutcome, 2)
	for range 2 {
		go func() { r, e := svc.AcceptDeletion(ctx, preview.Token, ""); repeated <- deletionOutcome{r, e} }()
	}
	for range 2 {
		select {
		case r := <-repeated:
			if r.err != nil || r.receipt.ID != committed.ID {
				t.Fatal("exact committed receipt did not replay", r.err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("receipt replay blocked")
		}
	}
	if _, err = svc.AcceptDeletion(ctx, preview.Token, identity.DeletePersonalResources); !errors.Is(err, identity.ErrDeletionConflict) {
		t.Fatal("different disposition replayed", err)
	}
	other := identity.Account{ID: "strict-other", Username: "strict-other", DisplayName: "Fixture"}
	pending, err := svc.PrepareDeletion(ctx, other, "CurrentPass9")
	if err != nil {
		t.Fatal(err)
	}
	erase, err := f.store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer erase.Rollback()
	for _, q := range []string{`DELETE FROM account_sessions WHERE account_id='strict-other'`, `DELETE FROM session_families WHERE account_id='strict-other'`, `DELETE FROM accounts WHERE id='strict-other'`} {
		if _, err = erase.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	if err = erase.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.AcceptDeletion(ctx, pending.Token, ""); !errors.Is(err, identity.ErrCredentials) {
		t.Fatal("missing account treated as uncommitted deletion authority", err)
	}
}

func TestCurrentPasswordDeletionFailureRollsBackAuthority(t *testing.T) {
	f, svc, account := passwordAccountFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := f.store.DB.ExecContext(ctx, `INSERT INTO account_external_identities(id,account_id,provider,issuer,subject,verified_at) VALUES('erasure-fence',$1,'apple','https://appleid.apple.com','fixture-subject',clock_timestamp())`, account.ID); err != nil {
		t.Fatal(err)
	}
	p, err := svc.PrepareDeletion(ctx, account, "CurrentPass9")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.AcceptDeletion(ctx, p.Token, "")
	requireForeignKeyViolation(t, err)
	var sessions int
	var accepted sql.NullTime
	if err = f.store.DB.QueryRowContext(ctx, `SELECT count(*) FROM account_sessions WHERE account_id=$1`, account.ID).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatal("failed deletion lost authority", err)
	}
	if err = f.store.DB.QueryRowContext(ctx, `SELECT accepted_at FROM account_deletions WHERE token_hash=$1`, security.Hash(p.Token)).Scan(&accepted); err != nil || accepted.Valid {
		t.Fatal("failed deletion committed receipt", err)
	}
	// Remove only the deliberately injected erasure-fence fixture, then prove
	// rollback left the current preview and account usable; no provider is called.
	if _, err = f.store.DB.ExecContext(ctx, `DELETE FROM account_external_identities WHERE id='erasure-fence'`); err != nil {
		t.Fatal(err)
	}
	if r, e := svc.AcceptDeletion(ctx, p.Token, ""); e != nil || r.Status != "complete" {
		t.Fatal("retry after rolled-back transaction failed", e)
	}
}

func TestCurrentIdentitySchemaInitializationIsAtomic(t *testing.T) {
	f := currentFamilySetup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	schema := "testx" + hex.EncodeToString(security.Hash(security.RandomID("schema")))[:15]
	initialize := func() *sql.Tx {
		t.Helper()
		tx, err := f.store.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = tx.Rollback() })
		if _, err = tx.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.ExecContext(ctx, `SET LOCAL search_path TO `+schema); err != nil {
			t.Fatal(err)
		}
		if err = postgres.InitializeCurrentHostedSchemaTx(ctx, tx); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	first := initialize()
	if _, err := first.ExecContext(ctx, `SELECT 1/0`); err == nil {
		t.Fatal("injected transaction failure absent")
	}
	if err := first.Rollback(); err != nil {
		t.Fatal(err)
	}
	var exists bool
	if err := f.store.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)`, schema).Scan(&exists); err != nil || exists {
		t.Fatal("partial current schema survived rollback", err)
	}
	// A function-only namespace is occupied too; it cannot be silently adopted.
	occupied, err := f.store.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Rollback()
	if _, err = occupied.ExecContext(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema+`; CREATE FUNCTION unexpected() RETURNS integer LANGUAGE SQL AS 'SELECT 1'`); err != nil {
		t.Fatal(err)
	}
	if err = postgres.InitializeCurrentHostedSchemaTx(ctx, occupied); err == nil {
		t.Fatal("function-only namespace was adopted")
	}
	if err = occupied.Rollback(); err != nil {
		t.Fatal(err)
	}
	second := initialize()
	if err := second.Commit(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 5*time.Second)
		defer done()
		if _, err := f.store.DB.ExecContext(cleanupCtx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error("owned schema cleanup failed", err)
		}
	})
	var version int
	if err := f.store.DB.QueryRowContext(ctx, `SELECT version FROM `+schema+`.hosted_schema_contract WHERE singleton`).Scan(&version); err != nil || version != postgres.CurrentHostedSchemaVersion {
		t.Fatal("current schema marker absent", version, err)
	}
	var count int
	if err := f.store.DB.QueryRowContext(ctx, `SELECT count(*) FROM `+schema+`.hosted_schema_migrations`).Scan(&count); err != nil || count != postgres.CurrentHostedSchemaVersion {
		t.Fatal("numbered migration history incomplete", count, err)
	}
}
