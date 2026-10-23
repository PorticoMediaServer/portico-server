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
	"os/exec"
	"strings"
	"testing"
	"time"

	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/identity"
	"portico.local/hosted/internal/postgres"
	"portico.local/hosted/internal/security"
	"portico.local/hosted/internal/serverreturn"
	"portico.local/hosted/internal/sessions"
)

// Execute the actual stdin driver with a fresh current-format schema. Its initial
// credential and every raw HTTP response stay in the child pipe, including on
// failure. This qualifies the driver lifecycle, not a Server-produced owner pair.
func TestOwnerContinuationBridgeLifecycle(t *testing.T) {
	if os.Getenv("PORTICO_TEST_DATABASE_URL_FILE") == "" {
		t.Skip("isolated PostgreSQL is required")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal("bridge executable unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, executable, "-test.run=^TestOwnerContinuationActualProducerBridge$", "-test.count=1")
	child.Env = append(os.Environ(), "PORTICO_OWNER_CONTINUATION_BRIDGE=1")
	stdin, err := child.StdinPipe()
	if err != nil {
		t.Fatal("bridge input unavailable")
	}
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal("bridge output unavailable")
	}
	// Discard stderr rather than risking credentials in a failure report.
	if err = child.Start(); err != nil {
		t.Fatal("bridge failed to start")
	}
	stopped := false
	defer func() {
		if !stopped {
			_, _ = stdin.Write([]byte("{\"Action\":\"stop\"}\n"))
			_ = stdin.Close()
			_ = child.Wait()
		}
	}()
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 4096), 128<<10)
	read := func(out any) {
		t.Helper()
		for scanner.Scan() {
			const prefix = "PORTICO_OWNER_JSON:"
			if strings.HasPrefix(scanner.Text(), prefix) {
				if json.Unmarshal([]byte(strings.TrimPrefix(scanner.Text(), prefix)), out) != nil {
					t.Fatal("invalid private bridge reply")
				}
				return
			}
		}
		t.Fatal("private bridge reply unavailable")
	}
	send := func(value any) {
		t.Helper()
		if json.NewEncoder(stdin).Encode(value) != nil {
			t.Fatal("private bridge command failed")
		}
	}
	var hello struct{ Root, AccountID, OwnerToken, Audience string }
	read(&hello)
	root, err := base64.RawURLEncoding.DecodeString(hello.Root)
	if err != nil || len(root) != 32 || len(hello.OwnerToken) != 43 || hello.AccountID == "" || hello.Audience != "https://hosted.example" {
		t.Fatal("invalid bridge initialization")
	}
	type reply struct {
		Status                             int
		Headers                            http.Header
		Body                               json.RawMessage
		BodyEmpty                          bool
		FinalizeRequests, ExchangeRequests int
	}
	send(map[string]any{"Action": "snapshot"})
	var snapshot reply
	read(&snapshot)
	var initial struct {
		Server      any
		OwnerStarts []any
		Operations  []any
		Counts      map[string]int
	}
	if json.Unmarshal(snapshot.Body, &initial) != nil || snapshot.Status != 200 || initial.Server != nil || len(initial.OwnerStarts) != 0 || len(initial.Operations) != 0 || len(initial.Counts) != 6 || snapshot.FinalizeRequests != 0 || snapshot.ExchangeRequests != 0 {
		t.Fatal("bridge did not start with an empty owner snapshot")
	}
	for _, count := range initial.Counts {
		if count != 0 {
			t.Fatal("bridge invented owner state during initialization")
		}
	}
	request := func(method, path string) reply {
		t.Helper()
		send(map[string]any{"Action": "http", "Method": method, "Path": path, "Token": hello.OwnerToken, "Body": map[string]any{}})
		var result reply
		read(&result)
		if result.Headers.Get("Cache-Control") != "no-store" || result.Headers.Get("X-Content-Type-Options") != "nosniff" {
			t.Fatal("bridge lost actual HTTP response headers")
		}
		return result
	}
	if request(http.MethodGet, "/v1/me").Status != 200 {
		t.Fatal("bridge current family was not authenticated")
	}
	logout := request(http.MethodPost, "/v1/sessions/logout-all")
	if logout.Status != 204 || !logout.BodyEmpty || string(logout.Body) != "null" {
		t.Fatal("bridge did not preserve the actual empty revocation response")
	}
	if request(http.MethodGet, "/v1/me").Status != 401 {
		t.Fatal("bridge family remained authenticated after canonical revocation")
	}
	send(map[string]any{"Action": "stop"})
	_ = stdin.Close()
	if child.Wait() != nil {
		t.Fatal("bridge did not stop cleanly")
	}
	stopped = true
}

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
	ids := identity.New(f.store)
	handler := (&httpapi.Handler{
		Identity:  ids,
		Security:  &postgres.AccountSecurity{Store: f.store, Keys: keys, Identity: ids, Lifetime: 15 * time.Minute},
		Sessions:  sessions.New(f.store, keys, 15*time.Minute),
		Directory: dir, Claims: claimHandler,
		ServerReturns: &httpapi.ServerReturns{Store: f.store, Directory: dir},
	}).Routes()
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
