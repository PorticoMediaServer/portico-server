package app

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type memoryRangeBackend struct {
	data       string
	statObject storageObject
	mu         sync.Mutex
	calls      []string
}

func (b *memoryRangeBackend) Kind() string { return "memory" }
func (b *memoryRangeBackend) Inventory(context.Context, string, int) (storageInventoryPage, error) {
	return storageInventoryPage{}, nil
}
func (b *memoryRangeBackend) OpenRange(_ context.Context, object string, offset, length int64) (io.ReadCloser, error) {
	b.mu.Lock()
	b.calls = append(b.calls, object+":"+strconv.FormatInt(offset, 10)+":"+strconv.FormatInt(length, 10))
	b.mu.Unlock()
	return io.NopCloser(strings.NewReader(b.data[offset : offset+length])), nil
}
func (b *memoryRangeBackend) Stat(context.Context, string) (storageObject, error) {
	return b.statObject, nil
}

func TestRemoteCompleteRequiresStableProviderValidator(t *testing.T) {
	backend := &memoryRangeBackend{statObject: storageObject{Revision: "\x0042\x00\x00", Size: 42}}
	if _, err := statRemoteStorageObject(context.Background(), backend, "Movies/Film.mkv"); err == nil || !strings.Contains(err.Error(), "stable object validator") {
		t.Fatalf("validator error=%v", err)
	}
	backend.statObject.ETag = "etag-1"
	if _, err := statRemoteStorageObject(context.Background(), backend, "Movies/Film.mkv"); err != nil {
		t.Fatalf("stable validator rejected: %v", err)
	}
}

func TestRemoteProbeProxyServesExactLoopbackRanges(t *testing.T) {
	backend := &memoryRangeBackend{data: "0123456789"}
	probeURL, closeProxy, err := startRemoteProbeProxy(context.Background(), backend, "Movies/Film.mp4", 10)
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()
	if !strings.HasPrefix(probeURL, "http://127.0.0.1:") {
		t.Fatalf("proxy escaped loopback: %q", probeURL)
	}
	req, _ := http.NewRequest(http.MethodGet, probeURL, nil)
	req.Header.Set("Range", "bytes=7-9")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil || resp.StatusCode != http.StatusPartialContent || string(body) != "789" || resp.Header.Get("Content-Range") != "bytes 7-9/10" {
		t.Fatalf("status=%d range=%q body=%q err=%v", resp.StatusCode, resp.Header.Get("Content-Range"), body, readErr)
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	if len(backend.calls) != 1 || backend.calls[0] != "Movies/Film.mp4:7:3" {
		t.Fatalf("provider calls=%v", backend.calls)
	}
}

func TestRemoteCompleteStagesExactObjectAndReleasesReservation(t *testing.T) {
	server := newScannerTestServer(t)
	backend := &memoryRangeBackend{data: strings.Repeat("complete-analysis-", 1024)}
	path, cleanup, err := server.materializeRemoteObjectForCompleteAnalysis(context.Background(), backend, "Movies/Film.mkv", int64(len(backend.data)))
	if err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != backend.data {
		t.Fatalf("staged object mismatch: bytes=%d err=%v", len(contents), err)
	}
	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("staged object survived cleanup: %v", err)
	}
	governor := server.mediaResourceGovernor()
	governor.mu.Lock()
	defer governor.mu.Unlock()
	if len(governor.diskReservedBytes) != 0 {
		t.Fatalf("disk reservations leaked: %v", governor.diskReservedBytes)
	}
}

func TestFreshRemoteVideoLazilyProbesPlaybackFacts(t *testing.T) {
	if testing.Short() {
		t.Skip("uses a subprocess ffprobe stub")
	}
	server := newScannerTestServer(t)
	stub := filepath.Join(t.TempDir(), "ffprobe-stub")
	invocations := filepath.Join(t.TempDir(), "ffprobe-invocations")
	probeJSON := `{"format":{"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"120","bit_rate":"2500000"},"streams":[{"index":0,"codec_type":"video","codec_name":"h264","profile":"Main","width":1280,"height":720,"pix_fmt":"yuv420p","sample_aspect_ratio":"1:1","avg_frame_rate":"24/1"},{"index":1,"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2,"channel_layout":"stereo","sample_rate":"48000"}]}`
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nprintf x >> '"+invocations+"'\nprintf '%s' '"+probeJSON+"'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	server.cfg.FFprobePath = stub
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Remote movies", Type: "movie", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	source, err := server.createRemoteStorageSource(context.Background(), library.ID, RemoteStorageSourceRequest{Kind: "webdav", Name: "Archive", Endpoint: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	locator := remoteStorageLocator(source.ID, "Movies/Film.mp4")
	now := time.Now().UTC().Format(time.RFC3339Nano)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO storage_remote_objects(source_id,object_path,revision,size_bytes,first_seen_generation,last_seen_generation,updated_at) VALUES(?,'Movies/Film.mp4','rev-1',1000,'gen-1','gen-1',?)`, []any{source.ID, now}},
		{`INSERT INTO media_items(id,library_id,type,title,sort_title,source_url,added_at) VALUES('remote_probe_movie',?,'movie','Remote Probe','Remote Probe',?,?)`, []any{library.ID, locator, now}},
		{`INSERT INTO media_files(id,media_id,library_id,path,container,source_type,size_bytes,available,first_seen_at,last_seen_at,content_fingerprint) VALUES('remote_probe_file','remote_probe_movie',?,?,'mp4','remote',1000,1,?,?,'remote-rev-1')`, []any{library.ID, locator, now, now}},
		{`INSERT INTO media_streams(id,media_id,file_id,source_kind,stream_index,kind,codec,display_title) VALUES('remote_probe_generic_video','remote_probe_movie','remote_probe_file','scanner',-1,'video','h264','MP4 video')`, nil},
		{`INSERT INTO media_streams(id,media_id,file_id,source_kind,stream_index,kind,codec,channels,display_title) VALUES('remote_probe_generic_audio','remote_probe_movie','remote_probe_file','scanner',-1,'audio','aac',2,'MP4 audio')`, nil},
	}
	for index, statement := range statements {
		if _, err := server.db.Exec(statement.query, statement.args...); err != nil {
			t.Fatalf("fixture statement %d: %v", index, err)
		}
	}
	item, err := server.getMediaBackgroundSourceSeedContext(context.Background(), "remote_probe_movie")
	if err != nil {
		t.Fatal(err)
	}
	item.Streams, _ = server.listStreamsContext(context.Background(), item.ID)
	item.MediaFiles = server.primaryMediaFileForPlaybackContext(context.Background(), item.ID, item.SourceURL)
	facts, _, err := server.mediaFactsForPlayback(context.Background(), item)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts.Video) != 1 || facts.Video[0].CodedWidth != 1280 || len(facts.Audio) != 1 || facts.Audio[0].Channels != 2 {
		t.Fatalf("facts=%+v", facts)
	}
	if _, err := server.db.Exec(`UPDATE media_files SET content_fingerprint='remote-rev-2' WHERE id='remote_probe_file'`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := server.mediaFactsForPlayback(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	count, err := os.ReadFile(invocations)
	if err != nil || string(count) != "xx" {
		t.Fatalf("remote revision change did not re-probe: count=%q err=%v", count, err)
	}
	var persistedFingerprint string
	if err := server.db.QueryRow(`SELECT source_fingerprint FROM media_analysis_facts WHERE media_id='remote_probe_movie' AND media_file_id='remote_probe_file'`).Scan(&persistedFingerprint); err != nil || persistedFingerprint != "remote-rev-2" {
		t.Fatalf("persisted fingerprint=%q err=%v", persistedFingerprint, err)
	}
}

func TestReplacedRemoteScannerSelectionMapsToProbedDefault(t *testing.T) {
	previous := []Stream{{ID: "generic-audio", Kind: "audio", SourceKind: "scanner"}}
	current := []Stream{
		{ID: "probe-audio-1", Kind: "audio", SourceKind: "ffprobe"},
		{ID: "probe-audio-2", Kind: "audio", SourceKind: "ffprobe", Default: true},
	}
	if got := remapReplacedScannerStream(previous, current, "generic-audio", "audio"); got != "probe-audio-2" {
		t.Fatalf("remapped selection=%q", got)
	}
	if got := remapReplacedScannerStream(previous, current, "unknown", "audio"); got != "unknown" {
		t.Fatalf("non-scanner selection changed=%q", got)
	}
}
