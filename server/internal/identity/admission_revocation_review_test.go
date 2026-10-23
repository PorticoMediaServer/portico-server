package identity

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestRevokedSignerCannotFinishAPendingTelevisionApproval(t *testing.T) {
	ctx := context.Background()
	s, _, _ := directFixture(t)
	linkedMember(t, s, "linked", "acc_linked")
	in, err := s.PorticoSignIn(ctx, PorticoIdentity{AccountID: "acc_linked", SignerKeyID: "key-a"})
	if err != nil {
		t.Fatal(err)
	}
	approver, err := s.Authenticate(in.Session.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	tv := onDevice(t, memberTV, "Living room")
	code, err := s.StartQuick(tv, QuickStart{RequestID: "00000000-0000-4000-8000-000000000091", DeviceName: "Living room", Platform: "tvos", AppVersion: "1.0"}, true)
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
	revokeKey(t, s, "key-a")
	if _, err = s.db.Exec(`UPDATE quick_connect_v1 SET next_poll=0`); err != nil {
		t.Fatal(err)
	}
	if out, err := s.PollQuick(tv, code.DeviceCode); err == nil {
		t.Fatalf("a revoked signer issued a television session: %s", out.SessionFamilyID)
	}
	var live int
	if err = s.db.QueryRow(`SELECT count(*) FROM authorization_session_families WHERE account_id='linked' AND revoked=0`).Scan(&live); err != nil || live != 0 {
		t.Fatalf("live sessions after refusal: %d %v", live, err)
	}
}

func TestRevokedSignerCannotFinishAPendingSecondFactor(t *testing.T) {
	ctx := context.Background()
	s, _, owner := directFixture(t)
	enrol, err := s.EnrolTwoFactor(ctx, owner.AccountToken, "Testing1!")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.VerifyTwoFactor(ctx, owner.AccountToken, "Testing1!", currentTOTP(t, enrol.Secret)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`INSERT INTO account_portico_links(account_id,hosted_account_id) VALUES(?,'acc_owner')`, owner.Account.ID); err != nil {
		t.Fatal(err)
	}
	pending, err := s.PorticoSignIn(ctx, PorticoIdentity{AccountID: "acc_owner", SignerKeyID: "key-a"})
	if err != nil || pending.Challenge == nil {
		t.Fatalf("challenge: %+v %v", pending, err)
	}
	revokeKey(t, s, "key-a")
	if out, err := s.CompleteSignInChallenge(ctx, pending.Challenge.Token, enrol.RecoveryCodes[0], true); err == nil {
		t.Fatalf("a revoked signer completed second factor: %+v", out.Session.Viewer)
	}
	var used int
	if err = s.db.QueryRow(`SELECT count(*) FROM identity_recovery_codes WHERE account_id=? AND used=1`, owner.Account.ID).Scan(&used); err != nil || used != 0 {
		t.Fatalf("refusal spent recovery code: %d %v", used, err)
	}
}

func TestPorticoAdmissionRechecksSignerAfterAssertionVerification(t *testing.T) {
	s, _, _ := directFixture(t)
	linkedMember(t, s, "linked", "acc_linked")
	revokeKey(t, s, "key-a")
	if _, err := s.PorticoSignIn(context.Background(), PorticoIdentity{AccountID: "acc_linked", SignerKeyID: "key-a"}); err == nil {
		t.Fatal("an assertion verified before revocation admitted after revocation")
	}
	if out, err := s.PorticoSignIn(context.Background(), PorticoIdentity{AccountID: "acc_linked", SignerKeyID: "key-b"}); err != nil || out.Session == nil {
		t.Fatalf("unrelated signer refused: %v", err)
	}
}

func TestNewDeviceStillPassesSignInAdmission(t *testing.T) {
	s, _, _, _ := memberWithSession(t)
	remote := WithSignInAdmission(onDevice(t, memberPhone, "Phone"), SignInAdmission{Remote: true, Policy: RemoteSignInOwnerOnly})
	if _, err := s.DirectLogin(remote, "member", "Testing2!"); !errors.Is(err, ErrRemoteSignIn) {
		t.Fatalf("outside member awaiting approval bypassed admission: %v", err)
	}
	scheduleErr := errors.New("outside allowed hours")
	closed := WithAccountAdmitter(onDevice(t, memberPhone, "Phone"), func(context.Context, *sql.Tx, string) error { return scheduleErr })
	if _, err := s.DirectLogin(closed, "member", "Testing2!"); !errors.Is(err, scheduleErr) {
		t.Fatalf("pending device bypassed schedule: %v", err)
	}
	var devices int
	if err := s.db.QueryRow(`SELECT count(*) FROM identity_devices WHERE installation_id=?`, memberPhone).Scan(&devices); err != nil || devices != 0 {
		t.Fatalf("refused sign-in registered %d devices: %v", devices, err)
	}
}

func TestRemoteOwnerOnlyRegistrationCannotCommitAMember(t *testing.T) {
	s, _, _ := directFixture(t)
	if err := s.SetSelfRegistration(SelfRegistrationOpen); err != nil {
		t.Fatal(err)
	}
	ctx := WithSignInAdmission(onDevice(t, memberPhone, "Phone"), SignInAdmission{Remote: true, Policy: RemoteSignInOwnerOnly})
	if _, err := s.Register(ctx, SelfRegistrationRequest{Username: "outside", Password: "Testing2!"}, false); !errors.Is(err, ErrRemoteSignIn) {
		t.Fatalf("outside registration: %v", err)
	}
	var accounts int
	if err := s.db.QueryRow(`SELECT count(*) FROM accounts WHERE username='outside'`).Scan(&accounts); err != nil || accounts != 0 {
		t.Fatalf("refused registration left %d accounts: %v", accounts, err)
	}
}

func TestMemberDeviceDecisionRechecksLiveAuthority(t *testing.T) {
	for _, change := range []string{"logout", "transfer", "demote"} {
		t.Run(change, func(t *testing.T) {
			ctx := context.Background()
			s, owner, member, login := memberWithSession(t)
			actorSession := *owner.Session
			adminID := ""
			if change == "demote" {
				admin, err := s.CreateDirectMember(ctx, owner.AccountToken, DirectMemberCreate{Username: "admin", Password: "Testing3!", Name: "Admin", AllowedLibraries: []string{}})
				if err != nil {
					t.Fatal(err)
				}
				adminID = admin.ID
				if _, err = s.db.Exec(`UPDATE direct_memberships SET role='admin' WHERE account_id=?`, adminID); err != nil {
					t.Fatal(err)
				}
				actorSession, err = s.Issue(admin.ID, admin.PrimaryProfileID, "local", TierAdmin, 1)
				if err != nil {
					t.Fatal(err)
				}
			}
			actor, err := s.Authenticate(actorSession.AccessToken)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "logout":
				err = s.Logout(actor)
			case "transfer":
				err = s.TransferDirectOwnership(ctx, owner.AccountToken, member.ID, "Testing1!", owner.Account.Revision)
			case "demote":
				if _, err = s.db.Exec(`UPDATE direct_memberships SET role='member' WHERE account_id=?`, adminID); err == nil {
					err = func() error {
						tx, e := s.db.Begin()
						if e != nil {
							return e
						}
						defer tx.Rollback()
						if e = RetierFamiliesTx(ctx, tx, adminID, TierMember); e != nil {
							return e
						}
						return tx.Commit()
					}()
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.MemberDevices(ctx, actor, MemberDeviceQuery{}); err == nil {
				t.Error("stale administrator listed devices")
			} else if change != "logout" && !errors.Is(err, ErrForbidden) {
				t.Fatalf("live demotion ended the sign-in: %v", err)
			}
			if err = s.SignOutMemberDevices(ctx, actor, member.ID, login.DeviceID); err == nil {
				t.Error("stale administrator blocked a device")
			} else if change != "logout" && !errors.Is(err, ErrForbidden) {
				t.Fatalf("live demotion ended the sign-in: %v", err)
			}
			if _, err = s.Authenticate(login.Session.AccessToken); err != nil {
				t.Fatalf("refused sign-out changed target: %v", err)
			}
		})
	}
}
