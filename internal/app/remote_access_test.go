package app

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
	"golang.org/x/crypto/bcrypt"
)

func TestRemoteAccessHostedAuthorityIsEnvironmentPinned(t *testing.T) {
	settings := func(hostedBaseURL string) RemoteAccessSettings {
		return RemoteAccessSettings{
			HostedBaseURL:           hostedBaseURL,
			PublicPortMode:          "disabled",
			PreferredRemoteAuthMode: "portico",
		}
	}

	production := &Server{cfg: config.Config{Environment: "production", HostedAPIAuthority: "https://api.production.example"}}
	if err := production.validateRemoteAccessSettings(settings("https://api.production.example")); err != nil {
		t.Fatalf("approved production authority rejected: %v", err)
	}
	for _, hostedBaseURL := range []string{"https://attacker.example", "https://api.production.example.evil", "https://api.production.example/api"} {
		if err := production.validateRemoteAccessSettings(settings(hostedBaseURL)); err == nil {
			t.Fatalf("unapproved production authority %q accepted", hostedBaseURL)
		}
	}
	missingProductionAuthority := &Server{cfg: config.Config{Environment: "production"}}
	if err := missingProductionAuthority.validateRemoteAccessSettings(settings("https://api.production.example")); err == nil {
		t.Fatal("production accepted a missing pinned authority")
	}

	development := &Server{cfg: config.Config{Environment: "development"}}
	if err := development.validateRemoteAccessSettings(settings("https://custom.example")); err == nil {
		t.Fatal("development accepted an implicit non-loopback custom authority")
	}
	customDevelopment := &Server{cfg: config.Config{Environment: "development", HostedAPIAuthority: "https://custom.example"}}
	if err := customDevelopment.validateRemoteAccessSettings(settings("https://custom.example")); err != nil {
		t.Fatalf("explicit development authority rejected: %v", err)
	}
	test := &Server{cfg: config.Config{Environment: "test"}}
	if err := test.validateRemoteAccessSettings(settings("http://127.0.0.1:8080")); err != nil {
		t.Fatalf("test loopback authority rejected: %v", err)
	}
}

func TestRemoteAccessDiagnosticsDoNotPersistTransportDetails(t *testing.T) {
	secret := "https://user:password@private.example/owner-path"
	if got := remoteAccessFailureCode(errors.New("request failed for " + secret)); got != "hosted_request_failed" || strings.Contains(got, secret) {
		t.Fatalf("failure classification leaked transport detail: %q", got)
	}
	if got := remoteAccessDiagnosticCode("certificate mismatch for " + secret); got != "route_verification_failed" || strings.Contains(got, secret) {
		t.Fatalf("route classification leaked upstream detail: %q", got)
	}
}

func TestRemoteAccessLeaseIntervalRenewsBeforeExpiry(t *testing.T) {
	server := &Server{}
	if got, want := server.remoteAccessLeaseInterval(), 30*time.Minute; got != want {
		t.Fatalf("default lease renewal interval = %s, want %s", got, want)
	}
	server.remoteAccessLeaseSeconds.Store(900)
	if got, want := server.remoteAccessLeaseInterval(), 10*time.Minute; got != want {
		t.Fatalf("lease renewal interval = %s, want %s", got, want)
	}
	server.remoteAccessLeaseSeconds.Store(2700)
	if got, want := server.remoteAccessLeaseInterval(), 30*time.Minute; got != want {
		t.Fatalf("45-minute lease renewal interval = %s, want %s", got, want)
	}
}

func TestLANEndpointCandidatesExcludeListenerWildcardsAndLoopback(t *testing.T) {
	server := newRemoteAccessUnitServer(t)
	server.cfg.Addr = "0.0.0.0:32500"
	candidates := server.lanEndpointCandidates(RemoteAccessSettings{
		LANDiscoveryEnabled: true,
		ManualPublicPort:    32500,
	})
	for _, candidate := range candidates {
		host, _ := candidate["host"].(string)
		if host == "0.0.0.0" || host == "::" || host == "127.0.0.1" || host == "::1" {
			t.Fatalf("non-routable listener address was advertised: %#v", candidate)
		}
	}
	for _, privateHost := range localPrivateInterfaceHosts() {
		foundSecure := false
		for _, candidate := range candidates {
			if candidate["host"] == privateHost && candidate["port"] == 32500 && candidate["scheme"] == "https" {
				foundSecure = true
				break
			}
		}
		if !foundSecure {
			t.Fatalf("private interface %s did not receive a secure unified-port candidate: %#v", privateHost, candidates)
		}
	}
}

func TestPublicInterfaceHostsRetainBoundedDualStackPublicAddresses(t *testing.T) {
	hosts := publicInterfaceHostsFromAddresses([]string{
		"127.0.0.1/8", "10.0.0.10/24", "169.254.10.20/16", "188.68.34.120/22",
		"2a03:4000:10:342:a8d9:4cff:fe3b:fdcd/64", "188.68.34.120", "100.112.214.39/32",
		"192.0.2.10/24", "198.51.100.20/24", "203.0.113.30/24", "2001:db8::10/64",
	})
	want := []string{"188.68.34.120", "2a03:4000:10:342:a8d9:4cff:fe3b:fdcd"}
	if !reflect.DeepEqual(hosts, want) {
		t.Fatalf("public interface hosts = %#v, want %#v", hosts, want)
	}
	if got := publicInterfaceHostsFromAddresses([]string{"188.68.34.120/22"}); !reflect.DeepEqual(got, []string{"188.68.34.120"}) {
		t.Fatalf("IPv4-only public interface hosts = %#v", got)
	}
	if got := publicInterfaceHostsFromAddresses([]string{"2a03:4000:10:342::10/64"}); !reflect.DeepEqual(got, []string{"2a03:4000:10:342::10"}) {
		t.Fatalf("IPv6-only public interface hosts = %#v", got)
	}
	bounded := publicInterfaceHostsFromAddresses([]string{"8.8.8.8", "9.9.9.9", "1.1.1.1", "208.67.222.222", "208.67.220.220"})
	if len(bounded) != 4 {
		t.Fatalf("bounded public interface hosts = %#v", bounded)
	}
}

func TestRemoteAccessStatusRequiresAuthAndCreatesPersistentIdentity(t *testing.T) {
	serverURL, appDataDir, _ := newRemoteAccessTestServer(t)

	status, body := doJSON(t, http.DefaultClient, http.MethodGet, serverURL+"/api/remote-access/status", nil, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, body: %s", status, body)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	var first RemoteAccessStatus
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/remote-access/status", nil, &first)
	if status != http.StatusOK {
		t.Fatalf("status = %d, body: %s", status, body)
	}
	if first.ServerPublicKeyFingerprint == "" || first.ServerPublicKeyFingerprint == "not-generated" {
		t.Fatalf("expected generated fingerprint, got %q", first.ServerPublicKeyFingerprint)
	}
	if first.ServerPublicKey == "" {
		t.Fatalf("expected public key in status")
	}
	if first.Settings.Enabled {
		t.Fatalf("remote access should default disabled")
	}

	keyPath := filepath.Join(appDataDir, "remote-access", "server-identity-ed25519.pem")
	info, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("expected identity key file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("identity key mode = %o, expected 600", info.Mode().Perm())
	}

	var second RemoteAccessStatus
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/remote-access/status", nil, &second)
	if status != http.StatusOK {
		t.Fatalf("second status = %d, body: %s", status, body)
	}
	if second.ServerPublicKeyFingerprint != first.ServerPublicKeyFingerprint {
		t.Fatalf("fingerprint changed from %q to %q", first.ServerPublicKeyFingerprint, second.ServerPublicKeyFingerprint)
	}

	var health struct {
		Status                     string `json:"status"`
		ServerPublicKeyFingerprint string `json:"serverPublicKeyFingerprint"`
		CertificateStatus          string `json:"certificateStatus"`
	}
	status, body = doJSON(t, http.DefaultClient, http.MethodGet, serverURL+"/api/remote-access/health", nil, &health)
	if status != http.StatusOK {
		t.Fatalf("health status = %d, body: %s", status, body)
	}
	if health.Status != "ok" || health.ServerPublicKeyFingerprint != first.ServerPublicKeyFingerprint || health.CertificateStatus == "" {
		t.Fatalf("unexpected health response: %#v", health)
	}
}

func TestRemoteAccessResilienceScheduling(t *testing.T) {
	if got := remoteAccessFailureRetryInterval(1); got != 30*time.Second {
		t.Fatalf("first retry interval = %s", got)
	}
	if got := remoteAccessFailureRetryInterval(2); got != time.Minute {
		t.Fatalf("second retry interval = %s", got)
	}
	if got := remoteAccessFailureRetryInterval(3); got != 2*time.Minute {
		t.Fatalf("third retry interval = %s", got)
	}
	if got := remoteAccessFailureRetryInterval(4); got != 5*time.Minute {
		t.Fatalf("capped retry interval = %s", got)
	}
	now := time.Now().UTC()
	if !remoteAccessPublicIPCheckDue(RemoteAccessSettings{}, now, 15*time.Minute) {
		t.Fatal("missing public IP should be due")
	}
	fresh := RemoteAccessSettings{LastPublicIPAddress: "203.0.113.10", LastPublicIPCheckAt: now.Add(-time.Minute).Format(time.RFC3339)}
	if remoteAccessPublicIPCheckDue(fresh, now, 15*time.Minute) {
		t.Fatal("fresh public IP check should not be due")
	}
	stale := RemoteAccessSettings{LastPublicIPAddress: "203.0.113.10", LastPublicIPCheckAt: now.Add(-16 * time.Minute).Format(time.RFC3339)}
	if !remoteAccessPublicIPCheckDue(stale, now, 15*time.Minute) {
		t.Fatal("stale public IP check should be due")
	}
}

func TestRemoteAccessPublicEndpointUsesStatelessIPEncodedHostname(t *testing.T) {
	server := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{
		AssignedHostname:    "ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv",
		LastPublicIPAddress: "203.0.113.44",
		ManualPublicPort:    32500,
	}
	endpoint := server.remotePublicEndpoint(settings)
	if endpoint.Host != "203-0-113-44.ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv" {
		t.Fatalf("public endpoint host = %q", endpoint.Host)
	}
	if endpoint.URL != "https://203-0-113-44.ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv:32500" {
		t.Fatalf("public endpoint URL = %q", endpoint.URL)
	}
}

func TestRemoteAccessPublicEndpointRequiresNamespaceAndObservedPublicIP(t *testing.T) {
	server := newRemoteAccessUnitServer(t)
	for _, settings := range []RemoteAccessSettings{
		{AssignedHostname: "ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv"},
		{AssignedHostname: "legacy.direct.getportico.tv", LastPublicIPAddress: "203.0.113.44"},
		{AssignedHostname: "ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv", LastPublicIPAddress: "192.168.1.10"},
	} {
		if endpoint := server.remotePublicEndpoint(settings); endpoint != (RemoteAccessEndpoint{}) {
			t.Fatalf("invalid route produced public endpoint: %#v", endpoint)
		}
	}
}

func TestRemoteAccessHeartbeatCadenceDoesNotWaitForCertificateRenewal(t *testing.T) {
	certificateStarted := make(chan struct{})
	releaseCertificate := make(chan struct{})
	heartbeatSeen := make(chan struct{}, 3)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_startup/heartbeat" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-startup" {
				t.Fatalf("heartbeat auth = %q", r.Header.Get("Authorization"))
			}
			heartbeatSeen <- struct{}{}
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":                            true,
				"serverId":                      "srv_startup",
				"assignedHostname":              "ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv",
				"remoteAccessEnabled":           true,
				"publicConsoleOriginGeneration": 1,
			})
		case r.URL.Path == "/api/servers/srv_startup/certificate-orders" && r.Method == http.MethodPost:
			close(certificateStarted)
			<-releaseCertificate
			writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]string{"message": "blocked by test"}})
		case r.URL.Path == "/api/servers/srv_startup/policy-snapshot" && r.Method == http.MethodGet:
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": map[string]string{"message": "not needed"}})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(func() {
		close(releaseCertificate)
		hosted.Close()
	})

	srv := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_startup",
		AssignedHostname:        "ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv",
		PublicPortMode:          "disabled",
		ManualPublicPort:        32400,
		PreferredRemoteAuthMode: "portico",
		CertificateStatus:       "not_requested",
		LANDiscoveryEnabled:     true,
		LastHeartbeatAt:         time.Now().UTC().Format(time.RFC3339),
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-startup"); err != nil {
		t.Fatalf("save credential: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	// Preserve production fleet spreading while keeping this ordering test fast.
	srv.remoteAccessStartupCohortWindowNanos.Store(int64(time.Millisecond))
	srv.remoteAccessHeartbeatIntervalNanos.Store(int64(20 * time.Millisecond))
	go srv.runRemoteAccessHeartbeat(ctx)
	go srv.runRemoteAccessCertificateMaintenance(ctx)

	select {
	case <-heartbeatSeen:
	case <-time.After(time.Second):
		t.Fatal("startup heartbeat did not run before blocked certificate renewal")
	}
	select {
	case <-certificateStarted:
	case <-time.After(time.Second):
		t.Fatal("certificate renewal was not attempted after startup heartbeat")
	}
	select {
	case <-heartbeatSeen:
	case <-time.After(3 * time.Second):
		t.Fatal("next lease heartbeat waited behind blocked certificate renewal")
	}
}

func TestRemoteAccessHeartbeatRecordsSuccessBeforePolicySync(t *testing.T) {
	policyStarted := make(chan struct{})
	releasePolicy := make(chan struct{})
	heartbeatPayload := make(chan map[string]any, 1)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_fast/heartbeat" && r.Method == http.MethodPost:
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode heartbeat: %v", err)
			}
			heartbeatPayload <- payload
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":                            true,
				"serverId":                      "srv_fast",
				"assignedHostname":              "ptc-fast.direct.getportico.tv",
				"remoteAccessEnabled":           true,
				"publicIp":                      "198.51.100.44",
				"topologyChanged":               true,
				"publicConsoleOriginGeneration": 1,
			})
		case r.URL.Path == "/api/servers/srv_fast/policy-snapshot" && r.Method == http.MethodGet:
			close(policyStarted)
			<-releasePolicy
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": map[string]string{"message": "blocked by test"}})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(func() {
		close(releasePolicy)
		hosted.Close()
	})

	srv := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_fast",
		AssignedHostname:        "ptc-fast.direct.getportico.tv",
		PublicPortMode:          "manual",
		ManualPublicPort:        32400,
		PreferredRemoteAuthMode: "local",
		CertificateStatus:       "valid",
		LANDiscoveryEnabled:     false,
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-fast"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	if err := srv.sendRemoteAccessHeartbeat(context.Background(), settings); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	updated, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if updated.LastHeartbeatAt == "" {
		t.Fatal("heartbeat success was not recorded before policy sync completed")
	}
	if updated.LastHostedRemoteAccessState != "enabled" || updated.LastHeartbeatError != "" {
		t.Fatalf("unexpected hosted heartbeat state: state=%q error=%q", updated.LastHostedRemoteAccessState, updated.LastHeartbeatError)
	}
	if updated.LastPublicIPAddress != "198.51.100.44" || updated.LastPublicIPCheckAt == "" {
		t.Fatalf("hosted-observed public IP was not adopted: ip=%q checkedAt=%q", updated.LastPublicIPAddress, updated.LastPublicIPCheckAt)
	}
	if updated.LastReachabilityResult != "public_checking" || updated.LastPublicRouteError != "" {
		t.Fatalf("changed topology retained stale diagnostics: result=%q error=%q", updated.LastReachabilityResult, updated.LastPublicRouteError)
	}
	select {
	case payload := <-heartbeatPayload:
		if payload["serverName"] != "Portico" {
			t.Fatalf("heartbeat serverName = %#v, want current system friendly name", payload["serverName"])
		}
		if payload["preferredAuthMode"] != "local" {
			t.Fatalf("heartbeat preferredAuthMode = %#v", payload["preferredAuthMode"])
		}
		if payload["remoteAccessEnabled"] != true {
			t.Fatalf("heartbeat remoteAccessEnabled = %#v", payload["remoteAccessEnabled"])
		}
	case <-time.After(time.Second):
		t.Fatal("heartbeat payload was not captured")
	}
	select {
	case <-policyStarted:
	case <-time.After(time.Second):
		t.Fatal("policy sync was not started asynchronously")
	}
}

func TestRemoteAccessHeartbeatKeepsDirectCapabilityAvailableAcrossTopologyProbe(t *testing.T) {
	var mu sync.Mutex
	heartbeatCount := 0
	capabilityStates := make([]string, 0, 2)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/servers/srv_topology_probe/heartbeat" || r.Method != http.MethodPost {
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
		var payload struct {
			Capabilities []CompatibilityCapability `json:"capabilities"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode heartbeat: %v", err)
		}
		state := ""
		for _, capability := range payload.Capabilities {
			if capability.ID == "remote-access.direct" {
				state = capability.State
				break
			}
		}
		mu.Lock()
		heartbeatCount++
		current := heartbeatCount
		capabilityStates = append(capabilityStates, state)
		mu.Unlock()
		response := map[string]any{
			"ok":                            true,
			"serverId":                      "srv_topology_probe",
			"assignedHostname":              "ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv",
			"remoteAccessEnabled":           true,
			"publicIp":                      "198.51.100.45",
			"publicConsoleOriginGeneration": 1,
		}
		if current == 1 {
			response["topologyChanged"] = true
		} else {
			response["repair"] = map[string]any{
				"publicRouteStatus":    "reachable",
				"publicRouteCheckedAt": time.Now().UTC().Format(time.RFC3339),
			}
		}
		writeJSON(w, http.StatusOK, response)
	}))
	t.Cleanup(hosted.Close)

	srv := newRemoteAccessUnitServer(t)
	_ = srv.Handler() // Build the production route registry used by the capability envelope.
	settings := RemoteAccessSettings{
		Enabled:                     true,
		HostedBaseURL:               hosted.URL,
		ClaimStatus:                 "claimed",
		ServerID:                    "srv_topology_probe",
		AssignedHostname:            "ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv",
		PublicPortMode:              "manual",
		ManualPublicPort:            32500,
		PreferredRemoteAuthMode:     "portico",
		CertificateStatus:           "valid",
		LastPublicIPAddress:         "198.51.100.45",
		LastReachabilityResult:      "public_reachable",
		LastReachabilityCheckAt:     time.Now().UTC().Format(time.RFC3339),
		LastHostedRemoteAccessState: "enabled",
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-topology-probe"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	if err := srv.sendRemoteAccessHeartbeatWithOptions(context.Background(), settings, remoteAccessHeartbeatOptions{SyncPolicy: false}); err != nil {
		t.Fatalf("topology-change heartbeat: %v", err)
	}
	checking, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatalf("reload checking settings: %v", err)
	}
	if checking.LastReachabilityResult != "public_checking" {
		t.Fatalf("topology-change result=%q, want public_checking", checking.LastReachabilityResult)
	}
	if err := srv.sendRemoteAccessHeartbeatWithOptions(context.Background(), checking, remoteAccessHeartbeatOptions{SyncPolicy: false}); err != nil {
		t.Fatalf("reachable heartbeat: %v", err)
	}
	reachable, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatalf("reload reachable settings: %v", err)
	}
	if reachable.LastReachabilityResult != "public_reachable" {
		t.Fatalf("probe result=%q, want public_reachable", reachable.LastReachabilityResult)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(capabilityStates) != 2 || capabilityStates[0] != "available" || capabilityStates[1] != "available" {
		t.Fatalf("remote-access.direct states across topology probe=%v, want [available available]", capabilityStates)
	}
}

func TestRemoteAccessHeartbeatSkipsPolicySyncWhenDigestMatches(t *testing.T) {
	const digest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	var mu sync.Mutex
	policyRequests := 0
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_policy_current/heartbeat" && r.Method == http.MethodPost:
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Errorf("decode heartbeat: %v", err)
			}
			if payload["policyDigest"] != digest {
				t.Errorf("heartbeat policy digest = %#v", payload["policyDigest"])
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":                            true,
				"serverId":                      "srv_policy_current",
				"remoteAccessEnabled":           true,
				"policyDigest":                  digest,
				"policyChanged":                 false,
				"publicConsoleOriginGeneration": 1,
			})
		case r.URL.Path == "/api/servers/srv_policy_current/policy-snapshot":
			mu.Lock()
			policyRequests++
			mu.Unlock()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unexpected policy request"})
		default:
			t.Errorf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unexpected request"})
		}
	}))
	t.Cleanup(hosted.Close)

	srv := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_policy_current",
		PublicPortMode:          "disabled",
		ManualPublicPort:        32500,
		PreferredRemoteAuthMode: "portico",
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "policy-current-credential"); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveRemotePolicyState(remotePolicyState{SnapshotID: "snap_current", Generation: 1, IssuedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339Nano), PolicyDigest: digest}); err != nil {
		t.Fatal(err)
	}
	if err := srv.sendRemoteAccessHeartbeatWithOptions(context.Background(), settings, remoteAccessHeartbeatOptions{SyncPolicy: true}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if policyRequests != 0 {
		t.Fatalf("unchanged policy triggered %d snapshot request(s)", policyRequests)
	}
}

func TestRemoteAccessHeartbeatOmittedDigestDoesNotRefetchAfterLocalPolicyEvidence(t *testing.T) {
	const digest = "123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0"
	var mu sync.Mutex
	policyRequests := 0
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_policy_digest_omitted/heartbeat" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": true, "serverId": "srv_policy_digest_omitted", "remoteAccessEnabled": true, "publicConsoleOriginGeneration": 1,
			})
		case r.URL.Path == "/api/servers/srv_policy_digest_omitted/policy-snapshot":
			mu.Lock()
			policyRequests++
			mu.Unlock()
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unexpected policy request"})
		default:
			t.Errorf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unexpected request"})
		}
	}))
	t.Cleanup(hosted.Close)

	srv := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{Enabled: true, HostedBaseURL: hosted.URL, ClaimStatus: "claimed", ServerID: "srv_policy_digest_omitted", PublicPortMode: "disabled", ManualPublicPort: 32500, PreferredRemoteAuthMode: "portico"}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "policy-digest-omitted-credential"); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveRemotePolicyState(remotePolicyState{SnapshotID: "snap_local_evidence", Generation: 4, IssuedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339Nano), PolicyDigest: digest}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := srv.sendRemoteAccessHeartbeatWithOptions(context.Background(), settings, remoteAccessHeartbeatOptions{SyncPolicy: true}); err != nil {
			t.Fatalf("heartbeat: %v", err)
		}
	}
	time.Sleep(100 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if policyRequests != 0 {
		t.Fatalf("repeated omitted policy digests triggered %d snapshot request(s)", policyRequests)
	}
}

func TestRemoteAccessCertificatePollIntervalHasFiveSecondFloorAndBoundedRetryAfter(t *testing.T) {
	for attempt := 0; attempt < 10; attempt++ {
		delay := remoteAccessCertificatePollInterval("certord_poll_floor", attempt, 0)
		if delay < 5*time.Second || delay > time.Minute {
			t.Fatalf("attempt %d delay=%v outside bounded polling window", attempt, delay)
		}
	}
	if delay := remoteAccessCertificatePollInterval("certord_retry_after", 0, 45*time.Second); delay < 45*time.Second || delay > time.Minute {
		t.Fatalf("Retry-After delay=%v was not honored within the cap", delay)
	}
	if delay := remoteAccessCertificatePollInterval("certord_retry_after_cap", 0, 2*time.Hour); delay != time.Minute {
		t.Fatalf("oversized Retry-After delay=%v, want one-minute cap", delay)
	}
}

func TestRemoteAccessHeartbeatRetriesPendingPolicyAckWithoutRefetch(t *testing.T) {
	const policyDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	const snapshotDigest = "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"
	ackSeen := make(chan map[string]string, 1)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_policy_ack/heartbeat" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "serverId": "srv_policy_ack", "remoteAccessEnabled": true, "policyDigest": policyDigest, "policyChanged": false, "publicConsoleOriginGeneration": 1})
		case r.URL.Path == "/api/servers/srv_policy_ack/policy-sync-ack" && r.Method == http.MethodPost:
			var ack map[string]string
			if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
				t.Errorf("decode policy ack: %v", err)
			}
			ackSeen <- ack
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		case strings.Contains(r.URL.Path, "policy-snapshot"):
			t.Errorf("pending ack triggered a policy refetch")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "unexpected policy request"})
		default:
			t.Errorf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "unexpected request"})
		}
	}))
	t.Cleanup(hosted.Close)

	srv := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{Enabled: true, HostedBaseURL: hosted.URL, ClaimStatus: "claimed", ServerID: "srv_policy_ack", PublicPortMode: "disabled", ManualPublicPort: 32500, PreferredRemoteAuthMode: "portico"}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "policy-ack-credential"); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveRemotePolicyState(remotePolicyState{SnapshotID: "snap_pending", SnapshotDigest: snapshotDigest, Generation: 1, IssuedAt: time.Now().UTC().Format(time.RFC3339Nano), ExpiresAt: time.Now().UTC().Add(7 * 24 * time.Hour).Format(time.RFC3339Nano), PolicyDigest: policyDigest, AckPending: true}); err != nil {
		t.Fatal(err)
	}
	if err := srv.sendRemoteAccessHeartbeatWithOptions(context.Background(), settings, remoteAccessHeartbeatOptions{SyncPolicy: true}); err != nil {
		t.Fatal(err)
	}
	select {
	case ack := <-ackSeen:
		if ack["snapshotId"] != "snap_pending" || ack["digest"] != snapshotDigest || ack["status"] != "applied" {
			t.Fatalf("policy ack = %#v", ack)
		}
	case <-time.After(time.Second):
		t.Fatal("pending policy ack was not retried")
	}
	deadline := time.Now().Add(time.Second)
	for srv.loadRemotePolicyState().AckPending && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if srv.loadRemotePolicyState().AckPending {
		t.Fatal("successful policy ack remained pending")
	}
}

func TestRemoteAccessNetworkMonitorPushesChangeWithoutPublicIPPoll(t *testing.T) {
	heartbeatSeen := make(chan map[string]any, 1)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_ip_change/heartbeat" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-ip-change" {
				t.Fatalf("heartbeat auth = %q", r.Header.Get("Authorization"))
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode heartbeat: %v", err)
			}
			heartbeatSeen <- payload
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":                            true,
				"serverId":                      "srv_ip_change",
				"assignedHostname":              "ptc-ip-change.direct.getportico.tv",
				"remoteAccessEnabled":           true,
				"publicIp":                      "198.51.100.77",
				"publicConsoleOriginGeneration": 1,
			})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	srv := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_ip_change",
		AssignedHostname:        "ptc-ip-change.direct.getportico.tv",
		PublicPortMode:          "manual",
		ManualPublicPort:        32400,
		PreferredRemoteAuthMode: "portico",
		CertificateStatus:       "valid",
		LastPublicIPAddress:     "203.0.113.10",
		LastNetworkSignature:    "hmac-sha256:old",
		LANDiscoveryEnabled:     false,
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-ip-change"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	changed, err := srv.checkRemoteAccessNetworkSignatureAndRefresh(context.Background(), "hmac-sha256:new")
	if err != nil {
		t.Fatalf("check network signature: %v", err)
	}
	if !changed {
		t.Fatal("expected network change to be detected")
	}
	select {
	case payload := <-heartbeatSeen:
		if _, ok := payload["publicIpCandidates"].([]any); !ok {
			t.Fatalf("heartbeat public IP candidates = %#v, want canonical array", payload["publicIpCandidates"])
		}
	case <-time.After(time.Second):
		t.Fatal("network change did not trigger heartbeat")
	}
	updated, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if updated.LastPublicIPAddress != "198.51.100.77" || updated.LastNetworkSignature != "hmac-sha256:new" || updated.LastRouteRepairReason != "network_changed" || updated.LastHeartbeatAt == "" {
		t.Fatalf("unexpected network repair settings: %#v", updated)
	}
}

func TestRemoteAccessNetworkMonitorEstablishesBaselineWithoutHostedRequest(t *testing.T) {
	srv := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           "http://127.0.0.1:1",
		ClaimStatus:             "claimed",
		ServerID:                "srv_network_baseline",
		PublicPortMode:          "disabled",
		ManualPublicPort:        32500,
		PreferredRemoteAuthMode: "portico",
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-network-baseline"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	changed, err := srv.checkRemoteAccessNetworkSignatureAndRefresh(context.Background(), "hmac-sha256:baseline")
	if err != nil {
		t.Fatalf("establish network baseline: %v", err)
	}
	if changed {
		t.Fatal("initial local network baseline should not trigger a Hosted request")
	}
	updated, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if updated.LastNetworkSignature != "hmac-sha256:baseline" || updated.LastHeartbeatAt != "" {
		t.Fatalf("unexpected baseline settings: %#v", updated)
	}
}

func TestRemoteAccessHeartbeatCarriesRepairWithoutSeparatePoll(t *testing.T) {
	heartbeats := make(chan struct{}, 2)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/servers/srv_heartbeat_repair/heartbeat" || r.Method != http.MethodPost {
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
		heartbeats <- struct{}{}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":                            true,
			"serverId":                      "srv_heartbeat_repair",
			"assignedHostname":              "ptc-heartbeat-repair.direct.getportico.tv",
			"remoteAccessEnabled":           true,
			"publicIp":                      "198.51.100.90",
			"leaseSeconds":                  900,
			"publicConsoleOriginGeneration": 1,
			"repair": map[string]any{
				"hostedServicesReachable": true,
				"publicRouteStatus":       "failed",
				"publicRouteError":        "The route could not be reached.",
				"publicRouteCheckedAt":    time.Now().UTC().Format(time.RFC3339),
				"repairRequested":         true,
				"reason":                  "The route could not be reached.",
				"routeType":               "public_direct",
				"host":                    "198.51.100.90",
				"status":                  "failed",
			},
		})
	}))
	t.Cleanup(hosted.Close)

	srv := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_heartbeat_repair",
		AssignedHostname:        "ptc-heartbeat-repair.direct.getportico.tv",
		PublicPortMode:          "manual",
		ManualPublicPort:        32500,
		PreferredRemoteAuthMode: "portico",
		CertificateStatus:       "valid",
		LastPublicIPAddress:     "198.51.100.90",
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-heartbeat-repair"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	if err := srv.sendRemoteAccessHeartbeatWithOptions(context.Background(), settings, remoteAccessHeartbeatOptions{SyncPolicy: false}); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	for count := 0; count < 2; count++ {
		select {
		case <-heartbeats:
		case <-time.After(2 * time.Second):
			t.Fatalf("heartbeat-carried repair did not complete follow-up heartbeat %d", count+1)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		updated, err := srv.remoteAccessSettings()
		if err != nil {
			t.Fatalf("reload settings: %v", err)
		}
		if updated.LastRouteRepairReason == "hosted_route_failure" && updated.LastHeartbeatAt != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("heartbeat repair state was not recorded: %#v", updated)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRemoteAccessRepairSignalTriggersImmediateHeartbeat(t *testing.T) {
	heartbeatSeen := make(chan map[string]any, 1)
	var certificateCSR string
	certificateOrderCount := 0
	certificateFinalizeCount := 0
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_repair_signal/repair-signal" && r.Method == http.MethodGet:
			if r.Header.Get("Authorization") != "Bearer server-credential-repair-signal" {
				t.Fatalf("repair signal auth = %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"repairRequested": true,
				"reason":          "Browser TLS verification failed.",
				"status":          "repair_requested",
				"routeType":       "public_direct",
				"host":            "203.0.113.41",
				"lastRequestedAt": time.Now().UTC().Format(time.RFC3339),
			})
		case r.URL.Path == "/api/network/public-ip" && r.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]string{"publicIp": "198.51.100.88"})
		case r.URL.Path == "/api/servers/srv_repair_signal/certificate-orders" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-repair-signal" {
				t.Fatalf("certificate order auth = %q", r.Header.Get("Authorization"))
			}
			certificateOrderCount++
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode certificate order: %v", err)
			}
			certificateCSR = req["csrPem"]
			writeJSON(w, http.StatusOK, map[string]string{"id": "certord_repair_signal"})
		case r.URL.Path == "/api/servers/srv_repair_signal/certificate-orders/certord_repair_signal/finalize" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-repair-signal" {
				t.Fatalf("certificate finalize auth = %q", r.Header.Get("Authorization"))
			}
			certificateFinalizeCount++
			writeJSON(w, http.StatusOK, map[string]string{
				"status":              "valid",
				"certificateChainPem": certificateForCSR(t, certificateCSR, time.Now().UTC().Add(60*24*time.Hour)),
				"expiresAt":           time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.RFC3339),
			})
		case r.URL.Path == "/api/servers/srv_repair_signal/heartbeat" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-repair-signal" {
				t.Fatalf("heartbeat auth = %q", r.Header.Get("Authorization"))
			}
			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode heartbeat: %v", err)
			}
			heartbeatSeen <- payload
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":                            true,
				"serverId":                      "srv_repair_signal",
				"assignedHostname":              "ptc-bbbbbbbbbbbbbbbbbbbb.direct.getportico.tv",
				"remoteAccessEnabled":           true,
				"publicIp":                      "198.51.100.88",
				"publicConsoleOriginGeneration": 1,
			})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	srv := newRemoteAccessUnitServer(t)
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_repair_signal",
		AssignedHostname:        "ptc-bbbbbbbbbbbbbbbbbbbb.direct.getportico.tv",
		PublicPortMode:          "manual",
		ManualPublicPort:        32400,
		PreferredRemoteAuthMode: "portico",
		CertificateStatus:       "valid",
		LastPublicIPAddress:     "203.0.113.10",
		LANDiscoveryEnabled:     false,
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-repair-signal"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	repaired, err := srv.checkRemoteAccessRepairSignalAndRepair(context.Background())
	if err != nil {
		t.Fatalf("check repair signal: %v", err)
	}
	if !repaired {
		t.Fatal("expected hosted repair signal to trigger repair")
	}
	select {
	case payload := <-heartbeatSeen:
		if _, ok := payload["publicIpCandidates"].([]any); !ok {
			t.Fatalf("repair heartbeat public IP candidates = %#v, want canonical array", payload["publicIpCandidates"])
		}
	case <-time.After(time.Second):
		t.Fatal("repair signal did not trigger heartbeat")
	}
	updated, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatalf("reload settings: %v", err)
	}
	if updated.LastRouteRepairReason != "hosted_route_failure" || updated.LastReachabilityResult != "public_checking" || updated.LastPublicIPAddress != "198.51.100.88" || updated.LastHeartbeatAt == "" {
		t.Fatalf("unexpected repair signal settings: %#v", updated)
	}
	if certificateOrderCount != 1 || certificateFinalizeCount != 1 || updated.CertificateRenewalError != "" || updated.LastCertificateRenewalAt == "" {
		t.Fatalf("certificate repair did not run cleanly: order=%d finalize=%d settings=%#v", certificateOrderCount, certificateFinalizeCount, updated)
	}
}

func TestCanonicalConfiguredPublicConsoleOrigin(t *testing.T) {
	t.Parallel()
	if got := canonicalConfiguredPublicConsoleOrigin("https://DEMO.getportico.tv:443"); got != "https://demo.getportico.tv" {
		t.Fatalf("canonical public console origin = %q", got)
	}
	for _, raw := range []string{"http://demo.getportico.tv", "https://demo.getportico.tv:8443", "https://user@demo.getportico.tv", "https://demo.getportico.tv/path", "https://127.0.0.1", "https://-demo.getportico.tv", "https://demo。getportico.tv"} {
		if got := canonicalConfiguredPublicConsoleOrigin(raw); got != "" {
			t.Fatalf("invalid public console origin %q accepted as %q", raw, got)
		}
	}
}

func TestRemoteAccessSettingsValidateAndAudit(t *testing.T) {
	serverURL, _, _ := newRemoteAccessTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{
		"hostedBaseUrl": "http://hosted.example.test",
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid settings status = %d, body: %s", status, body)
	}

	var remoteStatus RemoteAccessStatus
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{
		"enabled":                         true,
		"hostedBaseUrl":                   "https://api.getportico.tv",
		"publicPortMode":                  "manual",
		"manualPublicPort":                32401,
		"preferredRemoteAuthMode":         "portico",
		"allowManualLocalAuthRemoteLogin": true,
		"lanDiscoveryEnabled":             true,
		"remoteBitrateLimitMbps":          12,
	}, &remoteStatus)
	if status != http.StatusOK {
		t.Fatalf("valid settings status = %d, body: %s", status, body)
	}
	if !remoteStatus.Settings.Enabled || remoteStatus.Settings.ManualPublicPort != 32401 || remoteStatus.Settings.RemoteBitrateLimitMbps != 12 {
		t.Fatalf("unexpected settings: %#v", remoteStatus.Settings)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{
		"remoteBitrateLimitMbps": 5000,
	}, &remoteStatus)
	if status != http.StatusOK {
		t.Fatalf("sanitized bitrate settings status = %d, body: %s", status, body)
	}
	if remoteStatus.Settings.RemoteBitrateLimitMbps != 1000 {
		t.Fatalf("remote bitrate cap was not sanitized: %#v", remoteStatus.Settings)
	}

	var audit ListResponse[AuditEvent]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/audit-events", nil, &audit)
	if status != http.StatusOK {
		t.Fatalf("audit status = %d, body: %s", status, body)
	}
	for _, event := range audit.Items {
		if event.Action == "remote_access.settings_updated" {
			return
		}
	}
	t.Fatalf("expected remote access audit event, got %#v", audit.Items)
}

func TestClaimedRemoteAccessLifecyclePublishesEveryOwnerTransition(t *testing.T) {
	heartbeats := make(chan bool, 2)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/servers/srv_lifecycle/heartbeat" {
			t.Fatalf("unexpected Hosted request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer server-credential-lifecycle" {
			t.Fatalf("heartbeat authorization = %q", r.Header.Get("Authorization"))
		}
		var payload struct {
			Enabled                       bool  `json:"remoteAccessEnabled"`
			PublicConsoleOriginGeneration int64 `json:"publicConsoleOriginGeneration"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		heartbeats <- payload.Enabled
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "serverId": "srv_lifecycle", "remoteAccessEnabled": payload.Enabled, "publicConsoleOriginGeneration": payload.PublicConsoleOriginGeneration})
	}))
	t.Cleanup(hosted.Close)

	serverURL, _, server := newRemoteAccessTestServer(t)
	settings, err := server.remoteAccessSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.HostedBaseURL = hosted.URL
	settings.ClaimStatus = "claimed"
	settings.ServerID = "srv_lifecycle"
	settings.AssignedHostname = "ptc-lifecycle.direct.getportico.tv"
	settings.PublicPortMode = "manual"
	settings.ManualPublicPort = 32400
	settings.PublicConsoleOriginGeneration = 1
	if err := server.saveRemoteAccessSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := server.saveSecretSetting(remoteAccessCredentialKey, "server-credential-lifecycle"); err != nil {
		t.Fatal(err)
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	for _, enabled := range []bool{false, true} {
		status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{"enabled": enabled}, nil)
		if status != http.StatusOK {
			t.Fatalf("set enabled=%t status=%d body=%s", enabled, status, body)
		}
		select {
		case published := <-heartbeats:
			if published != enabled {
				t.Fatalf("published enabled=%t, want %t", published, enabled)
			}
		case <-time.After(time.Second):
			t.Fatalf("enabled=%t was not published", enabled)
		}
	}
	updated, err := server.remoteAccessSettings()
	if err != nil {
		t.Fatal(err)
	}
	if updated.LastHeartbeatAt == "" || updated.LastHeartbeatError != "" {
		t.Fatalf("lifecycle heartbeat did not commit cleanly: %#v", updated)
	}
}

func TestRemotePolicyMembershipAuthorityRejectsAliasesAndControlPlanePermissions(t *testing.T) {
	owner := RemoteAccessMember{PorticoMembershipID: "mem_owner", PorticoUserID: "usr_owner", Role: "owner", Status: "active"}
	user := RemoteAccessMember{PorticoMembershipID: "mem_user", PorticoUserID: "usr_user", Role: "user", Status: "active", PermissionTemplate: RemotePermissionTemplate{Permissions: map[string]bool{"playMedia": true}}}
	if err := validateRemotePolicyMembershipAuthority([]RemoteAccessMember{owner, user}); err != nil {
		t.Fatalf("canonical owner/user snapshot rejected: %v", err)
	}
	for _, role := range []string{"admin", "viewer", "administrator"} {
		invalid := user
		invalid.Role = role
		if err := validateRemotePolicyMembershipAuthority([]RemoteAccessMember{owner, invalid}); err == nil {
			t.Fatalf("accepted remote role alias %q", role)
		}
	}
	invalidPermission := user
	invalidPermission.PermissionTemplate.Permissions = map[string]bool{"manageServer": false}
	if err := validateRemotePolicyMembershipAuthority([]RemoteAccessMember{owner, invalidPermission}); err == nil {
		t.Fatal("accepted remote control-plane permission key")
	}
	if err := validateRemotePolicyMembershipAuthority([]RemoteAccessMember{user}); err == nil {
		t.Fatal("accepted snapshot without exactly one owner")
	}
}

func TestRemoteAccessMemberSyncAutoProvisionsPorticoProfile(t *testing.T) {
	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir, HostedDocumentPublicKeys: testHostedDocumentPublicKeys()}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	err = srv.replaceRemoteAccessMembers([]RemoteAccessMember{{
		PorticoMembershipID: "mem_portico_viewer",
		PorticoUserID:       "usr_portico_viewer",
		Email:               "viewer@example.test",
		DisplayName:         "Portico Viewer",
		Role:                "user",
		Status:              "active",
	}})
	if err != nil {
		t.Fatalf("replace members: %v", err)
	}

	members := srv.listRemoteAccessMembers()
	if len(members) != 1 || members[0].LocalUserID == "" {
		t.Fatalf("expected synced member to have a local profile: %#v", members)
	}
	user, err := srv.getUser(members[0].LocalUserID)
	if err != nil {
		t.Fatalf("get provisioned user: %v", err)
	}
	if user.AuthOrigin != "portico" || user.PorticoUserID != "usr_portico_viewer" || user.PorticoMembershipID != "mem_portico_viewer" || user.HasLocalPassword {
		t.Fatalf("unexpected provisioned profile identity: %#v", user)
	}
	if user.Permissions["manageServer"] || len(user.LibraryIDs) != 0 {
		t.Fatalf("Portico profile should start restricted until invite templates or owner grants apply: %#v", user)
	}
}

func TestRemoteAccessFleetCohortDelayIsBoundedAndDistributed(t *testing.T) {
	const buckets = 10
	window := 5 * time.Minute
	counts := [buckets]int{}
	for index := 0; index < 10_000; index++ {
		serverID := fmt.Sprintf("srv_cohort_%d", index)
		delay := remoteAccessFleetCohortDelay(serverID, window)
		if delay < 0 || delay >= window {
			t.Fatalf("cohort delay %s is outside [0,%s)", delay, window)
		}
		if repeat := remoteAccessFleetCohortDelay(serverID, window); repeat != delay {
			t.Fatalf("cohort delay changed for %q: %s != %s", serverID, delay, repeat)
		}
		counts[int(delay/(window/buckets))]++
	}
	for bucket, count := range counts {
		if count < 850 || count > 1150 {
			t.Fatalf("startup cohort bucket %d is uneven: %d of 10000 (%v)", bucket, count, counts)
		}
	}
	if remoteAccessFleetCohortDelay("", window) != 0 || remoteAccessFleetCohortDelay("srv", 0) != 0 {
		t.Fatal("empty cohort inputs must not delay startup")
	}
	now := time.Date(2026, 8, 25, 12, 3, 0, 0, time.UTC)
	if wait := remoteAccessFleetCohortWait("srv_cohort_1", window, now); wait < 0 || wait >= window {
		t.Fatalf("restart-stable cohort wait %s is outside [0,%s)", wait, window)
	}
	retryFloor := 30 * time.Second
	if retry := remoteAccessRetryCohortDelay("srv_cohort_1", 1, retryFloor); retry < retryFloor || retry >= 2*retryFloor {
		t.Fatalf("retry cohort delay %s did not preserve floor %s", retry, retryFloor)
	}
}

func TestRemoteAccessMemberRevocationInvalidatesLocalSessions(t *testing.T) {
	srv := newPorticoIdentitySyncTestServer(t)
	stamp := time.Now().UTC().Format(time.RFC3339)
	insertPorticoIdentityTestUser(t, srv.db, "local-owner", "owner@example.test", "owner", "local", "", "", nil, stamp)
	var ownerID string
	if err := srv.db.QueryRow(`SELECT id FROM users WHERE role = 'owner'`).Scan(&ownerID); err != nil {
		t.Fatalf("load local owner: %v", err)
	}
	owner := RemoteAccessMember{
		PorticoMembershipID: "mem_revoked_owner", PorticoUserID: "usr_revoked_owner",
		Email: "owner@example.test", DisplayName: "Owner", Role: "owner", Status: "active", LocalUserID: ownerID,
	}
	member := RemoteAccessMember{
		PorticoMembershipID: "mem_revoked_session",
		PorticoUserID:       "usr_revoked_session",
		Email:               "revoked-session@example.test",
		DisplayName:         "Revoked Session",
		Role:                "user",
		Status:              "active",
	}
	if err := srv.replaceRemoteAccessMembers([]RemoteAccessMember{owner, member}); err != nil {
		t.Fatalf("initial member sync: %v", err)
	}
	members := srv.listRemoteAccessMembers()
	var localUserID string
	for _, candidate := range members {
		if candidate.PorticoMembershipID == member.PorticoMembershipID {
			localUserID = candidate.LocalUserID
		}
	}
	if len(members) != 2 || localUserID == "" {
		t.Fatalf("expected synced member with local profile: %#v", members)
	}
	now := time.Now().UTC()
	if _, err := srv.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"sess_revoked_session", localUserID, localUserID, hashToken("revoked-session-token"),
		now.Add(time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("insert local session: %v", err)
	}

	if err := srv.replaceRemoteAccessMembers([]RemoteAccessMember{owner}); err != nil {
		t.Fatalf("Hosted policy snapshot revocation enforcement: %v", err)
	}
	var sessionCount int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = ?`, localUserID).Scan(&sessionCount); err != nil {
		t.Fatalf("count local sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("revoked member retained %d local sessions", sessionCount)
	}
}

func TestRemotePolicySnapshotCheckpointFailureRollsBackAuthorityProjection(t *testing.T) {
	srv := newPorticoIdentitySyncTestServer(t)
	stamp := time.Now().UTC().Format(time.RFC3339)
	insertPorticoIdentityTestUser(t, srv.db, "atomic-owner", "atomic-owner@example.test", "owner", "local", "", "", nil, stamp)
	var ownerID string
	if err := srv.db.QueryRow(`SELECT id FROM users WHERE role = 'owner'`).Scan(&ownerID); err != nil {
		t.Fatalf("load atomic policy owner: %v", err)
	}
	owner := RemoteAccessMember{
		PorticoMembershipID: "mem_atomic_owner", PorticoUserID: "usr_atomic_owner",
		Email: "atomic-owner@example.test", DisplayName: "Atomic Owner", Role: "owner", Status: "active", LocalUserID: ownerID,
	}
	viewer := RemoteAccessMember{
		PorticoMembershipID: "mem_atomic_viewer", PorticoUserID: "usr_atomic_viewer",
		Email: "atomic-viewer@example.test", DisplayName: "Atomic Viewer", Role: "user", Status: "active",
	}
	if err := srv.replaceRemoteAccessMembers([]RemoteAccessMember{owner, viewer}); err != nil {
		t.Fatalf("seed atomic policy projection: %v", err)
	}
	var viewerID string
	if err := srv.db.QueryRow(`SELECT local_user_id FROM remote_access_members WHERE portico_membership_id = ?`, viewer.PorticoMembershipID).Scan(&viewerID); err != nil || viewerID == "" {
		t.Fatalf("load projected viewer: id=%q err=%v", viewerID, err)
	}
	now := time.Now().UTC()
	if _, err := srv.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES ('sess_atomic_viewer', ?, ?, ?, ?, ?, ?)`, viewerID, viewerID, hashToken("atomic-viewer-token"), now.Add(time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed projected viewer session: %v", err)
	}
	if _, err := srv.db.Exec(`
		CREATE TRIGGER fail_remote_policy_checkpoint
		BEFORE INSERT ON settings
		WHEN NEW.key = 'remoteAccessPolicyState'
		BEGIN
			SELECT RAISE(ABORT, 'injected checkpoint failure');
		END`); err != nil {
		t.Fatalf("install checkpoint failure trigger: %v", err)
	}
	state := remotePolicyState{
		SnapshotID: "policy_atomic_failure", SnapshotDigest: strings.Repeat("a", 64), Generation: 2,
		IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(maximumPolicyLifetime).Format(time.RFC3339Nano),
		PolicyDigest: strings.Repeat("b", 64), AckPending: true, TrustedTimeFloor: now.Format(time.RFC3339Nano),
	}
	if err := srv.applyRemotePolicySnapshotAtomically([]RemoteAccessMember{owner}, nil, state); err == nil {
		t.Fatal("policy projection committed despite checkpoint persistence failure")
	}
	var memberStatus string
	if err := srv.db.QueryRow(`SELECT status FROM remote_access_members WHERE portico_membership_id = ?`, viewer.PorticoMembershipID).Scan(&memberStatus); err != nil {
		t.Fatalf("load rolled-back member status: %v", err)
	}
	var sessionCount, checkpointCount int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE id = 'sess_atomic_viewer'`).Scan(&sessionCount); err != nil {
		t.Fatalf("count rolled-back viewer session: %v", err)
	}
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM settings WHERE key = 'remoteAccessPolicyState'`).Scan(&checkpointCount); err != nil {
		t.Fatalf("count rolled-back checkpoint: %v", err)
	}
	if memberStatus != "active" || sessionCount != 1 || checkpointCount != 0 {
		t.Fatalf("policy/checkpoint transaction was not atomic: status=%q sessions=%d checkpoints=%d", memberStatus, sessionCount, checkpointCount)
	}
}

func newPorticoIdentitySyncTestServer(t *testing.T) *Server {
	t.Helper()
	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir: appDataDir, DatabasePath: filepath.Join(appDataDir, "portico.db"),
		WebDistDir: filepath.Join("web", "dist"), SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open identity-sync database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func insertPorticoIdentityTestUser(t *testing.T, db *sql.DB, id, email, role, origin, porticoUserID, membershipID string, passwordHash []byte, now string) {
	t.Helper()
	var storedPassword any
	if len(passwordHash) > 0 {
		storedPassword = string(passwordHash)
	}
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, auth_origin, portico_user_id, portico_membership_id, permissions_json, preferences_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', '{}', ?, ?)`,
		id, id, email, id, storedPassword, role, origin, porticoUserID, membershipID, now, now); err != nil {
		t.Fatalf("insert identity test user %s: %v", id, err)
	}
}

func TestRemoteAccessMemberSyncLinksSingleExistingOwner(t *testing.T) {
	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("owner-password-123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	permissionsJSON, _ := json.Marshal(ownerPermissions())
	preferencesJSON, _ := json.Marshal(defaultUserPreferences())
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, auth_origin, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_existing_owner', 'owner', 'owner@example.test', 'Existing Owner', ?, 'owner', 'local', ?, ?, ?, ?)`,
		string(passwordHash), string(permissionsJSON), string(preferencesJSON), now, now); err != nil {
		t.Fatalf("insert owner: %v", err)
	}
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	err = srv.replaceRemoteAccessMembers([]RemoteAccessMember{{
		PorticoMembershipID: "mem_existing_owner",
		PorticoUserID:       "usr_portico_owner",
		Email:               "owner@example.test",
		DisplayName:         "Existing Owner",
		Role:                "owner",
		Status:              "active",
	}})
	if err != nil {
		t.Fatalf("replace members: %v", err)
	}

	members := srv.listRemoteAccessMembers()
	if len(members) != 1 || members[0].LocalUserID != "usr_existing_owner" {
		t.Fatalf("expected hosted owner to link to existing local owner: %#v", members)
	}
	user, err := srv.getUser("usr_existing_owner")
	if err != nil {
		t.Fatalf("get linked owner: %v", err)
	}
	if user.AuthOrigin != "portico" || user.PorticoUserID != "usr_portico_owner" || user.PorticoMembershipID != "mem_existing_owner" || !user.HasLocalPassword {
		t.Fatalf("unexpected linked owner: %#v", user)
	}
}

func TestPorticoSessionAttachDoesNotRequireThisServerPassword(t *testing.T) {
	chdirRepoRoot(t)
	now := time.Now().UTC().Truncate(time.Second)
	const installationID = "portico-attach-test-0001"
	profile := HostedProfileSnapshot{
		ExternalProfileID: "prf_usr_plus", AccountID: "usr_plus", DisplayName: "Plus User",
		IsPrimary: true, IsAccountAdmin: true, SortOrder: 0, PolicyUpdatedAt: now.Add(-time.Minute),
		Restrictions: defaultProfileRestrictions(),
	}
	rawEnvelope := signedHostedProfileSelectionEnvelope(t, testHostedDocumentPrivateKey(), HostedProfileSelectionEnvelope{
		Version: hostedProfileSelectionAssertionVersion, AssertionID: "psa_plus_attach", Audience: hostedDocumentAudience,
		AccountID: "usr_plus", ProfileID: profile.ExternalProfileID, ServerID: "srv_plus", DeviceID: "dev_plus", InstallationID: installationID,
		AccountRevision: 1, PINRevision: 0, Profiles: []HostedProfileSnapshot{profile},
		IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(2 * time.Minute).Format(time.RFC3339Nano),
		SignatureAlgorithm: hostedSignatureAlgorithm, SignatureKeyID: testHostedDocumentKeyID,
	})
	var selectionEnvelope HostedProfileSelectionEnvelope
	if err := json.Unmarshal(rawEnvelope, &selectionEnvelope); err != nil {
		t.Fatalf("decode profile selection envelope: %v", err)
	}
	signingKeySet := testHostedDocumentSigningKeySet(t)
	var introspectSeen bool
	var introspectionCount int
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/signing-keys" {
			if r.Method != http.MethodGet {
				t.Fatalf("signing key lifecycle method = %s", r.Method)
			}
			writeJSON(w, http.StatusOK, signingKeySet)
			return
		}
		if r.Header.Get("Authorization") != "Bearer server-credential-plus" {
			t.Fatalf("hosted auth = %q", r.Header.Get("Authorization"))
		}
		switch r.URL.Path {
		case "/api/servers/srv_plus/portico-sessions/introspect":
			introspectionCount++
			if introspectionCount > 1 {
				writeError(w, http.StatusUnauthorized, "invalid_portico_session", "Portico access token is invalid or expired.")
				return
			}
			var request struct {
				AccessToken       string                         `json:"accessToken"`
				SelectionEnvelope HostedProfileSelectionEnvelope `json:"selectionEnvelope"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.AccessToken != "ptc_clt_plus" || request.SelectionEnvelope.AssertionID != selectionEnvelope.AssertionID {
				t.Fatalf("unexpected introspection request: %#v err=%v", request, err)
			}
			introspectSeen = true
			writeJSON(w, http.StatusOK, map[string]any{
				"active":            true,
				"deviceId":          "dev_plus",
				"selectionEnvelope": selectionEnvelope,
				"member": map[string]any{
					"id":                  "mem_plus",
					"porticoMembershipId": "mem_plus",
					"userId":              "usr_plus",
					"porticoUserId":       "usr_plus",
					"email":               "plus@example.test",
					"displayName":         "Plus User",
					"role":                "user",
					"status":              "active",
					"permissionTemplate": map[string]any{
						"permissions": map[string]bool{"playMedia": true},
					},
				},
			})
		case "/api/servers/srv_plus/profile-selection-exchanges":
			var request struct {
				SelectionEnvelope HostedProfileSelectionEnvelope `json:"selectionEnvelope"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil || request.SelectionEnvelope.AssertionID != selectionEnvelope.AssertionID {
				t.Fatalf("unexpected profile exchange request: %#v err=%v", request, err)
			}
			writeJSON(w, http.StatusOK, request.SelectionEnvelope)
		default:
			t.Fatalf("unexpected hosted request: %s", r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)
	appDataDir := t.TempDir()
	cfg := config.Config{
		Addr:                     "127.0.0.1:0",
		AppDataDir:               appDataDir,
		DatabasePath:             filepath.Join(appDataDir, "portico.db"),
		WebDistDir:               filepath.Join("web", "dist"),
		SampleMediaURL:           "https://media.example.test/sample.mp4",
		HostedDocumentPublicKeys: testHostedDocumentPublicKeys(),
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	passwordHash, err := bcrypt.GenerateFromPassword([]byte("local-password-123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	permissionsJSON, _ := json.Marshal(permissionsForRole("user"))
	preferencesJSON, _ := json.Marshal(defaultUserPreferences())
	nowText := now.Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, auth_origin, portico_user_id, portico_membership_id, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_local_plus', 'plus', 'plus@example.test', 'Plus User', ?, 'user', 'portico', 'usr_plus', 'mem_plus', ?, ?, ?, ?)`,
		string(passwordHash), string(permissionsJSON), string(preferencesJSON), nowText, nowText); err != nil {
		t.Fatalf("insert plus user: %v", err)
	}
	srv := &Server{cfg: cfg, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil)), logSubscribers: map[chan LogEvent]bool{}, scannerWatch: map[string]string{}, transcodes: map[string]*transcodeSession{}}
	if err := srv.saveRemoteAccessSettings(RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_plus",
		PublicPortMode:          "manual",
		ManualPublicPort:        32401,
		PreferredRemoteAuthMode: "portico",
	}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-plus"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	testServer := httptest.NewServer(srv.Handler())
	t.Cleanup(testServer.Close)
	status, body := performProtectedPorticoAttachmentRequest(t, srv, PorticoSessionAttachRequest{
		AccessToken: "ptc_clt_plus", InstallationID: installationID,
		DeviceName: "Portico Attach Test", App: "Portico Test", Platform: "TestOS",
	}, nil)
	if status != http.StatusBadRequest || introspectSeen {
		t.Fatalf("missing selection envelope status=%d body=%s introspected=%v", status, body, introspectSeen)
	}
	var credentials NativeSessionCredentials
	var recovered NativeSessionCredentials
	status, body = performProtectedPorticoAttachmentRequest(t, srv, PorticoSessionAttachRequest{
		AccessToken: "ptc_clt_plus", SelectionEnvelope: selectionEnvelope, InstallationID: "portico-attach-other-0001",
		DeviceName: "Portico Attach Test", App: "Portico Test", Platform: "TestOS",
	}, &credentials)
	if status != http.StatusCreated || credentials.AccessToken == "" || credentials.RefreshToken == "" {
		t.Fatalf("Portico attach status=%d body=%s credentials=%#v", status, body, credentials)
	}
	if credentials.Authority != "hosted" || credentials.AccountID != profile.AccountID || credentials.ProfileID != profile.ExternalProfileID || credentials.User.ProfileID != profile.ExternalProfileID || credentials.ServerID != "srv_plus" {
		t.Fatalf("Portico attach exposed the wrong public viewer identity: %#v", credentials)
	}
	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("server-local Portico auth request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+credentials.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("server-local Portico auth: %v", err)
	}
	var auth AuthMeResponse
	if err := json.NewDecoder(resp.Body).Decode(&auth); err != nil {
		t.Fatalf("decode bearer-only auth: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !auth.Authenticated || auth.User == nil || auth.User.ID != "usr_local_plus" {
		t.Fatalf("server-local Portico auth status = %d body=%#v", resp.StatusCode, auth)
	}
	if auth.Authority != "hosted" || auth.AccountID != profile.AccountID || auth.ProfileID != profile.ExternalProfileID || auth.User.ProfileID != profile.ExternalProfileID || auth.ServerID != "srv_plus" {
		t.Fatalf("Portico auth/me exposed the wrong public viewer identity: %#v", auth)
	}
	if !introspectSeen {
		t.Fatalf("expected Portico session introspection")
	}
	status, body = performProtectedPorticoAttachmentRequest(t, srv, PorticoSessionAttachRequest{
		AccessToken: "ptc_clt_plus", SelectionEnvelope: selectionEnvelope, InstallationID: installationID,
		DeviceName: "Portico Attach Test", App: "Portico Test", Platform: "TestOS",
	}, &recovered)
	if status != http.StatusCreated || recovered.AccessToken != credentials.AccessToken || recovered.RefreshToken != credentials.RefreshToken {
		t.Fatalf("Portico attach receipt recovery status=%d body=%s recovered=%#v original=%#v", status, body, recovered, credentials)
	}
	status, body = performProtectedPorticoAttachmentRequest(t, srv, PorticoSessionAttachRequest{
		AccessToken: "ptc_clt_plus", SelectionEnvelope: selectionEnvelope, InstallationID: installationID,
		DeviceName: "Different Portico Attach Test", App: "Portico Test", Platform: "TestOS",
	}, nil)
	if status != http.StatusUnauthorized || !strings.Contains(body, `"code":"profile_selection_failed"`) || strings.Contains(body, "server_session_revoked") {
		t.Fatalf("rejected first-attachment bootstrap status=%d body=%s", status, body)
	}
	introspectSeen = false
	queryReq := httptest.NewRequest(http.MethodGet, "/api/artwork/movie/poster.svg?access_token=ptc_clt_plus", nil)
	if queryUser, ok := srv.currentPorticoUser(queryReq); ok {
		t.Fatalf("hosted query-token auth should be rejected, got %#v", queryUser)
	}
	if introspectSeen {
		t.Fatalf("rejected query credentials must not be introspected")
	}
	introspectSeen = false
	liveLogoReq := httptest.NewRequest(http.MethodGet, "/api/live-tv/logos/chan_portico?access_token=ptc_clt_plus", nil)
	if liveLogoUser, ok := srv.currentPorticoUser(liveLogoReq); ok {
		t.Fatalf("hosted Live TV logo query-token auth should be rejected, got %#v", liveLogoUser)
	}
	if introspectSeen {
		t.Fatalf("rejected Live TV logo query credentials must not be introspected")
	}
	directCloudReq, _ := http.NewRequest(http.MethodGet, testServer.URL+"/api/auth/me", nil)
	directCloudReq.Header.Set("Authorization", "Bearer ptc_clt_plus")
	directCloudResp, err := http.DefaultClient.Do(directCloudReq)
	if err != nil {
		t.Fatalf("ordinary Cloud bearer request: %v", err)
	}
	var directCloudAuth AuthMeResponse
	if err := json.NewDecoder(directCloudResp.Body).Decode(&directCloudAuth); err != nil {
		t.Fatalf("decode ordinary Cloud bearer auth: %v", err)
	}
	directCloudResp.Body.Close()
	if directCloudAuth.Authenticated || introspectSeen {
		t.Fatalf("ordinary server API must not accept or introspect Cloud bearer: auth=%#v introspected=%v", directCloudAuth, introspectSeen)
	}
	disallowedQueryReq := httptest.NewRequest(http.MethodGet, "/api/auth/me?access_token=ptc_clt_plus", nil)
	if queryUser, ok := srv.currentPorticoUser(disallowedQueryReq); ok {
		t.Fatalf("query-token auth should be rejected on JSON auth endpoints, got %#v", queryUser)
	}
	if err := srv.replaceRemoteAccessMembers(nil); err != nil {
		t.Fatalf("apply authoritative membership revocation: %v", err)
	}
	status, body, _ = refreshNativeCredentialsForTest(testServer.URL, credentials.RefreshToken)
	if status != http.StatusForbidden || !strings.Contains(body, "membership_inactive") {
		t.Fatalf("revoked Portico membership refresh status=%d body=%s", status, body)
	}
}

func TestRemoteAccessMemberSyncPreservesLocalLibraryAssignments(t *testing.T) {
	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	err = srv.replaceRemoteAccessMembers([]RemoteAccessMember{{
		PorticoMembershipID: "mem_template_viewer",
		PorticoUserID:       "usr_template_viewer",
		Email:               "templated@example.test",
		DisplayName:         "Templated Viewer",
		Role:                "user",
		Status:              "active",
		PermissionTemplate: RemotePermissionTemplate{
			MaxContentRating: "PG-13",
			Permissions: map[string]bool{
				"playMedia":     true,
				"downloadMedia": true,
				"transcode":     false,
				"manageServer":  true,
			},
		},
	}})
	if err != nil {
		t.Fatalf("replace members: %v", err)
	}

	members := srv.listRemoteAccessMembers()
	if len(members) != 1 || members[0].LocalUserID == "" {
		t.Fatalf("expected synced member to have a local profile: %#v", members)
	}
	user, err := srv.getUser(members[0].LocalUserID)
	if err != nil {
		t.Fatalf("get provisioned user: %v", err)
	}
	if !user.Permissions["playMedia"] || !user.Permissions["downloadMedia"] || user.Permissions["transcode"] || user.Permissions["manageServer"] {
		t.Fatalf("permission template was not applied with viewer safeguards: %#v", user.Permissions)
	}
	if user.MaxContentRating != "PG-13" {
		t.Fatalf("max content rating = %q", user.MaxContentRating)
	}
	libraries, err := srv.listLibraries()
	if err != nil || len(libraries) == 0 {
		t.Fatalf("list local libraries: %v (%#v)", err, libraries)
	}
	assigned := libraries[0].ID
	if _, err := srv.db.Exec(`INSERT INTO user_library_access (user_id, library_id, created_at) VALUES (?, ?, ?)`, user.ID, assigned, time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatalf("assign local library: %v", err)
	}
	if err := srv.replaceRemoteAccessMembers([]RemoteAccessMember{{
		PorticoMembershipID: "mem_template_viewer", PorticoUserID: "usr_template_viewer",
		Email: "templated@example.test", DisplayName: "Templated Viewer", Role: "user", Status: "active",
		PermissionTemplate: RemotePermissionTemplate{MaxContentRating: "G"},
	}}); err != nil {
		t.Fatalf("refresh members: %v", err)
	}
	user, err = srv.getUser(user.ID)
	if err != nil || !slices.Contains(user.LibraryIDs, assigned) || len(user.LibraryIDs) != 1 {
		t.Fatalf("Hosted policy refresh changed server-local library assignment: %#v (err=%v)", user.LibraryIDs, err)
	}
}

func TestRemoteAccessMemberSyncNewMemberStartsWithoutLibraries(t *testing.T) {
	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}

	err = srv.replaceRemoteAccessMembers([]RemoteAccessMember{{
		PorticoMembershipID: "mem_all_libraries",
		PorticoUserID:       "usr_all_libraries",
		Email:               "all-libraries@example.test",
		DisplayName:         "All Libraries",
		Role:                "user",
		Status:              "active",
		PermissionTemplate: RemotePermissionTemplate{
			Permissions: map[string]bool{
				"playMedia": true,
				"transcode": true,
			},
		},
	}})
	if err != nil {
		t.Fatalf("replace members: %v", err)
	}

	members := srv.listRemoteAccessMembers()
	if len(members) != 1 || members[0].LocalUserID == "" {
		t.Fatalf("expected synced member to have a local profile: %#v", members)
	}
	user, err := srv.getUser(members[0].LocalUserID)
	if err != nil {
		t.Fatalf("get provisioned user: %v", err)
	}
	if len(user.LibraryIDs) != 0 {
		t.Fatalf("new Hosted member must start without server-local libraries: %#v", user.LibraryIDs)
	}
}

func TestRemotePolicySnapshotTombstonePurgesPorticoPrincipalCache(t *testing.T) {
	srv := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := srv.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, auth_origin, portico_user_id, portico_membership_id, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_portico_deleted', 'deleted', 'deleted@example.test', 'Deleted Portico', NULL, 'user', 'portico', 'portico_deleted', 'mem_deleted', '{}', '{}', ?, ?)`, now, now); err != nil {
		t.Fatalf("insert hosted user: %v", err)
	}
	if _, err := srv.db.Exec(`INSERT INTO sessions (id, user_id, profile_id, token_hash, expires_at, created_at, last_seen_at) VALUES ('sess_deleted', 'usr_portico_deleted', 'usr_portico_deleted', 'hash', ?, ?, ?)`, now, now, now); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := srv.db.Exec(`
		INSERT INTO remote_access_members (portico_membership_id, portico_user_id, email, display_name, role, status, local_user_id, last_synced_at)
		VALUES ('mem_deleted', 'portico_deleted', 'deleted@example.test', 'Deleted Portico', 'user', 'active', 'usr_portico_deleted', ?)`, now); err != nil {
		t.Fatalf("insert remote member: %v", err)
	}

	if err := srv.applyRemoteDeletedAccountTombstones([]RemoteDeletedAccountTombstone{{UserID: "portico_deleted", DeletedAt: now}}); err != nil {
		t.Fatalf("apply tombstone: %v", err)
	}
	var authOrigin, porticoUserID, porticoMembershipID, email string
	if err := srv.db.QueryRow(`SELECT auth_origin, portico_user_id, portico_membership_id, email FROM users WHERE id = 'usr_portico_deleted'`).Scan(&authOrigin, &porticoUserID, &porticoMembershipID, &email); err != nil {
		t.Fatalf("load user: %v", err)
	}
	if authOrigin != "portico_deleted" || porticoUserID != "" || porticoMembershipID != "" || email == "deleted@example.test" {
		t.Fatalf("Portico principal cache was not purged: origin=%q porticoUserID=%q membership=%q email=%q", authOrigin, porticoUserID, porticoMembershipID, email)
	}
	var sessionCount int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = 'usr_portico_deleted'`).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessionCount != 0 {
		t.Fatalf("sessions remaining = %d", sessionCount)
	}
}

func TestPorticoProfileProvisioningAvoidsTakingOverLocalPasswordAccountWithSameEmail(t *testing.T) {
	srv := newScannerTestServer(t)
	now := time.Now().UTC().Format(time.RFC3339)
	passwordHash, _ := bcrypt.GenerateFromPassword([]byte("LocalPassword123!"), bcrypt.DefaultCost)
	permissionsJSON, _ := json.Marshal(permissionsForRole("user"))
	preferencesJSON, _ := json.Marshal(defaultUserPreferences())
	if _, err := srv.db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, auth_origin, portico_user_id, portico_membership_id, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_local_same_email', 'localuser', 'same@example.test', 'Local User', ?, 'user', 'local', '', '', ?, ?, ?, ?)`,
		string(passwordHash), string(permissionsJSON), string(preferencesJSON), now, now); err != nil {
		t.Fatalf("insert local user: %v", err)
	}

	_, err := srv.userForPorticoMembership(RemoteAccessMember{
		PorticoMembershipID: "mem_same_email",
		PorticoUserID:       "usr_same_email_portico",
		Email:               "same@example.test",
		DisplayName:         "Portico User",
		Role:                "user",
		Status:              "active",
		PermissionTemplate: RemotePermissionTemplate{
			Permissions: map[string]bool{"playMedia": true, "transcode": true},
		},
	})
	if !errors.Is(err, errPorticoIdentityLinkRequired) {
		t.Fatalf("expected an explicit identity-link requirement, got %v", err)
	}
	localUser, err := srv.getUser("usr_local_same_email")
	if err != nil {
		t.Fatalf("get local user: %v", err)
	}
	if localUser.AuthOrigin != "local" || localUser.PorticoUserID != "" || localUser.PorticoMembershipID != "" {
		t.Fatalf("local user was modified: %#v", localUser)
	}
	var count int
	if err := srv.db.QueryRow(`SELECT COUNT(*) FROM users WHERE lower(email) = lower('same@example.test')`).Scan(&count); err != nil {
		t.Fatalf("count same-email users: %v", err)
	}
	if count != 1 {
		t.Fatalf("identity-link requirement created a duplicate user: count=%d", count)
	}
}

func TestCloudBootstrapIsRejectedByNormalServerAPIsWithoutMutatingCachedPrincipals(t *testing.T) {
	chdirRepoRoot(t)
	var sawIntrospect bool
	var sawPolicySnapshot bool
	deletedAt := time.Now().UTC().Format(time.RFC3339)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_deleted_token/portico-sessions/introspect" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-deleted-token" {
				t.Fatalf("introspect auth = %q", r.Header.Get("Authorization"))
			}
			sawIntrospect = true
			writeError(w, http.StatusUnauthorized, "invalid_portico_session", "Portico access token is invalid or expired.")
		case r.URL.Path == "/api/servers/srv_deleted_token/policy-snapshot" && r.Method == http.MethodGet:
			if r.Header.Get("Authorization") != "Bearer server-credential-deleted-token" {
				t.Fatalf("policy auth = %q", r.Header.Get("Authorization"))
			}
			sawPolicySnapshot = true
			writeJSON(w, http.StatusOK, signedTestPolicySnapshot(t, map[string]any{
				"snapshotId":               "policy_deleted_token",
				"version":                  1,
				"serverId":                 "srv_deleted_token",
				"members":                  []any{},
				"pendingInvites":           []any{},
				"deletedAccountTombstones": []map[string]any{{"userId": "portico_deleted_token", "deletedAt": deletedAt}},
				"issuedAt":                 deletedAt,
				"expiresAt":                time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339),
				"minimumServerVersion":     "0.1.0",
			}))
		case r.URL.Path == "/api/servers/srv_deleted_token/policy-sync-ack" && r.Method == http.MethodPost:
			var ack map[string]string
			if err := json.NewDecoder(r.Body).Decode(&ack); err != nil {
				t.Fatalf("decode policy ack: %v", err)
			}
			if ack["digest"] != strings.Repeat("a", 64) || ack["status"] != "applied" {
				t.Fatalf("policy ack = %#v", ack)
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	appDataDir := t.TempDir()
	cfg := config.Config{
		Addr:                     "127.0.0.1:0",
		AppDataDir:               appDataDir,
		DatabasePath:             filepath.Join(appDataDir, "portico.db"),
		WebDistDir:               filepath.Join("web", "dist"),
		SampleMediaURL:           "https://media.example.test/sample.mp4",
		HostedDocumentPublicKeys: testHostedDocumentPublicKeys(),
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UTC().Format(time.RFC3339)
	permissionsJSON, _ := json.Marshal(permissionsForRole("user"))
	preferencesJSON, _ := json.Marshal(defaultUserPreferences())
	if _, err := db.Exec(`
		INSERT INTO users (id, username, email, display_name, password_hash, role, auth_origin, portico_user_id, portico_membership_id, permissions_json, preferences_json, created_at, updated_at)
		VALUES ('usr_deleted_token', 'deletedtoken', 'deleted-token@example.test', 'Deleted Token', NULL, 'user', 'portico', 'portico_deleted_token', 'mem_deleted_token', ?, ?, ?, ?)`,
		string(permissionsJSON), string(preferencesJSON), now, now); err != nil {
		t.Fatalf("insert hosted user: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO sessions (id, user_id, profile_id, token_hash, expires_at, created_at, last_seen_at) VALUES ('sess_deleted_token', 'usr_deleted_token', 'usr_deleted_token', 'hash', ?, ?, ?)`, now, now, now); err != nil {
		t.Fatalf("insert session: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO remote_access_members (portico_membership_id, portico_user_id, email, display_name, role, status, local_user_id, last_synced_at)
		VALUES ('mem_deleted_token', 'portico_deleted_token', 'deleted-token@example.test', 'Deleted Token', 'user', 'active', 'usr_deleted_token', ?)`, now); err != nil {
		t.Fatalf("insert remote member: %v", err)
	}
	srv := &Server{cfg: cfg, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil)), logSubscribers: map[chan LogEvent]bool{}, scannerWatch: map[string]string{}, transcodes: map[string]*transcodeSession{}}
	if err := srv.saveRemoteAccessSettings(RemoteAccessSettings{Enabled: true, HostedBaseURL: hosted.URL, ClaimStatus: "claimed", ServerID: "srv_deleted_token", PublicPortMode: "manual", ManualPublicPort: 32401, PreferredRemoteAuthMode: "portico"}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-deleted-token"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	savedSettings, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatalf("load settings: %v", err)
	}
	if savedSettings.PreferredRemoteAuthMode != "portico" || savedSettings.ServerID != "srv_deleted_token" || srv.secretSetting(remoteAccessCredentialKey) == "" {
		t.Fatalf("unexpected remote access settings: %#v credential=%q", savedSettings, srv.secretSetting(remoteAccessCredentialKey))
	}
	testServer := httptest.NewServer(srv.Handler())
	t.Cleanup(testServer.Close)

	req, err := http.NewRequest(http.MethodGet, testServer.URL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("Portico bearer auth request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer ptc_clt_deleted_token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Portico bearer auth response: %v", err)
	}
	responseBody, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("auth/me status = %d body=%s", resp.StatusCode, responseBody)
	}
	if sawIntrospect || sawPolicySnapshot {
		t.Fatalf("normal server API consulted Hosted Services for a bootstrap token: introspect=%t policy=%t body=%s", sawIntrospect, sawPolicySnapshot, responseBody)
	}
	var authOrigin, porticoUserID, porticoMembershipID, email string
	if err := db.QueryRow(`SELECT auth_origin, portico_user_id, portico_membership_id, email FROM users WHERE id = 'usr_deleted_token'`).Scan(&authOrigin, &porticoUserID, &porticoMembershipID, &email); err != nil {
		t.Fatalf("load user: %v", err)
	}
	if authOrigin != "portico" || porticoUserID != "portico_deleted_token" || porticoMembershipID != "mem_deleted_token" || email != "deleted-token@example.test" {
		t.Fatalf("rejected bootstrap mutated the cached principal: origin=%q porticoUserID=%q membership=%q email=%q", authOrigin, porticoUserID, porticoMembershipID, email)
	}
	var sessionCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id = 'usr_deleted_token'`).Scan(&sessionCount); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessionCount != 1 {
		t.Fatalf("rejected bootstrap mutated server-native sessions: count=%d", sessionCount)
	}
}

func TestRemoteAccessAutomaticRouterMappingIsOptIn(t *testing.T) {
	previousMapper := routerMapper
	routerMapper = fakeRouterMapper{
		add:    RouterMappingResult{Status: "mapped", Protocol: "nat-pmp"},
		remove: RouterMappingResult{Status: "removed", Protocol: "nat-pmp"},
	}
	t.Cleanup(func() { routerMapper = previousMapper })

	serverURL, _, _ := newRemoteAccessTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{
		"enabled":                 true,
		"publicPortMode":          "automatic",
		"routerAutomationEnabled": true,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("settings status = %d, body: %s", status, body)
	}

	var remoteStatus RemoteAccessStatus
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/test-direct", nil, &remoteStatus)
	if status != http.StatusOK {
		t.Fatalf("test-direct status = %d, body: %s", status, body)
	}
	if remoteStatus.Settings.RouterMappingStatus != "mapped" || remoteStatus.Settings.LastReachabilityResult != "router_mapping_active" {
		t.Fatalf("unexpected router mapping status: %#v", remoteStatus.Settings)
	}

	var audit ListResponse[AuditEvent]
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/audit-events", nil, &audit)
	if status != http.StatusOK {
		t.Fatalf("audit status = %d, body: %s", status, body)
	}
	if !hasAuditAction(audit.Items, "remote_access.router_mapping_attempted") || !hasAuditAction(audit.Items, "remote_access.router_mapping_completed") {
		t.Fatalf("expected router mapping audit events, got %#v", audit.Items)
	}

	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{
		"publicPortMode":          "manual",
		"routerAutomationEnabled": false,
	}, &remoteStatus)
	if status != http.StatusOK {
		t.Fatalf("disable automatic mapping status = %d, body: %s", status, body)
	}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/audit-events", nil, &audit)
	if status != http.StatusOK {
		t.Fatalf("audit after remove status = %d, body: %s", status, body)
	}
	if !hasAuditAction(audit.Items, "remote_access.router_mapping_removed") {
		t.Fatalf("expected router mapping removal audit event, got %#v", audit.Items)
	}
}

func TestRemoteAccessClaimStartCallsHosted(t *testing.T) {
	var received map[string]string
	var cancelAuth string
	var claimOperationKey string
	const claimReceipt = "claim-receipt-123"
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/server-claims/claim_hosted_123/result" && r.Method == http.MethodGet {
			if got := r.Header.Get("X-Portico-Claim-Receipt"); got != claimReceipt {
				t.Fatalf("claim result receipt = %q", got)
			}
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("claim result authorization leaked bearer: %q", got)
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"claimId": "claim_hosted_123",
				"status":  "pending",
			})
			return
		}
		if r.URL.Path == "/api/server-claims/claim_hosted_123/cancel" && r.Method == http.MethodPost {
			cancelAuth = r.Header.Get("Authorization")
			if got := r.Header.Get("X-Portico-Claim-Receipt"); got != "" {
				t.Fatalf("claim cancel unexpectedly used response receipt: %q", got)
			}
			writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
			return
		}
		if r.URL.Path != "/api/server-claims" || r.Method != http.MethodPost {
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode hosted request: %v", err)
		}
		claimOperationKey = r.Header.Get("Idempotency-Key")
		writeJSON(w, http.StatusCreated, map[string]any{
			"claimId":             "claim_hosted_123",
			"claimCode":           "ABCD2345",
			"claimToken":          "hosted-claim-token",
			"lostResponseReceipt": claimReceipt,
			"claimUrl":            hostedClaimURL(r),
			"expiresAt":           time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			"status":              "pending",
		})
	}))
	t.Cleanup(hosted.Close)

	serverURL, _, server := newRemoteAccessTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{
		"hostedBaseUrl": hosted.URL,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("settings status = %d, body: %s", status, body)
	}

	var remoteStatus RemoteAccessStatus
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/claim/start", nil, &remoteStatus)
	if status != http.StatusCreated {
		t.Fatalf("claim start status = %d, body: %s", status, body)
	}
	if remoteStatus.Claim == nil || remoteStatus.Claim.ClaimID != "claim_hosted_123" || !remoteStatus.Claim.HostedReady {
		t.Fatalf("unexpected claim: %#v", remoteStatus.Claim)
	}
	if received["serverPublicKey"] == "" || received["serverPublicKeyFingerprint"] == "" {
		t.Fatalf("hosted did not receive server identity: %#v", received)
	}
	if claimOperationKey == "" {
		t.Fatal("hosted claim omitted Idempotency-Key")
	}
	if got := server.secretSetting(remoteAccessClaimReceiptKey); got != claimReceipt {
		t.Fatalf("stored claim receipt = %q", got)
	}
	storedSettings, err := server.loadSettings()
	if err != nil {
		t.Fatalf("load stored settings: %v", err)
	}
	if raw, err := json.Marshal(storedSettings[remoteAccessClaimReceiptKey]); err != nil || strings.Contains(string(raw), claimReceipt) {
		t.Fatalf("claim receipt was not protected in settings: %s err=%v", raw, err)
	}
	var cancelledStatus RemoteAccessStatus
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/claim/cancel", nil, &cancelledStatus)
	if status != http.StatusOK {
		t.Fatalf("claim cancel status = %d, body: %s", status, body)
	}
	if cancelAuth != "Bearer hosted-claim-token" {
		t.Fatalf("claim cancel auth = %q", cancelAuth)
	}
	if got := server.secretSetting(remoteAccessClaimReceiptKey); got != "" {
		t.Fatalf("claim receipt survived cancellation: %q", got)
	}
	if cancelledStatus.Claim != nil {
		t.Fatalf("claim should be cleared after cancel: %#v", cancelledStatus.Claim)
	}
}

func TestRemoteAccessClaimStartReusesOperationAfterAmbiguousFailure(t *testing.T) {
	requestCount := 0
	operationKeys := []string{}
	const claimReceipt = "replayed-claim-receipt"
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/server-claims/claim_replayed/result" && r.Method == http.MethodGet {
			if got := r.Header.Get("X-Portico-Claim-Receipt"); got != claimReceipt {
				t.Fatalf("claim result receipt = %q", got)
			}
			if got := r.Header.Get("Authorization"); got != "" {
				t.Fatalf("claim result authorization leaked bearer: %q", got)
			}
			writeJSON(w, http.StatusOK, map[string]any{"claimId": "claim_replayed", "status": "pending"})
			return
		}
		if r.URL.Path != "/api/server-claims" || r.Method != http.MethodPost {
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
		requestCount++
		operationKeys = append(operationKeys, r.Header.Get("Idempotency-Key"))
		if requestCount == 1 {
			// This models a committed operation whose response was lost or replaced
			// by an intermediary failure. The retry must identify the same operation.
			writeError(w, http.StatusBadGateway, "ambiguous", "response unavailable")
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"claimId": "claim_replayed", "claimCode": "ABCD2345", "claimToken": "hosted-claim-token", "lostResponseReceipt": claimReceipt,
			"claimUrl": hostedClaimURL(r), "expiresAt": time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339), "status": "pending",
		})
	}))
	t.Cleanup(hosted.Close)

	serverURL, _, _ := newRemoteAccessTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{"hostedBaseUrl": hosted.URL}, nil)
	if status != http.StatusOK {
		t.Fatalf("settings status = %d, body: %s", status, body)
	}
	status, _ = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/claim/start", nil, nil)
	if status != http.StatusBadGateway {
		t.Fatalf("first claim status = %d", status)
	}
	var remoteStatus RemoteAccessStatus
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/claim/start", nil, &remoteStatus)
	if status != http.StatusCreated {
		t.Fatalf("retry claim status = %d, body: %s", status, body)
	}
	if len(operationKeys) != 2 || operationKeys[0] == "" || operationKeys[0] != operationKeys[1] {
		t.Fatalf("claim operation keys = %#v", operationKeys)
	}
}

func TestRemoteAccessClaimStartRejectsTokenInClaimURL(t *testing.T) {
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/server-claims" || r.Method != http.MethodPost {
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"claimId":             "claim_leaky",
			"claimCode":           "ABCD2345",
			"claimToken":          "hosted-claim-token",
			"lostResponseReceipt": "leaky-claim-receipt",
			"claimUrl":            "https://hosted.example.test/claim?code=ABCD2345&token=hosted-claim-token",
			"expiresAt":           "2026-05-03T12:10:00Z",
			"status":              "pending",
		})
	}))
	t.Cleanup(hosted.Close)

	serverURL, _, _ := newRemoteAccessTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{
		"hostedBaseUrl": hosted.URL,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("settings status = %d, body: %s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/claim/start", nil, nil)
	if status != http.StatusBadGateway {
		t.Fatalf("leaky claim URL status = %d, body: %s", status, body)
	}
}

func TestRemoteAccessClaimStartRequiresLostResponseReceipt(t *testing.T) {
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/server-claims" || r.Method != http.MethodPost {
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"claimId":    "claim_without_receipt",
			"claimCode":  "NORECEIPT",
			"claimToken": "claim-token-without-receipt",
			"claimUrl":   hostedClaimURL(r),
			"expiresAt":  time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
			"status":     "pending",
		})
	}))
	t.Cleanup(hosted.Close)

	serverURL, _, server := newRemoteAccessTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)
	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{"hostedBaseUrl": hosted.URL}, nil)
	if status != http.StatusOK {
		t.Fatalf("settings status = %d, body: %s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/claim/start", nil, nil)
	if status != http.StatusBadGateway {
		t.Fatalf("missing receipt claim status = %d, body: %s", status, body)
	}
	if server.currentRemoteAccessClaim() != nil || server.secretSetting(remoteAccessClaimTokenKey) != "" || server.secretSetting(remoteAccessClaimReceiptKey) != "" {
		t.Fatalf("incomplete claim response left local claim material: claim=%#v token=%q receipt=%q", server.currentRemoteAccessClaim(), server.secretSetting(remoteAccessClaimTokenKey), server.secretSetting(remoteAccessClaimReceiptKey))
	}
}

func TestRemoteAccessClaimPollingFailsClosedWithoutReceipt(t *testing.T) {
	srv := newRemoteAccessUnitServer(t)
	resultRequests := 0
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resultRequests++
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
	}))
	t.Cleanup(hosted.Close)
	settings, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.HostedBaseURL = hosted.URL
	claim := RemoteAccessClaim{ClaimID: "claim_missing_receipt", Status: "pending", HostedReady: true, ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)}
	if err := srv.saveSecretSetting(remoteAccessClaimTokenKey, "pending-cancel-token"); err != nil {
		t.Fatal(err)
	}
	_, _, err = srv.pollHostedClaim(context.Background(), claim, settings)
	if err == nil || !strings.Contains(err.Error(), "claim receipt is missing") {
		t.Fatalf("missing receipt error = %v", err)
	}
	if resultRequests != 0 {
		t.Fatalf("result endpoint was contacted without a receipt: %d requests", resultRequests)
	}
}

func TestRemoteAccessClaimPollingRejectsExpiredReceiptWithoutBearer(t *testing.T) {
	srv := newRemoteAccessUnitServer(t)
	const claimReceipt = "expired-claim-receipt"
	resultRequests := 0
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resultRequests++
		if r.Header.Get("X-Portico-Claim-Receipt") != claimReceipt {
			t.Fatalf("expired receipt header = %q", r.Header.Get("X-Portico-Claim-Receipt"))
		}
		if r.Header.Get("Authorization") != "" {
			t.Fatalf("expired receipt poll leaked bearer: %q", r.Header.Get("Authorization"))
		}
		writeError(w, http.StatusUnauthorized, "invalid_claim_receipt", "The claim result receipt is invalid or expired.")
	}))
	t.Cleanup(hosted.Close)
	settings, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.HostedBaseURL = hosted.URL
	claim := RemoteAccessClaim{ClaimID: "claim_expired_receipt", Status: "pending", HostedReady: true, ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)}
	if err := srv.saveSecretSetting(remoteAccessClaimTokenKey, "pending-cancel-token"); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveSecretSetting(remoteAccessClaimReceiptKey, claimReceipt); err != nil {
		t.Fatal(err)
	}
	_, _, err = srv.pollHostedClaim(context.Background(), claim, settings)
	if err == nil || !strings.Contains(err.Error(), "receipt is invalid or expired") {
		t.Fatalf("expired receipt error = %v", err)
	}
	if resultRequests != 1 || srv.secretSetting(remoteAccessClaimReceiptKey) != claimReceipt {
		t.Fatalf("expired receipt state requests=%d receipt=%q", resultRequests, srv.secretSetting(remoteAccessClaimReceiptKey))
	}
}

func TestRemoteAccessClaimAcknowledgementRetainsReceiptUntilSuccess(t *testing.T) {
	srv := newRemoteAccessUnitServer(t)
	const claimReceipt = "ack-retry-receipt"
	resultRequests := 0
	ackRequests := 0
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/server-claims/claim_ack_retry/result" && r.Method == http.MethodGet:
			resultRequests++
			if r.Header.Get("X-Portico-Claim-Receipt") != claimReceipt || r.Header.Get("Authorization") != "" {
				t.Fatalf("claim result headers receipt=%q authorization=%q", r.Header.Get("X-Portico-Claim-Receipt"), r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"claimId":          "claim_ack_retry",
				"status":           "claimed",
				"serverCredential": "ack-retry-server-credential",
				"server": map[string]any{
					"id":                "srv_ack_retry",
					"assignedHostname":  "ptc-ackretry.direct.getportico.tv",
					"certificateStatus": "not_requested",
					"preferredAuthMode": "portico",
				},
			})
		case r.URL.Path == "/api/server-claims/claim_ack_retry/result/ack" && r.Method == http.MethodPost:
			ackRequests++
			if r.Header.Get("X-Portico-Claim-Receipt") != claimReceipt || r.Header.Get("Authorization") != "" {
				t.Fatalf("claim ack headers receipt=%q authorization=%q", r.Header.Get("X-Portico-Claim-Receipt"), r.Header.Get("Authorization"))
			}
			settings, settingsErr := srv.remoteAccessSettings()
			if settingsErr != nil || settings.ServerID != "srv_ack_retry" || srv.secretSetting(remoteAccessCredentialKey) != "ack-retry-server-credential" {
				t.Fatalf("claim ack raced local activation: settings=%#v err=%v credential=%q", settings, settingsErr, srv.secretSetting(remoteAccessCredentialKey))
			}
			if ackRequests == 1 {
				writeError(w, http.StatusServiceUnavailable, "temporary", "ack unavailable")
				return
			}
			writeJSON(w, http.StatusOK, map[string]bool{"acknowledged": true})
		case r.URL.Path == "/api/servers/srv_ack_retry/policy-snapshot" && r.Method == http.MethodGet:
			writeError(w, http.StatusServiceUnavailable, "temporary", "policy unavailable")
		case r.URL.Path == "/api/servers/srv_ack_retry/heartbeat" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer ack-retry-server-credential" {
				t.Fatalf("heartbeat authorization = %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "publicConsoleOriginGeneration": 1})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)
	settings, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.HostedBaseURL = hosted.URL
	settings.ClaimStatus = "pending"
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatal(err)
	}
	claim := RemoteAccessClaim{ClaimID: "claim_ack_retry", Status: "pending", HostedReady: true, ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339)}
	if err := srv.saveRemoteAccessClaim(claim); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveSecretSetting(remoteAccessClaimTokenKey, "pending-cancel-token"); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveSecretSetting(remoteAccessClaimReceiptKey, claimReceipt); err != nil {
		t.Fatal(err)
	}

	first, err := srv.remoteAccessStatus()
	if err != nil {
		t.Fatalf("first claim status: %v", err)
	}
	if !first.PorticoConnected || srv.secretSetting(remoteAccessCredentialKey) != "ack-retry-server-credential" {
		t.Fatalf("local activation did not commit before failed ack: status=%#v credential=%q", first.Settings, srv.secretSetting(remoteAccessCredentialKey))
	}
	if got := srv.secretSetting(remoteAccessClaimReceiptKey); got != claimReceipt {
		t.Fatalf("receipt was removed before successful acknowledgement: %q", got)
	}

	restarted := &Server{cfg: srv.cfg, db: srv.db, log: srv.log}
	second, err := restarted.remoteAccessStatus()
	if err != nil {
		t.Fatalf("restart claim status: %v", err)
	}
	if !second.PorticoConnected || ackRequests != 2 || resultRequests < 2 {
		t.Fatalf("restart acknowledgement did not replay safely: connected=%v ackRequests=%d resultRequests=%d", second.PorticoConnected, ackRequests, resultRequests)
	}
	if got := restarted.secretSetting(remoteAccessClaimReceiptKey); got != "" {
		t.Fatalf("receipt survived successful acknowledgement: %q", got)
	}
}

func TestPorticoPrimarySetupClaimCreatesLinkedOwnerWithoutLocalUser(t *testing.T) {
	var certificateCSR string
	var claimAckSeen bool
	var compatibilityHeartbeatSeen atomic.Bool
	var rejectedPreHeartbeatPolicyRequests atomic.Int64
	const claimReceipt = "setup-claim-receipt"
	signingKeySet := testHostedDocumentSigningKeySet(t)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/signing-keys" && r.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, signingKeySet)
		case r.URL.Path == "/api/server-claims" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusCreated, map[string]any{
				"claimId":             "claim_setup_owner",
				"claimCode":           "ABCD2345",
				"claimToken":          "setup-claim-token",
				"lostResponseReceipt": claimReceipt,
				"claimUrl":            hostedClaimURL(r),
				"expiresAt":           time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
				"status":              "pending",
			})
		case r.URL.Path == "/api/server-claims/claim_setup_owner/result" && r.Method == http.MethodGet:
			if r.Header.Get("X-Portico-Claim-Receipt") != claimReceipt {
				t.Fatalf("claim result receipt = %q", r.Header.Get("X-Portico-Claim-Receipt"))
			}
			if r.Header.Get("Authorization") != "" {
				t.Fatalf("claim result authorization leaked bearer: %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"claimId":          "claim_setup_owner",
				"status":           "claimed",
				"serverCredential": "setup-server-credential",
				"server": map[string]any{
					"id":                "srv_setup_owner",
					"assignedHostname":  "ptc-setup.direct.getportico.tv",
					"certificateStatus": "not_requested",
					"preferredAuthMode": "portico",
				},
			})
		case r.URL.Path == "/api/server-claims/claim_setup_owner/result/ack" && r.Method == http.MethodPost:
			claimAckSeen = true
			if r.Header.Get("X-Portico-Claim-Receipt") != claimReceipt {
				t.Fatalf("claim ack receipt = %q", r.Header.Get("X-Portico-Claim-Receipt"))
			}
			if r.Header.Get("Authorization") != "" {
				t.Fatalf("claim ack authorization leaked bearer: %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]bool{"acknowledged": true})
		case r.URL.Path == "/api/servers/srv_setup_owner/heartbeat" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer setup-server-credential" {
				t.Fatalf("heartbeat auth = %q", r.Header.Get("Authorization"))
			}
			var heartbeat struct {
				SoftwareVersion   string                    `json:"softwareVersion"`
				BuildCommit       string                    `json:"buildCommit"`
				APIContractDigest string                    `json:"apiContractDigest"`
				Capabilities      []CompatibilityCapability `json:"capabilities"`
			}
			if err := json.NewDecoder(r.Body).Decode(&heartbeat); err != nil {
				t.Fatalf("decode heartbeat: %v", err)
			}
			if heartbeat.SoftwareVersion == "" || heartbeat.BuildCommit == "" || heartbeat.APIContractDigest == "" || len(heartbeat.Capabilities) == 0 {
				t.Fatalf("heartbeat omitted compatibility identity: %#v", heartbeat)
			}
			compatibilityHeartbeatSeen.Store(true)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "publicConsoleOriginGeneration": 1})
		case r.URL.Path == "/api/servers/srv_setup_owner/policy-snapshot" && r.Method == http.MethodGet:
			if r.Header.Get("Authorization") != "Bearer setup-server-credential" {
				t.Fatalf("policy snapshot auth = %q", r.Header.Get("Authorization"))
			}
			if !compatibilityHeartbeatSeen.Load() {
				rejectedPreHeartbeatPolicyRequests.Add(1)
				writeJSON(w, http.StatusConflict, map[string]any{"code": "server_build_identity_invalid", "detail": "Heartbeat required before policy."})
				return
			}
			writeJSON(w, http.StatusOK, signedTestPolicySnapshot(t, map[string]any{
				"snapshotId": "policy_setup_owner",
				"version":    1,
				"serverId":   "srv_setup_owner",
				"members": []map[string]any{{
					"id":          "mem_setup_owner",
					"serverId":    "srv_setup_owner",
					"userId":      "usr_setup_owner",
					"email":       "setup-owner@example.test",
					"displayName": "Setup Owner",
					"role":        "owner",
					"status":      "active",
				}},
			}))
		case r.URL.Path == "/api/servers/srv_setup_owner/policy-sync-ack" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer setup-server-credential" {
				t.Fatalf("policy sync ack auth = %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "publicConsoleOriginGeneration": 1})
		case r.URL.Path == "/api/servers/srv_setup_owner/members/sync" && r.Method == http.MethodGet:
			if r.Header.Get("Authorization") != "Bearer setup-server-credential" {
				t.Fatalf("member sync auth = %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{
				"id":          "mem_setup_owner",
				"serverId":    "srv_setup_owner",
				"userId":      "usr_setup_owner",
				"email":       "setup-owner@example.test",
				"displayName": "Setup Owner",
				"role":        "owner",
				"status":      "active",
			}}, "total": 1})
		case r.URL.Path == "/api/servers/srv_setup_owner/certificate-orders" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer setup-server-credential" {
				t.Fatalf("certificate order auth = %q", r.Header.Get("Authorization"))
			}
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode certificate order: %v", err)
			}
			certificateCSR = req["csrPem"]
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_setup", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_setup_owner/certificate-orders/certord_setup/finalize" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer setup-server-credential" {
				t.Fatalf("certificate finalize auth = %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                  "certord_setup",
				"status":              "valid",
				"certificateChainPem": certificateForCSR(t, certificateCSR, time.Now().UTC().Add(60*24*time.Hour)),
				"expiresAt":           time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.RFC3339),
			})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir, HostedDocumentPublicKeys: testHostedDocumentPublicKeys()}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	settings, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatalf("remote settings: %v", err)
	}
	settings.HostedBaseURL = hosted.URL
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "http://localhost:32500/api/auth/portico-setup/claim/start", strings.NewReader(`{"serverName":"Setup Server"}`))
	request.RemoteAddr = "127.0.0.1:41234"
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	srv.handlePorticoSetupClaimStart(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("start setup claim status = %d body=%s", response.Code, response.Body.String())
	}
	var setupResponse struct {
		RemoteAccess RemoteAccessStatus `json:"remoteAccess"`
	}
	if err := json.NewDecoder(response.Body).Decode(&setupResponse); err != nil {
		t.Fatalf("decode setup claim response: %v", err)
	}
	status := setupResponse.RemoteAccess
	if status.Claim == nil || status.Claim.ClaimID != "claim_setup_owner" || !status.Claim.HostedReady {
		t.Fatalf("unexpected setup claim: %#v", status.Claim)
	}
	claimURL, err := url.Parse(status.Claim.ClaimURL)
	if err != nil {
		t.Fatalf("parse setup claim URL: %v", err)
	}
	if got := claimURL.Query().Get("returnUrl"); got != "http://localhost:32500/?porticoSetup=continue" {
		t.Fatalf("setup return URL = %q", got)
	}

	statusRequest := httptest.NewRequest(http.MethodGet, "http://localhost:32500/api/auth/portico-setup/status", nil)
	statusRequest.RemoteAddr = "[::1]:41235"
	statusResponse := httptest.NewRecorder()
	srv.handlePorticoSetupStatus(statusResponse, statusRequest)
	if statusResponse.Code != http.StatusOK {
		t.Fatalf("setup status = %d body=%s", statusResponse.Code, statusResponse.Body.String())
	}
	var activation struct {
		SetupRequired bool               `json:"setupRequired"`
		RemoteAccess  RemoteAccessStatus `json:"remoteAccess"`
	}
	if err := json.NewDecoder(statusResponse.Body).Decode(&activation); err != nil {
		t.Fatalf("decode setup activation status: %v", err)
	}
	status = activation.RemoteAccess
	if activation.SetupRequired || !status.PorticoConnected || status.Settings.ServerID != "srv_setup_owner" {
		t.Fatalf("expected completed local activation, got setupRequired=%v settings=%#v", activation.SetupRequired, status.Settings)
	}
	if !compatibilityHeartbeatSeen.Load() {
		t.Fatal("setup completed without publishing a compatibility heartbeat")
	}
	if rejected := rejectedPreHeartbeatPolicyRequests.Load(); rejected != 0 {
		t.Fatalf("policy requested before compatibility heartbeat %d time(s)", rejected)
	}
	if !claimAckSeen || srv.secretSetting(remoteAccessClaimReceiptKey) != "" {
		t.Fatalf("claim acknowledgement/receipt cleanup incomplete: ack=%v receipt=%q", claimAckSeen, srv.secretSetting(remoteAccessClaimReceiptKey))
	}
	if certificateCSR != "" {
		t.Fatal("first-run activation waited for remote certificate provisioning")
	}
	if err := srv.ensurePorticoSetupOwnerProfile(); err != nil {
		t.Fatalf("ensure owner profile: %v", err)
	}
	user, err := srv.userForPorticoMembership(RemoteAccessMember{PorticoMembershipID: "mem_setup_owner", PorticoUserID: "usr_setup_owner", Status: "active"})
	if err != nil {
		t.Fatalf("get linked owner: %v", err)
	}
	if user.Role != "owner" || user.AuthOrigin != "portico" || user.PorticoUserID != "usr_setup_owner" || user.HasLocalPassword {
		t.Fatalf("unexpected linked owner profile: %#v", user)
	}
}

func TestPorticoSetupActivationRecoveryPublishesHeartbeatBeforeOwnerPolicy(t *testing.T) {
	srv := newRemoteAccessUnitServer(t)
	srv.cfg.HostedDocumentPublicKeys = testHostedDocumentPublicKeys()
	var compatibilityHeartbeatSeen atomic.Bool
	var rejectedPreHeartbeatPolicyRequests atomic.Int64
	signingKeySet := testHostedDocumentSigningKeySet(t)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/signing-keys" && r.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, signingKeySet)
		case r.URL.Path == "/api/servers/srv_setup_recovery/heartbeat" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer setup-recovery-credential" {
				t.Fatalf("heartbeat authorization = %q", r.Header.Get("Authorization"))
			}
			var heartbeat struct {
				SoftwareVersion   string                    `json:"softwareVersion"`
				BuildCommit       string                    `json:"buildCommit"`
				APIContractDigest string                    `json:"apiContractDigest"`
				Capabilities      []CompatibilityCapability `json:"capabilities"`
			}
			if err := json.NewDecoder(r.Body).Decode(&heartbeat); err != nil {
				t.Fatalf("decode heartbeat: %v", err)
			}
			if heartbeat.SoftwareVersion == "" || heartbeat.BuildCommit == "" || heartbeat.APIContractDigest == "" || len(heartbeat.Capabilities) == 0 {
				t.Fatalf("heartbeat omitted compatibility identity: %#v", heartbeat)
			}
			compatibilityHeartbeatSeen.Store(true)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "publicConsoleOriginGeneration": 1})
		case r.URL.Path == "/api/servers/srv_setup_recovery/policy-snapshot" && r.Method == http.MethodGet:
			if !compatibilityHeartbeatSeen.Load() {
				rejectedPreHeartbeatPolicyRequests.Add(1)
				writeJSON(w, http.StatusConflict, map[string]any{"code": "server_build_identity_invalid", "detail": "Heartbeat required before policy."})
				return
			}
			writeJSON(w, http.StatusOK, signedTestPolicySnapshot(t, map[string]any{
				"snapshotId": "policy_setup_recovery",
				"version":    1,
				"serverId":   "srv_setup_recovery",
				"members": []map[string]any{{
					"id": "mem_setup_recovery", "serverId": "srv_setup_recovery", "userId": "usr_setup_recovery",
					"email": "recovery-owner@example.test", "displayName": "Recovery Owner", "role": "owner", "status": "active",
				}},
			}))
		case r.URL.Path == "/api/servers/srv_setup_recovery/policy-sync-ack" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "publicConsoleOriginGeneration": 1})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	settings, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.ClaimStatus = "claimed"
	settings.ServerID = "srv_setup_recovery"
	settings.HostedBaseURL = hosted.URL
	settings.PreferredRemoteAuthMode = "portico"
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "setup-recovery-credential"); err != nil {
		t.Fatal(err)
	}

	if err := srv.finishPorticoSetupActivation(context.Background(), RemoteAccessStatus{Settings: settings, PorticoConnected: true}); err != nil {
		t.Fatalf("recover setup activation: %v", err)
	}
	if !compatibilityHeartbeatSeen.Load() {
		t.Fatal("recovery completed without publishing a compatibility heartbeat")
	}
	if rejected := rejectedPreHeartbeatPolicyRequests.Load(); rejected != 0 {
		t.Fatalf("recovery requested owner policy before compatibility heartbeat %d time(s)", rejected)
	}
	if err := srv.ensurePorticoSetupOwnerProfile(); err != nil {
		t.Fatalf("ensure recovered owner profile: %v", err)
	}
}

func TestPorticoSetupRoutesRequireActualLoopbackPeer(t *testing.T) {
	srv := &Server{}
	for _, test := range []struct {
		method string
		path   string
		call   func(http.ResponseWriter, *http.Request)
	}{
		{http.MethodPost, "/api/auth/portico-setup/claim/start", srv.handlePorticoSetupClaimStart},
		{http.MethodGet, "/api/auth/portico-setup/status", srv.handlePorticoSetupStatus},
	} {
		request := httptest.NewRequest(test.method, "http://localhost:32500"+test.path, nil)
		request.RemoteAddr = "192.168.1.50:54321"
		request.Header.Set("Origin", "http://localhost:32500")
		request.Header.Set("X-Forwarded-For", "127.0.0.1")
		response := httptest.NewRecorder()
		test.call(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s from LAN peer status = %d body=%s", test.path, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "claimCode") {
			t.Fatalf("%s exposed claim details to LAN peer: %s", test.path, response.Body.String())
		}
	}
}

func TestRemoteAccessStatusCompletesClaimAndSendsHeartbeat(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	const installationID = "remote-outage-test-0001"
	profile := HostedProfileSnapshot{
		ExternalProfileID: "prf_usr_portico_owner", AccountID: "usr_portico_owner", DisplayName: "Owner",
		IsPrimary: true, IsAccountAdmin: true, PolicyUpdatedAt: now.Add(-time.Minute),
		Restrictions: defaultProfileRestrictions(),
	}
	rawSelectionEnvelope := signedHostedProfileSelectionEnvelope(t, testHostedDocumentPrivateKey(), HostedProfileSelectionEnvelope{
		Version: hostedProfileSelectionAssertionVersion, AssertionID: "psa_portico_owner_attach", Audience: hostedDocumentAudience,
		AccountID: "usr_portico_owner", ProfileID: profile.ExternalProfileID, ServerID: "srv_portico_done",
		DeviceID: "dev_portico_owner", InstallationID: installationID, AccountRevision: 1,
		Profiles: []HostedProfileSnapshot{profile}, IssuedAt: now.Format(time.RFC3339Nano), ExpiresAt: now.Add(2 * time.Minute).Format(time.RFC3339Nano),
		SignatureAlgorithm: hostedSignatureAlgorithm, SignatureKeyID: testHostedDocumentKeyID,
	})
	var selectionEnvelope HostedProfileSelectionEnvelope
	if err := json.Unmarshal(rawSelectionEnvelope, &selectionEnvelope); err != nil {
		t.Fatalf("decode profile selection envelope: %v", err)
	}
	var heartbeatAuth string
	var heartbeatSeen bool
	var introspectionUnavailable bool
	var policySnapshotUnavailable bool
	var certificateCSR string
	var certificateOrderCount int
	var certificateFinalizeCount int
	const claimReceipt = "completed-claim-receipt"
	certificateProvisioned := make(chan struct{})
	signingKeySet := testHostedDocumentSigningKeySet(t)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/signing-keys" && r.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, signingKeySet)
		case r.URL.Path == "/api/server-claims" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusCreated, map[string]any{
				"claimId":             "claim_hosted_done",
				"claimCode":           "ABCD2345",
				"claimToken":          "hosted-claim-token-done",
				"lostResponseReceipt": claimReceipt,
				"claimUrl":            hostedClaimURL(r),
				"expiresAt":           time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
				"status":              "pending",
			})
		case r.URL.Path == "/api/server-claims/claim_hosted_done/result" && r.Method == http.MethodGet:
			if r.Header.Get("X-Portico-Claim-Receipt") != claimReceipt {
				t.Fatalf("claim result receipt = %q", r.Header.Get("X-Portico-Claim-Receipt"))
			}
			if r.Header.Get("Authorization") != "" {
				t.Fatalf("claim result authorization leaked bearer: %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"claimId":          "claim_hosted_done",
				"status":           "claimed",
				"serverCredential": "server-credential-done",
				"server": map[string]any{
					"id":                "srv_portico_done",
					"assignedHostname":  "ptc-cccccccccccccccccccc.direct.getportico.tv",
					"certificateStatus": "not_requested",
					"preferredAuthMode": "portico",
				},
			})
		case r.URL.Path == "/api/server-claims/claim_hosted_done/result/ack" && r.Method == http.MethodPost:
			if r.Header.Get("X-Portico-Claim-Receipt") != claimReceipt {
				t.Fatalf("claim ack receipt = %q", r.Header.Get("X-Portico-Claim-Receipt"))
			}
			if r.Header.Get("Authorization") != "" {
				t.Fatalf("claim ack authorization leaked bearer: %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]bool{"acknowledged": true})
		case r.URL.Path == "/api/servers/srv_portico_done/heartbeat" && r.Method == http.MethodPost:
			if !heartbeatSeen {
				heartbeatAuth = r.Header.Get("Authorization")
				heartbeatSeen = true
			}
			var req map[string]any
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode heartbeat: %v", err)
			}
			if enabled, _ := req["remoteAccessEnabled"].(bool); !enabled {
				t.Fatalf("local detach must not send a disabled heartbeat to Hosted Services")
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "publicConsoleOriginGeneration": 1})
		case r.URL.Path == "/api/servers/srv_portico_done/policy-snapshot" && r.Method == http.MethodGet:
			if r.Header.Get("Authorization") != "Bearer server-credential-done" {
				t.Fatalf("policy snapshot auth = %q", r.Header.Get("Authorization"))
			}
			if policySnapshotUnavailable {
				writeError(w, http.StatusServiceUnavailable, "hosted_unavailable", "Policy snapshot is unavailable.")
				return
			}
			writeJSON(w, http.StatusOK, signedTestPolicySnapshot(t, map[string]any{
				"snapshotId": "policy_hosted_done",
				"version":    1,
				"serverId":   "srv_portico_done",
				"members": []map[string]any{{
					"id":          "mem_portico_owner",
					"serverId":    "srv_portico_done",
					"userId":      "usr_portico_owner",
					"email":       "owner@example.test",
					"displayName": "Owner",
					"role":        "owner",
					"status":      "active",
				}, {
					"id":          "mem_portico_user",
					"serverId":    "srv_portico_done",
					"userId":      "usr_portico_user",
					"email":       "user@example.test",
					"displayName": "User",
					"role":        "user",
					"status":      "active",
				}},
			}))
		case r.URL.Path == "/api/servers/srv_portico_done/policy-sync-ack" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-done" {
				t.Fatalf("policy sync ack auth = %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		case r.URL.Path == "/api/servers/srv_portico_done/members/sync" && r.Method == http.MethodGet:
			if r.Header.Get("Authorization") != "Bearer server-credential-done" {
				t.Fatalf("member sync auth = %q", r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{"items": []map[string]any{{
				"id":          "mem_portico_owner",
				"serverId":    "srv_portico_done",
				"userId":      "usr_portico_owner",
				"email":       "owner@example.test",
				"displayName": "Owner",
				"role":        "owner",
				"status":      "active",
			}, {
				"id":          "mem_portico_user",
				"serverId":    "srv_portico_done",
				"userId":      "usr_portico_user",
				"email":       "user@example.test",
				"displayName": "User",
				"role":        "user",
				"status":      "active",
			}}, "total": 2})
		case r.URL.Path == "/api/servers/srv_portico_done/certificate-orders" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-done" {
				t.Fatalf("certificate order auth = %q", r.Header.Get("Authorization"))
			}
			certificateOrderCount++
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode certificate order: %v", err)
			}
			certificateCSR = req["csrPem"]
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_done", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_portico_done/certificate-orders/certord_done/finalize" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-done" {
				t.Fatalf("certificate finalize auth = %q", r.Header.Get("Authorization"))
			}
			certificateFinalizeCount++
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                  "certord_done",
				"status":              "valid",
				"certificateChainPem": certificateForCSR(t, certificateCSR, time.Now().UTC().Add(60*24*time.Hour)),
				"expiresAt":           time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.RFC3339),
			})
			close(certificateProvisioned)
		case r.URL.Path == "/api/servers/srv_portico_done/portico-sessions/introspect" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-done" {
				t.Fatalf("introspect auth = %q", r.Header.Get("Authorization"))
			}
			if introspectionUnavailable {
				writeError(w, http.StatusServiceUnavailable, "hosted_unavailable", "Hosted introspection is unavailable.")
				return
			}
			var req struct {
				AccessToken       string                         `json:"accessToken"`
				SelectionEnvelope HostedProfileSelectionEnvelope `json:"selectionEnvelope"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode introspect: %v", err)
			}
			if req.AccessToken != "ptc_clt_test" || req.SelectionEnvelope.AssertionID != selectionEnvelope.AssertionID {
				t.Fatalf("unexpected introspection request: %#v", req)
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"active":            true,
				"deviceId":          "dev_portico_owner",
				"selectionEnvelope": selectionEnvelope,
				"member": map[string]any{
					"id":          "mem_portico_owner",
					"serverId":    "srv_portico_done",
					"userId":      "usr_portico_owner",
					"email":       "owner@example.test",
					"displayName": "Owner",
					"role":        "owner",
					"status":      "active",
				},
			})
		case r.URL.Path == "/api/servers/srv_portico_done/profile-selection-exchanges" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-done" {
				t.Fatalf("profile exchange auth = %q", r.Header.Get("Authorization"))
			}
			var req struct {
				SelectionEnvelope HostedProfileSelectionEnvelope `json:"selectionEnvelope"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.SelectionEnvelope.AssertionID != selectionEnvelope.AssertionID {
				t.Fatalf("unexpected profile exchange request: %#v err=%v", req, err)
			}
			writeJSON(w, http.StatusOK, req.SelectionEnvelope)
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	serverURL, _, server := newRemoteAccessTestServer(t)
	backgroundCtx, backgroundCancel := context.WithCancel(context.Background())
	server.backgroundCtx = backgroundCtx
	t.Cleanup(func() {
		backgroundCancel()
		server.closeOwnedAsync()
	})
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{"hostedBaseUrl": hosted.URL, "enabled": true}, nil)
	if status != http.StatusOK {
		t.Fatalf("settings status = %d, body: %s", status, body)
	}

	var remoteStatus RemoteAccessStatus
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/claim/start", nil, &remoteStatus)
	if status != http.StatusCreated {
		t.Fatalf("claim start status = %d, body: %s", status, body)
	}
	if remoteStatus.Settings.ClaimStatus != "claimed" || remoteStatus.Settings.ServerID != "srv_portico_done" {
		t.Fatalf("expected claimed settings, got %#v", remoteStatus.Settings)
	}
	if strings.Contains(body, "hosted-claim-token-done") || strings.Contains(body, "server-credential-done") {
		t.Fatalf("remote access status exposed hosted secret material: %s", body)
	}
	if remoteStatus.Settings.AssignedHostname != "ptc-cccccccccccccccccccc.direct.getportico.tv" {
		t.Fatalf("hostname = %q", remoteStatus.Settings.AssignedHostname)
	}
	if got := server.secretSetting(remoteAccessClaimReceiptKey); got != "" {
		t.Fatalf("claim receipt survived successful acknowledgement: %q", got)
	}
	select {
	case <-certificateProvisioned:
	case <-time.After(3 * time.Second):
		t.Fatal("post-claim certificate provisioning did not run asynchronously")
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/remote-access/status", nil, &remoteStatus)
		if status != http.StatusOK {
			t.Fatalf("post-claim status = %d body=%s", status, body)
		}
		if remoteStatus.Settings.CertificateStatus == "valid" && remoteStatus.Settings.CertificateExpiresAt != "" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected certificate to be requested after claim, got %#v", remoteStatus.Settings)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if certificateOrderCount != 1 || certificateFinalizeCount != 1 {
		t.Fatalf("certificateOrderCount=%d certificateFinalizeCount=%d", certificateOrderCount, certificateFinalizeCount)
	}
	status, body = doJSON(t, http.DefaultClient, http.MethodGet, serverURL+"/api/remote-access/health", nil, nil)
	if status != http.StatusOK {
		t.Fatalf("remote health status = %d, body: %s", status, body)
	}
	for _, secret := range []string{"hosted-claim-token-done", "server-credential-done", "certificateChainPem", "privateKey"} {
		if strings.Contains(body, secret) {
			t.Fatalf("remote health exposed %q in body: %s", secret, body)
		}
	}
	var network NetworkConnectionInfo
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/network/connection-info", nil, &network)
	if status != http.StatusOK {
		t.Fatalf("network connection status = %d, body: %s", status, body)
	}
	if network.Placeholder || network.RemoteAccess.Placeholder {
		t.Fatalf("network remote access should be runtime-backed after hosted integration: %#v", network)
	}
	if network.RemoteAccess.Status != "enabled" || network.RemoteAccess.Discovery != "claimed" || network.RemoteAccess.DynamicDNS != "ptc-cccccccccccccccccccc.direct.getportico.tv" {
		t.Fatalf("unexpected network remote access state: %#v", network.RemoteAccess)
	}
	var identity SystemIdentityResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/system/identity", nil, &identity)
	if status != http.StatusOK {
		t.Fatalf("identity status = %d, body: %s", status, body)
	}
	if !identity.Claimed {
		t.Fatalf("system identity should reflect claimed remote access: %#v", identity)
	}
	if !heartbeatSeen || heartbeatAuth != "Bearer server-credential-done" {
		t.Fatalf("heartbeatSeen=%v auth=%q", heartbeatSeen, heartbeatAuth)
	}
	if len(remoteStatus.PorticoMembers) != 2 {
		t.Fatalf("expected synced Portico member, got %#v", remoteStatus.PorticoMembers)
	}
	memberByID := func(id string) (RemoteAccessMember, bool) {
		for _, member := range remoteStatus.PorticoMembers {
			if member.PorticoMembershipID == id {
				return member, true
			}
		}
		return RemoteAccessMember{}, false
	}
	var managementAuth AuthMeResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/auth/me", nil, &managementAuth)
	if status != http.StatusOK || !managementAuth.Authenticated || managementAuth.User == nil || managementAuth.User.Role != "owner" || managementAuth.Authority != "local" {
		t.Fatalf("claim synchronization displaced the interactive owner: status=%d body=%s user=%#v", status, body, managementAuth.User)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/members/mem_portico_user", map[string]any{
		"permissionTemplate": map[string]any{"permissions": map[string]bool{"playMedia": true}},
	}, nil)
	if status != http.StatusBadRequest {
		t.Fatalf("local member route accepted Hosted permission mutation: status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodDelete, serverURL+"/api/remote-access/members/mem_portico_user", nil, nil)
	if status != http.StatusMethodNotAllowed {
		t.Fatalf("local member route accepted Hosted member deletion: status=%d body=%s", status, body)
	}
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/remote-access/status", nil, &remoteStatus)
	ownerMember, ownerFound := memberByID("mem_portico_owner")
	if status != http.StatusOK || !ownerFound || ownerMember.Role != "owner" {
		t.Fatalf("failed Cloud mutation changed local policy: status=%d body=%s members=%#v", status, body, remoteStatus.PorticoMembers)
	}
	policySnapshotUnavailable = true
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/policy-sync", nil, nil)
	if status != http.StatusBadGateway || !strings.Contains(body, "portico_policy_sync_failed") {
		t.Fatalf("policy sync outage status=%d body=%s", status, body)
	}
	policySnapshotUnavailable = false
	var auth AuthMeResponse
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/auth/me", nil, &auth)
	if status != http.StatusOK || auth.User == nil {
		t.Fatalf("auth/me status = %d, body: %s", status, body)
	}
	status, body = doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/members/mem_portico_owner", map[string]any{"localUserId": auth.User.ID}, &remoteStatus)
	if status != http.StatusOK {
		t.Fatalf("member map status = %d, body: %s", status, body)
	}
	if remoteStatus.PorticoMembers[0].LocalUserID != auth.User.ID {
		t.Fatalf("expected mapped member, got %#v", remoteStatus.PorticoMembers[0])
	}
	var porticoCredentials NativeSessionCredentials
	status, body = performProtectedPorticoAttachmentRequest(t, server, PorticoSessionAttachRequest{
		AccessToken: "ptc_clt_test", SelectionEnvelope: selectionEnvelope, InstallationID: installationID,
		DeviceName: "Remote Outage Test", App: "Portico Test", Platform: "TestOS",
	}, &porticoCredentials)
	if status != http.StatusCreated || porticoCredentials.AccessToken == "" {
		t.Fatalf("Portico attach status=%d body=%s credentials=%#v", status, body, porticoCredentials)
	}
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("Portico auth request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+porticoCredentials.AccessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Portico auth request failed: %v", err)
	}
	var porticoAuth AuthMeResponse
	if err := json.NewDecoder(resp.Body).Decode(&porticoAuth); err != nil {
		t.Fatalf("decode Portico auth: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !porticoAuth.Authenticated || porticoAuth.User == nil || porticoAuth.User.ID != auth.User.ID {
		t.Fatalf("unexpected Portico auth status=%d body=%#v", resp.StatusCode, porticoAuth)
	}
	introspectionUnavailable = true
	status, body = doJSON(t, client, http.MethodGet, serverURL+"/api/auth/me", nil, &auth)
	if status != http.StatusOK || !auth.Authenticated || auth.User == nil {
		t.Fatalf("existing local session should survive Hosted Services outage, status = %d, body: %s", status, body)
	}
	req, err = http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("Hosted Services outage auth request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+porticoCredentials.AccessToken)
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Hosted Services outage auth request failed: %v", err)
	}
	var outageAuth AuthMeResponse
	if err := json.NewDecoder(resp.Body).Decode(&outageAuth); err != nil {
		t.Fatalf("decode Hosted Services outage auth: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !outageAuth.Authenticated || outageAuth.User == nil {
		t.Fatalf("server-local Portico auth should survive Hosted Services outage, status=%d body=%#v", resp.StatusCode, outageAuth)
	}
	var outageRefresh NativeSessionCredentials
	status, body = doJSON(t, http.DefaultClient, http.MethodPost, serverURL+"/api/auth/sessions/refresh", NativeSessionRefreshRequest{RefreshToken: porticoCredentials.RefreshToken, RotationKey: strings.Repeat("A", 43)}, &outageRefresh)
	if status != http.StatusOK || outageRefresh.AccessToken == "" || outageRefresh.RefreshToken == "" {
		t.Fatalf("server-local Portico refresh should survive Hosted Services outage, status=%d body=%s credentials=%#v", status, body, outageRefresh)
	}
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/unclaim", nil, &remoteStatus)
	if status != http.StatusOK {
		t.Fatalf("unclaim status = %d, body: %s", status, body)
	}
	if remoteStatus.Settings.ServerID != "" || remoteStatus.Settings.ClaimStatus != "not_claimed" {
		t.Fatalf("expected unclaimed settings, got %#v", remoteStatus.Settings)
	}
}

func TestRemoteAccessCertificateRenewRequestsHostedCertificate(t *testing.T) {
	var sawOrder bool
	var sawFinalize bool
	var csrPEM string
	var csrDNSNames []string
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_cert/certificate-orders" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-cert" {
				t.Fatalf("order auth = %q", r.Header.Get("Authorization"))
			}
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode cert order: %v", err)
			}
			block, _ := pem.Decode([]byte(req["csrPem"]))
			if block == nil || block.Type != "CERTIFICATE REQUEST" {
				http.Error(w, "invalid csr pem", http.StatusBadRequest)
				return
			}
			csr, err := x509.ParseCertificateRequest(block.Bytes)
			if err != nil {
				http.Error(w, "invalid csr", http.StatusBadRequest)
				return
			}
			csrPEM = req["csrPem"]
			csrDNSNames = append(csrDNSNames, csr.DNSNames...)
			sawOrder = true
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_1", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_cert/certificate-orders/certord_1/finalize" && r.Method == http.MethodPost:
			if r.Header.Get("Authorization") != "Bearer server-credential-cert" {
				t.Fatalf("finalize auth = %q", r.Header.Get("Authorization"))
			}
			sawFinalize = true
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                  "certord_1",
				"status":              "valid",
				"certificateChainPem": certificateForCSR(t, csrPEM, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)),
				"expiresAt":           "2026-07-01T00:00:00Z",
			})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	serverURL, appDataDir, _ := newRemoteAccessTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{
		"enabled":          true,
		"hostedBaseUrl":    hosted.URL,
		"publicPortMode":   "manual",
		"manualPublicPort": 32400,
	}, nil)
	if status != http.StatusOK {
		t.Fatalf("settings status = %d, body: %s", status, body)
	}
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_cert",
		AssignedHostname:        "ptc-dddddddddddddddddddd.direct.getportico.tv",
		PublicPortMode:          "manual",
		ManualPublicPort:        32400,
		PreferredRemoteAuthMode: "local",
		CertificateStatus:       "not_requested",
		LANDiscoveryEnabled:     true,
	}
	db := openExistingTestDB(t, appDataDir)
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-cert"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close settings database: %v", err)
	}
	loginUser(t, client, serverURL)
	var remoteStatus RemoteAccessStatus
	status, body = doJSON(t, client, http.MethodPost, serverURL+"/api/remote-access/certificates/renew", nil, &remoteStatus)
	if status != http.StatusOK {
		t.Fatalf("renew status = %d, body: %s", status, body)
	}
	if !sawOrder || !sawFinalize {
		t.Fatalf("sawOrder=%v sawFinalize=%v", sawOrder, sawFinalize)
	}
	if len(csrDNSNames) != 1 || csrDNSNames[0] != "*.ptc-dddddddddddddddddddd.direct.getportico.tv" {
		t.Fatalf("csr dns names = %#v", csrDNSNames)
	}
	if remoteStatus.Settings.CertificateStatus != "valid" {
		t.Fatalf("certificate status = %q", remoteStatus.Settings.CertificateStatus)
	}
	generations, err := srv.publishedCertificateGenerations()
	if err != nil || len(generations) != 1 {
		t.Fatalf("published certificate generations = %#v, error = %v", generations, err)
	}
	generationDir := filepath.Join(appDataDir, "remote-access", "certificate-generations", generations[0])
	for _, name := range []string{"certificate-key.pem", "certificate-chain.pem"} {
		info, statErr := os.Stat(filepath.Join(generationDir, name))
		if statErr != nil {
			t.Fatalf("published certificate %s missing: %v", name, statErr)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("published certificate %s mode = %o", name, info.Mode().Perm())
		}
	}
}

func TestRemoteAccessCertificateOrderPollsQueuedUntilValid(t *testing.T) {
	chdirRepoRoot(t)
	var mu sync.Mutex
	createCount, finalizeCount, getCount := 0, 0, 0
	csrPEM := ""
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/api/servers/srv_async_cert/certificate-orders" && r.Method == http.MethodPost:
			createCount++
			if r.Header.Get("Idempotency-Key") == "" {
				t.Fatal("certificate create omitted idempotency key")
			}
			var body struct {
				CSRPem string `json:"csrPem"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode CSR: %v", err)
			}
			csrPEM = body.CSRPem
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_async", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_async_cert/certificate-orders/certord_async/finalize" && r.Method == http.MethodPost:
			finalizeCount++
			writeJSON(w, http.StatusAccepted, map[string]any{"id": "certord_async", "status": "queued"})
		case r.URL.Path == "/api/servers/srv_async_cert/certificate-orders/certord_async" && r.Method == http.MethodGet:
			getCount++
			if getCount == 1 {
				writeJSON(w, http.StatusOK, map[string]any{"id": "certord_async", "status": "leased"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"id": "certord_async", "status": "valid",
				"certificateChainPem": certificateForCSR(t, csrPEM, time.Now().UTC().Add(60*24*time.Hour)),
				"expiresAt":           time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.RFC3339),
			})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{AppDataDir: appDataDir, DatabasePath: filepath.Join(appDataDir, "portico.db"), WebDistDir: filepath.Join("web", "dist"), SampleMediaURL: "https://media.example.test/sample.mp4"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	settings := RemoteAccessSettings{Enabled: true, HostedBaseURL: hosted.URL, ClaimStatus: "claimed", ServerID: "srv_async_cert", AssignedHostname: "ptc-asyncaaaaaaaaaaaaaaa.direct.getportico.tv", PublicPortMode: "manual", ManualPublicPort: 32500, PreferredRemoteAuthMode: "portico", CertificateStatus: "not_requested"}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-async"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	updated, err := srv.ensureRemoteAccessCertificateFresh(context.Background(), settings)
	if err != nil {
		t.Fatalf("ensure certificate: %v", err)
	}
	if updated.CertificateStatus != "valid" || createCount != 1 || finalizeCount != 1 || getCount != 2 {
		t.Fatalf("updated=%#v create=%d finalize=%d get=%d", updated, createCount, finalizeCount, getCount)
	}
	if _, err := os.Stat(srv.pendingCertificateOrderPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("completed pending order was not removed: %v", err)
	}
}

func TestRemoteAccessCertificateOrderResumesAfterRestartWithoutCreate(t *testing.T) {
	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{AppDataDir: appDataDir, DatabasePath: filepath.Join(appDataDir, "portico.db"), WebDistDir: filepath.Join("web", "dist"), SampleMediaURL: "https://media.example.test/sample.mp4"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	settings := RemoteAccessSettings{Enabled: true, ClaimStatus: "claimed", ServerID: "srv_resume_cert", AssignedHostname: "ptc-resumeaaaaaaaaaaaaaa.direct.getportico.tv", PublicPortMode: "manual", ManualPublicPort: 32500, PreferredRemoteAuthMode: "portico", CertificateStatus: "not_requested"}
	certificateHostname := remoteAccessCertificateHostname(settings.AssignedHostname)
	privateKey, csr, err := (&Server{}).generateCertificateCSR(certificateHostname)
	if err != nil {
		t.Fatalf("generate CSR: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	pending := remoteAccessPendingCertificateOrder{OrderID: "certord_resume", Hostname: certificateHostname, PrivateKeyPEM: string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes})), CSRPEM: string(csr)}
	createCount := 0
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_resume_cert/certificate-orders":
			createCount++
			t.Fatal("resume created a replacement certificate order")
		case r.URL.Path == "/api/servers/srv_resume_cert/certificate-orders/certord_resume/finalize" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusAccepted, map[string]any{"id": "certord_resume", "status": "queued"})
		case r.URL.Path == "/api/servers/srv_resume_cert/certificate-orders/certord_resume" && r.Method == http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]any{"id": "certord_resume", "status": "valid", "certificateChainPem": certificateForCSR(t, string(csr), time.Now().UTC().Add(60*24*time.Hour)), "expiresAt": time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.RFC3339)})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)
	settings.HostedBaseURL = hosted.URL
	first := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := first.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := first.saveSecretSetting(remoteAccessCredentialKey, "server-credential-resume"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	if err := first.savePendingCertificateOrder(pending); err != nil {
		t.Fatalf("save pending order: %v", err)
	}
	// A new Server value with the same app-data directory models process restart.
	restarted := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if _, err := restarted.ensureRemoteAccessCertificateFresh(context.Background(), settings); err != nil {
		t.Fatalf("resume certificate: %v", err)
	}
	if createCount != 0 {
		t.Fatalf("create count = %d", createCount)
	}
}

func TestRemoteAccessCertificateProvisioningCoalescesConcurrentRepairs(t *testing.T) {
	chdirRepoRoot(t)
	var mu sync.Mutex
	createCount := 0
	csrPEM := ""
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/api/servers/srv_single_cert/certificate-orders" && r.Method == http.MethodPost:
			createCount++
			var body struct {
				CSRPem string `json:"csrPem"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode CSR: %v", err)
			}
			csrPEM = body.CSRPem
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_single", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_single_cert/certificate-orders/certord_single/finalize" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusOK, map[string]any{"id": "certord_single", "status": "valid", "certificateChainPem": certificateForCSR(t, csrPEM, time.Now().UTC().Add(60*24*time.Hour)), "expiresAt": time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.RFC3339)})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)
	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{AppDataDir: appDataDir, DatabasePath: filepath.Join(appDataDir, "portico.db"), WebDistDir: filepath.Join("web", "dist"), SampleMediaURL: "https://media.example.test/sample.mp4"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	settings := RemoteAccessSettings{Enabled: true, HostedBaseURL: hosted.URL, ClaimStatus: "claimed", ServerID: "srv_single_cert", AssignedHostname: "ptc-singleaaaaaaaaaaaaaa.direct.getportico.tv", PublicPortMode: "manual", ManualPublicPort: 32500, PreferredRemoteAuthMode: "portico", CertificateStatus: "not_requested"}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-single"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			_, ensureErr := srv.ensureRemoteAccessCertificateFresh(context.Background(), settings)
			errs <- ensureErr
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent ensure: %v", err)
		}
	}
	if createCount != 1 {
		t.Fatalf("certificate create count = %d, want 1", createCount)
	}
}

func TestRemoteAccessCertificateRenewalDoesNotReplaceFilesOnFinalizeFailure(t *testing.T) {
	chdirRepoRoot(t)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_cert_fail/certificate-orders" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_fail", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_cert_fail/certificate-orders/certord_fail/finalize" && r.Method == http.MethodPost:
			writeError(w, http.StatusBadGateway, "acme_failed", "ACME finalization failed.")
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_cert_fail",
		AssignedHostname:        "ptc-cert-fail.direct.getportico.tv",
		PublicPortMode:          "manual",
		ManualPublicPort:        32400,
		PreferredRemoteAuthMode: "portico",
		CertificateStatus:       "valid",
		CertificateExpiresAt:    time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339),
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-cert-fail"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	writeTestCertificate(t, srv, "ptc-cert-fail.direct.getportico.tv")
	generationsBefore, err := srv.publishedCertificateGenerations()
	if err != nil || len(generationsBefore) != 1 {
		t.Fatalf("published certificate generations before failure = %#v, error = %v", generationsBefore, err)
	}
	generationDir := filepath.Join(appDataDir, "remote-access", "certificate-generations", generationsBefore[0])
	oldKey, err := os.ReadFile(filepath.Join(generationDir, "certificate-key.pem"))
	if err != nil {
		t.Fatalf("read old key: %v", err)
	}
	oldChain, err := os.ReadFile(filepath.Join(generationDir, "certificate-chain.pem"))
	if err != nil {
		t.Fatalf("read old chain: %v", err)
	}

	err = srv.requestRemoteAccessCertificate(context.Background(), settings)
	if err == nil {
		t.Fatalf("expected renewal failure")
	}
	generationsAfter, readErr := srv.publishedCertificateGenerations()
	if readErr != nil {
		t.Fatalf("read generations after failure: %v", readErr)
	}
	if !slices.Equal(generationsAfter, generationsBefore) {
		t.Fatalf("certificate generation changed on failure: before=%#v after=%#v", generationsBefore, generationsAfter)
	}
	keyAfter, readErr := os.ReadFile(filepath.Join(generationDir, "certificate-key.pem"))
	if readErr != nil {
		t.Fatalf("read key after failure: %v", readErr)
	}
	chainAfter, readErr := os.ReadFile(filepath.Join(generationDir, "certificate-chain.pem"))
	if readErr != nil {
		t.Fatalf("read chain after failure: %v", readErr)
	}
	if string(keyAfter) != string(oldKey) || string(chainAfter) != string(oldChain) {
		t.Fatalf("certificate files changed on failure, key=%q chain=%q", keyAfter, chainAfter)
	}
}

func TestRemoteAccessCertificateRenewalRejectsMismatchedCertificate(t *testing.T) {
	chdirRepoRoot(t)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_cert_mismatch/certificate-orders" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_mismatch", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_cert_mismatch/certificate-orders/certord_mismatch/finalize" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                  "certord_mismatch",
				"status":              "valid",
				"certificateChainPem": certificateForNewKey(t, "ptc-cert-mismatch.direct.getportico.tv", time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)),
				"expiresAt":           "2026-07-01T00:00:00Z",
			})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_cert_mismatch",
		AssignedHostname:        "ptc-cert-mismatch.direct.getportico.tv",
		PublicPortMode:          "manual",
		ManualPublicPort:        32400,
		PreferredRemoteAuthMode: "portico",
		CertificateStatus:       "not_requested",
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-cert-mismatch"); err != nil {
		t.Fatalf("save credential: %v", err)
	}

	err = srv.requestRemoteAccessCertificate(context.Background(), settings)
	if err == nil {
		t.Fatalf("expected mismatched certificate failure")
	}
	if _, statErr := os.Stat(srv.remoteCertificateManifestPath()); !os.IsNotExist(statErr) {
		t.Fatalf("certificate manifest should not be written on mismatch, stat err = %v", statErr)
	}
}

func TestRemoteAccessCertificateRenewalRejectsWrongHostnameCertificate(t *testing.T) {
	chdirRepoRoot(t)
	var csrPEM string
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_cert_wrong_host/certificate-orders" && r.Method == http.MethodPost:
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode cert order: %v", err)
			}
			csrPEM = req["csrPem"]
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_wrong_host", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_cert_wrong_host/certificate-orders/certord_wrong_host/finalize" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                  "certord_wrong_host",
				"status":              "valid",
				"certificateChainPem": certificateForCSRWithDNSNames(t, csrPEM, []string{"wrong.direct.getportico.tv"}, time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)),
				"expiresAt":           "2026-07-01T00:00:00Z",
			})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_cert_wrong_host",
		AssignedHostname:        "ptc-cert-wrong-host.direct.getportico.tv",
		PublicPortMode:          "manual",
		ManualPublicPort:        32400,
		PreferredRemoteAuthMode: "portico",
		CertificateStatus:       "not_requested",
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-cert-wrong-host"); err != nil {
		t.Fatalf("save credential: %v", err)
	}

	err = srv.requestRemoteAccessCertificate(context.Background(), settings)
	if err == nil {
		t.Fatalf("expected wrong-hostname certificate failure")
	}
	if _, statErr := os.Stat(srv.remoteCertificateManifestPath()); !os.IsNotExist(statErr) {
		t.Fatalf("certificate manifest should not be written on wrong hostname, stat err = %v", statErr)
	}
}

func TestRemoteAccessCertificateHostnameUsesScopedDirectWildcard(t *testing.T) {
	if got := remoteAccessCertificateHostname("ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv"); got != "*.ptc-aaaaaaaaaaaaaaaaaaaa.direct.getportico.tv" {
		t.Fatalf("certificate hostname = %q", got)
	}
	if got := remoteAccessCertificateHostname("custom.example.test"); got != "" {
		t.Fatalf("custom certificate hostname = %q", got)
	}
}

func TestRemoteAccessCertificateValidationAcceptsScopedWildcard(t *testing.T) {
	srv := &Server{}
	privateKey, csrPEM, err := srv.generateCertificateCSR("*.ptc-token123.direct.getportico.tv")
	if err != nil {
		t.Fatalf("generate wildcard CSR: %v", err)
	}
	chain := certificateForCSR(t, string(csrPEM), time.Now().UTC().Add(60*24*time.Hour))
	if err := validateCertificateChainForPrivateKeyAndHostname([]byte(chain), privateKey, "*.ptc-token123.direct.getportico.tv"); err != nil {
		t.Fatalf("expected wildcard certificate to validate: %v", err)
	}
	if err := validateCertificateChainForPrivateKeyAndHostname([]byte(chain), privateKey, "*.wrong.direct.getportico.tv"); err == nil {
		t.Fatalf("expected wrong wildcard certificate name to fail")
	}
}

func TestRemoteAccessCertificateFileWriterUsesRestrictedFiles(t *testing.T) {
	appDataDir := t.TempDir()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}}
	privateKey, csrPEM, err := srv.generateCertificateCSR("filewriter.example.test")
	if err != nil {
		t.Fatalf("generate certificate: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal certificate key: %v", err)
	}
	chainBytes := []byte(certificateForCSR(t, string(csrPEM), time.Now().UTC().Add(time.Hour)))
	if err := srv.writeRemoteAccessCertificateFiles(
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}), chainBytes,
	); err != nil {
		t.Fatalf("write certificate generation: %v", err)
	}
	generations, err := srv.publishedCertificateGenerations()
	if err != nil || len(generations) != 1 {
		t.Fatalf("published certificate generations = %#v, error = %v", generations, err)
	}
	generationDir := filepath.Join(appDataDir, "remote-access", "certificate-generations", generations[0])
	keyPath := filepath.Join(generationDir, "certificate-key.pem")
	chainPath := filepath.Join(generationDir, "certificate-chain.pem")
	keyInfo, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("stat certificate key: %v", err)
	}
	chainInfo, err := os.Stat(chainPath)
	if err != nil {
		t.Fatalf("stat certificate chain: %v", err)
	}
	if keyInfo.Mode().Perm() != 0o600 || chainInfo.Mode().Perm() != 0o600 {
		t.Fatalf("certificate modes key=%o chain=%o", keyInfo.Mode().Perm(), chainInfo.Mode().Perm())
	}
	storedKey, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatalf("read certificate key: %v", err)
	}
	storedChain, err := os.ReadFile(chainPath)
	if err != nil {
		t.Fatalf("read certificate chain: %v", err)
	}
	if string(storedKey) == "" || string(storedChain) == "" {
		t.Fatalf("published certificate generation is empty")
	}
}

func TestCustomCertificateValidationRequiresMatchingRestrictedKey(t *testing.T) {
	appDataDir := t.TempDir()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}}
	privateKey, csrPEM, err := srv.generateCertificateCSR("custom.example.test")
	if err != nil {
		t.Fatalf("generate csr: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPath := filepath.Join(appDataDir, "custom-key.pem")
	chainPath := filepath.Join(appDataDir, "custom-chain.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(chainPath, []byte(certificateForCSR(t, string(csrPEM), time.Now().UTC().Add(30*24*time.Hour))), 0o644); err != nil {
		t.Fatalf("write chain: %v", err)
	}
	expiresAt, err := validateCustomCertificateFiles(chainPath, keyPath)
	if err != nil {
		t.Fatalf("validate custom cert: %v", err)
	}
	if expiresAt.IsZero() {
		t.Fatalf("expected expiry from custom certificate")
	}
	if err := os.Chmod(keyPath, 0o644); err != nil {
		t.Fatalf("chmod key: %v", err)
	}
	if _, err := validateCustomCertificateFiles(chainPath, keyPath); err == nil || !strings.Contains(err.Error(), "must not be readable") {
		t.Fatalf("expected private key permission failure, got %v", err)
	}
}

func TestRemoteAccessSettingsAcceptCustomCertificateFiles(t *testing.T) {
	serverURL, appDataDir, _ := newRemoteAccessTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}}
	privateKey, csrPEM, err := srv.generateCertificateCSR("custom.example.test")
	if err != nil {
		t.Fatalf("generate csr: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPath := filepath.Join(appDataDir, "owner-key.pem")
	chainPath := filepath.Join(appDataDir, "owner-chain.pem")
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if err := os.WriteFile(chainPath, []byte(certificateForCSR(t, string(csrPEM), time.Now().UTC().Add(30*24*time.Hour))), 0o644); err != nil {
		t.Fatalf("write chain: %v", err)
	}

	var remoteStatus RemoteAccessStatus
	status, body := doJSON(t, client, http.MethodPatch, serverURL+"/api/remote-access/settings", map[string]any{
		"customCertificateEnabled": true,
		"customCertificatePath":    chainPath,
		"customCertificateKeyPath": keyPath,
	}, &remoteStatus)
	if status != http.StatusOK {
		t.Fatalf("custom cert settings status=%d body=%s", status, body)
	}
	if !remoteStatus.Settings.CustomCertificateEnabled || remoteStatus.Settings.CertificateStatus != "custom_valid" || remoteStatus.Settings.CertificateExpiresAt == "" {
		t.Fatalf("unexpected custom certificate settings: %#v", remoteStatus.Settings)
	}
}

func TestRemoteAccessCertificateAutoRenewWhenExpiring(t *testing.T) {
	chdirRepoRoot(t)
	var orderCount int
	var finalizeCount int
	var csrPEM string
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_auto_cert/certificate-orders" && r.Method == http.MethodPost:
			orderCount++
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decode cert order: %v", err)
			}
			csrPEM = req["csrPem"]
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_auto", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_auto_cert/certificate-orders/certord_auto/finalize" && r.Method == http.MethodPost:
			finalizeCount++
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                  "certord_auto",
				"status":              "valid",
				"certificateChainPem": certificateForCSR(t, csrPEM, time.Now().UTC().Add(60*24*time.Hour)),
				"expiresAt":           time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.RFC3339),
			})
		default:
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hosted.URL,
		ClaimStatus:             "claimed",
		ServerID:                "srv_auto_cert",
		AssignedHostname:        "ptc-eeeeeeeeeeeeeeeeeeee.direct.getportico.tv",
		PublicPortMode:          "manual",
		ManualPublicPort:        32400,
		PreferredRemoteAuthMode: "portico",
		CertificateStatus:       "valid",
		CertificateExpiresAt:    time.Now().UTC().Add(48 * time.Hour).Format(time.RFC3339),
		LANDiscoveryEnabled:     true,
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-auto"); err != nil {
		t.Fatalf("save credential: %v", err)
	}

	updated, err := srv.ensureRemoteAccessCertificateFresh(context.Background(), settings)
	if err != nil {
		t.Fatalf("ensure fresh certificate: %v", err)
	}
	if orderCount != 1 || finalizeCount != 1 {
		t.Fatalf("orderCount=%d finalizeCount=%d", orderCount, finalizeCount)
	}
	if updated.CertificateStatus != "valid" || updated.CertificateExpiresAt == "" || updated.LastCertificateRenewalAt == "" || updated.CertificateRenewalError != "" {
		t.Fatalf("unexpected updated settings: %#v", updated)
	}
}

func TestRemoteAccessCertificateRenewalNotDueWhenFresh(t *testing.T) {
	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()
	srv := &Server{cfg: config.Config{AppDataDir: appDataDir}, db: db, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	assignedHostname := "ptc-ffffffffffffffffffff.direct.getportico.tv"
	certificateHostname := remoteAccessCertificateHostname(assignedHostname)
	privateKey, csrPEM, err := srv.generateCertificateCSR(certificateHostname)
	if err != nil {
		t.Fatalf("generate certificate CSR: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal certificate key: %v", err)
	}
	if err := srv.writeRemoteAccessCertificateFiles(
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}),
		[]byte(certificateForCSR(t, string(csrPEM), time.Now().UTC().Add(45*24*time.Hour))),
	); err != nil {
		t.Fatalf("write fresh certificate: %v", err)
	}
	settings := RemoteAccessSettings{
		HostedBaseURL:        "https://hosted.example.test",
		ServerID:             "srv_fresh_cert",
		AssignedHostname:     assignedHostname,
		CertificateStatus:    "valid",
		CertificateExpiresAt: time.Now().UTC().Add(45 * 24 * time.Hour).Format(time.RFC3339),
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-fresh"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	due, err := srv.remoteAccessCertificateRenewalDue(settings, time.Now().UTC())
	if err != nil {
		t.Fatalf("renewal due: %v", err)
	}
	if due {
		t.Fatalf("fresh certificate should not be due")
	}

	mismatchedKey, _, err := srv.generateCertificateCSR(certificateHostname)
	if err != nil {
		t.Fatalf("generate mismatched certificate key: %v", err)
	}
	mismatchedKeyBytes, err := x509.MarshalECPrivateKey(mismatchedKey)
	if err != nil {
		t.Fatalf("marshal mismatched certificate key: %v", err)
	}
	generations, err := srv.publishedCertificateGenerations()
	if err != nil || len(generations) == 0 {
		t.Fatalf("published generations: %#v, error = %v", generations, err)
	}
	currentKeyPath := filepath.Join(appDataDir, "remote-access", "certificate-generations", generations[0], "certificate-key.pem")
	if err := os.WriteFile(currentKeyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: mismatchedKeyBytes}), 0o600); err != nil {
		t.Fatalf("replace certificate key: %v", err)
	}
	due, err = srv.remoteAccessCertificateRenewalDue(settings, time.Now().UTC())
	if err != nil {
		t.Fatalf("mismatched pair renewal due: %v", err)
	}
	if !due {
		t.Fatal("a mismatched installed certificate and key must remain due so staged recovery is retained")
	}

	exactKey, exactCSR, err := srv.generateCertificateCSR(assignedHostname)
	if err != nil {
		t.Fatalf("generate exact-host certificate CSR: %v", err)
	}
	exactKeyBytes, err := x509.MarshalECPrivateKey(exactKey)
	if err != nil {
		t.Fatalf("marshal legacy certificate key: %v", err)
	}
	if err := srv.writeRemoteAccessCertificateFiles(
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: exactKeyBytes}),
		[]byte(certificateForCSR(t, string(exactCSR), time.Now().UTC().Add(45*24*time.Hour))),
	); err != nil {
		t.Fatalf("write exact-host certificate: %v", err)
	}
	due, err = srv.remoteAccessCertificateRenewalDue(settings, time.Now().UTC())
	if err != nil {
		t.Fatalf("legacy renewal due: %v", err)
	}
	if !due {
		t.Fatal("an unexpired exact-host certificate must be replaced by the scoped wildcard certificate")
	}
}

func openExistingTestDB(t *testing.T, appDataDir string) *sql.DB {
	t.Helper()
	db, err := database.Open(config.Config{
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	})
	if err != nil {
		t.Fatalf("open existing test database: %v", err)
	}
	return db
}

func hostedClaimURL(r *http.Request) string {
	return "http://" + r.Host + "/claim?code=ABCD2345"
}

func TestValidateHostedClaimURLRejectsUntrustedNavigation(t *testing.T) {
	base := "https://api.getportico.tv"
	valid := "https://web.getportico.tv/claim?code=ABCD2345&serverName=Living+Room"
	if err := validateHostedClaimURL(base, valid, "ABCD2345"); err != nil {
		t.Fatalf("valid claim URL: %v", err)
	}
	for _, candidate := range []string{
		"https://evil.example/claim?code=ABCD2345",
		"https://web.getportico.tv.evil.example/claim?code=ABCD2345",
		"https://user@web.getportico.tv/claim?code=ABCD2345",
		"https://web.getportico.tv/other?code=ABCD2345",
		"https://web.getportico.tv/claim?code=WRONG123",
		"https://web.getportico.tv/claim?code=ABCD2345&code=ABCD2345",
		"https://web.getportico.tv/claim?code=ABCD2345&token=secret",
		"https://web.getportico.tv/claim?code=ABCD2345#fragment",
		"javascript:alert(1)",
	} {
		if err := validateHostedClaimURL(base, candidate, "ABCD2345"); err == nil {
			t.Fatalf("untrusted claim URL accepted: %s", candidate)
		}
	}
}

func certificateForCSR(t *testing.T, csrPEM string, expiresAt time.Time) string {
	t.Helper()
	return certificateForCSRWithDNSNames(t, csrPEM, nil, expiresAt)
}

func certificateForCSRWithDNSNames(t *testing.T, csrPEM string, dnsNames []string, expiresAt time.Time) string {
	t.Helper()
	block, _ := pem.Decode([]byte(csrPEM))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		t.Fatalf("invalid CSR PEM")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("parse CSR: %v", err)
	}
	if err := csr.CheckSignature(); err != nil {
		t.Fatalf("CSR signature: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      csr.Subject,
		DNSNames:     csr.DNSNames,
		NotBefore:    time.Now().UTC().Add(-time.Minute),
		NotAfter:     expiresAt,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	if dnsNames != nil {
		template.DNSNames = dnsNames
	}
	issuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("issuer key: %v", err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, csr.PublicKey, issuerKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func certificateForNewKey(t *testing.T, hostname string, expiresAt time.Time) string {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("certificate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkixName(hostname),
		DNSNames:     []string{hostname},
		NotBefore:    time.Now().UTC().Add(-time.Minute),
		NotAfter:     expiresAt,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func pkixName(commonName string) pkix.Name {
	return pkix.Name{CommonName: commonName}
}

func hasAuditAction(events []AuditEvent, action string) bool {
	for _, event := range events {
		if event.Action == action {
			return true
		}
	}
	return false
}

type fakeRouterMapper struct {
	add    RouterMappingResult
	remove RouterMappingResult
}

func (m fakeRouterMapper) AddMapping(ctx context.Context, internalPort, externalPort int, description string) RouterMappingResult {
	return m.add
}

func (m fakeRouterMapper) DeleteMapping(ctx context.Context, internalPort, externalPort int) RouterMappingResult {
	if m.remove.Status == "" {
		return RouterMappingResult{Status: "removed"}
	}
	return m.remove
}

func newRemoteAccessTestServer(t *testing.T) (string, string, *Server) {
	t.Helper()
	chdirRepoRoot(t)

	appDataDir := t.TempDir()
	cfg := config.Config{
		Addr:                     "127.0.0.1:0",
		AppDataDir:               appDataDir,
		DatabasePath:             filepath.Join(appDataDir, "portico.db"),
		WebDistDir:               filepath.Join("web", "dist"),
		SampleMediaURL:           "https://media.example.test/sample.mp4",
		HostedDocumentPublicKeys: testHostedDocumentPublicKeys(),
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	s := &Server{
		cfg:            cfg,
		db:             db,
		log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		logSubscribers: map[chan LogEvent]bool{},
		scannerWatch:   map[string]string{},
		transcodes:     map[string]*transcodeSession{},
	}
	testServer := httptest.NewServer(s.Handler())
	t.Cleanup(testServer.Close)

	status, body := doJSON(t, testServer.Client(), http.MethodPost, testServer.URL+"/api/auth/setup", map[string]string{
		"serverName":  "Remote Access Test Server",
		"username":    "admin",
		"email":       "admin@example.test",
		"displayName": "Admin",
		"password":    "Password1234",
	}, nil)
	if status != http.StatusCreated {
		t.Fatalf("setup status = %d, body: %s", status, body)
	}

	return testServer.URL, appDataDir, s
}

const testHostedDocumentKeyID = "test-hosted-documents"

func testHostedDocumentPrivateKey() ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("portico-hosted-document-test-key"))
	return ed25519.NewKeyFromSeed(seed[:])
}

func testHostedDocumentPublicKeys() map[string]string {
	publicKey := testHostedDocumentPrivateKey().Public().(ed25519.PublicKey)
	return map[string]string{testHostedDocumentKeyID: base64.StdEncoding.EncodeToString(publicKey)}
}

func testHostedDocumentSigningKeySet(t *testing.T) hostedDocumentSigningKeySet {
	t.Helper()
	now := time.Now().UTC()
	keySet := hostedDocumentSigningKeySet{
		SchemaVersion: 1,
		Generation:    1,
		IssuedAt:      now.Add(-time.Minute).Format(time.RFC3339Nano),
		ExpiresAt:     now.Add(time.Hour).Format(time.RFC3339Nano),
		ActiveKeyID:   testHostedDocumentKeyID,
		Keys: []hostedDocumentSigningPublicKey{{
			KeyID:        testHostedDocumentKeyID,
			Algorithm:    hostedSignatureAlgorithm,
			PublicKeyB64: testHostedDocumentPublicKeys()[testHostedDocumentKeyID],
			State:        "active",
		}},
	}
	var err error
	keySet.Fingerprint, err = hostedDocumentKeySetFingerprint(keySet)
	if err != nil {
		t.Fatalf("fingerprint Hosted signing key set: %v", err)
	}
	return keySet
}

func signedTestPolicySnapshot(t *testing.T, document map[string]any) map[string]any {
	t.Helper()
	now := time.Now().UTC()
	if _, ok := document["kind"]; !ok {
		document["kind"] = hostedPolicyKind
	}
	if _, ok := document["version"]; !ok {
		document["version"] = 1
	}
	if _, ok := document["generation"]; !ok {
		document["generation"] = 1
	}
	document["chunkCount"] = 1
	document["chunkIndex"] = 0
	document["itemCount"] = policySnapshotTestItemCount(document)
	document["audience"] = hostedDocumentAudience
	document["signatureAlgorithm"] = hostedSignatureAlgorithm
	document["signatureKeyId"] = testHostedDocumentKeyID
	if _, ok := document["digest"]; !ok {
		document["digest"] = strings.Repeat("a", 64)
	}
	if _, ok := document["policyDigest"]; !ok {
		document["policyDigest"] = strings.Repeat("b", 64)
	}
	if _, ok := document["issuedAt"]; !ok {
		document["issuedAt"] = now.Format(time.RFC3339Nano)
	}
	if _, ok := document["expiresAt"]; !ok {
		document["expiresAt"] = now.Add(10 * time.Minute).Format(time.RFC3339Nano)
	}
	unsigned, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal unsigned policy snapshot: %v", err)
	}
	payload, err := canonicalHostedDocument("policy-snapshot", unsigned)
	if err != nil {
		t.Fatalf("canonicalize policy snapshot: %v", err)
	}
	document["signature"] = base64.RawURLEncoding.EncodeToString(ed25519.Sign(testHostedDocumentPrivateKey(), payload))
	return document
}

func policySnapshotTestItemCount(document map[string]any) int {
	count := 0
	for _, field := range []string{"members", "deletedAccountTombstones"} {
		switch values := document[field].(type) {
		case []any:
			count += len(values)
		case []map[string]any:
			count += len(values)
		case []RemoteAccessMember:
			count += len(values)
		case []RemoteDeletedAccountTombstone:
			count += len(values)
		}
	}
	return count
}

func newRemoteAccessUnitServer(t *testing.T) *Server {
	t.Helper()
	chdirRepoRoot(t)
	appDataDir := t.TempDir()
	cfg := config.Config{
		Addr:           "127.0.0.1:0",
		AppDataDir:     appDataDir,
		DatabasePath:   filepath.Join(appDataDir, "portico.db"),
		WebDistDir:     filepath.Join("web", "dist"),
		SampleMediaURL: "https://media.example.test/sample.mp4",
	}
	db, err := database.Open(cfg)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return &Server{
		cfg:                 cfg,
		db:                  db,
		log:                 slog.New(slog.NewTextHandler(io.Discard, nil)),
		logSubscribers:      map[chan LogEvent]bool{},
		appEventSubscribers: map[chan AppEvent]bool{},
		scannerWatch:        map[string]string{},
		transcodes:          map[string]*transcodeSession{},
	}
}

func TestRemoteAccessClaimActivationJournalRecoversAfterRestartWindow(t *testing.T) {
	srv := newRemoteAccessUnitServer(t)
	settings, err := srv.remoteAccessSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.ClaimStatus = "claimed"
	settings.ServerID = "srv_recovered"
	settings.AssignedHostname = "ptc-recovered.direct.getportico.tv"
	claim := RemoteAccessClaim{ClaimID: "claim_recovered", Status: "claimed", HostedReady: true}
	activation := remoteAccessClaimActivation{Claim: claim, Settings: settings, ServerCredential: "credential-recovered"}
	if err := srv.saveRemoteAccessClaimActivation(activation); err != nil {
		t.Fatal(err)
	}
	var rawJournal string
	if err := srv.db.QueryRow(`SELECT value_json FROM settings WHERE key = ?`, remoteAccessClaimActivationKey).Scan(&rawJournal); err != nil {
		t.Fatalf("read raw activation journal: %v", err)
	}
	if strings.Contains(rawJournal, activation.ServerCredential) {
		t.Fatalf("raw activation journal exposed server credential: %s", rawJournal)
	}
	// Simulate a crash immediately after journaling the Hosted result.
	restarted := &Server{cfg: srv.cfg, db: srv.db, log: srv.log}
	status, err := restarted.remoteAccessStatus()
	if err != nil {
		t.Fatalf("reconcile activation: %v", err)
	}
	if !status.PorticoConnected || status.Settings.ServerID != "srv_recovered" {
		t.Fatalf("status = %#v", status.Settings)
	}
	if got := restarted.secretSetting(remoteAccessCredentialKey); got != "credential-recovered" {
		t.Fatalf("credential = %q", got)
	}
	if got := restarted.secretSetting(remoteAccessClaimTokenKey); got != "" {
		t.Fatalf("claim token survived: %q", got)
	}
	all, err := restarted.loadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := all[remoteAccessClaimActivationKey]; ok {
		t.Fatal("activation journal was not cleared")
	}
}

func TestPorticoSetupClaimMissingTokenStartsFreshOperation(t *testing.T) {
	srv := newRemoteAccessUnitServer(t)
	var operation string
	const claimReceipt = "fresh-claim-receipt"
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/server-claims":
			operation = r.Header.Get("Idempotency-Key")
			writeJSON(w, http.StatusCreated, map[string]any{"claimId": "fresh", "claimCode": "FRESH123", "claimToken": "fresh-token", "lostResponseReceipt": claimReceipt, "claimUrl": "http://" + r.Host + "/claim?code=FRESH123", "status": "pending", "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
		case r.URL.Path == "/api/server-claims/fresh/result":
			if r.Header.Get("X-Portico-Claim-Receipt") != claimReceipt || r.Header.Get("Authorization") != "" {
				t.Fatalf("claim result headers receipt=%q authorization=%q", r.Header.Get("X-Portico-Claim-Receipt"), r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
		default:
			t.Fatalf("unexpected request %s", r.URL.Path)
		}
	}))
	defer hosted.Close()
	settings, _ := srv.remoteAccessSettings()
	settings.HostedBaseURL = hosted.URL
	_ = srv.saveRemoteAccessSettings(settings)
	old := RemoteAccessClaim{ClaimID: "corrupt", Status: "pending", HostedReady: true, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
	if err := srv.saveRemoteAccessClaim(old); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveSecretSetting(remoteAccessClaimOperationKey, "old-operation"); err != nil {
		t.Fatal(err)
	}
	status, err := srv.startPorticoSetupClaim(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.Claim == nil || status.Claim.ClaimID != "fresh" {
		t.Fatalf("claim = %#v", status.Claim)
	}
	if operation == "" || operation == "old-operation" {
		t.Fatalf("operation key = %q", operation)
	}
}

func TestPorticoSetupClaimReuseRepairsSettingsCrashWindow(t *testing.T) {
	srv := newRemoteAccessUnitServer(t)
	var creates, polls int
	const claimReceipt = "staged-claim-receipt"
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			creates++
		}
		if r.URL.Path == "/api/server-claims/staged/result" {
			polls++
			if r.Header.Get("X-Portico-Claim-Receipt") != claimReceipt || r.Header.Get("Authorization") != "" {
				t.Fatalf("claim result headers receipt=%q authorization=%q", r.Header.Get("X-Portico-Claim-Receipt"), r.Header.Get("Authorization"))
			}
			writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
			return
		}
		t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
	}))
	defer hosted.Close()
	settings, _ := srv.remoteAccessSettings()
	settings.HostedBaseURL = hosted.URL
	settings.Enabled = false
	settings.ClaimStatus = "not_claimed"
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatal(err)
	}
	claim := RemoteAccessClaim{ClaimID: "staged", Status: "pending", HostedReady: true, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)}
	if err := srv.saveSecretSetting(remoteAccessClaimTokenKey, "staged-token"); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveSecretSetting(remoteAccessClaimReceiptKey, claimReceipt); err != nil {
		t.Fatal(err)
	}
	if err := srv.saveRemoteAccessClaim(claim); err != nil {
		t.Fatal(err)
	}
	status, err := srv.startPorticoSetupClaim(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Settings.Enabled || status.Settings.ClaimStatus != "pending" {
		t.Fatalf("settings were not repaired: %#v", status.Settings)
	}
	if creates != 0 || polls != 1 {
		t.Fatalf("creates=%d polls=%d", creates, polls)
	}
}

func TestPorticoSetupExpiredClaimDoesNotReplayOperation(t *testing.T) {
	srv := newRemoteAccessUnitServer(t)
	var operation string
	const claimReceipt = "new-claim-receipt"
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/server-claims" {
			operation = r.Header.Get("Idempotency-Key")
			writeJSON(w, http.StatusCreated, map[string]any{"claimId": "new", "claimCode": "NEW12345", "claimToken": "new-token", "lostResponseReceipt": claimReceipt, "claimUrl": "http://" + r.Host + "/claim?code=NEW12345", "status": "pending", "expiresAt": time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
	}))
	defer hosted.Close()
	settings, _ := srv.remoteAccessSettings()
	settings.HostedBaseURL = hosted.URL
	_ = srv.saveRemoteAccessSettings(settings)
	_ = srv.saveRemoteAccessClaim(RemoteAccessClaim{ClaimID: "expired", Status: "pending", HostedReady: true, ExpiresAt: time.Now().Add(-time.Minute).UTC().Format(time.RFC3339)})
	_ = srv.saveSecretSetting(remoteAccessClaimTokenKey, "expired-token")
	_ = srv.saveSecretSetting(remoteAccessClaimOperationKey, "expired-operation")
	if _, err := srv.startPorticoSetupClaim(context.Background()); err != nil {
		t.Fatal(err)
	}
	if operation == "" || operation == "expired-operation" {
		t.Fatalf("operation key = %q", operation)
	}
}

func TestRemoteAccessStatusCoalescesConcurrentClaimPolls(t *testing.T) {
	srv := newRemoteAccessUnitServer(t)
	var mu sync.Mutex
	polls := 0
	const claimReceipt = "shared-claim-receipt"
	release := make(chan struct{})
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Portico-Claim-Receipt") != claimReceipt || r.Header.Get("Authorization") != "" {
			t.Fatalf("claim result headers receipt=%q authorization=%q", r.Header.Get("X-Portico-Claim-Receipt"), r.Header.Get("Authorization"))
		}
		mu.Lock()
		polls++
		mu.Unlock()
		<-release
		writeJSON(w, http.StatusOK, map[string]any{"status": "pending"})
	}))
	defer hosted.Close()
	settings, _ := srv.remoteAccessSettings()
	settings.HostedBaseURL = hosted.URL
	settings.Enabled = true
	settings.ClaimStatus = "pending"
	_ = srv.saveRemoteAccessSettings(settings)
	_ = srv.saveRemoteAccessClaim(RemoteAccessClaim{ClaimID: "shared", Status: "pending", HostedReady: true, ExpiresAt: time.Now().Add(time.Hour).UTC().Format(time.RFC3339)})
	_ = srv.saveSecretSetting(remoteAccessClaimTokenKey, "shared-token")
	_ = srv.saveSecretSetting(remoteAccessClaimReceiptKey, claimReceipt)
	if _, err := srv.loadOrCreateServerIdentity(); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = srv.remoteAccessStatus() }()
	}
	time.Sleep(100 * time.Millisecond)
	close(release)
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	if polls != 1 {
		t.Fatalf("polls = %d, want 1", polls)
	}
}

func authenticatePorticoTestClient(t *testing.T, srv *Server, client *http.Client, serverURL string, member RemoteAccessMember) User {
	t.Helper()
	if member.PorticoMembershipID == "" {
		member.PorticoMembershipID = member.ID
	}
	if member.PorticoUserID == "" {
		member.PorticoUserID = member.UserID
	}
	user, err := srv.userForPorticoMembership(member)
	if err != nil {
		t.Fatalf("provision Portico test principal: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	displayName := strings.TrimSpace(member.DisplayName)
	if displayName == "" {
		displayName = user.DisplayName
	}
	profile := HostedProfileSnapshot{
		ExternalProfileID: member.PorticoUserID,
		AccountID:         member.PorticoUserID,
		DisplayName:       displayName,
		IsPrimary:         true,
		IsAccountAdmin:    true,
		PolicyUpdatedAt:   now.Add(-time.Minute),
		Restrictions:      defaultProfileRestrictions(),
	}
	if err := srv.reconcileHostedProfileSelectionEnvelopeContext(context.Background(), user.ID, HostedProfileSelectionEnvelope{
		AssertionID:     "test-profile-directory-" + user.ID,
		AccountID:       member.PorticoUserID,
		AccountRevision: 1,
		Profiles:        []HostedProfileSnapshot{profile},
		IssuedAt:        now.Format(time.RFC3339Nano),
		ExpiresAt:       now.Add(5 * time.Minute).Format(time.RFC3339Nano),
	}, now); err != nil {
		t.Fatalf("seed Portico test profile directory: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	recorder := httptest.NewRecorder()
	if _, err := srv.createSessionForProvider(recorder, request, user.ID, "portico"); err != nil {
		t.Fatalf("create Portico test session: %v", err)
	}
	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	for _, cookie := range recorder.Result().Cookies() {
		client.Jar.SetCookies(parsed, []*http.Cookie{cookie})
	}
	return user
}
