package postgres_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/oidclogin"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
)

func rulingNotificationCall(h http.Handler, token, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	out := httptest.NewRecorder()
	h.ServeHTTP(out, r)
	return out
}

func rulingNotificationFeed(t *testing.T, h http.Handler, token, path string) postgres.NotificationFeed {
	t.Helper()
	out := rulingNotificationCall(h, token, "GET", path, "")
	var feed postgres.NotificationFeed
	if out.Code != 200 || json.Unmarshal(out.Body.Bytes(), &feed) != nil {
		t.Fatalf("feed status %d: %s", out.Code, out.Body.String())
	}
	return feed
}

func TestNotificationPreferencesRequireAnExplicitBoolean(t *testing.T) {
	for _, body := range []string{`null`, `{}`, `[]`, `false`, `{"serverOffline":null}`, `{"serverOffline":"false"}`, `{"serverOffline":0}`, `{"serverOffline":false,"storageNearlyFull":true}`, `{"serverOffline":false,"invitationAccepted":true}`} {
		var preferences postgres.NotificationPreferences
		if json.Unmarshal([]byte(body), &preferences) == nil {
			t.Fatalf("accepted incomplete or retired preference: %s", body)
		}
	}
	for _, value := range []bool{false, true} {
		raw, err := json.Marshal(postgres.NotificationPreferences{ServerOffline: value})
		if err != nil {
			t.Fatal(err)
		}
		var preferences postgres.NotificationPreferences
		if json.Unmarshal(raw, &preferences) != nil || preferences.ServerOffline != value {
			t.Fatalf("boolean round trip: %s", raw)
		}
	}
}

func TestNotificationPreferenceHTTPFalseSaveAndAccountReadback(t *testing.T) {
	s := accountSecurityFixture(t)
	a := verifiedAccount(t, s, "notificationprefs")
	b := verifiedAccount(t, s, "notificationother")
	h := (&httpapi.Handler{Identity: s.Identity, Security: s}).Routes()
	const path = "/v1/account/notification-preferences"
	read := func(token, want string) {
		t.Helper()
		out := rulingNotificationCall(h, token, "GET", path, "")
		if out.Code != 200 || out.Body.String() != want+"\n" {
			t.Fatalf("preference readback %d: %s", out.Code, out.Body.String())
		}
	}
	read(a.AccessToken, `{"serverOffline":true}`)
	for _, body := range []string{`{}`, `null`, `{"serverOffline":null}`, `{"serverOffline":"false"}`, `{"serverOffline":false,"storageNearlyFull":false}`, `{"serverOffline":false,"invitationAccepted":false}`} {
		out := rulingNotificationCall(h, a.AccessToken, "PUT", path, body)
		if out.Code != 400 {
			t.Fatalf("invalid preference %s returned %d", body, out.Code)
		}
		read(a.AccessToken, `{"serverOffline":true}`)
	}
	for _, body := range []string{`{"serverOffline":false}`, `{"serverOffline":true}`} {
		out := rulingNotificationCall(h, a.AccessToken, "PUT", path, body)
		if out.Code != 200 || out.Body.String() != body+"\n" {
			t.Fatalf("preference save %d: %s", out.Code, out.Body.String())
		}
		read(a.AccessToken, body)
		read(b.AccessToken, `{"serverOffline":true}`)
	}
}

// Sign-in, export and quiet-registration items come from their real writers.
// No frame example name or positive notification row is seeded here.
func TestNotificationFeedUsesRegistrationFactsAndExcludesRetiredNotices(t *testing.T) {
	f := setup(t)
	s := &postgres.AccountSecurity{Store: f.store, Keys: f.keys, Identity: f.ids, Lifetime: 15 * time.Minute}
	ctx := context.Background()
	h := (&httpapi.Handler{Identity: f.ids, Security: s}).Routes()
	const feedPath = "/v1/account/notifications"
	feed := rulingNotificationFeed(t, h, f.owner.AccessToken, feedPath)
	if len(feed.Items) != 1 || feed.Items[0].Kind != "new_device" {
		t.Fatalf("actual sign-in missing: %#v", feed)
	}
	var device struct {
		ID   string `json:"deviceId"`
		Name string `json:"name"`
	}
	if json.Unmarshal(feed.Items[0].Detail, &device) != nil {
		t.Fatal("invalid sign-in detail")
	}
	var deviceName string
	if err := f.store.DB.QueryRow(`SELECT name FROM hosted_devices WHERE id=$1 AND account_id=$2`, device.ID, f.owner.Account.ID).Scan(&deviceName); err != nil || device.Name != deviceName {
		t.Fatalf("sign-in name did not come from the actual device: %v", err)
	}
	job, err := s.RequestExport(ctx, f.owner.AccessToken, postgres.ExportRequest{OperationID: security.RandomID("")})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ProcessExport(ctx); err != nil {
		t.Fatal(err)
	}
	// Only the registration clock is advanced; the offline producer makes the row.
	if _, err = f.store.DB.Exec(`UPDATE servers SET presence_expires_at=now()-interval '2 days',last_seen_at=now()-interval '3 days' WHERE id=$1`, f.server); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = s.ScanOfflineServers(ctx); err != nil {
			t.Fatal(err)
		}
	}
	feed = rulingNotificationFeed(t, h, f.owner.AccessToken, feedPath)
	var quietID, exportID string
	var serverName string
	if err = f.store.DB.QueryRow(`SELECT name FROM servers WHERE id=$1`, f.server).Scan(&serverName); err != nil {
		t.Fatal(err)
	}
	for _, item := range feed.Items {
		switch item.Kind {
		case "server_offline":
			if quietID != "" || item.Server == nil || item.Server.ID != f.server || item.Server.Name != serverName {
				t.Fatalf("quiet notice does not describe its actual registration: %#v", item)
			}
			quietID = item.ID
		case "account_export_ready":
			var detail struct {
				ExportID string `json:"exportId"`
			}
			if json.Unmarshal(item.Detail, &detail) != nil || detail.ExportID != job.ID {
				t.Fatal("export notice did not name the completed export")
			}
			exportID = item.ID
		}
	}
	if quietID == "" || exportID == "" || feed.Unread != len(feed.Items) {
		t.Fatalf("actual registration/export facts missing: %#v", feed)
	}
	visible := len(feed.Items)
	// Simulate unsupported historical rows only to verify API exclusion. The
	// production baseline never accepts either retired kind.
	if _, err = f.store.DB.Exec(`ALTER TABLE account_notification_events DROP CONSTRAINT account_notification_events_kind_check`); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"invitation_accepted", "storage_nearly_full", "unknown_kind"} {
		id := "historical_" + kind
		if _, err = f.store.DB.Exec(`INSERT INTO account_notification_events(id,account_id,kind) VALUES($1,$2,$3)`, id, f.owner.Account.ID, kind); err != nil {
			t.Fatal(err)
		}
		if out := rulingNotificationCall(h, f.owner.AccessToken, "GET", feedPath+"/"+id, ""); out.Code == 200 {
			t.Fatalf("retired kind is individually readable: %s", kind)
		}
	}
	feed = rulingNotificationFeed(t, h, f.owner.AccessToken, feedPath)
	if len(feed.Items) != visible || feed.Unread != visible {
		t.Fatalf("retired kinds surfaced or changed unread: %#v", feed)
	}
	out := rulingNotificationCall(h, f.owner.AccessToken, "PUT", "/v1/account/notification-preferences", `{"serverOffline":false}`)
	if out.Code != 200 {
		t.Fatalf("quiet switch save: %d", out.Code)
	}
	feed = rulingNotificationFeed(t, h, f.owner.AccessToken, feedPath)
	if len(feed.Items) != visible-1 || feed.Unread != visible-1 {
		t.Fatalf("quiet switch hid more than the quiet notice: %#v", feed)
	}
	if out = rulingNotificationCall(h, f.owner.AccessToken, "GET", feedPath+"/"+quietID, ""); out.Code == 200 {
		t.Fatal("hidden quiet notice remained individually readable")
	}
	if out = rulingNotificationCall(h, f.owner.AccessToken, "GET", feedPath+"/"+exportID, ""); out.Code != 200 {
		t.Fatal("quiet switch hid an export")
	}
	if out = rulingNotificationCall(h, f.member.AccessToken, "GET", feedPath+"/"+exportID, ""); out.Code == 200 {
		t.Fatal("another account read the export notice")
	}
	// The existing operator recipient still receives their own key-expiry alert;
	// it never enters an ordinary account's list, unread count or item lookup.
	if err = s.QueueSigningKeyAlert(ctx, f.member.Account.ID, "ruling10-key", time.Now().Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	operatorFeed := rulingNotificationFeed(t, h, f.member.AccessToken, feedPath)
	var operatorID string
	for _, item := range operatorFeed.Items {
		if item.Kind == "signing_key_expiring" {
			operatorID = item.ID
		}
	}
	if operatorID == "" {
		t.Fatal("operator key alert was lost")
	}
	if out = rulingNotificationCall(h, f.owner.AccessToken, "GET", feedPath+"/"+operatorID, ""); out.Code == 200 {
		t.Fatal("ordinary account read the operator notice")
	}
	feed = rulingNotificationFeed(t, h, f.owner.AccessToken, feedPath)
	if len(feed.Items) != visible-1 || feed.Unread != visible-1 {
		t.Fatal("operator notice changed an ordinary account's list or unread")
	}
	if out = rulingNotificationCall(h, f.owner.AccessToken, "POST", feedPath+"/read", `{"all":true,"ids":[]}`); out.Code != 200 {
		t.Fatal("mark read failed")
	}
	if feed = rulingNotificationFeed(t, h, f.owner.AccessToken, feedPath); feed.Unread != 0 {
		t.Fatal("visible unread did not clear")
	}
}

func TestProviderLinkedNoticeRequiresAnActualAddedMethod(t *testing.T) {
	s := accountSecurityFixture(t)
	ctx := context.Background()
	a := verifiedAccount(t, s, "notificationlinker")
	o := &postgres.OIDCLogin{Store: s.Store, Keys: s.Keys, Security: s, Lifetime: 15 * time.Minute, Providers: map[string]*oidclogin.Provider{"google": {ID: "google", Issuer: "https://accounts.google.com", Audience: "fixture-client", Callback: "https://web.getportico.tv/auth/sso/google/callback"}}}
	if err := o.Configure(ctx); err != nil {
		t.Fatal(err)
	}
	link := func() (string, string, string) {
		t.Helper()
		binding := security.RandomID("")
		intent := &identity.SecurityIntent{Purpose: "provider_link", Target: "google", OperationID: security.RandomID("")}
		v, _, _, err := o.StartFlow(ctx, "google", binding, a.AccessToken, postgres.OIDCStartInput{Mode: "link", SessionMode: "native", RequestID: security.RandomID(""), Intent: intent, Completion: postgres.NativeCompletionCode})
		if err != nil {
			t.Fatal(err)
		}
		// Same verified provider-assertion fixture seam as provider_link_test.go;
		// no provider is contacted and no notification is inserted by the test.
		if _, err = s.Store.DB.Exec(`INSERT INTO identity_verified_assertions(transaction_id,issuer,subject,audience,provider_issued_at,verified_at,nonce_hash,provider_email,email_verified,transaction_revision,lease_id) SELECT id,'https://accounts.google.com','ruling10-link-subject','fixture-client',clock_timestamp(),clock_timestamp(),nonce_hash,'linker@example.test',true,revision,'fixture-lease' FROM identity_transactions WHERE id=$1`, v.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = s.Store.DB.Exec(`UPDATE identity_transactions SET status='verified' WHERE id=$1`, v.ID); err != nil {
			t.Fatal(err)
		}
		code := security.RandomID("")
		if _, err = s.Store.DB.Exec(`INSERT INTO identity_native_completions(transaction_id,code_hash,expires_at) VALUES($1,$2,clock_timestamp()+interval '2 minutes')`, v.ID, security.Hash(code)); err != nil {
			t.Fatal(err)
		}
		out, err := o.CompleteFlow(ctx, v.ID, binding, a.AccessToken, postgres.OIDCCompleteInput{CompletionCode: code}, true)
		if err != nil || out.Result == nil || out.Result.SignOut {
			t.Fatalf("link completion: %#v %v", out, err)
		}
		return v.ID, binding, code
	}
	countAdded := func(want int) {
		t.Helper()
		feed, err := s.Notifications(ctx, a.AccessToken, "")
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, item := range feed.Items {
			var detail struct {
				Event string `json:"event"`
			}
			if item.Kind == "security_event" && json.Unmarshal(item.Detail, &detail) == nil && detail.Event == "provider_linked" {
				count++
			}
		}
		if count != want {
			t.Fatalf("method-added notices %d want %d", count, want)
		}
	}
	id, binding, code := link()
	countAdded(1)
	_, _ = o.CompleteFlow(ctx, id, binding, a.AccessToken, postgres.OIDCCompleteInput{CompletionCode: code}, true)
	countAdded(1)
	link() // A fresh operation over the still-active same subject adds nothing.
	countAdded(1)
	var identityID string
	if err := s.Store.DB.QueryRow(`SELECT id FROM account_external_identities WHERE account_id=$1 AND provider='google' AND revoked_at IS NULL`, a.Account.ID).Scan(&identityID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Mutate(ctx, a.AccessToken, postgres.SecurityAction{SecurityIntent: identity.SecurityIntent{Purpose: "provider_unlink", Target: identityID, OperationID: security.RandomID("")}}); err != nil {
		t.Fatal(err)
	}
	link() // Restoring a revoked method is an actual addition.
	countAdded(2)
	if _, err := s.NotificationPreferences(ctx, a.AccessToken, &postgres.NotificationPreferences{ServerOffline: false}); err != nil {
		t.Fatal(err)
	}
	countAdded(2) // Security events remain shown with quiet-server notices off.
}
