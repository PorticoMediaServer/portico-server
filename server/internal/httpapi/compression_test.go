package httpapi

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http/httptest"
	"portico.local/server/internal/httpapi/fixture"
	"strings"
	"testing"
	"time"
)

// A home document is hundreds of kilobytes of JSON on the wire and about a
// seventh of that compressed. On the networks this server is expected to serve
// over, that difference is whether the page arrives.
func TestCompositeResponsesCompressAndMediaDoesNot(t *testing.T) {
	tier := performanceTier{name: "gzip", shape: fixture.Tiny(), concurrentViewers: 1, iterations: 1, maximumP95: time.Minute, maximumP99: time.Minute, allowBoundedOverload: true}
	f := newLoadFixture(t, tier)
	token := f.viewers[0].AccessToken

	ask := func(path, encoding string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, bytes.NewReader(nil))
		r.Header.Set("Authorization", "Bearer "+token)
		if encoding != "" {
			r.Header.Set("Accept-Encoding", encoding)
		}
		w := httptest.NewRecorder()
		f.handler.ServeHTTP(w, r)
		return w
	}

	plain := ask("/v1/home", "")
	if plain.Code != 200 {
		t.Fatalf("home %d %s", plain.Code, plain.Body.String())
	}
	if plain.Header().Get("Content-Encoding") != "" {
		t.Fatal("a client that did not ask for gzip was sent gzip")
	}
	zipped := ask("/v1/home", "gzip")
	if zipped.Code != 200 {
		t.Fatalf("home %d", zipped.Code)
	}
	if zipped.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("home was not compressed for a client that asked (%d bytes)", plain.Body.Len())
	}
	if zipped.Body.Len() >= plain.Body.Len() {
		t.Fatalf("compression made the body larger: %d against %d", zipped.Body.Len(), plain.Body.Len())
	}
	t.Logf("GET /v1/home %d bytes uncompressed, %d gzipped (%.0f%%)", plain.Body.Len(), zipped.Body.Len(), 100*float64(zipped.Body.Len())/float64(plain.Body.Len()))

	// The bytes must survive the round trip, not merely be smaller.
	reader, err := gzip.NewReader(bytes.NewReader(zipped.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	// Separately composed responses have fresh timestamps and cursor expiry
	// signatures. Compare cursor payloads too, excluding only their expiry;
	// membership, ranking fingerprints and paging positions must still agree.
	strip := func(raw []byte) string {
		var document map[string]any
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatalf("not the JSON document: %v", err)
		}
		delete(document, "generatedAt")
		var scrub func(any)
		scrub = func(v any) {
			switch x := v.(type) {
			case map[string]any:
				for key, value := range x {
					if key == "nextCursor" {
						if raw, ok := value.(string); ok && raw != "" {
							payload, _, ok := strings.Cut(raw, ".")
							if !ok {
								t.Fatal("unsigned home cursor")
							}
							data, err := base64.RawURLEncoding.DecodeString(payload)
							if err != nil {
								t.Fatal(err)
							}
							var cursor map[string]any
							if err = json.Unmarshal(data, &cursor); err != nil {
								t.Fatal(err)
							}
							delete(cursor, "expires")
							x[key] = cursor
						}
					} else {
						scrub(value)
					}
				}
			case []any:
				for _, value := range x {
					scrub(value)
				}
			}
		}
		scrub(document)
		normalised, _ := json.Marshal(document)
		return string(normalised)
	}
	if strip(decoded) != strip(plain.Body.Bytes()) {
		t.Fatal("the compressed body decoded to something other than the plain one")
	}

	// A 304 has no body, and its headers must not describe one.
	tag := plain.Header().Get("ETag")
	if tag == "" {
		t.Fatal("home carried no ETag")
	}
	r := httptest.NewRequest("GET", "/v1/home", bytes.NewReader(nil))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("Accept-Encoding", "gzip")
	r.Header.Set("If-None-Match", tag)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 304 {
		t.Fatalf("a matching If-None-Match on home answered %d", w.Code)
	}
	if w.Body.Len() != 0 || w.Header().Get("Content-Encoding") != "" {
		t.Fatalf("the 304 carried %d bytes and Content-Encoding %q", w.Body.Len(), w.Header().Get("Content-Encoding"))
	}
}
