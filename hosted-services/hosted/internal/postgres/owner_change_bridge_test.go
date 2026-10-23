package postgres_test

// This private stdin protocol connects two real Go modules in the disposable
// producer/consumer regression. Replies are actual handler JSON, never invented
// identity or claim facts. It opens no app/browser/device listener.
import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"strings"
	"testing"
)

func TestOwnerChangeRepairBridge(t *testing.T) {
	if os.Getenv("PORTICO_OWNER_REPAIR_BRIDGE") != "1" {
		t.Skip("private real producer/consumer driver")
	}
	f := registeredReturnFixture(t)
	service, e := claims.New(f.store, f.keys, "https://hosted.example")
	if e != nil {
		t.Fatal(e)
	}
	claimHandler := &httpapi.ClaimHandler{Service: service, CaptureAuthority: f.store.CaptureClaimAuthority, Web: &httpapi.ClaimWeb{Store: f.store, Keys: f.keys, Audience: "https://hosted.example"}}
	handler := (&httpapi.Handler{Identity: identity.New(f.store), Directory: f.directory, Claims: claimHandler, ServerReturns: &httpapi.ServerReturns{Store: f.store, Directory: f.directory}}).Routes()
	const targetToken = "strict-other-valid-access-token-fixture"
	if _, e = f.store.DB.Exec(`INSERT INTO account_sessions(token_hash,account_id,family_id,expires_at) VALUES($1,'strict-other','strict-other-family',clock_timestamp()+interval '15 minutes')`, security.Hash(targetToken)); e != nil {
		t.Fatal(e)
	}
	emit := func(v any) { raw, _ := json.Marshal(v); fmt.Println("PORTICO_REPAIR_JSON:" + string(raw)) }
	emit(map[string]any{"root": base64.RawURLEncoding.EncodeToString(f.keys.RootPublicKey), "targetToken": targetToken})
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 128<<10)
	finalizes := 0
	for scanner.Scan() {
		var q struct {
			Action, Method, Path, Token, Operation string
			Body                                   json.RawMessage
		}
		if json.Unmarshal(scanner.Bytes(), &q) != nil {
			t.Fatal("bad private command")
		}
		switch q.Action {
		case "stop":
			return
		case "snapshot":
			var raw []byte
			e = f.store.DB.QueryRow(`SELECT json_build_object('owner',s.owner_account_id,'claim',s.claim_generation,'credential',s.credential_generation,'token',encode(s.token_hash,'hex'),'roles',(SELECT json_agg(x ORDER BY x.account_id) FROM (SELECT account_id,role FROM server_members WHERE server_id=s.id) x),'operations',(SELECT json_agg(x ORDER BY x.id) FROM (SELECT id,state,claim_generation,credential_generation FROM claim_operations WHERE server_id=s.id AND state IN('installed','committed_pending_install')) x),'routes',(SELECT count(*) FROM networking_route_state WHERE server_id=s.id)) FROM servers s WHERE s.id=$1`, f.input.ServerID).Scan(&raw)
			if e != nil {
				t.Fatal(e)
			}
			emit(map[string]any{"status": 200, "body": json.RawMessage(raw), "finalizes": finalizes})
		case "route":
			var claim, credential int64
			if e = f.store.DB.QueryRow(`SELECT claim_generation,credential_generation FROM claim_operations WHERE id=$1 AND state='installed'`, q.Operation).Scan(&claim, &credential); e != nil {
				t.Fatal(e)
			}
			_, e = f.store.DB.Exec(`INSERT INTO networking_route_state(server_id,operation_id,claim_generation,credential_generation,generation,observation_digest,remote_enabled) VALUES($1,$2,$3,$4,1,$5,true)`, f.input.ServerID, q.Operation, claim, credential, bytes.Repeat([]byte{2}, 32))
			if e != nil {
				t.Fatal(e)
			}
			_, e = f.store.DB.Exec(`INSERT INTO networking_route_candidates(server_id,operation_id,generation,origin,class,state,verified_at) VALUES($1,$2,1,'https://server.example:32500','public','verified',clock_timestamp())`, f.input.ServerID, q.Operation)
			if e != nil {
				t.Fatal(e)
			}
			emit(map[string]any{"status": 200, "body": map[string]any{}})
		case "withdraw-route":
			if _, e = f.store.DB.Exec(`DELETE FROM networking_route_state WHERE server_id=$1`, f.input.ServerID); e != nil {
				t.Fatal(e)
			}
			emit(map[string]any{"status": 200, "body": map[string]any{}})
		case "http":
			method := q.Method
			if method == "" {
				method = "POST"
			}
			r := httptest.NewRequest(method, q.Path, bytes.NewReader(q.Body))
			r.Header.Set("Content-Type", "application/json")
			if q.Token != "" {
				r.Header.Set("Authorization", "Bearer "+q.Token)
			}
			if strings.HasSuffix(q.Path, "/finalize") {
				finalizes++
			}
			out := httptest.NewRecorder()
			handler.ServeHTTP(out, r)
			body := json.RawMessage(out.Body.Bytes())
			if !json.Valid(body) {
				body = json.RawMessage(`{}`)
			}
			emit(map[string]any{"status": out.Code, "body": body})
		default:
			t.Fatal("unknown private command")
		}
	}
	if e = scanner.Err(); e != nil {
		t.Fatal(e)
	}
	_ = context.Background()
}
