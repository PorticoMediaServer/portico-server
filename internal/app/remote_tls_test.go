package app

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestRemoteTLSStatusUsesSharedServicePort(t *testing.T) {
	srv := &Server{cfg: config.Config{AppDataDir: t.TempDir(), Addr: "127.0.0.1:32500"}, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	settings := RemoteAccessSettings{Enabled: true, PublicPortMode: "manual", ManualPublicPort: 40000}
	srv.configureRemoteTLS(settings)
	address, port, errText := srv.remoteTLSStatus()
	if errText == "" || port != 32500 || address != "127.0.0.1:32500" {
		t.Fatalf("shared port status = address %q port %d error %q", address, port, errText)
	}
	writeTestCertificate(t, srv, "shared.example.test")
	srv.configureRemoteTLS(settings)
	address, port, errText = srv.remoteTLSStatus()
	if errText != "" || port != 32500 || address != "127.0.0.1:32500" {
		t.Fatalf("ready shared port status = address %q port %d error %q", address, port, errText)
	}
}

func TestRemoteTLSRepairReloadsCertificateAndHeartbeats(t *testing.T) {
	heartbeatSeen := make(chan map[string]any, 1)
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/servers/srv_tls_repair/heartbeat" || r.Method != http.MethodPost {
			t.Fatalf("unexpected hosted request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer server-credential-tls-repair" {
			t.Fatalf("heartbeat auth = %q", r.Header.Get("Authorization"))
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode heartbeat: %v", err)
		}
		heartbeatSeen <- payload
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "serverId": "srv_tls_repair", "assignedHostname": "ptc-tls-repair.direct.getportico.tv", "remoteAccessEnabled": true, "publicConsoleOriginGeneration": 1})
	}))
	t.Cleanup(hosted.Close)

	srv := newRemoteAccessUnitServer(t)
	srv.cfg.Addr = "127.0.0.1:32500"
	settings := RemoteAccessSettings{
		Enabled: true, HostedBaseURL: hosted.URL, ClaimStatus: "claimed", ServerID: "srv_tls_repair",
		AssignedHostname: "ptc-tls-repair.direct.getportico.tv", PublicPortMode: "manual", ManualPublicPort: 32500,
		PreferredRemoteAuthMode: "portico", CertificateStatus: "valid",
		CertificateExpiresAt: time.Now().UTC().Add(90 * 24 * time.Hour).Format(time.RFC3339),
	}
	if err := srv.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	if err := srv.saveSecretSetting(remoteAccessCredentialKey, "server-credential-tls-repair"); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	srv.configureRemoteTLS(settings)
	if _, _, errText := srv.remoteTLSStatus(); errText == "" {
		t.Fatal("expected initial TLS status to report a missing certificate")
	}
	writeTestCertificate(t, srv, "ptc-tls-repair.direct.getportico.tv")
	repaired, err := srv.checkRemoteTLSListenerAndRepair(context.Background())
	if err != nil {
		t.Fatalf("repair TLS: %v", err)
	}
	if !repaired {
		t.Fatal("expected TLS repair to load the new certificate")
	}
	address, repairedPort, errText := srv.remoteTLSStatus()
	if errText != "" || repairedPort != 32500 || address != "127.0.0.1:32500" {
		t.Fatalf("repaired TLS status = address %q port %d error %q", address, repairedPort, errText)
	}
	select {
	case payload := <-heartbeatSeen:
		if payload["publicPort"] != float64(32500) {
			t.Fatalf("heartbeat publicPort = %#v", payload["publicPort"])
		}
	case <-time.After(time.Second):
		t.Fatal("TLS repair did not heartbeat")
	}
}

func TestRemoteAccessCertificateLoaderUsesImmutableRevisedMaterial(t *testing.T) {
	srv := &Server{cfg: config.Config{AppDataDir: t.TempDir(), Addr: "127.0.0.1:32500"}, log: slog.Default()}
	loader := srv.RemoteAccessCertificateLoader()
	if _, err := loader(&tls.ClientHelloInfo{}); err == nil {
		t.Fatalf("expected missing certificate to fail")
	}
	writeTestCertificate(t, srv, "first.example.test")
	settings := RemoteAccessSettings{Enabled: true, PublicPortMode: "automatic"}
	srv.configureRemoteTLS(settings)
	first, err := loader(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("load first certificate: %v", err)
	}
	firstLeaf, err := x509.ParseCertificate(first.Certificate[0])
	if err != nil || firstLeaf.Subject.CommonName != "first.example.test" {
		t.Fatalf("first certificate = %#v error=%v", firstLeaf, err)
	}
	writeTestCertificate(t, srv, "renewed.example.test")
	stillFirst, err := loader(&tls.ClientHelloInfo{})
	if err != nil || stillFirst.Leaf == nil || stillFirst.Leaf.Subject.CommonName != "first.example.test" {
		t.Fatalf("handshake observed uncommitted certificate revision: certificate=%#v error=%v", stillFirst, err)
	}
	srv.configureRemoteTLS(settings)
	renewed, err := loader(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("load renewed certificate: %v", err)
	}
	renewedLeaf, err := x509.ParseCertificate(renewed.Certificate[0])
	if err != nil || renewedLeaf.Subject.CommonName != "renewed.example.test" {
		t.Fatalf("renewed certificate = %#v error=%v", renewedLeaf, err)
	}
	if _, err := loader(&tls.ClientHelloInfo{ServerName: "renewed.example.test"}); err != nil {
		t.Fatalf("load certificate for matching SNI: %v", err)
	}
	if _, err := loader(&tls.ClientHelloInfo{ServerName: "wrong.example.test"}); err == nil {
		t.Fatal("expected certificate loader to reject an uncovered SNI name")
	}
}

func writeTestCertificate(t *testing.T, srv *Server, commonName string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()), Subject: pkix.Name{CommonName: commonName}, DNSNames: []string{commonName},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := srv.publishRemoteCertificatePair(keyPEM, certPEM); err != nil {
		t.Fatalf("publish cert generation: %v", err)
	}
}
