package searchcursor

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPrivateCursorBoundsReplayAndExpiry(t *testing.T) {
	var c Cache
	value := map[string]string{"privateTitle": "Secret programme", "privateID": "raw-hidden-id"}
	expiry := time.Now().Add(time.Minute)
	token, err := c.Put("guide", value, expiry)
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.Put("guide", value, expiry)
	if err != nil || again != token {
		t.Fatal("nonidentical replay", err)
	}
	if strings.Contains(token, "Secret") || len(token) != 43 {
		t.Fatal("revealing token", token)
	}
	var out map[string]string
	if c.Get(token, "other", &out) == nil {
		t.Fatal("cross namespace")
	}
	if err = c.Get(token, "guide", &out); err != nil || out["privateID"] != value["privateID"] {
		t.Fatal(out, err)
	}
	out["privateID"] = "changed"
	if c.Get(token, "guide", &out) != nil || out["privateID"] != value["privateID"] {
		t.Fatal("shared mutable value")
	}
	for i := 0; i < maxEntries; i++ {
		if _, err = c.Put("guide", fmt.Sprint(i), expiry); err != nil {
			t.Fatal(err)
		}
	}
	if c.Get(token, "guide", &out) == nil {
		t.Fatal("unbounded cache")
	}
	c.mu.Lock()
	for _, el := range c.entries {
		e := el.Value.(entry)
		e.expires = time.Now().Add(-time.Second)
		el.Value = e
	}
	c.mu.Unlock()
	tok, err := c.Put("guide", "fresh", expiry)
	if err != nil {
		t.Fatal(err)
	}
	c.mu.Lock()
	e := c.entries[tok].Value.(entry)
	e.expires = time.Now().Add(-time.Second)
	c.entries[tok].Value = e
	c.mu.Unlock()
	if c.Get(tok, "guide", &out) == nil {
		t.Fatal("expired reference")
	}
	var restarted Cache
	if restarted.Get(tok, "guide", &out) == nil {
		t.Fatal("restart fallback")
	}
}
