package contract_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestActiveSessionProfilesContract(t *testing.T) {
	store := database(t)
	ctx := context.Background()
	keys := &security.Keys{SealKey: bytes.Repeat([]byte{17}, 32)}
	ids := identity.New(store)
	account, err := ids.Register(ctx, "profilecontract", "Profiles", "StrongPass9")
	if err != nil {
		t.Fatal(err)
	}
	// Seed erased history before filling the active-profile capacity.
	if _, err = store.DB.Exec(`INSERT INTO profiles(id,account_id,name,deleted_at) SELECT 'deleted-'||n,$1,'Deleted profile',clock_timestamp() FROM generate_series(1,93) n`, account.Account.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		if _, err = store.DB.Exec(`INSERT INTO profiles(id,account_id,name) VALUES($1,$2,$3)`, fmt.Sprintf("active-%d", i), account.Account.ID, fmt.Sprintf("Profile %d", i)); err != nil {
			t.Fatal(err)
		}
	}
	// The insert trigger chooses positions; exercise a subsequently reordered list.
	if _, err = store.DB.Exec(`UPDATE profiles SET position=CASE WHEN id LIKE 'active-%' THEN substring(id from 8)::int ELSE 7 END WHERE account_id=$1 AND deleted_at IS NULL`, account.Account.ID); err != nil {
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
		if len(result.Profiles) != 8 {
			t.Fatalf("%s returned %d profiles", id, len(result.Profiles))
		}
		for i, p := range result.Profiles {
			if i < 7 && p.ID != fmt.Sprintf("active-%d", i) || i == 7 && p.ID != account.Profiles[0].ID {
				t.Fatalf("%s wrong profile/order: %+v", id, result.Profiles)
			}
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
	if err = devices.Decide(ctx, account.Account.ID, "192.0.2.2", review.RequestID, auth.UserCode, "approve"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.DB.Exec(`UPDATE device_authorizations SET next_poll_at=now()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	check("device_authorization_token", httpapi.DeviceTokenRequest{DeviceCode: auth.DeviceCode})
}
