package postgres_test

import (
	"bytes"
	"context"
	"encoding/base32"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

// These are isolated PostgreSQL fixtures, not provider or email delivery tests.
func accountSecurityFixture(t *testing.T) *postgres.AccountSecurity {
	t.Helper()
	store := database(t)
	return &postgres.AccountSecurity{Store: store, Keys: &security.Keys{SealKey: bytes.Repeat([]byte{11}, 32)}, Identity: identity.New(store), Lifetime: 15 * time.Minute, MailEnabled: true}
}
func mailPayload(t *testing.T, s *postgres.AccountSecurity, kind mailoutbox.Kind, resource string) mailoutbox.Payload {
	t.Helper()
	var id string
	var ciphertext []byte
	if err := s.Store.DB.QueryRow(`SELECT id,ciphertext FROM account_mail_outbox WHERE kind=$1 AND resource_id=$2 ORDER BY created_at DESC LIMIT 1`, kind, resource).Scan(&id, &ciphertext); err != nil {
		t.Fatal(err)
	}
	raw, err := s.Keys.Open(ciphertext, "mail:"+id)
	if err != nil {
		t.Fatal(err)
	}
	var p mailoutbox.Payload
	if err = json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}
func verifiedAccount(t *testing.T, s *postgres.AccountSecurity, username string) identity.Session {
	t.Helper()
	ctx := context.Background()
	binding := security.RandomID("")
	r, err := s.CreateRegistration(ctx, binding, postgres.RegistrationInput{Username: username, DisplayName: "Fixture", Password: "StrongPass9", ContactAddress: username + "@example.test", SessionMode: "native", RequestID: security.RandomID("")})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.VerifyRegistration(ctx, r.ID, binding, mailPayload(t, s, mailoutbox.Registration, r.ID).Code, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return a
}
func securityAction(t *testing.T, s *postgres.AccountSecurity, a identity.Session, purpose, target string) identity.SecurityResult {
	t.Helper()
	ctx := context.Background()
	i := identity.SecurityIntent{Purpose: purpose, Target: target, OperationID: security.RandomID("")}
	proof, err := s.PasswordProof(ctx, a.AccessToken, i, "StrongPass9", "")
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Mutate(ctx, a.AccessToken, postgres.SecurityAction{SecurityIntent: i, Proof: proof.Proof})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func TestVerifiedRegistrationAtomicRetryAndMailEligibility(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	binding := security.RandomID("")
	q := postgres.RegistrationInput{Username: "pending", DisplayName: "Pending", Password: "StrongPass9", ContactAddress: "pending@example.test", SessionMode: "native", RequestID: security.RandomID("")}
	r, err := s.CreateRegistration(ctx, binding, q)
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.CreateRegistration(ctx, binding, q)
	if err != nil || again.ID != r.ID {
		t.Fatalf("registration retry: %v", err)
	}
	var count int
	if err = s.Store.DB.QueryRow(`SELECT count(*) FROM accounts WHERE username='pending'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("unverified account created: %v", err)
	}
	mail := &postgres.AccountMail{Store: s.Store, Keys: s.Keys}
	delivery, err := mail.ClaimMail(ctx)
	if err != nil || delivery == nil {
		t.Fatalf("durable mail missing: %v", err)
	}
	if ok, e := mail.MailEligible(ctx, *delivery); e != nil || !ok {
		t.Fatalf("pending verification ineligible: %v", e)
	}
	if _, err = s.VerifyRegistration(ctx, r.ID, security.RandomID(""), delivery.Payload.Code, "", ""); err == nil {
		t.Fatal("different device binding accepted")
	}
	a, err := s.VerifyRegistration(ctx, r.ID, binding, delivery.Payload.Code, "", "")
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.VerifyRegistration(ctx, r.ID, binding, delivery.Payload.Code, "", "")
	if err != nil || a.AccessToken != b.AccessToken || a.FamilyID != b.FamilyID {
		t.Fatalf("session retry changed family: %v", err)
	}
	if ok, e := mail.MailEligible(ctx, *delivery); e != nil || ok {
		t.Fatalf("consumed verification still deliverable: %v", e)
	}
	q.Password = "DifferentPass8"
	if _, err = s.CreateRegistration(ctx, binding, q); !errors.Is(err, identity.ErrConflict) {
		t.Fatalf("changed request reused: %v", err)
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(a.AccessToken)); err != nil {
		t.Fatal(err)
	}
}
func TestMFAEnrollmentRevocationAndSingleUseRecovery(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "mfaowner")
	enrollment := securityAction(t, s, a, "mfa_enroll", "")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err = s.Store.DB.QueryRow(`SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	code := identity.HOTP(key, uint64(now.Unix()/30), 6)
	op := security.RandomID("")
	enabled, err := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, code, op)
	if err != nil || len(enabled.RecoveryCodes) != 10 || !enabled.SignOut {
		t.Fatalf("enrollment: %v", err)
	}
	retry, err := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, code, op)
	if err != nil || retry.RecoveryCodes[0] != enabled.RecoveryCodes[0] {
		t.Fatalf("lost enrollment response: %v", err)
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(a.AccessToken)); err == nil {
		t.Fatal("pre-MFA session survived")
	}
	login, err := s.PasswordLoginBound(ctx, "mfaowner", "StrongPass9", "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
	if err != nil || login.Challenge == nil || login.Session != nil {
		t.Fatalf("MFA bypass: %v", err)
	}
	c := login.Challenge
	signed, err := s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, enabled.RecoveryCodes[0])
	if err != nil {
		t.Fatal(err)
	}
	replay, err := s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, enabled.RecoveryCodes[0])
	if err != nil || replay.AccessToken != signed.AccessToken {
		t.Fatalf("MFA receipt retry: %v", err)
	}
	other, err := s.PasswordLoginBound(ctx, "mfaowner", "StrongPass9", "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
	if err != nil || other.Challenge == nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteMFA(ctx, other.Challenge.ChallengeID, other.Challenge.ChallengeToken, enabled.RecoveryCodes[0]); !errors.Is(err, identity.ErrChallenge) {
		t.Fatalf("recovery code reused for different challenge: %v", err)
	}
	if _, err = s.CompleteMFA(ctx, other.Challenge.ChallengeID, other.Challenge.ChallengeToken, enabled.RecoveryCodes[1]); err != nil {
		t.Fatal(err)
	}
}
func TestContactChangeAndRecoveryStayAccountBound(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "emailowner")
	other := verifiedAccount(t, s, "otherowner")
	change := securityAction(t, s, a, "email_change", "new@example.test")
	code := mailPayload(t, s, mailoutbox.EmailChange, change.ChallengeID).Code
	op := security.RandomID("")
	state, err := s.State(ctx, a.AccessToken)
	if err != nil || state.ContactAddress != "emailowner@example.test" {
		t.Fatalf("unverified address replaced contact: %v", err)
	}
	if _, err = s.ConfirmEmailChange(ctx, other.AccessToken, change.ChallengeID, code, op); err == nil {
		t.Fatal("cross-account email proof accepted")
	}
	if _, err = s.ConfirmEmailChange(ctx, a.AccessToken, change.ChallengeID, code, op); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(a.AccessToken)); err == nil {
		t.Fatal("contact change did not revoke old sessions")
	}
	login, err := s.PasswordLoginBound(ctx, "emailowner", "StrongPass9", "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
	if err != nil || login.Session == nil {
		t.Fatal(err)
	}
	if err = s.RequestRecovery(ctx, postgres.RecoveryRequest{Username: "unknown", ContactAddress: "unknown@example.test", RequestID: security.RandomID("")}); err != nil {
		t.Fatalf("unknown recovery outcome differs: %v", err)
	}
	q := postgres.RecoveryRequest{Username: "emailowner", ContactAddress: "new@example.test", RequestID: security.RandomID("")}
	if err = s.RequestRecovery(ctx, q); err != nil {
		t.Fatal(err)
	}
	if err = s.RequestRecovery(ctx, q); err != nil {
		t.Fatal(err)
	}
	var id string
	var count int
	if err = s.Store.DB.QueryRow(`SELECT min(id),count(*) FROM account_email_challenges WHERE purpose='password_reset' AND account_id=$1`, a.Account.ID).Scan(&id, &count); err != nil || count != 1 {
		t.Fatalf("duplicate recovery request: %v", err)
	}
	token := mailPayload(t, s, mailoutbox.PasswordReset, id).Code
	if err = s.ResetPassword(ctx, token, "NewStrongPass8", ""); err != nil {
		t.Fatal(err)
	}
	if err = s.ResetPassword(ctx, token, "NewStrongPass8", ""); err != nil {
		t.Fatalf("identical reset retry: %v", err)
	}
	if err = s.ResetPassword(ctx, token, "AnotherStrong9", ""); !errors.Is(err, identity.ErrChallenge) {
		t.Fatalf("changed reset replay accepted: %v", err)
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(login.Session.AccessToken)); err == nil {
		t.Fatal("password reset left previous session usable")
	}
	if _, err = s.Store.Authenticate(ctx, security.Hash(other.AccessToken)); err != nil {
		t.Fatal("unrelated account was revoked")
	}
}

// passwordOutcomes signs in as user with a wrong password n times from network
// and records each answer, with a cool-off's Retry-After in whole seconds.
func passwordOutcomes(t *testing.T, s *postgres.AccountSecurity, user, network string, n int) []string {
	t.Helper()
	var out []string
	for range n {
		out = append(out, passwordOutcome(s, user, "WrongPass9", network))
	}
	return out
}
func passwordOutcome(s *postgres.AccountSecurity, user, password, network string) string {
	r, err := s.PasswordLoginBound(context.Background(), user, password, "browser", security.RandomID(""), security.RandomID(""), network)
	var cool identity.CoolOff
	switch {
	case err == nil && (r.Session != nil || r.Challenge != nil):
		return "signed-in"
	case errors.As(err, &cool) && errors.Is(err, identity.ErrRateLimited):
		return fmt.Sprintf("cool-off:%.0fs", time.Until(cool.Until).Round(5*time.Second).Seconds())
	case errors.Is(err, identity.ErrCredentials):
		return "credentials"
	default:
		return fmt.Sprint(err)
	}
}
func unknownPasswordRows(t *testing.T, s *postgres.AccountSecurity) int {
	t.Helper()
	var rows int
	if err := s.Store.DB.QueryRow(`SELECT count(*) FROM account_security_limits WHERE kind IN($1,$2,$3)`, postgres.UnknownPasswordNetwork, postgres.UnknownPasswordLock, postgres.UnknownPasswordWide).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

// HOST03: an unknown username answers exactly like an existing one under a
// wrong password: the same invalid-credentials results, then the same cool-off
// with the same Retry-After, from the same network. Its state is a fixed bucket
// row, never the name, and a malformed name gets no state at all.
func TestPasswordCoolOffDoesNotRevealUsernames(t *testing.T) {
	s := accountSecurityFixture(t)
	_ = verifiedAccount(t, s, "realname")
	known, unknown := passwordOutcomes(t, s, "realname", "203.0.113.9", 8), passwordOutcomes(t, s, "ghostname", "203.0.113.9", 8)
	if fmt.Sprint(known) != fmt.Sprint(unknown) {
		t.Fatalf("existing %v, unknown %v", known, unknown)
	}
	if fmt.Sprint(known[4:6]) != "[credentials cool-off:30s]" {
		t.Fatalf("unexpected schedule: %v", known)
	}
	var rows int
	if err := s.Store.DB.QueryRow(`SELECT count(*) FROM account_password_backoff`).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("unknown name stored as an account backoff: %d rows, %v", rows, err)
	}
	if rows = unknownPasswordRows(t, s); rows != 3 {
		t.Fatalf("unknown name used %d bucket rows, want 3", rows)
	}
	var named int
	if err := s.Store.DB.QueryRow(`SELECT count(*) FROM account_security_limits WHERE position(convert_to('ghostname','UTF8') in subject_hash)>0 OR kind LIKE '%ghost%'`).Scan(&named); err != nil || named != 0 {
		t.Fatal("the attacker's name was stored", named, err)
	}
	for _, malformed := range []string{"x", "Has Space!", strings.Repeat("a", 40)} {
		if out := passwordOutcomes(t, s, malformed, "203.0.113.9", 7); fmt.Sprint(out) != fmt.Sprint([]string{"credentials", "credentials", "credentials", "credentials", "credentials", "credentials", "credentials"}) {
			t.Fatalf("malformed %q: %v", malformed, out)
		}
	}
	if rows = unknownPasswordRows(t, s); rows != 3 {
		t.Fatalf("a malformed name was given state: %d rows", rows)
	}
	// The correct password still signs in from a network that hasn't failed.
	if got := passwordOutcome(s, "realname", "StrongPass9", "198.51.100.7"); got != "signed-in" {
		t.Fatal(got)
	}
}

// HOST03: the unknown-username count is durable and shared. A restarted
// process or another replica (a new AccountSecurity on the same database and
// seal key) continues it rather than starting over.
func TestUnknownPasswordCoolOffSurvivesRestartAndReplicas(t *testing.T) {
	s := accountSecurityFixture(t)
	replica := func() *postgres.AccountSecurity {
		return &postgres.AccountSecurity{Store: s.Store, Keys: &security.Keys{SealKey: bytes.Repeat([]byte{11}, 32)}, Identity: identity.New(s.Store), Lifetime: s.Lifetime, MailEnabled: true}
	}
	got := append(passwordOutcomes(t, s, "ghostname", "203.0.113.9", 3), passwordOutcomes(t, replica(), "ghostname", "203.0.113.9", 2)...)
	got = append(got, passwordOutcome(replica(), "ghostname", "WrongPass9", "203.0.113.9"))
	if fmt.Sprint(got) != "[credentials credentials credentials credentials credentials cool-off:30s]" {
		t.Fatalf("count did not carry across instances: %v", got)
	}
}

// HOST03: parallel guesses at an unknown name get no more tries than parallel
// guesses at an existing one, as the account row lock guarantees for accounts.
func TestUnknownPasswordCoolOffSerializesParallelAttempts(t *testing.T) {
	s := accountSecurityFixture(t)
	_ = verifiedAccount(t, s, "realname")
	burst := func(user string) map[string]int {
		var mu sync.Mutex
		var wg sync.WaitGroup
		counts := map[string]int{}
		for range 16 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				got := passwordOutcome(s, user, "WrongPass9", "203.0.113.9")
				if strings.HasPrefix(got, "cool-off") {
					got = "cool-off"
				}
				mu.Lock()
				counts[got]++
				mu.Unlock()
			}()
		}
		wg.Wait()
		return counts
	}
	known, unknown := burst("realname"), burst("ghostname")
	if fmt.Sprint(known) != fmt.Sprint(unknown) || known["credentials"] != networkFreeFailuresPlusOne {
		t.Fatalf("existing %v, unknown %v", known, unknown)
	}
}

// networkFreeFailuresPlusOne is how many failures a network earns before its
// first cool-off: four free ones and the one that starts the cool-off.
const networkFreeFailuresPlusOne = 5

// HOST03: there is no "full" state in which unknown names stop cooling off.
// With every network bucket already live, and after a flood of other names,
// the table holds no more than its fixed rows and an unknown name still
// answers like an existing account.
func TestUnknownPasswordCoolOffAtFullCapacity(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	_ = verifiedAccount(t, s, "realname")
	subjects := make([][]byte, 0, postgres.UnknownPasswordBuckets)
	for slot := range uint32(postgres.UnknownPasswordBuckets) {
		subjects = append(subjects, s.UnknownPasswordBucket(postgres.UnknownPasswordNetwork, slot))
	}
	if _, err := s.Store.DB.Exec(`INSERT INTO account_security_limits(subject_hash,kind,window_start,attempts) SELECT unnest($1::bytea[]),$2,clock_timestamp(),0`, subjects, postgres.UnknownPasswordNetwork); err != nil {
		t.Fatal(err)
	}
	for n := range 2000 {
		if _, _, err := s.AdmitUnknownPasswordAttempt(ctx, fmt.Sprintf("flood%d", n), fmt.Sprintf("192.0.2.%d", n%250)); err != nil {
			t.Fatal(err)
		}
	}
	if rows := unknownPasswordRows(t, s); rows > 3*postgres.UnknownPasswordBuckets {
		t.Fatalf("%d rows; bound is %d", rows, 3*postgres.UnknownPasswordBuckets)
	}
	known, unknown := passwordOutcomes(t, s, "realname", "203.0.113.9", 6), passwordOutcomes(t, s, "ghostname", "203.0.113.9", 6)
	if fmt.Sprint(known) != fmt.Sprint(unknown) || !strings.HasPrefix(unknown[5], "cool-off") {
		t.Fatalf("existing %v, unknown %v", known, unknown)
	}
}

// HOST02: a password reset proved by the emailed link and a factor ends the
// sign-in factor cool-off the old password's holder earned, so the owner signs
// in with the new password and a code at once.
func TestPasswordResetClearsSignInFactorCoolOff(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "resetowner")
	enrollment := securityAction(t, s, a, "mfa_enroll", "")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	step := func() uint64 {
		var now time.Time
		if err := s.Store.DB.QueryRow(`SELECT clock_timestamp()`).Scan(&now); err != nil {
			t.Fatal(err)
		}
		return uint64(now.Unix() / 30)
	}
	enabled, err := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, identity.HOTP(key, step(), 6), security.RandomID(""))
	if err != nil {
		t.Fatal(err)
	}
	challenge := func(password string) *identity.MFAChallenge {
		t.Helper()
		if _, err := s.Store.DB.Exec(`DELETE FROM account_security_limits WHERE kind IN('mfa-account','factor-account','mfa-challenge')`); err != nil {
			t.Fatal(err)
		}
		r, err := s.PasswordLoginBound(ctx, "resetowner", password, "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
		if err != nil || r.Challenge == nil {
			t.Fatal(err)
		}
		return r.Challenge
	}
	wrong := func() string {
		current := step()
		for n := 0; ; n++ {
			code := fmt.Sprintf("%06d", n)
			if code != identity.HOTP(key, current-1, 6) && code != identity.HOTP(key, current, 6) && code != identity.HOTP(key, current+1, 6) {
				return code
			}
		}
	}
	for range 2 {
		c := challenge("StrongPass9")
		for range 5 {
			if _, err := s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, wrong()); !errors.Is(err, identity.ErrChallenge) {
				t.Fatal(err)
			}
		}
	}
	var cool identity.CoolOff
	if c := challenge("StrongPass9"); !errors.As(func() error { _, err := s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, wrong()); return err }(), &cool) {
		t.Fatal("sign-in factor cool-off was not reached")
	}
	if err = s.RequestRecovery(ctx, postgres.RecoveryRequest{Username: "resetowner", ContactAddress: "resetowner@example.test", RequestID: security.RandomID("")}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err = s.Store.DB.QueryRow(`SELECT id FROM account_email_challenges WHERE purpose='password_reset' AND account_id=$1`, a.Account.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err = s.ResetPassword(ctx, mailPayload(t, s, mailoutbox.PasswordReset, id).Code, "NewStrongPass8", enabled.RecoveryCodes[0]); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err = s.Store.DB.QueryRow(`SELECT count(*) FROM account_security_limits WHERE kind='factor-backoff'`).Scan(&rows); err != nil || rows != 0 {
		t.Fatal("reset left the sign-in factor count", rows, err)
	}
	c := challenge("NewStrongPass8")
	if _, err = s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, identity.HOTP(key, step()+1, 6)); err != nil {
		t.Fatal("owner still cooled off after reset", err)
	}
}

// HOST02: second-factor failures escalate across fresh challenges and do not
// replenish with the fifteen-minute window, while a recovery code still gets
// the owner in and clears the count.
func TestMFAFailuresEscalateAcrossFreshChallenges(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "mfaguess")
	enrollment := securityAction(t, s, a, "mfa_enroll", "")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	step := func() uint64 {
		var now time.Time
		if err := s.Store.DB.QueryRow(`SELECT clock_timestamp()`).Scan(&now); err != nil {
			t.Fatal(err)
		}
		return uint64(now.Unix() / 30)
	}
	enabled, err := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, identity.HOTP(key, step(), 6), security.RandomID(""))
	if err != nil {
		t.Fatal(err)
	}
	wrong := func() string {
		current := step()
		for n := 0; ; n++ {
			code := fmt.Sprintf("%06d", n)
			if code != identity.HOTP(key, current-1, 6) && code != identity.HOTP(key, current, 6) && code != identity.HOTP(key, current+1, 6) {
				return code
			}
		}
	}
	// Each round starts a new fifteen-minute window, so only the sustained
	// count can refuse.
	freshChallenge := func() *identity.MFAChallenge {
		if _, err := s.Store.DB.Exec(`DELETE FROM account_security_limits WHERE kind IN('mfa-account','factor-account','mfa-challenge')`); err != nil {
			t.Fatal(err)
		}
		r, err := s.PasswordLoginBound(ctx, "mfaguess", "StrongPass9", "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
		if err != nil || r.Challenge == nil {
			t.Fatal(err)
		}
		return r.Challenge
	}
	coolOff := func(c *identity.MFAChallenge, factor string) time.Duration {
		t.Helper()
		_, err := s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, factor)
		var cool identity.CoolOff
		if !errors.As(err, &cool) || !errors.Is(err, identity.ErrRateLimited) {
			t.Fatalf("expected a cool-off, got %v", err)
		}
		return time.Until(cool.Until)
	}
	pastCoolOff := func() {
		if _, err := s.Store.DB.Exec(`UPDATE account_security_limits SET window_start=clock_timestamp()-interval '31 minutes' WHERE kind='factor-backoff'`); err != nil {
			t.Fatal(err)
		}
	}
	for range 2 {
		c := freshChallenge()
		for range 5 {
			if _, err := s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, wrong()); !errors.Is(err, identity.ErrChallenge) {
				t.Fatal(err)
			}
		}
	}
	c := freshChallenge()
	// Even the right code waits: the cool-off is not an answer about the code.
	if wait := coolOff(c, identity.HOTP(key, step()+1, 6)); wait <= 0 || wait > 30*time.Second {
		t.Fatalf("first cool-off %v", wait)
	}
	var attempts int
	if err = s.Store.DB.QueryRow(`SELECT attempts FROM account_mfa_challenges WHERE id=$1`, c.ChallengeID).Scan(&attempts); err != nil || attempts != 0 {
		t.Fatal("a refused code consumed the challenge", attempts, err)
	}
	// After the cool-off, one more failure earns a longer one.
	pastCoolOff()
	if _, err = s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, wrong()); !errors.Is(err, identity.ErrChallenge) {
		t.Fatal("cool-off outlasted its end", err)
	}
	if wait := coolOff(c, wrong()); wait <= 45*time.Second || wait > time.Minute {
		t.Fatalf("cool-off did not double: %v", wait)
	}
	// The wait never exceeds its cap, however long the attack.
	if _, err = s.Store.DB.Exec(`UPDATE account_security_limits SET attempts=500 WHERE kind='factor-backoff'`); err != nil {
		t.Fatal(err)
	}
	if wait := coolOff(c, wrong()); wait > 30*time.Minute {
		t.Fatalf("cool-off past its cap: %v", wait)
	}
	// A recovery code can't be guessed, so it is still checked: the owner gets
	// past an attacker who holds the password, and the count clears.
	if _, err = s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, enabled.RecoveryCodes[0]); err != nil {
		t.Fatal("recovery code refused during cool-off", err)
	}
	var rows int
	if err = s.Store.DB.QueryRow(`SELECT count(*) FROM account_security_limits WHERE kind='factor-backoff'`).Scan(&rows); err != nil || rows != 0 {
		t.Fatal("success did not clear the count", rows, err)
	}
	// Failures are forgotten after a quiet day, not after fifteen minutes.
	c = freshChallenge()
	if _, err = s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, wrong()); !errors.Is(err, identity.ErrChallenge) {
		t.Fatal(err)
	}
	if _, err = s.Store.DB.Exec(`UPDATE account_security_limits SET attempts=40,window_start=clock_timestamp()-interval '25 hours' WHERE kind='factor-backoff'`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteMFA(ctx, c.ChallengeID, c.ChallengeToken, wrong()); !errors.Is(err, identity.ErrChallenge) {
		t.Fatal("a day-old count still refused", err)
	}
	if err = s.Store.DB.QueryRow(`SELECT attempts FROM account_security_limits WHERE kind='factor-backoff'`).Scan(&rows); err != nil || rows != 1 {
		t.Fatal("stale count not restarted", rows, err)
	}
}

// A live link's rejected factor is distinct from an unusable link. The typed
// presentation result must not consume the link or relax its attempt budget.
func TestResetFactorResultPreservesLinkAndFactorGuards(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "resettyped")
	enrollment := securityAction(t, s, a, "mfa_enroll", "")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(enrollment.Secret)
	if err != nil {
		t.Fatal(err)
	}
	var now time.Time
	if err = s.Store.DB.QueryRow(`SELECT clock_timestamp()`).Scan(&now); err != nil {
		t.Fatal(err)
	}
	enabled, err := s.ConfirmEnrollment(ctx, a.AccessToken, enrollment.ChallengeID, identity.HOTP(key, uint64(now.Unix()/30), 6), security.RandomID(""))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.RequestRecovery(ctx, postgres.RecoveryRequest{Username: "resettyped", ContactAddress: "resettyped@example.test", RequestID: security.RandomID("")}); err != nil {
		t.Fatal(err)
	}
	var id string
	if err = s.Store.DB.QueryRow(`SELECT id FROM account_email_challenges WHERE account_id=$1 AND purpose='password_reset'`, a.Account.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	token := mailPayload(t, s, mailoutbox.PasswordReset, id).Code
	if err = s.ResetPassword(ctx, token, "NewStrongPass8", "not-a-factor"); !errors.Is(err, identity.ErrResetFactor) || !errors.Is(err, identity.ErrChallenge) {
		t.Fatalf("factor result: %v", err)
	}
	var attempts int
	if err = s.Store.DB.QueryRow(`SELECT attempts FROM account_email_challenges WHERE id=$1`, id).Scan(&attempts); err != nil || attempts != 1 {
		t.Fatal("factor attempt was not retained", attempts, err)
	}
	if err = s.ResetPassword(ctx, token, "x", enabled.RecoveryCodes[0]); !errors.Is(err, identity.ErrInvalid) {
		t.Fatalf("password result: %v", err)
	}
	if err = s.ResetPassword(ctx, token, "NewStrongPass8", enabled.RecoveryCodes[0]); err != nil {
		t.Fatal("invalid password consumed recovery factor or link", err)
	}
	if err = s.ResetPassword(ctx, token, "NewStrongPass8", enabled.RecoveryCodes[0]); err != nil {
		t.Fatal("exact reset replay changed", err)
	}
	if err = s.ResetPassword(ctx, token, "AnotherStrong9", "not-a-factor"); !errors.Is(err, identity.ErrChallenge) || errors.Is(err, identity.ErrResetFactor) {
		t.Fatalf("used link reported a live factor refusal: %v", err)
	}
}
