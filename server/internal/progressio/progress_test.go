package progressio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlowResponseExtendsDeadlineAndStallEnds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for i := 0; i < 8; i++ {
			_, _ = w.Write([]byte("x"))
			w.(http.Flusher).Flush()
			select {
			case <-time.After(10 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
		}
		if r.URL.Path == "/stall" {
			<-r.Context().Done()
		}
	}))
	defer server.Close()
	client := &http.Client{Transport: Transport{Base: server.Client().Transport, Idle: 50 * time.Millisecond, Overall: time.Second}}
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || len(data) != 8 {
		t.Fatalf("progressing body failed %q %v", data, err)
	}
	response, err = client.Get(server.URL + "/stall")
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.ReadAll(response.Body)
	response.Body.Close()
	if err == nil {
		t.Fatal("stalled response succeeded")
	}
}
func TestOverallSafetyCapAndParentCancellation(t *testing.T) {
	ctx, watch := New(context.Background(), time.Second, 20*time.Millisecond)
	defer watch.Close()
	<-ctx.Done()
	parent, cancel := context.WithCancel(context.Background())
	ctx, other := New(parent, time.Second, 0)
	defer other.Close()
	cancel()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("lost parent cancellation")
	}
}

func TestWaitingForAdmissionDoesNotStartIdleTimer(t *testing.T) {
	ctx, watch := NewDormant(context.Background(), 20*time.Millisecond)
	defer watch.Close()
	time.Sleep(40 * time.Millisecond)
	if ctx.Err() != nil {
		t.Fatal("resource queue consumed the producer deadline")
	}
	watch.Ping()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("started producer had no idle bound")
	}
}

func TestQueuedExpiryDoesNotCancelNewProgress(t *testing.T) {
	ctx, watch := New(context.Background(), 100*time.Millisecond, 0)
	defer watch.Close()
	// Queue actual progress first, then let the old callback queue on the same
	// mutex. Go's mutex hands off to the long-waiting progress goroutine first.
	watch.mu.Lock()
	started, pinged := make(chan struct{}), make(chan struct{})
	go func() { close(started); watch.Ping(); close(pinged) }()
	<-started
	time.Sleep(160 * time.Millisecond)
	watch.mu.Unlock()
	<-pinged
	select {
	case <-ctx.Done():
		t.Fatal("queued expiry cancelled newly progressing work")
	case <-time.After(20 * time.Millisecond):
	}
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("real idle stall did not expire")
	}
}
