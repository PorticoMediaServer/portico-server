// Package searchcursor keeps private traversal anchors out of public cursors.
package searchcursor

import (
	"container/list"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

const maxEntries = 256
const maxPayload = 64 << 10

var ErrCursor = errors.New("search continuation expired or unavailable")

type entry struct {
	token, namespace string
	payload          []byte
	expires          time.Time
}

// Cache is a bounded process-local LRU. Eviction/restart requires an explicit
// restart, never a fallback to a caller-supplied position. Its zero value works.
type Cache struct {
	mu      sync.Mutex
	key     []byte
	entries map[string]*list.Element
	lru     list.List
}

func (c *Cache) Put(namespace string, value any, expires time.Time) (string, error) {
	payload, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(payload) > maxPayload || !expires.After(time.Now()) {
		return "", ErrCursor
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.key == nil {
		c.key = make([]byte, 32)
		if _, err = rand.Read(c.key); err != nil {
			c.key = nil
			return "", err
		}
		c.entries = map[string]*list.Element{}
	}
	mac := hmac.New(sha256.New, c.key)
	mac.Write([]byte(namespace))
	mac.Write([]byte{0})
	mac.Write(payload)
	token := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if el := c.entries[token]; el != nil {
		c.lru.MoveToFront(el)
		return token, nil
	}
	el := c.lru.PushFront(entry{token, namespace, payload, expires})
	c.entries[token] = el
	for c.lru.Len() > maxEntries {
		last := c.lru.Back()
		delete(c.entries, last.Value.(entry).token)
		c.lru.Remove(last)
	}
	return token, nil
}
func (c *Cache) Get(token, namespace string, value any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	el := c.entries[token]
	if el == nil {
		return ErrCursor
	}
	e := el.Value.(entry)
	if e.namespace != namespace {
		return ErrCursor
	}
	if !e.expires.After(time.Now()) {
		delete(c.entries, token)
		c.lru.Remove(el)
		return ErrCursor
	}
	c.lru.MoveToFront(el)
	return json.Unmarshal(e.payload, value)
}
