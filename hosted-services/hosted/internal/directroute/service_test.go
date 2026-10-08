package directroute

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"testing"

	"portico.local/dns/directdns"
	"portico.local/hosted/internal/claims"
	"portico.local/hosted/internal/directory"
)

const testNamespace = "ptc-aaaaaaaaaaaaaaaaaaaa"

type fakeRepo struct {
	fail      error
	calls     int
	recorded  []netip.Addr
	withdrawn []string
	hasFamily map[string]bool
	namespace string
}

func (r *fakeRepo) namespaceOrDefault() string {
	if r.namespace != "" {
		return r.namespace
	}
	return testNamespace
}

func (r *fakeRepo) WithdrawDirectAddress(ctx context.Context, a directory.ServerAuthority, family string) (string, bool, error) {
	r.calls++
	if r.fail != nil {
		return "", false, r.fail
	}
	r.withdrawn = append(r.withdrawn, family)
	if r.hasFamily == nil {
		return r.namespaceOrDefault(), true, nil
	}
	had := r.hasFamily[family]
	r.hasFamily[family] = false
	return r.namespaceOrDefault(), had, nil
}

func (r *fakeRepo) RecordDirectAddress(ctx context.Context, a directory.ServerAuthority, address netip.Addr) (string, bool, error) {
	r.calls++
	if r.fail != nil {
		return "", false, r.fail
	}
	r.recorded = append(r.recorded, address)
	return testNamespace, true, nil
}
func (r *fakeRepo) DirectAddressChanges(ctx context.Context, after int64, limit int) ([]AddressChange, error) {
	return []AddressChange{{Sequence: after + 1, Namespace: testNamespace, IPv4: "188.68.34.120"}}, nil
}
func authority() directory.ServerAuthority {
	public := bytes.Repeat([]byte{7}, 32)
	id, _ := claims.ServerID(public)
	return directory.ServerAuthority{ServerID: id, AccountID: "account", OperationID: "operation", PublicKey: public, TokenHash: bytes.Repeat([]byte{1}, 32), ClaimGeneration: 1, CredentialGeneration: 1}
}
func TestIssueRecordsTheAddressAndNamesThePermanentRoute(t *testing.T) {
	repo := &fakeRepo{}
	s, e := New(repo, bytes.Repeat([]byte{8}, 32))
	if e != nil {
		t.Fatal(e)
	}
	a := authority()
	result, e := s.Issue(context.Background(), a, "188.68.34.120")
	if e != nil {
		t.Fatal(e)
	}
	want := directdns.CurrentHostname(testNamespace, Zone, "0123456789abcdef0123456789abcdef")
	if result.Hostname != want || result.BaseURL != "https://"+want+":32500" || result.CertificateDNSName != "*."+testNamespace+"."+Zone || result.DNS01Target != testNamespace+".acme.getportico.tv" || result.Namespace != testNamespace {
		t.Fatal(result)
	}
	if len(repo.recorded) != 1 || repo.recorded[0] != netip.MustParseAddr("188.68.34.120") {
		t.Fatal("the reported address was not recorded", repo.recorded)
	}
	// The name is covered by the server's one wildcard: exactly one label under the namespace.
	if got, ok := directdns.NamespaceFromCurrentHostname(result.Hostname, Zone); !ok || got != testNamespace {
		t.Fatal(got, ok)
	}
	repo.fail = directory.ErrDenied
	if result, e = s.Issue(context.Background(), a, "188.68.34.120"); !errors.Is(e, directory.ErrDenied) || result != (Route{}) {
		t.Fatal("denied authority leaked a route", e)
	}
}
func TestFeedRequiresItsSecret(t *testing.T) {
	if _, e := New(&fakeRepo{}, []byte("short")); e == nil {
		t.Fatal("a weak feed secret was accepted")
	}
	secret := bytes.Repeat([]byte{8}, 32)
	s, _ := New(&fakeRepo{}, secret)
	if s.FeedAuthorized("") || s.FeedAuthorized("wrong") || !s.FeedAuthorized(string(secret)) {
		t.Fatal("feed authorization")
	}
	if _, e := s.Changes(context.Background(), -1, 10); e == nil {
		t.Fatal("a negative cursor was accepted")
	}
	if rows, e := s.Changes(context.Background(), 41, 0); e != nil || len(rows) != 1 || rows[0].Sequence != 42 {
		t.Fatal(rows, e)
	}
}
func TestRefusesWrongIdentityAndUnsafeAddresses(t *testing.T) {
	repo := &fakeRepo{}
	s, _ := New(repo, bytes.Repeat([]byte{8}, 32))
	a := authority()
	for _, ip := range []string{"127.0.0.1", "10.0.0.1", "169.254.1.1", "::1", "::ffff:188.68.34.120", "0.0.0.0", "224.0.0.1", "188.68.34.120:443"} {
		if _, e := s.Issue(context.Background(), a, ip); e == nil {
			t.Fatal("unsafe address", ip)
		}
	}
	a.PublicKey[0]++
	if _, e := s.Issue(context.Background(), a, "188.68.34.120"); !errors.Is(e, directory.ErrDenied) {
		t.Fatal("wrong key identity accepted")
	}
	if repo.calls != 0 {
		t.Fatal("invalid input reached authority store")
	}
}

func TestAddressAndPortChangeReuseIndependentNamespace(t *testing.T) {
	s, e := New(&fakeRepo{}, bytes.Repeat([]byte{8}, 32))
	if e != nil {
		t.Fatal(e)
	}
	a := authority()
	one, e := s.Issue(context.Background(), a, "8.8.8.8")
	if e != nil {
		t.Fatal(e)
	}
	two, e := s.IssuePort(context.Background(), a, "2606:4700:4700::1111", 4443)
	if e != nil {
		t.Fatal(e)
	}
	if one.Namespace != two.Namespace || one.CertificateDNSName != two.CertificateDNSName || one.Hostname != two.Hostname || two.BaseURL != "https://"+two.Hostname+":4443" {
		t.Fatal("an address or port change changed the permanent name or certificate identity")
	}
	for _, p := range []int{0, -1, 65536} {
		if _, e = s.IssuePort(context.Background(), a, "8.8.8.8", p); e == nil {
			t.Fatal("invalid port")
		}
	}
}

// A lost address family is withdrawn once, and a second withdrawal changes nothing. Until
// this existed the address stayed in the table for ever and the DNS nodes answered it.
func TestWithdrawClearsOneFamilyAndIsIdempotent(t *testing.T) {
	repo := &fakeRepo{hasFamily: map[string]bool{FamilyIPv4: true, FamilyIPv6: true}}
	s, e := New(repo, bytes.Repeat([]byte{8}, 32))
	if e != nil {
		t.Fatal(e)
	}
	a := authority()
	namespace, changed, e := s.Withdraw(context.Background(), a, FamilyIPv6)
	if e != nil || namespace != testNamespace || !changed {
		t.Fatal(namespace, changed, e)
	}
	// The other family is untouched: this is a withdrawal, not a reset.
	if !repo.hasFamily[FamilyIPv4] {
		t.Fatal("withdrawing IPv6 cleared IPv4")
	}
	if _, changed, e = s.Withdraw(context.Background(), a, FamilyIPv6); e != nil || changed {
		t.Fatal("a repeated withdrawal reported a change", changed, e)
	}
	if len(repo.withdrawn) != 2 || repo.withdrawn[0] != FamilyIPv6 || repo.withdrawn[1] != FamilyIPv6 {
		t.Fatal(repo.withdrawn)
	}
}

func TestWithdrawRefusesUnknownFamiliesAndWrongIdentity(t *testing.T) {
	repo := &fakeRepo{}
	s, _ := New(repo, bytes.Repeat([]byte{8}, 32))
	a := authority()
	for _, family := range []string{"", "ipv5", "IPv6", "ipv4;DROP", "*"} {
		if _, _, e := s.Withdraw(context.Background(), a, family); !errors.Is(e, ErrInvalid) {
			t.Fatal("accepted family", family, e)
		}
	}
	if repo.calls != 0 {
		t.Fatal("an invalid family reached the store")
	}
	wrong := authority()
	wrong.PublicKey[0]++
	if _, _, e := s.Withdraw(context.Background(), wrong, FamilyIPv6); !errors.Is(e, directory.ErrDenied) {
		t.Fatal("wrong key identity accepted", e)
	}
	empty := directory.ServerAuthority{}
	if _, _, e := s.Withdraw(context.Background(), empty, FamilyIPv6); !errors.Is(e, directory.ErrDenied) {
		t.Fatal("empty authority accepted", e)
	}
	if repo.calls != 0 {
		t.Fatal("a denied request reached the store")
	}
	repo.fail = directory.ErrDenied
	if _, _, e := s.Withdraw(context.Background(), a, FamilyIPv4); !errors.Is(e, directory.ErrDenied) {
		t.Fatal(e)
	}
}

// The feed carries a withdrawal as an ordinary row with the family blank, which is what the
// DNS table reads as "drop it".
func TestAWithdrawnFamilyLeavesTheFeedRowBlank(t *testing.T) {
	rows := []AddressChange{{Sequence: 9, Namespace: testNamespace, IPv4: "188.68.34.120"}}
	raw, e := json.Marshal(rows[0])
	if e != nil {
		t.Fatal(e)
	}
	var decoded directdns.FeedRow
	if e = json.Unmarshal(raw, &decoded); e != nil {
		t.Fatal(e)
	}
	if decoded.IPv6 != "" || decoded.IPv4 != "188.68.34.120" || decoded.Sequence != 9 {
		t.Fatal(decoded)
	}
	table, e := directdns.OpenTable("")
	if e != nil {
		t.Fatal(e)
	}
	if e = table.Apply([]directdns.FeedRow{{Sequence: 8, Namespace: testNamespace, IPv4: "188.68.34.120", IPv6: "2a03:4000:6:8::1"}}); e != nil {
		t.Fatal(e)
	}
	if e = table.Apply([]directdns.FeedRow{decoded}); e != nil {
		t.Fatal(e)
	}
	addresses, ok := table.Lookup(testNamespace)
	if !ok || addresses.IPv6.IsValid() || !addresses.IPv4.IsValid() {
		t.Fatal("the withdrawn family survived the feed", addresses, ok)
	}
}

func (r *fakeRepo) DirectRouteLabel(context.Context, string) (string, error) {
	return "0123456789abcdef0123456789abcdef", nil
}

func TestUnprovenAddressFailsBeforePublication(t *testing.T) {
	s, _ := New(&fakeRepo{}, make([]byte, 32))
	if _, err := s.IssuePortFrom(context.Background(), directory.ServerAuthority{}, "8.8.8.8", 443, "1.1.1.1"); !errors.Is(err, ErrUnverified) {
		t.Fatalf("got %v", err)
	}
}

// An unchanged feed answers 304 from the in-memory head with no second database read, and
// even a client without If-None-Match gets its empty page from the cache.
func TestChangesWithCacheAnswers304WithoutADatabaseRead(t *testing.T) {
	repo := &fakeRepo{}
	s, _ := New(repo, bytes.Repeat([]byte{8}, 32))
	ctx := context.Background()
	items, etag, notModified, e := s.ChangesWithCache(ctx, 41, 5000, "")
	if e != nil || notModified || len(items) != 1 || items[0].Sequence != 42 || etag != `"42"` || repo.calls != 0 {
		// Note: fakeRepo.Record/Withdraw count calls, but DirectAddressChanges does not; use
		// a counting wrapper below for the DB-call assertion. Here just check the shape.
		t.Fatal(items, etag, notModified, e)
	}
	counting := &countingRepo{inner: repo}
	s2, _ := New(counting, bytes.Repeat([]byte{8}, 32))
	if _, _, _, e = s2.ChangesWithCache(ctx, 41, 5000, ""); e != nil || counting.calls != 1 {
		t.Fatal("first read must reach the database", counting.calls, e)
	}
	if _, etag, notModified, e = s2.ChangesWithCache(ctx, 42, 5000, `"42"`); e != nil || !notModified || etag != `"42"` || counting.calls != 1 {
		t.Fatal("304 must cost no database read", etag, notModified, counting.calls, e)
	}
	if items, etag, notModified, e = s2.ChangesWithCache(ctx, 42, 5000, ""); e != nil || notModified || len(items) != 0 || etag != `"42"` || counting.calls != 1 {
		t.Fatal("empty page must also avoid the database", items, etag, notModified, counting.calls, e)
	}
}

type countingRepo struct {
	inner *fakeRepo
	calls int
}

func (r *countingRepo) RecordDirectAddress(ctx context.Context, a directory.ServerAuthority, address netip.Addr) (string, bool, error) {
	return r.inner.RecordDirectAddress(ctx, a, address)
}
func (r *countingRepo) WithdrawDirectAddress(ctx context.Context, a directory.ServerAuthority, family string) (string, bool, error) {
	return r.inner.WithdrawDirectAddress(ctx, a, family)
}
func (r *countingRepo) DirectRouteLabel(ctx context.Context, namespace string) (string, error) {
	return r.inner.DirectRouteLabel(ctx, namespace)
}
func (r *countingRepo) DirectAddressChanges(ctx context.Context, after int64, limit int) ([]AddressChange, error) {
	r.calls++
	return r.inner.DirectAddressChanges(ctx, after, limit)
}

// A full page never establishes the head: the next read must go back to the database even
// when its ETag matches the last row, or pagination would stop early and lose rows.
func TestTruncatedPageNever304FromCache(t *testing.T) {
	repo := &truncatedRepo{}
	s, _ := New(repo, bytes.Repeat([]byte{8}, 32))
	ctx := context.Background()
	items, etag, notModified, e := s.ChangesWithCache(ctx, 0, 2, "")
	if e != nil || notModified || len(items) != 2 || etag != `"2"` || repo.calls != 1 {
		t.Fatal(items, etag, notModified, repo.calls, e)
	}
	if _, _, notModified, e = s.ChangesWithCache(ctx, 2, 2, `"2"`); e != nil || notModified || repo.calls != 2 {
		t.Fatal("a truncated head must not 304 from cache", notModified, repo.calls, e)
	}
}

type truncatedRepo struct {
	fakeRepo
	calls int
}

func (r *truncatedRepo) DirectAddressChanges(_ context.Context, after int64, limit int) ([]AddressChange, error) {
	r.calls++
	out := make([]AddressChange, limit)
	for i := range out {
		out[i] = AddressChange{Sequence: after + int64(i) + 1, Namespace: testNamespace, IPv4: "188.68.34.120"}
	}
	return out, nil
}

// A service-observed address write invalidates the head, so the next poll picks up the new
// sequence within one interval (a newly claimed server resolves within about a minute).
func TestWriteInvalidatesFeedHeadCache(t *testing.T) {
	repo := &countingRepo{inner: &fakeRepo{}}
	s, _ := New(repo, bytes.Repeat([]byte{8}, 32))
	ctx := context.Background()
	if _, _, _, e := s.ChangesWithCache(ctx, 41, 5000, ""); e != nil || repo.calls != 1 {
		t.Fatal(e, repo.calls)
	}
	if _, _, notModified, e := s.ChangesWithCache(ctx, 42, 5000, `"42"`); e != nil || !notModified || repo.calls != 1 {
		t.Fatal("expected a cached 304", notModified, repo.calls, e)
	}
	if _, e := s.Issue(ctx, authority(), "188.68.34.120"); e != nil {
		t.Fatal(e)
	}
	items, _, notModified, e := s.ChangesWithCache(ctx, 42, 5000, `"42"`)
	if e != nil || notModified || repo.calls != 2 || len(items) != 1 || items[0].Sequence != 43 {
		t.Fatal("a write must force the next read to the database", items, notModified, repo.calls, e)
	}
}
