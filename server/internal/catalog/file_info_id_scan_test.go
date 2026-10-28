package catalog

import (
	"context"
	"errors"
	"math"
	"reflect"
	"runtime"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/server/internal/readprep"
)

type fileInfoIDTestSource struct {
	ids       []int64
	match     func(int64) bool
	reads     atomic.Int64
	checks    atomic.Int64
	staleAt   int64
	badCursor bool
}

func (s *fileInfoIDTestSource) AccountedBytes() int64 { return 128 + int64(cap(s.ids))*8 }
func (s *fileInfoIDTestSource) Check(context.Context) error {
	if n := s.checks.Add(1); s.staleAt > 0 && n >= s.staleAt {
		return readprep.ErrStale
	}
	return nil
}
func (s *fileInfoIDTestSource) Next(_ context.Context, has bool, after int64) (int64, bool, bool, error) {
	s.reads.Add(1)
	i := 0
	if has {
		i = sort.Search(len(s.ids), func(i int) bool { return s.ids[i] > after })
		if s.badCursor {
			return after, true, true, nil
		}
	}
	if i == len(s.ids) {
		return 0, false, false, nil
	}
	id := s.ids[i]
	return id, s.match == nil || s.match(id), true, nil
}

func fileInfoIDTestRun(t *testing.T, source fileInfoIDSource, spec fileInfoIDSpec) (fileInfoIDSummary, error) {
	t.Helper()
	store, err := readprep.New(readprep.Config{Workers: 1, MaxBytes: 128 << 10})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	plan, err := fileInfoIDPlan(source, spec)
	if err != nil {
		return fileInfoIDSummary{}, err
	}
	ctx := context.Background()
	h, err := store.Submit(ctx, "private-caller", readprep.Key{Namespace: "file-info-id-test", Basis: "exact-source", Canonical: "window"}, plan)
	if err != nil {
		return fileInfoIDSummary{}, err
	}
	defer store.Cancel(ctx, "private-caller", h)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		result, err := store.Poll(ctx, "private-caller", h)
		if err != nil {
			return fileInfoIDSummary{}, err
		}
		if result.State == readprep.Ready {
			defer result.Release()
			if err := result.Reserve(800); err != nil {
				return fileInfoIDSummary{}, err
			}
			return decodeFileInfoIDs(result.Payload)
		}
		result.Release()
		select {
		case <-timer.C:
			t.Fatal("ID preparation did not finish")
		default:
			runtime.Gosched()
		}
	}
}

func TestFileInfoIDWindowsExactTotalsAndDirections(t *testing.T) {
	ids := make([]int64, 1000)
	for i := range ids {
		ids[i] = int64(i)
	}
	for _, tc := range []struct {
		name           string
		spec           fileInfoIDSpec
		want           []int64
		earlier, later bool
	}{
		{"initial", fileInfoIDSpec{Limit: 3, Direction: fileInfoForward}, []int64{0, 2, 4}, false, true},
		{"after", fileInfoIDSpec{Limit: 3, Direction: fileInfoForward, HasAnchor: true, Anchor: 4}, []int64{6, 8, 10}, true, true},
		{"reload", fileInfoIDSpec{Limit: 3, Direction: fileInfoReload, HasAnchor: true, Anchor: 4}, []int64{4, 6, 8}, true, true},
		{"before", fileInfoIDSpec{Limit: 3, Direction: fileInfoBackward, HasAnchor: true, Anchor: 10}, []int64{4, 6, 8}, true, true},
		{"first-before", fileInfoIDSpec{Limit: 3, Direction: fileInfoBackward, HasAnchor: true, Anchor: 4}, []int64{0, 2}, false, true},
		{"last-after", fileInfoIDSpec{Limit: 3, Direction: fileInfoForward, HasAnchor: true, Anchor: 994}, []int64{996, 998}, true, false},
		{"empty-after", fileInfoIDSpec{Limit: 3, Direction: fileInfoForward, HasAnchor: true, Anchor: 999}, []int64{}, true, false},
		{"empty-before", fileInfoIDSpec{Limit: 3, Direction: fileInfoBackward, HasAnchor: true, Anchor: 0}, []int64{}, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := &fileInfoIDTestSource{ids: ids, match: func(id int64) bool { return id%2 == 0 }}
			got, err := fileInfoIDTestRun(t, source, tc.spec)
			if err != nil {
				t.Fatal(err)
			}
			if got.Total != 500 || !reflect.DeepEqual(got.IDs, tc.want) || got.Earlier != tc.earlier || got.Later != tc.later {
				t.Fatalf("got %+v, expected IDs%v edges%v/%v", got, tc.want, tc.earlier, tc.later)
			}
			if source.reads.Load() != 1001 {
				t.Fatalf("raw rejects/EOF were skipped: %d reads", source.reads.Load())
			}
			if source.checks.Load() < 17 { // >=8 bounded quanta, each with before/after check.
				t.Fatal("whole source was consumed in one quantum")
			}
		})
	}
}

func TestFileInfoIDWindowsIntegerEdgesAndBasisDrift(t *testing.T) {
	source := &fileInfoIDTestSource{ids: []int64{math.MinInt64, -1, 0, math.MaxInt64}}
	got, err := fileInfoIDTestRun(t, source, fileInfoIDSpec{Limit: 100, Direction: fileInfoForward})
	if err != nil || !reflect.DeepEqual(got.IDs, source.ids) || got.Total != 4 || got.Earlier || got.Later {
		t.Fatal(got, err)
	}
	for _, n := range []int64{1, 2, 3} {
		source = &fileInfoIDTestSource{ids: []int64{1, 2}, staleAt: n}
		_, err = fileInfoIDTestRun(t, source, fileInfoIDSpec{Limit: 1, Direction: fileInfoForward})
		if !errors.Is(err, readprep.ErrStale) {
			t.Fatalf("basis drift at check%d: %v", n, err)
		}
	}
	source = &fileInfoIDTestSource{ids: []int64{1, 2}, badCursor: true}
	if _, err = fileInfoIDTestRun(t, source, fileInfoIDSpec{Limit: 1, Direction: fileInfoForward}); err == nil {
		t.Fatal("non-progressing source was accepted")
	}
}

func TestFileInfoIDWindowsMalformedPayloadAndAdmission(t *testing.T) {
	for _, raw := range [][]byte{nil, make([]byte, 12), {1, 255, 0, 0}, append([]byte{1, 101}, make([]byte, 818)...)} {
		if _, err := decodeFileInfoIDs(raw); err == nil {
			t.Fatal("malformed ID payload accepted")
		}
	}
	for _, limit := range []int{0, 101, -1} {
		if _, err := fileInfoIDPlan(&fileInfoIDTestSource{}, fileInfoIDSpec{Limit: limit, Direction: fileInfoForward}); !errors.Is(err, readprep.ErrInvalid) {
			t.Fatal(limit, err)
		}
	}
	store, err := readprep.New(readprep.Config{MaxBytes: 4096, Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	source := &fileInfoIDTestSource{ids: []int64{1}}
	plan, err := fileInfoIDPlan(source, fileInfoIDSpec{Limit: 1, Direction: fileInfoForward})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Submit(context.Background(), "caller", readprep.Key{Namespace: "id-test", Basis: "basis", Canonical: "window"}, plan)
	if !errors.Is(err, readprep.ErrCapacity) || source.reads.Load() != 0 || source.checks.Load() != 0 {
		t.Fatal("allocation/read happened before budget admission", err)
	}
}
