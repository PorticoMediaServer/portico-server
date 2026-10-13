package linearinput_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"portico.local/server/internal/linearinput"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/remotemedia"
	"sync/atomic"
	"testing"
	"time"
)

func sharedFixture(t *testing.T) (livechannels.Input, *atomic.Int32, *httptest.Server) {
	t.Helper()
	count := &atomic.Int32{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		packet := make([]byte, 188*256)
		for i := 0; i < len(packet); i += 188 {
			packet[i] = 0x47
			packet[i+3] = 0x10
		}
		ticker := time.NewTicker(10 * time.Millisecond)
		defer ticker.Stop()
		for {
			if _, e := w.Write(packet); e != nil {
				return
			}
			w.(http.Flusher).Flush()
			select {
			case <-r.Context().Done():
				return
			case <-ticker.C:
			}
		}
	}))
	approval, e := (remotemedia.Policy{}).Approve(context.Background(), server.URL+"/")
	if e != nil {
		server.Close()
		t.Fatal(e)
	}
	return livechannels.Input{Locator: server.URL + "/stream", SourceID: server.URL, ChannelID: "one", Policy: remotemedia.Policy{Approvals: []remotemedia.Approval{approval}}, RetryWindowSeconds: 5}, count, server
}
func TestSharedChannelReadersHaveIndependentLifetime(t *testing.T) {
	in, count, server := sharedFixture(t)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	first, e := linearinput.Open(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	second, e := linearinput.Open(ctx, in)
	if e != nil {
		first.Close()
		t.Fatal(e)
	}
	if count.Load() != 1 {
		t.Fatalf("upstream connections: %d", count.Load())
	}
	response, e := http.Get(second.URL())
	if e != nil {
		t.Fatal(e)
	}
	b := make([]byte, 188)
	if _, e = io.ReadFull(response.Body, b); e != nil {
		t.Fatal(e)
	}
	if e = first.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = io.ReadFull(response.Body, b); e != nil {
		t.Fatalf("one viewer stopped another: %v", e)
	}
	response.Body.Close()
	if e = second.Close(); e != nil {
		t.Fatal(e)
	}
	third, e := linearinput.Open(ctx, in)
	if e != nil {
		t.Fatal(e)
	}
	if count.Load() != 2 {
		t.Fatalf("idle channel not retired: %d", count.Load())
	}
	third.Close()
}
