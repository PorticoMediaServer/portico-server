package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/foundationcontract"
)

func TestMediaBodiesAndDownloadsUseDedicatedDeadlineFreeLanes(t *testing.T) {
	server := &Server{}
	_ = server.Handler()
	for _, testCase := range []struct {
		path string
		lane string
	}{
		{"/api/media/movie/stream", workloadLaneMediaBody},
		{"/api/media/movie/hls/index.m3u8", workloadLaneMediaBody},
		{"/api/media/movie/download", workloadLaneBulkTransfer},
		{"/api/live-tv/hls/channel/item", workloadLaneMediaBody},
		{"/api/live-tv/streams/channel", workloadLaneMediaBody},
	} {
		request := httptest.NewRequest(http.MethodGet, testCase.path, nil)
		descriptor := server.requestWork(request)
		if lane := descriptor.Lane; lane != testCase.lane {
			t.Errorf("%s used lane %q, want %q", testCase.path, lane, testCase.lane)
		}
		if budget := requestBudgetForWork(descriptor); budget != 0 {
			t.Errorf("%s received body deadline %s", testCase.path, budget)
		}
	}
}

func TestMixedPressurePreservesEstablishedMediaDelivery(t *testing.T) {
	lanes := newWorkloadLanes()
	for _, laneID := range []string{workloadLaneBrowsing, workloadLaneBulkTransfer} {
		lane := lanes[laneID]
		for index := 0; index < lane.capacity; index++ {
			if !lane.tryAcquireUncounted() {
				t.Fatalf("could not saturate %s", laneID)
			}
		}
	}
	delivery := lanes[workloadLaneMediaBody]
	if !delivery.tryAcquireUncounted() {
		t.Fatal("interactive navigation or bulk transfer pressure blocked media delivery")
	}
	delivery.release()

	governor := &mediaResourceGovernor{cpuCapacity: 2, diskCapacity: 4, networkCapacity: 4}
	releaseAnalysis, ok := governor.tryAcquire(mediaResourceRequest{class: foundationcontract.WorkClassBackgroundMedia, cpu: 1, disk: 1})
	if !ok {
		t.Fatal("expected one bounded analysis task to start")
	}
	defer releaseAnalysis()
	if _, ok := governor.tryAcquire(mediaResourceRequest{class: foundationcontract.WorkClassBackgroundMedia, cpu: 1}); ok {
		t.Fatal("background work consumed the CPU reserved for foreground playback")
	}
	releasePlayback, ok := governor.tryAcquire(mediaResourceRequest{class: foundationcontract.WorkClassPlaybackStart, cpu: 1, disk: 2})
	if !ok {
		t.Fatal("foreground playback could not use its reserved processing capacity")
	}
	releasePlayback()
}

func TestLiveTVSegmentCacheUsesLRUAndHashesKeys(t *testing.T) {
	cache := newLiveTVSegmentCache(6, time.Minute)
	cache.set("one", http.StatusOK, http.Header{"Content-Type": {"video/mp2t"}}, []byte("111"))
	cache.set("two", http.StatusOK, nil, []byte("222"))
	if _, ok := cache.get("one"); !ok {
		t.Fatal("expected first entry before eviction")
	}
	cache.set("three", http.StatusOK, nil, []byte("333"))
	if _, ok := cache.get("two"); ok {
		t.Fatal("least-recently-used entry was not evicted")
	}
	if _, ok := cache.get("one"); !ok {
		t.Fatal("recently accessed entry was evicted")
	}
	binding := liveTVHLSItemBinding{channelID: "channel", approval: liveTVEndpointApproval{Scheme: "https", Host: "example.test", Port: "443"}}
	if key := liveTVSegmentCacheKey(binding, "https://user:secret@example.test/live.ts?token=sensitive"); key == "" || key == "https://user:secret@example.test/live.ts?token=sensitive" {
		t.Fatalf("cache key must be an opaque digest, got %q", key)
	}
}

func TestLiveTVSegmentCacheCoalescesFlights(t *testing.T) {
	cache := newLiveTVSegmentCache(1024, time.Minute)
	flight, leader := cache.beginFlight("segment")
	if !leader {
		t.Fatal("first caller must lead a cache fill")
	}
	follower, leader := cache.beginFlight("segment")
	if leader || follower != flight {
		t.Fatal("concurrent caller must join the existing cache fill")
	}
	cache.finishFlight("segment", flight)
	select {
	case <-follower.done:
	case <-time.After(time.Second):
		t.Fatal("cache follower was not notified")
	}
	_, leader = cache.beginFlight("segment")
	if !leader {
		t.Fatal("completed cache flight was not released")
	}
}

func TestMediaResourceGovernorReservesForegroundCPU(t *testing.T) {
	governor := &mediaResourceGovernor{cpuCapacity: 2, diskCapacity: 2, networkCapacity: 2}
	releaseBackground, ok := governor.tryAcquire(mediaResourceRequest{class: foundationcontract.WorkClassBackgroundMedia, cpu: 1})
	if !ok {
		t.Fatal("first background task should be admitted")
	}
	defer releaseBackground()
	if _, ok := governor.tryAcquire(mediaResourceRequest{class: foundationcontract.WorkClassBackgroundMedia, cpu: 1}); ok {
		t.Fatal("background work must leave one CPU slot available")
	}
	releaseForeground, ok := governor.tryAcquire(mediaResourceRequest{class: foundationcontract.WorkClassPlaybackStart, cpu: 1})
	if !ok {
		t.Fatal("reserved foreground CPU slot should remain available")
	}
	releaseForeground()
}

func TestMediaResourceGovernorAcquireHonorsCancellation(t *testing.T) {
	governor := &mediaResourceGovernor{cpuCapacity: 1, diskCapacity: 1, networkCapacity: 1}
	release, ok := governor.tryAcquire(mediaResourceRequest{class: foundationcontract.WorkClassPlaybackStart, cpu: 1})
	if !ok {
		t.Fatal("expected initial acquisition")
	}
	defer release()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := governor.acquireContext(ctx, mediaResourceRequest{class: foundationcontract.WorkClassPlaybackStart, cpu: 1}); err == nil {
		t.Fatal("cancelled acquisition should fail")
	}
}

func TestMediaResourceGovernorPlaybackPreemptsRegisteredBackgroundWork(t *testing.T) {
	governor := newMediaResourceGovernor()
	backgroundCtx, unregister := governor.registerBackgroundContext(context.Background())
	defer unregister()
	governor.preemptBackgroundForPlayback()
	select {
	case <-backgroundCtx.Done():
		if !errors.Is(context.Cause(backgroundCtx), errRemoteStoragePreempted) {
			t.Fatalf("preemption cause = %v", context.Cause(backgroundCtx))
		}
	case <-time.After(time.Second):
		t.Fatal("background analysis was not preempted")
	}
}

func TestDueQueuedJobsFairIncludesEveryReadyLane(t *testing.T) {
	server := newScannerTestServer(t)
	now := time.Now().UTC()
	for index := 0; index < 8; index++ {
		created := now.Add(time.Duration(index-20) * time.Minute).Format(time.RFC3339)
		if _, err := server.db.Exec(`INSERT INTO jobs
			(id, type, status, progress, message, resource_type, resource_id, metadata_json, priority, created_at, updated_at)
			VALUES (?, 'media_analyze', 'queued', 0, 'Queued.', 'media', ?, '{}', ?, ?, ?)`,
			"analysis_fair_"+time.Duration(index).String(), "media_"+time.Duration(index).String(), foundationcontract.WorkClassBackgroundMedia, created, created); err != nil {
			t.Fatalf("insert analysis job: %v", err)
		}
	}
	created := now.Add(-time.Minute).Format(time.RFC3339)
	if _, err := server.db.Exec(`INSERT INTO jobs
		(id, type, status, progress, message, resource_type, resource_id, metadata_json, priority, created_at, updated_at)
		VALUES ('maintenance_fair', 'system_storage_cleanup', 'queued', 0, 'Queued.', 'maintenance', 'storage', '{}', ?, ?, ?)`, foundationcontract.WorkClassMaintenance, created, created); err != nil {
		t.Fatalf("insert maintenance job: %v", err)
	}
	jobs, err := server.dueQueuedJobsFair(now.Format(time.RFC3339))
	if err != nil {
		t.Fatalf("load fair jobs: %v", err)
	}
	foundMaintenance := false
	for _, job := range jobs {
		if job.ID == "maintenance_fair" {
			foundMaintenance = true
		}
	}
	if !foundMaintenance {
		t.Fatal("older analysis backlog starved the ready maintenance lane")
	}
}

func TestTranscodeUpdateSignalWakesWaiters(t *testing.T) {
	session := &transcodeSession{updateCh: make(chan struct{})}
	wait := session.updateSignal()
	session.markFailure(context.Canceled, false)
	select {
	case <-wait:
	case <-time.After(time.Second):
		t.Fatal("transcode state change did not wake waiters")
	}
}

func TestTranscodeGenerationRetirementWaitsForReaders(t *testing.T) {
	session := &transcodeSession{}
	release, ok := session.acquireReader()
	if !ok {
		t.Fatal("active generation should accept a reader")
	}
	result := make(chan bool, 1)
	go func() { result <- session.retireAndWait(time.Second) }()
	time.Sleep(20 * time.Millisecond)
	if _, ok := session.acquireReader(); ok {
		t.Fatal("retiring generation accepted a new reader")
	}
	release()
	select {
	case retired := <-result:
		if !retired {
			t.Fatal("generation did not retire after its reader released")
		}
	case <-time.After(time.Second):
		t.Fatal("retirement did not wake after its reader released")
	}
}

func TestTranscodeManifestReaderUsesProducerNotification(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "index.m3u8")
	session := &transcodeSession{dir: dir, manifest: path, done: make(chan struct{}), updateCh: make(chan struct{})}
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "segment_00000.ts"), []byte("segment"), 0o600)
		_ = os.WriteFile(path, []byte("#EXTM3U\n#EXT-X-TARGETDURATION:4\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:4,\nsegment_00000.ts\n"), 0o600)
		session.stateMu.Lock()
		session.signalUpdateLocked()
		session.stateMu.Unlock()
	}()
	started := time.Now()
	manifest, err := (&Server{}).readTranscodeManifest(session, "movie", "original", "", 0, "copy", "", false, "")
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if manifest == "" || time.Since(started) >= 500*time.Millisecond {
		t.Fatalf("manifest reader did not wake promptly; elapsed=%s", time.Since(started))
	}
}

func TestTranscodeSegmentWaitUsesProducerNotification(t *testing.T) {
	path := filepath.Join(t.TempDir(), "segment_00000.ts")
	session := &transcodeSession{done: make(chan struct{}), updateCh: make(chan struct{}), admissionActive: true, lastProducedAt: time.Now()}
	go func() {
		time.Sleep(25 * time.Millisecond)
		_ = os.WriteFile(path, []byte("segment"), 0o600)
		session.stateMu.Lock()
		session.signalUpdateLocked()
		session.stateMu.Unlock()
	}()
	started := time.Now()
	if err := waitForHLSSegmentFile(session, path); err != nil {
		t.Fatalf("wait segment: %v", err)
	}
	if time.Since(started) >= 500*time.Millisecond {
		t.Fatalf("segment waiter did not wake promptly; elapsed=%s", time.Since(started))
	}
}

func TestDVRRecordingInputURLAndReconnectPolicy(t *testing.T) {
	if err := validateDVRRecordingInputURL("file:///etc/passwd"); err == nil {
		t.Fatal("DVR must reject non-network input schemes")
	}
	if err := validateDVRRecordingInputURL("https://example.test/live.m3u8"); err != nil {
		t.Fatalf("expected HTTPS DVR source to be accepted: %v", err)
	}
	args := dvrRecordingFFmpegArgs("https://example.test/live.m3u8", time.Minute, "/tmp/test.mp4", "copy", true)
	if !containsString(args, "-reconnect") || !containsString(args, "-reconnect_streamed") {
		t.Fatalf("network DVR input must enable FFmpeg reconnect support: %v", args)
	}
}
