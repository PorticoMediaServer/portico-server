package httpapi

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"testing"
)

// SEC-02 / BE-SRV-01 / BE-MEDIA-04 / ARCH-SRV-08, part two: the rest of the
// route table, and existence.
//
// TestRestrictedProfileRouteMatrix proves that no GET route shows a restricted
// profile a hidden identifier. Three things it can't see are proven here:
//
//  1. Every non-GET route is classified. The walk is over routeLanes (which
//     TestEveryRegisteredRouteHasALane proves complete), so a new write route
//     fails until someone decides its restriction story. Owner administration
//     (/v1/admin/…) is not listed by hand: every such route is called as the
//     restricted member and must refuse.
//  2. Hidden ≡ absent. A title, container or library the profile may not see
//     must be indistinguishable from one that doesn't exist: every probe that
//     names a hidden id is sent again with an id that doesn't exist, and the
//     status and error code must match. This runs over the read matrix's GET
//     probes as well as the write probes, so no route can answer 403 for "exists
//     but restricted" where an absent id answers 404.
//  3. No effect. A refused write changes nothing: after every probe, no row
//     anywhere in the database references a hidden id that didn't before.
func TestRestrictedProfileWriteRouteMatrix(t *testing.T) {
	m := buildRestrictionMatrix(t)
	routes := restrictionWriteRoutes()

	// 1. Exhaustive classification of the non-GET routes.
	for pattern := range routeLanes {
		if strings.HasPrefix(pattern, "GET ") || ownerAdministration(pattern) {
			continue
		}
		if _, ok := routes[pattern]; !ok {
			t.Errorf("%s has no restriction classification: add it to restrictionWriteRoutes (restriction_writes_test.go) with probes that name hidden ids, or the reason it can't reach catalogue content", pattern)
		}
	}
	for pattern := range routes {
		if _, ok := routeLanes[pattern]; !ok {
			t.Errorf("restrictionWriteRoutes lists %s, which the router does not register", pattern)
		}
		if ownerAdministration(pattern) {
			t.Errorf("%s is owner administration; it is probed by rule and must not be listed", pattern)
		}
	}

	before := hiddenReferenceDigest(t, m)

	// 2a. Owner administration refuses the member outright, whatever it names.
	admin := []string{}
	for pattern := range routeLanes {
		if ownerAdministration(pattern) {
			admin = append(admin, pattern)
		}
	}
	sort.Strings(admin)
	for _, pattern := range admin {
		method, path, _ := strings.Cut(pattern, " ")
		for _, id := range m.hiddenSamples() {
			concrete := pathParameter.ReplaceAllString(path, url.PathEscape(id))
			status, body := m.call(method, concrete, "{}", "restricted")
			if status != 403 && status != 404 {
				t.Errorf("%s: %s %s answered %d to a member profile; owner administration must refuse (403): %s", pattern, method, concrete, status, clip(body))
			}
			if found := m.marker.FindString(body); found != "" {
				t.Errorf("%s: refusal carries a hidden identifier %q", pattern, found)
			}
		}
	}

	// 2b. Content writes: hidden ≡ absent, and never a hidden identifier.
	patterns := make([]string, 0, len(routes))
	for pattern := range routes {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	probed := 0
	for _, pattern := range patterns {
		route := routes[pattern]
		if route.probes == nil {
			if route.reason == "" {
				t.Errorf("%s: an exemption needs a reason", pattern)
			}
			continue
		}
		for _, p := range route.probes(m) {
			probed++
			m.compareHiddenAbsent(t, pattern, p)
		}
	}

	// 2c. The read matrix's GET probes: hidden ≡ absent.
	reads := restrictionRoutes()
	readPatterns := make([]string, 0, len(reads))
	for pattern := range reads {
		readPatterns = append(readPatterns, pattern)
	}
	sort.Strings(readPatterns)
	for _, pattern := range readPatterns {
		route := reads[pattern]
		if route.probe == nil || route.viewer != "" {
			continue
		}
		for _, path := range route.probe(m) {
			for _, id := range m.hiddenIn(path) {
				probed++
				m.compareHiddenAbsent(t, pattern, idProbe{hidden: id, method: "GET", path: path})
			}
		}
	}

	// 3. No effect.
	if after := hiddenReferenceDigest(t, m); after != before {
		t.Errorf("refused writes changed rows that reference hidden ids:\nbefore %s\nafter  %s", before, after)
	}
	t.Logf("%d hidden-vs-absent comparisons over %d classified write routes and %d owner routes", probed, len(routes), len(admin))
}

// ownerAdministration is the rule for owner-only routes: everything under
// /v1/admin/. A member profile is refused before any catalogue read.
func ownerAdministration(pattern string) bool {
	_, path, _ := strings.Cut(pattern, " ")
	return strings.HasPrefix(path, "/v1/admin/")
}

// idProbe is one request that names a hidden id somewhere in its path or
// body. The same request is sent with that id replaced by one that doesn't
// exist; both answers must agree.
type idProbe struct {
	hidden             string
	method, path, body string
}

func (p idProbe) with(id string) idProbe {
	return idProbe{hidden: id, method: p.method, path: strings.ReplaceAll(p.path, p.hidden, id), body: strings.ReplaceAll(p.body, p.hidden, id)}
}

// absentTwin names something with the same shape as id that doesn't exist.
func absentTwin(id string) string { return id + "-absent" }

func (m *restrictionEnv) compareHiddenAbsent(t *testing.T, pattern string, p idProbe) {
	t.Helper()
	hiddenStatus, hiddenBody := m.call(p.method, p.path, p.body, "restricted")
	absent := p.with(absentTwin(p.hidden))
	absentStatus, absentBody := m.call(absent.method, absent.path, absent.body, "restricted")
	if hiddenStatus == 401 {
		t.Errorf("%s: %s %s answered 401 to a valid restricted session: %s", pattern, p.method, p.path, clip(hiddenBody))
	}
	if hiddenStatus >= 500 {
		t.Errorf("%s: %s %s answered %d: %s", pattern, p.method, p.path, hiddenStatus, clip(hiddenBody))
	}
	if found := m.marker.FindString(hiddenBody); found != "" && hiddenStatus >= 400 {
		t.Errorf("%s: %s %s refusal carries a hidden identifier %q: %s", pattern, p.method, p.path, found, clip(hiddenBody))
	}
	hiddenCode, absentCode := bodyErrorCode(hiddenBody), bodyErrorCode(absentBody)
	if hiddenStatus != absentStatus || hiddenCode != absentCode {
		t.Errorf("%s: a hidden id is distinguishable from an absent one: %s %s answered %d %q, the absent twin %d %q", pattern, p.method, p.path, hiddenStatus, hiddenCode, absentStatus, absentCode)
		return
	}
	// Both accepted (a batch or an asynchronous job that skips what it can't
	// reach): the answers must say the same thing about the two ids. Whether
	// anything was written for the hidden one is the no-effect check's job.
	if hiddenStatus < 300 {
		h, a := normalizedAnswer(hiddenBody, p.hidden), normalizedAnswer(absentBody, absent.hidden)
		if h != a {
			t.Errorf("%s: %s %s answers a hidden id differently from an absent one:\nhidden %s\nabsent %s", pattern, p.method, p.path, clip(h), clip(a))
		}
	}
}

// hiddenAnswerVolatile are answer fields that differ between any two requests.
var hiddenAnswerVolatile = map[string]bool{"jobId": true, "batchId": true, "viewerFence": true, "receipt": true, "receipts": true, "revision": true, "operationId": true, "createdAt": true, "updatedAt": true, "at": true, "expiresAt": true, "serverId": true, "requestId": true, "etag": true}

// normalizedAnswer is a JSON answer with the probed id replaced by a
// placeholder and volatile fields dropped, re-encoded canonically.
func normalizedAnswer(body, id string) string {
	var v any
	if json.Unmarshal([]byte(strings.ReplaceAll(body, id, "ID")), &v) != nil {
		return strings.ReplaceAll(body, id, "ID")
	}
	var strip func(any) any
	strip = func(x any) any {
		switch t := x.(type) {
		case map[string]any:
			for k := range t {
				if hiddenAnswerVolatile[k] {
					delete(t, k)
				} else {
					t[k] = strip(t[k])
				}
			}
		case []any:
			for i := range t {
				t[i] = strip(t[i])
			}
		}
		return x
	}
	raw, _ := json.Marshal(strip(v))
	return string(raw)
}

func bodyErrorCode(body string) string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(body), &envelope) == nil {
		return envelope.Error.Code
	}
	return ""
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}

// hiddenSamples is one hidden id of every kind: items, containers and the
// libraries the profile isn't given.
func (m *restrictionEnv) hiddenSamples() []string {
	out := m.hiddenItemSamples()
	kinds := make([]string, 0, len(m.hiddenEntities))
	for kind := range m.hiddenEntities {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		out = append(out, m.hiddenEntities[kind][0])
	}
	return append(out, m.hiddenLibraries()...)
}

func (m *restrictionEnv) hiddenLibraries() []string {
	out := []string{}
	for _, library := range m.libraries {
		for _, marker := range m.markers {
			if marker == library {
				out = append(out, library)
			}
		}
	}
	return out
}

// hiddenIn lists the hidden ids a probe path names (a path segment or a query
// value equal to a hidden id).
func (m *restrictionEnv) hiddenIn(path string) []string {
	hidden := map[string]bool{}
	for _, id := range m.markers {
		hidden[id] = true
	}
	found := map[string]bool{}
	base, query, _ := strings.Cut(path, "?")
	for _, segment := range strings.Split(base, "/") {
		if s, err := url.PathUnescape(segment); err == nil && hidden[s] {
			found[s] = true
		}
	}
	if values, err := url.ParseQuery(query); err == nil {
		for _, list := range values {
			for _, value := range list {
				for _, part := range strings.Split(value, ",") {
					if hidden[part] {
						found[part] = true
					}
				}
			}
		}
	}
	out := make([]string, 0, len(found))
	for id := range found {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// hiddenReferenceDigest counts, for every table column that holds a catalogue
// id, the rows that reference a hidden id. A refused write must not move any
// of these counts.
func hiddenReferenceDigest(t *testing.T, m *restrictionEnv) string {
	t.Helper()
	hidden := mustJSON(m.markers)
	tables, err := m.db.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' AND name NOT LIKE '%_fts%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for tables.Next() {
		var name string
		if err = tables.Scan(&name); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	tables.Close()
	idColumns := map[string]bool{"item_id": true, "media_id": true, "entity_id": true, "show_id": true, "season_id": true, "album_id": true, "artist_id": true, "book_id": true, "container_id": true, "library_id": true, "collection_id": true}
	parts := []string{}
	for _, table := range names {
		columns, err := m.db.Query(`SELECT name FROM pragma_table_info(?)`, table)
		if err != nil {
			t.Fatal(err)
		}
		cols := []string{}
		for columns.Next() {
			var column string
			if err = columns.Scan(&column); err != nil {
				t.Fatal(err)
			}
			if idColumns[column] {
				cols = append(cols, column)
			}
		}
		columns.Close()
		for _, column := range cols {
			var n int
			q := fmt.Sprintf(`SELECT count(*) FROM %q WHERE %q IN(SELECT value FROM json_each(?))`, table, column)
			if err := m.db.QueryRow(q, hidden).Scan(&n); err != nil && err != sql.ErrNoRows {
				t.Fatalf("%s.%s: %v", table, column, err)
			}
			if n > 0 {
				parts = append(parts, fmt.Sprintf("%s.%s=%d", table, column, n))
			}
		}
	}
	return strings.Join(parts, " ")
}

type writeRoute struct {
	probes func(m *restrictionEnv) []idProbe
	reason string
}

func writeExempt(reason string) writeRoute { return writeRoute{reason: reason} }
func writeProbes(f func(m *restrictionEnv) []idProbe) writeRoute {
	return writeRoute{probes: f}
}

// perHidden builds one probe per hidden sample of the given kinds ("item" is
// every item kind; "library" the libraries the profile isn't given; any other
// word is an entity kind). ID in method, path and body is replaced by the id.
func perHidden(method, path, body string, kinds ...string) writeRoute {
	return writeProbes(func(m *restrictionEnv) []idProbe {
		out := []idProbe{}
		for _, kind := range kinds {
			ids := []string{}
			switch kind {
			case "item":
				ids = m.hiddenItemSamples()
			case "library":
				ids = m.hiddenLibraries()
			default:
				if list := m.hiddenEntities[kind]; len(list) > 0 {
					ids = []string{list[0]}
				}
			}
			for _, id := range ids {
				// The operation id carries the id, so the absent twin is a new
				// operation rather than a reuse of the hidden probe's key.
				op := "matrix-" + kind + "-" + id
				out = append(out, idProbe{hidden: id, method: method,
					path: strings.ReplaceAll(strings.ReplaceAll(path, "ID", url.PathEscape(id)), "OP", op),
					body: strings.ReplaceAll(strings.ReplaceAll(body, "ID", id), "OP", op)})
			}
		}
		return out
	})
}

const (
	reasonOwnSession  = "acts on the caller's own session, device, profile or account; names no catalogue id"
	reasonSignIn      = "sign-in, registration, setup or recovery; runs before or outside a profile session"
	reasonNetworking  = "server networking and claim authority; owner-authorized (claim_handler tests), no catalogue ids"
	reasonOwnObject   = "addresses an object the viewer owns (its playlist, saved resource, pin, receiver, group or device) by that object's id; the object's content is read through the probed GET routes"
	reasonSelfPrefs   = "the viewer's own preferences; no catalogue ids"
	reasonGrantWrite  = "addresses an opaque session, queue, grant or handoff minted only by a request that already passed the restriction check (restriction_admission_test, playback_v1_security_test)"
	reasonLiveChannel = "Live TV and DVR act on guide programmes and channels, which carry their own restriction (channel_restriction_test, recordings_switch_test, TestChannelsARestrictionMidPlayFencesMedia)"
)

// restrictionWriteRoutes classifies every non-GET route except owner
// administration (/v1/admin/…), which is probed by rule.
func restrictionWriteRoutes() map[string]writeRoute {
	return map[string]writeRoute{
		// Catalogue content named by id.
		"PUT /v1/items/{id}/personal-state": perHidden("PUT", "/v1/items/ID/personal-state", `{"operationId":"OP","expectedRevision":0,"favorite":true}`, "item"),
		// Not interested also names a show, album or book: those are probed too.
		"PUT /v1/items/personal-state:batch": writeProbes(func(m *restrictionEnv) []idProbe {
			items := perHidden("PUT", "/v1/items/personal-state:batch", `{"operationId":"OP","items":[{"itemId":"ID","watchlisted":true}]}`, "item").probes(m)
			return append(items, perHidden("PUT", "/v1/items/personal-state:batch", `{"operationId":"OP","items":[{"itemId":"ID","notInterested":true}]}`, "item", "show", "album", "book").probes(m)...)
		}),
		"PUT /v1/containers/{kind}/{id}/personal-state": writeProbes(func(m *restrictionEnv) []idProbe {
			out := []idProbe{}
			for _, kind := range []string{"show", "season", "album", "artist", "collection", "book"} {
				out = append(out, perHidden("PUT", "/v1/containers/"+kind+"/ID/personal-state", `{"expectedRevision":0,"watched":true}`, kind).probes(m)...)
			}
			return out
		}),
		"DELETE /v1/items/{id}":                                           perHidden("DELETE", "/v1/items/ID", "", "item"),
		"POST /v1/items/{id}/delete":                                      perHidden("POST", "/v1/items/ID/delete", `{"operationId":"OP"}`, "item"),
		"POST /v1/items/{id}/delete/preview":                              perHidden("POST", "/v1/items/ID/delete/preview", `{}`, "item"),
		"POST /v1/items/{id}/metadata/refresh":                            perHidden("POST", "/v1/items/ID/metadata/refresh", `{}`, "item"),
		"POST /v1/items/{id}/metadata/local-audio/policy":                 perHidden("POST", "/v1/items/ID/metadata/local-audio/policy", `{}`, "item"),
		"POST /v1/metadata/{kind}/{id}/art/{role}/upload":                 perHidden("POST", "/v1/metadata/item/ID/art/poster/upload", `{}`, "item"),
		"DELETE /v1/metadata/{kind}/{id}/art/{role}/upload/{candidateId}": perHidden("DELETE", "/v1/metadata/item/ID/art/poster/upload/matrix-candidate", "", "item"),
		"POST /v1/metadata/bulk":                                          perHidden("POST", "/v1/metadata/bulk", `{"operationId":"OP","itemIds":["ID"]}`, "item"),
		"PUT /v1/shows/{id}/metadata/screen/season":                       perHidden("PUT", "/v1/shows/ID/metadata/screen/season", `{}`, "show"),
		"PUT /v1/shows/{id}/metadata/tvdb":                                perHidden("PUT", "/v1/shows/ID/metadata/tvdb", `{}`, "show"),
		"POST /v1/shows/{id}/metadata/tvdb/retry":                         perHidden("POST", "/v1/shows/ID/metadata/tvdb/retry", `{}`, "show"),
		"PATCH /v1/collections/{id}":                                      perHidden("PATCH", "/v1/collections/ID", `{"name":"x"}`, "collection"),
		"DELETE /v1/collections/{id}":                                     perHidden("DELETE", "/v1/collections/ID", "", "collection"),
		"POST /v1/collections/{id}/items:batch": writeProbes(func(m *restrictionEnv) []idProbe {
			out := perHidden("POST", "/v1/collections/ID/items:batch", `{"addItemIds":[]}`, "collection").probes(m)
			if visible := m.visibleEntities["collection"]; len(visible) > 0 {
				out = append(out, perHidden("POST", "/v1/collections/"+url.PathEscape(visible[0])+"/items:batch", `{"addItemIds":["ID"]}`, "item").probes(m)...)
			}
			return out
		}),
		"POST /v1/downloads/preparations":             perHidden("POST", "/v1/downloads/preparations", `{"operationId":"OP","mediaId":"ID","quality":"original"}`, "item"),
		"POST /v1/playback/sessions":                  perHidden("POST", "/v1/playback/sessions", `{"itemId":"ID","startFrom":"beginning"}`, "item"),
		"POST /v1/queues":                             perHidden("POST", "/v1/queues", `{"segments":[{"source":{"items":{"ids":["ID"]}}}],"startPlayback":{"state":"playing"}}`, "item"),
		"PATCH /v1/dvr/recordings/{id}":               perHidden("PATCH", "/v1/dvr/recordings/ID", `{"expectedRevision":0}`, "item"),
		"DELETE /v1/dvr/recordings/{id}":              perHidden("DELETE", "/v1/dvr/recordings/ID", "", "item"),
		"PUT /v1/dvr/recordings/{id}/keep":            perHidden("PUT", "/v1/dvr/recordings/ID/keep", `{"keep":true}`, "item"),
		"POST /v1/dvr/recordings/{id}/cancel":         perHidden("POST", "/v1/dvr/recordings/ID/cancel", `{}`, "item"),
		"PUT /v1/me/library-navigation":               perHidden("PUT", "/v1/me/library-navigation", `{"expectedRevision":0,"pinnedLibraryIds":["ID"]}`, "library"),
		"POST /v1/libraries":                          writeExempt("owner-only library creation (library_routes tests); names no existing catalogue id"),
		"POST /v1/libraries/{id}/browse":              perHidden("POST", "/v1/libraries/ID/browse", `{"pivot":"movies","limit":20}`, "library"),
		"POST /v1/libraries/{id}/collections":         perHidden("POST", "/v1/libraries/ID/collections", `{"name":"x"}`, "library"),
		"POST /v1/libraries/{id}/episode-assignments": perHidden("POST", "/v1/libraries/ID/episode-assignments", `{}`, "library"),
		"POST /v1/libraries/{id}/lyrics/fetch":        perHidden("POST", "/v1/libraries/ID/lyrics/fetch", `{}`, "library"),
		"POST /v1/libraries/{id}/network-roots":       perHidden("POST", "/v1/libraries/ID/network-roots", `{}`, "library"),
		"POST /v1/libraries/{id}/scans":               perHidden("POST", "/v1/libraries/ID/scans", `{}`, "library"),
		"PUT /v1/libraries/{id}/metadata/agent":       perHidden("PUT", "/v1/libraries/ID/metadata/agent", `{}`, "library"),
		"PUT /v1/libraries/{id}/metadata/screen":      perHidden("PUT", "/v1/libraries/ID/metadata/screen", `{}`, "library"),
		"PUT /v1/libraries/{id}/strm-analysis-policy": perHidden("PUT", "/v1/libraries/ID/strm-analysis-policy", `{}`, "library"),
		"POST /v1/jobs":                               perHidden("POST", "/v1/jobs", `{"operationId":"OP","command":"personal-state","selector":{"items":{"ids":["ID"]}},"args":{"watched":true}}`, "item"),
		"POST /v1/downloads/requests":                 writeExempt("probed with hidden shows, seasons and items by the read matrix's download-request-security subtest"),

		// The viewer's own objects, sessions and grants.
		"PUT /v1/playlists/{id}/order":                      writeExempt(reasonOwnObject),
		"PUT /v1/saved-pins/order":                          writeExempt(reasonOwnObject),
		"PUT /v1/saved-pins/{kind}/{id}":                    writeExempt(reasonOwnObject),
		"POST /v1/personal-history/actions":                 writeExempt("acts on the viewer's own history rows by action; history reads are filtered to visible titles (probed GET /v1/personal-history)"),
		"POST /v1/me/recommendations:reset":                 writeExempt("names no title: clears the viewer's own not-interested marks and learned taste, and answers with a count only"),
		"DELETE /v1/search/history":                         writeExempt(reasonOwnObject),
		"PATCH /v1/playback/sessions/{id}":                  writeExempt(reasonGrantWrite),
		"DELETE /v1/playback/sessions/{id}":                 writeExempt(reasonGrantWrite),
		"POST /v1/playback/sessions/{id}/progress":          writeExempt(reasonGrantWrite),
		"POST /v1/playback/sessions/{id}/timeline":          writeExempt(reasonGrantWrite),
		"POST /v1/playback/route-failures":                  writeExempt(reasonGrantWrite),
		"PATCH /v1/queues/{id}":                             writeExempt(reasonGrantWrite),
		"DELETE /v1/queues/{id}":                            writeExempt(reasonGrantWrite),
		"DELETE /v1/queues/{id}/entries/{entryId}":          writeExempt(reasonGrantWrite),
		"POST /v1/queues/{id}/entries/{entryId}:move":       writeExempt(reasonGrantWrite),
		"POST /v1/queues/{id}/segments":                     writeExempt(reasonGrantWrite + "; new segments are fenced per entry like a queue create (a6fc591d)"),
		"POST /v1/queues/{id}:advance":                      writeExempt(reasonGrantWrite),
		"POST /v1/queues/{id}:commit-next":                  writeExempt(reasonGrantWrite),
		"POST /v1/queues/{id}:prepare-next":                 writeExempt(reasonGrantWrite),
		"POST /v1/queues/{id}:save-as-playlist":             writeExempt(reasonGrantWrite),
		"POST /v1/downloads/preparations/{id}/actions":      writeExempt(reasonGrantWrite),
		"POST /v1/downloads/preparations/{id}/grant":        writeExempt(reasonGrantWrite),
		"POST /v1/downloads/progress":                       writeExempt(reasonGrantWrite),
		"POST /v1/downloads/receipts":                       writeExempt(reasonGrantWrite),
		"POST /v1/downloads/receipts/revalidate":            writeExempt(reasonGrantWrite),
		"POST /v1/downloads/receipts/revoke":                writeExempt(reasonGrantWrite),
		"POST /v1/handoffs":                                 writeExempt(reasonGrantWrite),
		"POST /v1/handoffs/{id}/commit":                     writeExempt(reasonGrantWrite),
		"POST /v1/handoffs/{id}/readiness":                  writeExempt(reasonGrantWrite),
		"POST /v1/handoffs/{id}/rollback":                   writeExempt(reasonGrantWrite),
		"POST /v1/groups":                                   writeExempt(reasonOwnObject + "; a group carries no playback authority (createGroup)"),
		"POST /v1/groups/join":                              writeExempt(reasonOwnObject),
		"POST /v1/groups/{id}/end":                          writeExempt(reasonOwnObject),
		"POST /v1/groups/{id}/heartbeat":                    writeExempt(reasonOwnObject),
		"POST /v1/groups/{id}/host-transfer":                writeExempt(reasonOwnObject),
		"POST /v1/groups/{id}/invites":                      writeExempt(reasonOwnObject),
		"DELETE /v1/groups/{id}/invites/{inviteId}":         writeExempt(reasonOwnObject),
		"POST /v1/groups/{id}/leave":                        writeExempt(reasonOwnObject),
		"POST /v1/groups/{id}/queue":                        writeExempt(reasonGrantWrite + "; the group queue follows the host's admitted v1 session"),
		"POST /v1/groups/{id}/readiness":                    writeExempt(reasonOwnObject),
		"POST /v1/groups/{id}/settings":                     writeExempt(reasonOwnObject),
		"POST /v1/groups/{id}/transport":                    writeExempt(reasonOwnObject),
		"POST /v1/receivers":                                writeExempt(reasonOwnObject),
		"DELETE /v1/receivers/{id}":                         writeExempt(reasonOwnObject),
		"POST /v1/receivers/{id}/grants":                    writeExempt(reasonGrantWrite),
		"POST /v1/receivers/{id}/grants/{grantId}/decision": writeExempt(reasonGrantWrite),
		"POST /v1/receivers/{id}/heartbeat":                 writeExempt(reasonOwnObject),
		"POST /v1/cast/bootstrap":                           writeExempt(reasonOwnSession + " (a pairing code)"),
		"POST /v1/cast/reconnect":                           writeExempt(reasonOwnSession),
		"POST /v1/cast/redeem":                              writeExempt(reasonOwnSession),
		"DELETE /v1/cast/devices/{id}":                      writeExempt(reasonOwnObject),
		"PUT /v1/cast/configuration":                        writeExempt("owner Cast configuration; no catalogue ids"),
		"POST /v1/topshelf/token":                           writeExempt(reasonOwnSession),

		// Live TV and DVR.
		"POST /v1/dvr/recordings":               writeExempt(reasonLiveChannel),
		"POST /v1/dvr/rules/preview":            writeExempt(reasonLiveChannel),
		"PUT /v1/dvr/rules/{id}":                writeExempt(reasonLiveChannel),
		"DELETE /v1/dvr/rules/{id}":             writeExempt(reasonLiveChannel),
		"PUT /v1/dvr/storage":                   writeExempt(reasonLiveChannel),
		"POST /v1/channels/preferences":         writeExempt(reasonLiveChannel),
		"POST /v1/library-channels/preferences": writeExempt(reasonLiveChannel),

		// Preferences.
		"PUT /v1/home/layout":                     writeExempt(reasonSelfPrefs),
		"POST /v1/home/layout/reset":              writeExempt(reasonSelfPrefs),
		"PUT /v1/listening/preferences":           writeExempt(reasonSelfPrefs),
		"PUT /v1/downloads/settings":              writeExempt(reasonSelfPrefs),
		"PUT /v1/lyrics/provider":                 writeExempt("owner lyrics provider setting; no catalogue ids"),
		"PUT /v1/playback/client-profile":         writeExempt(reasonSelfPrefs),
		"PUT /v1/me/devices/current/capabilities": writeExempt(reasonSelfPrefs),
		"POST /v1/diagnostics/client-logs":        writeExempt(reasonOwnSession),

		// Sessions, devices, identity.
		"POST /v1/sessions":                                 writeExempt(reasonSignIn),
		"DELETE /v1/sessions/current":                       writeExempt(reasonOwnSession),
		"POST /v1/auth/refresh":                             writeExempt(reasonSignIn),
		"POST /v1/auth/register":                            writeExempt(reasonSignIn),
		"POST /v1/auth/two-factor/challenge":                writeExempt(reasonSignIn),
		"POST /v1/access/invitations/accept":                writeExempt(reasonSignIn),
		"POST /v1/access/invitations/preview":               writeExempt(reasonSignIn),
		"POST /v1/devices":                                  writeExempt(reasonOwnSession),
		"PATCH /v1/devices/{id}":                            writeExempt(reasonOwnSession),
		"DELETE /v1/devices/{id}":                           writeExempt(reasonOwnSession),
		"DELETE /v1/devices/{id}/sessions":                  writeExempt(reasonOwnSession),
		"POST /v1/devices/{id}/approval":                    writeExempt(reasonOwnSession),
		"POST /v1/devices/{id}/sessions/bind":               writeExempt(reasonOwnSession),
		"POST /v1/quick-connect":                            writeExempt(reasonSignIn),
		"POST /v1/quick-connect/":                           writeExempt(reasonSignIn),
		"POST /v1/quick-connect/decision":                   writeExempt(reasonOwnSession),
		"POST /v1/quick-connect/review":                     writeExempt(reasonOwnSession),
		"POST /v1/setup":                                    writeExempt(reasonSignIn),
		"POST /v1/setup/":                                   writeExempt(reasonSignIn),
		"POST /v1/setup/browser":                            writeExempt(reasonSignIn),
		"POST /v1/setup/resume":                             writeExempt(reasonSignIn),
		"POST /v1/hosted/attach":                            writeExempt(reasonSignIn),
		"POST /v1/hosted/profiles/offline-select":           writeExempt(reasonOwnSession),
		"POST /v1/hosted/restrictions":                      writeExempt("Hosted pushes a profile's restriction document; server credential, no catalogue ids"),
		"POST /v1/hosted/wake":                              writeExempt(reasonNetworking),
		"POST /v1/direct/members":                           writeExempt("owner member management (admin_access_test); no catalogue ids"),
		"PATCH /v1/direct/members/{id}":                     writeExempt("owner member management (admin_access_test); no catalogue ids"),
		"POST /v1/direct/ownership":                         writeExempt(reasonOwnSession),
		"POST /v1/direct/ownership/custody":                 writeExempt(reasonOwnSession),
		"POST /v1/direct/password":                          writeExempt(reasonOwnSession),
		"POST /v1/direct/portico-challenge":                 writeExempt(reasonSignIn),
		"POST /v1/direct/profile-switch":                    writeExempt(reasonOwnSession),
		"POST /v1/direct/profiles":                          writeExempt(reasonOwnSession),
		"PATCH /v1/direct/profiles/{id}":                    writeExempt(reasonOwnSession),
		"DELETE /v1/direct/profiles/{id}":                   writeExempt(reasonOwnSession),
		"POST /v1/direct/profiles/{id}/avatar":              writeExempt(reasonOwnSession),
		"DELETE /v1/direct/profiles/{id}/avatar":            writeExempt(reasonOwnSession),
		"POST /v1/direct/profiles/{id}/pin-reset":           writeExempt(reasonOwnSession),
		"POST /v1/direct/profiles/{id}/select":              writeExempt(reasonOwnSession),
		"PUT /v1/direct/profiles/order":                     writeExempt(reasonOwnSession),
		"PUT /v1/direct/profiles/{id}/restrictions":         writeExempt("sets a profile's restriction document (restriction tests); names library ids the owner grants, never returns titles"),
		"PUT /v1/direct/registration-policy":                writeExempt(reasonOwnSession),
		"POST /v1/direct/remembered-accounts":               writeExempt(reasonOwnSession),
		"DELETE /v1/direct/remembered-accounts/{accountId}": writeExempt(reasonOwnSession),
		"DELETE /v1/direct/sessions/{id}":                   writeExempt(reasonOwnSession),
		"POST /v1/direct/sessions/sign-out-everywhere":      writeExempt(reasonOwnSession),
		"POST /v1/direct/sign-in":                           writeExempt(reasonSignIn),
		"DELETE /v1/direct/trust":                           writeExempt(reasonOwnSession),
		"DELETE /v1/direct/two-factor":                      writeExempt(reasonOwnSession),
		"POST /v1/direct/two-factor/enrol":                  writeExempt(reasonOwnSession),
		"POST /v1/direct/two-factor/verify":                 writeExempt(reasonOwnSession),

		// Registrations the lane table records in unusual forms.
		"/":                                    writeExempt("the web app shell and first-run setup page (installation.go): static; no catalogue ids"),
		"/dlna/":                               writeExempt("no handler is registered for it (stale lane entry); the admission test keeps the table complete"),
		"POST ":                                writeExempt("the lane scanner's reading of a concatenated registration (screen_metadata.go: \"POST \"+base+\"/search\" and \"/retry\"); owner-only screen metadata, refused to a member (probed through PUT /v1/libraries/{id}/metadata/screen)"),
		"PUT ":                                 writeExempt("the lane scanner's reading of a concatenated registration (screen_metadata.go: \"PUT \"+base); owner-only screen metadata"),
		"HEAD /v1/media/{grant}/audio":         writeExempt(reasonGrantWrite + " (HEAD of a grant-bound media URL)"),
		"HEAD /v1/downloads/artifacts/{grant}": writeExempt(reasonGrantWrite + " (HEAD of a grant-bound download)"),

		// Networking and claim.
		"POST /v1/networking/claim/":               writeExempt(reasonNetworking),
		"POST /v1/networking/claim/await-approval": writeExempt(reasonNetworking),
		"POST /v1/networking/claim/endpoint":       writeExempt(reasonNetworking),
		"POST /v1/networking/claim/web-approval":   writeExempt(reasonNetworking),
		"POST /v1/networking/endpoint-proof":       writeExempt(reasonNetworking),
		"POST /v1/networking/identity-proof":       writeExempt(reasonNetworking),
	}
}
