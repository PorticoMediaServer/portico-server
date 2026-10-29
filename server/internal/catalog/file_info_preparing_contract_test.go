package catalog

import (
	"encoding/json"
	"testing"
)

func TestFileInfoPreparingEchoesDetailOnlyAfterOpening(t *testing.T) {
	original := FileInfoDetailCapture{Scope: DetailScope{"server", "library", "item", "original"}, Revision: ContentRevision{Catalog: 41, Viewer: 7}}
	for _, capture := range []*FileInfoDetailCapture{nil, &original} {
		raw, e := json.Marshal(FileInfoPreparing{Kind: "file-info-preparing", State: "preparing", QueryID: "private", RetryAfterMs: 1000, Detail: capture})
		if e != nil {
			t.Fatal(e)
		}
		var wire map[string]json.RawMessage
		if e = json.Unmarshal(raw, &wire); e != nil {
			t.Fatal(e)
		}
		detail, exists := wire["detail"]
		if capture == nil {
			if exists {
				t.Fatal("opening preparation invented accepted Detail", string(raw))
			}
			continue
		}
		if !exists {
			t.Fatal("lane preparation lost original Detail")
		}
		var got FileInfoDetailCapture
		if e = json.Unmarshal(detail, &got); e != nil || got != original {
			t.Fatal(got, e)
		}
	}
}
