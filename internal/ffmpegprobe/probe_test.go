package ffmpegprobe

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (f *fakeClock) Now() time.Time { return f.now }

type fakeExecutor struct {
	mu       sync.Mutex
	calls    int
	results  []Execution
	commands []Command
}

func (f *fakeExecutor) Run(_ context.Context, cmd Command, _ int64) Execution {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.commands = append(f.commands, cmd)
	if len(f.results) == 0 {
		return Execution{ExitCode: 0}
	}
	r := f.results[0]
	f.results = f.results[1:]
	return r
}

func identity() Identity {
	return Identity{
		BinaryFingerprint: "sha256:one", FFmpegBuild: "7.1-portico", FFmpegConfigure: "sha256:cfg",
		Backend: "videotoolbox", DeviceIdentity: "gpu-0", DevicePath: "/dev/gpu0",
		DriverIdentity: "apple", DriverVersion: "1", OS: "darwin", Arch: "arm64", ConfigRevision: "rev-1",
	}
}
func specs() []Probe {
	return []Probe{
		{Name: "decode 10-bit", Kind: BitDepth10, Args: []string{"-f", "lavfi", "-i", "testsrc2", "-f", "null", "-"}, UnsupportedExitCodes: []int{22}, UnsupportedDiagnosticMarkers: []string{"probe: 10-bit decode unsupported"}},
		{Name: "reprobe output", Kind: OutputReprobe, Args: []string{"-show_streams", "output"}},
	}
}
func runner(ex Executor, clock Clock, cache Cache) Runner {
	return Runner{Binary: "/opt/portico/ffmpeg", Executor: ex, Clock: clock, Cache: cache, Timeout: time.Second, TTL: time.Hour, MaxOutputBytes: 64}
}

func TestProbeClassifiesExecutableResults(t *testing.T) {
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	clock := &fakeClock{now: now}
	tests := []struct {
		name      string
		execution Execution
		want      Status
	}{
		{"success", Execution{ExitCode: 0}, Supported},
		{"unsupported", Execution{ExitCode: 22, Err: errors.New("probe: 10-bit decode unsupported")}, Unsupported},
		{"same exit code device failure", Execution{ExitCode: 22, Err: errors.New("device initialization failed")}, Failed},
		{"error", Execution{ExitCode: 1, Err: errors.New("driver crashed")}, Failed},
		{"timeout", Execution{ExitCode: -1, Err: context.DeadlineExceeded, TimedOut: true}, TimedOut},
		{"unavailable", Execution{ExitCode: -1, Err: errors.New("not found"), Unavailable: true}, Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ex := &fakeExecutor{results: []Execution{tt.execution}}
			report, err := runner(ex, clock, nil).Probe(context.Background(), identity(), specs()[:1])
			if err != nil {
				t.Fatal(err)
			}
			if got := report.Results[0].Status; got != tt.want {
				t.Fatalf("status=%s want %s", got, tt.want)
			}
		})
	}
}

func TestListingPresenceIsNeverCapabilityProof(t *testing.T) {
	ex := &fakeExecutor{results: []Execution{{ExitCode: 22, Err: errors.New("listed encoder failed to initialize")}}}
	report, err := runner(ex, &fakeClock{now: time.Now()}, nil).Probe(context.Background(), identity(), []Probe{{Name: "listed h264", Kind: Encode, Args: []string{"-encoders"}, UnsupportedExitCodes: []int{22}, UnsupportedDiagnosticMarkers: []string{"probe: encoder unsupported"}}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Status != Failed {
		t.Fatalf("listing was incorrectly accepted: %+v", report.Results[0])
	}
}

func TestExactUnavailableDiagnosticMarker(t *testing.T) {
	probe := specs()[0]
	probe.UnavailableDiagnosticMarkers = []string{"probe: hardware device unavailable"}
	ex := &fakeExecutor{results: []Execution{{ExitCode: 22, Err: errors.New("probe: hardware device unavailable")}}}
	report, err := runner(ex, &fakeClock{now: time.Now()}, nil).Probe(context.Background(), identity(), []Probe{probe})
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Status != Unavailable {
		t.Fatalf("status=%s want %s", report.Results[0].Status, Unavailable)
	}
}

func TestCacheHitAndIdentityInvalidation(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	cache := NewMemoryCache(20)
	ex := &fakeExecutor{}
	r := runner(ex, clock, cache)
	first, err := r.Probe(context.Background(), identity(), specs())
	if err != nil {
		t.Fatal(err)
	}
	if first.FromCache {
		t.Fatal("first report came from cache")
	}
	second, err := r.Probe(context.Background(), identity(), specs())
	if err != nil {
		t.Fatal(err)
	}
	if !second.FromCache || ex.calls != 2 {
		t.Fatalf("cache miss: cached=%v calls=%d", second.FromCache, ex.calls)
	}
	mutations := []func(*Identity){
		func(i *Identity) { i.BinaryFingerprint = "sha256:two" }, func(i *Identity) { i.FFmpegBuild = "8" },
		func(i *Identity) { i.FFmpegConfigure = "other" }, func(i *Identity) { i.Backend = "vaapi" },
		func(i *Identity) { i.DeviceIdentity = "gpu-1" }, func(i *Identity) { i.DevicePath = "/dev/dri/renderD129" },
		func(i *Identity) { i.DriverIdentity = "mesa" }, func(i *Identity) { i.DriverVersion = "2" },
		func(i *Identity) { i.OS = "linux" }, func(i *Identity) { i.Arch = "amd64" }, func(i *Identity) { i.ConfigRevision = "rev-2" },
	}
	for n, mutate := range mutations {
		id := identity()
		mutate(&id)
		result, err := r.Probe(context.Background(), id, specs())
		if err != nil {
			t.Fatal(err)
		}
		if result.FromCache {
			t.Fatalf("mutation %d did not invalidate", n)
		}
	}
}

func TestCacheExpiryAndBoundedEviction(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	cache := NewMemoryCache(1)
	ex := &fakeExecutor{}
	r := runner(ex, clock, cache)
	r.TTL = time.Minute
	if _, err := r.Probe(context.Background(), identity(), specs()[:1]); err != nil {
		t.Fatal(err)
	}
	id2 := identity()
	id2.DeviceIdentity = "gpu-2"
	if _, err := r.Probe(context.Background(), id2, specs()[:1]); err != nil {
		t.Fatal(err)
	}
	third, err := r.Probe(context.Background(), identity(), specs()[:1])
	if err != nil {
		t.Fatal(err)
	}
	if third.FromCache {
		t.Fatal("evicted identity remained cached")
	}
	clock.now = clock.now.Add(2 * time.Minute)
	fourth, err := r.Probe(context.Background(), identity(), specs()[:1])
	if err != nil {
		t.Fatal(err)
	}
	if fourth.FromCache {
		t.Fatal("expired report remained cached")
	}
}

func TestTransientProbeResultsExpireQuicklyAndRecover(t *testing.T) {
	tests := []struct {
		name      string
		execution Execution
		want      Status
	}{
		{name: "error", execution: Execution{ExitCode: 1, Err: errors.New("driver temporarily failed")}, want: Error},
		{name: "timeout", execution: Execution{ExitCode: -1, Err: context.DeadlineExceeded, TimedOut: true}, want: TimedOut},
		{name: "unavailable", execution: Execution{ExitCode: -1, Err: errors.New("device disappeared"), Unavailable: true}, want: Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
			cache := NewMemoryCache(2)
			ex := &fakeExecutor{results: []Execution{tt.execution, {ExitCode: 0}}}
			r := runner(ex, clock, cache)
			r.TTL = 24 * time.Hour
			r.TransientTTL = 5 * time.Second
			first, err := r.Probe(context.Background(), identity(), specs()[:1])
			if err != nil {
				t.Fatal(err)
			}
			if first.Results[0].Status != tt.want || !first.ExpiresAt.Equal(clock.now.Add(5*time.Second)) {
				t.Fatalf("transient report=%+v", first)
			}
			clock.now = clock.now.Add(6 * time.Second)
			second, err := r.Probe(context.Background(), identity(), specs()[:1])
			if err != nil {
				t.Fatal(err)
			}
			if second.FromCache || second.Results[0].Status != Supported || ex.calls != 2 {
				t.Fatalf("recovery report=%+v calls=%d", second, ex.calls)
			}
			if !second.ExpiresAt.Equal(clock.now.Add(24 * time.Hour)) {
				t.Fatalf("supported expiry=%s", second.ExpiresAt)
			}
		})
	}
}

func TestUnsupportedProbeResultKeepsNormalTTL(t *testing.T) {
	clock := &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	ex := &fakeExecutor{results: []Execution{{ExitCode: 22, Err: errors.New("probe: 10-bit decode unsupported")}}}
	r := runner(ex, clock, NewMemoryCache(1))
	r.TTL = 24 * time.Hour
	r.TransientTTL = time.Second
	report, err := r.Probe(context.Background(), identity(), specs()[:1])
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Status != Unsupported || !report.ExpiresAt.Equal(clock.now.Add(24*time.Hour)) {
		t.Fatalf("report=%+v", report)
	}
}

func TestDiagnosticsAreBoundedAndSanitized(t *testing.T) {
	secretPath := "/opt/portico/ffmpeg"
	devicePath := "/dev/gpu0"
	output := secretPath + " " + devicePath + " password=hunter2 token=abc /Users/private/movie.mkv " + strings.Repeat("x", 200)
	ex := &fakeExecutor{results: []Execution{{ExitCode: 1, Err: errors.New("failed at /tmp/output"), Output: output, Truncated: true}}}
	report, err := runner(ex, &fakeClock{now: time.Now()}, nil).Probe(context.Background(), identity(), specs()[:1])
	if err != nil {
		t.Fatal(err)
	}
	result := report.Results[0]
	if !result.OutputTruncated {
		t.Fatal("truncation not retained")
	}
	for _, leak := range []string{secretPath, devicePath, "hunter2", "abc", "/Users", "/tmp"} {
		if strings.Contains(result.Diagnostic, leak) {
			t.Fatalf("diagnostic leaked %q: %s", leak, result.Diagnostic)
		}
	}
}

func TestRejectsIncompleteIdentityAndProbe(t *testing.T) {
	id := identity()
	id.DriverVersion = ""
	if _, err := runner(&fakeExecutor{}, &fakeClock{now: time.Now()}, nil).Probe(context.Background(), id, specs()); err == nil {
		t.Fatal("missing identity accepted")
	}
	if _, err := runner(&fakeExecutor{}, &fakeClock{now: time.Now()}, nil).Probe(context.Background(), identity(), []Probe{{Name: "empty", Kind: Decode}}); err == nil {
		t.Fatal("empty command accepted")
	}
}

func TestAllStructuredProbeKindsExecute(t *testing.T) {
	kinds := []Kind{Decode, Encode, BitDepth8, BitDepth10, HDRToneMap, ScaleDeinterlace, BitmapSubtitleBurn, DownloadReupload, OutputReprobe}
	probes := make([]Probe, 0, len(kinds))
	for _, kind := range kinds {
		probes = append(probes, Probe{Name: string(kind), Kind: kind, Args: []string{"-run", string(kind)}})
	}
	ex := &fakeExecutor{}
	report, err := runner(ex, &fakeClock{now: time.Now()}, nil).Probe(context.Background(), identity(), probes)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != len(kinds) || ex.calls != len(kinds) {
		t.Fatalf("results=%d calls=%d", len(report.Results), ex.calls)
	}
	for _, result := range report.Results {
		if result.Status != Supported {
			t.Fatalf("%s=%s", result.Kind, result.Status)
		}
	}
}
