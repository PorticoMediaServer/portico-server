package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDVRFFmpegArgvContainsOnlyOpaqueLoopbackInput(t *testing.T) {
	secretURL := "https://provider.example/live/channel.ts?username=alice&password=super-secret"
	transport, err := startDVRInputTransport(context.Background(), "recording-1", "lease-1", secretURL,
		fixedLiveTVResolver{addresses: []netip.Addr{netip.MustParseAddr("203.0.113.20")}})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.Close()
	args := dvrRecordingFFmpegArgs(transport.URL, time.Minute, "/tmp/output.mp4", "copy", true)
	joined := strings.Join(args, " ")
	for _, forbidden := range []string{"provider.example", "alice", "super-secret", "username=", "password="} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("FFmpeg argv leaked %q: %s", forbidden, joined)
		}
	}
	if !strings.Contains(joined, "http://127.0.0.1:") || !strings.Contains(joined, "/input/dvrinput_") {
		t.Fatalf("FFmpeg argv did not use opaque loopback input: %s", joined)
	}
}

func TestLiveTVTranscodeFFmpegArgvContainsOnlyOpaqueLoopbackInput(t *testing.T) {
	server := newScannerTestServer(t)
	tempDir, argsPath := configureFakeFFmpeg(t, server)
	transport, err := startLiveTVInputTransport(context.Background(), "channel-secret", "https://provider.example/live/alice/super-secret.ts", fixedLiveTVResolver{addresses: []netip.Addr{netip.MustParseAddr("203.0.113.20")}})
	if err != nil {
		t.Fatal(err)
	}
	item := MediaItem{
		ID: "channel-secret", Type: "live_channel", Title: "Secret Channel", SourceURL: "https://provider.example/live/alice/super-secret.ts",
		Streams: []Stream{{Kind: "video", Codec: "h264", Width: 640, Height: 360}, {Kind: "audio", Codec: "aac", Channels: 2}},
	}
	settings := transcodeSettings{Enabled: true, TemporaryDirectory: tempDir, MaxConcurrentSessions: 2, X264Preset: "veryfast", ThrottleBufferSeconds: 10}
	server.transcodeMu.Lock()
	session, err := server.startTranscodeLockedWithInputTransport("viewer", item, transport.URL, "328p", settings, "", 0, "transcode", "", false, false, transport)
	server.transcodeMu.Unlock()
	if err != nil {
		transport.Close()
		t.Fatal(err)
	}
	select {
	case <-session.done:
	case <-time.After(5 * time.Second):
		t.Fatal("live TV fake transcode did not finish")
	}
	args := readFakeFFmpegArgs(t, session, argsPath)
	for _, forbidden := range []string{"provider.example", "alice", "super-secret", "username=", "password="} {
		if strings.Contains(args, forbidden) {
			t.Fatalf("Live TV FFmpeg argv leaked %q: %s", forbidden, args)
		}
	}
	if !strings.Contains(args, "http://127.0.0.1:") || !strings.Contains(args, "/input/dvrinput_") {
		t.Fatalf("Live TV FFmpeg argv did not use opaque loopback input: %s", args)
	}
}

func TestDVRRestartReconciliationKeepsIncompletePublicationIncomplete(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	insertReleaseCandidateLiveSource(t, server, "source_incomplete_crash", "channel_incomplete_crash", 1)
	now := time.Now().UTC().Truncate(time.Second)
	stamp := now.Format(time.RFC3339)
	output := filepath.Join(server.cfg.AppDataDir, "recordings", "crash-window.mp4")
	if err := os.MkdirAll(filepath.Dir(output), 0o700); err != nil {
		t.Fatal(err)
	}
	working := dvrLeaseWorkingPath(output, "crash_lease")
	incomplete, ok := dvrIncompletePathFromLeaseWorkingPath(working)
	if !ok {
		t.Fatal("incomplete path was not derived")
	}
	if err := os.WriteFile(incomplete, []byte("retained partial media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO live_tv_recordings (id,user_id,profile_id,source_id,channel_id,title,status,starts_at,ends_at,path,created_at,updated_at) VALUES ('recording_incomplete_crash',?,?, 'source_incomplete_crash','channel_incomplete_crash','Crash window','running',?,?, ?,?,?)`, user.ID, viewerProfileID(user), now.Add(-time.Minute).Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339), working, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	stale := now.Add(-liveTVAllocationStaleAfter - time.Second).Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO live_tv_tuner_allocations (id,source_id,channel_id,allocation_kind,consumer_id,allocation_key,lease_token,acquired_at,heartbeat_at) VALUES ('allocation_incomplete_crash','source_incomplete_crash','channel_incomplete_crash','dvr_recording','recording_incomplete_crash','dvr_recording:recording_incomplete_crash','crash_lease',?,?)`, stale, stale); err != nil {
		t.Fatal(err)
	}
	if err := server.reconcileDVRStateAfterRestart(context.Background(), now); err != nil {
		t.Fatal(err)
	}
	var status, path string
	if err := server.db.QueryRow(`SELECT status,path FROM live_tv_recordings WHERE id='recording_incomplete_crash'`).Scan(&status, &path); err != nil {
		t.Fatal(err)
	}
	if status != "incomplete" || path != incomplete || path == output {
		t.Fatalf("status=%q path=%q output=%q", status, path, output)
	}
	if _, err := os.Stat(incomplete); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("normal final artifact unexpectedly exists: %v", err)
	}
}

func TestDVRInputTransportRejectsRebindingAndCrossAuthorityRedirect(t *testing.T) {
	_, err := startDVRInputTransport(context.Background(), "recording-1", "lease-1", "https://provider.example/live.ts",
		fixedLiveTVResolver{addresses: []netip.Addr{netip.MustParseAddr("203.0.113.20"), netip.MustParseAddr("10.0.0.9")}})
	if err == nil {
		t.Fatal("mixed public/private DNS answer was accepted")
	}
	approval, _, err := approveLiveTVEndpoint("https://provider.example/live.ts", "dvr-stream")
	if err != nil {
		t.Fatal(err)
	}
	client, err := newApprovedLiveTVHTTPClient(context.Background(), approval,
		fixedLiveTVResolver{addresses: []netip.Addr{netip.MustParseAddr("203.0.113.20")}})
	if err != nil {
		t.Fatal(err)
	}
	redirectURL, _ := url.Parse("https://redirect.example/stolen.ts")
	if err := client.CheckRedirect(&http.Request{URL: redirectURL}, nil); err == nil {
		t.Fatal("cross-authority DVR redirect was accepted")
	}
}

func TestDVRInputTransportForwardsOnlySanitizedProviderUserAgent(t *testing.T) {
	var receivedUserAgent string
	var receivedAuthorization string
	var receivedCookie string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUserAgent = r.Header.Get("User-Agent")
		receivedAuthorization = r.Header.Get("Authorization")
		receivedCookie = r.Header.Get("Cookie")
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write([]byte("opaque-provider-body"))
	}))
	defer provider.Close()

	handler := newDVRInputTransportHandler(
		"/input/test-capability",
		context.Background(),
		provider.URL+"?username=alice&password=super-secret",
		provider.Client(),
		"Provider/1.0\r\nInjected-Header",
	)
	loopback := httptest.NewServer(handler)
	defer loopback.Close()
	request, err := http.NewRequest(http.MethodGet, loopback.URL+"/input/test-capability", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("User-Agent", "loopback-client-secret")
	request.Header.Set("Authorization", "Bearer loopback-secret")
	request.Header.Set("Cookie", "credential=loopback-secret")
	response, err := loopback.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || string(body) != "opaque-provider-body" {
		t.Fatalf("loopback response status=%d body=%q", response.StatusCode, body)
	}
	if receivedUserAgent != "Provider/1.0 Injected-Header" || strings.ContainsAny(receivedUserAgent, "\r\n") {
		t.Fatalf("provider received unsanitized User-Agent %q", receivedUserAgent)
	}
	if receivedAuthorization != "" || receivedCookie != "" || strings.Contains(receivedUserAgent, "loopback-client-secret") {
		t.Fatalf("loopback credentials or User-Agent leaked upstream: ua=%q authorization=%q cookie=%q", receivedUserAgent, receivedAuthorization, receivedCookie)
	}
}

func TestDVRIncompleteArtifactCannotAliasCompletedOutput(t *testing.T) {
	working := "/recordings/show.lease-lease_123.partial.mp4"
	complete, completeOK := dvrFinalPathFromLeaseWorkingPath(working)
	incomplete, incompleteOK := dvrIncompletePathFromLeaseWorkingPath(working)
	if !completeOK || !incompleteOK || complete == incomplete {
		t.Fatalf("complete=%q/%v incomplete=%q/%v", complete, completeOK, incomplete, incompleteOK)
	}
	if complete != "/recordings/show.mp4" || incomplete != "/recordings/show.incomplete.mp4" {
		t.Fatalf("unexpected artifact paths: complete=%q incomplete=%q", complete, incomplete)
	}
}
