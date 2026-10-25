package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"portico.local/hosted/internal/identity"
	"strings"
	"testing"
	"time"
)

// A11: every budget class has its own fixed table, cycling through many keys
// resets no one's count, and a client cycling /64s inside one /48 runs out of
// the network budget.
func TestBudgetIsolationAndNoResetByChurn(t *testing.T) {
	h := &Handler{}
	// Churn must be measured inside one fixed window. The production wrapper
	// reads time.Now for every call and may legitimately cross a minute here.
	now := time.Date(2026, time.October, 25, 12, 0, 0, 0, time.UTC)
	allow := func(key string, limit int) bool { return h.budgets.allow(key, limit, now) }
	for i := 0; i < 31; i++ {
		allow("auth:/v1/sessions:"+budgetIP("2001:db8:9::1"), 30)
	}
	if allow("auth:/v1/sessions:"+budgetIP("2001:db8:9::1"), 30) {
		t.Fatal("limit not applied")
	}
	for i := 0; i < 200000; i++ {
		allow(fmt.Sprint("auth:/v1/sessions:churn-", i), 30)
	}
	if allow("auth:/v1/sessions:"+budgetIP("2001:db8:9::1"), 30) {
		t.Fatal("churning other keys reset the attacker's count")
	}
	if !allow("auth:/v1/sessions:"+budgetIP("2001:db8:2::1"), 30) {
		t.Fatal("other network blocked")
	}
	for i := 0; i < 200000; i++ {
		allow(fmt.Sprint("refresh:churn-", i), 120)
	}
	if !allow("account:acct_1", 240) || !allow("server:srv_1", 120) {
		t.Fatal("unauthenticated churn reached the account or server budget")
	}
	blocked := false
	for i := 0; i < 400; i++ {
		r := httptest.NewRequest("POST", "/v1/sessions", nil)
		r.RemoteAddr = fmt.Sprintf("[2001:db8:1:%x::1]:443", i)
		if !h.allowAuth(r) {
			blocked = true
			break
		}
	}
	if !blocked {
		t.Fatal("cycling /64s inside one /48 never hit the network budget")
	}
	if len(h.budgets.classes) > 8 {
		t.Fatal("classes grow with keys")
	}
}
func TestBrowserRefreshCSRF(t *testing.T) {
	h := &Handler{Origins: map[string]bool{"https://web.getportico.tv": true}}
	r := httptest.NewRequest("POST", "https://web.getportico.tv/v1/sessions", nil)
	r.Header.Set("Origin", "https://web.getportico.tv")
	w := httptest.NewRecorder()
	h.sessionResponse(w, r, 200, identity.Session{Mode: "browser", RefreshToken: "refresh"})
	cookies := w.Result().Cookies()
	r2 := r.Clone(r.Context())
	for _, c := range cookies {
		if c.SameSite != http.SameSiteLaxMode {
			t.Fatal("cross-site cookie")
		}
		r2.AddCookie(c)
	}
	if _, _, ok := h.refreshInput(httptest.NewRecorder(), r2, ""); ok {
		t.Fatal("accepted missing CSRF")
	}
	r2.Header.Set("X-CSRF-Token", strings.Repeat("a", 43))
	if _, _, ok := h.refreshInput(httptest.NewRecorder(), r2, ""); ok {
		t.Fatal("accepted wrong CSRF")
	}
	r2.Header.Set("X-CSRF-Token", w.Header().Get("X-CSRF-Token"))
	if _, _, ok := h.refreshInput(httptest.NewRecorder(), r2, ""); !ok {
		t.Fatal("rejected matched CSRF")
	}
}
