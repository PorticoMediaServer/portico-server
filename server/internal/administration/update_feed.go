package administration

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	"portico.local/server/internal/remotemedia"
	"portico.local/server/internal/supervise"
)

type cachedUpdateFeed struct {
	data     []byte
	err      error
	checked  time.Time
	inFlight bool
}

// FetchReleaseFeed is installed by the composition root. Only an owner-set
// HTTP(S) URL is read, with a bounded request and response. A redirect must
// remain within that origin and cannot downgrade TLS or carry URL credentials.
func FetchReleaseFeed(ctx context.Context, rawURL string) ([]byte, error) {
	if !safeFeedURL(rawURL) {
		return nil, ErrInput
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	locator, err := url.Parse(rawURL)
	if err != nil {
		return nil, ErrInput
	}
	// Configuring this origin is the owner's explicit permission to fetch it,
	// including a private feed. The shared policy still blocks metadata services,
	// pins DNS answers and prevents redirects escaping to another origin.
	policy := remotemedia.Policy{ReadTimeout: 8 * time.Second}
	approval, err := policy.Approve(ctx, locator.Scheme+"://"+locator.Host+"/")
	if err != nil {
		return nil, err
	}
	policy.Approvals = []remotemedia.Approval{approval}
	client, err := remotemedia.New(ctx, rawURL, policy)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	response, err := client.Open(ctx, http.MethodGet, "")
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, errors.New("release feed response unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, errors.New("release feed exceeds byte limit")
	}
	return data, nil
}

func safeFeedURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && u.Hostname() != "" && (u.Scheme == "https" || u.Scheme == "http")
}

// CachedUpdates never performs network I/O on the API request. The first
// read starts one background check; subsequent reads use the last result.
// Successful checks refresh every six hours and failures retry after five
// minutes. A changed feed URL gets its own cache entry.
func (s *Service) CachedUpdates(ctx context.Context, current UpdateBuild, channel, feedURL string) UpdateReport {
	if feedURL == "" {
		return s.updatesFrom(ctx, current, channel, feedURL, nil)
	}
	if s.Fetch == nil {
		return s.updatesFrom(ctx, current, channel, feedURL, nil)
	}
	s.updateMu.Lock()
	if s.updateFeeds == nil {
		s.updateFeeds = make(map[string]*cachedUpdateFeed)
	}
	entry := s.updateFeeds[feedURL]
	if entry == nil {
		entry = &cachedUpdateFeed{}
		s.updateFeeds[feedURL] = entry
	}
	data, priorErr, checked := entry.data, entry.err, entry.checked
	staleAfter := 6 * time.Hour
	if priorErr != nil {
		staleAfter = 5 * time.Minute
	}
	if s.Fetch != nil && !entry.inFlight && (checked.IsZero() || s.now().Sub(checked) >= staleAfter) {
		entry.inFlight = true
		fetch := s.Fetch
		supervise.Go("administration.update-feed", func() {
			fetchCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			var fresh []byte
			err := errors.New("release feed check interrupted")
			defer func() {
				s.updateMu.Lock()
				entry.data, entry.err, entry.checked, entry.inFlight = fresh, err, s.now(), false
				s.updateMu.Unlock()
			}()
			fresh, err = fetch(fetchCtx, feedURL)
		})
	}
	s.updateMu.Unlock()
	if checked.IsZero() {
		out := s.updatesFrom(ctx, current, channel, feedURL, nil)
		out.Message = "The release feed is being checked in the background."
		out.CheckedAt = ""
		return out
	}
	out := s.updatesFrom(ctx, current, channel, feedURL, func(context.Context, string) ([]byte, error) { return data, priorErr })
	out.CheckedAt = checked.UTC().Format(time.RFC3339)
	return out
}
