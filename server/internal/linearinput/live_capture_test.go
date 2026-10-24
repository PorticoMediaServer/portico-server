package linearinput_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/decoder"
	"portico.local/server/internal/decodertest"
	"portico.local/server/internal/linearbuffer"
	"portico.local/server/internal/linearinput"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/storage"
)

// Exercise the real rolling HLS origin -> shared gateway -> confined probe ->
// confined copy/mux -> private publisher path beyond the configured startup
// buffer. A finite three-second probe cannot establish this continuation.
func TestConfinedRollingLiveCaptureAdvancesPastStartupBuffer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 55*time.Second)
	defer cancel()
	if !decoder.CheckConfinement(ctx) {
		decodertest.Unavailable(t, "decoder confinement unavailable")
	}
	ffmpeg, ffprobe := decodertest.QualifiedFFmpeg(t), decodertest.QualifiedFFprobe(t)
	ffmpeg, _ = filepath.EvalSymlinks(ffmpeg)
	ffprobe, _ = filepath.EvalSymlinks(ffprobe)
	libraries, err := decoder.ResolveLibraries(ffmpeg, ffprobe, nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	pattern := filepath.Join(dir, "pattern.mp4")
	generate := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=25", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "6", "-c:v", "libx264", "-threads:v", "1", "-g", "50", "-keyint_min", "50", "-sc_threshold", "0", "-pix_fmt", "yuv420p", "-c:a", "aac", pattern)
	if output, err := generate.CombinedOutput(); err != nil {
		t.Fatalf("generate procedural source: %v: %s", err, output)
	}
	origin := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-v", "error", "-re", "-stream_loop", "-1", "-i", pattern, "-c", "copy", "-f", "hls", "-hls_time", "2", "-hls_list_size", "12", "-hls_delete_threshold", "4", "-hls_flags", "delete_segments+omit_endlist+temp_file", "-hls_segment_filename", filepath.Join(dir, "segment-%09d.ts"), filepath.Join(dir, "index.m3u8"))
	if err = origin.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = origin.Process.Kill(); _ = origin.Wait() }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		data, _ := os.ReadFile(filepath.Join(dir, "index.m3u8"))
		if strings.Count(string(data), "#EXTINF:") >= 3 {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("procedural origin did not publish", ctx.Err())
		case <-ticker.C:
		}
	}
	server := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer server.Close()
	approval, err := (remotemedia.Policy{}).Approve(ctx, server.URL+"/")
	if err != nil {
		t.Fatal(err)
	}
	gateway, err := linearinput.Open(ctx, livechannels.Input{SourceID: server.URL, ChannelID: "procedural", Locator: server.URL + "/index.m3u8", Policy: remotemedia.Policy{Approvals: []remotemedia.Approval{approval}}, RetryWindowSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	defer gateway.Close()
	supervisor := storage.New("").Supervisor
	reservation, err := gateway.Reserve()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = decoder.LinearProbe(ctx, supervisor, "rolling-live-probe", ffprobe, gateway.URL(), true, reservation, nil, libraries...); err != nil {
		t.Fatal("confined source probe", err)
	}
	buffer, err := linearbuffer.New(linearbuffer.Options{Parent: dir, OriginMS: time.Now().UnixMilli(), Retention: time.Minute, MaxBytes: 64 << 20, LiveDelay: 8 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer buffer.Close()
	publisher, err := buffer.Publisher(linearbuffer.Producer{ID: 1}, func() error { return ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	defer publisher.Close()
	output, err := gateway.AttachOutput(publisher)
	if err != nil {
		t.Fatal(err)
	}
	reservation, err = gateway.Reserve()
	if err != nil {
		t.Fatal(err)
	}
	work, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- decoder.RunLinearHLS(work, supervisor, "rolling-live-capture", ffmpeg, gateway.URL(), output, decoder.LinearEncoding{HLSInput: true, Video: true, Audio: true}, reservation, nil, libraries...)
	}()
	defer func() { stop(); <-done }()
	for {
		window := buffer.Window()
		if window.Ready && window.EndUS-window.StartUS >= 12_000_000 {
			return
		}
		select {
		case err := <-done:
			done <- err // cleanup still waits for the physically reaped worker
			if err == nil {
				err = errors.New("live capture exited")
			}
			t.Fatalf("producer exited before ready: %+v: %v", window, err)
		case <-ctx.Done():
			t.Fatalf("producer never advanced past startup buffer: %+v", window)
		case <-ticker.C:
		}
	}
}
