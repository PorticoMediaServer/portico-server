package networking

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

func routeRaceFixture(t *testing.T) (*sqlFixture, *RemoteManager, CertificateConfig, certificateState, string, context.Context) {
	t.Helper()
	f, m, topo, _, ctx := contactFixture(t)
	cert := m.h.certificates
	m.h.endpoints.pending = map[string]endpointPending{}
	installSyntheticCertificate(t, cert, ctx)
	cert.SetListening(32500, true)
	_, _, _, _, key, e := m.snapshot(ctx)
	clear(key)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.db.Exec(`UPDATE networking_remote_settings SET enabled=1,mapping=0,ipv6_open=1`); e != nil {
		t.Fatal(e)
	}
	if _, e = cert.observedPublicAddress(ctx, "188.68.34.120"); e != nil {
		t.Fatal(e)
	}
	c, q, authority, e := cert.snapshot(ctx)
	if e != nil {
		t.Fatal(e)
	}
	for _, address := range []string{"188.68.34.120", "2a03:4000:6:8::1"} {
		if _, e = f.db.Exec(`INSERT INTO networking_route_names(authority,address,port,url) VALUES(?,?,32500,?)`, authority, address, "https://current."+q.Namespace+".direct.getportico.tv:32500"); e != nil {
			t.Fatal(e)
		}
	}
	if e = m.saveFamilyAbsence(ctx, authority, familyIPv4, familyAbsence{Since: time.Now().Add(-30 * time.Minute), Observations: 3}); e != nil {
		t.Fatal(e)
	}
	topo.LAN = nil
	topo.LocalAddress = ""
	topo.Gateway = ""
	topo.Public = []string{"2a03:4000:6:8::1"}
	return f, m, c, q, authority, ctx
}

// Either remote-effect ordering must converge to withdrawn. In particular a
// rejected local certificate publication cannot undo a prior remote naming call.
func TestRouteMutationSerializesCertificateNamingAndFamilyWithdrawal(t *testing.T) {
	for _, first := range []string{"naming", "withdrawal"} {
		t.Run(first, func(t *testing.T) {
			f, m, c, q, authority, ctx := routeRaceFixture(t)
			cert := m.h.certificates
			entered := make(chan struct{})
			release := make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			var mu sync.Mutex
			remoteIPv4 := true
			names, withdrawals := 0, 0
			cert.transport.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/direct-route/withdraw"):
					if first == "withdrawal" {
						close(entered)
						select {
						case <-release:
						case <-r.Context().Done():
							return nil, r.Context().Err()
						}
					}
					mu.Lock()
					remoteIPv4 = false
					withdrawals++
					mu.Unlock()
					raw, _ := json.Marshal(map[string]any{"namespace": q.Namespace, "family": "ipv4", "withdrawn": true})
					return reply(200, string(raw)), nil
				case strings.HasSuffix(r.URL.Path, "/direct-route"):
					if first == "naming" {
						close(entered)
						select {
						case <-release:
						case <-r.Context().Done():
							return nil, r.Context().Err()
						}
					}
					mu.Lock()
					remoteIPv4 = true
					names++
					mu.Unlock()
					host := "current." + q.Namespace + ".direct.getportico.tv"
					raw, _ := json.Marshal(certificateRoute{Namespace: q.Namespace, Hostname: host, BaseURL: "https://" + host + ":32500", CertificateDNSName: "*." + q.Namespace + ".direct.getportico.tv"})
					return reply(200, string(raw)), nil
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
			named, withdrawn := make(chan struct{}), make(chan struct{})
			name := func() { cert.updateRoute(ctx, q, c); close(named) }
			withdraw := func() { _ = m.step(ctx); close(withdrawn) }
			if first == "naming" {
				go name()
			} else {
				go withdraw()
			}
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("first route mutation never reached HTTP")
			}
			if first == "naming" {
				go withdraw()
			} else {
				go name()
			}
			// Prove a database writer remains available during the blocked HTTP effect.
			writeCtx, cancel := context.WithTimeout(ctx, time.Second)
			_, e := f.db.ExecContext(writeCtx, `UPDATE networking_remote_state SET generation=generation WHERE singleton=1`)
			cancel()
			if e != nil {
				t.Fatal("network mutation held database writer", e)
			}
			select {
			case <-named:
				if first == "withdrawal" {
					t.Fatal("stale naming bypassed withdrawal lane")
				}
			case <-withdrawn:
				if first == "naming" {
					t.Fatal("withdrawal bypassed in-flight naming")
				}
			case <-time.After(50 * time.Millisecond):
			}
			unblock()
			select {
			case <-named:
			case <-time.After(3 * time.Second):
				t.Fatal("naming stuck")
			}
			select {
			case <-withdrawn:
			case <-time.After(3 * time.Second):
				t.Fatal("withdrawal stuck")
			}
			// The stale snapshot is also rejected after all earlier work has completed.
			cert.updateRoute(ctx, q, c)
			mu.Lock()
			remaining, n, w := remoteIPv4, names, withdrawals
			mu.Unlock()
			expectedNames := 0
			if first == "naming" {
				expectedNames = 1
			}
			if remaining || n != expectedNames || w != 1 {
				t.Fatalf("remote address resurrected: present=%v names=%d withdrawals=%d", remaining, n, w)
			}
			if reportedAddresses(t, f, authority)[familyIPv4] != "" {
				t.Fatal("withdrawn family retained locally")
			}
		})
	}
}

func TestRouteMutationWaitIsCancelledWithoutRunningEffect(t *testing.T) {
	f, _, _, _, _, ctx := routeRaceFixture(t)
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- withRouteMutation(ctx, f.db, f.binding.ServerID, func(context.Context) error { close(entered); <-release; return nil })
	}()
	<-entered
	wait, cancel := context.WithCancel(ctx)
	cancel()
	e := withRouteMutation(wait, f.db, f.binding.ServerID, func(context.Context) error { t.Error("cancelled waiter executed"); return nil })
	close(release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled waiter did not return", e)
	}
	routeMutations.Lock()
	remaining := len(routeMutations.lanes)
	routeMutations.Unlock()
	if remaining != 0 {
		t.Fatal("coordinator retained inactive database", remaining)
	}
}
