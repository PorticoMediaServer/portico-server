package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// PERF-12: grid and row projections answer a revalidation with 304.
//
// The validator is the published state plus the viewer's scope. The published
// state is this process's count of committed write transactions (every
// catalogue publication, personal-state change, restriction or policy change
// and sign-out is one) under a per-process nonce, so a restart never reuses a
// tag. Any write at all changes the tag, which is conservative: during a scan
// grids revalidate to 200, and an idle library answers 304 without composing
// the page. The count is read before the page is, so a page that already
// includes a later write carries an older tag and can never be answered 304
// after that write.
//
// The time bucket bounds how long a cached body (and the continuation cursor
// inside it, which expires after 30 minutes) can be reused.
var gridProcessNonce = func() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}()

const gridTagBucket = 5 * time.Minute

// gridTag is computed after the request is authorized and before the page is
// read. extra folds any further facts the caller's bytes depend on.
func gridTag(r *http.Request, p identity.Principal, bucket time.Duration, extra ...string) string {
	parts := []string{"grid", gridProcessNonce, strconv.FormatUint(dbwork.Publications(), 10), r.URL.Path, canonicalQuery(r.URL.Query()), viewerScope(p), p.Hash, strconv.Itoa(p.Epoch), identity.PersonalKey(p.Viewer), strconv.FormatInt(time.Now().UTC().UnixNano()/int64(bucket), 10)}
	return responseTag(append(parts, extra...)...)
}

// canonicalQuery orders the query so parameter order does not split the cache.
func canonicalQuery(q url.Values) string { return q.Encode() }

// gridNotModified answers 304 when the client's validator matches. It sets no
// header otherwise: the validator goes only on a successful page (setGridTag),
// never on an error response.
func gridNotModified(w http.ResponseWriter, r *http.Request, tag string) bool {
	quoted := `"` + tag + `"`
	if !matchesETag(r.Header.Get("If-None-Match"), quoted) {
		return false
	}
	return conditional(w, r, tag)
}

// setGridTag marks a successful page with its validator.
func setGridTag(w http.ResponseWriter, tag string) {
	w.Header().Set("ETag", `"`+tag+`"`)
	w.Header().Set("Cache-Control", revalidate)
}
