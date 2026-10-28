package networking

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A shutdown removes Hosted's DNS/LAN records and endpoint proof, while the
// installed claim, wildcard certificate, cached names and weekly deadline stay
// on disk. A new manager must restore those records through the normal calls.
func TestRemoteRestartRepublishesBothFamiliesLANAndEndpoint(t *testing.T) {
	for _, dual := range []bool{false, true} {
		for _, priorFailure := range []int{0, 1, 2, 3} {
			t.Run(map[bool]string{false: "ipv4", true: "dual"}[dual]+"/"+map[int]string{0: "healthy", 1: "publication retry", 2: "Retry-After", 3: "withdrawal retry"}[priorFailure], func(t *testing.T) {
				f, m, topo, _, ctx := contactFixture(t)
				cert := m.h.certificates
				m.h.endpoints.pending = map[string]endpointPending{}
				installSyntheticCertificate(t, cert, ctx)
				cert.SetListening(32500, true)
				_, q, _, _, key, err := m.snapshot(ctx)
				clear(key)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = f.db.Exec(`UPDATE networking_remote_settings SET enabled=1,mapping=0,ipv6_open=1`); err != nil {
					t.Fatal(err)
				}
				topo.Public = []string{"188.68.34.120"}
				if dual {
					topo.Public = append(topo.Public, "2a03:4000:6:8::1")
				}
				familyCount := len(topo.Public)
				now := time.Now().UTC()
				m.now = func() time.Time { return now }
				names := map[string]int{}
				published, proves := 0, 0
				var hostedGeneration int64
				records := map[string]bool{}
				lan, endpoint := false, false
				failNames := false
				cert.transport.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
					switch {
					case strings.HasSuffix(r.URL.Path, "/direct-route"):
						var in struct {
							Address string `json:"address"`
							Port    int    `json:"port"`
						}
						if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
							t.Fatal(err)
						}
						names[in.Address]++
						if failNames {
							return nil, errors.New("temporary route outage")
						}
						records[in.Address] = true
						host := "c-0123456789abcdef0123456789abcdef." + q.Namespace + ".direct.getportico.tv"
						raw, _ := json.Marshal(certificateRoute{Namespace: q.Namespace, Hostname: host, BaseURL: "https://" + host + ":" + strconv.Itoa(in.Port), CertificateDNSName: "*." + q.Namespace + ".direct.getportico.tv"})
						return reply(200, string(raw)), nil
					case strings.HasSuffix(r.URL.Path, "/routes/prove"):
						var in struct {
							Endpoint struct {
								BaseURL string `json:"baseUrl"`
							} `json:"endpoint"`
						}
						if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
							t.Fatal(err)
						}
						proves++
						endpoint = true
						raw, _ := json.Marshal(EndpointReceipt{BaseURL: in.Endpoint.BaseURL, VerifiedAt: now})
						return reply(200, string(raw)), nil
					case strings.HasSuffix(r.URL.Path, "/routes"):
						var in RouteObservation
						if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
							t.Fatal(err)
						}
						published++
						// ObserveRoutes ACKs an equal-generation replay without restoring LAN DNS.
						if in.Generation == hostedGeneration {
							raw, _ := json.Marshal(struct {
								Generation int64 `json:"generation,string"`
							}{in.Generation})
							return reply(200, string(raw)), nil
						}
						hostedGeneration = in.Generation
						lan = false
						for _, candidate := range in.Candidates {
							if candidate.Class == "lan" && candidate.BaseURL == "https://192.168.1.20:32500" {
								lan = true
							}
						}
						raw, _ := json.Marshal(struct {
							Generation int64 `json:"generation,string"`
						}{in.Generation})
						return reply(200, string(raw)), nil
					default:
						t.Errorf("unexpected call (claim/order/namespace forbidden): %s", r.URL.Path)
						return reply(500, `{}`), nil
					}
				})
				if err = m.step(ctx); err != nil {
					t.Fatal("initial", err)
				}
				if len(records) != familyCount || !lan || !endpoint {
					t.Fatal("initial publication incomplete", records, lan, endpoint)
				}
				// Model Hosted's normal shutdown withdrawal, preserving all local state.
				clear(records)
				lan = false
				endpoint = false
				restarted := &RemoteManager{h: m.h, bind: m.bind, wan: m.wan, observe: m.observe, now: m.now, wake: make(chan struct{}, 1)}
				if priorFailure != 0 {
					deadline := now.Add(time.Hour)
					config, _, authority, state, key, err := restarted.snapshot(ctx)
					clear(key)
					if err != nil {
						t.Fatal(err)
					}
					state.Attempts = 2
					state.NextAttempt = deadline
					state.PublicationFailed = priorFailure != 3
					if priorFailure >= 2 {
						state.NotBefore = deadline
					}
					if err = restarted.saveState(ctx, authority, config, state, remoteStatusOf(t, f)); err != nil {
						t.Fatal(err)
					}
					if err = restarted.step(ctx); err != nil {
						t.Fatal(err)
					}
					if published != 1 || len(records) != 0 || endpoint || lan {
						t.Fatal("startup bypassed saved failure/Retry-After")
					}
					now = deadline.Add(time.Second)
				}
				if err = restarted.step(ctx); err != nil {
					t.Fatal("restart", err)
				}
				if len(records) != familyCount || !lan || !endpoint || published != 2 || proves != 2 {
					t.Fatal("restart did not restore both DNS families, LAN and endpoint", records, lan, endpoint, published, proves)
				}
				for _, address := range topo.Public {
					if names[address] != 2 {
						t.Fatal("family not refreshed exactly once", address, names)
					}
				}
				for range 3 {
					if err = restarted.step(ctx); err != nil {
						t.Fatal(err)
					}
				}
				if published != 2 || proves != 2 {
					t.Fatal("healthy idle tick contacted Hosted", published, proves)
				}
				for _, address := range topo.Public {
					if names[address] != 2 {
						t.Fatal("healthy family renamed", names)
					}
				}
				// A failed post-restart refresh remains retryable and preserves TLS material.
				clear(records)
				failNames = true
				again := &RemoteManager{h: m.h, bind: m.bind, wan: m.wan, observe: m.observe, now: m.now, wake: make(chan struct{}, 1)}
				if err = again.step(ctx); err == nil {
					t.Fatal("failed refresh hidden")
				}
				totalNames := func() int {
					n := 0
					for _, v := range names {
						n += v
					}
					return n
				}
				failedCalls := totalNames()
				for range 3 {
					if err = again.step(ctx); err != nil {
						t.Fatal("backoff tick", err)
					}
				}
				if totalNames() != failedCalls {
					t.Fatal("failed refresh ignored backoff")
				}
				status, err := cert.Status(ctx)
				if err != nil || !status.TLSReady {
					t.Fatal("route failure removed serving certificate", err)
				}
				_, _, _, state, k, err := again.snapshot(ctx)
				clear(k)
				if err != nil {
					t.Fatal(err)
				}
				now = state.NextAttempt.Add(time.Second)
				failNames = false
				if err = again.step(ctx); err != nil {
					t.Fatal("retry", err)
				}
				if len(records) != familyCount {
					t.Fatal("failed family refresh was marked complete", records)
				}
			})
		}
	}
}

func TestRemoteRestartRestoresLANOnlyPublication(t *testing.T) {
	f, m, _, counts, ctx := contactFixture(t)
	transport := m.h.certificates.transport.client.Transport
	installSyntheticCertificate(t, m.h.certificates, ctx)
	m.h.certificates.SetListening(32500, true)
	m.h.certificates.transport.client.Transport = transport
	_, _, _, _, key, err := m.snapshot(ctx)
	clear(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE networking_remote_settings SET enabled=0,lan_sharing=1`); err != nil {
		t.Fatal(err)
	}
	if err = m.step(ctx); err != nil {
		t.Fatal(err)
	}
	first := remoteStatusOf(t, f)
	restarted := &RemoteManager{h: m.h, bind: m.bind, wan: m.wan, observe: m.observe, wake: make(chan struct{}, 1)}
	if err = restarted.step(ctx); err != nil {
		t.Fatal(err)
	}
	status := remoteStatusOf(t, f)
	if counts.publishes != 2 || counts.names != 0 || status.Generation <= first.Generation || len(status.Candidates) != 1 || status.Candidates[0].Class != "lan" {
		t.Fatal("LAN restart did not publish a new generation without public naming", counts, status)
	}
	for range 3 {
		if err = restarted.step(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if counts.publishes != 2 || counts.names != 0 {
		t.Fatal("LAN-only idle tick contacted Hosted", counts)
	}
}

func TestRemoteRestartKeepsRenewalAfterEarlyLocalFailure(t *testing.T) {
	f, m, _, counts, ctx := contactFixture(t)
	transport := m.h.certificates.transport.client.Transport
	installSyntheticCertificate(t, m.h.certificates, ctx)
	m.h.certificates.SetListening(32500, true)
	m.h.certificates.transport.client.Transport = transport
	_, _, _, _, key, err := m.snapshot(ctx)
	clear(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.db.Exec(`UPDATE networking_remote_settings SET enabled=0,lan_sharing=1`); err != nil {
		t.Fatal(err)
	}
	if err = m.step(ctx); err != nil {
		t.Fatal(err)
	}
	restarted := &RemoteManager{h: m.h, bind: m.bind, wan: m.wan, wake: make(chan struct{}, 1)}
	once := true
	restarted.observe = func(ctx context.Context, bind, gateway, advertised string) (Topology, error) {
		topo, err := m.observe(ctx, bind, gateway, advertised)
		if once {
			once = false
			if _, e := f.db.Exec(`ALTER TABLE networking_mappings RENAME TO fixture_held_mappings`); e != nil {
				t.Fatal(e)
			}
		}
		return topo, err
	}
	if err = restarted.step(ctx); err == nil {
		t.Fatal("missing mapping table did not fail before state save")
	}
	if _, err = f.db.Exec(`ALTER TABLE fixture_held_mappings RENAME TO networking_mappings`); err != nil {
		t.Fatal(err)
	}
	if err = restarted.step(ctx); err != nil {
		t.Fatal(err)
	}
	if counts.publishes != 2 {
		t.Fatal("early local failure consumed startup renewal", counts)
	}
}

func TestRemoteRoute443RefreshUsesOneCanonicalOrigin(t *testing.T) {
	_, m, _, _, ctx := contactFixture(t)
	installSyntheticCertificate(t, m.h.certificates, ctx)
	_, q, _, err := m.h.certificates.snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	host := "current." + q.Namespace + ".direct.getportico.tv"
	failed := false
	m.h.certificates.transport.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.URL.Path, "/direct-route") {
			t.Fatalf("unexpected call: %s", r.URL.Path)
		}
		if failed {
			return nil, errors.New("route unavailable")
		}
		raw, _ := json.Marshal(certificateRoute{Namespace: q.Namespace, Hostname: host, BaseURL: "https://" + host + ":443", CertificateDNSName: "*." + q.Namespace + ".direct.getportico.tv"})
		return reply(200, string(raw)), nil
	})
	want := "https://" + host
	for _, allow := range []bool{true, false} {
		got, err := m.routeName(ctx, q, "188.68.34.120", 443, allow, true)
		if err != nil || got != want {
			t.Fatal("successful/cached origin not canonical", got, err)
		}
	}
	failed = true
	restarted := &RemoteManager{h: m.h}
	got, err := restarted.routeName(ctx, q, "188.68.34.120", 443, true, true)
	if err == nil || got != want {
		t.Fatal("failed refresh changed retained origin", got, err)
	}
}
