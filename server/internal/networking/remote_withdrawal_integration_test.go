package networking

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestManagerIPv4LossDoesNotResurrectRetainedObservation(t *testing.T) {
	f, m, topo, _, ctx := contactFixture(t)
	m.h.endpoints.pending = map[string]endpointPending{}
	cert := m.h.certificates
	installSyntheticCertificate(t, cert, ctx)
	cert.SetListening(32500, true)
	_, q, authority, err := cert.snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Initialize the remote authority before applying the owner's enabled policy.
	_, _, _, _, key, err := m.snapshot(ctx)
	clear(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE networking_remote_settings SET enabled=1,mapping=0,ipv6_open=1`); err != nil {
		t.Fatal(err)
	}
	if _, err = cert.observedPublicAddress(ctx, "188.68.34.120"); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"188.68.34.120", "2a03:4000:6:8::1"} {
		if _, err = f.db.Exec(`INSERT INTO networking_route_names(authority,address,port,url) VALUES(?,?,32500,?)`, q.Scope.ID, address, "https://current."+q.Namespace+".direct.getportico.tv:32500"); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	m.now = func() time.Time { return now }
	topo.Public = []string{"2a03:4000:6:8::1"}
	m.wan = &WANObserver{now: func() time.Time { return now }, remote: netip.MustParseAddr("188.68.34.120")}
	withdrawals, names := 0, 0
	failWithdrawal := false
	cert.transport.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/direct-route/withdraw"):
			withdrawals++
			if failWithdrawal {
				return nil, errors.New("temporary outage")
			}
			var request struct {
				Family string `json:"family"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Fatal(err)
			}
			if request.Family != "ipv4" {
				t.Fatalf("withdrew healthy family %s", request.Family)
			}
			raw, _ := json.Marshal(map[string]any{"namespace": q.Namespace, "family": "ipv4", "withdrawn": true})
			return reply(200, string(raw)), nil
		case strings.HasSuffix(r.URL.Path, "/direct-route"):
			names++
			return reply(500, `{}`), nil
		case strings.HasSuffix(r.URL.Path, "/routes"):
			raw, _ := io.ReadAll(r.Body)
			var in RouteObservation
			_ = json.Unmarshal(raw, &in)
			raw, _ = json.Marshal(struct {
				Generation int64 `json:"generation,string"`
			}{in.Generation})
			return reply(200, string(raw)), nil
		default:
			return reply(503, `{"code":"proof_unavailable"}`), nil
		}
	})
	// Repeated observer outages do not imply loss while a local IPv4 exists.
	for range 4 {
		now = now.Add(10 * time.Minute)
		_ = m.step(ctx)
	}
	if withdrawals != 0 {
		t.Fatal("observer outage withdrew usable IPv4")
	}
	if _, found, err := m.familyAbsence(ctx, authority, familyIPv4); err != nil || found {
		t.Fatal("unknown observer evidence counted as absence", err)
	}
	// A complete IPv6-only interface observation is affirmative local IPv4 loss.
	topo.LAN = nil
	topo.LocalAddress = ""
	topo.Gateway = ""
	for range 2 {
		now = now.Add(10 * time.Minute)
		_ = m.step(ctx)
	}
	if withdrawals != 0 {
		t.Fatal("withdrew before observation/time threshold")
	}
	// Process restart retains the first two observations in SQLite.
	restarted := &RemoteManager{h: m.h, bind: m.bind, wan: m.wan, observe: func(context.Context, string, string, string) (Topology, error) { return *topo, nil }, now: func() time.Time { return now }, wake: make(chan struct{}, 1)}
	failWithdrawal = true
	now = now.Add(10 * time.Minute)
	_ = restarted.step(ctx)
	if withdrawals != 1 || reportedAddresses(t, f, q.Scope.ID)[familyIPv4] == "" {
		t.Fatal("failed acknowledgement lost durable address", withdrawals)
	}
	failWithdrawal = false
	now = now.Add(10 * time.Minute)
	_ = restarted.step(ctx)
	if withdrawals != 2 || reportedAddresses(t, f, q.Scope.ID)[familyIPv4] != "" || reportedAddresses(t, f, q.Scope.ID)[familyIPv6] == "" {
		t.Fatal("withdrawal did not preserve only IPv6", withdrawals)
	}
	for range 3 {
		now = now.Add(10 * time.Minute)
		_ = restarted.step(ctx)
	}
	if withdrawals != 2 || names != 0 {
		t.Fatal("old address was republished", withdrawals, names)
	}
	c, _, _, err := cert.snapshot(ctx)
	if err != nil || c.PublicAddress != "" {
		t.Fatal("certificate loop retained old IPv4 hint", c, err)
	}
}
