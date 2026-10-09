package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/downloads"
	"portico.local/server/internal/hosted"
	"portico.local/server/internal/httpapi/fixture"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/livechannels"
	"portico.local/server/internal/livechannels/dvr"
	"portico.local/server/internal/mediaanalysis"
	"portico.local/server/internal/metadata"
	"portico.local/server/internal/operations"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/playback"
	"portico.local/server/internal/recordingaccess"
)

// SEC-02 / BE-SRV-01: a restricted profile sees nothing restricted on ANY route.
//
// The matrix walks the router's own route table (routeLanes, which the
// admission test proves complete). Every GET route must appear in
// restrictionRoutes, either with the concrete requests that reach catalogue
// content, or with the reason it cannot carry a title. A new GET route without
// an entry fails this test, so adding a route means deciding its restriction
// story. The content-bearing POST routes (browse, search-like reads) are listed
// in restrictionReadPOSTs and probed the same way.
//
// A probe passes when the restricted viewer is refused (any 4xx) or when the
// answer contains no identifier of anything that viewer may not see: a title
// over the rating ceiling or carrying a blocked label, a container with no
// visible member, or anything in a library the profile is not given. The
// hidden set is computed with the catalogue's own visibility predicate
// (catalog.VisibleItem/VisibleEntity), the single rule every surface must
// apply, and checked for being non-empty and visible to the open viewer so the
// test cannot pass vacuously.
func TestRestrictedProfileRouteMatrix(t *testing.T) {
	m := buildRestrictionMatrix(t)
	routes := restrictionRoutes()

	// 1. Exhaustive: every GET route is classified, and nothing stale is.
	for pattern := range routeLanes {
		if !strings.HasPrefix(pattern, "GET ") {
			continue
		}
		if _, ok := routes[pattern]; !ok {
			t.Errorf("%s has no restriction classification: add it to restrictionRoutes (restriction_matrix_test.go) with the requests that reach catalogue content, or the reason it carries none", pattern)
		}
	}
	for pattern := range routes {
		if _, ok := routeLanes[pattern]; !ok {
			t.Errorf("restrictionRoutes lists %s, which the router does not register", pattern)
		}
	}
	for pattern := range restrictionReadPOSTs(m) {
		if _, ok := routeLanes[pattern]; !ok {
			t.Errorf("restrictionReadPOSTs lists %s, which the router does not register", pattern)
		}
	}

	// 2. No route answers 401 to a valid session (CD-51). Every GET route is
	// called as the restricted member, with generic path parameters, except
	// routes whose credential is not the session (a grant or device token).
	for pattern := range routeLanes {
		if !strings.HasPrefix(pattern, "GET ") || nonSessionCredential[pattern] != "" {
			continue
		}
		path := pathParameter.ReplaceAllString(strings.TrimPrefix(pattern, "GET "), "matrix-unknown")
		if path == "" {
			path = "/"
		}
		status, response := m.callWithin("GET", path, "restricted", 300*time.Millisecond)
		if status == 401 {
			t.Errorf("%s: GET %s answered 401 to a valid restricted session: %s", pattern, path, strings.TrimSpace(response))
		}
	}

	// 3. Probes.
	probed := 0
	patterns := make([]string, 0, len(routes))
	for pattern := range routes {
		patterns = append(patterns, pattern)
	}
	sort.Strings(patterns)
	for _, pattern := range patterns {
		route := routes[pattern]
		if route.probe == nil {
			if route.reason == "" {
				t.Errorf("%s: an exemption needs a reason", pattern)
			}
			continue
		}
		viewer := route.viewer
		if viewer == "" {
			viewer = "restricted"
		}
		for _, path := range route.probe(m) {
			probed++
			m.check(t, pattern, viewer, "GET", path, "")
		}
	}
	for pattern, bodies := range restrictionReadPOSTs(m) {
		for _, request := range bodies {
			probed++
			m.check(t, pattern, "restricted", "POST", request.path, request.body)
		}
	}
	t.Run("container-writes", func(t *testing.T) {
		for _, id := range m.hiddenEntities["show"] {
			code, _ := m.call("PUT", "/v1/containers/show/"+id+"/personal-state", `{"expectedRevision":0,"watched":true}`, "restricted")
			if code != 404 {
				t.Errorf("hidden container: %d", code)
			}
		}
		if len(m.visibleEntities["show"]) > 0 {
			id := m.visibleEntities["show"][0]
			code, body := m.call("PUT", "/v1/containers/show/"+id+"/personal-state", `{"expectedRevision":0,"watched":true}`, "restricted")
			if code != 200 {
				t.Fatalf("visible container: %d %s", code, body)
			}
			m.check(t, "GET /v1/containers/{kind}/{id}/personal-state", "restricted", "GET", "/v1/containers/show/"+id+"/personal-state", "")
		}
	})
	t.Run("container-watermark-security", func(t *testing.T) { m.containerWatermarkSecurity(t) })
	t.Run("download-request-security", func(t *testing.T) { m.downloadRequestSecurity(t) })
	// Non-vacuous: most probed routes must actually answer the restricted
	// profile, so their bodies were checked rather than refused wholesale.
	answered := 0
	for pattern, statuses := range m.statuses {
		ok := false
		for _, status := range statuses {
			ok = ok || status < 400
		}
		if ok {
			answered++
		}
		if testing.Verbose() {
			t.Logf("%-55s %v", pattern, statuses)
		}
	}
	if answered < 40 {
		t.Errorf("only %d probed routes answered the restricted profile; the fixture or the probes are wrong", answered)
	}
	t.Logf("%d restricted-viewer requests over %d routes (%d answered with content); %d hidden identifiers", probed, len(routes), answered, len(m.markers))
}

// refusedStatus is the answer to a refused request (CD-51): 401 only without
// a credential, 403 for a valid session that may not do this.
func refusedStatus(token string) int {
	if token == "" {
		return 401
	}
	return 403
}

// nonSessionCredential are the GET routes authenticated by something other than
// the session, so a session bearer there is rightly "not a credential" (401).
var nonSessionCredential = map[string]string{
	"GET /v1/topshelf":                                      "Top Shelf device token",
	"GET /v1/topshelf/art/{id}":                             "Top Shelf device token",
	"GET /v1/media/{grant}":                                 "media grant",
	"GET /v1/media/{grant}/{file}":                          "media grant",
	"GET /v1/media/{grant}/audio-converted":                 "media grant",
	"GET /v1/media/{grant}/audio":                           "media grant",
	"GET /v1/media/{grant}/subtitles/{resource}/{revision}": "media grant",
	"GET /v1/downloads/artifacts/{grant}":                   "download grant",
}

var pathParameter = regexp.MustCompile(`\{[^}]+\}`)

type restrictionRoute struct {
	probe  func(*restrictionEnv) []string
	reason string
	// viewer names the credential the probe uses; empty is the restricted
	// profile's session. The Top Shelf feed takes its own device token.
	viewer string
}

type postProbe struct{ path, body string }

func exempt(reason string) restrictionRoute { return restrictionRoute{reason: reason} }
func probe(f func(*restrictionEnv) []string) restrictionRoute {
	return restrictionRoute{probe: f}
}
func static(paths ...string) restrictionRoute {
	return probe(func(*restrictionEnv) []string { return paths })
}

// perItem expands a suffix over one hidden sample of every item kind, plus one
// visible title (whose answer must still not leak hidden neighbours).
func perItem(suffix string) restrictionRoute {
	return probe(func(m *restrictionEnv) []string {
		out := []string{}
		for _, id := range append(m.hiddenItemSamples(), m.visibleMovie) {
			out = append(out, "/v1/items/"+id+suffix)
		}
		return out
	})
}

func perLibrary(suffix string) restrictionRoute {
	return probe(func(m *restrictionEnv) []string {
		out := []string{}
		for _, id := range m.libraries {
			out = append(out, "/v1/libraries/"+id+suffix)
		}
		return out
	})
}

func perEntity(prefix, kind, suffix string) restrictionRoute {
	return probe(func(m *restrictionEnv) []string {
		out := []string{}
		for _, id := range append(append([]string{}, m.hiddenEntities[kind]...), m.visibleEntities[kind]...) {
			out = append(out, prefix+id+suffix)
		}
		return out
	})
}

const (
	reasonAdmin    = "owner-only administration: a member profile is refused before any catalogue read (admin_access_test, owner_authority_test)"
	reasonNoTitles = "carries no catalogue titles"
	reasonGrant    = "addresses an opaque grant, session or job minted only by a request that already passed the restriction check (restriction_admission_test, playback_v1_security_test, downloads_routes_test)"
	reasonStream   = "long-lived event stream; its payloads are the viewer's own changes and ids already admitted (social_routes_test, notifications tests)"
)

// restrictionRoutes classifies every GET route the router registers.
func restrictionRoutes() map[string]restrictionRoute {
	return map[string]restrictionRoute{
		"GET /v1/admin/ingestion/jobs/{id}":             static("/v1/admin/ingestion/jobs/unknown"),
		"GET /v1/downloads/requests/{id}":               static("/v1/downloads/requests/unknown"),
		"GET /v1/jobs/{id}":                             static("/v1/jobs/unknown"),
		"GET /v1/jobs/{id}/failures":                    static("/v1/jobs/unknown/failures"),
		"GET /v1/containers/{kind}/{id}/personal-state": perEntity("/v1/containers/show/", "show", "/personal-state"),
		// Health, identity, device and server description.
		"GET ":                                          exempt("the web app shell and first-run setup page (installation.go); static, no catalogue data"),
		"GET /health/live":                              exempt(reasonNoTitles),
		"GET /health/ready":                             exempt(reasonNoTitles),
		"GET /receiver/cast/":                           exempt("static Cast receiver page"),
		"GET /v1/attribution":                           exempt(reasonNoTitles),
		"GET /v1/auth/capabilities":                     exempt(reasonNoTitles),
		"GET /v1/auth/remembered-accounts":              exempt(reasonNoTitles),
		"GET /v1/capabilities":                          exempt(reasonNoTitles),
		"GET /v1/devices":                               exempt(reasonNoTitles),
		"GET /v1/direct":                                exempt(reasonNoTitles),
		"GET /v1/direct/members":                        exempt(reasonAdmin),
		"GET /v1/direct/pin-recovery":                   exempt(reasonNoTitles),
		"GET /v1/direct/profiles/avatars":               exempt(reasonNoTitles),
		"GET /v1/direct/profiles/{id}/deletion-preview": exempt(reasonNoTitles),
		"GET /v1/direct/profiles/{id}/restrictions":     exempt("the restriction document itself, no titles"),
		"GET /v1/direct/registration-policy":            exempt(reasonNoTitles),
		"GET /v1/direct/sessions":                       exempt(reasonNoTitles),
		"GET /v1/direct/two-factor":                     exempt(reasonNoTitles),
		"GET /v1/me":                                    exempt(reasonNoTitles),
		"GET /v1/me/devices/current/capabilities":       exempt(reasonNoTitles),
		"GET /v1/profiles/{id}/avatar":                  exempt(reasonNoTitles),
		"GET /v1/rating-systems":                        exempt(reasonNoTitles),
		"GET /v1/readiness":                             exempt(reasonNoTitles),
		"GET /v1/server":                                exempt(reasonNoTitles),
		"GET /v1/setup/state":                           exempt(reasonNoTitles),
		"GET /v1/system":                                exempt(reasonNoTitles),
		"GET /v1/networking/certificate":                exempt(reasonNoTitles),
		"GET /v1/networking/claim":                      exempt(reasonNoTitles),
		"GET /v1/networking/remote":                     exempt(reasonNoTitles),
		"GET /v1/playback/client-profile":               exempt(reasonNoTitles),
		"GET /v1/playback/delivery-policy":              exempt(reasonNoTitles),
		"GET /v1/listening/preferences":                 exempt(reasonNoTitles),
		"GET /v1/downloads/settings":                    exempt(reasonNoTitles),
		"GET /v1/downloads/usage":                       exempt(reasonNoTitles),
		"GET /v1/downloads/receipt-keys":                exempt(reasonNoTitles),
		"GET /v1/downloads/receipts/revocations":        exempt(reasonNoTitles),
		"GET /v1/cast/configuration":                    exempt(reasonNoTitles),
		"GET /v1/storage/mounts":                        exempt(reasonAdmin),
		"GET /v1/storage/sources":                       exempt(reasonAdmin),
		"GET /v1/storage/operations/{id}":               exempt(reasonAdmin),
		"GET /v1/storage/source-operations/{id}":        exempt(reasonAdmin),
		"GET /v1/saved-pins/order":                      exempt("pin order: resource ids only, each read through the saved-resource routes probed below"),

		// Streams.
		"GET /v1/events":               exempt(reasonStream),
		"GET /v1/groups/{id}/events":   exempt(reasonStream),
		"GET /v1/notifications/events": exempt(reasonStream),
		"GET /v1/notifications/wait":   exempt(reasonStream),

		// Grants, sessions, jobs and social objects created by admitted requests.
		"GET /v1/cast/bootstrap/{id}":                                   exempt(reasonGrant),
		"GET /v1/downloads/artifacts/{grant}":                           exempt(reasonGrant),
		"GET /v1/downloads/preparations/{id}":                           exempt(reasonGrant),
		"GET /v1/groups/{id}":                                           exempt(reasonGrant),
		"GET /v1/groups/{id}/queue":                                     exempt(reasonGrant),
		"GET /v1/handoffs/{id}":                                         exempt(reasonGrant),
		"GET /v1/media/{grant}":                                         exempt(reasonGrant),
		"GET /v1/media/{grant}/audio":                                   exempt(reasonGrant),
		"GET /v1/media/{grant}/audio-converted":                         exempt(reasonGrant),
		"GET /v1/media/{grant}/subtitles/{resource}/{revision}":         exempt(reasonGrant),
		"GET /v1/media/{grant}/{file}":                                  exempt(reasonGrant),
		"GET /v1/playback/sessions/{id}":                                exempt(reasonGrant),
		"GET /v1/playback/sessions/{id}/chapters":                       exempt(reasonGrant),
		"GET /v1/queues/{id}":                                           exempt(reasonGrant),
		"GET /v1/queues/{id}/entries":                                   exempt(reasonGrant),
		"GET /v1/queues/{id}/window":                                    exempt(reasonGrant),
		"GET /v1/receivers/{id}/grants":                                 exempt(reasonGrant),
		"GET /v1/receivers/{id}/inbox":                                  exempt(reasonGrant),
		"GET /v1/receivers/{id}/inbox/wait":                             exempt(reasonGrant),
		"GET /v1/media/linear/{playbackId}/{generation}/{token}/{name}": exempt(reasonGrant + "; v1 capability handler, with live revocation proven by TestChannelsARestrictionMidPlayFencesMedia"),

		// Owner-only administration.
		"GET /v1/admin/access/api-keys":                         exempt(reasonAdmin),
		"GET /v1/admin/access/devices":                          exempt(reasonAdmin),
		"GET /v1/admin/access/invitations":                      exempt(reasonAdmin),
		"GET /v1/admin/access/members":                          exempt(reasonAdmin),
		"GET /v1/admin/play-history":                            exempt(reasonAdmin),
		"GET /v1/admin/play-history/summary":                    exempt(reasonAdmin),
		"GET /v1/admin/access/members/{id}/limits":              exempt(reasonAdmin),
		"GET /v1/admin/analysis-operations":                     exempt(reasonAdmin),
		"GET /v1/admin/backups":                                 exempt(reasonAdmin),
		"GET /v1/admin/connectivity/policy":                     exempt(reasonAdmin),
		"GET /v1/admin/connectivity/status":                     exempt(reasonAdmin),
		"GET /v1/admin/diagnostics/bundle":                      exempt(reasonAdmin),
		"GET /v1/admin/diagnostics/capabilities":                exempt(reasonAdmin),
		"GET /v1/admin/diagnostics/client-logs":                 exempt(reasonAdmin),
		"GET /v1/admin/diagnostics/client-logs/{id}":            exempt(reasonAdmin),
		"GET /v1/admin/diagnostics/concurrency":                 exempt(reasonAdmin),
		"GET /v1/admin/diagnostics/pprof/{profile}":             exempt(reasonAdmin),
		"GET /v1/admin/dvr/recording-groups":                    exempt(reasonAdmin),
		"GET /v1/admin/dvr/recording-owners":                    exempt(reasonAdmin),
		"GET /v1/admin/dvr/recording-permissions":               exempt(reasonAdmin),
		"GET /v1/admin/dvr/settings":                            exempt(reasonAdmin),
		"GET /v1/admin/dvr/tuners":                              exempt(reasonAdmin),
		"GET /v1/admin/filesystem":                              exempt(reasonAdmin),
		"GET /v1/admin/libraries/{id}/inventory":                exempt(reasonAdmin),
		"GET /v1/admin/libraries/{id}/inventory-config":         exempt(reasonAdmin),
		"GET /v1/admin/libraries/{id}/settings":                 exempt(reasonAdmin),
		"GET /v1/admin/libraries/{id}/sources/{source}/changes": exempt(reasonAdmin),
		"GET /v1/admin/library-channels":                        exempt(reasonAdmin),
		"GET /v1/admin/library-channels/block-presets":          exempt(reasonAdmin),
		"GET /v1/admin/library-channels/criteria":               exempt(reasonAdmin),
		"GET /v1/admin/library-channels/templates":              exempt(reasonAdmin),
		"GET /v1/admin/library-channels/{id}/health":            exempt(reasonAdmin),
		"GET /v1/admin/library-settings":                        exempt(reasonAdmin),
		"GET /v1/admin/live-sources":                            exempt(reasonAdmin),
		"GET /v1/admin/live-sources/{id}/channel-map":           exempt(reasonAdmin),
		"GET /v1/admin/live-sources/{id}/configuration":         exempt(reasonAdmin),
		"GET /v1/admin/live/settings":                           exempt(reasonAdmin),
		"GET /v1/admin/logs":                                    exempt(reasonAdmin),
		"GET /v1/admin/logs/events":                             exempt(reasonAdmin),
		"GET /v1/admin/logs/settings":                           exempt(reasonAdmin),
		"GET /v1/admin/maintenance/settings":                    exempt(reasonAdmin),
		"GET /v1/admin/metadata-providers":                      exempt(reasonAdmin),
		"GET /v1/admin/operations/{panel}":                      exempt(reasonAdmin),
		"GET /v1/admin/playback/diagnostics":                    exempt(reasonAdmin),
		"GET /v1/admin/playback/history.csv":                    exempt(reasonAdmin),
		"GET /v1/admin/sessions":                                exempt(reasonAdmin),
		"GET /v1/admin/storage-usage":                           exempt(reasonAdmin),
		"GET /v1/admin/support-report":                          exempt(reasonAdmin),
		"GET /v1/admin/trash":                                   exempt(reasonAdmin),
		"GET /v1/admin/updates":                                 exempt(reasonAdmin),
		"GET /v1/library-kinds/{kind}/metadata-agents":          exempt(reasonAdmin),

		// Catalogue: global lists.
		"GET /v1/bootstrap":             static("/v1/bootstrap"),
		"GET /v1/home":                  static("/v1/home"),
		"GET /v1/home/layout":           static("/v1/home/layout"),
		"GET /v1/content":               static("/v1/content?view=home"),
		"GET /v1/suggestions":           static("/v1/suggestions?limit=50"),
		"GET /v1/libraries":             static("/v1/libraries"),
		"GET /v1/me/library-navigation": static("/v1/me/library-navigation"),
		"GET /v1/personal-history":      static("/v1/personal-history"),
		"GET /v1/topshelf":              {probe: func(*restrictionEnv) []string { return []string{"/v1/topshelf"} }, viewer: "topshelf"},
		"GET /v1/search/history":        static("/v1/search/history"),
		"GET /v1/items": probe(func(m *restrictionEnv) []string {
			return []string{"/v1/items?ids=" + strings.Join(m.hiddenItemSamples(), ","), "/v1/items?limit=200"}
		}),
		"GET /v1/search":               probe(func(m *restrictionEnv) []string { return m.searchPaths() }),
		"GET /v1/people":               static("/v1/people?q=Person&limit=100"),
		"GET /v1/home/rows/{id}":       probe(func(m *restrictionEnv) []string { return m.homeRows() }),
		"GET /v1/saved-resources":      static("/v1/saved-resources?kind=collection", "/v1/saved-resources?kind=collection&pinned=true"),
		"GET /v1/saved-resources/{id}": probe(func(m *restrictionEnv) []string { return []string{"/v1/saved-resources/" + m.savedResource} }),
		"GET /v1/saved-resources/{id}/content": probe(func(m *restrictionEnv) []string {
			return []string{"/v1/saved-resources/" + m.savedResource + "/content"}
		}),
		"GET /v1/saved-resources/{id}/share-candidates": probe(func(m *restrictionEnv) []string {
			return []string{"/v1/saved-resources/" + m.savedResource + "/share-candidates"}
		}),
		"GET /v1/playlists/{id}":                  probe(func(m *restrictionEnv) []string { return []string{"/v1/playlists/" + m.playlist} }),
		"GET /v1/playlists/{id}/content":          probe(func(m *restrictionEnv) []string { return []string{"/v1/playlists/" + m.playlist + "/content"} }),
		"GET /v1/playlists/{id}/order":            probe(func(m *restrictionEnv) []string { return []string{"/v1/playlists/" + m.playlist + "/order"} }),
		"GET /v1/playlists/{id}/share-candidates": probe(func(m *restrictionEnv) []string { return []string{"/v1/playlists/" + m.playlist + "/share-candidates"} }),
		"GET /v1/downloads/preparations":          static("/v1/downloads/preparations"),
		"GET /v1/receivers":                       static("/v1/receivers"),
		"GET /v1/groups":                          static("/v1/groups"),

		// Catalogue: one title.
		"GET /v1/items/{id}":                                perItem(""),
		"GET /v1/items/{id}/detail":                         perItem("/detail"),
		"GET /v1/items/{id}/credits":                        perItem("/credits?group=cast"),
		"GET /v1/items/{id}/art/{kind}":                     perItem("/art/poster"),
		"GET /v1/items/{id}/chapters/{chapter}/image":       perItem("/chapters/0/image"),
		"GET /v1/items/{id}/download-options":               perItem("/download-options"),
		"GET /v1/items/{id}/listening":                      perItem("/listening"),
		"GET /v1/items/{id}/playback-offers":                perItem("/playback-offers"),
		"GET /v1/items/{id}/recommendations":                perItem("/recommendations"),
		"GET /v1/items/{id}/source-availability":            perItem("/source-availability"),
		"GET /v1/items/{id}/trickplay":                      perItem("/trickplay"),
		"GET /v1/items/{id}/trickplay/{set}/thumbnails.vtt": perItem("/trickplay/set/thumbnails.vtt"),
		"GET /v1/items/{id}/trickplay/{set}/tiles/{tile}":   perItem("/trickplay/set/tiles/0"),
		"GET /v1/items/{itemId}/playback-options":           perItem("/playback-options"),
		"GET /v1/topshelf/art/{id}":                         {probe: func(m *restrictionEnv) []string { return prefixed("/v1/topshelf/art/", m.hiddenItemSamples()) }, viewer: "topshelf"},
		"GET /v1/metadata/{kind}/{id}/art/{role}": probe(func(m *restrictionEnv) []string {
			return prefixed("/v1/metadata/item/", m.hiddenItemSamples(), "/art/poster")
		}),
		"GET /v1/metadata/{kind}/{id}/preview": probe(func(m *restrictionEnv) []string {
			return prefixed("/v1/metadata/item/", m.hiddenItemSamples(), "/preview")
		}),

		// Catalogue: containers and people.
		"GET /v1/collections/{id}":           perEntity("/v1/collections/", "collection", ""),
		"GET /v1/collections/{id}/items":     perEntity("/v1/collections/", "collection", "/items"),
		"GET /v1/shows/{id}/episodes":        perEntity("/v1/shows/", "show", "/episodes"),
		"GET /v1/shows/{id}/seasons":         perEntity("/v1/shows/", "show", "/seasons"),
		"GET /v1/shows/{id}/recommendations": perEntity("/v1/shows/", "show", "/recommendations"),
		"GET /v1/shows/{id}/metadata/tvdb":   perEntity("/v1/shows/", "show", "/metadata/tvdb"),
		"GET /v1/seasons/{id}/episodes":      perEntity("/v1/seasons/", "season", "/episodes"),
		"GET /v1/people/{id}":                probe(func(m *restrictionEnv) []string { return prefixed("/v1/people/", m.people) }),
		"GET /v1/people/{id}/portrait":       probe(func(m *restrictionEnv) []string { return prefixed("/v1/people/", m.people, "/portrait") }),

		// Catalogue: one library.
		"GET /v1/libraries/{id}/artists":              perLibrary("/artists"),
		"GET /v1/libraries/{id}/books":                perLibrary("/books"),
		"GET /v1/libraries/{id}/browse":               perLibrary("/browse"),
		"GET /v1/libraries/{id}/browse-capabilities":  perLibrary("/browse-capabilities"),
		"GET /v1/libraries/{id}/categories":           perLibrary("/categories"),
		"GET /v1/libraries/{id}/collections":          perLibrary("/collections"),
		"GET /v1/libraries/{id}/content":              probe(func(m *restrictionEnv) []string { return m.contentPaths() }),
		"GET /v1/libraries/{id}/discover":             perLibrary("/discover"),
		"GET /v1/libraries/{id}/episode-issues":       perLibrary("/episode-issues"),
		"GET /v1/libraries/{id}/facets":               probe(func(m *restrictionEnv) []string { return m.facetPaths() }),
		"GET /v1/libraries/{id}/inventory-status":     perLibrary("/inventory-status"),
		"GET /v1/libraries/{id}/listening/selection":  perLibrary("/listening/selection"),
		"GET /v1/libraries/{id}/metadata/agent":       perLibrary("/metadata/agent"),
		"GET /v1/libraries/{id}/metadata/screen":      perLibrary("/metadata/screen"),
		"GET /v1/libraries/{id}/network-roots":        perLibrary("/network-roots"),
		"GET /v1/libraries/{id}/show-workspace":       probe(func(m *restrictionEnv) []string { return m.showWorkspacePaths() }),
		"GET /v1/libraries/{id}/shows":                perLibrary("/shows"),
		"GET /v1/libraries/{id}/strm-analysis-policy": perLibrary("/strm-analysis-policy"),

		// Live TV, guide and recordings: channels and recordings carry their own
		// restriction (channel_restriction_test, recordings_switch_test); the
		// matrix still checks that no catalogue title leaks through them.
		"GET /v1/guide":                 static("/v1/guide?kind=live-source&start=2026-09-19T12:00:00Z&end=2026-09-19T14:00:00Z&timezone=UTC&limit=30"),
		"GET /v1/guide/channels":        static("/v1/guide/channels?kind=all&sort=number&offset=0&limit=50"),
		"GET /v1/guide/sources":         static("/v1/guide/sources"),
		"GET /v1/guide/images/{digest}": exempt("image bytes addressed by content digest; a programme the profile may not see is published without its image path"),
		"GET /v1/dvr":                   static("/v1/dvr"),
		"GET /v1/dvr/channels":          static("/v1/dvr/channels"),
		"GET /v1/dvr/storage":           static("/v1/dvr/storage"),
		"GET /v1/dvr/recordings/{id}":   probe(func(m *restrictionEnv) []string { return prefixed("/v1/dvr/recordings/", m.hiddenItemSamples()) }),
		"GET /v1/dvr/recordings/{id}/delete-preview": probe(func(m *restrictionEnv) []string {
			return prefixed("/v1/dvr/recordings/", m.hiddenItemSamples(), "/delete-preview")
		}),
		"GET /v1/library-channels/{id}/logo": exempt("channel logo image; library channels apply the profile restriction to their schedule (channel_restriction_test)"),
	}
}

// restrictionReadPOSTs are the POST routes whose answer is catalogue content.
func restrictionReadPOSTs(m *restrictionEnv) map[string][]postProbe {
	browse := []postProbe{}
	if m != nil {
		for _, library := range m.libraries {
			for _, pivot := range []string{"movies", "shows", "episodes", "collections", "categories", "artists", "albums", "songs", "genres", "authors", "books", "series"} {
				browse = append(browse, postProbe{"/v1/libraries/" + library + "/browse", fmt.Sprintf(`{"pivot":%q,"limit":200}`, pivot)})
			}
		}
	}
	jobs := []postProbe{}
	if m != nil {
		jobs = append(jobs, postProbe{"/v1/jobs", fmt.Sprintf(`{"operationId":"matrix-job","command":"personal-state","selector":{"items":{"ids":[%q]}},"args":{"watched":true}}`, m.visibleMovie)})
	}
	return map[string][]postProbe{"POST /v1/libraries/{id}/browse": browse, "POST /v1/jobs": jobs}
}

func prefixed(prefix string, ids []string, suffix ...string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, prefix+id+strings.Join(suffix, ""))
	}
	return out
}

type restrictionEnv struct {
	downloads  *downloads.Service
	db         *sql.DB
	catalog    *catalog.Service
	fixtures   *catalogtest.Catalog
	names      catalogtest.Names
	itemsByID  map[string]catalogtest.Item
	principals map[string]identity.Principal
	bulkAccess catalog.BulkAccess

	call            func(method, path, body, viewer string) (int, string)
	callWithin      func(method, path, viewer string, limit time.Duration) (int, string)
	libraries       []string
	hiddenItems     map[string][]string // item kind -> hidden ids
	hiddenEntities  map[string][]string // entity kind -> hidden ids
	visibleEntities map[string][]string
	visibleMovie    string
	people          []string
	playlist        string
	savedResource   string
	markers         []string
	statuses        map[string][]int
	marker          *regexp.Regexp
}

func (m *restrictionEnv) hiddenItemSamples() []string {
	out := []string{}
	kinds := make([]string, 0, len(m.hiddenItems))
	for kind := range m.hiddenItems {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	for _, kind := range kinds {
		out = append(out, m.hiddenItems[kind][0])
	}
	return out
}

func (m *restrictionEnv) homeRows() []string {
	rows := []string{"continue", "continue_listening", "ondeck", "recommended", "trending"}
	for _, library := range m.libraries {
		rows = append(rows, "recent_"+library)
	}
	return prefixed("/v1/home/rows/", rows, "?limit=100")
}

func (m *restrictionEnv) searchPaths() []string {
	out := []string{}
	for _, term := range []string{"Word001", "Word002", "Volume", "Release", "Person", "movie", "episode"} {
		out = append(out, "/v1/search?q="+url.QueryEscape(term)+"&limit=100")
	}
	return out
}

// showWorkspacePaths opens the show workspace on hidden and visible shows.
func (m *restrictionEnv) showWorkspacePaths() []string {
	out := []string{}
	for _, library := range m.libraries {
		for _, id := range append(append([]string{}, m.hiddenEntities["show"]...), m.visibleEntities["show"]...) {
			out = append(out, "/v1/libraries/"+library+"/show-workspace?showId="+url.QueryEscape(id))
		}
	}
	return out
}

// contentPaths covers the library content page and its entity views, each
// opened on a hidden container and on a visible one.
func (m *restrictionEnv) contentPaths() []string {
	out := []string{}
	for _, library := range m.libraries {
		out = append(out, "/v1/libraries/"+library+"/content?limit=200")
		for _, view := range []string{"collection", "show", "season", "artist", "album", "book"} {
			for _, id := range append(append([]string{}, m.hiddenEntities[view]...), m.visibleEntities[view]...) {
				out = append(out, "/v1/libraries/"+library+"/content?limit=200&view="+view+"&entityId="+url.QueryEscape(id))
			}
		}
	}
	return out
}

func (m *restrictionEnv) facetPaths() []string {
	out := []string{}
	for _, library := range m.libraries {
		for _, field := range catalog.BrowseFacetFields() {
			out = append(out, "/v1/libraries/"+library+"/facets?field="+field)
		}
	}
	return out
}

// check runs one request as the restricted viewer.
func (m *restrictionEnv) check(t *testing.T, pattern, viewer, method, path, body string) {
	t.Helper()
	status, response := m.call(method, path, body, viewer)
	m.statuses[pattern] = append(m.statuses[pattern], status)
	if status == 401 && viewer == "restricted" {
		// CD-51: the session is valid; a refusal is 403 or 404, and a 401 would
		// send the person back to the sign-in screen.
		t.Errorf("%s: %s %s answered 401 to a valid restricted session: %s", pattern, method, path, strings.TrimSpace(response))
		return
	}
	if status >= 400 {
		return
	}
	if found := m.marker.FindString(response); found != "" {
		// PORTICO_MATRIX_DUMP=<dir> keeps each leaking answer for inspection.
		if dump := os.Getenv("PORTICO_MATRIX_DUMP"); dump != "" {
			_ = os.WriteFile(filepath.Join(dump, strings.NewReplacer("/", "_", "?", "_", "&", "_", "=", "_").Replace(path)+".json"), []byte(response), 0o600)
		}
		snippet := response
		if i := strings.Index(response, strings.Trim(found, "\"/?&=,: ")); i >= 0 {
			start, end := max(0, i-120), min(len(response), i+120)
			snippet = response[start:end]
		}
		t.Errorf("%s: %s %s answered %d to the restricted profile with a hidden identifier %q: …%s…", pattern, method, path, status, strings.Trim(found, "\"/?&=,: "), snippet)
	}
}

func buildRestrictionMatrix(t *testing.T) *restrictionEnv {
	t.Helper()
	ctx := context.Background()
	// Canonical: the analysis artifact store refuses a symlinked directory
	// (macOS's /var is a link to /private/var).
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "db")
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	libraries, err := fixture.Generate(ctx, db, fixture.Tiny(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ident, err := identity.New(db, root)
	if err != nil {
		t.Fatal(err)
	}
	cat := catalog.New(db)
	catalogFixtures := catalogtest.New(t, db)
	if _, err = cat.ClassifyPendingRatings(ctx); err != nil {
		t.Fatal(err)
	}
	cat.Clock = func() time.Time { return time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC) }
	host, err := hosted.New(db, ident, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	// Every optional service a read route consults is wired, so a route answers
	// on its merits instead of "not configured".
	offline, err := downloads.New(downloads.Options{DB: db, OpenSource: func(_ context.Context, name string, _, _ int64) (io.ReadSeekCloser, error) { return os.Open(name) }})
	if err != nil {
		t.Fatal(err)
	}
	// The fixture has no media files: analysis opens nothing, so a route
	// reaches its restriction check and then finds no artifact.
	analysis, err := mediaanalysis.New(mediaanalysis.Options{DB: db, Directory: filepath.Join(root, "analysis"),
		Open: func(context.Context, string, string) (mediaanalysis.Input, error) { return nil, sql.ErrNoRows },
		Decoder: func(mediaanalysis.Input) mediaanalysis.Decode {
			return func(context.Context, string, decoder.AnalysisSpec, func(io.Reader) error) error { return sql.ErrNoRows }
		}})
	if err != nil {
		t.Fatal(err)
	}
	live, err := livechannels.New(db)
	if err != nil {
		t.Fatal(err)
	}
	// A live source every member may see, whose guide holds a TV-MA programme
	// and a TV-Y one in the probed window: the Kids path (Channels spec §8.1).
	liveOwner := func(context.Context, *sql.Tx, bool) (string, func(string, string) bool, error) {
		return "matrix-live", func(string, string) bool { return true }, nil
	}
	liveSource, err := live.Save(ctx, liveOwner, livechannels.SourceInput{ID: strings.Repeat("7a", 24), RequestID: strings.Repeat("7b", 24), Name: "Matrix antenna", TunerCount: 1,
		Playlist: "#EXTM3U\n#EXTINF:-1 tvg-id=\"m\" tvg-chno=\"5\",Matrix Five\nhttps://provider.invalid/m\n",
		Guide:    `<?xml version="1.0"?><tv><programme channel="m" start="20260919120000 +0000" stop="20260919130000 +0000"><title>MatrixAdultProgramme</title><rating system="VCHIP"><value>TV-MA</value></rating></programme><programme channel="m" start="20260919130000 +0000" stop="20260919140000 +0000"><title>MatrixKidsProgramme</title><rating system="VCHIP"><value>TV-Y</value></rating></programme></tv>`})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO live_source_settings(source_id,viewer_access) VALUES(?,'server-members') ON CONFLICT(source_id) DO UPDATE SET viewer_access='server-members'`, liveSource.ID); err != nil {
		t.Fatal(err)
	}
	recordings, err := dvr.New(db, live, recordingaccess.Policy{Cached: host}.Durable)
	if err != nil {
		t.Fatal(err)
	}
	scheduler := operations.NewScheduler(operations.New(db))
	handler := New(Dependencies{Scheduler: scheduler, DB: db, Identity: ident, Catalog: cat, Hosted: host, Playback: playback.New(db), Downloads: offline, Metadata: metadata.New(db, ""), Analysis: analysis, LiveChannels: live, DVR: recordings})

	all := []string{libraries.Movies, libraries.Shows, libraries.Music, libraries.Books}
	allJSON, _ := json.Marshal(all)
	restriction := fixture.Restriction(libraries)
	narrowed, _ := json.Marshal(restriction.Libraries)
	blocked, _ := json.Marshal(restriction.BlockedLabels)
	if _, err = db.Exec(`INSERT INTO accounts VALUES('matrix-owner','matrix-owner',x'00','matrix-owner-profile',1)`); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{}
	principals := map[string]identity.Principal{}
	for _, viewer := range []struct {
		name, account, profile, libraries string
	}{
		{"open", "matrix-open", "matrix-open-profile", string(allJSON)},
		{"restricted", "matrix-limited", "matrix-limited-profile", string(narrowed)},
	} {
		if _, err = db.Exec(`INSERT INTO accounts VALUES(?,?,x'00',?,1)`, viewer.account, viewer.account, viewer.profile); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO direct_memberships(account_id,role,allowed_libraries,revision,disabled) VALUES(?,'member',?,1,0)
 ON CONFLICT(account_id) DO UPDATE SET role='member',allowed_libraries=excluded.allowed_libraries,disabled=0`, viewer.account, string(allJSON)); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(`INSERT INTO direct_profiles(id,account_id,name,is_primary,position,allowed_libraries) VALUES(?,?,?,1,0,?)
 ON CONFLICT(id) DO UPDATE SET account_id=excluded.account_id,is_primary=1,deleted=0,allowed_libraries=excluded.allowed_libraries`,
			viewer.profile, viewer.account, viewer.name, viewer.libraries); err != nil {
			t.Fatal(err)
		}
		envelope, issueErr := ident.Issue(viewer.account, viewer.profile, "local", "member", 1)
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		tokens[viewer.name] = envelope.AccessToken
		p, authErr := ident.Authenticate(envelope.AccessToken)
		if authErr != nil {
			t.Fatal(authErr)
		}
		principals[viewer.name] = p
	}
	restricted := principals["restricted"]
	key := identity.PersonalKey(restricted.Viewer)

	// The restricted profile's own history, lists and saved things reach hidden
	// titles too: state recorded before the restriction, or on a title that has
	// since been re-rated. Copy one fixture profile's personal state.
	for _, q := range []string{
		`INSERT INTO personal_items(profile_id,item_id,watched,favorite,watchlisted,last_played_at,revision) SELECT ?,item_id,watched,favorite,watchlisted,last_played_at,revision FROM personal_items WHERE profile_id='fixture-profile-000'`,
		`INSERT INTO progress(profile_id,item_id,position,playback_id) SELECT ?,item_id,position,playback_id FROM progress WHERE profile_id='fixture-profile-000'`,
		`INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) SELECT ?,library_id,item_id,updated_at,state FROM progress_activity WHERE profile_id='fixture-profile-000'`,
	} {
		if _, err = db.Exec(q, key); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO profile_restrictions(profile_id,maximum_age,blocked_labels,allow_unrated,revision) VALUES(?,?,?,0,1)`,
		restricted.ProfileID, restriction.MaximumAge, string(blocked)); err != nil {
		t.Fatal(err)
	}
	// An approved tvOS device of the restricted account, for the Top Shelf token.
	if _, err = db.Exec(`INSERT INTO identity_devices(id,authority,account_id,installation_id,name,platform,app,app_version,first_seen,last_seen) VALUES('matrix-device','local',?,'matrixinstallation0000000000000000001','Apple TV','tvos','portico','1',?,?)`,
		restricted.AccountID, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	restrictions, err := ident.ViewerRestrictions(ctx, restricted)
	if err != nil {
		t.Fatal(err)
	}
	oracle := catalog.Viewer{Profile: key, Libraries: restriction.Libraries, Restrictions: restrictions}
	open := catalog.Viewer{Profile: identity.PersonalKey(principals["open"].Viewer), Libraries: all}
	// The permission oracle reads compact item and membership facts. Publish
	// the fixture before deriving its hidden/visible marker sets.
	catalogFixtures.Drain()

	m := &restrictionEnv{downloads: offline, db: db, catalog: cat, fixtures: catalogFixtures, names: catalogtest.Names{}, itemsByID: map[string]catalogtest.Item{}, principals: principals, bulkAccess: (Dependencies{DB: db, Identity: ident, Hosted: host}).bulkAccess, statuses: map[string][]int{}, libraries: all, hiddenItems: map[string][]string{}, hiddenEntities: map[string][]string{}, visibleEntities: map[string][]string{}}
	rows, err := db.Query(`SELECT e.id,pid(e.public_id),k.name,i.source_key FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind JOIN catalog_identities i ON i.public_id=e.public_id WHERE k.playable=1 ORDER BY e.id`)
	if err != nil {
		t.Fatal(err)
	}
	type row struct {
		id, public, kind, key string
	}
	items := []row{}
	for rows.Next() {
		var r row
		if err = rows.Scan(&r.id, &r.public, &r.kind, &r.key); err != nil {
			t.Fatal(err)
		}
		items = append(items, r)
		itemID, _ := strconv.ParseInt(r.id, 10, 64)
		fixtureItem := catalogtest.Item{ID: itemID, Public: r.public}
		m.names[r.key] = fixtureItem
		m.itemsByID[r.public] = fixtureItem
	}
	rows.Close()
	hidden := []string{}
	for _, item := range items {
		if cat.VisibleItem(ctx, open, item.public) != nil {
			continue // not visible to anyone: not a meaningful marker
		}
		switch err := cat.VisibleItem(ctx, oracle, item.public); {
		case errors.Is(err, sql.ErrNoRows):
			m.hiddenItems[item.kind] = append(m.hiddenItems[item.kind], item.public)
			hidden = append(hidden, item.public)
		case err != nil:
			t.Fatal(err)
		case item.kind == "movie" && m.visibleMovie == "":
			m.visibleMovie = item.public
		}
	}
	entities, err := db.Query(`SELECT e.id,pid(e.public_id),k.name,i.source_key FROM catalog_entities e JOIN catalog_kinds k ON k.id=e.kind JOIN catalog_identities i ON i.public_id=e.public_id WHERE k.browsable=1 ORDER BY e.id`)
	if err != nil {
		t.Fatal(err)
	}
	type entity struct {
		id, public, kind, key string
	}
	all2 := []entity{}
	for entities.Next() {
		var e entity
		if err = entities.Scan(&e.id, &e.public, &e.kind, &e.key); err != nil {
			t.Fatal(err)
		}
		all2 = append(all2, e)
		entityID, _ := strconv.ParseInt(e.id, 10, 64)
		fixtureItem := catalogtest.Item{ID: entityID, Public: e.public}
		m.names[e.key] = fixtureItem
		m.itemsByID[e.public] = fixtureItem
	}
	entities.Close()
	for _, e := range all2 {
		if cat.VisibleEntity(ctx, open, e.kind, e.public) != nil {
			continue
		}
		switch err := cat.VisibleEntity(ctx, oracle, e.kind, e.public); {
		case errors.Is(err, sql.ErrNoRows):
			m.hiddenEntities[e.kind] = append(m.hiddenEntities[e.kind], e.public)
			hidden = append(hidden, e.public)
		case err != nil:
			t.Fatal(err)
		default:
			m.visibleEntities[e.kind] = append(m.visibleEntities[e.kind], e.public)
		}
	}
	// Libraries the profile is not given are hidden too.
	for _, library := range all {
		if !oracle.AllowsLibrary(library) {
			hidden = append(hidden, library)
		}
	}
	if len(m.hiddenItems["movie"]) == 0 || m.visibleMovie == "" || len(m.hiddenItems) < 3 {
		t.Fatalf("fixture gives no meaningful split: hidden %v, visible movie %q", m.hiddenItems, m.visibleMovie)
	}
	// People credited on hidden titles.
	credits, err := db.Query(`SELECT DISTINCT p.token FROM catalog_credits c JOIN catalog_people p ON p.id=c.person_id JOIN catalog_entities e ON e.id=c.entity_id WHERE e.public_id IN(SELECT pid_blob(value) FROM json_each(?)) ORDER BY p.token LIMIT 6`, mustJSON(hidden))
	if err != nil {
		t.Fatal(err)
	}
	for credits.Next() {
		var id string
		if err = credits.Scan(&id); err != nil {
			t.Fatal(err)
		}
		m.people = append(m.people, id)
	}
	credits.Close()

	// A playlist and a saved collection of the restricted profile's own, each
	// holding a hidden and a visible title.
	now := time.Now().UTC().Format(time.RFC3339)
	m.playlist, m.savedResource = "matrix-playlist", "matrix-saved"
	actor := catalog.ResourceActor{Authority: restricted.Authority, AccountID: restricted.AccountID, ProfileID: restricted.ProfileID}
	playlistName := "Mixed"
	createdPlaylist, err := cat.MutatePlaylist(actor, "", "create", "", catalog.PlaylistMutation{OperationID: "matrix-playlist-create", Name: &playlistName}, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.playlist = createdPlaylist.PlaylistID
	addedHidden, err := cat.MutatePlaylist(actor, m.playlist, "add", "", catalog.PlaylistMutation{OperationID: "matrix-playlist-hidden", ExpectedRevision: createdPlaylist.Revision, ItemID: m.hiddenItems["movie"][0]}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cat.MutatePlaylist(actor, m.playlist, "add", "", catalog.PlaylistMutation{OperationID: "matrix-playlist-visible", ExpectedRevision: addedHidden.Revision, ItemID: m.visibleMovie}, nil); err != nil {
		t.Fatal(err)
	}
	for _, q := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO saved_resources(id,kind,owner_key,owner_authority,owner_account,owner_profile,name,created_at) VALUES(?,'collection',?,?,?,?,'Mixed',?)`, []any{m.savedResource, key, restricted.Authority, restricted.AccountID, restricted.ProfileID, now}},
		{`INSERT INTO saved_resource_entries(id,resource_id,item_id) VALUES('matrix-saved-1',?,?),('matrix-saved-2',?,?)`, []any{m.savedResource, m.itemsByID[m.hiddenItems["movie"][0]].ID, m.savedResource, m.itemsByID[m.visibleMovie].ID}},
	} {
		if _, err = db.Exec(q.sql, q.args...); err != nil {
			t.Fatal(err)
		}
	}

	hidden = append(hidden, "MatrixAdultProgramme")
	m.markers = hidden
	quoted := make([]string, 0, len(hidden))
	for _, id := range hidden {
		quoted = append(quoted, regexp.QuoteMeta(id))
	}
	sort.Slice(quoted, func(i, j int) bool { return len(quoted[i]) > len(quoted[j]) })
	m.marker = regexp.MustCompile(`(?:^|[^A-Za-z0-9_-])(?:` + strings.Join(quoted, "|") + `)(?:$|[^A-Za-z0-9_-])`)

	catalogFixtures.Drain()
	m.callWithin = func(method, path, viewer string, limit time.Duration) (int, string) {
		ctx, cancel := context.WithTimeout(context.Background(), limit)
		defer cancel()
		r := httptest.NewRequest(method, path, nil).WithContext(ctx)
		r.Header.Set("Authorization", "Bearer "+tokens[viewer])
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}
	m.call = func(method, path, body, viewer string) (int, string) {
		if viewer == "topshelf" && tokens["topshelf"] == "" {
			// The Top Shelf extension's token, minted by the restricted session.
			r := httptest.NewRequest("POST", "/v1/topshelf/token", bytes.NewBufferString(`{"installationId":"matrixinstallation0000000000000000001"}`))
			r.Header.Set("Authorization", "Bearer "+tokens["restricted"])
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			var minted TopShelfToken
			if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &minted) != nil {
				t.Fatalf("top shelf token: %d %s", w.Code, w.Body.String())
			}
			tokens["topshelf"] = minted.Token
		}
		r := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		r.Header.Set("Authorization", "Bearer "+tokens[viewer])
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w.Code, w.Body.String()
	}

	// Controls: the restricted viewer is a working viewer, and the hidden set
	// is really served to the open viewer, so an absence means something.
	for _, path := range []string{"/v1/home", "/v1/libraries", "/v1/items/" + m.visibleMovie + "/detail"} {
		if status, response := m.call("GET", path, "", "restricted"); status != 200 {
			t.Fatalf("control: restricted viewer %s: %d %s", path, status, response)
		}
	}
	if status, response := m.call("GET", "/v1/libraries/"+libraries.Movies+"/content?limit=200", "", "open"); status != 200 || !m.marker.MatchString(response) {
		t.Fatalf("control: the open viewer does not see the hidden titles (%d)", status)
	}
	guideProbe := "/v1/guide?kind=live-source&start=2026-09-19T12:00:00Z&end=2026-09-19T14:00:00Z&timezone=UTC&limit=30"
	if status, response := m.call("GET", guideProbe, "", "open"); status != 200 || !strings.Contains(response, "MatrixAdultProgramme") {
		t.Fatalf("control: the open viewer does not see the live guide (%d) %s", status, response)
	}
	if status, response := m.call("GET", guideProbe, "", "restricted"); status != 200 || !strings.Contains(response, "MatrixKidsProgramme") {
		t.Fatalf("control: the restricted viewer does not see the live guide (%d) %s", status, response)
	}
	// The restricted profile's visibility classes are built by the server's
	// background rebuilder after first use; the matrix probes a server whose
	// classes serve, not the build window (which answers "building" for every
	// id alike). Build them here, then publish them.
	effective := catalog.Viewer{Restrictions: restrictions, MemberMaxRating: restrictions.MemberMaxRating, MemberAllowUnrated: restrictions.MemberAllowUnrated, MemberDeniedLabels: restrictions.MemberDeniedLabels}.EffectiveRestrictions()
	for _, library := range m.libraries {
		if err := cat.RebuildVisibilityClass(ctx, library, effective); err != nil {
			t.Fatal(err)
		}
	}
	catalogFixtures.Drain()
	return m
}

func mustJSON(v any) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
