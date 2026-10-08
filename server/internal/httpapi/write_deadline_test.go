package httpapi

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"portico.local/server/internal/testtier"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// The risk in a rolling deadline is getting it backwards and cutting off the
// client least able to recover: someone on a poor connection, part way through a
// long file. So this reads half a megabyte at a hundred kilobytes a second —
// five seconds of transfer, twenty-five refreshes of a two-second stall budget —
// and requires every byte to arrive.
func TestASlowButAliveClientIsNeverDropped(t *testing.T) {
	testtier.Media(t, "a slow client paced over real write deadlines (about 5 s)")
	restore := shortDeadlines(t, 2*time.Second, 200*time.Millisecond)
	defer restore()
	const total = 512 << 10
	payload := bytes.Repeat([]byte("p"), total)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := withRollingDeadline(w)
		defer body.release()
		http.ServeContent(body, r, "media.mp4", time.Time{}, bytes.NewReader(payload))
	}))
	defer server.Close()
	response, err := http.Get(server.URL + "/v1/media/grant")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	// 100 kB/s: eight kilobytes every eighty milliseconds.
	read := 0
	buffer := make([]byte, 8<<10)
	for read < total {
		time.Sleep(80 * time.Millisecond)
		n, err := io.ReadFull(response.Body, buffer)
		read += n
		if err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			t.Fatalf("a client reading at 100 kB/s was dropped after %d of %d bytes: %v", read, total, err)
		}
	}
	if read != total {
		t.Fatalf("the slow client received %d of %d bytes", read, total)
	}
}

// The other half: a client that stops reading entirely must not hold the
// goroutine, its lane slot and its open file until the operating system gives
// up, which can be minutes.
func TestAClientThatStopsReadingIsReclaimedWithinTheDeadline(t *testing.T) {
	restore := shortDeadlines(t, 300*time.Millisecond, 50*time.Millisecond)
	defer restore()
	var returned atomic.Bool
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		body := withRollingDeadline(w)
		defer body.release()
		// Far more than any socket buffer, so the write blocks once the client
		// stops reading.
		http.ServeContent(body, r, "media.mp4", time.Time{}, bytes.NewReader(bytes.Repeat([]byte("p"), 64<<20)))
		returned.Store(true)
	}))
	defer server.Close()
	connection, err := net.Dial("tcp", strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	if _, err = fmt.Fprintf(connection, "GET /v1/media/grant HTTP/1.1\r\nHost: x\r\n\r\n"); err != nil {
		t.Fatal(err)
	}
	// Read nothing at all from here on.
	select {
	case <-finished:
		if !returned.Load() {
			t.Fatal("the handler ended without returning from the body write")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("a client that stopped reading held the handler past the write deadline")
	}
}

// A frame deadline must not leak into the next response on a keep-alive
// connection, or an ordinary request minutes later inherits a deadline in the
// past and fails for no reason.
func TestTheRollingDeadlineIsReleasedWithTheBody(t *testing.T) {
	restore := shortDeadlines(t, 300*time.Millisecond, 50*time.Millisecond)
	defer restore()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/media/grant" {
			body := withRollingDeadline(w)
			body.Write([]byte("bytes"))
			body.release()
			return
		}
		time.Sleep(600 * time.Millisecond)
		w.Write([]byte("late but fine"))
	}))
	defer server.Close()
	client := &http.Client{}
	first, err := client.Get(server.URL + "/v1/media/grant")
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, first.Body)
	first.Body.Close()
	second, err := client.Get(server.URL + "/v1/items/one")
	if err != nil {
		t.Fatalf("the next request on the same connection failed: %v", err)
	}
	defer second.Body.Close()
	raw, err := io.ReadAll(second.Body)
	if err != nil || string(raw) != "late but fine" {
		t.Fatalf("a deadline leaked into the next response: %q %v", raw, err)
	}
}

func shortDeadlines(t *testing.T, window, refresh time.Duration) func() {
	t.Helper()
	previousWindow, previousRefresh, previousFrame := mediaWriteDeadline, deadlineRefreshInterval, streamFrameDeadline
	mediaWriteDeadline, deadlineRefreshInterval, streamFrameDeadline = window, refresh, window
	return func() {
		mediaWriteDeadline, deadlineRefreshInterval, streamFrameDeadline = previousWindow, previousRefresh, previousFrame
	}
}
