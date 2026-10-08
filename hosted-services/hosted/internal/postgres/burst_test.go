package postgres_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/httpapi"
	"portico.local/hosted/internal/security"
)

// A recovery burst: far more simultaneous check-ins than Hosted has admission slots or
// database connections, as when it returns from an outage. What matters is HOW it fails.
// Work it accepts must finish quickly, and work it cannot take must be refused at once with a
// Retry-After, never left to queue for a connection until the request deadline.
//
//	PORTICO_HOSTED_BURST_TEST=1 PORTICO_TEST_DATABASE_URL_FILE=... go test ./internal/postgres -run TestRecoveryBurst -v
func TestRecoveryBurstIsShedFastNotQueuedToTimeout(t *testing.T) {
	if os.Getenv("PORTICO_HOSTED_BURST_TEST") != "1" {
		t.Skip("opt-in recovery burst workload")
	}
	f := setup(t)
	ctx := context.Background()
	const fleet = 20000
	if _, e := f.store.DB.ExecContext(ctx, `WITH fleet AS (
 SELECT g,sha256(convert_to('fleet-key-'||g,'UTF8')) AS pub,
 sha256(convert_to('fleet-test-token-'||lpad(g::text,6,'0'),'UTF8')) AS token FROM generate_series(1,$2::integer) g)
 INSERT INTO servers(id,owner_account_id,name,base_url,public_key,token_hash,token_ciphertext,claim_generation,credential_generation)
 SELECT 'srv_'||rtrim(translate(encode(sha256(convert_to('portico.server.identity.v1','UTF8')||decode('00','hex')||pub),'base64'),'+/','-_'),'='),$1,'Fleet fixture',NULL,
 rtrim(translate(encode(pub,'base64'),'+/','-_'),'='),token,decode(repeat('00',28),'hex'),1,1 FROM fleet`, f.owner.Account.ID, fleet); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.DB.ExecContext(ctx, `INSERT INTO claim_operations(id,server_id,account_id,public_key,local_generation,server_name,owner_profile_id,intent_digest,state,claim_generation,credential_generation,committed_token_hash,installed_at)
 SELECT 'fleet_'||substring(id FROM 5),id,owner_account_id,decode(translate(public_key,'-_','+/')||'=','base64'),0,name,$1,token_hash,'installed',1,1,token_hash,clock_timestamp()
 FROM servers WHERE name='Fleet fixture'`, f.owner.Profiles[0].ID); e != nil {
		t.Fatal(e)
	}
	if _, e := f.store.DB.ExecContext(ctx, `INSERT INTO wake_state(server_id,kind,desired_revision,acked_revision) SELECT id,'policy',1,1 FROM servers WHERE name='Fleet fixture'; ANALYZE servers; ANALYZE wake_state; ANALYZE claim_operations`); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer((&httpapi.Handler{Identity: f.ids, Directory: f.dir}).Routes())
	defer server.Close()
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 2048, MaxConnsPerHost: 0, DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext}}

	concurrency, _ := strconv.Atoi(os.Getenv("PORTICO_HOSTED_BURST_CONCURRENCY"))
	if concurrency < 1 {
		concurrency = 600
	}
	const requests = 12000
	type outcome struct {
		status  int
		elapsed time.Duration
		retry   string
	}
	results := make(chan outcome, requests)
	jobs := make(chan int, requests)
	for i := range requests {
		jobs <- i
	}
	close(jobs)
	var workers sync.WaitGroup
	began := time.Now()
	for range concurrency {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				id := 1 + (i*7919)%fleet
				serverID, _ := claims.ServerID(security.Hash(fmt.Sprintf("fleet-key-%d", id)))
				request, _ := http.NewRequest("POST", fmt.Sprintf("%s/v1/servers/%s/heartbeat", server.URL, serverID), strings.NewReader(`{"appliedPolicyRevision":1}`))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", fmt.Sprintf("Bearer fleet-test-token-%06d", id))
				start := time.Now()
				response, e := client.Do(request)
				if e != nil {
					results <- outcome{status: -1, elapsed: time.Since(start), retry: e.Error()}
					continue
				}
				io.Copy(io.Discard, response.Body)
				response.Body.Close()
				results <- outcome{response.StatusCode, time.Since(start), response.Header.Get("Retry-After")}
			}
		}()
	}
	workers.Wait()
	wall := time.Since(began)
	close(results)
	by := map[int][]time.Duration{}
	missingRetry := 0
	transport := map[string]int{}
	for r := range results {
		if r.status == -1 {
			parts := strings.Split(r.retry, ": ")
			transport[parts[len(parts)-1]]++
		}
		by[r.status] = append(by[r.status], r.elapsed)
		if r.status == 429 && r.retry == "" {
			missingRetry++
		}
	}
	quantile := func(v []time.Duration, q float64) time.Duration {
		if len(v) == 0 {
			return 0
		}
		sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
		return v[min(len(v)-1, int(float64(len(v))*q))]
	}
	for status, v := range by {
		t.Logf("status=%d count=%d p50=%s p99=%s max=%s", status, len(v), quantile(v, 0.5), quantile(v, 0.99), quantile(v, 1))
	}
	t.Logf("transport errors: %v", transport)
	t.Logf("concurrency=%d requests=%d wall=%s accepted=%.0f/s", concurrency, requests, wall, float64(len(by[200]))/wall.Seconds())
	// Hundreds of brand-new connections in one instant overflow the test listener's accept
	// backlog (128 on macOS) and are reset by the kernel before Hosted sees them. That is the
	// harness, not the service, and a real deployment sits behind a connection-pooling edge.
	resets := transport["connection reset by peer"] + transport["EOF"]
	if resets > requests/20 {
		t.Errorf("%d connections were reset", resets)
	}
	if n := len(by[-1]) - resets + len(by[500]) + len(by[502]) + len(by[503]) + len(by[504]); n > 0 {
		t.Errorf("%d requests failed as errors or timeouts rather than being refused: %v", n, transport)
	}
	if missingRetry > 0 {
		t.Errorf("%d refusals carried no Retry-After", missingRetry)
	}
	if p99 := quantile(by[200], 0.99); p99 > 2*time.Second {
		t.Errorf("accepted work took p99=%s: it queued for a connection instead of being refused", p99)
	}
	if p99 := quantile(by[429], 0.99); p99 > 250*time.Millisecond {
		t.Errorf("refusals took p99=%s: shedding must be immediate", p99)
	}
	if len(by[200]) == 0 {
		t.Error("nothing was accepted")
	}
}
