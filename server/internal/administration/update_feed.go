package administration

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
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

// FetchReleaseFeed reads a bounded owner-configured feed. Custom feeds stay
// within their configured origin. Only the official Portico GitHub manifest
// may cross to GitHub's HTTPS release asset host; every hop is independently
// admitted by the remote-media policy (including public-only asset DNS).
func FetchReleaseFeed(ctx context.Context, rawURL string) ([]byte, error) {
	return fetchReleaseFeed(ctx, rawURL, remotemedia.Policy{ReadTimeout: 8 * time.Second})
}

func fetchReleaseFeed(ctx context.Context, rawURL string, policy remotemedia.Policy) ([]byte, error) {
	if !safeFeedURL(rawURL) {
		return nil, ErrInput
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	initial, err := url.Parse(rawURL)
	if err != nil {
		return nil, ErrInput
	}
	policy.ManualRedirects = true
	// An owner may configure a private feed. Pin only that initial origin;
	// never grant private-network approval to a redirected GitHub asset host.
	if !officialGitHubFeed(initial) {
		approval, err := policy.Approve(ctx, initial.Scheme+"://"+initial.Host+"/")
		if err != nil {
			return nil, err
		}
		policy.Approvals = []remotemedia.Approval{approval}
	}
	current := initial
	for hop := 0; hop < 5; hop++ {
		client, err := remotemedia.New(ctx, current.String(), policy)
		if err != nil {
			return nil, err
		}
		response, err := client.Open(ctx, http.MethodGet, "")
		if err != nil {
			client.Close()
			return nil, err
		}
		if response.StatusCode == http.StatusOK {
			data, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
			response.Body.Close()
			client.Close()
			if err != nil {
				return nil, err
			}
			if len(data) > 1<<20 {
				return nil, errors.New("release feed exceeds byte limit")
			}
			return data, nil
		}
		next, err := response.Location()
		response.Body.Close()
		client.Close()
		if err != nil || !feedRedirectAllowed(initial, current, next) {
			return nil, remotemedia.ErrPolicy
		}
		current = next
	}
	return nil, remotemedia.ErrPolicy
}

var githubFeedPath = regexp.MustCompile(`^/PorticoMediaServer/portico-server/releases/(latest/download|download/v[0-9][0-9A-Za-z.+-]*)/portico-update-manifest\.json$`)
var githubAssetPath = regexp.MustCompile(`^/github-production-release-asset/[0-9]+/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

func httpsFeedHost(u *url.URL, host string) bool {
	return u != nil && u.Scheme == "https" && strings.EqualFold(u.Hostname(), host) &&
		(u.Port() == "" || u.Port() == "443") && u.User == nil && u.Fragment == "" && u.RawPath == ""
}

func officialGitHubFeed(u *url.URL) bool {
	return httpsFeedHost(u, "github.com") && u.RawQuery == "" && githubFeedPath.MatchString(u.Path)
}

func feedOrigin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return u.Scheme + "://" + strings.ToLower(u.Hostname()) + ":" + port
}

func feedRedirectAllowed(initial, current, next *url.URL) bool {
	if next == nil || !safeFeedURL(next.String()) || next.Fragment != "" ||
		(current.Scheme == "https" && next.Scheme != "https") {
		return false
	}
	if officialGitHubFeed(initial) {
		// GitHub's latest URL first resolves to a tagged asset in this repo,
		// then to its signed CDN URL. The CDN cannot redirect onward.
		return officialGitHubFeed(current) && (officialGitHubFeed(next) ||
			(httpsFeedHost(next, "release-assets.githubusercontent.com") && githubAssetPath.MatchString(next.Path)))
	}
	return feedOrigin(initial) == feedOrigin(next)
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
