package linearbuffer

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/decodertest"
	"strings"
	"testing"
	"time"
)

// Synthetic muxer/clock contract test, not an alternate production execution
// path. Production FFmpeg remains behind decoder confinement and owned input.
func TestFFmpegPreservedMidProgrammeClock(t *testing.T) {
	ffmpeg := decodertest.QualifiedFFmpeg(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	source := filepath.Join(t.TempDir(), "fixture.mp4")
	generate := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=24", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000", "-t", "12", "-c:v", "libx264", "-threads:v", "1", "-g", "96", "-keyint_min", "96", "-sc_threshold", "0", "-pix_fmt", "yuv420p", "-c:a", "aac", "-movflags", "+faststart", source)
	if output, e := generate.CombinedOutput(); e != nil {
		t.Fatalf("generate fixture: %v: %s", e, output)
	}
	b := testBuffer(t, true)
	p := publisher(t, b, 1, b.options.OriginMS)
	p.producer.EndMS = p.producer.TimelineBaseMS + 10000
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/")
		raw, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(raw))
		rec := httptest.NewRecorder()
		p.ServeHTTP(rec, r)
		if rec.Code != http.StatusCreated {
			t.Logf("publisher rejected %s: %d %s %s", r.URL.Path, rec.Code, rec.Body.String(), raw)
			p.mu.Lock()
			t.Logf("last=%d pending=%v", p.last, p.pending)
			p.mu.Unlock()
		}
		for k, values := range rec.Header() {
			w.Header()[k] = values
		}
		w.WriteHeader(rec.Code)
		_, _ = w.Write(rec.Body.Bytes())
	}))
	defer server.Close()
	cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-loglevel", "error", "-copyts", "-start_at_zero", "-ss", "5", "-i", source, "-t", "10", "-map", "0:v:0", "-map", "0:a:0", "-c", "copy", "-avoid_negative_ts", "disabled", "-muxdelay", "0", "-f", "hls", "-hls_time", "4", "-hls_list_size", "12", "-hls_flags", "independent_segments+program_date_time", "-hls_segment_options", "mpegts_copyts=1", "-method", "PUT", "-hls_segment_filename", server.URL+"/segment-%09d.ts", server.URL+"/index.m3u8")
	if output, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("publish fixture: %v: %s", e, output)
	}
	if e := p.Finish(ctx); e != nil {
		t.Fatal("incomplete output", e)
	}
	w := b.Window()
	if !w.Ready || w.StartUS < 3900000 || w.StartUS > 4100000 || w.EndUS < 9900000 || w.EndUS > 10100000 {
		t.Fatalf("copyts lost selected source clock: %+v", w)
	}
	target := int64(5000000)
	if actual, e := b.Resolve(&target); e != nil || actual != target {
		t.Fatalf("mid-programme target: %d %v", actual, e)
	}
}
