package linearinput

import (
	"context"
	"io"
	"time"
)

type cachedResource struct {
	ready                  chan struct{}
	data                   []byte
	location, contentRange string
	expires                time.Time
	err                    error
}

// Acquisition is shared by probe, viewer and capture readers. Retention is
// bounded in both bytes and entries; each reader still has a private route.
func (g *Gateway) resourceBytes(ctx context.Context, res resource, rng string) (*cachedResource, error) {
	key := res.kind + "\n" + res.url + "\n" + rng
	g.mu.Lock()
	now := time.Now()
	for k, v := range g.cache {
		if !v.expires.IsZero() && !v.expires.After(now) {
			g.cacheBytes -= int64(len(v.data))
			delete(g.cache, k)
		}
	}
	entry := g.cache[key]
	if entry != nil {
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-entry.ready:
			return entry, entry.err
		}
	}
	if len(g.cache) >= 512 {
		g.mu.Unlock()
		return nil, ErrUnavailable
	}
	entry = &cachedResource{ready: make(chan struct{})}
	g.cache[key] = entry
	g.mu.Unlock()
	select {
	case g.resourceSlots <- struct{}{}:
		defer func() { <-g.resourceSlots }()
	case <-g.ctx.Done():
		g.mu.Lock()
		entry.err = ErrUnavailable
		delete(g.cache, key)
		close(entry.ready)
		g.mu.Unlock()
		return nil, ErrUnavailable
	}
	work, cancel := context.WithTimeout(g.ctx, 30*time.Second)
	defer cancel()
	limit := int64(maxSegment)
	if res.kind == "manifest" {
		limit = maxManifest
	}
	if res.kind == "key" {
		limit = 16
	}
	client, resp, e := g.fetch(work, res.url, rng)
	if e == nil {
		entry.data, e = io.ReadAll(io.LimitReader(resp.Body, limit+1))
		resp.Body.Close()
		client.Close()
		g.untrack(client, resp.Body)
		entry.location = res.url
		if resp.Request != nil && resp.Request.URL != nil {
			entry.location = resp.Request.URL.String()
		}
		if resp.StatusCode == 206 && res.kind == "media" {
			entry.contentRange = resp.Header.Get("Content-Range")
		}
		if int64(len(entry.data)) > limit {
			e = ErrFormat
		}
	}
	g.mu.Lock()
	for e == nil && g.cacheBytes+int64(len(entry.data)) > maxSegment {
		oldest := ""
		var until time.Time
		for key, value := range g.cache {
			if value == entry || value.expires.IsZero() {
				continue
			}
			if oldest == "" || value.expires.Before(until) {
				oldest, until = key, value.expires
			}
		}
		if oldest == "" {
			e = ErrUnavailable
			break
		}
		g.cacheBytes -= int64(len(g.cache[oldest].data))
		delete(g.cache, oldest)
	}
	if e != nil {
		entry.data = nil
		delete(g.cache, key)
	} else {
		g.cacheBytes += int64(len(entry.data))
	}
	entry.err = e
	ttl := time.Minute
	if res.kind == "manifest" {
		ttl = 500 * time.Millisecond
	}
	entry.expires = time.Now().Add(ttl)
	close(entry.ready)
	g.mu.Unlock()
	return entry, e
}
