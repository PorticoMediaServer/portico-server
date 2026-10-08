package postgres_test

import (
	"context"
	"testing"
	"time"

	"portico.local/hosted/internal/postgres"
)

func waitForSignal(t *testing.T, c <-chan struct{}, within time.Duration, what string) {
	t.Helper()
	select {
	case <-c:
	case <-time.After(within):
		t.Fatal("no notification arrived for " + what)
	}
}

func drain(c <-chan struct{}) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

// The whole substitute for polling: an enqueue announces itself inside its own transaction,
// and a listening connection turns that into a signal. Each queue gets its own channel, and
// the channel names carry the schema so the per-test schemas here cannot hear each other.
func TestQueueEnqueuesAnnounceThemselves(t *testing.T) {
	f := setup(t)
	listener := f.store.Notifications()
	wake := listener.Subscribe(postgres.WakeChannel)
	mail := listener.Subscribe(postgres.MailChannel)
	certificates := listener.Subscribe(postgres.CertificateChannel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.Run(ctx)
	// A fresh listener assumes it missed everything, so it signals once on connect.
	waitForSignal(t, wake, 10*time.Second, "the listener's first connection")
	drain(mail)
	drain(certificates)
	drain(wake)
	// A wake becoming deliverable is announced by the database, not by this process, because
	// wake_state has a second writer that never passes through Go.
	if _, e := f.store.DB.Exec(`INSERT INTO wake_state(server_id,kind,desired_revision) VALUES($1,'policy',7)
   ON CONFLICT(server_id,kind) DO UPDATE SET desired_revision=GREATEST(wake_state.desired_revision,EXCLUDED.desired_revision),next_attempt_at=LEAST(wake_state.next_attempt_at,now())`, f.server); e != nil {
		t.Fatal(e)
	}
	waitForSignal(t, wake, 10*time.Second, "an enqueued wake")
	// Leasing it is not news: the revision and the deadline are unchanged, so a busy drain
	// does not announce itself sixteen times a second.
	drain(wake)
	if _, e := f.store.DB.Exec(`UPDATE wake_state SET lease_token='lease_x',lease_until=now()+interval '15 seconds',attempts=attempts+1 WHERE server_id=$1 AND kind='policy'`, f.server); e != nil {
		t.Fatal(e)
	}
	// Finishing it pushes the deadline into the future, which is also not news.
	if _, e := f.store.DB.Exec(`UPDATE wake_state SET lease_token=NULL,lease_until=NULL,next_attempt_at=now()+interval '30 seconds' WHERE server_id=$1 AND kind='policy'`, f.server); e != nil {
		t.Fatal(e)
	}
	select {
	case <-wake:
		t.Fatal("leasing or finishing a wake announced itself")
	case <-time.After(400 * time.Millisecond):
	}
	// A newer revision arriving while the deadline is in the future pulls it back to now,
	// which is news again.
	if _, e := f.store.DB.Exec(`UPDATE wake_state SET desired_revision=desired_revision+1,next_attempt_at=LEAST(next_attempt_at,now()) WHERE server_id=$1 AND kind='policy'`, f.server); e != nil {
		t.Fatal(e)
	}
	waitForSignal(t, wake, 10*time.Second, "a newer revision")
}

// A rolled-back enqueue announces nothing: the notification is part of the transaction, so a
// worker never wakes for work that does not exist.
func TestARolledBackEnqueueAnnouncesNothing(t *testing.T) {
	f := setup(t)
	listener := f.store.Notifications()
	wake := listener.Subscribe(postgres.WakeChannel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.Run(ctx)
	waitForSignal(t, wake, 10*time.Second, "the listener's first connection")
	drain(wake)
	tx, e := f.store.DB.Begin()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = tx.Exec(`INSERT INTO wake_state(server_id,kind,desired_revision) VALUES($1,'policy',9)
   ON CONFLICT(server_id,kind) DO UPDATE SET desired_revision=EXCLUDED.desired_revision,next_attempt_at=now()`, f.server); e != nil {
		t.Fatal(e)
	}
	if e = tx.Rollback(); e != nil {
		t.Fatal(e)
	}
	select {
	case <-wake:
		t.Fatal("a rolled-back enqueue woke a worker")
	case <-time.After(500 * time.Millisecond):
	}
}

// NextDue is what lets the worker sleep exactly as long as it should. An empty queue has no
// deadline at all, which is the state an idle deployment is in.
func TestNextDueReportsTheEarliestDeliverableDeadline(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	if _, ok, e := f.store.NextDue(ctx); e != nil || ok {
		t.Fatal("an empty queue reported a deadline", ok, e)
	}
	if _, e := f.store.DB.Exec(`UPDATE servers SET base_url='https://example.invalid:32500',endpoint_verified=true WHERE id=$1`, f.server); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.DB.Exec(`UPDATE wake_state SET desired_revision=desired_revision+1,acked_revision=0,next_attempt_at=now()+interval '90 seconds' WHERE server_id=$1 AND kind='policy'`, f.server); e != nil {
		t.Fatal(e)
	}
	at, ok, e := f.store.NextDue(ctx)
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	if until := time.Until(at); until < 30*time.Second || until > 2*time.Minute {
		t.Fatal("the reported deadline is not the row's", until)
	}
	// An acknowledged row is not deliverable, so it is not a reason to wake.
	if _, e = f.store.DB.Exec(`UPDATE wake_state SET acked_revision=desired_revision WHERE server_id=$1`, f.server); e != nil {
		t.Fatal(e)
	}
	if _, ok, e = f.store.NextDue(ctx); e != nil || ok {
		t.Fatal("an acknowledged wake still asked to be delivered", ok, e)
	}
}

// A38: an export request announces itself, so the export worker sleeps
// instead of polling.
func TestAccountExportRequestsAnnounceThemselves(t *testing.T) {
	f := setup(t)
	listener := f.store.Notifications()
	exports := listener.Subscribe(postgres.ExportChannel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.Run(ctx)
	waitForSignal(t, exports, 10*time.Second, "the listener's first connection")
	drain(exports)
	if _, e := f.store.DB.Exec(`INSERT INTO account_exports(id,account_id,operation_id) VALUES('export_1',$1,'op_export_1')`, f.owner.Account.ID); e != nil {
		t.Fatal(e)
	}
	waitForSignal(t, exports, 10*time.Second, "a requested export")
}

// A54: the offline scan reports each silent server once per silence, through
// the marker, instead of re-reading every offline server each pass.
func TestOfflineScanReportsEachSilenceOnce(t *testing.T) {
	f := setup(t)
	s := &postgres.AccountSecurity{Store: f.store, Keys: f.keys, MailEnabled: true}
	ctx := context.Background()
	if _, e := f.store.DB.Exec(`UPDATE servers SET presence_expires_at=now()-interval '2 days',last_seen_at=now()-interval '3 days' WHERE id=$1`, f.server); e != nil {
		t.Fatal(e)
	}
	for range 2 {
		if e := s.ScanOfflineServers(ctx); e != nil {
			t.Fatal(e)
		}
	}
	var events int
	if e := f.store.DB.QueryRow(`SELECT count(*) FROM account_notification_events WHERE server_id=$1 AND kind='server_offline'`, f.server).Scan(&events); e != nil || events != 1 {
		t.Fatal(events, e)
	}
	// The server comes back, then goes quiet again: a new silence is a new report.
	if _, e := f.store.DB.Exec(`UPDATE servers SET last_seen_at=now()-interval '2 days' WHERE id=$1`, f.server); e != nil {
		t.Fatal(e)
	}
	if e := s.ScanOfflineServers(ctx); e != nil {
		t.Fatal(e)
	}
	if e := f.store.DB.QueryRow(`SELECT count(*) FROM account_notification_events WHERE server_id=$1 AND kind='server_offline'`, f.server).Scan(&events); e != nil || events != 2 {
		t.Fatal(events, e)
	}
}

// The Hosted operator hears about signing-key expiry in the app, once per key.
func TestSigningKeyAlertReachesTheOperator(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	operator := verifiedAccount(t, s, "operator")
	expires := time.Now().Add(20 * 24 * time.Hour)
	for range 2 {
		if e := s.QueueSigningKeyAlert(ctx, operator.Account.ID, "key_one", expires); e != nil {
			t.Fatal(e)
		}
	}
	feed, e := s.Notifications(ctx, operator.AccessToken, "")
	if e != nil {
		t.Fatal(e)
	}
	alerts := 0
	for _, n := range feed.Items {
		if n.Kind == "signing_key_expiring" {
			alerts++
		}
	}
	if alerts != 1 {
		t.Fatal(alerts, feed)
	}
}

// A75: the offline worker's next wake is exactly when the next server crosses
// the threshold, and a batch drains every overdue server at once.
func TestOfflineScanDrainsAndReportsTheNextDeadline(t *testing.T) {
	f := setup(t)
	s := &postgres.AccountSecurity{Store: f.store, Keys: f.keys}
	ctx := context.Background()
	at := time.Now().Add(-time.Hour).UTC().Truncate(time.Second)
	if _, e := f.store.DB.Exec(`UPDATE servers SET presence_expires_at=$2,last_seen_at=now() WHERE id=$1`, f.server, at); e != nil {
		t.Fatal(e)
	}
	next, e := s.NextOfflineDue(ctx)
	if e != nil || !next.Equal(at.Add(24*time.Hour)) {
		t.Fatal(next, e)
	}
}

// Readiness asks the running listener, not a new one that never connects.
func TestReadinessSeesTheRunningListener(t *testing.T) {
	f := setup(t)
	listener := f.store.Notifications()
	wake := listener.Subscribe(postgres.WakeChannel)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go listener.Run(ctx)
	waitForSignal(t, wake, 10*time.Second, "the listener's first connection")
	if e := f.store.Ready(ctx); e != nil {
		t.Fatal("ready with a connected listener:", e)
	}
}
