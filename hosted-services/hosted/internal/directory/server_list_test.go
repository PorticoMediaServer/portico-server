package directory

import (
	"crypto/rand"
	"runtime"
	"strconv"
	"testing"

	"portico.local/hosted/internal/security"
)

func accountID(i int) string { return "acct_" + strconv.Itoa(i) + "_Asdv8vT10kDRnnZHN6Ge7x132aRlnh" }

func versionService(t testing.TB, accounts int) *Service {
	t.Helper()
	seal := make([]byte, 32)
	rand.Read(seal)
	s := &Service{keys: &security.Keys{SealKey: seal}}
	v := s.listVersions()
	for i := range accounts {
		v.slots[v.slot(accountID(i))] = int64(i + 1)
	}
	v.high, v.loaded = int64(accounts), true
	return s
}

// A bump always takes a value above every version handed out before it, so a
// slot shared by two accounts can cost a refetch but never hide a change.
func TestServerListSlotsNeverHideAChange(t *testing.T) {
	s := versionService(t, 1000)
	v := s.listVersions()
	// Two accounts sharing one slot behave as below.
	a := accountID(1)
	k := v.slot(a)
	v.slots[k] = 50
	held := s.ServerListVersion(a)
	// The other account changes; the slot moves; a refetches (harmless).
	v.slots[k] = 51
	if _, same, _ := s.CheckServerList(s.WatchToken(a), held); same {
		t.Fatal("slot change not seen")
	}
	// a changes after b: a larger value again.
	held = s.ServerListVersion(a)
	v.slots[k] = 52
	if _, same, _ := s.CheckServerList(s.WatchToken(a), held); same {
		t.Fatal("a change was hidden")
	}
}

// The version check at the scale Hosted is sized for: a million accounts in
// memory. Run with -bench ServerListCheck -benchmem; the report includes the
// map's heap cost per account.
func BenchmarkServerListCheck(b *testing.B) {
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	s := versionService(b, 1_000_000)
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	tokens := make([]string, 1024)
	held := make([]string, len(tokens))
	for i := range tokens {
		tokens[i] = s.WatchToken(accountID(i * 977))
		held[i] = s.ServerListVersion(accountID(i * 977))
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if _, same, e := s.CheckServerList(tokens[i&1023], held[i&1023]); e != nil || !same {
				b.Fatal("check failed", e)
			}
			i++
		}
	})
	b.ReportMetric(float64(after.HeapAlloc-before.HeapAlloc)/1e6, "MB-map")
}
