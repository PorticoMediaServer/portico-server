package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"

	"portico.local/hosted/internal/directory"
	"portico.local/hosted/internal/directroute"
)

type feedRepo struct {
	after     int64
	withdrawn string
}

func (r *feedRepo) RecordDirectAddress(context.Context, directory.ServerAuthority, netip.Addr) (string, bool, error) {
	return "", false, directory.ErrDenied
}
func (r *feedRepo) WithdrawDirectAddress(_ context.Context, _ directory.ServerAuthority, family string) (string, bool, error) {
	r.withdrawn = family
	return "ptc-aaaaaaaaaaaaaaaaaaaa", true, nil
}
func (r *feedRepo) DirectAddressChanges(_ context.Context, after int64, _ int) ([]directroute.AddressChange, error) {
	r.after = after
	return []directroute.AddressChange{{Sequence: after + 1, Namespace: "ptc-aaaaaaaaaaaaaaaaaaaa", IPv4: "156.57.135.50"}}, nil
}

// The feed is read through the same pipeline as every request, which refuses query
// parameters by default and sheds load: it must pass both, and only with the secret.
func TestDNSFeedThroughTheRequestPipeline(t *testing.T) {
	secret := strings.Repeat("s", 44)
	repo := &feedRepo{}
	issuer, e := directroute.New(repo, []byte(secret))
	if e != nil {
		t.Fatal(e)
	}
	h := (&Handler{DirectRoutes: issuer}).Routes()
	for _, authorization := range []string{"", "Bearer wrong", secret} {
		r := httptest.NewRequest("GET", "/v1/dns/addresses?after=41&limit=5000", nil)
		if authorization != "" {
			r.Header.Set("Authorization", authorization)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal("the feed answered without the secret", authorization, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "/v1/dns/addresses?after=41&limit=5000", nil)
	r.Header.Set("Authorization", "Bearer "+secret)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var body struct {
		Items []directroute.AddressChange `json:"items"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Items) != 1 || body.Items[0].Sequence != 42 || repo.after != 41 {
		t.Fatal(w.Code, w.Body.String(), repo.after)
	}
	etag := w.Header().Get("ETag")
	if etag != `"42"` {
		t.Fatal("the feed did not name its head", etag)
	}
	// A repeat with If-None-Match is a 304 answered from memory: the store sees no second
	// read (repo.after stays 41).
	w = httptest.NewRecorder()
	againReq := httptest.NewRequest("GET", "/v1/dns/addresses?after=42&limit=5000", nil)
	againReq.Header.Set("Authorization", "Bearer "+secret)
	againReq.Header.Set("If-None-Match", etag)
	h.ServeHTTP(w, againReq)
	if w.Code != 304 || repo.after != 41 {
		t.Fatal("the unchanged feed did not 304 without a database read", w.Code, repo.after)
	}
	bad := httptest.NewRequest("GET", "/v1/dns/addresses?after=x", nil)
	bad.Header.Set("Authorization", "Bearer "+secret)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, bad)
	if w.Code != 400 {
		t.Fatal(w.Code)
	}
	// Withdrawal is registered on the same pipeline and is a server call: an unauthenticated
	// one is refused before anything reaches the store.
	withdraw := httptest.NewRequest("POST", "/v1/servers/srv_1/direct-route/withdraw", strings.NewReader(`{"family":"ipv6"}`))
	withdraw.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, withdraw)
	if w.Code != 401 && w.Code != 403 {
		t.Fatal("an unauthenticated withdrawal was accepted", w.Code, w.Body.String())
	}
	if repo.withdrawn != "" {
		t.Fatal("an unauthenticated withdrawal reached the store", repo.withdrawn)
	}
}

func (r *feedRepo) DirectRouteLabel(context.Context, string) (string, error) {
	return "0123456789abcdef0123456789abcdef", nil
}
