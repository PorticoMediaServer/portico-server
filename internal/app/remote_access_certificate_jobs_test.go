package app

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func configureCertificateJobIdentity(t *testing.T, server *Server, hostedBaseURL, serverID string) RemoteAccessSettings {
	t.Helper()
	settings := RemoteAccessSettings{
		Enabled:                 true,
		HostedBaseURL:           hostedBaseURL,
		ClaimStatus:             "claimed",
		ServerID:                serverID,
		AssignedHostname:        "ptc-cccccccccccccccccccc.direct.getportico.tv",
		PublicPortMode:          "disabled",
		PreferredRemoteAuthMode: "portico",
		CertificateStatus:       "not_requested",
		LANDiscoveryEnabled:     true,
	}
	if err := server.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save remote access settings: %v", err)
	}
	if err := server.saveSecretSetting(remoteAccessCredentialKey, "certificate-job-credential"); err != nil {
		t.Fatalf("save remote access credential: %v", err)
	}
	return settings
}

func onlyCertificateMaintenanceJob(t *testing.T, server *Server) Job {
	t.Helper()
	var jobID string
	if err := server.db.QueryRow(`SELECT id FROM jobs WHERE type = ? ORDER BY created_at, id LIMIT 1`, remoteAccessCertificateMaintenanceJobType).Scan(&jobID); err != nil {
		t.Fatalf("load certificate job ID: %v", err)
	}
	job, err := server.getJob(jobID)
	if err != nil {
		t.Fatalf("load certificate job: %v", err)
	}
	return job
}

func TestRemoteAccessCertificateStartupScheduledAndPostClaimTriggersUseDurableJob(t *testing.T) {
	tests := []struct {
		name    string
		trigger string
		queue   func(*Server, RemoteAccessSettings)
	}{
		{name: "startup", trigger: "startup", queue: func(server *Server, _ RemoteAccessSettings) {
			server.queueStartupRemoteAccessCertificateMaintenance()
		}},
		{name: "scheduled", trigger: "scheduled", queue: func(server *Server, _ RemoteAccessSettings) {
			server.queueScheduledRemoteAccessCertificateMaintenance()
		}},
		{name: "post claim", trigger: "post_claim", queue: func(server *Server, settings RemoteAccessSettings) {
			server.queueRemoteAccessPostClaimCertificateMaintenance(settings)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newRemoteAccessUnitServer(t)
			settings := configureCertificateJobIdentity(t, server, "http://127.0.0.1:32599", "srv_"+strings.ReplaceAll(test.name, " ", "_"))
			test.queue(server, settings)
			job := onlyCertificateMaintenanceJob(t, server)
			if job.Type != remoteAccessCertificateMaintenanceJobType || job.ResourceType != "remote_access" || job.ResourceID != "certificate" || job.Metadata["trigger"] != test.trigger {
				t.Fatalf("trigger job = %#v", job)
			}
			if descriptor, ok := durableJobDescriptorForType(job.Type); !ok || descriptor.WorkClass != job.Priority || descriptor.ResourceLane != jobLaneMaintenance || !descriptor.Singleton || !descriptor.ActiveKey {
				t.Fatalf("certificate descriptor = %#v, job = %#v", descriptor, job)
			}
		})
	}
}

func TestRemoteAccessCertificateManualForcePromotionCannotMissRunningJob(t *testing.T) {
	var csrPEM atomic.Value
	var createCount atomic.Int32
	var heartbeatCount atomic.Int32
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_force_race/certificate-orders" && r.Method == http.MethodPost:
			createCount.Add(1)
			var body struct {
				CSRPem string `json:"csrPem"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode certificate request: %v", err)
			}
			csrPEM.Store(body.CSRPem)
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_force_race", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_force_race/certificate-orders/certord_force_race/finalize" && r.Method == http.MethodPost:
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                  "certord_force_race",
				"status":              "valid",
				"certificateChainPem": certificateForCSR(t, csrPEM.Load().(string), time.Now().UTC().Add(60*24*time.Hour)),
				"expiresAt":           time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.RFC3339),
			})
		case r.URL.Path == "/api/servers/srv_force_race/heartbeat" && r.Method == http.MethodPost:
			heartbeatCount.Add(1)
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":                            true,
				"serverId":                      "srv_force_race",
				"assignedHostname":              "ptc-cccccccccccccccccccc.direct.getportico.tv",
				"remoteAccessEnabled":           true,
				"publicConsoleOriginGeneration": 1,
			})
		default:
			t.Fatalf("unexpected Hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	server := newRemoteAccessUnitServer(t)
	settings := configureCertificateJobIdentity(t, server, hosted.URL, "srv_force_race")
	hostname := remoteAccessCertificateHostname(settings.AssignedHostname)
	privateKey, initialCSR, err := server.generateCertificateCSR(hostname)
	if err != nil {
		t.Fatalf("generate initial certificate: %v", err)
	}
	keyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		t.Fatalf("marshal initial certificate key: %v", err)
	}
	expiresAt := time.Now().UTC().Add(60 * 24 * time.Hour)
	if err := server.writeRemoteAccessCertificateFiles(
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}),
		[]byte(certificateForCSR(t, string(initialCSR), expiresAt)),
	); err != nil {
		t.Fatalf("publish initial certificate: %v", err)
	}
	settings.CertificateStatus = "valid"
	settings.CertificateExpiresAt = expiresAt.Format(time.RFC3339)
	if err := server.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save fresh certificate state: %v", err)
	}
	job, existing, err := server.enqueueRemoteAccessCertificateMaintenance("scheduled", false)
	if err != nil || existing {
		t.Fatalf("enqueue scheduled certificate check existing=%v error=%v", existing, err)
	}
	metadataRead := make(chan struct{})
	releaseWorker := make(chan struct{})
	var once sync.Once
	server.remoteAccessCertificateMetadataHook = func(Job) {
		once.Do(func() {
			close(metadataRead)
			<-releaseWorker
		})
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.runJob(job)
	}()
	select {
	case <-metadataRead:
	case <-time.After(3 * time.Second):
		t.Fatal("certificate worker did not capture scheduled metadata")
	}
	forced, reused, err := server.enqueueRemoteAccessCertificateMaintenance("manual", true)
	if err != nil || !reused || forced.ID != job.ID || forced.Metadata["force"] != "true" {
		t.Fatalf("manual force promotion job=%#v reused=%v error=%v", forced, reused, err)
	}
	close(releaseWorker)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("certificate worker did not consume force promotion")
	}
	completed, err := server.getJob(job.ID)
	if err != nil || completed.Status != "complete" || completed.Metadata["force"] != "false" {
		t.Fatalf("force-promoted certificate job = %#v, error = %v", completed, err)
	}
	if createCount.Load() != 1 || heartbeatCount.Load() != 1 {
		t.Fatalf("force promotion execution create=%d heartbeat=%d", createCount.Load(), heartbeatCount.Load())
	}
}

func TestRemoteAccessCertificateDurableJobRetriesTransientFailureAndRecoversAfterRestart(t *testing.T) {
	var createCount atomic.Int32
	var finalizeCount atomic.Int32
	var heartbeatCount atomic.Int32
	var csrPEM atomic.Value
	var committedStateServer atomic.Pointer[Server]
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/servers/srv_cert_restart/certificate-orders" && r.Method == http.MethodPost:
			attempt := createCount.Add(1)
			if attempt == 1 {
				w.Header().Set("Retry-After", "1")
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"code": "temporarily_unavailable"})
				return
			}
			var body struct {
				CSRPem string `json:"csrPem"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode certificate request: %v", err)
			}
			csrPEM.Store(body.CSRPem)
			writeJSON(w, http.StatusCreated, map[string]any{"id": "certord_restart", "status": "pending"})
		case r.URL.Path == "/api/servers/srv_cert_restart/certificate-orders/certord_restart/finalize" && r.Method == http.MethodPost:
			finalizeCount.Add(1)
			writeJSON(w, http.StatusOK, map[string]any{
				"id":                  "certord_restart",
				"status":              "valid",
				"certificateChainPem": certificateForCSR(t, csrPEM.Load().(string), time.Now().UTC().Add(60*24*time.Hour)),
				"expiresAt":           time.Now().UTC().Add(60 * 24 * time.Hour).Format(time.RFC3339),
			})
		case r.URL.Path == "/api/servers/srv_cert_restart/heartbeat" && r.Method == http.MethodPost:
			heartbeatCount.Add(1)
			server := committedStateServer.Load()
			if server == nil {
				t.Fatal("certificate heartbeat ran without a committed-state observer")
			}
			settings, err := server.remoteAccessSettings()
			if err != nil || settings.CertificateStatus != "valid" || settings.LastCertificateRenewalAt == "" {
				t.Fatalf("heartbeat preceded committed certificate state: %#v, error = %v", settings, err)
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":                            true,
				"serverId":                      "srv_cert_restart",
				"assignedHostname":              "ptc-cccccccccccccccccccc.direct.getportico.tv",
				"remoteAccessEnabled":           true,
				"publicConsoleOriginGeneration": 1,
			})
		default:
			t.Fatalf("unexpected Hosted request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(hosted.Close)

	server := newRemoteAccessUnitServer(t)
	configureCertificateJobIdentity(t, server, hosted.URL, "srv_cert_restart")
	job, existing, err := server.enqueueRemoteAccessCertificateMaintenance("manual", true)
	if err != nil || existing {
		t.Fatalf("initial certificate enqueue existing=%v error=%v job=%#v", existing, err, job)
	}
	server.runJob(job)
	delayed, err := server.getJob(job.ID)
	if err != nil || delayed.Status != "queued" || delayed.AttemptCount != 1 || delayed.FailureKind != "hosted_transient" || delayed.LastError == "" || delayed.NextRunAt == "" {
		t.Fatalf("transient certificate attempt = %#v, error = %v", delayed, err)
	}

	expired := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	old := time.Now().UTC().Add(-3 * time.Hour).Format(time.RFC3339Nano)
	if _, err := server.db.Exec(`UPDATE jobs SET status = 'running', phase = 'running', leased_by = 'departed-worker', lease_expires_at = ?, updated_at = ? WHERE id = ?`, expired, old, job.ID); err != nil {
		t.Fatalf("stage interrupted certificate job: %v", err)
	}
	restarted := &Server{
		cfg:         server.cfg,
		db:          server.db,
		log:         slog.New(slog.NewTextHandler(io.Discard, nil)),
		jobWorkerID: randomID("replacement-worker"),
	}
	if err := restarted.requeueRunningJobsOnStartup(); err != nil {
		t.Fatalf("recover certificate job after restart: %v", err)
	}
	recovered, err := restarted.getJob(job.ID)
	if err != nil || recovered.Status != "queued" || recovered.ErrorCode != "restart_interrupted" {
		t.Fatalf("recovered certificate job = %#v, error = %v", recovered, err)
	}
	committedStateServer.Store(restarted)
	restarted.runJob(recovered)
	completed, err := restarted.getJob(job.ID)
	if err != nil || completed.Status != "complete" || completed.AttemptCount != 2 || completed.RetentionUntil == "" {
		t.Fatalf("completed recovered certificate job = %#v, error = %v", completed, err)
	}
	if createCount.Load() != 2 || finalizeCount.Load() != 1 || heartbeatCount.Load() != 1 {
		t.Fatalf("Hosted counts create=%d finalize=%d heartbeat=%d", createCount.Load(), finalizeCount.Load(), heartbeatCount.Load())
	}
}

func TestRemoteAccessCertificateDurableJobRecordsTerminalFailure(t *testing.T) {
	hosted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/servers/srv_cert_terminal/certificate-orders" || r.Method != http.MethodPost {
			t.Fatalf("unexpected Hosted request: %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "certificate_request_invalid"})
	}))
	t.Cleanup(hosted.Close)
	server := newRemoteAccessUnitServer(t)
	configureCertificateJobIdentity(t, server, hosted.URL, "srv_cert_terminal")
	job, _, err := server.enqueueRemoteAccessCertificateMaintenance("manual", true)
	if err != nil {
		t.Fatalf("enqueue certificate job: %v", err)
	}
	server.runJob(job)
	failed, err := server.getJob(job.ID)
	if err != nil || failed.Status != "failed" || failed.AttemptCount != 1 || failed.ErrorCode != "remote_access_certificate_failed" || failed.LastError == "" || failed.RetentionUntil == "" {
		t.Fatalf("terminal certificate job = %#v, error = %v", failed, err)
	}
}

func TestRemoteAccessCertificateMaintenanceHasOneProductionExecutionOwner(t *testing.T) {
	remoteAccessSource, err := os.ReadFile(filepath.Join("remote_access.go"))
	if err != nil {
		remoteAccessSource, err = os.ReadFile(filepath.Join("internal", "app", "remote_access.go"))
	}
	if err != nil {
		t.Fatalf("read remote access source: %v", err)
	}
	serverSource, err := os.ReadFile(filepath.Join("server.go"))
	if err != nil {
		serverSource, err = os.ReadFile(filepath.Join("internal", "app", "server.go"))
	}
	if err != nil {
		t.Fatalf("read server source: %v", err)
	}
	remoteAccessText := string(remoteAccessSource)
	serverText := string(serverSource)
	if strings.Contains(remoteAccessText, "runRemoteAccessCertificateMaintenance(ctx") || strings.Contains(serverText, "startBackground(\"remote-access-certificate-maintenance\"") {
		t.Fatal("a production certificate timer/loop owner survives beside the durable job")
	}
	if strings.Contains(remoteAccessText, "ensureRemoteAccessCertificateFresh") {
		t.Fatal("a synchronous certificate-maintenance compatibility path survives")
	}
	if count := strings.Count(remoteAccessText, "performRemoteAccessCertificateMaintenance("); count != 2 {
		t.Fatalf("certificate execution primitive has %d production references, want definition plus durable worker call", count)
	}
	if !strings.Contains(serverText, "s.runRemoteAccessCertificateMaintenanceJob(ctx, job)") || !strings.Contains(serverText, "s.startBackground(\"remote-access-heartbeat\", s.runRemoteAccessHeartbeat)") {
		t.Fatal("durable certificate dispatch or the sole continuous lease worker is missing")
	}
}
