package identity

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func linkedMember(t *testing.T, s *Service, local, hosted string) {
	t.Helper()
	if _, err := s.db.Exec(`INSERT INTO accounts(id,username,password_hash,profile_id) VALUES(?,?,x'',?)`, local, local, local+"-profile"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO account_portico_links(account_id,hosted_account_id) VALUES(?,?)`, local, hosted); err != nil {
		t.Fatal(err)
	}
}

func revokeKey(t *testing.T, s *Service, key string) int {
	t.Helper()
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`INSERT OR IGNORE INTO hosted_signing_revocations(key_id) VALUES(?)`, key); err != nil {
		t.Fatal(err)
	}
	n, err := s.RevokeSessionsAdmittedByRevokedKeysTx(context.Background(), tx)
	if err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return n
}

// A television signed in by a code that a Portico-admitted session approved
// is admitted by the same signing key, and ends with it.
func TestATelevisionApprovedByAPorticoSessionEndsWithItsSigningKey(t *testing.T) {
	ctx := context.Background()
	s, _, _ := directFixture(t)
	linkedMember(t, s, "linked", "acc_linked")
	in, err := s.PorticoSignIn(ctx, PorticoIdentity{AccountID: "acc_linked", SignerKeyID: "key-a"})
	if err != nil || in.Session == nil {
		t.Fatalf("%+v %v", in, err)
	}
	approver, err := s.Authenticate(in.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	tv := onDevice(t, memberTV, "Living room")
	code, err := s.StartQuick(tv, QuickStart{RequestID: "00000000-0000-4000-8000-000000000009", DeviceName: "Living room", Platform: "tvos", AppVersion: "1.0"}, true)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.ReviewQuick(ctx, code.UserCode)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.DecideQuick(ctx, approver, preview.RequestID, code.UserCode, "approve"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE quick_connect_v1 SET next_poll=0`); err != nil {
		t.Fatal(err)
	}
	television, err := s.PollQuick(tv, code.DeviceCode)
	if err != nil {
		t.Fatal(err)
	}
	var signer string
	if err = s.db.QueryRow(`SELECT signer_key_id FROM portico_admitted_families WHERE family_id=?`, television.SessionFamilyID).Scan(&signer); err != nil || signer != "key-a" {
		t.Fatalf("the television's session records signing key %q: %v", signer, err)
	}
	// Revoking an unrelated key ends nothing.
	if n := revokeKey(t, s, "key-unrelated"); n != 0 {
		t.Fatalf("revoking an unrelated key ended %d sessions", n)
	}
	if _, err = s.Authenticate(television.AccessToken); err != nil {
		t.Fatal(err)
	}
	if n := revokeKey(t, s, "key-a"); n != 2 {
		t.Fatalf("revoking the key ended %d sessions, want the phone's and the television's", n)
	}
	for name, token := range map[string]string{"the phone": in.Session.AccessToken, "the television": television.AccessToken} {
		if _, err = s.Authenticate(token); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("%s is still signed in after its signing key was revoked: %v", name, err)
		}
	}
	// A second revocation pass finds nothing left to end.
	if n := revokeKey(t, s, "key-a"); n != 0 {
		t.Fatalf("a repeated pass ended %d sessions", n)
	}
	// A session whose signing key was never recorded is not swept up by an
	// empty key id.
	plain, err := s.PorticoSignIn(ctx, PorticoIdentity{AccountID: "acc_linked"})
	if err != nil || plain.Session == nil {
		t.Fatal(err)
	}
	if n := revokeKey(t, s, ""); n != 0 {
		t.Fatalf("an empty key id ended %d sessions", n)
	}
}

// "Does this server sign people in with Portico Accounts" is whether its claim
// is installed, and nothing else.
func TestHostedAttachmentIsWhetherTheClaimIsInstalled(t *testing.T) {
	s, _, _ := directFixture(t)
	if s.Hosted() {
		t.Fatal("a server with no claim reports Portico Account sign-in")
	}
	installed := false
	s.SetupClaimReadyTx = func(_ context.Context, tx *sql.Tx) (bool, error) {
		if tx == nil {
			t.Fatal("the claim was read outside a transaction")
		}
		return installed, nil
	}
	if s.Hosted() {
		t.Fatal("a server whose claim is not installed reports Portico Account sign-in")
	}
	installed = true
	if !s.Hosted() {
		t.Fatal("a claimed server does not report Portico Account sign-in")
	}
	capabilities, err := s.Capabilities(context.Background())
	if err != nil || !capabilities.HostedAttach {
		t.Fatalf("capabilities: %+v %v", capabilities, err)
	}
	s.SetupClaimReadyTx = func(context.Context, *sql.Tx) (bool, error) { return true, errors.New("unreadable") }
	if s.Hosted() {
		t.Fatal("an unreadable claim was reported as installed")
	}
}
