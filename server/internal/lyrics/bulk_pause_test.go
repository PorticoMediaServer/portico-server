package lyrics

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// A rate-limit answer is a pause the provider asked for, not a verdict on the
// song: the client says so, and sends nothing more until the pause has passed.
func TestProviderRateLimitIsReportedAsAPause(t *testing.T) {
	sent := 0
	p := &LRCLIB{origin: "https://lyrics.example", client: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		sent++
		return &http.Response{StatusCode: 429, Header: http.Header{"Retry-After": {"60"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	for i := 0; i < 3; i++ {
		if _, e := p.Search(context.Background(), "Alpha"); !errors.Is(e, errProviderPaused) || !errors.Is(e, ErrUnavailable) {
			t.Fatalf("search %d during a rate limit: %v", i, e)
		}
	}
	if sent != 1 {
		t.Fatalf("%d requests were sent during the provider's pause", sent)
	}
	// Any other failure is still an ordinary unavailable answer.
	p = &LRCLIB{origin: "https://lyrics.example", client: &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	})}}
	if _, e := p.Search(context.Background(), "Alpha"); !errors.Is(e, ErrUnavailable) || errors.Is(e, errProviderPaused) {
		t.Fatalf("a server error was reported as %v", e)
	}
}

// A library run that meets a provider pause waits on the song it was about to
// ask for. Nothing is counted missing, and the run finishes the library once
// the provider answers again.
func TestBulkLyricFetchWaitsOutAProviderPause(t *testing.T) {
	c, bulk, names := bulkFixture(t)
	db := c.DB
	ctx := context.Background()
	document, e := Parse([]byte("[00:01.00]First line\n[00:10.00]Second line"), "lrc")
	if e != nil {
		t.Fatal(e)
	}
	paused := true
	asked := 0
	bulk.provider = func(context.Context, string) ([]acquired, error) {
		asked++
		if paused {
			return nil, errProviderPaused
		}
		return []acquired{{doc: document, language: "en", provenance: Provenance{Origin: "lrclib", ProviderID: "7", Label: "Song", Rights: "LRCLIB record."}}}, nil
	}
	tx, e := db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = bulk.QueueTx(ctx, tx, "job", "library"); e != nil {
		t.Fatal(e)
	}
	if e = tx.Commit(); e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		more, e := bulk.Step(ctx)
		if !errors.Is(e, errProviderPaused) || more {
			t.Fatalf("step %d during the pause: more=%v %v", i, more, e)
		}
	}
	run, e := bulk.Observe(ctx, "job")
	if e != nil || run.State != "running" || run.Processed != 0 || run.Missing != 0 || !strings.Contains(run.Phase, "waiting") {
		t.Fatalf("run during the pause: %+v %v", run, e)
	}
	if asked != 3 {
		t.Fatalf("the provider was asked %d times for the one song the run is waiting on", asked)
	}
	paused = false
	for i := 0; i < 4; i++ {
		if _, e = bulk.Step(ctx); e != nil {
			t.Fatal(e)
		}
	}
	run, e = bulk.Observe(ctx, "job")
	if e != nil || run.State != "succeeded" || run.Published != 2 || run.Missing != 0 || run.Processed != 2 {
		t.Fatalf("run after the pause: %+v %v", run, e)
	}
	for _, name := range []string{"song-a", "song-b"} {
		var n int
		if e = db.QueryRow(`SELECT count(*) FROM lyric_resources WHERE item_id=? AND scope='library' AND deleted=0`, names[name].ID).Scan(&n); e != nil || n != 1 {
			t.Fatalf("%s has %d library lyrics after the run: %v", name, n, e)
		}
	}
}
