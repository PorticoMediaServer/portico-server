package httpapi

import (
	"fmt"
	"portico.local/server/internal/playbackv1"
	"reflect"
	"testing"
	"time"
)

func querySource(library string, sort string) map[string]any {
	return map[string]any{"query": map[string]any{"libraryId": library, "pivot": "movies", "sort": []any{map[string]any{"field": "title", "direction": sort}}}}
}

func TestPlaybackV1QueueQueryCapturesBeyond500AndKeepsItsSnapshot(t *testing.T) {
	f := newV1Fixture(t, 520)
	// Hold the background build: the request answers with its first batch, and
	// the rest is built only after the catalogue changes below.
	f.v1.HoldQueueBuilds()
	var q playbackv1.QueueReply
	f.call("POST", "/v1/queues", map[string]string{"Idempotency-Key": "whole-query-000001"}, map[string]any{"selector": querySource(f.library, "desc"), "startPlayback": map[string]any{"state": "playing"}}, 201, &q)
	if q.Queue.Total != 256 || q.Session == nil || q.Session.ItemID != f.items[519] || len(q.Queue.Segments) != 1 || q.Queue.Segments[0].SourceRevision == "" {
		t.Fatalf("first batch: %+v", q)
	}
	// Move an uncaptured title and add another title while the retained reader is
	// open. Neither mutation may change this already accepted queue membership.
	f.catalogTest.Fields(f.records[0].ID, map[string]any{"title": "ZZZ changed"})
	f.catalogTest.Movie(f.libraryHandle, f.root+"/later.mp4", "Later", 2026)
	f.catalogTest.Drain()
	f.v1.ResumeQueueBuilds()
	done := waitBuilt(t, f, f.owner.AccessToken, q.Queue.ID, 30*time.Second)
	if done.Total != 520 {
		t.Fatalf("total %d", done.Total)
	}
	got := queueItemsInOrder(t, f, f.owner.AccessToken, q.Queue.ID, done.Total)
	for i := range got {
		if got[i] != f.items[len(f.items)-1-i] {
			t.Fatalf("position %d: %s", i, got[i])
		}
	}
}

func TestPlaybackV1QueueQueryUsesBrowseRestrictionsAndFilters(t *testing.T) {
	f, member, ids := queueAccessFixture(t)
	var q playbackv1.QueueReply
	var library string
	if err := f.db.QueryRow(`SELECT l.library_id FROM catalog_entities e JOIN catalog_libraries l ON l.id=e.library_id WHERE e.public_id=pid_blob(?)`, ids["film"]).Scan(&library); err != nil {
		t.Fatal(err)
	}
	source := querySource(library, "asc")
	f.callAs(member, "POST", "/v1/queues", map[string]string{"Idempotency-Key": "query-restricted-0001"}, map[string]any{"selector": source}, 201, &q)
	got := queueItemsInOrder(t, f, member, q.Queue.ID, q.Queue.Total)
	if !reflect.DeepEqual(got, []string{ids["film"]}) {
		t.Fatalf("restricted membership: %v", got)
	}
	source["query"].(map[string]any)["query"] = map[string]any{"field": "title", "operator": "equals", "value": "Grown-up film"}
	f.callAs(member, "POST", "/v1/queues", map[string]string{"Idempotency-Key": "query-restricted-0002"}, map[string]any{"selector": source}, 404, nil)
	source["query"].(map[string]any)["libraryId"] = f.library
	f.callAs(member, "POST", "/v1/queues", map[string]string{"Idempotency-Key": "query-restricted-0003"}, map[string]any{"selector": source}, 404, nil)
}

func TestPlaybackV1QueueQueryShuffleIsSeededAndIncludesEveryMatch(t *testing.T) {
	f := newV1Fixture(t, 12)
	var prior []string
	for n := 0; n < 2; n++ {
		var q playbackv1.QueueReply
		f.call("POST", "/v1/queues", map[string]string{"Idempotency-Key": fmt.Sprintf("query-shuffle-%06d", n)}, map[string]any{"selector": querySource(f.library, "asc"), "shuffle": map[string]any{"seed": "42"}}, 201, &q)
		got := queueItemsInOrder(t, f, f.owner.AccessToken, q.Queue.ID, q.Queue.Total)
		sameSet(t, got, f.items)
		if n > 0 && !reflect.DeepEqual(got, prior) {
			t.Fatal("same seed changed order")
		}
		prior = got
	}
}

func TestSearchSongQueueHTTPRetainsCaptureAndSeededShuffle(t *testing.T) {
	f := newV1Fixture(t, 0)
	c := f.catalogTest
	music := c.Library("search-music", "Music", "music", f.root+"/music")
	artist := c.Artist(music, "Artist")
	album := c.Album(artist, "Album", 2025)
	ids := []string{}
	records := []int64{}
	for i := 0; i < 603; i++ {
		song := c.Song(album, i+1, fmt.Sprintf("%s/music/%04d.flac", f.root, i), fmt.Sprintf("Harbor %04d", i))
		ids = append(ids, song.Public)
		records = append(records, song.ID)
	}
	c.Drain()
	source := map[string]any{"search": map[string]any{"q": "Harbor", "group": "songs", "sort": "title", "direction": "asc", "libraryIds": []string{"search-music"}}}
	f.v1.HoldQueueBuilds()
	var first playbackv1.QueueReply
	f.call("POST", "/v1/queues", map[string]string{"Idempotency-Key": "search-capture-000001"}, map[string]any{"selector": source}, 201, &first)
	if first.Queue.Total != 256 || len(first.Queue.Segments) != 1 || first.Queue.Segments[0].SourceRevision == "" {
		t.Fatalf("bounded capture %+v", first)
	}
	c.Fields(records[0], map[string]any{"title": "Changed outside Search"})
	added := c.Song(album, 604, f.root+"/music/new.flac", "Harbor new")
	c.Drain()
	f.v1.ResumeQueueBuilds()
	done := waitBuilt(t, f, f.owner.AccessToken, first.Queue.ID, 30*time.Second)
	got := queueItemsInOrder(t, f, f.owner.AccessToken, first.Queue.ID, done.Total)
	if done.Total != 603 || !reflect.DeepEqual(got, ids) {
		t.Fatalf("retained Search capture changed: %d", done.Total)
	}
	current := append(append([]string{}, ids[1:]...), added.Public)
	var previous []string
	for round := 0; round < 2; round++ {
		var shuffled playbackv1.QueueReply
		f.call("POST", "/v1/queues", map[string]string{"Idempotency-Key": fmt.Sprintf("search-shuffle-%06d", round)}, map[string]any{"selector": source, "shuffle": map[string]any{"seed": "42"}}, 201, &shuffled)
		built := waitBuilt(t, f, f.owner.AccessToken, shuffled.Queue.ID, 30*time.Second)
		order := queueItemsInOrder(t, f, f.owner.AccessToken, shuffled.Queue.ID, built.Total)
		sameSet(t, order, current)
		if round > 0 && !reflect.DeepEqual(order, previous) {
			t.Fatal("same Search shuffle seed changed order")
		}
		previous = order
	}
}
