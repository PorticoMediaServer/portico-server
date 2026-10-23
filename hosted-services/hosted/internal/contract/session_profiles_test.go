package contract_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"portico.local/hosted/internal/device"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/sessions"
)

func TestHostedSessionsContainIdentityOnly(t *testing.T) {
	store := database(t)
	ctx := context.Background()
	keys := &security.Keys{SealKey: bytes.Repeat([]byte{17}, 32)}
	ids := identity.New(store)
	account, err := ids.Register(ctx, "profilecontract", "Profiles", "StrongPass9")
	if err != nil {
		t.Fatal(err)
	}
	sec := &postgres.AccountSecurity{Store: store, Keys: keys, Identity: ids, Lifetime: 15 * time.Minute}
	devices, err := device.New(store, keys, "https://example.test/device", false)
	if err != nil {
		t.Fatal(err)
	}
	handler := (&httpapi.Handler{Identity: ids, Security: sec, Sessions: sessions.New(store, keys, 15*time.Minute), Devices: devices}).Routes()
	spec := openAPI(t)
	check := func(id string, body any) identity.Session {
		t.Helper()
		def := definition(t, id)
		input, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(def.Method, def.Path, bytes.NewReader(input))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Portico-Continuation", security.RandomID(""))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if response.Code != def.Status {
			t.Fatalf("%s: %d %s", id, response.Code, response.Body.String())
		}
		raw := response.Body.Bytes()
		var value any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&value); err != nil {
			t.Fatal(err)
		}
		if issues := conform(spec.response(t, def), value, "$"); len(issues) > 0 {
			t.Fatal(issues)
		}
		var result identity.Session
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		var envelope map[string]json.RawMessage
		if e := json.Unmarshal(raw, &envelope); e != nil {
			t.Fatal(e)
		}
		if _, exists := envelope["profiles"]; exists {
			t.Fatalf("%s exposed Hosted profiles: %s", id, raw)
		}
		if result.Account.ID != account.Account.ID {
			t.Fatalf("%s wrong account", id)
		}
		return result
	}
	login := check("session_create", httpapi.SignInRequest{Username: "profilecontract", Password: "StrongPass9", SessionMode: "native", RequestID: security.RandomID("")})
	check("session_refresh", httpapi.RefreshRequest{RefreshToken: login.RefreshToken, FamilyID: login.FamilyID, RequestID: "12345678-1234-4123-8123-000000000001"})
	auth, err := devices.Start(ctx, "192.0.2.1", device.Start{RequestID: "12345678-1234-4123-8123-000000000002", DeviceName: "TV", Platform: "tvos", AppVersion: "1"})
	if err != nil {
		t.Fatal(err)
	}
	review, err := devices.Review(ctx, account.Account.ID, "192.0.2.2", auth.UserCode)
	if err != nil {
		t.Fatal(err)
	}
	if err = devices.Decide(ctx, account.AccessToken, "192.0.2.2", review.RequestID, auth.UserCode, "approve"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(`UPDATE device_authorizations SET next_poll_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	check("device_authorization_token", httpapi.DeviceTokenRequest{DeviceCode: auth.DeviceCode})
}
