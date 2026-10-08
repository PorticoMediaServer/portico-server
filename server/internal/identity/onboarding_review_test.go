package identity

import (
	"context"
	"errors"
	"golang.org/x/crypto/bcrypt"
	"testing"
)

func TestW2I02RecoveryLoginPolicyAndIssuance(t *testing.T) {
	s, db, _ := familyFixture(t)
	password := "correct recovery password"
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`UPDATE accounts SET password_hash=? WHERE id='account'`, hash); e != nil {
		t.Fatal(e)
	}
	if _, e = db.Exec(`INSERT INTO onboarding_state_v1(singleton,auth_mode,operation_id,request_hash,setup_hash,receipt_until,recovery_account,recovery_confirmed,remote_recovery,ready) VALUES(1,'hosted','review','hash','hash',0,'account',1,0,1)`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.LoginFrom(context.Background(), "OWNER", password, false); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("private recovery account issued remote family", e)
	}
	count := func() int {
		var n int
		if e := db.QueryRow(`SELECT count(*) FROM authorization_session_families`).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	if count() != 0 {
		t.Fatal("denied login minted a family")
	}
	if _, e = s.LoginFrom(context.Background(), "owner", password, true); e != nil {
		t.Fatal("private recovery unavailable", e)
	}
	if _, e = db.Exec(`UPDATE onboarding_state_v1 SET remote_recovery=1,revision=revision+1`); e != nil {
		t.Fatal(e)
	}
	if _, e = s.LoginFrom(context.Background(), "owner", password, false); e != nil {
		t.Fatal("explicitly enabled recovery rejected", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	before := count()
	if _, e = s.LoginFrom(ctx, "owner", password, false); !errors.Is(e, context.Canceled) || count() != before {
		t.Fatal("cancelled request issued a family", e)
	}
}
