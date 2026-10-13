package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAccountExportProofIsolationExpiryAndRetention(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "exporter")
	b := verifiedAccount(t, s, "outsider")
	// A signed-in session is enough to request an export.
	job, e := s.RequestExport(ctx, a.AccessToken, postgres.ExportRequest{OperationID: security.RandomID("")})
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ProcessExport(ctx); e != nil {
		t.Fatal(e)
	}
	job, _, e = s.Export(ctx, a.AccessToken, job.ID, false)
	if e != nil || job.State != "ready" {
		t.Fatal(job, e)
	}
	// Completion is announced in the app, with the export's id.
	feed, e := s.Notifications(ctx, a.AccessToken, "")
	if e != nil || len(feed.Items) == 0 || feed.Items[0].Kind != "account_export_ready" || !strings.Contains(string(feed.Items[0].Detail), job.ID) {
		t.Fatal("no export notification", feed, e)
	}
	if _, _, e = s.Export(ctx, b.AccessToken, job.ID, true); e == nil {
		t.Fatal("cross-account export")
	}
	_, data, e := s.Export(ctx, a.AccessToken, job.ID, true)
	if e != nil || !json.Valid(data) {
		t.Fatal(string(data), e)
	}
	for _, secret := range []string{"StrongPass9", "password_hash", "current_refresh_hash", b.Account.ID} {
		if strings.Contains(string(data), secret) {
			t.Fatal("export leaked secret or another account")
		}
	}
	if _, e = s.Store.DB.Exec(`UPDATE account_exports SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, job.ID); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.Export(ctx, a.AccessToken, job.ID, true); e == nil {
		t.Fatal("expired export")
	}
	if _, e = s.Store.DB.Exec(`INSERT INTO security_audit(actor_id,action,target_id,created_at) VALUES($1,'old',$1,now()-interval '401 days'),($1,'keep',$1,now()-interval '399 days')`, a.Account.ID); e != nil {
		t.Fatal(e)
	}
	if e = s.Store.PruneExpired(ctx, 10000); e != nil {
		t.Fatal(e)
	}
	var old, keep, exports int
	if e = s.Store.DB.QueryRow(`SELECT (SELECT count(*) FROM security_audit WHERE action='old'),(SELECT count(*) FROM security_audit WHERE action='keep'),(SELECT count(*) FROM account_exports)`).Scan(&old, &keep, &exports); e != nil || old != 0 || keep != 1 || exports != 0 {
		t.Fatal(old, keep, exports, e)
	}
}
func TestPasswordSessionCreatesRevocableDeviceAndNotificationPreferences(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "deviceowner")
	devices, e := s.Devices(ctx, a.AccessToken)
	if e != nil || len(devices.Items) != 1 || devices.Items[0].FamilyID != a.FamilyID {
		t.Fatal(devices, e)
	}
	// A new sign-in is an in-app notification with the device, never an email.
	feed, e := s.Notifications(ctx, a.AccessToken, "")
	if e != nil || len(feed.Items) != 1 || feed.Items[0].Kind != "new_device" || feed.Unread != 1 || !strings.Contains(string(feed.Items[0].Detail), `"deviceId"`) {
		t.Fatal(feed, e)
	}
	var mails int
	if e = s.Store.DB.QueryRow(`SELECT count(*) FROM account_mail_outbox WHERE account_id=$1 AND kind NOT IN('registration')`, a.Account.ID).Scan(&mails); e != nil || mails != 0 {
		t.Fatal("alert email sent", mails, e)
	}
	if e = s.MarkNotificationsRead(ctx, a.AccessToken, nil, true); e != nil {
		t.Fatal(e)
	}
	if feed, e = s.Notifications(ctx, a.AccessToken, ""); e != nil || feed.Unread != 0 || !feed.Items[0].Read {
		t.Fatal(feed, e)
	}
	prefs, e := s.NotificationPreferences(ctx, a.AccessToken, nil)
	if e != nil || !prefs.ServerOffline || !prefs.StorageNearlyFull || !prefs.InvitationAccepted {
		t.Fatal(prefs, e)
	}
	prefs.ServerOffline = false
	if _, e = s.NotificationPreferences(ctx, a.AccessToken, &prefs); e != nil {
		t.Fatal(e)
	}
	if e = s.Identity.Revoke(ctx, a.AccessToken); e != nil {
		t.Fatal(e)
	}
	var revoked bool
	if e = s.Store.DB.QueryRow(`SELECT revoked_at IS NOT NULL FROM hosted_devices WHERE family_id=$1`, a.FamilyID).Scan(&revoked); e != nil || !revoked {
		t.Fatal(revoked, e)
	}
}

func TestPasswordBackoffIsPerAccountAndClearsOnSuccess(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "backoffA")
	_ = verifiedAccount(t, s, "backoffB")
	login := func(user, password, network string) error {
		_, err := s.PasswordLoginBound(ctx, user, password, "browser", security.RandomID(""), security.RandomID(""), network)
		return err
	}
	attacker, owner := "203.0.113.9", "198.51.100.7"
	for range 5 {
		if err := login("backoffA", "WrongPass9", attacker); err != identity.ErrCredentials {
			t.Fatal(err)
		}
	}
	// The failing network cools off: its next attempt waits, and says so.
	err := login("backoffA", "StrongPass9", attacker)
	var cool identity.CoolOff
	if !errors.As(err, &cool) || !errors.Is(err, identity.ErrRateLimited) || !cool.Until.After(time.Now()) {
		t.Fatal("no cool-off after repeated failures", err)
	}
	// A8/A70: the owner's own network, and every other account, are unaffected.
	if err := login("backoffA", "StrongPass9", owner); err != nil {
		t.Fatal("attacker locked the real user out", err)
	}
	if err := login("backoffB", "StrongPass9", attacker); err != nil {
		t.Fatal("other account blocked", err)
	}
	// Once the cool-off ends, the correct password signs in and clears it.
	if _, err := s.Store.DB.Exec(`UPDATE account_password_backoff SET locked_until=clock_timestamp()-interval '1 second' WHERE account_id=$1`, a.Account.ID); err != nil {
		t.Fatal(err)
	}
	if err := login("backoffA", "StrongPass9", attacker); err != nil {
		t.Fatal(err)
	}
	var failures int
	if err := s.Store.DB.QueryRow(`SELECT COALESCE(max(failures),0) FROM account_password_backoff WHERE account_id=$1 AND network<>'\x0000000000000000000000000000000000000000000000000000000000000000'`, a.Account.ID).Scan(&failures); err != nil || failures != 0 {
		t.Fatal(failures, err)
	}
}

// A distributed attack past the account-wide ceiling only lengthens cool-offs
// after failures. A correct password from any network that hasn't just failed
// always signs in (Justin, 23 Sep).
func TestPasswordBackoffCeilingNeverRefusesACorrectPassword(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "wideowner")
	login := func(password, network string) error {
		_, err := s.PasswordLoginBound(ctx, "wideowner", password, "browser", security.RandomID(""), security.RandomID(""), network)
		return err
	}
	if _, err := s.Store.DB.Exec(`INSERT INTO account_password_backoff(account_id,network,failures,window_start) VALUES($1,'\x0000000000000000000000000000000000000000000000000000000000000000',500,clock_timestamp()) ON CONFLICT(account_id,network) DO UPDATE SET failures=500,window_start=clock_timestamp()`, a.Account.ID); err != nil {
		t.Fatal(err)
	}
	if err := login("StrongPass9", "203.0.113.50"); err != nil {
		t.Fatal("a correct password was refused under attack", err)
	}
	// Under attack, even a first failure earns a short cool-off for that network.
	if err := login("WrongPass9", "203.0.113.51"); err != identity.ErrCredentials {
		t.Fatal(err)
	}
	if err := login("StrongPass9", "203.0.113.51"); !errors.Is(err, identity.ErrRateLimited) {
		t.Fatal("no cool-off under attack", err)
	}
}

// networkFreeGuessBudget: the free failures plus the one that starts the cool-off.
const networkFreeGuessBudget = 5

func TestPasswordBackoffCountsParallelGuessesAtomically(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "burstowner")
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.PasswordLoginBound(ctx, "burstowner", "WrongPass9", "browser", security.RandomID(""), security.RandomID(""), "203.0.113.9")
		}()
	}
	wg.Wait()
	var failures int
	if err := s.Store.DB.QueryRow(`SELECT failures FROM account_password_backoff WHERE account_id=$1 AND network<>'\x0000000000000000000000000000000000000000000000000000000000000000'`, a.Account.ID).Scan(&failures); err != nil {
		t.Fatal(err)
	}
	if failures > networkFreeGuessBudget {
		t.Fatalf("%d parallel guesses were checked; the backoff admits %d", failures, networkFreeGuessBudget)
	}
}

// Security events land in the feed as codes, once per operation.
func TestSecurityEventsAreInAppNotifications(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "eventowner")
	_ = securityAction(t, s, a, "email_change", "moved@example.test")
	code := mailPayload(t, s, mailoutbox.EmailChange, securityActionLastChallenge(t, s, a)).Code
	if _, err := s.ConfirmEmailLink(ctx, "email_change", securityActionLastChallenge(t, s, a), code); err != nil {
		t.Fatal(err)
	}
	login, err := s.PasswordLoginBound(ctx, "eventowner", "StrongPass9", "native", security.RandomID(""), security.RandomID(""), "198.51.100.7")
	if err != nil || login.Session == nil {
		t.Fatal(err)
	}
	feed, err := s.Notifications(ctx, login.Session.AccessToken, "")
	if err != nil {
		t.Fatal(err)
	}
	events := map[string]bool{}
	for _, n := range feed.Items {
		if n.Kind == "security_event" {
			var d map[string]string
			if json.Unmarshal(n.Detail, &d) != nil {
				t.Fatal(string(n.Detail))
			}
			events[d["event"]] = true
		}
	}
	if !events["contact_email_changed"] || !events["contact_email_verified"] {
		t.Fatalf("security events missing from the feed: %v", events)
	}
	item, err := s.NotificationByID(ctx, login.Session.AccessToken, feed.Items[0].ID)
	if err != nil || item.ID != feed.Items[0].ID {
		t.Fatal(item, err)
	}
}

func securityActionLastChallenge(t *testing.T, s *postgres.AccountSecurity, a identity.Session) string {
	t.Helper()
	var id string
	if err := s.Store.DB.QueryRow(`SELECT id FROM account_email_challenges WHERE account_id=$1 AND purpose='email_change' ORDER BY created_at DESC LIMIT 1`, a.Account.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestHostedRejectsEarlierPreReleaseSchema(t *testing.T) {
	store := database(t)
	ctx := context.Background()
	if _, e := store.DB.Exec(`DELETE FROM hosted_schema_migrations WHERE version>2;UPDATE hosted_schema_contract SET version=2 WHERE singleton`); e != nil {
		t.Fatal(e)
	}
	before := catalogue(t, store)
	if e := store.Migrate(ctx); e == nil {
		t.Fatal("upgraded unsupported pre-release format")
	}
	if after := catalogue(t, store); after != before {
		t.Fatal("modified old schema")
	}
}
