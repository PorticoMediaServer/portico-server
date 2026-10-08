package directdns

import (
	"encoding/base64"
	"net"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

type recorder struct{ msg *dns.Msg }

func (r *recorder) LocalAddr() net.Addr         { return &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 5353} }
func (r *recorder) RemoteAddr() net.Addr        { return &net.UDPAddr{IP: net.IPv4(1, 1, 1, 1), Port: 1234} }
func (r *recorder) WriteMsg(m *dns.Msg) error   { r.msg = m; return nil }
func (r *recorder) Write(b []byte) (int, error) { return len(b), nil }
func (r *recorder) Close() error                { return nil }
func (r *recorder) TsigStatus() error           { return nil }
func (r *recorder) TsigTimersOnly(bool)         {}
func (r *recorder) Hijack()                     {}
func TestCurrentNamespaceAndAuthority(t *testing.T) {
	ns, err := NewNamespace()
	if err != nil {
		t.Fatal(err)
	}
	unknown, _ := NewNamespace()
	table, err := OpenTable("")
	if err != nil {
		t.Fatal(err)
	}
	if err = table.Apply([]FeedRow{{Sequence: 1, Namespace: ns, Label: "0123456789abcdef0123456789abcdef", IPv4: "188.68.34.120", IPv6: "2a03:4000:6:8::1"}}); err != nil {
		t.Fatal(err)
	}
	name := CurrentHostname(ns, "direct.getportico.tv", "0123456789abcdef0123456789abcdef")
	server, err := New(Config{Zone: "direct.getportico.tv", ACMEDelegationZone: "acme.getportico.tv", Table: table})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		qtype uint16
		rcode int
		value string
	}{
		{name, dns.TypeA, dns.RcodeSuccess, "188.68.34.120"},
		{"_acme-challenge." + ns + ".direct.getportico.tv", dns.TypeTXT, dns.RcodeSuccess, ns + ".acme.getportico.tv."},
		{name, dns.TypeAAAA, dns.RcodeSuccess, "2a03:4000:6:8::1"},
		// Only the permanent name exists: not the bare namespace, not another label, not an
		// address written into a name, and not a namespace this node has never been told about.
		{"current." + ns + ".direct.getportico.tv", dns.TypeA, dns.RcodeNameError, ""},
		{ns + ".direct.getportico.tv", dns.TypeA, dns.RcodeNameError, ""},
		{"other." + ns + ".direct.getportico.tv", dns.TypeA, dns.RcodeNameError, ""},
		{"188-68-34-121." + ns + ".direct.getportico.tv", dns.TypeA, dns.RcodeNameError, ""},
		{CurrentHostname(unknown, "direct.getportico.tv", "0123456789abcdef0123456789abcdef"), dns.TypeA, dns.RcodeNameError, ""},
		{"_acme-challenge." + name, dns.TypeTXT, dns.RcodeNameError, ""},
	} {
		m := new(dns.Msg)
		m.SetQuestion(tc.name+".", tc.qtype)
		w := new(recorder)
		server.handleDNS(w, m)
		if w.msg.Rcode != tc.rcode {
			t.Fatalf("%s: %v", tc.name, w.msg)
		}
		if tc.value != "" && (len(w.msg.Answer) != 1 || !strings.Contains(w.msg.Answer[0].String(), tc.value)) {
			t.Fatalf("wrong answer %v", w.msg)
		}
	}
	m := new(dns.Msg)
	m.SetUpdate("direct.getportico.tv.")
	w := new(recorder)
	server.handleDNS(w, m)
	if w.msg.Rcode != dns.RcodeRefused {
		t.Fatal("update accepted")
	}
}
func TestCurrentNamespaceValidation(t *testing.T) {
	for _, id := range []string{"srv_x", "srv_" + base64.RawURLEncoding.EncodeToString(make([]byte, 31)), "srv_" + base64.URLEncoding.EncodeToString(make([]byte, 32)), "SRV_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32))} {
		if _, e := NamespaceFromServerID(id); e == nil {
			t.Fatal("accepted", id)
		}
	}
	ns, _ := NamespaceFromServerID("srv_" + base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	if validServerToken(ns[:55] + "b") {
		t.Fatal("noncanonical trailing bits accepted")
	}
}

// A86: after a member removal rotates the label, the previous name answers
// until its grace ends, survives a reopen, and then stops; the new one answers
// throughout.
func TestPreviousMemberLabelAnswersDuringGrace(t *testing.T) {
	ns, _ := NewNamespace()
	path := filepath.Join(t.TempDir(), "table.json")
	table, err := OpenTable(path)
	if err != nil {
		t.Fatal(err)
	}
	const oldLabel, newLabel = "0123456789abcdef0123456789abcdef", "fedcba9876543210fedcba9876543210"
	until := time.Now().Add(time.Hour).UTC().Format(time.RFC3339)
	if err = table.Apply([]FeedRow{{Sequence: 1, Namespace: ns, Label: newLabel, PreviousLabel: oldLabel, PreviousLabelUntil: until, IPv4: "188.68.34.120"}}); err != nil {
		t.Fatal(err)
	}
	ask := func(table *Table, label string) int {
		server, err := New(Config{Zone: "direct.getportico.tv", ACMEDelegationZone: "acme.getportico.tv", Table: table})
		if err != nil {
			t.Fatal(err)
		}
		m := new(dns.Msg)
		m.SetQuestion(CurrentHostname(ns, "direct.getportico.tv", label)+".", dns.TypeA)
		w := new(recorder)
		server.handleDNS(w, m)
		return w.msg.Rcode
	}
	if ask(table, newLabel) != dns.RcodeSuccess || ask(table, oldLabel) != dns.RcodeSuccess {
		t.Fatal("new or grace label did not answer")
	}
	if err = table.Compact(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenTable(path)
	if err != nil {
		t.Fatal(err)
	}
	if ask(reopened, oldLabel) != dns.RcodeSuccess {
		t.Fatal("grace label lost across a reopen")
	}
	expired := time.Now().Add(-time.Second).UTC().Format(time.RFC3339)
	if err = table.Apply([]FeedRow{{Sequence: 2, Namespace: ns, Label: newLabel, PreviousLabel: oldLabel, PreviousLabelUntil: expired, IPv4: "188.68.34.120"}}); err != nil {
		t.Fatal(err)
	}
	if ask(table, oldLabel) != dns.RcodeNameError || ask(table, newLabel) != dns.RcodeSuccess {
		t.Fatal("grace label answered after its grace")
	}
}
