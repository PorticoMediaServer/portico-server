package httpapi

import (
	"portico.local/server/internal/httpapi/fixture"
	"testing"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// The principal cache is the one cache in this server that could do real harm:
// hold an entry a moment too long and a revoked session keeps working. These are
// the properties that stop it, each asserted rather than argued.
func TestPrincipalCacheIsFencedAgainstRevocation(t *testing.T) {
	tier := performanceTier{name: "auth", shape: fixture.Tiny(), concurrentViewers: 1, iterations: 1, maximumP95: time.Minute, maximumP99: time.Minute, allowBoundedOverload: true}
	f := newLoadFixture(t, tier)
	token := f.viewers[0].AccessToken

	// It saves work: the second identical request resolves from the cache.
	ResetRouteCosts()
	if code, _, body := f.callTimed(token, "GET", "/v1/items?limit=5", nil); code != 200 {
		t.Fatalf("first %d %s", code, body)
	}
	first := RouteCostFor("GET /v1/items").Statements
	ResetRouteCosts()
	if code, _, body := f.callTimed(token, "GET", "/v1/items?limit=5", nil); code != 200 {
		t.Fatalf("second %d %s", code, body)
	}
	second := RouteCostFor("GET /v1/items").Statements
	t.Logf("GET /v1/items statements: %d uncached, %d cached", first, second)
	if second >= first {
		t.Fatalf("the second request cost %d statements against %d; the principal was not cached", second, first)
	}

	// A revocation lands, and the very next request is refused. The write is a
	// bare family UPDATE, not a gated security-fence transaction, so this is
	// the SQL-marker half of the two-signal discipline on its own.
	if _, err := f.db.Exec(`UPDATE authorization_session_families SET revoked=1 WHERE id=(SELECT family_id FROM authorization_family_tokens WHERE token_hash=?)`, identity.Digest(token)); err != nil {
		t.Fatal(err)
	}
	if code, _, _ := f.callTimed(token, "GET", "/v1/items?limit=5", nil); code != 401 {
		t.Fatalf("a revoked session was answered %d from the cache", code)
	}

	// A denial is never cached, so restoring the family can authenticate again.
	if _, err := f.db.Exec(`UPDATE authorization_session_families SET revoked=0 WHERE id IN(SELECT family_id FROM authorization_family_tokens WHERE token_hash=?)`, identity.Digest(token)); err != nil {
		t.Fatal(err)
	}
	if code, _, body := f.callTimed(token, "GET", "/v1/items?limit=5", nil); code != 200 {
		t.Fatalf("a restored session was answered %d %s", code, body)
	}

	// The security-fence lane never reads the cache. `/v1/me` is on it, so its
	// cost does not fall on a repeat.
	ResetRouteCosts()
	f.callTimed(token, "GET", "/v1/me", nil)
	strictFirst := RouteCostFor("GET /v1/me").Statements
	ResetRouteCosts()
	f.callTimed(token, "GET", "/v1/me", nil)
	strictSecond := RouteCostFor("GET /v1/me").Statements
	if strictSecond < strictFirst {
		t.Fatalf("an authority-lane request was served from the principal cache (%d then %d statements)", strictFirst, strictSecond)
	}

	// An unknown token is refused every time and leaves nothing behind.
	for i := 0; i < 2; i++ {
		if code, _, _ := f.callTimed("not-a-real-token-that-is-long-enough-to-be-plausible-0000", "GET", "/v1/items", nil); code != 401 {
			t.Fatalf("an unknown token was answered %d", code)
		}
	}
}

// A revocation that commits while a check is still running must not be outlived by that
// check's answer: the entry is stamped with the generation read before the check began.
func TestAuthorityCachesDoNotOutliveARevocationThatRacedTheCheck(t *testing.T) {
	now := time.Now()
	before := dbwork.AuthorityGeneration()
	// ... the check reads "allowed" here, then the revocation commits ...
	dbwork.BumpAuthority()

	principals := newPrincipalCache()
	principals.store("digest", identity.Principal{}, now, before)
	if _, ok := principals.lookup("digest", now); ok {
		t.Fatal("a principal resolved before a revocation was served after it")
	}
	access := newAccessCache()
	access.grant("grant", now, before)
	if access.allowed("grant", now) {
		t.Fatal("a library grant resolved before a revocation was served after it")
	}
	restrictions := newRestrictionCache()
	restrictions.store("profile", identity.ContentRestrictions{}, now, before)
	if _, ok := restrictions.lookup("profile", now); ok {
		t.Fatal("restrictions read before a restriction change were served after it")
	}
	// Stamped with the current generation, the same entries are served.
	current := dbwork.AuthorityGeneration()
	principals.store("digest", identity.Principal{}, now, current)
	if _, ok := principals.lookup("digest", now); !ok {
		t.Fatal("a current entry was not served")
	}
}
