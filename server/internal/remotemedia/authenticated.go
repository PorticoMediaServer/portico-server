package remotemedia

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// NewAuthenticated is restricted to one configured DAV origin/root. Secrets are
// headers only and are stripped/reapplied only after a same-origin/root redirect
// has passed the existing DNS/IP policy. No caller can supply an arbitrary proxy.
func NewAuthenticated(ctx context.Context, locator string, policy Policy, username, password string) (*Client, error) {
	c, err := New(ctx, locator, policy)
	if err != nil {
		return nil, err
	}
	root, err := url.Parse(locator)
	if err != nil {
		c.Close()
		return nil, ErrPolicy
	}
	if len(username) > 1024 || len(password) > 8192 {
		c.Close()
		return nil, ErrPolicy
	}
	auth := ""
	if username != "" || password != "" {
		auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
	}
	c.authorization = auth
	check := c.client.CheckRedirect
	c.client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if origin(r.URL) != origin(root) {
			return ErrPolicy
		}
		if err := check(r, via); err != nil {
			return err
		}
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		return nil
	}
	return c, nil
}

// Request allows only read-only DAV/media operations. It returns no URL-bearing
// transport errors and retries only before a body is handed to its caller.
func (c *Client) Request(ctx context.Context, method string, body []byte, headers http.Header) (*http.Response, error) {
	switch method {
	case "GET", "HEAD", "PROPFIND", "REPORT", "OPTIONS":
	default:
		return nil, ErrPolicy
	}
	if len(body) > 16<<10 {
		return nil, ErrPolicy
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.url, bytes.NewReader(body))
		if err != nil {
			return nil, ErrPolicy
		}
		req.Header.Set("Accept-Encoding", "identity")
		req.Header.Set("User-Agent", "Portico/1")
		if c.authorization != "" {
			req.Header.Set("Authorization", c.authorization)
		}
		for key, values := range headers {
			switch http.CanonicalHeaderKey(key) {
			case "Depth", "Content-Type", "Range", "If-Match", "If-None-Match":
			default:
				return nil, ErrPolicy
			}
			if len(values) != 1 || len(values[0]) > 8192 {
				return nil, ErrPolicy
			}
			req.Header.Set(key, values[0])
		}
		resp, err := c.client.Do(req)
		if err != nil {
			return nil, boundRequestError(ctx, err)
		}
		transient := resp.StatusCode == 429 || resp.StatusCode == 502 || resp.StatusCode == 503 || resp.StatusCode == 504
		if !transient || attempt == 2 {
			return resp, nil
		}
		delay := time.Duration(attempt+1) * 200 * time.Millisecond
		if n, e := strconv.Atoi(resp.Header.Get("Retry-After")); e == nil && n > 0 {
			delay = min(time.Duration(n)*time.Second, 2*time.Second)
		}
		resp.Body.Close()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, ErrUnavailable
}

// Credentialed version discovery shares all range/ETag checks with STRM. The
// binding is exact-root/object authority, not equivalence guessed from an ETag.
func DiscoverAuthenticatedVersion(ctx context.Context, locator string, policy Policy, username, password, scope, object string, binding LocatorBinding) (*VersionedClient, error) {
	base, err := NewAuthenticated(ctx, locator, policy, username, password)
	if err != nil {
		return nil, err
	}
	if err = bindLocator(ctx, locator, scope, object, binding); err != nil {
		base.Close()
		return nil, err
	}
	check := base.client.CheckRedirect
	base.client.CheckRedirect = func(r *http.Request, via []*http.Request) error {
		if err := check(r, via); err != nil {
			return err
		}
		return bindLocator(r.Context(), r.URL.String(), scope, object, binding)
	}
	return discoverVersion(ctx, base, scope, object, binding)
}

func safeResponseError(resp *http.Response) error {
	switch resp.StatusCode {
	case 401, 403:
		return ErrDenied
	case 429:
		return ErrBusy
	default:
		return ErrUnavailable
	}
}
