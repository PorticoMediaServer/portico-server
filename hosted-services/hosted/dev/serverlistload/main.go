// Command serverlistload drives the version-check path, GET
// /v1/server-list/{watch}/{version}, the way a fleet of devices does, and reports the
// rate, the answers and the latency. It is the load test for the one Hosted
// route whose volume grows with every installed app (Spec — Hosted at Scale).
//
//	go run ./dev/serverlistload -url http://127.0.0.1:19410 -tokens tokens.txt -c 256 -d 60s
//
// tokens.txt holds one "watch version" pair per line, taken from GET
// /v1/servers answers (the watch and version fields) of real or seeded test
// accounts. A pair whose version is current is answered 304, which is what a
// device sees nearly always; add stale versions to exercise the 200 path.
//
// Each request is a fresh connection when -fresh is set, which is what a phone
// checking a few times a day costs (a full TLS handshake); without it,
// connections are reused and the run measures Hosted's own work.
package main

import (
	"bufio"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	base := flag.String("url", "http://127.0.0.1:19410", "Hosted origin")
	file := flag.String("tokens", "", `file of "watch version" lines`)
	workers := flag.Int("c", 64, "concurrent clients")
	duration := flag.Duration("d", 30*time.Second, "run time")
	fresh := flag.Bool("fresh", false, "new connection (and TLS handshake) per request")
	insecure := flag.Bool("k", false, "skip TLS verification (local test certificates)")
	flag.Parse()
	pairs, e := readPairs(*file)
	if e != nil || len(pairs) == 0 {
		fmt.Fprintln(os.Stderr, "need -tokens with at least one \"watch version\" line:", e)
		os.Exit(2)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: *insecure}, DisableKeepAlives: *fresh, MaxIdleConnsPerHost: *workers}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	var (
		mu        sync.Mutex
		latencies []time.Duration
		statuses  = map[int]int{}
		failures  atomic.Int64
		next      atomic.Int64
	)
	deadline := time.Now().Add(*duration)
	var wg sync.WaitGroup
	for range *workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var local []time.Duration
			localStatus := map[int]int{}
			for time.Now().Before(deadline) {
				p := pairs[int(next.Add(1))%len(pairs)]
				req, _ := http.NewRequest("GET", strings.TrimRight(*base, "/")+"/v1/server-list/"+p[0]+"/"+p[1], nil)
				start := time.Now()
				res, e := client.Do(req)
				if e != nil {
					failures.Add(1)
					continue
				}
				res.Body.Close()
				local = append(local, time.Since(start))
				localStatus[res.StatusCode]++
			}
			mu.Lock()
			latencies = append(latencies, local...)
			for k, v := range localStatus {
				statuses[k] += v
			}
			mu.Unlock()
		}()
	}
	wg.Wait()
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	n := len(latencies)
	fmt.Printf("requests %d in %s (%.0f/s), transport failures %d\n", n, *duration, float64(n)/duration.Seconds(), failures.Load())
	fmt.Printf("statuses %v\n", statuses)
	if n > 0 {
		at := func(q float64) time.Duration { return latencies[min(n-1, int(q*float64(n)))] }
		fmt.Printf("latency p50 %s p90 %s p99 %s max %s\n", at(0.5), at(0.9), at(0.99), latencies[n-1])
	}
	// The budget (Spec — Hosted at Scale, §6): ~300 checks a second at peak
	// for a million accounts, every one a 304 from memory.
}

func readPairs(name string) ([][2]string, error) {
	f, e := os.Open(name)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	var out [][2]string
	scan := bufio.NewScanner(f)
	for scan.Scan() {
		fields := strings.Fields(scan.Text())
		if len(fields) == 2 {
			out = append(out, [2]string{fields[0], fields[1]})
		}
	}
	return out, scan.Err()
}
