package catalog

import (
	"context"
	"encoding/binary"
	"errors"
	"reflect"
	"runtime"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"portico.local/server/internal/readprep"
)

type orderedInfoTestSource struct {
	rows                       []fileInfoOrderedRow
	index                      int
	reads, comparisons, checks atomic.Int64
	staleAt                    int64
	badSequence                bool
}

func (s *orderedInfoTestSource) AccountedBytes() int64 { return 128 + int64(cap(s.rows))*32 }
func (s *orderedInfoTestSource) Check(context.Context) error {
	if n := s.checks.Add(1); s.staleAt > 0 && n >= s.staleAt {
		return readprep.ErrStale
	}
	return nil
}
func (s *orderedInfoTestSource) Next(context.Context) (fileInfoOrderedRow, bool, error) {
	s.reads.Add(1)
	if s.index == len(s.rows) {
		return fileInfoOrderedRow{}, false, nil
	}
	row := s.rows[s.index]
	s.index++
	row.Sequence = int64(s.index)
	if s.badSequence {
		row.Sequence = 1
	}
	return row, true, nil
}
func orderedInfoTestCompare(a, b fileInfoOccurrence) int {
	if a.Item < b.Item {
		return -1
	}
	if a.Item > b.Item {
		return 1
	}
	x, y := a.Asset%997, b.Asset%997
	if x < y {
		return -1
	}
	if x > y {
		return 1
	}
	if a.Asset < b.Asset {
		return -1
	}
	if a.Asset > b.Asset {
		return 1
	}
	return 0
}
func (s *orderedInfoTestSource) Compare(_ context.Context, a, b fileInfoOccurrence) (int, error) {
	s.comparisons.Add(1)
	return orderedInfoTestCompare(a, b), nil
}
func orderedInfoTestRun(t *testing.T, source fileInfoOrderedSource, spec fileInfoOrderedSpec) (fileInfoOrderedSummary, error) {
	t.Helper()
	store, err := readprep.New(readprep.Config{Workers: 1, MaxBytes: 128 << 10})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	plan, err := fileInfoOrderedPlan(source, spec)
	if err != nil {
		return fileInfoOrderedSummary{}, err
	}
	ctx := context.Background()
	handle, err := store.Submit(ctx, "ordered-private", readprep.Key{Namespace: "ordered-test", Basis: "original-info-head", Canonical: "window"}, plan)
	if err != nil {
		return fileInfoOrderedSummary{}, err
	}
	defer store.Cancel(ctx, "ordered-private", handle)
	timeout := time.NewTimer(5 * time.Second)
	defer timeout.Stop()
	for {
		result, err := store.Poll(ctx, "ordered-private", handle)
		if err != nil {
			return fileInfoOrderedSummary{}, err
		}
		if result.State == readprep.Ready {
			defer result.Release()
			if err := result.Reserve(1600); err != nil {
				return fileInfoOrderedSummary{}, err
			}
			if len(result.Payload) > 1620 {
				t.Fatal("summary retained something beyond bounded numeric coordinates")
			}
			return decodeFileInfoOrderedSummary(result.Payload)
		}
		result.Release()
		select {
		case <-timeout.C:
			t.Fatal("ordered preparation did not finish")
		default:
			runtime.Gosched()
		}
	}
}

func TestFileInfoOrderedWindowsPreserveFullComparatorAndExactCounts(t *testing.T) {
	rows := make([]fileInfoOrderedRow, 1000)
	all := []fileInfoOccurrence{}
	eligible := int64(0)
	for i := range rows {
		rows[i] = fileInfoOrderedRow{Occurrence: fileInfoOccurrence{int64(i%7 + 1), int64(1000 - i)}, Match: i%5 != 0, Eligible: i%2 == 0}
		if rows[i].Match {
			all = append(all, rows[i].Occurrence)
			if rows[i].Eligible {
				eligible++
			}
		}
	}
	sort.Slice(all, func(i, j int) bool { return orderedInfoTestCompare(all[i], all[j]) < 0 })
	for _, tc := range []struct {
		name      string
		direction byte
		anchor    int
	}{{"initial", fileInfoForward, -1}, {"after-first", fileInfoForward, 0}, {"after-middle", fileInfoForward, 400}, {"after-last", fileInfoForward, len(all) - 1}, {"before-first", fileInfoBackward, 0}, {"before-middle", fileInfoBackward, 400}, {"before-last", fileInfoBackward, len(all) - 1}, {"reload-first", fileInfoReload, 0}, {"reload-middle", fileInfoReload, 400}, {"reload-last", fileInfoReload, len(all) - 1}} {
		t.Run(tc.name, func(t *testing.T) {
			source := &orderedInfoTestSource{rows: rows}
			spec := fileInfoOrderedSpec{Limit: 7, Direction: tc.direction}
			lo, hi := 0, min(7, len(all))
			if tc.anchor >= 0 {
				spec.HasAnchor = true
				spec.Anchor = all[tc.anchor]
				switch tc.direction {
				case fileInfoForward:
					lo, hi = tc.anchor+1, min(tc.anchor+8, len(all))
				case fileInfoBackward:
					lo, hi = max(0, tc.anchor-7), tc.anchor
				case fileInfoReload:
					lo, hi = tc.anchor, min(tc.anchor+7, len(all))
				}
			}
			page, err := orderedInfoTestRun(t, source, spec)
			if err != nil {
				t.Fatal(err)
			}
			if page.Total != int64(len(all)) || page.Eligible != eligible || !reflect.DeepEqual(page.Occurrences, all[lo:hi]) || page.Earlier != (lo > 0) || page.Later != (hi < len(all)) {
				t.Fatalf("page %+v want coordinates%v earlier%v later%v", page, all[lo:hi], lo > 0, hi < len(all))
			}
			if source.reads.Load() != 1001 || source.comparisons.Load() > 6000 {
				t.Fatal("source rewalked or used unbounded rank work", source.reads.Load(), source.comparisons.Load())
			}
		})
	}
}

func TestFileInfoOrderedRejectsScopeDriftUnknownAnchorAndNonprogress(t *testing.T) {
	rows := []fileInfoOrderedRow{{Occurrence: fileInfoOccurrence{1, 2}, Match: true}, {Occurrence: fileInfoOccurrence{1, 3}, Match: true}}
	for _, source := range []*orderedInfoTestSource{{rows: rows, staleAt: 1}, {rows: rows, staleAt: 2}, {rows: rows, staleAt: 3}, {rows: rows, badSequence: true}} {
		if _, err := orderedInfoTestRun(t, source, fileInfoOrderedSpec{Limit: 1, Direction: fileInfoForward}); err == nil {
			t.Fatal("invalid/stale source succeeded")
		}
	}
	if _, err := orderedInfoTestRun(t, &orderedInfoTestSource{rows: rows}, fileInfoOrderedSpec{Limit: 1, Direction: fileInfoForward, HasAnchor: true, Anchor: fileInfoOccurrence{2, 5}}); !errors.Is(err, readprep.ErrStale) {
		t.Fatal("foreign opening occurrence accepted", err)
	}
	empty, err := orderedInfoTestRun(t, &orderedInfoTestSource{}, fileInfoOrderedSpec{Limit: 100, Direction: fileInfoForward})
	if err != nil || empty.Total != 0 || len(empty.Occurrences) != 0 || empty.Earlier || empty.Later {
		t.Fatal(empty, err)
	}
}

func TestFileInfoOrderedSummaryValidationAndAdmission(t *testing.T) {
	for _, payload := range [][]byte{nil, make([]byte, 20), {1, 101, 0, 0}, append([]byte{1, 1, 2, 0}, make([]byte, 32)...)} {
		if _, err := decodeFileInfoOrderedSummary(payload); err == nil {
			t.Fatal("malformed summary accepted")
		}
	}
	duplicate := make([]byte, 52)
	duplicate[0], duplicate[1] = 1, 2
	binary.BigEndian.PutUint64(duplicate[4:12], 2)
	for _, offset := range []int{20, 28, 36, 44} {
		binary.BigEndian.PutUint64(duplicate[offset:offset+8], 1)
	}
	if _, err := decodeFileInfoOrderedSummary(duplicate); err == nil {
		t.Fatal("duplicate page coordinates accepted")
	}
	store, err := readprep.New(readprep.Config{Workers: 1, MaxBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	source := &orderedInfoTestSource{}
	plan, err := fileInfoOrderedPlan(source, fileInfoOrderedSpec{Limit: 1, Direction: fileInfoForward})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Submit(context.Background(), "caller", readprep.Key{Namespace: "ordered", Basis: "basis", Canonical: "window"}, plan); !errors.Is(err, readprep.ErrCapacity) || source.reads.Load() != 0 || source.checks.Load() != 0 {
		t.Fatal("work started before memory admission", err)
	}
}
