package administration

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCachedUpdatesChecksInBackgroundAndReportsFailureAsUnavailable(t *testing.T) {
	s, _, _ := newService(t)
	started := make(chan struct{}, 2)
	release := make(chan struct{})
	calls := 0
	s.Fetch = func(ctx context.Context, rawURL string) ([]byte, error) {
		calls++
		started <- struct{}{}
		select {
		case <-release:
			return nil, errors.New("feed offline")
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	current := UpdateBuild{Version: "1.2.0"}
	first := s.CachedUpdates(context.Background(), current, "stable", "https://example.invalid/feed")
	if first.State != "unavailable" || !first.FeedConfigured || first.CheckedAt != "" {
		t.Fatalf("first read blocked or misreported feed: %+v", first)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background fetch never started")
	}
	second := s.CachedUpdates(context.Background(), current, "stable", "https://example.invalid/feed")
	if second.State != "unavailable" || calls != 1 {
		t.Fatalf("request repeated in-flight fetch: %+v, calls=%d", second, calls)
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		result := s.CachedUpdates(context.Background(), current, "stable", "https://example.invalid/feed")
		if result.CheckedAt != "" {
			if result.State != "unavailable" || result.Status != "unavailable" || !result.FeedConfigured || calls != 1 {
				t.Fatalf("failed configured check: %+v, calls=%d", result, calls)
			}
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("background result was not cached")
}

func TestFetchReleaseFeedBoundsTheResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"channels":{"stable":[{"version":"1.2.0"}]}}`))
	}))
	defer server.Close()
	data, err := FetchReleaseFeed(context.Background(), server.URL)
	if err != nil || len(data) == 0 {
		t.Fatalf("configured feed: %q %v", data, err)
	}
	if _, err := FetchReleaseFeed(context.Background(), "file:///etc/passwd"); !errors.Is(err, ErrInput) {
		t.Fatalf("non-HTTP feed accepted: %v", err)
	}
}
