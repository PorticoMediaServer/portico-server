package networking

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// withdrawalFixture gives a remote manager whose route-name cache already holds one address of
// each family — the state a dual-stack server is in after reporting both to Hosted.
func withdrawalFixture(t *testing.T) (*sqlFixture, *RemoteManager, certificateState, string, context.Context) {
	t.Helper()
	f, cert, ctx := certificateFixture(t)
	if e := seedRemoteSettings(ctx, f.db); e != nil {
		t.Fatal(e)
	}
	m := &RemoteManager{h: &ClaimHandler{store: f.store, certificates: cert}}
	_, q, authority, e := cert.snapshot(ctx)
	if e != nil || authority == "" {
		t.Fatal("missing current claim", e)
	}
	for _, address := range []string{"188.68.34.120", "2a03:4000:6:8::1"} {
		if _, e = f.db.Exec(`INSERT INTO networking_route_names(authority,address,port,url) VALUES(?,?,?,?)`, authority, address, 32500, "https://current.example:32500"); e != nil {
			t.Fatal(e)
		}
	}
	return f, m, q, authority, ctx
}

func reportedAddresses(t *testing.T, f *sqlFixture, authority string) map[string]string {
	t.Helper()
	rows, e := f.db.Query(`SELECT address FROM networking_route_names WHERE authority=?`, authority)
	if e != nil {
		t.Fatal(e)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var address string
		if e = rows.Scan(&address); e != nil {
			t.Fatal(e)
		}
		if strings.Contains(address, ":") {
			out[familyIPv6] = address
		} else {
			out[familyIPv4] = address
		}
	}
	return out
}

func absenceRow(t *testing.T, f *sqlFixture, authority, family string) (int, bool) {
	t.Helper()
	var observations int
	e := f.db.QueryRow(`SELECT observations FROM networking_address_absence WHERE authority=? AND family=?`, authority, family).Scan(&observations)
	if e != nil {
		return 0, false
	}
	return observations, true
}

// answerWithdrawals points the transport at a stub that accepts exactly the withdrawal call
// and counts it.
func answerWithdrawals(m *RemoteManager, q certificateState, calls *int, fail error) {
	m.h.certificates.transport.client.Transport = claimRoundTrip(func(r *http.Request) (*http.Response, error) {
		if !strings.HasSuffix(r.URL.Path, "/direct-route/withdraw") {
			return reply(http.StatusNotFound, `{"code":"unexpected"}`), nil
		}
		if fail != nil {
			return nil, fail
		}
		var in struct {
			Family string `json:"family"`
		}
		raw, _ := io.ReadAll(r.Body)
		if json.Unmarshal(raw, &in) != nil || !(in.Family == familyIPv4 || in.Family == familyIPv6) {
			return reply(http.StatusBadRequest, `{"code":"invalid"}`), nil
		}
		*calls++
		body, _ := json.Marshal(map[string]any{"namespace": q.Namespace, "family": in.Family, "withdrawn": true})
		return reply(http.StatusOK, string(body)), nil
	})
}

// The whole point of the rule: a family is not withdrawn because one observation missed it.
// It takes three conclusive observations spanning a quarter of an hour, and a single sighting
// in between starts the count again.
func TestALostFamilyIsWithdrawnOnlyAfterRepeatedObservationsOverTime(t *testing.T) {
	f, m, q, authority, ctx := withdrawalFixture(t)
	calls := 0
	answerWithdrawals(m, q, &calls, nil)
	present := map[string]bool{familyIPv4: true}
	conclusive := map[string]bool{familyIPv4: true, familyIPv6: true}
	start := time.Now().UTC()
	for i := range 3 {
		if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(time.Duration(i)*30*time.Second)); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 0 {
		t.Fatal("an address was withdrawn before the window elapsed")
	}
	if observations, ok := absenceRow(t, f, authority, familyIPv6); !ok || observations != 3 {
		t.Fatal("the absence was not counted", observations, ok)
	}
	// It comes back. Everything counted so far is discarded: an address that is there is not
	// being withdrawn, however long it was away.
	if e := m.withdrawLostFamilies(ctx, q, authority, map[string]bool{familyIPv4: true, familyIPv6: true}, conclusive, start.Add(2*time.Minute)); e != nil {
		t.Fatal(e)
	}
	if _, ok := absenceRow(t, f, authority, familyIPv6); ok {
		t.Fatal("a returning address left its absence behind")
	}
	if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(3*time.Minute)); e != nil {
		t.Fatal(e)
	}
	// Long past the window, but this run has only one observation behind it.
	if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(time.Hour)); e != nil {
		t.Fatal(e)
	}
	if calls != 0 {
		t.Fatal("a fresh absence skipped the observation count", calls)
	}
	if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(time.Hour+time.Minute)); e != nil {
		t.Fatal(e)
	}
	if calls != 1 {
		t.Fatal("the family was never withdrawn", calls)
	}
	// Exactly one call, the IPv6 name is forgotten, the IPv4 name is untouched, and the
	// counter is cleared so nothing here runs again.
	addresses := reportedAddresses(t, f, authority)
	if addresses[familyIPv6] != "" || addresses[familyIPv4] != "188.68.34.120" {
		t.Fatal("the wrong family was forgotten", addresses)
	}
	if _, ok := absenceRow(t, f, authority, familyIPv6); ok {
		t.Fatal("the absence counter survived the withdrawal")
	}
	for i := range 4 {
		if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(time.Duration(2+i)*time.Hour)); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 1 {
		t.Fatal("a withdrawn family was withdrawn again", calls)
	}
}

// A pass that could not look is not evidence. An IPv4 answer from the observer's five-minute
// memory neither confirms nor denies, so it must not age a family towards withdrawal — and
// must not reset a real absence either.
func TestAnInconclusiveObservationNeitherWithdrawsNorResets(t *testing.T) {
	f, m, q, authority, ctx := withdrawalFixture(t)
	calls := 0
	answerWithdrawals(m, q, &calls, nil)
	present := map[string]bool{familyIPv6: true}
	start := time.Now().UTC()
	// Fifty inconclusive passes over two hours: the IPv4 family is not visible, but nothing
	// asked for it either.
	for i := range 50 {
		if e := m.withdrawLostFamilies(ctx, q, authority, present, map[string]bool{familyIPv4: false, familyIPv6: true}, start.Add(time.Duration(i)*150*time.Second)); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 0 {
		t.Fatal("a family nothing looked for was withdrawn", calls)
	}
	if _, ok := absenceRow(t, f, authority, familyIPv4); ok {
		t.Fatal("an inconclusive pass was counted as an absence")
	}
	// Now the observer does run and finds nothing, three times across the window.
	conclusive := map[string]bool{familyIPv4: true, familyIPv6: true}
	for i := range 3 {
		if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(3*time.Hour).Add(time.Duration(i)*6*time.Minute)); e != nil {
			t.Fatal(e)
		}
	}
	// An inconclusive pass in the middle leaves the count alone rather than restarting it.
	if e := m.withdrawLostFamilies(ctx, q, authority, present, map[string]bool{familyIPv6: true}, start.Add(4*time.Hour)); e != nil {
		t.Fatal(e)
	}
	if observations, ok := absenceRow(t, f, authority, familyIPv4); !ok || observations != 3 {
		t.Fatal("an inconclusive pass disturbed a real absence", observations, ok)
	}
	if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(5*time.Hour)); e != nil {
		t.Fatal(e)
	}
	if calls != 1 || reportedAddresses(t, f, authority)[familyIPv4] != "" {
		t.Fatal("the IPv4 address was not withdrawn once it was certain", calls)
	}
}

// A server that can see nothing at all keeps what it has. Withdrawing the last family would
// take a name out of DNS entirely, which helps nobody and is exactly what a dropped link, a
// sleeping router or an unplugged cable looks like.
func TestTheLastFamilyIsNeverWithdrawn(t *testing.T) {
	f, m, q, authority, ctx := withdrawalFixture(t)
	calls := 0
	answerWithdrawals(m, q, &calls, nil)
	conclusive := map[string]bool{familyIPv4: true, familyIPv6: true}
	start := time.Now().UTC()
	for i := range 20 {
		if e := m.withdrawLostFamilies(ctx, q, authority, map[string]bool{}, conclusive, start.Add(time.Duration(i)*time.Hour)); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 0 {
		t.Fatal("a server with no addresses withdrew its last ones", calls)
	}
	addresses := reportedAddresses(t, f, authority)
	if addresses[familyIPv4] == "" || addresses[familyIPv6] == "" {
		t.Fatal("a total outage emptied the reported addresses", addresses)
	}
}

// Hosted being unreachable is not a reason to forget what it was told. The call is retried,
// and the name stays until Hosted actually accepts the withdrawal.
func TestAFailedWithdrawalKeepsTheNameAndIsRetried(t *testing.T) {
	f, m, q, authority, ctx := withdrawalFixture(t)
	calls := 0
	answerWithdrawals(m, q, &calls, errors.New("hosted unreachable"))
	present := map[string]bool{familyIPv4: true}
	conclusive := map[string]bool{familyIPv4: true, familyIPv6: true}
	start := time.Now().UTC()
	for i := range 3 {
		if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(time.Duration(i)*6*time.Minute)); e != nil {
			t.Fatal(e)
		}
	}
	if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(time.Hour)); e == nil {
		t.Fatal("an unreachable Hosted reported success")
	}
	if reportedAddresses(t, f, authority)[familyIPv6] == "" {
		t.Fatal("a failed withdrawal forgot the name anyway")
	}
	if observations, ok := absenceRow(t, f, authority, familyIPv6); !ok || observations < withdrawObservations {
		t.Fatal("a failed withdrawal lost its count", observations, ok)
	}
	// A reply that names another server's namespace is refused: a withdrawal receipt is only
	// evidence when it is about this server.
	m.h.certificates.transport.client.Transport = claimRoundTrip(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, `{"namespace":"ptc-zzzzzzzzzzzzzzzzzzzz","family":"ipv6","withdrawn":true}`), nil
	})
	if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(90*time.Minute)); !errors.Is(e, ErrInvalid) {
		t.Fatal("a receipt for another namespace was accepted", e)
	}
	if reportedAddresses(t, f, authority)[familyIPv6] == "" {
		t.Fatal("a foreign receipt forgot the name")
	}
	answerWithdrawals(m, q, &calls, nil)
	if e := m.withdrawLostFamilies(ctx, q, authority, present, conclusive, start.Add(2*time.Hour)); e != nil {
		t.Fatal(e)
	}
	if calls != 1 || reportedAddresses(t, f, authority)[familyIPv6] != "" {
		t.Fatal("the retry did not complete the withdrawal", calls)
	}
}

// Nothing is tracked for a family that was never reported: there is no row to withdraw, so an
// IPv4-only server does no work here at all.
func TestAFamilyThatWasNeverReportedIsNeverTracked(t *testing.T) {
	f, m, q, authority, ctx := withdrawalFixture(t)
	if _, e := f.db.Exec(`DELETE FROM networking_route_names WHERE instr(address,':')>0`); e != nil {
		t.Fatal(e)
	}
	calls := 0
	answerWithdrawals(m, q, &calls, nil)
	start := time.Now().UTC()
	for i := range 10 {
		if e := m.withdrawLostFamilies(ctx, q, authority, map[string]bool{familyIPv4: true}, map[string]bool{familyIPv4: true, familyIPv6: true}, start.Add(time.Duration(i)*time.Hour)); e != nil {
			t.Fatal(e)
		}
	}
	if calls != 0 {
		t.Fatal("an unreported family was withdrawn", calls)
	}
	if _, ok := absenceRow(t, f, authority, familyIPv6); ok {
		t.Fatal("an unreported family was tracked")
	}
}

func TestFamilyClassificationAndObservation(t *testing.T) {
	families := publishedFamilies(map[string]int{"188.68.34.120": 32500, "2a03:4000:6:8::1": 32500, "not an address": 1})
	if !families[familyIPv4] || !families[familyIPv6] || len(families) != 2 {
		t.Fatal(families)
	}
	if otherFamily(familyIPv4) != familyIPv6 || otherFamily(familyIPv6) != familyIPv4 {
		t.Fatal("family pairing")
	}
	if !hasPublicIPv4(Topology{Public: []string{"188.68.34.120"}}) || hasPublicIPv4(Topology{Public: []string{"2a03:4000:6:8::1"}}) || hasPublicIPv4(Topology{}) {
		t.Fatal("public IPv4 detection")
	}
	only4 := map[string]bool{familyIPv4: true}
	if observeFamily(familyIPv4, only4, true) != familyPresent {
		t.Fatal("a present family was not seen")
	}
	if observeFamily(familyIPv6, only4, true) != familyAbsent {
		t.Fatal("a conclusive absence was not recorded")
	}
	if observeFamily(familyIPv6, only4, false) != familyUnknown {
		t.Fatal("an inconclusive pass was treated as an absence")
	}
	if observeFamily(familyIPv6, map[string]bool{}, true) != familyUnknown {
		t.Fatal("the last family was treated as an absence")
	}
}
