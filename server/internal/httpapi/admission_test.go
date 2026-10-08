package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
)

// Every route the router registers has to be classified, or the lane table is
// only a suggestion. This walks the same source the OpenAPI coverage test does
// and fails when a route has been added without deciding what kind of load it is.
func TestEveryRegisteredRouteHasALane(t *testing.T) {
	// Routes are registered directly on the mux and through small local wrappers
	// that add a header before delegating; both have to be classified.
	pattern := regexp.MustCompile(`(?:mux\.(?:Handle|HandleFunc)|\bhandle)\("((?:[A-Z]+ )?/[^"]+)"`)
	var missing []string
	for _, dir := range []string{".", "../networking"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if developmentOnlySource(data) {
				// A development-only file classifies its own routes when it is built.
				continue
			}
			for _, match := range pattern.FindAllStringSubmatch(string(data), -1) {
				if _, ok := routeLanes[match[1]]; !ok {
					missing = append(missing, match[1])
				}
			}
		}
	}
	registry, err := FoundationRegistry(Dependencies{})
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range registry.Definitions() {
		key := route.Method + " " + route.Path
		if _, ok := routeLanes[key]; !ok {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		t.Fatalf("these routes are registered but not classified in routeLanes:\n%s", strings.Join(missing, "\n"))
	}
}

func TestLaneSpecsAreCoherent(t *testing.T) {
	for _, name := range []string{laneMediaBody, laneBulkTransfer, laneRealtime} {
		spec := laneSpecs[name]
		// A stream or a byte range must fail fast rather than sit in a queue: the
		// slot it is waiting for is one it would be holding open itself.
		if spec.queueWait != 0 {
			t.Fatalf("lane %s queues; it must be refused immediately", name)
		}
		// These legitimately run as long as the client keeps reading, so a request
		// deadline would cut off a healthy download or a live stream.
		if spec.budget != 0 {
			t.Fatalf("lane %s has a request deadline; long-lived transfers must not", name)
		}
		if spec.pressure {
			t.Fatalf("lane %s counts as foreground load; a long transfer would then starve every scan", name)
		}
	}
	for _, name := range []string{laneSecurityFence, lanePlayback} {
		spec := laneSpecs[name]
		// Protected work waits far longer for a slot than ordinary traffic: a
		// viewer mid-playback and a revocation both deserve more than 1.5 seconds.
		if spec.queueWait < 9*time.Second {
			t.Fatalf("lane %s queues only %s; protected classes wait longer", name, spec.queueWait)
		}
	}
	for _, name := range []string{laneBrowsing, laneExpensive, laneAuth, laneAdmin, laneDefault, laneMedia} {
		spec := laneSpecs[name]
		if spec.queueWait != 1500*time.Millisecond {
			t.Fatalf("lane %s has an unexpected interactive queue wait %s", name, spec.queueWait)
		}
		if spec.budget <= 0 {
			t.Fatalf("lane %s has no request budget", name)
		}
	}
	for name, spec := range laneSpecs {
		if spec.capacity < 1 {
			t.Fatalf("lane %s has no capacity", name)
		}
		if !spec.class.Valid() {
			t.Fatalf("lane %s has no work class", name)
		}
	}
}

func TestAdmissionRefusesOverCapacityWithATypedAnswer(t *testing.T) {
	a := newAdmission()
	mux := http.NewServeMux()
	release := make(chan struct{})
	var served sync.WaitGroup
	mux.HandleFunc("GET /v1/search", func(w http.ResponseWriter, r *http.Request) {
		served.Done()
		<-release
	})
	handler := a.wrap(mux, mux)
	capacity := laneSpecs[laneExpensive].capacity
	served.Add(capacity)
	for i := 0; i < capacity; i++ {
		go func() {
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/search?q=x", nil))
		}()
	}
	served.Wait()
	// The lane is full. The next request waits out its queue budget and is then
	// refused with an answer a client can act on, not a hung connection.
	start := time.Now()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/v1/search?q=y", nil))
	elapsed := time.Since(start)
	if w.Code != 503 {
		t.Fatalf("an over-capacity request returned %d", w.Code)
	}
	if w.Header().Get("Retry-After") != "1" {
		t.Fatalf("no Retry-After on the refusal: %#v", w.Header())
	}
	if !strings.Contains(w.Body.String(), "server_busy") {
		t.Fatalf("refusal was not typed: %s", w.Body.String())
	}
	if elapsed < laneSpecs[laneExpensive].queueWait {
		t.Fatalf("the request was refused after %s without waiting its queue budget", elapsed)
	}
	close(release)
}

func TestStreamsAreRefusedImmediatelyRatherThanQueued(t *testing.T) {
	a := newAdmission()
	mux := http.NewServeMux()
	release := make(chan struct{})
	var served sync.WaitGroup
	mux.HandleFunc("GET /v1/notifications/events", func(w http.ResponseWriter, r *http.Request) {
		served.Done()
		<-release
	})
	handler := a.wrap(mux, mux)
	// Shrink the lane so the test does not have to open 512 streams.
	a.lanes[laneRealtime] = newLane(laneRealtime, laneSpec{capacity: 1, queueWait: 0, budget: 0, class: dbwork.ClassInteractive})
	served.Add(1)
	go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/notifications/events", nil))
	served.Wait()
	start := time.Now()
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/v1/notifications/events", nil))
	if w.Code != 503 {
		t.Fatalf("a stream over capacity returned %d", w.Code)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("a stream waited %s before being refused; it must fail fast", time.Since(start))
	}
	close(release)
}

func TestAdmissionPutsTheLaneClassInTheContext(t *testing.T) {
	a := newAdmission()
	mux := http.NewServeMux()
	var seen dbwork.Class
	var deadline bool
	mux.HandleFunc("POST /v1/playback/sessions", func(w http.ResponseWriter, r *http.Request) {
		seen = dbwork.ClassFrom(r.Context(), dbwork.ClassMaintenance)
		_, deadline = r.Context().Deadline()
	})
	handler := a.wrap(mux, mux)
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", "/v1/playback/sessions", nil))
	if seen != dbwork.ClassEstablishedPlayback {
		t.Fatalf("the playback lane's class did not reach the handler: %s", seen)
	}
	if !deadline {
		t.Fatal("the playback lane's request budget did not reach the handler")
	}
	// A media body has no deadline: it runs as long as the client keeps reading.
	var bodyDeadline bool
	mux.HandleFunc("GET /v1/media/{grant}", func(w http.ResponseWriter, r *http.Request) {
		_, bodyDeadline = r.Context().Deadline()
	})
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/media/token", nil))
	if bodyDeadline {
		t.Fatal("a media body was given a request deadline")
	}
}

func TestSearchGovernorCapsPerViewerAndGlobally(t *testing.T) {
	a := newAdmission()
	var releases []func()
	for i := 0; i < maxSearchesPerViewer; i++ {
		done, ok := a.acquireSearch("viewer")
		if !ok {
			t.Fatalf("search %d was refused below the per-viewer limit", i)
		}
		releases = append(releases, done)
	}
	if _, ok := a.acquireSearch("viewer"); ok {
		t.Fatal("one viewer exceeded the per-viewer search limit")
	}
	// A different viewer is unaffected: the cap exists so one client cannot take
	// the whole expensive lane, not to make search rare.
	other, ok := a.acquireSearch("other-viewer")
	if !ok {
		t.Fatal("a second viewer was refused because the first was busy")
	}
	other()
	for _, release := range releases {
		release()
		release()
	}
	if _, ok = a.acquireSearch("viewer"); !ok {
		t.Fatal("released search slots were not reusable")
	}
	if a.searchDenied.Load() == 0 {
		t.Fatal("refusals were not counted for diagnostics")
	}
}

func TestForegroundPressureIsPublishedToBackgroundLoops(t *testing.T) {
	dbwork.ResetProbes()
	t.Cleanup(dbwork.ResetProbes)
	a := newAdmission()
	a.registerPressure()
	if dbwork.ForegroundWorkActive() {
		t.Fatal("an idle server reported foreground pressure")
	}
	mux := http.NewServeMux()
	inside := make(chan struct{})
	release := make(chan struct{})
	mux.HandleFunc("GET /v1/items", func(w http.ResponseWriter, r *http.Request) {
		close(inside)
		<-release
	})
	handler := a.wrap(mux, mux)
	go handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/v1/items", nil))
	<-inside
	if !dbwork.ForegroundWorkActive() {
		t.Fatal("a browse request in flight did not register as foreground pressure")
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !dbwork.ForegroundWorkActive() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("foreground pressure stayed set after the request finished")
}

func TestDiagnosticsExposeEveryLane(t *testing.T) {
	a := newAdmission()
	report := a.diagnostics()
	if len(report) != len(laneSpecs) {
		t.Fatalf("diagnostics reported %d lanes, there are %d", len(report), len(laneSpecs))
	}
	for _, lane := range report {
		if lane.Capacity != laneSpecs[lane.Lane].capacity {
			t.Fatalf("lane %s reported capacity %d", lane.Lane, lane.Capacity)
		}
		if lane.WorkClass == "" {
			t.Fatalf("lane %s reported no work class", lane.Lane)
		}
	}
}

// developmentOnlySource reports whether a Go file is built only with the devtrust tag.
func developmentOnlySource(data []byte) bool {
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.HasPrefix(line, "//go:build ") && strings.Contains(line, "devtrust") && !strings.Contains(line, "!devtrust")
}
