package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"portico.local/hosted/internal/device"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"strings"
	"sync"
	"testing"
)

func activation(t *testing.T) (*postgres.Store, *device.Service, *identity.Service, identity.Session, *security.Keys) {
	t.Helper()
	store := database(t)
	keys, e := fixtureKeys()
	if e != nil {
		t.Fatal(e)
	}
	ids := identity.New(store)
	account, e := ids.Register(context.Background(), "tvowner", "TV Owner", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	svc, e := device.New(store, keys, "https://web.getportico.tv/device", false)
	if e != nil {
		t.Fatal(e)
	}
	return store, svc, ids, account, keys
}
func startTV(t *testing.T, s *device.Service, n int) device.Authorization {
	t.Helper()
	v, e := s.Start(context.Background(), "192.0.2.1", device.Start{RequestID: fmt.Sprintf("12345678-1234-4123-8123-%012d", n), DeviceName: "Living Room TV", Platform: "tvos", AppVersion: "0.1.0"})
	if e != nil {
		t.Fatal(e)
	}
	return v
}
func due(t *testing.T, s *postgres.Store) {
	t.Helper()
	if _, e := s.DB.Exec(`UPDATE device_authorizations SET next_poll_at=now()-interval '1 second'`); e != nil {
		t.Fatal(e)
	}
}
func codeError(t *testing.T, e error, code string) {
	t.Helper()
	var p *device.ProtocolError
	if !errors.As(e, &p) || p.Code != code {
		t.Fatalf("want %s got %v", code, e)
	}
}
func approveTV(t *testing.T, s *device.Service, a identity.Session, auth device.Authorization) device.Preview {
	t.Helper()
	ctx := context.Background()
	p, e := s.Review(ctx, a.Account.ID, "192.0.2.2", strings.ToLower(strings.ReplaceAll(auth.UserCode, "-", " ")))
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Decide(ctx, a.AccessToken, "192.0.2.2", p.RequestID, p.UserCode, "approve"); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestDeviceApprovalRequiresExplicitAccountAndZeroServers(t *testing.T) {
	store, svc, ids, a, _ := activation(t)
	ctx := context.Background()
	auth := startTV(t, svc, 1)
	if len(auth.DeviceCode) != 50 || len(auth.UserCode) != 9 || strings.Contains(auth.VerificationURIComplete, auth.DeviceCode) || !strings.HasSuffix(auth.VerificationURIComplete, "#code="+auth.UserCode) {
		t.Fatal("unsafe code/QR projection")
	}
	for _, r := range strings.ReplaceAll(auth.UserCode, "-", "") {
		if !strings.ContainsRune(device.Alphabet, r) {
			t.Fatal("wrong alphabet")
		}
	}
	due(t, store)
	_, e := svc.Poll(ctx, auth.DeviceCode)
	codeError(t, e, "authorization_pending")
	preview, e := svc.Review(ctx, a.Account.ID, "192.0.2.2", auth.UserCode)
	if e != nil {
		t.Fatal(e)
	}
	due(t, store)
	_, e = svc.Poll(ctx, auth.DeviceCode)
	codeError(t, e, "authorization_pending")
	if e = svc.Decide(ctx, a.AccessToken, "192.0.2.2", preview.RequestID, "AAAA-AAAA", "approve"); e == nil {
		t.Fatal("wrong-code approval")
	}
	if e = svc.Decide(ctx, a.AccessToken, "192.0.2.2", preview.RequestID, auth.UserCode, "approve"); e != nil {
		t.Fatal(e)
	}
	due(t, store)
	session, e := svc.Poll(ctx, auth.DeviceCode)
	if e != nil {
		t.Fatal(e)
	}
	if session.Account.ID != a.Account.ID || len(session.Profiles) != 1 {
		t.Fatal("wrong account scope")
	}
	if _, e = ids.Authenticate(ctx, session.AccessToken); e != nil {
		t.Fatal(e)
	}
	var servers, devices int
	store.DB.QueryRow(`SELECT count(*) FROM servers`).Scan(&servers)
	store.DB.QueryRow(`SELECT count(*) FROM hosted_devices WHERE id IN(SELECT id FROM device_authorizations)`).Scan(&devices)
	if servers != 0 || devices != 1 {
		t.Fatalf("servers=%d devices=%d", servers, devices)
	}
}
func TestDeviceCreateRetryAndLostRedemptionRecoverOneSession(t *testing.T) {
	store, svc, ids, a, keys := activation(t)
	ctx := context.Background()
	auth := startTV(t, svc, 1)
	again := startTV(t, svc, 1)
	if auth.DeviceCode != again.DeviceCode || auth.UserCode != again.UserCode {
		t.Fatal("creation retry changed codes")
	}
	_, e := svc.Start(ctx, "192.0.2.1", device.Start{RequestID: "12345678-1234-4123-8123-000000000001", DeviceName: "Changed", Platform: "tvos", AppVersion: "0.1.0"})
	if !errors.Is(e, device.ErrConflict) {
		t.Fatalf("changed create retry %v", e)
	}
	approveTV(t, svc, a, auth)
	due(t, store)
	first, e := svc.Poll(ctx, auth.DeviceCode)
	if e != nil {
		t.Fatal(e)
	}
	restarted, e := device.New(store, keys, "https://web.getportico.tv/device", false)
	if e != nil {
		t.Fatal(e)
	}
	due(t, store)
	second, e := restarted.Poll(ctx, auth.DeviceCode)
	if e != nil || first.AccessToken != second.AccessToken {
		t.Fatalf("lost response changed credential: %v", e)
	}
	var n int
	store.DB.QueryRow(`SELECT count(*) FROM hosted_devices WHERE id IN(SELECT id FROM device_authorizations)`).Scan(&n)
	if n != 1 {
		t.Fatal("multiple devices")
	}
	store.DB.QueryRow(`SELECT count(*) FROM account_sessions`).Scan(&n)
	if n != 2 {
		t.Fatal("multiple device sessions")
	}
	if e = ids.Revoke(ctx, first.AccessToken); e != nil {
		t.Fatal(e)
	}
	due(t, store)
	_, e = restarted.Poll(ctx, auth.DeviceCode)
	codeError(t, e, "access_denied")
}
func TestDevicePollingSlowdownExpiryDenialAndPrivateSecret(t *testing.T) {
	store, svc, _, a, _ := activation(t)
	ctx := context.Background()
	auth := startTV(t, svc, 1)
	_, e := svc.Poll(ctx, auth.UserCode)
	codeError(t, e, "invalid_grant")
	_, e = svc.Poll(ctx, auth.DeviceCode)
	codeError(t, e, "slow_down")
	var first *device.ProtocolError
	errors.As(e, &first)
	if first.Interval != 10 {
		t.Fatal(first.Interval)
	}
	_, e = svc.Poll(ctx, auth.DeviceCode)
	codeError(t, e, "slow_down")
	var second *device.ProtocolError
	errors.As(e, &second)
	if second.Interval != 15 {
		t.Fatal(second.Interval)
	}
	p, e := svc.Review(ctx, a.Account.ID, "192.0.2.2", auth.UserCode)
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.Decide(ctx, a.AccessToken, "192.0.2.2", p.RequestID, p.UserCode, "deny"); e != nil {
		t.Fatal(e)
	}
	_, e = svc.Poll(ctx, auth.DeviceCode)
	codeError(t, e, "access_denied")
	if e = svc.Decide(ctx, a.AccessToken, "192.0.2.2", p.RequestID, p.UserCode, "approve"); !errors.Is(e, device.ErrConflict) {
		t.Fatal("denial reversed", e)
	}
	auth2 := startTV(t, svc, 2)
	if _, e = store.DB.Exec(`UPDATE device_authorizations SET expires_at=now()-interval '1 second' WHERE status='pending'`); e != nil {
		t.Fatal(e)
	}
	_, e = svc.Poll(ctx, auth2.DeviceCode)
	codeError(t, e, "expired_token")
}
func TestDeviceCancelFencesConsumedSessionAndPasswordChangeFencesApproval(t *testing.T) {
	store, svc, ids, a, _ := activation(t)
	ctx := context.Background()
	auth := startTV(t, svc, 1)
	approveTV(t, svc, a, auth)
	due(t, store)
	session, e := svc.Poll(ctx, auth.DeviceCode)
	if e != nil {
		t.Fatal(e)
	}
	if e = svc.Cancel(ctx, auth.DeviceCode); e != nil {
		t.Fatal(e)
	}
	if e = svc.Cancel(ctx, auth.DeviceCode); e != nil {
		t.Fatal(e)
	}
	if _, e = ids.Authenticate(ctx, session.AccessToken); !errors.Is(e, identity.ErrCredentials) {
		t.Fatal("cancelled session still live")
	}
	_, e = svc.Poll(ctx, auth.DeviceCode)
	codeError(t, e, "access_denied")
	auth2 := startTV(t, svc, 2)
	approveTV(t, svc, a, auth2)
	if e = ids.ChangePassword(ctx, a.Account, "StrongPass9", "ChangedPass8"); e != nil {
		t.Fatal(e)
	}
	due(t, store)
	_, e = svc.Poll(ctx, auth2.DeviceCode)
	codeError(t, e, "access_denied")
}
func TestConcurrentDeviceConsumptionOneEffect(t *testing.T) {
	store, svc, _, a, _ := activation(t)
	ctx := context.Background()
	auth := startTV(t, svc, 1)
	approveTV(t, svc, a, auth)
	due(t, store)
	var wg sync.WaitGroup
	errorsOut := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := svc.Poll(ctx, auth.DeviceCode); errorsOut <- e }()
	}
	wg.Wait()
	close(errorsOut)
	success := 0
	for e := range errorsOut {
		if e == nil {
			success++
		} else {
			codeError(t, e, "slow_down")
		}
	}
	if success != 1 {
		t.Fatal(success)
	}
	var count int
	store.DB.QueryRow(`SELECT count(*) FROM hosted_devices WHERE id IN(SELECT id FROM device_authorizations)`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate device")
	}
}
func TestDeviceHTTPApprovalAuthenticationAndTrustedProxyQuotas(t *testing.T) {
	store, svc, ids, a, _ := activation(t)
	prefixes, _ := httpapi.ParseTrustedProxies("127.0.0.1/32")
	h := (&httpapi.Handler{Identity: ids, Devices: svc, TrustedProxies: prefixes}).Routes()
	call := func(path, token, ip, body string, status int) []byte {
		t.Helper()
		r := httptest.NewRequest("POST", path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:3210"
		r.Header.Set("X-Forwarded-For", ip)
		r.Header.Set("Content-Type", "application/json")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s status %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	var auth device.Authorization
	raw := call("/v1/device-authorizations", "", "192.0.2.1", `{"requestId":"12345678-1234-4123-8123-000000000001","deviceName":"TV","platform":"tvos","appVersion":"0.1.0"}`, 201)
	json.Unmarshal(raw, &auth)
	call("/v1/device-authorizations/review", "", "192.0.2.2", `{"userCode":"`+auth.UserCode+`"}`, 401)
	raw = call("/v1/device-authorizations/review", a.AccessToken, "192.0.2.2", `{"userCode":"`+auth.UserCode+`"}`, 200)
	var preview device.Preview
	json.Unmarshal(raw, &preview)
	call("/v1/device-authorizations/decision", "", "192.0.2.2", `{"requestId":"`+preview.RequestID+`","userCode":"`+auth.UserCode+`","decision":"approve"}`, 401)
	call("/v1/device-authorizations/decision", a.AccessToken, "192.0.2.2", `{"requestId":"`+preview.RequestID+`","userCode":"`+auth.UserCode+`","decision":"approve"}`, 204)
	due(t, store)
	call("/v1/device-authorizations/token", "", "192.0.2.1", `{"deviceCode":"`+auth.DeviceCode+`"}`, 200)
	// Per-client quotas do not collapse distinct Caddy-forwarded clients into one loopback bucket.
	for n := 2; n <= 10; n++ {
		call("/v1/device-authorizations", "", "192.0.2.1", fmt.Sprintf(`{"requestId":"12345678-1234-4123-8123-%012d","deviceName":"TV","platform":"tvos","appVersion":"0.1.0"}`, n), 201)
	}
	call("/v1/device-authorizations", "", "192.0.2.1", `{"requestId":"12345678-1234-4123-8123-000000000011","deviceName":"TV","platform":"tvos","appVersion":"0.1.0"}`, 429)
	call("/v1/device-authorizations", "", "192.0.2.3", `{"requestId":"12345678-1234-4123-8123-000000000012","deviceName":"TV","platform":"tvos","appVersion":"0.1.0"}`, 201)
}

func TestDeviceApprovalRaceRecoveryExpiryAndCodeGuessQuota(t *testing.T) {
	store, svc, ids, a, _ := activation(t)
	ctx := context.Background()
	other, e := ids.Register(ctx, "otheruser", "Other", "StrongPass9")
	if e != nil {
		t.Fatal(e)
	}
	auth := startTV(t, svc, 1)
	preview, e := svc.Review(ctx, a.Account.ID, "192.0.2.2", auth.UserCode)
	if e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for _, account := range []identity.Session{a, other} {
		wg.Add(1)
		go func(account identity.Session) {
			defer wg.Done()
			out <- svc.Decide(ctx, account.AccessToken, "192.0.2.2", preview.RequestID, preview.UserCode, "approve")
		}(account)
	}
	wg.Wait()
	close(out)
	success := 0
	for e := range out {
		if e == nil {
			success++
		} else if !errors.Is(e, device.ErrConflict) {
			t.Fatal(e)
		}
	}
	if success != 1 {
		t.Fatal("approval race", success)
	}
	due(t, store)
	first, e := svc.Poll(ctx, auth.DeviceCode)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = store.DB.Exec(`UPDATE device_authorizations SET recovery_until=now()-interval '1 second'`); e != nil {
		t.Fatal(e)
	}
	due(t, store)
	_, e = svc.Poll(ctx, auth.DeviceCode)
	codeError(t, e, "invalid_grant")
	if _, e = ids.Authenticate(ctx, first.AccessToken); e != nil {
		t.Fatal("recovery expiry revoked valid session")
	}
	// Invalid guesses consume a durable account quota, including after service restart.
	for range 10 {
		_, e = svc.Review(ctx, other.Account.ID, "192.0.2.44", "not-a-code")
		codeError(t, e, "invalid_grant")
	}
	_, e = svc.Review(ctx, other.Account.ID, "192.0.2.44", auth.UserCode)
	codeError(t, e, "rate_limited")
}
