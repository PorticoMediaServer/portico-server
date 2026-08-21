package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type blockedPlaybackReader struct {
	release chan struct{}
	once    sync.Once
}

func (r *blockedPlaybackReader) Read([]byte) (int, error) {
	<-r.release
	return 0, io.EOF
}

func (r *blockedPlaybackReader) Seek(offset int64, _ int) (int64, error) { return offset, nil }

func (r *blockedPlaybackReader) unblock() { r.once.Do(func() { close(r.release) }) }

func TestPlaybackStorageReadSeekerBoundsStalledReadAndRedactsPath(t *testing.T) {
	originalTimeout := storageIOOperationTimeout
	storageIOOperationTimeout = func(storageSourceClass) time.Duration { return 25 * time.Millisecond }
	t.Cleanup(func() { storageIOOperationTimeout = originalTimeout })

	blocked := &blockedPlaybackReader{release: make(chan struct{})}
	t.Cleanup(blocked.unblock)
	secretPath := filepath.Join(t.TempDir(), "owner-private", "movie.mkv")
	reader := &playbackStorageReadSeeker{
		server: &Server{},
		ctx:    context.Background(),
		path:   secretPath,
		file:   blocked,
	}
	started := time.Now()
	_, err := reader.Read(make([]byte, 1))
	if !errors.Is(err, errPlaybackStorageStalled) {
		t.Fatalf("Read error = %v, expected stalled storage error", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("stalled read returned after %s", elapsed)
	}
	if strings.Contains(err.Error(), secretPath) || strings.Contains(err.Error(), "owner-private") {
		t.Fatalf("public storage error disclosed path: %v", err)
	}
	blocked.unblock()
}

func TestServeLocalPlaybackFilePreservesRangeAndHeadSemantics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "media.bin")
	if err := os.WriteFile(path, []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := &Server{}

	rangeRequest := httptest.NewRequest(http.MethodGet, "/stream", nil)
	rangeRequest.Header.Set("Range", "bytes=2-5")
	rangeResponse := httptest.NewRecorder()
	if err := server.serveLocalPlaybackFile(rangeResponse, rangeRequest, path, "media.bin"); err != nil {
		t.Fatalf("serve range: %v", err)
	}
	if rangeResponse.Code != http.StatusPartialContent || rangeResponse.Body.String() != "2345" {
		t.Fatalf("range response status=%d body=%q", rangeResponse.Code, rangeResponse.Body.String())
	}
	if got := rangeResponse.Header().Get("Content-Range"); got != "bytes 2-5/10" {
		t.Fatalf("Content-Range = %q", got)
	}

	headRequest := httptest.NewRequest(http.MethodHead, "/stream", nil)
	headResponse := httptest.NewRecorder()
	if err := server.serveLocalPlaybackFile(headResponse, headRequest, path, "media.bin"); err != nil {
		t.Fatalf("serve HEAD: %v", err)
	}
	if headResponse.Code != http.StatusOK || headResponse.Body.Len() != 0 {
		t.Fatalf("HEAD response status=%d bytes=%d", headResponse.Code, headResponse.Body.Len())
	}
	if got := headResponse.Header().Get("Content-Length"); got != "10" {
		t.Fatalf("HEAD Content-Length = %q", got)
	}
}

type watchdogReadCloser struct {
	release chan struct{}
	once    sync.Once
}

func (r *watchdogReadCloser) Read([]byte) (int, error) {
	<-r.release
	return 0, errors.New("body closed")
}

func (r *watchdogReadCloser) Close() error {
	r.once.Do(func() { close(r.release) })
	return nil
}

func TestCopyRemotePlaybackBodyStopsAfterNoProgress(t *testing.T) {
	originalTimeout := storageIOOperationTimeout
	storageIOOperationTimeout = func(storageSourceClass) time.Duration { return 25 * time.Millisecond }
	t.Cleanup(func() { storageIOOperationTimeout = originalTimeout })

	body := &watchdogReadCloser{release: make(chan struct{})}
	response := httptest.NewRecorder()
	started := time.Now()
	err := copyRemotePlaybackBody(context.Background(), response, body)
	if !errors.Is(err, errPlaybackStorageStalled) {
		t.Fatalf("copy error = %v, expected stalled storage error", err)
	}
	if elapsed := time.Since(started); elapsed > 500*time.Millisecond {
		t.Fatalf("remote watchdog returned after %s", elapsed)
	}
}

func TestCopyRemotePlaybackBodyAllowsProgressingTransfer(t *testing.T) {
	originalTimeout := storageIOOperationTimeout
	storageIOOperationTimeout = func(storageSourceClass) time.Duration { return 100 * time.Millisecond }
	t.Cleanup(func() { storageIOOperationTimeout = originalTimeout })

	want := bytes.Repeat([]byte("portico"), 20_000)
	body := io.NopCloser(bytes.NewReader(want))
	response := httptest.NewRecorder()
	if err := copyRemotePlaybackBody(context.Background(), response, body); err != nil {
		t.Fatalf("copy progressing body: %v", err)
	}
	if !bytes.Equal(response.Body.Bytes(), want) {
		t.Fatalf("copied %d bytes, expected %d", response.Body.Len(), len(want))
	}
}
