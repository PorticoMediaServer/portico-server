package app

import (
	"net/http"
	"net/http/cookiejar"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestCanonicalArtworkResizeBoundsVariantCardinality(t *testing.T) {
	tests := []struct {
		name                  string
		width, height         int
		wantWidth, wantHeight int
		wantOK                bool
	}{
		{name: "web poster", width: 160, height: 240, wantWidth: 160, wantHeight: 240, wantOK: true},
		{name: "near poster", width: 300, height: 450, wantWidth: 320, wantHeight: 480, wantOK: true},
		{name: "profile", width: 96, height: 96, wantWidth: 96, wantHeight: 96, wantOK: true},
		{name: "oversized square normalizes", width: 1000, height: 1000, wantWidth: 384, wantHeight: 384, wantOK: true},
		{name: "single dimension", width: 300, wantWidth: 320, wantOK: true},
		{name: "noncanonical aspect", width: 500, height: 300, wantOK: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			width, height, ok := canonicalArtworkResize(test.width, test.height)
			if ok != test.wantOK || width != test.wantWidth || height != test.wantHeight {
				t.Fatalf("canonicalArtworkResize(%d, %d) = (%d, %d, %v), want (%d, %d, %v)", test.width, test.height, width, height, ok, test.wantWidth, test.wantHeight, test.wantOK)
			}
		})
	}
}

func TestMissingArtworkResolutionCoalescesAndNegativeCachesConcurrentRequests(t *testing.T) {
	serverURL, _, server := newDiscoveryTestServer(t, config.Config{})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	started := make(chan struct{})
	release := make(chan struct{})
	var resolutions atomic.Int32
	server.artworkResolutionHook = func() {
		if resolutions.Add(1) == 1 {
			close(started)
		}
		<-release
	}

	const requestCount = 24
	statuses := make(chan int, requestCount)
	var requests sync.WaitGroup
	requests.Add(requestCount)
	for range requestCount {
		go func() {
			defer requests.Done()
			resp, err := client.Get(serverURL + "/api/artwork/movie_saffron/poster.svg?v=missing-cache-test")
			if err != nil {
				statuses <- 0
				return
			}
			_ = resp.Body.Close()
			statuses <- resp.StatusCode
		}()
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("artwork resolution did not start")
	}
	// Give every request time to join the same in-flight lookup.
	time.Sleep(25 * time.Millisecond)
	close(release)
	requests.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusNoContent {
			t.Fatalf("missing artwork status = %d", status)
		}
	}
	if got := resolutions.Load(); got != 1 {
		t.Fatalf("concurrent missing artwork performed %d resolutions, want 1", got)
	}

	resp, err := client.Get(serverURL + "/api/artwork/movie_saffron/poster.svg?v=missing-cache-test")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("negative-cache status = %d", resp.StatusCode)
	}
	if got := resolutions.Load(); got != 1 {
		t.Fatalf("negative-cache request performed another resolution; count=%d", got)
	}
}
