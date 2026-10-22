package ingestion

import (
	"context"
	"sync"
	"testing"
)

func TestActivityFileIsActualRelativeAndCancellationFenced(t *testing.T) {
	s := &Service{}
	for _, bad := range []string{"/private/movie.mkv", "../movie.mkv", "."} {
		clear := s.activityFile(context.Background(), "job", bad)
		if s.CurrentActivityFile("job") != "" {
			t.Fatal("unsafe relative path published")
		}
		clear()
	}
	ctx, cancel := context.WithCancel(context.Background())
	first := s.activityFile(ctx, "job", "Movies/Film.mkv")
	if s.CurrentActivityFile("job") != "Movies/Film.mkv" {
		t.Fatal("actual file missing")
	}
	cancel()
	if s.CurrentActivityFile("job") != "" {
		t.Fatal("cancelled worker still visible")
	}
	second := s.activityFile(context.Background(), "job", "Movies/Film.mkv")
	first()
	if s.CurrentActivityFile("job") == "" {
		t.Fatal("old cleanup removed replacement observation")
	}
	second()
	if s.CurrentActivityFile("job") != "" {
		t.Fatal("finished file retained")
	}
}
func TestActivityFileConcurrentWorkersAndReads(t *testing.T) {
	s := &Service{}
	var wg sync.WaitGroup
	for n := 0; n < 8; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 100; n++ {
				clear := s.activityFile(context.Background(), "job", "Movie.mkv")
				_ = s.CurrentActivityFile("job")
				clear()
			}
		}()
	}
	wg.Wait()
	if s.CurrentActivityFile("job") != "" {
		t.Fatal("file observation leaked after work")
	}
}
