package remotemedia

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestServeListingSnapshotNeverReturnsPartialChildren(t *testing.T) {
	child := `<d:response><d:href>/disc/clip.m2ts</d:href><d:propstat><d:prop><d:getcontentlength>8</d:getcontentlength><d:getetag>"etag"</d:getetag><d:getlastmodified>Fri, 23 Oct 2026 00:00:00 GMT</d:getlastmodified></d:prop><d:status>HTTP/1.1 200 OK</d:status></d:propstat></d:response>`
	body := `<d:multistatus xmlns:d="DAV:">` + child + `</d:multistatus>`
	for _, bad := range []string{"", "truncated", "oversized", "duplicate", "traversal"} {
		t.Run(bad, func(t *testing.T) {
			s, err := NewServeEndpoint(12345, "private", "secret", 2)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			wire := body
			switch bad {
			case "truncated":
				wire = strings.TrimSuffix(body, `</d:multistatus>`)
			case "oversized":
				wire = `<d:multistatus xmlns:d="DAV:">` + strings.Repeat(" ", 4<<20) + `</d:multistatus>`
			case "duplicate":
				wire = `<d:multistatus xmlns:d="DAV:">` + child + child + `</d:multistatus>`
			case "traversal":
				wire = strings.ReplaceAll(body, "/disc/clip.m2ts", "/disc/../outside")
			}
			s.client.Transport = serveRoundTrip(func(r *http.Request) (*http.Response, error) {
				if r.Method != "PROPFIND" || r.URL.Path != "/disc/" || r.Header.Get("Depth") != "1" {
					t.Fatal("wrong listing request")
				}
				request, _ := io.ReadAll(r.Body)
				if strings.Contains(string(request), "allprop") || strings.Contains(string(request), "getcontenttype") {
					t.Fatal("expensive listing properties")
				}
				return &http.Response{StatusCode: 207, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(wire))}, nil
			})
			page, err := s.ListSnapshot(context.Background(), "disc", func(context.Context) error { return nil })
			if bad == "" && (err != nil || len(page.Entries) != 1) {
				t.Fatalf("%+v %v", page, err)
			}
			if bad != "" && (err == nil || len(page.Entries) != 0) {
				t.Fatalf("partial snapshot escaped: %+v %v", page, err)
			}
		})
	}
}
