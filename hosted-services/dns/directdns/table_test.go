package directdns

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestTableSurvivesRestartAndAppliesInOrder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "table.json")
	a, _ := NewNamespace()
	b, _ := NewNamespace()
	table, e := OpenTable(path)
	if e != nil || table.Len() != 0 {
		t.Fatal(table, e)
	}
	if e = table.Apply([]FeedRow{{Sequence: 4, Namespace: a, IPv4: "156.57.135.50"}, {Sequence: 7, Namespace: b, IPv6: "2606:4700:4700::1111"}}); e != nil {
		t.Fatal(e)
	}
	// A replayed or older row never moves an address backwards.
	if e = table.Apply([]FeedRow{{Sequence: 3, Namespace: a, IPv4: "8.8.8.8"}}); e != nil {
		t.Fatal(e)
	}
	if got, _ := table.Lookup(a); got.IPv4.String() != "156.57.135.50" || table.Sequence() != 7 {
		t.Fatal(got, table.Sequence())
	}
	// Private, loopback and malformed addresses are never answerable; a row with none is removed.
	if e = table.Apply([]FeedRow{{Sequence: 8, Namespace: b, IPv4: "192.168.1.4", IPv6: "::1"}, {Sequence: 9, Namespace: "not-a-namespace", IPv4: "8.8.8.8"}}); e != nil {
		t.Fatal(e)
	}
	if _, ok := table.Lookup(b); ok || table.Len() != 1 {
		t.Fatal("an unanswerable row stayed in the table")
	}
	reopened, e := OpenTable(path)
	if e != nil || reopened.Sequence() != 9 || reopened.Len() != 1 {
		t.Fatal(reopened, e)
	}
	if got, ok := reopened.Lookup(a); !ok || got.IPv4.String() != "156.57.135.50" {
		t.Fatal("a restarted node forgot what it knew", got, ok)
	}
}

func TestFeedResumesFromItsSequenceAndRequiresTheSecret(t *testing.T) {
	ns, _ := NewNamespace()
	var asked atomic.Value
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer feed-secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		asked.Store(r.URL.Query().Get("after"))
		items := []FeedRow{}
		if r.URL.Query().Get("after") == "0" {
			items = append(items, FeedRow{Sequence: 12, Namespace: ns, Label: "0123456789abcdef0123456789abcdef", IPv4: "156.57.135.50"})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer hosted.Close()
	table, _ := OpenTable("")
	feed := &Feed{URL: hosted.URL, Secret: "wrong", Table: table}
	if _, e := feed.once(context.Background()); e == nil {
		t.Fatal("the feed was read without the secret")
	}
	feed.Secret = "feed-secret"
	if more, e := feed.once(context.Background()); e != nil || more {
		t.Fatal(more, e)
	}
	if got, ok := table.Lookup(ns); !ok || got.IPv4.String() != "156.57.135.50" {
		t.Fatal(got, ok)
	}
	if _, e := feed.once(context.Background()); e != nil || asked.Load() != "12" {
		t.Fatal("the next read did not resume after the applied sequence", asked.Load(), e)
	}
}

func TestFeedKeepsAnsweringWhileHostedIsDown(t *testing.T) {
	ns, _ := NewNamespace()
	table, _ := OpenTable("")
	_ = table.Apply([]FeedRow{{Sequence: 1, Namespace: ns, Label: "0123456789abcdef0123456789abcdef", IPv4: "156.57.135.50"}})
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	(&Feed{URL: "http://127.0.0.1:1", Secret: "s", Interval: 10 * time.Millisecond, Table: table}).Run(ctx)
	if got, ok := table.Lookup(ns); !ok || got.IPv4.String() != "156.57.135.50" {
		t.Fatal("an unreachable Hosted emptied the table")
	}
}

// A server that loses one address family withdraws it, and the feed carries that as a row
// whose family is blank. The table has to stop answering it and has to remember that across a
// restart — otherwise a reopened node resurrects an address nothing listens on and every
// dual-stack client tries it first.
func TestAWithdrawnFamilyIsDroppedAndStaysDroppedAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "table.json")
	ns, _ := NewNamespace()
	gone, _ := NewNamespace()
	table, e := OpenTable(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = table.Apply([]FeedRow{
		{Sequence: 2, Namespace: ns, Label: "0123456789abcdef0123456789abcdef", IPv4: "156.57.135.50", IPv6: "2606:4700:4700::1111"},
		{Sequence: 3, Namespace: gone, IPv4: "8.8.8.8"},
	}); e != nil {
		t.Fatal(e)
	}
	if got, _ := table.Lookup(ns); !got.IPv6.IsValid() {
		t.Fatal("the IPv6 address was never installed")
	}
	// The ISP stops handing out IPv6: the row arrives with ipv6 blank and ipv4 unchanged.
	if e = table.Apply([]FeedRow{{Sequence: 5, Namespace: ns, Label: "0123456789abcdef0123456789abcdef", IPv4: "156.57.135.50"}}); e != nil {
		t.Fatal(e)
	}
	got, ok := table.Lookup(ns)
	if !ok || got.IPv6.IsValid() || got.IPv4.String() != "156.57.135.50" {
		t.Fatal("a withdrawn AAAA is still answerable", got, ok)
	}
	// Withdrawing the last family removes the name entirely rather than leaving an empty one.
	if e = table.Apply([]FeedRow{{Sequence: 6, Namespace: gone}}); e != nil {
		t.Fatal(e)
	}
	if _, ok = table.Lookup(gone); ok {
		t.Fatal("a server with no addresses left is still in the table")
	}
	reopened, e := OpenTable(path)
	if e != nil || reopened.Sequence() != 6 || reopened.Len() != 1 {
		t.Fatal(reopened, e)
	}
	if got, ok = reopened.Lookup(ns); !ok || got.IPv6.IsValid() || got.IPv4.String() != "156.57.135.50" {
		t.Fatal("a restart resurrected a withdrawn address", got, ok)
	}
	if _, ok = reopened.Lookup(gone); ok {
		t.Fatal("a restart resurrected a fully withdrawn server")
	}
}

// The DNS answer after a withdrawal: AAAA is an empty NOERROR, not a dead address, and A
// still answers. A client with both families then goes straight to the one that works.
func TestAWithdrawnFamilyAnswersEmptyNotDead(t *testing.T) {
	ns, _ := NewNamespace()
	table, _ := OpenTable("")
	if e := table.Apply([]FeedRow{{Sequence: 1, Namespace: ns, Label: "0123456789abcdef0123456789abcdef", IPv4: "156.57.135.50", IPv6: "2606:4700:4700::1111"}}); e != nil {
		t.Fatal(e)
	}
	if e := table.Apply([]FeedRow{{Sequence: 2, Namespace: ns, Label: "0123456789abcdef0123456789abcdef", IPv4: "156.57.135.50"}}); e != nil {
		t.Fatal(e)
	}
	server, e := New(Config{Zone: "direct.getportico.tv", ACMEDelegationZone: "acme.getportico.tv", Table: table})
	if e != nil {
		t.Fatal(e)
	}
	name := CurrentHostname(ns, "direct.getportico.tv", "0123456789abcdef0123456789abcdef")
	for _, tc := range []struct {
		qtype   uint16
		answers int
	}{{dns.TypeAAAA, 0}, {dns.TypeA, 1}} {
		m := new(dns.Msg)
		m.SetQuestion(name+".", tc.qtype)
		w := new(recorder)
		server.handleDNS(w, m)
		if w.msg.Rcode != dns.RcodeSuccess || len(w.msg.Answer) != tc.answers {
			t.Fatalf("qtype %d: %v", tc.qtype, w.msg)
		}
	}
}

// Hosted contact must be cheap and rare: the default poll interval is 30 s with up to 30 s
// of jitter (30–60 s), so a newly claimed server still resolves within about a minute while
// two nodes cost ~16 requests per 5 minutes instead of ~120.
func TestFeedDefaultIntervalIsThirtySeconds(t *testing.T) {
	if feedDefaultInterval != 30*time.Second {
		t.Fatal("default interval moved", feedDefaultInterval)
	}
}

func TestFeedETagRoundTrip(t *testing.T) {
	for _, seq := range []int64{0, 1, 42, 9000} {
		etag := FeedETag(seq)
		if parsed, ok := parseFeedETag(etag); !ok || parsed != seq {
			t.Fatal("etag did not round-trip", seq, etag)
		}
		if !MatchFeedETag(etag, seq) || !MatchFeedETag("W/"+etag+", \"999\"", seq) {
			t.Fatal("etag did not match", seq, etag)
		}
		if MatchFeedETag(etag, seq+1) || MatchFeedETag("", seq) || MatchFeedETag("*", seq) {
			t.Fatal("etag matched what it should not", seq, etag)
		}
	}
}

// The feed sends its sequence as If-None-Match and treats a 304 as a synced empty page:
// no table change, no error, still counts as a successful sync for readiness.
func TestFeedSendsConditionalRequestAndAcceptsNotModified(t *testing.T) {
	ns, _ := NewNamespace()
	var seen []string
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("If-None-Match"))
		if r.URL.Query().Get("after") != "0" {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []FeedRow{{Sequence: 12, Namespace: ns, Label: "0123456789abcdef0123456789abcdef", IPv4: "156.57.135.50"}}})
	}))
	defer hosted.Close()
	table, _ := OpenTable("")
	feed := &Feed{URL: hosted.URL, Secret: "s", Table: table, Readiness: &Readiness{}}
	if more, e := feed.once(context.Background()); e != nil || more {
		t.Fatal("first page", more, e)
	}
	if table.Sequence() != 12 {
		t.Fatal("first page not applied", table.Sequence())
	}
	if more, e := feed.once(context.Background()); e != nil || more {
		t.Fatal("304 must be a synced success", more, e)
	}
	if table.Sequence() != 12 || len(seen) != 2 || seen[0] != `"0"` || seen[1] != `"12"` {
		t.Fatal("conditional requests", table.Sequence(), seen)
	}
}
