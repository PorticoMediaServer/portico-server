package identity

import (
	"context"
	"errors"
	"testing"
)

func TestWave2RecoverySelectionRechecksActualPeerAndPolicy(t *testing.T) {
	s, db, login := directFixture(t)
	ctx := context.Background()
	if _, err := db.Exec(`INSERT INTO onboarding_state_v1(singleton,auth_mode,operation_id,request_hash,setup_hash,receipt_until,recovery_account,recovery_confirmed,remote_recovery,ready) VALUES(1,'hosted','wave2','hash','hash',0,'account',1,1,1)`); err != nil {
		t.Fatal(err)
	}
	remote, err := s.DirectLoginFrom(ctx, "owner", "Testing1!", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE onboarding_state_v1 SET remote_recovery=0,revision=revision+1`); err != nil {
		t.Fatal(err)
	}
	var before, after int
	if err = db.QueryRow(`SELECT count(*) FROM authorization_session_families`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SelectDirectProfileFrom(ctx, remote.AccountToken, login.Account.PrimaryProfileID, DirectSelection{}, false); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("remote recovery selection admitted after policy changed", err)
	}
	if _, err = s.DirectLoginFrom(ctx, "owner", "Testing1!", false); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("remote recovery login admitted", err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM authorization_session_families`).Scan(&after); err != nil || before != after {
		t.Fatal("denial minted viewing family", before, after, err)
	}
	if _, err = s.SelectDirectProfileFrom(ctx, remote.AccountToken, login.Account.PrimaryProfileID, DirectSelection{}, true); err != nil {
		t.Fatal("private recovery selection rejected", err)
	}
}
