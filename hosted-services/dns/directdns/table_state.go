package directdns

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

const tableShards = 4096

type tableState struct {
	shards [tableShards]map[string]Addresses
	count  int
}

func shardFor(key string) uint32 {
	h := uint32(2166136261)
	for i := 0; i < len(key); i++ {
		h = (h ^ uint32(key[i])) * 16777619
	}
	return h % tableShards
}
func (s *tableState) rows() map[string]Addresses {
	out := make(map[string]Addresses, s.count)
	for _, part := range s.shards {
		for key, value := range part {
			out[key] = value
		}
	}
	return out
}

type Table struct {
	current   atomic.Pointer[tableState]
	mu        sync.Mutex
	compactMu sync.Mutex
	sequence  int64
	path      string
	journal   string
	// stale are journals no snapshot references any more whose deletion failed
	// or was deferred by a failed snapshot write; the next successful
	// compaction removes them (A91).
	stale        []string
	AllowPrivate bool
}
type tableSnapshot struct {
	Sequence int64                `json:"sequence"`
	Journal  string               `json:"journal,omitempty"`
	Rows     map[string]Addresses `json:"rows"`
}
type FeedRow struct {
	Label              string `json:"label"`
	PreviousLabel      string `json:"previousLabel,omitempty"`
	PreviousLabelUntil string `json:"previousLabelUntil,omitempty"`
	LANIPv4            string `json:"lanIpv4,omitempty"`
	LANIPv6            string `json:"lanIpv6,omitempty"`
	Sequence           int64  `json:"sequence,string"`
	Namespace          string `json:"namespace"`
	IPv4               string `json:"ipv4"`
	IPv6               string `json:"ipv6"`
}

func OpenTable(path string, allowPrivate ...bool) (*Table, error) {
	t := &Table{path: path, AllowPrivate: len(allowPrivate) > 0 && allowPrivate[0]}
	t.journal = path + ".journal"
	t.current.Store(&tableState{})
	if path == "" {
		return t, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		var snap tableSnapshot
		if err = json.Unmarshal(raw, &snap); err != nil {
			return nil, err
		}
		if snap.Journal != "" {
			if filepath.Base(snap.Journal) != snap.Journal {
				return nil, errors.New("invalid journal path")
			}
			t.journal = filepath.Join(filepath.Dir(path), snap.Journal)
		}
		state := &tableState{}
		for key, a := range snap.Rows {
			if validated, ok := t.parseAddresses(key, a.IPv4.String(), a.IPv6.String()); ok || ValidNamespace(key) && (a.LANIPv4.IsPrivate() || a.LANIPv6.IsPrivate()) {
				idx := shardFor(key)
				if state.shards[idx] == nil {
					state.shards[idx] = map[string]Addresses{}
				}
				validated.Label = a.Label
				validated.PreviousLabel, validated.PreviousUntil = a.PreviousLabel, a.PreviousUntil
				if a.LANIPv4.Is4() && a.LANIPv4.IsPrivate() {
					validated.LANIPv4 = a.LANIPv4
				}
				if a.LANIPv6.Is6() && !a.LANIPv6.Is4In6() && a.LANIPv6.IsPrivate() {
					validated.LANIPv6 = a.LANIPv6
				}
				state.shards[idx][key] = validated
				state.count++
			}
		}
		t.sequence = snap.Sequence
		t.current.Store(state)
	}
	if err = t.removeUnreferencedJournals(); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(t.journal, os.O_RDWR, 0600)
	if errors.Is(err, os.ErrNotExist) {
		return t, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	var offset int64
	for {
		line, readErr := reader.ReadBytes('\n')
		if readErr == io.EOF {
			if len(line) > 0 {
				if err = f.Truncate(offset); err != nil {
					return nil, err
				}
				if err = f.Sync(); err != nil {
					return nil, err
				}
			}
			break
		}
		if readErr != nil {
			return nil, readErr
		}
		var rows []FeedRow
		if err = json.Unmarshal(line, &rows); err != nil {
			return nil, err
		}
		state, seq := t.updated(rows)
		t.current.Store(state)
		t.sequence = seq
		offset += int64(len(line))
	}
	return t, nil
}
func (t *Table) parseAddresses(namespace, v4, v6 string) (Addresses, bool) {
	var out Addresses
	if !ValidNamespace(namespace) {
		return out, false
	}
	usable := func(ip netip.Addr) bool {
		return validateAnswerAddress(ip) == nil && !netip.MustParsePrefix("100.64.0.0/10").Contains(ip) && (t.AllowPrivate || !ip.IsPrivate())
	}
	if ip, e := netip.ParseAddr(v4); e == nil && ip.Is4() && usable(ip) {
		out.IPv4 = ip
	}
	if ip, e := netip.ParseAddr(v6); e == nil && ip.Is6() && !ip.Is4In6() && usable(ip) {
		out.IPv6 = ip
	}
	return out, out.IPv4.IsValid() || out.IPv6.IsValid()
}
func (t *Table) Lookup(namespace string) (Addresses, bool) {
	s := t.current.Load()
	a, ok := s.shards[shardFor(namespace)][namespace]
	return a, ok
}
func (t *Table) Sequence() int64 { t.mu.Lock(); defer t.mu.Unlock(); return t.sequence }
func (t *Table) Len() int        { return t.current.Load().count }
func (t *Table) updated(rows []FeedRow) (*tableState, int64) {
	next := *t.current.Load()
	changed := map[uint32]bool{}
	seq := t.sequence
	for _, row := range rows {
		if row.Sequence <= seq {
			continue
		}
		idx := shardFor(row.Namespace)
		if !changed[idx] {
			old := next.shards[idx]
			copy := make(map[string]Addresses, len(old)+1)
			for k, v := range old {
				copy[k] = v
			}
			next.shards[idx] = copy
			changed[idx] = true
		}
		_, exists := next.shards[idx][row.Namespace]
		a, ok := t.parseAddresses(row.Namespace, row.IPv4, row.IPv6)
		lan4, e4 := netip.ParseAddr(row.LANIPv4)
		lan6, e6 := netip.ParseAddr(row.LANIPv6)
		if ValidNamespace(row.Namespace) && ((e4 == nil && lan4.Is4() && lan4.IsPrivate()) || (e6 == nil && lan6.Is6() && !lan6.Is4In6() && lan6.IsPrivate())) {
			ok = true
		}
		if ok {
			a.Label = row.Label
			if until, err := time.Parse(time.RFC3339, row.PreviousLabelUntil); err == nil && ValidMemberLabel(row.PreviousLabel) {
				a.PreviousLabel, a.PreviousUntil = row.PreviousLabel, until.UTC()
			}
			if ip, e := netip.ParseAddr(row.LANIPv4); e == nil && ip.Is4() && ip.IsPrivate() {
				a.LANIPv4 = ip
			}
			if ip, e := netip.ParseAddr(row.LANIPv6); e == nil && ip.Is6() && !ip.Is4In6() && ip.IsPrivate() {
				a.LANIPv6 = ip
			}
			next.shards[idx][row.Namespace] = a
			if !exists {
				next.count++
			}
		} else {
			delete(next.shards[idx], row.Namespace)
			if exists {
				next.count--
			}
		}
		seq = row.Sequence
	}
	return &next, seq
}
func (t *Table) Apply(rows []FeedRow) error {
	if len(rows) == 0 {
		return nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	next, seq := t.updated(rows)
	if seq == t.sequence {
		return nil
	}
	if t.path != "" {
		raw, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		raw = append(raw, '\n')
		f, err := os.OpenFile(t.journal, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		offset, err := f.Seek(0, io.SeekEnd)
		if err == nil {
			var n int
			n, err = f.Write(raw)
			if err == nil && n != len(raw) {
				err = io.ErrShortWrite
			}
		}
		if err == nil {
			err = f.Sync()
		}
		if err != nil {
			_ = f.Truncate(offset)
			_ = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if offset == 0 {
			if err = syncDirectory(filepath.Dir(t.path)); err != nil {
				return err
			}
		}
	}
	t.current.Store(next)
	t.sequence = seq
	return nil
}
func syncDirectory(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func (t *Table) persist(rows map[string]Addresses, sequence int64) error {
	return t.persistWith(rows, sequence, t.journal)
}

// persistWith writes a snapshot that names journal as the file to replay after it.
func (t *Table) persistWith(rows map[string]Addresses, sequence int64, journal string) error {
	if t.path == "" {
		return nil
	}
	raw, err := json.Marshal(tableSnapshot{Sequence: sequence, Journal: filepath.Base(journal), Rows: rows})
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(t.path), ".table-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = io.Copy(f, bytes.NewReader(raw)); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if err = os.Rename(f.Name(), t.path); err != nil {
		return err
	}
	return syncDirectory(filepath.Dir(t.path))
}

// Compact writes a snapshot and truncates the journal without holding the
// mutation lock while it serializes. A19: the journal is rotated under the lock
// at the start, so rows applied during a long (O(fleet)) persist go to the new
// journal, which the new snapshot references; the old journal is deleted once
// the snapshot is durable. Under steady churn the journal therefore always
// truncates. If the process dies before the snapshot lands, the previous
// snapshot and journal still describe the table up to the rotation point, and
// the feed resumes from that sequence.
func (t *Table) Compact() error {
	if t.path == "" {
		return nil
	}
	t.compactMu.Lock()
	defer t.compactMu.Unlock()
	t.mu.Lock()
	state, seq, old := t.current.Load(), t.sequence, t.journal
	next, err := t.newJournal()
	if err != nil {
		t.mu.Unlock()
		return err
	}
	t.journal = next
	t.stale = append(t.stale, old)
	t.mu.Unlock()
	if err = t.persistWith(state.rows(), seq, next); err != nil {
		// The on-disk snapshot still names an older journal; it stays until a
		// snapshot naming a newer one is durable.
		return err
	}
	// The durable snapshot names next: every earlier journal is unreferenced.
	t.mu.Lock()
	stale := t.stale
	t.stale = nil
	t.mu.Unlock()
	for _, file := range stale {
		if file == next {
			continue
		}
		if err = os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			t.mu.Lock()
			t.stale = append(t.stale, file)
			t.mu.Unlock()
			return err
		}
	}
	return nil
}

// removeUnreferencedJournals deletes rotation journals left by a crash
// mid-compaction or a failed snapshot write: every .dns-journal-* next to the
// snapshot except the one it names (A91). Rows only such a journal held are
// fetched again from the feed, which resumes from the snapshot's sequence.
func (t *Table) removeUnreferencedJournals() error {
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(t.path), ".dns-journal-*"))
	if err != nil {
		return err
	}
	for _, file := range matches {
		if filepath.Clean(file) == filepath.Clean(t.journal) {
			continue
		}
		if err = os.Remove(file); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// newJournal creates an empty, durable journal file next to the snapshot.
func (t *Table) newJournal() (string, error) {
	j, err := os.CreateTemp(filepath.Dir(t.path), ".dns-journal-*")
	if err != nil {
		return "", err
	}
	if err = j.Sync(); err != nil {
		j.Close()
		os.Remove(j.Name())
		return "", err
	}
	if err = j.Close(); err != nil {
		os.Remove(j.Name())
		return "", err
	}
	return j.Name(), syncDirectory(filepath.Dir(t.path))
}
