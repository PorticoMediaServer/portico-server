package postgres_test

// This private stdin driver joins the real Server producer to real Hosted
// handlers. Credentials and response bodies stay in the paired process pipe;
// the consumer must never copy its raw protocol into shared evidence.
import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
)

func TestOwnerContinuationActualProducerBridge(t *testing.T) {
	if os.Getenv("PORTICO_OWNER_CONTINUATION_BRIDGE") != "1" {
		t.Skip("private genuine owner producer/consumer driver")
	}
	f := currentFamilySetup(t)
	keys, e := fixtureKeys()
	if e != nil {
		t.Fatal(e)
	}
	service, e := claims.New(f.store, keys, "https://hosted.example")
	if e != nil {
		t.Fatal(e)
	}
	dir := directory.New(f.store, keys)
	claimHandler := &httpapi.ClaimHandler{Service: service, CaptureAuthority: f.store.CaptureClaimAuthority, Web: &httpapi.ClaimWeb{Store: f.store, Keys: keys, Audience: "https://hosted.example"}}
	handler := (&httpapi.Handler{Identity: identity.New(f.store), Directory: dir, Claims: claimHandler, ServerReturns: &httpapi.ServerReturns{Store: f.store, Directory: dir}}).Routes()
	// The token is newly generated in this isolated fixture and exists only in
	// memory and its ordinary hashed account-session row, never an artifact.
	token := security.RandomID("")
	if _, e = f.store.DB.Exec(`INSERT INTO account_sessions(token_hash,account_id,family_id,expires_at) VALUES($1,$2,$3,clock_timestamp()+interval '15 minutes')`, security.Hash(token), f.expected.AccountID, f.expected.FamilyID); e != nil {
		t.Fatal(e)
	}
	emit := func(v any) {
		raw, e := json.Marshal(v)
		if e != nil {
			t.Fatal("private reply encoding")
		}
		fmt.Println("PORTICO_OWNER_JSON:" + string(raw))
	}
	emit(map[string]any{"root": base64.RawURLEncoding.EncodeToString(keys.RootPublicKey), "accountId": f.expected.AccountID, "ownerToken": token, "audience": "https://hosted.example"})
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 4096), 128<<10)
	finalizes, exchanges := 0, 0
	for scanner.Scan() {
		var q struct {
			Action, Method, Path, Token, ServerID, Origin string
			Generation                                    int64
			Headers                                       http.Header
			Body                                          json.RawMessage
		}
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&q) != nil {
			t.Fatal("invalid private command")
		}
		switch q.Action {
		case "stop":
			return
		case "http":
			method := q.Method
			if method == "" {
				method = "POST"
			}
			request := httptest.NewRequest(method, q.Path, bytes.NewReader(q.Body))
			request.Header = q.Headers.Clone()
			if request.Header == nil {
				request.Header = make(http.Header)
			}
			if request.Header.Get("Content-Type") == "" {
				request.Header.Set("Content-Type", "application/json")
			}
			if q.Token != "" {
				request.Header.Set("Authorization", "Bearer "+q.Token)
			}
			if q.Path == "/v1/server-claims/finalize" {
				finalizes++
			}
			if q.Path == "/v1/server-returns/exchange" {
				exchanges++
			}
			out := httptest.NewRecorder()
			handler.ServeHTTP(out, request)
			body := json.RawMessage(out.Body.Bytes())
			if out.Body.Len() == 0 {
				body = nil
			} else if !json.Valid(body) {
				t.Fatal("non-JSON private handler response")
			}
			emit(map[string]any{"status": out.Code, "headers": out.Header(), "bodyEmpty": out.Body.Len() == 0, "body": body})
		case "snapshot":
			// Explicit select list: never emit hashes, signed bytes, credentials,
			// browser state, code challenge, installation id or response assertions.
			var raw []byte
			e = f.store.DB.QueryRow(`SELECT json_build_object(
    'server',(SELECT json_build_object('id',s.id,'accountId',s.owner_account_id,'claimGeneration',s.claim_generation,'credentialGeneration',s.credential_generation,'revoked',s.revoked_at IS NOT NULL) FROM servers s WHERE s.id=$1),
    'ownerStarts',COALESCE((SELECT json_agg(x ORDER BY x.operation_id) FROM(SELECT operation_id,phase,approved_account_id,claim_generation,credential_generation,route_generation,completed_at IS NOT NULL AS completed,consumed_at IS NOT NULL AS consumed FROM owner_claim_starts WHERE server_id=$1)x),'[]'::json),
    'operations',COALESCE((SELECT json_agg(x ORDER BY x.id) FROM(SELECT id,state,account_id,claim_generation,credential_generation FROM claim_operations WHERE server_id=$1)x),'[]'::json),
    'counts',json_build_object('claims',(SELECT count(*) FROM claim_operations WHERE server_id=$1),'cancellations',(SELECT count(*) FROM claim_cancellations c JOIN claim_operations o ON o.id=c.operation_id WHERE o.server_id=$1),'returns',(SELECT count(*) FROM server_returns WHERE server_id=$1),'ownerStarts',(SELECT count(*) FROM owner_claim_starts WHERE server_id=$1),'members',(SELECT count(*) FROM server_members WHERE server_id=$1),'routes',(SELECT count(*) FROM networking_route_state WHERE server_id=$1)))`, q.ServerID).Scan(&raw)
			if e != nil {
				t.Fatal(e)
			}
			emit(map[string]any{"status": 200, "body": json.RawMessage(raw), "finalizeRequests": finalizes, "exchangeRequests": exchanges})
		case "verified-route-fact":
			// This is an explicit verified HTTPS fixture observation, not an ACME,
			// public DNS or provisioning producer. No identity or claim is invented.
			if !serverreturn.Origin(q.Origin) || q.Generation < 1 {
				t.Fatal("invalid route fixture fact")
			}
			a, e := dir.CaptureServerAuthority(context.Background(), q.Token)
			if e != nil || a.ServerID != q.ServerID {
				t.Fatal("route requires actual installed credential")
			}
			tx, e := f.store.DB.BeginTx(context.Background(), nil)
			if e != nil {
				t.Fatal(e)
			}
			var id string
			e = tx.QueryRow(`SELECT s.id FROM accounts a JOIN servers s ON s.owner_account_id=a.id JOIN claim_operations o ON o.id=$2 AND o.server_id=s.id WHERE s.id=$1 AND s.owner_account_id=$3 AND s.token_hash=$4 AND s.claim_generation=$5 AND s.credential_generation=$6 AND s.revoked_at IS NULL AND o.state='installed' AND o.committed_token_hash=s.token_hash FOR UPDATE OF a,s FOR SHARE OF o`, a.ServerID, a.OperationID, a.AccountID, a.TokenHash, a.ClaimGeneration, a.CredentialGeneration).Scan(&id)
			if e != nil {
				tx.Rollback()
				t.Fatal("route authority changed")
			}
			result, e := tx.Exec(`INSERT INTO networking_route_state(server_id,operation_id,claim_generation,credential_generation,generation,observation_digest,remote_enabled) VALUES($1,$2,$3,$4,$5,$6,true) ON CONFLICT(server_id) DO UPDATE SET operation_id=EXCLUDED.operation_id,claim_generation=EXCLUDED.claim_generation,credential_generation=EXCLUDED.credential_generation,generation=EXCLUDED.generation,observation_digest=EXCLUDED.observation_digest,remote_enabled=true WHERE networking_route_state.generation<EXCLUDED.generation`, a.ServerID, a.OperationID, a.ClaimGeneration, a.CredentialGeneration, q.Generation, bytes.Repeat([]byte{9}, 32))
			if e != nil {
				tx.Rollback()
				t.Fatal(e)
			}
			n, _ := result.RowsAffected()
			if n != 1 {
				tx.Rollback()
				t.Fatal("route fixture must advance generation")
			}
			_, e = tx.Exec(`INSERT INTO networking_route_candidates(server_id,operation_id,generation,origin,class,state,verified_at) VALUES($1,$2,$3,$4,'public','verified',clock_timestamp())`, a.ServerID, a.OperationID, q.Generation, q.Origin)
			if e != nil {
				tx.Rollback()
				t.Fatal(e)
			}
			if e = tx.Commit(); e != nil {
				t.Fatal(e)
			}
			emit(map[string]any{"status": 200, "body": map[string]any{}})
		default:
			t.Fatal("unknown private action")
		}
	}
	if e = scanner.Err(); e != nil {
		t.Fatal("private command stream failed")
	}
}
