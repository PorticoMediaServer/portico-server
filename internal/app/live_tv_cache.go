package app

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

const (
	liveTVSegmentCacheMaxBytes     int64 = 96 << 20
	liveTVSegmentCacheMaxItemBytes int64 = 12 << 20
	liveTVSegmentCacheTTL                = 2 * time.Minute
)

var liveTVSegments = newLiveTVSegmentCache(liveTVSegmentCacheMaxBytes, liveTVSegmentCacheTTL)

type liveTVSegmentCache struct {
	mu       sync.Mutex
	items    map[string]liveTVSegmentCacheItem
	lru      *list.List
	flights  map[string]*liveTVSegmentFlight
	maxBytes int64
	bytes    int64
	ttl      time.Duration
}

type liveTVSegmentCacheItem struct {
	status    int
	header    http.Header
	body      []byte
	size      int64
	createdAt time.Time
	expiresAt time.Time
	element   *list.Element
}

type liveTVSegmentFlight struct {
	done chan struct{}
}

// liveTVHLSItemBinding is the server-side half of the opaque HLS item
// capability. The URL alone is never authority: redemption remains bound to
// the channel and to the exact provider authority approved for its manifest.
type liveTVHLSItemBinding struct {
	channelID string
	approval  liveTVEndpointApproval
}

func newLiveTVSegmentCache(maxBytes int64, ttl time.Duration) *liveTVSegmentCache {
	return &liveTVSegmentCache{
		items:    map[string]liveTVSegmentCacheItem{},
		lru:      list.New(),
		flights:  map[string]*liveTVSegmentFlight{},
		maxBytes: maxBytes,
		ttl:      ttl,
	}
}

func (cache *liveTVSegmentCache) get(key string) (liveTVSegmentCacheItem, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	item, ok := cache.items[key]
	if !ok {
		return liveTVSegmentCacheItem{}, false
	}
	if time.Now().After(item.expiresAt) {
		cache.removeLocked(key, item)
		return liveTVSegmentCacheItem{}, false
	}
	if item.element != nil {
		cache.lru.MoveToFront(item.element)
	}
	return item, true
}

func (cache *liveTVSegmentCache) set(key string, status int, header http.Header, body []byte) {
	if int64(len(body)) > liveTVSegmentCacheMaxItemBytes {
		return
	}
	now := time.Now()
	item := liveTVSegmentCacheItem{
		status: status,
		header: header.Clone(),
		// The caller has just created body and never mutates it after publication.
		// Retaining it directly avoids a second segment-sized allocation.
		body:      body,
		size:      int64(len(body)),
		createdAt: now,
		expiresAt: now.Add(cache.ttl),
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if existing, ok := cache.items[key]; ok {
		cache.removeLocked(key, existing)
	}
	item.element = cache.lru.PushFront(key)
	cache.items[key] = item
	cache.bytes += item.size
	cache.evictLocked(now)
}

func (cache *liveTVSegmentCache) beginFlight(key string) (*liveTVSegmentFlight, bool) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if flight := cache.flights[key]; flight != nil {
		return flight, false
	}
	flight := &liveTVSegmentFlight{done: make(chan struct{})}
	cache.flights[key] = flight
	return flight, true
}

func (cache *liveTVSegmentCache) finishFlight(key string, flight *liveTVSegmentFlight) {
	cache.mu.Lock()
	if cache.flights[key] == flight {
		delete(cache.flights, key)
		close(flight.done)
	}
	cache.mu.Unlock()
}

func (cache *liveTVSegmentCache) removeLocked(key string, item liveTVSegmentCacheItem) {
	delete(cache.items, key)
	cache.bytes -= item.size
	if item.element != nil {
		cache.lru.Remove(item.element)
	}
}

func (cache *liveTVSegmentCache) evictLocked(now time.Time) {
	for key, item := range cache.items {
		if now.After(item.expiresAt) {
			cache.removeLocked(key, item)
		}
	}
	for cache.bytes > cache.maxBytes {
		oldest := cache.lru.Back()
		if oldest == nil {
			return
		}
		oldestKey, _ := oldest.Value.(string)
		item, ok := cache.items[oldestKey]
		if !ok {
			cache.lru.Remove(oldest)
			continue
		}
		cache.removeLocked(oldestKey, item)
	}
}

func liveTVSegmentCacheKey(binding liveTVHLSItemBinding, upstreamURL string) string {
	identity := binding.channelID + "\x00" + binding.approval.Scheme + "\x00" + binding.approval.Host + "\x00" + binding.approval.Port + "\x00" + upstreamURL
	sum := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(sum[:])
}

func proxyCachedLiveTVHLSItem(w http.ResponseWriter, r *http.Request, binding liveTVHLSItemBinding, upstreamURL string, userAgent string) {
	parsed, err := binding.approval.validateURL(upstreamURL)
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsafe_stream_url", "The provider stream URL is not allowed.")
		return
	}
	cacheKey := liveTVSegmentCacheKey(binding, parsed.String())
	var flight *liveTVSegmentFlight
	if r.Method == http.MethodGet && r.Header.Get("Range") == "" {
		if item, ok := liveTVSegments.get(cacheKey); ok {
			copyLiveTVHeaders(w.Header(), item.header)
			w.Header().Set("Cache-Control", "private, max-age=30")
			w.Header().Set("X-Portico-Stream-Cache", "hit")
			w.WriteHeader(item.status)
			_, _ = w.Write(item.body)
			return
		}
		var leader bool
		flight, leader = liveTVSegments.beginFlight(cacheKey)
		if !leader {
			select {
			case <-flight.done:
				if item, ok := liveTVSegments.get(cacheKey); ok {
					copyLiveTVHeaders(w.Header(), item.header)
					w.Header().Set("Cache-Control", "private, max-age=30")
					w.Header().Set("X-Portico-Stream-Cache", "coalesced")
					w.WriteHeader(item.status)
					_, _ = w.Write(item.body)
					return
				}
				writeError(w, http.StatusBadGateway, "stream_unavailable", "The provider stream is not responding yet. Try Restart Stream.")
				return
			case <-r.Context().Done():
				return
			}
		}
		defer liveTVSegments.finishFlight(cacheKey, flight)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, parsed.String(), nil)
	if err != nil {
		writeError(w, http.StatusBadGateway, "stream_request_failed", "Unable to prepare the provider stream request.")
		return
	}
	req.Header.Set("User-Agent", effectiveLiveTVUserAgent(userAgent))
	req.Header.Set("Accept", "*/*")
	if rangeHeader := r.Header.Get("Range"); rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}
	client := liveTVHTTPClientForContext(r.Context())
	if _, injected := r.Context().Value(liveTVHTTPClientContextKey{}).(*http.Client); !injected {
		client, err = newApprovedLiveTVHTTPClient(r.Context(), binding.approval, nil)
		if err != nil {
			writeError(w, http.StatusBadGateway, "stream_unavailable", "The provider stream is not responding yet. Try Restart Stream.")
			return
		}
	}
	resp, err := client.Do(req)
	if err != nil {
		writeError(w, http.StatusBadGateway, "stream_unavailable", "The provider stream is not responding yet. Try Restart Stream.")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeError(w, http.StatusBadGateway, "stream_unavailable", "The provider stream is not responding yet. Try Restart Stream.")
		return
	}
	copyLiveTVHeaders(w.Header(), resp.Header)
	w.Header().Set("Cache-Control", "private, max-age=30")
	if r.Method == http.MethodHead {
		w.Header().Set("X-Portico-Stream-Cache", "pass")
		w.WriteHeader(resp.StatusCode)
		return
	}
	if r.Method == http.MethodGet && r.Header.Get("Range") == "" && resp.ContentLength <= liveTVSegmentCacheMaxItemBytes {
		body, err := io.ReadAll(io.LimitReader(resp.Body, liveTVSegmentCacheMaxItemBytes+1))
		if err == nil && int64(len(body)) <= liveTVSegmentCacheMaxItemBytes {
			liveTVSegments.set(cacheKey, resp.StatusCode, resp.Header, body)
			w.Header().Set("Content-Length", strconv.Itoa(len(body)))
			w.Header().Set("X-Portico-Stream-Cache", "miss")
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(body)
			return
		}
		if err == nil {
			w.Header().Del("Content-Length")
			w.Header().Set("X-Portico-Stream-Cache", "pass")
			w.WriteHeader(resp.StatusCode)
			_, _ = w.Write(body)
			_, _ = io.Copy(w, resp.Body)
			return
		}
	}
	w.Header().Set("X-Portico-Stream-Cache", "pass")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
