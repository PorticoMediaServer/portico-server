package catalog

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unsafe"

	"portico.local/server/internal/compactcatalog"
)

// This backend is deliberately not dispatched from Facets until the shared
// readprep/client protocol and production member indexes have been qualified.
var errReadingFacetCapacity = errors.New("Reading facet memory capacity")

const readingFacetRawLimit = 4096
const readingFacetByteLimit = 4 << 20
const readingFacetQuantum = 10 * time.Millisecond

type readingFacetBudget struct {
	Raw, Bytes int
	deadline   time.Time
	oversized  bool
}

func newReadingFacetBudget(raw, bytes int, duration time.Duration) *readingFacetBudget {
	return &readingFacetBudget{Raw: min(max(raw, 1), readingFacetRawLimit), Bytes: min(max(bytes, 1), readingFacetByteLimit), deadline: time.Now().Add(min(max(duration, time.Nanosecond), readingFacetQuantum))}
}
func (b *readingFacetBudget) available() bool {
	return b.Raw > 0 && b.Bytes > 0 && !b.oversized && time.Now().Before(b.deadline)
}

// A scalar is read losslessly once. Its actual bytes are charged even when it
// exceeds the quantum; no second read follows it in that quantum. Time is
// cooperative between statements, not a SQLite instruction/preemption limit.
func (b *readingFacetBudget) charge(bytes int) {
	b.Raw--
	b.Bytes -= bytes
	if b.Bytes < 0 {
		b.oversized = true
	}
}

type readingFacetMember struct {
	Key, Label, Source string
	Item               int64
}

func (m readingFacetMember) bytes() int {
	return int(unsafe.Sizeof(m)) + len(m.Key) + len(m.Label) + len(m.Source)
}

type readingFacetInput interface {
	// next returns only a fully evaluated member, including rejected members.
	// Every raw/nested row and EOF probe must consume b; no read cursor survives.
	next(*Service, *readingFacetBudget) (m readingFacetMember, eligible, present, eof bool, err error)
	reset()
	bytes() int
	scalarBytes() int
}

type readingFacetChoice struct {
	key, label string
	count      int
}

func (v readingFacetChoice) bytes() int { return int(unsafe.Sizeof(v)) + len(v.key) + len(v.label) }

type readingFacetReduction struct {
	request                   FacetRequest
	witness, revision, needle string
	input                     readingFacetInput
	page                      []readingFacetChoice
	// One value and one spelling, never a map of values or spellings.
	key, label, winner                                       string
	frequency, owners, total, winnerFrequency, winnerOwners  int
	have                                                     bool
	phase                                                    string // first, threshold, rank, output, done
	distinct, maxCount, low, high, threshold, above, tieSkip int
	passes                                                   int
	memoryLimit                                              int
	uncacheable                                              bool
	scalarBytes                                              int
	failed                                                   error
}

func newReadingFacetReduction(request FacetRequest, witness string, input readingFacetInput, memoryLimit int) (*readingFacetReduction, error) {
	if input == nil || witness == "" {
		return nil, ErrReadingProjectionChanged
	}
	if request.Field != "author" && request.Field != "series" {
		return nil, browseIssue("field", "Reading counts author and series facets")
	}
	if len(request.Q) > 100 || request.Offset < 0 {
		return nil, browseIssue("q", "facet window is invalid")
	}
	if request.Sort != "" && request.Sort != "title" && request.Sort != "count" {
		return nil, browseIssue("sort", "Reading facets sort by title or count")
	}
	if request.Limit < 1 || request.Limit > browseFacetLimit {
		request.Limit = browseFacetLimit
	}
	if memoryLimit < 1 {
		return nil, errReadingFacetCapacity
	}
	revision := fmt.Sprintf("%x", sha256.Sum256([]byte("reading-facet-v1:"+witness+":"+request.Library+":"+request.Field+":"+queryFingerprint(browseAnd(request.Group, request.Query)))))
	return &readingFacetReduction{request: FacetRequest{Library: request.Library, Field: request.Field, Q: request.Q, Sort: request.Sort, Limit: request.Limit, Offset: request.Offset}, witness: witness, revision: revision, needle: compactcatalog.ReadingValueKey(request.Q), input: input, phase: "first", memoryLimit: memoryLimit}, nil
}

// step must be called within a fresh short read snapshot. The owner captures
// browseExactWitness(library,profile,effectiveRestrictions) in that snapshot,
// checks live authority, and passes it here. It repeats those checks before
// exposing result(). No snapshot/Service/transaction is retained by this state.
func (r *readingFacetReduction) step(s *Service, currentWitness string, b *readingFacetBudget) (bool, error) {
	if currentWitness == "" || currentWitness != r.witness {
		r.failed = ErrReadingProjectionChanged
	}
	if r.failed != nil {
		return false, r.failed
	}
	if r.phase == "done" {
		return true, nil
	}
	for b.available() {
		m, eligible, present, eof, err := r.input.next(s, b)
		if err != nil {
			r.failed = err
			return false, err
		}
		if present && eligible {
			if r.have && (r.key != m.Key || r.label != m.Label) {
				r.finishSpelling()
			}
			if r.have && r.key != m.Key {
				r.finishValue()
				r.have = false
			}
			if !r.have {
				r.key = m.Key
				r.label = m.Label
				r.have = true
			} else {
				r.label = m.Label
			}
			r.frequency++
			r.total++
			if m.Source == "owner" {
				r.owners++
			}
		}
		if eof {
			if r.have {
				r.finishSpelling()
				r.finishValue()
				r.have = false
			}
			r.endPass()
		}
		// Charge retained state plus the projected DTO. A legal scalar may have
		// a constant number of references/copies across cursor, reduction and
		// output. The exception is O(B), never an unbounded page or cache entry.
		size := r.bytes()
		r.scalarBytes = max(r.scalarBytes, len(m.Key)+len(m.Label)+len(m.Source), r.input.scalarBytes())
		scalar := r.scalarBytes
		if b.oversized {
			r.uncacheable = true
		}
		if size > r.memoryLimit {
			r.uncacheable = true
		}
		if size > r.memoryLimit+4*scalar {
			r.failed = errReadingFacetCapacity
			return false, r.failed
		}
		if r.phase == "done" {
			return true, nil
		}
		if !present && !eof && !b.available() {
			break
		}
	}
	return false, nil
}
func (r *readingFacetReduction) finishSpelling() {
	if r.frequency == 0 {
		return
	}
	win := r.winnerFrequency == 0 || (r.owners > 0 && r.winnerOwners == 0)
	if (r.owners > 0) == (r.winnerOwners > 0) {
		score, old := r.frequency, r.winnerFrequency
		if r.owners > 0 {
			score, old = r.owners, r.winnerOwners
		}
		win = win || score > old || score == old && r.label < r.winner
	}
	if win {
		r.winner, r.winnerFrequency, r.winnerOwners = r.label, r.frequency, r.owners
	}
	r.frequency, r.owners = 0, 0
}
func (r *readingFacetReduction) finishValue() {
	if strings.Contains(r.key, r.needle) {
		v := readingFacetChoice{r.key, r.winner, r.total}
		switch r.phase {
		case "first":
			r.distinct++
			r.maxCount = max(r.maxCount, v.count)
			if r.request.Sort != "count" && r.distinct > r.request.Offset && len(r.page) < r.request.Limit {
				r.page = append(r.page, v)
			}
		case "threshold", "rank":
			if v.count > r.threshold {
				r.above++
			}
		case "output":
			if v.count < r.threshold {
				r.offer(v)
			} else if v.count == r.threshold {
				if r.tieSkip > 0 {
					r.tieSkip--
				} else {
					r.offer(v)
				}
			}
		}
	}
	r.key, r.label, r.winner = "", "", ""
	r.total, r.winnerFrequency, r.winnerOwners = 0, 0, 0
}
func (r *readingFacetReduction) offer(v readingFacetChoice) {
	i := sort.Search(len(r.page), func(i int) bool { p := r.page[i]; return p.count < v.count || p.count == v.count && p.key > v.key })
	if i >= r.request.Limit {
		return
	}
	if len(r.page) < r.request.Limit {
		r.page = append(r.page, readingFacetChoice{})
	}
	copy(r.page[i+1:], r.page[i:len(r.page)-1])
	r.page[i] = v
}
func (r *readingFacetReduction) endPass() {
	r.passes++
	switch r.phase {
	case "first":
		if r.request.Sort != "count" || r.request.Offset >= r.distinct {
			r.phase = "done"
			return
		}
		r.low, r.high = 1, r.maxCount
	case "threshold":
		if r.above > r.request.Offset {
			r.low = r.threshold + 1
		} else {
			r.high = r.threshold
		}
	case "rank":
		r.tieSkip = r.request.Offset - r.above
		r.phase = "output"
		r.input.reset()
		return
	case "output":
		r.phase = "done"
		return
	}
	// Rank selection uses O(log maximum eligible value frequency) full passes,
	// independent of offset. Final output retains at most Limit choices.
	if r.low < r.high {
		r.phase = "threshold"
		r.threshold = r.low + (r.high-r.low)/2
	} else {
		r.phase = "rank"
		r.threshold = r.low
	}
	r.above = 0
	r.input.reset()
}
func (r *readingFacetReduction) bytes() int {
	n := int(unsafe.Sizeof(*r)) + len(r.witness) + len(r.revision) + len(r.needle) + len(r.key) + len(r.label) + len(r.winner) + r.input.bytes()
	n += len(r.request.Library) + len(r.request.Field) + len(r.request.Q) + len(r.request.Sort)
	// Input owns the compiled bounded query. Do not retain the original AST.
	for _, v := range r.page {
		n += v.bytes()
	}
	n += (cap(r.page) - len(r.page)) * int(unsafe.Sizeof(readingFacetChoice{}))
	return n + r.resultBytes()
}

// The protocol owner reserves both state and output until it drops the
// computation. Text aliases are deliberately charged again conservatively.
func (r *readingFacetReduction) resultBytes() int {
	n := int(unsafe.Sizeof(FacetPage{})) + len(r.request.Field) + len(r.revision)
	for _, v := range r.page {
		n += int(unsafe.Sizeof(FacetValue{})) + 2*len(v.label)
		if r.request.Field == "series" {
			n += 43
		}
	}
	return n
}
func (r *readingFacetReduction) cacheable() bool {
	return r.phase == "done" && r.failed == nil && !r.uncacheable && r.bytes() <= r.memoryLimit
}
func (r *readingFacetReduction) result() (FacetPage, bool) {
	if r.phase != "done" || r.failed != nil {
		return FacetPage{}, false
	}
	out := FacetPage{Field: r.request.Field, Total: r.distinct, Revision: r.revision, Values: make([]FacetValue, 0, len(r.page))}
	for _, v := range r.page {
		f := FacetValue{Value: v.label, Label: v.label, Count: v.count}
		if r.request.Field == "series" {
			f.SeriesID = ReadingFacetSeriesID(compactcatalog.ReadingSeriesID(r.request.Library, v.label))
		}
		out.Values = append(out.Values, f)
	}
	return out, true
}
