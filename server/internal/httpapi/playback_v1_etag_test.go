package httpapi

import (
	"context"
	"testing"
	"time"

	"portico.local/server/internal/playback"
	"portico.local/server/internal/playbackv1"
)

func TestPlaybackV1SessionConditionalGetIncludesLeaseAndObservedOutput(t *testing.T) {
	f := newV1Fixture(t, 1)
	f.call("PUT", "/v1/me/devices/current/capabilities", nil, webCapabilities(), 204, nil)
	var started playbackv1.SessionView
	f.call("POST", "/v1/playback/sessions", map[string]string{"Idempotency-Key": "session-etag-start-0001"}, startBody(f.items[0], nil), 201, &started)
	path := "/v1/playback/sessions/" + started.ID
	var before playbackv1.SessionView
	w := f.call("GET", path, nil, nil, 200, &before)
	tag := w.Header().Get("ETag")
	if tag == "" || tag == `"`+before.Revision+`"` {
		t.Fatalf("GET used the command revision as its validator: %q", tag)
	}
	f.call("GET", path, map[string]string{"If-None-Match": tag}, nil, 304, nil)

	// A same-state report changes the lease and renderer facts without
	// invalidating a controller's command revision.
	now := time.Now().Add(10 * time.Second)
	f.v1.Now = func() time.Time { return now }
	p, err := f.id.Authenticate(f.owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	actual := &playback.ObservedPlayback{Video: &playback.ObservedVideo{Width: 1280, Height: 720}}
	if _, err = f.v1.Timeline(context.Background(), playbackv1.Caller{Principal: p, DeviceID: p.DeviceID}, started.ID, playbackv1.Report{
		Seq: 1, Generation: before.Presentation.Generation, State: "playing", PositionMs: 5000, ObservedPlayback: actual,
	}); err != nil {
		t.Fatal(err)
	}
	var after playbackv1.SessionView
	w = f.call("GET", path, map[string]string{"If-None-Match": tag}, nil, 200, &after)
	if after.Revision != before.Revision || after.Lease.ExpiresAt == before.Lease.ExpiresAt || after.ObservedPlayback == nil || after.ObservedPlayback.Video == nil || *after.ObservedPlayback.Video != *actual.Video {
		t.Fatalf("same-revision GET lost fresh lease/output: before=%+v after=%+v", before, after)
	}
	updatedTag := w.Header().Get("ETag")
	if updatedTag == "" || updatedTag == tag {
		t.Fatalf("changed representation reused ETag %q", updatedTag)
	}
	f.call("GET", path, map[string]string{"If-None-Match": updatedTag}, nil, 304, nil)

	// The body's revision, independently of the GET ETag, still admits PATCH.
	f.call("PATCH", path, map[string]string{"If-Match": `"` + after.Revision + `"`}, map[string]any{"state": "paused"}, 200, nil)
}
