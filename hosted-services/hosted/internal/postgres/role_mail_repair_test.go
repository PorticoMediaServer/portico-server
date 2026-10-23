package postgres_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/mailoutbox"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"strings"
	"testing"
	"time"
)

func TestDirectoryRoleRepairActualHTTPProducer(t *testing.T) {
	f := registeredReturnFixture(t)
	f.handler = (&httpapi.Handler{Identity: identity.New(f.store), Directory: f.directory}).Routes()
	actual := map[string]json.RawMessage{}
	var last json.RawMessage
	call := func(token string) []map[string]any {
		r := httptest.NewRequest("GET", "/v1/servers", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatal("actual directory route", w.Code)
		}
		last = append(json.RawMessage(nil), w.Body.Bytes()...)
		var out struct {
			Servers []map[string]any `json:"items"`
		}
		var direct []map[string]any
		if json.Unmarshal(w.Body.Bytes(), &out) != nil || out.Servers == nil {
			if json.Unmarshal(w.Body.Bytes(), &direct) != nil {
				t.Fatal("directory JSON")
			}
			return direct
		}
		return out.Servers
	}
	const token = "strict-other-valid-access-token-fixture"
	if _, e := f.store.DB.Exec(`INSERT INTO account_sessions(token_hash,account_id,family_id,expires_at) VALUES($1,'strict-other','strict-other-family',clock_timestamp()+interval '15 minutes')`, security.Hash(token)); e != nil {
		t.Fatal(e)
	}
	owner := call("claim-owner-access-token-fixture")
	actual["owner"] = last
	if len(owner) != 1 || owner[0]["role"] != "owner" || owner[0]["ownedByMe"] != true {
		t.Fatal("owner projection")
	}
	for _, role := range []string{"admin", "member"} {
		if _, e := f.store.DB.Exec(`INSERT INTO server_members(server_id,account_id,role) VALUES($1,'strict-other',$2) ON CONFLICT(server_id,account_id) DO UPDATE SET role=EXCLUDED.role`, f.input.ServerID, role); e != nil {
			t.Fatal(e)
		}
		rows := call(token)
		actual[role] = last
		if len(rows) != 1 || rows[0]["role"] != role || rows[0]["ownedByMe"] != false {
			t.Fatal("member role projection")
		}
		for _, item := range rows {
			for _, value := range []any{item["lastSeenAt"], item["presence"].(map[string]any)["lastSeenAt"]} {
				if value != nil && !strings.HasSuffix(value.(string), "T00:00:00Z") {
					t.Fatal("member presence precision changed")
				}
			}
		}
		raw, _ := json.Marshal(rows)
		if bytes.Contains(raw, []byte("strict-owner")) || bytes.Contains(raw, []byte("@")) {
			t.Fatal("owner privacy")
		}
	}
	if _, e := f.store.DB.Exec(`DELETE FROM server_members WHERE server_id=$1 AND account_id='strict-other'`, f.input.ServerID); e != nil {
		t.Fatal(e)
	}
	if len(call(token)) != 0 {
		t.Fatal("missing membership still listed")
	}
	if _, e := f.store.DB.Exec(`UPDATE servers SET credential_generation=credential_generation+1 WHERE id=$1`, f.input.ServerID); e != nil {
		t.Fatal(e)
	}
	if len(call("claim-owner-access-token-fixture")) != 0 {
		t.Fatal("stale claim still listed")
	}
	if path := os.Getenv("PORTICO_REPAIR_ROLE_PRODUCER_JSON"); path != "" {
		raw, _ := json.Marshal(actual)
		if e := os.WriteFile(path, raw, 0600); e != nil {
			t.Fatal(e)
		}
	}

}
func TestInvitationMailRepairActualSchemaAndEligibility(t *testing.T) {
	f := currentClaimJourney(t)
	keys, e := fixtureKeys()
	if e != nil {
		t.Fatal(e)
	}
	m := &postgres.AccountMail{Store: f.store, Keys: keys}
	// The current format contains only four legitimate account-email kinds.
	for _, kind := range []string{"registration", "oidc_contact", "password_reset", "email_change", "invitation"} {
		_, e = f.store.DB.Exec(`INSERT INTO account_mail_outbox(id,kind,resource_id,revision,expires_at,ciphertext,payload_hash) VALUES($1,$2,'fixture',1,clock_timestamp()+interval '1 hour',$3,$3)`, "mail_schema_"+kind, kind, []byte("fixture"))
		if (e == nil) != (kind != "invitation") {
			t.Fatalf("actual schema kind %s: %v", kind, e)
		}
	}
	d := mailoutbox.Delivery{Intent: mailoutbox.Intent{ID: "mail_schema_registration", Kind: mailoutbox.Kind("invitation"), ResourceID: "fixture", Revision: 1, ExpiresAt: time.Now().Add(time.Hour), Payload: mailoutbox.Payload{To: "person@example.test", Code: "fixture"}}, Lease: "fixture-lease"}
	if _, e = f.store.DB.Exec(`UPDATE account_mail_outbox SET status='sending',lease_id='fixture-lease',lease_until=clock_timestamp()+interval '1 minute' WHERE id=$1`, d.ID); e != nil {
		t.Fatal(e)
	}
	allowed, e := m.MailEligible(context.Background(), d)
	if e != nil || allowed {
		t.Fatal("retired invitation mail eligible", e)
	}
	if d.Intent.Validate() == nil {
		t.Fatal("invitation validation")
	}
	raw, e := mailoutbox.Render(d, "sender@example.test", "")
	if e == nil || len(raw) > 0 {
		t.Fatal("invitation rendered")
	}
	if strings.Contains(mailoutbox.Link(d), "invite=") {
		t.Fatal("invitation email link")
	}
}
