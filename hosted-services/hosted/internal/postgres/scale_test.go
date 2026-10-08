package postgres_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/security"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

// This opt-in local primitive workload is not a 200k-server fleet certification:
// it measures authenticated heartbeat lookups/writes against a 200k-row registry.
func TestScale200kAuthenticatedHeartbeat(t *testing.T) {
	if os.Getenv("PORTICO_HOSTED_SCALE_TEST") != "1" {
		t.Skip("opt-in 200k registry workload")
	}
	f := setup(t)
	ctx := context.Background()
	start := time.Now()
	// Each synthetic public key has its actual derived server ID and matching
	// installed claim. No production constraint or auth check is relaxed.
	_, e := f.store.DB.ExecContext(ctx, `WITH fleet AS (
 SELECT g,sha256(convert_to('fleet-key-'||g,'UTF8')) AS pub,
 sha256(convert_to('fleet-test-token-'||lpad(g::text,6,'0'),'UTF8')) AS token FROM generate_series(1,200000) g)
 INSERT INTO servers(id,owner_account_id,name,base_url,public_key,token_hash,token_ciphertext,claim_generation,credential_generation)
 SELECT 'srv_'||rtrim(translate(encode(sha256(convert_to('portico.server.identity.v1','UTF8')||decode('00','hex')||pub),'base64'),'+/','-_'),'='),$1,'Fleet fixture',NULL,
 rtrim(translate(encode(pub,'base64'),'+/','-_'),'='),token,decode(repeat('00',28),'hex'),1,1 FROM fleet`, f.owner.Account.ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.store.DB.ExecContext(ctx, `INSERT INTO claim_operations(id,server_id,account_id,public_key,local_generation,server_name,owner_profile_id,intent_digest,state,claim_generation,credential_generation,committed_token_hash,installed_at)
 SELECT 'fleet_'||substring(id FROM 5),id,owner_account_id,decode(translate(public_key,'-_','+/')||'=','base64'),0,name,$1,token_hash,'installed',1,1,token_hash,clock_timestamp()
 FROM servers WHERE name='Fleet fixture'`, f.owner.Profiles[0].ID)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.store.DB.ExecContext(ctx, `INSERT INTO wake_state(server_id,kind,desired_revision,acked_revision) SELECT id,'policy',1,1 FROM servers WHERE name='Fleet fixture'`)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.store.DB.ExecContext(ctx, `ANALYZE servers; ANALYZE wake_state`); e != nil {
		t.Fatal(e)
	}
	t.Logf("fixture seed duration=%s", time.Since(start))
	server := httptest.NewServer((&httpapi.Handler{Identity: f.ids, Directory: f.dir}).Routes())
	defer server.Close()
	const requests = 4000
	const concurrency = 32
	jobs := make(chan int, requests)
	for i := range requests {
		jobs <- i
	}
	close(jobs)
	latency := make(chan time.Duration, requests)
	failures := make(chan string, requests)
	var workers sync.WaitGroup
	start = time.Now()
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				// Spread the measured sample across the complete index, not only its first pages.
				id := 1 + (i*7919)%200000
				serverID, err := claims.ServerID(security.Hash(fmt.Sprintf("fleet-key-%d", id)))
				if err != nil {
					failures <- "fixture identity"
					continue
				}
				request, _ := http.NewRequest("POST", fmt.Sprintf("%s/v1/servers/%s/heartbeat", server.URL, serverID), strings.NewReader(`{"appliedPolicyRevision":1}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", fmt.Sprintf("Bearer fleet-test-token-%06d", id))
				began := time.Now()
				response, e := http.DefaultClient.Do(request)
				if e != nil {
					failures <- "transport"
					continue
				}
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				latency <- time.Since(began)
				if response.StatusCode != 200 {
					failures <- fmt.Sprint(response.StatusCode)
				}
			}
		}()
	}
	workers.Wait()
	elapsed := time.Since(start)
	close(latency)
	close(failures)
	for failure := range failures {
		t.Errorf("heartbeat failed: %s", failure)
	}
	values := make([]time.Duration, 0, requests)
	for d := range latency {
		values = append(values, d)
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	if len(values) != requests {
		t.Fatalf("got %d responses", len(values))
	}
	t.Logf("registry=200000 requests=%d concurrency=%d elapsed=%s throughput=%.1f/s p50=%s p95=%s p99=%s max=%s", requests, concurrency, elapsed, float64(requests)/elapsed.Seconds(), values[len(values)/2], values[len(values)*95/100], values[len(values)*99/100], values[len(values)-1])
}
