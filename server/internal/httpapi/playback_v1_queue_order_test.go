package httpapi

import (
	"portico.local/server/internal/playbackv1"
	"testing"
)

func TestShuffledQueueEditsKeepPlayedOrder(t *testing.T) {
	f := newV1Fixture(t, 6)
	var q playbackv1.QueueReply
	f.call("POST", "/v1/queues", map[string]string{"Idempotency-Key": "shuffle-edit-key-0001"}, map[string]any{"segments": []any{map[string]any{"source": queueItems(f.items...)}}}, 201, &q)
	base := "/v1/queues/" + q.Queue.ID
	call := func(method, path string, body any) {
		t.Helper()
		f.call(method, path, map[string]string{"If-Match": q.Queue.Revision}, body, 200, &q)
	}
	call("PATCH", base, map[string]any{"shuffle": map[string]any{"on": true, "seed": "42"}})
	call("POST", base+":advance", map[string]any{"reason": "next"})
	var before playbackv1.EntryPage
	f.call("GET", base+"/entries?from=0&limit=20", nil, nil, 200, &before)
	current := q.Queue.Current.EntryID
	call("POST", base+"/segments", map[string]any{"placement": "next", "source": queueItems(f.items[5])})
	var after playbackv1.EntryPage
	f.call("GET", base+"/entries?from=0&limit=20", nil, nil, 200, &after)
	if len(after.Items) != 7 || after.Items[2].ItemID != f.items[5] || q.Queue.Current.EntryID != current || q.Queue.Current.Position != 1 {
		t.Fatalf("next %+v %+v", q.Queue, after.Items)
	}
	for i := 3; i < len(after.Items); i++ {
		if after.Items[i].EntryID != before.Items[i-1].EntryID {
			t.Fatalf("order changed at %d", i)
		}
	}
	removed := after.Items[4].EntryID
	call("DELETE", base+"/entries/"+removed, nil)
	var fewer playbackv1.EntryPage
	f.call("GET", base+"/entries?from=0&limit=20", nil, nil, 200, &fewer)
	if len(fewer.Items) != 6 || q.Queue.Current.Position != 1 || fewer.Items[0].EntryID != before.Items[0].EntryID {
		t.Fatalf("remove %+v", fewer.Items)
	}
	moved := fewer.Items[5].EntryID
	call("POST", base+"/entries/"+moved+":move", map[string]any{"before": fewer.Items[2].EntryID})
	f.call("GET", base+"/entries?from=0&limit=20", nil, nil, 200, &after)
	if after.Items[2].EntryID != moved || q.Queue.Current.Position != 1 {
		t.Fatalf("move %+v", after.Items)
	}
	call("PATCH", base, map[string]any{"shuffle": map[string]any{"on": false}})
	if q.Queue.Total != 6 || q.Queue.Current.EntryID != current {
		t.Fatalf("unshuffle %+v", q.Queue)
	}
}
