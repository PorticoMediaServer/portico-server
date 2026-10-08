package directdns

import (
	"encoding/base32"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func scaleNamespace(i int) string {
	var b [12]byte
	binary.BigEndian.PutUint64(b[4:], uint64(i))
	return "ptc-" + strings.ToLower(base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:]))
}
func TestMillionRowIncrementalApply(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test")
	}
	path := filepath.Join(t.TempDir(), "table.json")
	table, e := OpenTable(path)
	if e != nil {
		t.Fatal(e)
	}
	// 50,000 rows by default: a rewrite of the table on a one-row apply would
	// already be megabytes, far over the journal bound below. The release and
	// deep performance tiers run the full million.
	size := 50000
	switch os.Getenv("PORTICO_PERFORMANCE_TIER") {
	case "release", "deep":
		size = 1000000
	}
	rows := make([]FeedRow, size)
	for i := range rows {
		rows[i] = FeedRow{Sequence: int64(i + 1), Namespace: scaleNamespace(i), Label: "0123456789abcdef0123456789abcdef", IPv4: "8.8.8.8"}
	}
	if e = table.Apply(rows); e != nil {
		t.Fatal(e)
	}
	if table.Len() != len(rows) {
		t.Fatalf("rows=%d", table.Len())
	}
	if e = table.Compact(); e != nil {
		t.Fatal(e)
	}
	row := rows[12345]
	row.Sequence = int64(size + 1)
	row.IPv4 = "1.1.1.1"
	start := time.Now()
	if e = table.Apply([]FeedRow{row}); e != nil {
		t.Fatal(e)
	}
	elapsed := time.Since(start)
	info, e := os.Stat(table.journal)
	if e != nil {
		t.Fatal(e)
	}
	if info.Size() > 65536 {
		t.Fatalf("one row wrote %d bytes", info.Size())
	}
	t.Logf("%d-row table, one-row apply: %s, %d bytes", size, elapsed, info.Size())
	// A26: one row after a million must cost a journal append, not a rewrite.
	// The bound is generous for a loaded CI disk yet far below the several
	// seconds an O(fleet) persist takes.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("one-row apply took %s at %d rows", elapsed, size)
	}
	reopened, e := OpenTable(path)
	if e != nil {
		t.Fatal(e)
	}
	if reopened.Summary()["digest"] != table.Summary()["digest"] {
		t.Fatal("recovery differs")
	}
}
func TestTornJournalTail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "table")
	table, _ := OpenTable(path)
	ns, _ := NewNamespace()
	if e := table.Apply([]FeedRow{{Sequence: 1, Namespace: ns, IPv4: "8.8.8.8"}}); e != nil {
		t.Fatal(e)
	}
	f, _ := os.OpenFile(table.journal, os.O_APPEND|os.O_WRONLY, 0600)
	f.WriteString("[{truncated")
	f.Close()
	restored, e := OpenTable(path)
	if e != nil {
		t.Fatal(e)
	}
	if restored.Len() != 1 || restored.Sequence() != 1 {
		t.Fatal("lost committed row")
	}
}

// A19: a compaction that overlaps applies still truncates. Rows applied while
// the snapshot is written go to the new journal; the old one is deleted; a
// reopen sees every row.
func TestCompactionTruncatesUnderSteadyChurn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "table.json")
	table, e := OpenTable(path)
	if e != nil {
		t.Fatal(e)
	}
	ns := make([]string, 64)
	for i := range ns {
		ns[i] = scaleNamespace(i)
	}
	seq := int64(0)
	apply := func() {
		seq++
		if err := table.Apply([]FeedRow{{Sequence: seq, Namespace: ns[seq%64], Label: "0123456789abcdef0123456789abcdef", IPv4: "8.8.8.8"}}); err != nil {
			t.Error(err)
		}
	}
	for range 5000 {
		apply()
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				apply()
			}
		}
	}()
	for range 5 {
		if e = table.Compact(); e != nil {
			t.Fatal(e)
		}
	}
	close(stop)
	<-done
	before := table.Sequence()
	if e = table.Compact(); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(table.journal)
	if e != nil {
		t.Fatal(e)
	}
	if info.Size() != 0 || table.Sequence() != before {
		t.Fatalf("journal %d bytes after a quiet compaction", info.Size())
	}
	journals, _ := filepath.Glob(filepath.Join(dir, ".dns-journal-*"))
	legacy, _ := filepath.Glob(path + ".journal")
	if len(journals)+len(legacy) != 1 {
		t.Fatalf("old journals left behind: %v %v", journals, legacy)
	}
	reopened, e := OpenTable(path)
	if e != nil {
		t.Fatal(e)
	}
	if reopened.Sequence() != table.Sequence() || reopened.Summary()["digest"] != table.Summary()["digest"] {
		t.Fatal("reopen differs after compaction under churn")
	}
}

// A91: a crash mid-compaction or a failed snapshot write leaves no journal
// behind: opening removes unreferenced ones, and the next successful
// compaction removes every journal the durable snapshot no longer names.
func TestUnreferencedJournalsAreRemoved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "table.json")
	table, e := OpenTable(path)
	if e != nil {
		t.Fatal(e)
	}
	if e = table.Apply([]FeedRow{{Sequence: 1, Namespace: scaleNamespace(1), IPv4: "8.8.8.8", Label: "0123456789abcdef0123456789abcdef"}}); e != nil {
		t.Fatal(e)
	}
	if e = table.Compact(); e != nil {
		t.Fatal(e)
	}
	// A crash between rotation and snapshot leaves an orphan.
	orphan := filepath.Join(dir, ".dns-journal-orphan")
	if e = os.WriteFile(orphan, []byte("[]\n"), 0600); e != nil {
		t.Fatal(e)
	}
	reopened, e := OpenTable(path)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = os.Stat(orphan); !os.IsNotExist(e) {
		t.Fatal("orphan journal kept at open")
	}
	// A failed snapshot write: make the directory unwritable for the temp file.
	if e = os.Chmod(dir, 0500); e != nil {
		t.Fatal(e)
	}
	failed := reopened.Compact()
	if e = os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if failed == nil {
		t.Skip("directory permissions not enforced (running as root)")
	}
	if e = reopened.Compact(); e != nil {
		t.Fatal(e)
	}
	journals, _ := filepath.Glob(filepath.Join(dir, ".dns-journal-*"))
	legacy, _ := filepath.Glob(path + ".journal")
	if len(journals)+len(legacy) != 1 {
		t.Fatalf("journals left after a failed then successful compaction: %v %v", journals, legacy)
	}
}
