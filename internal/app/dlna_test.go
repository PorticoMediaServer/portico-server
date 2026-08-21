package app

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestDLNABrowseItemsUseBoundedSQLPages(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_dlna_page', 'DLNA Page', 'movie', 955, '/tmp/dlna-page', '{}', ?)`, now); err != nil {
		t.Fatalf("insert dlna library: %v", err)
	}
	for index := 0; index < 12; index++ {
		id := fmt.Sprintf("dlna_movie_%02d", index)
		title := fmt.Sprintf("DLNA Movie %02d", index)
		if _, err := db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, source_url, genres_json, added_at)
			VALUES (?, 'lib_dlna_page', 'movie', ?, ?, ?, '[]', ?)`,
			id, title, title, "https://media.example.test/"+id+".mp4", now); err != nil {
			t.Fatalf("insert dlna media %d: %v", index, err)
		}
	}

	items, total, err := server.dlnaLibraryItems("lib_dlna_page", 5, 3)
	if err != nil {
		t.Fatalf("load dlna library page: %v", err)
	}
	if total != 9 || len(items) != 3 {
		t.Fatalf("dlna page total=%d len=%d, expected conservative total=9 len=3", total, len(items))
	}
	if items[0].ID != "dlna_movie_05" || items[2].ID != "dlna_movie_07" {
		t.Fatalf("unexpected dlna page items: %#v", items)
	}
	items, total, err = server.dlnaLibraryItems("lib_dlna_page", 10, 3)
	if err != nil {
		t.Fatalf("load final dlna library page: %v", err)
	}
	if total != 12 || len(items) != 2 {
		t.Fatalf("final dlna page total=%d len=%d, expected total=12 len=2", total, len(items))
	}

	capped, _, err := server.dlnaLibraryItems("lib_dlna_page", 0, dlnaMaxBrowsePageSize+50)
	if err != nil {
		t.Fatalf("load capped dlna page: %v", err)
	}
	if len(capped) > dlnaMaxBrowsePageSize {
		t.Fatalf("dlna page returned %d items, expected cap %d", len(capped), dlnaMaxBrowsePageSize)
	}
}

func TestDLNABrowseQueriesRespectCancellation(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := server.dlnaLibraryItemsContext(ctx, "lib_movies", 0, 10); !errors.Is(err, context.Canceled) {
		t.Fatalf("dlna library items error = %v, expected context.Canceled", err)
	}
	if _, err := server.dlnaMediaNodesContext(ctx, []MediaItem{{ID: "movie_meridian", Type: "movie", Title: "Meridian"}}, "library:lib_movies", "http://127.0.0.1"); !errors.Is(err, context.Canceled) {
		t.Fatalf("dlna media nodes error = %v, expected context.Canceled", err)
	}
}

func TestDLNAChildCountsAreBatchedAndExact(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO libraries (id, name, type, sort_order, path, settings_json, created_at)
		VALUES ('lib_dlna_child_counts', 'DLNA Child Counts', 'music', 954, '/tmp/dlna-child-counts', '{}', ?)`, now); err != nil {
		t.Fatalf("insert dlna child count library: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, source_url, genres_json, added_at, index_number)
		VALUES
			('dlna_album_many_children', 'lib_dlna_child_counts', NULL, 'album', 'Many Children', 'Many Children', '', '[]', ?, 0),
			('dlna_track_1', 'lib_dlna_child_counts', 'dlna_album_many_children', 'track', 'Track 1', 'Track 1', 'https://media.example.test/1.mp3', '[]', ?, 1),
			('dlna_track_2', 'lib_dlna_child_counts', 'dlna_album_many_children', 'track', 'Track 2', 'Track 2', 'https://media.example.test/2.mp3', '[]', ?, 2),
			('dlna_track_3', 'lib_dlna_child_counts', 'dlna_album_many_children', 'track', 'Track 3', 'Track 3', 'https://media.example.test/3.mp3', '[]', ?, 3)`,
		now, now, now, now); err != nil {
		t.Fatalf("insert dlna child count media: %v", err)
	}
	counts, err := server.dlnaChildCounts([]MediaItem{{ID: "dlna_album_many_children"}})
	if err != nil {
		t.Fatalf("load dlna child counts: %v", err)
	}
	if counts["dlna_album_many_children"] != 3 {
		t.Fatalf("dlna child count = %d, expected exact batched count", counts["dlna_album_many_children"])
	}
}

func TestDLNAChildCountsUseGroupedBatchQuery(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate dlna test source")
	}
	sourcePath := filepath.Join(filepath.Dir(filename), "dlna.go")
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read dlna source: %v", err)
	}
	source := string(sourceBytes)
	start := strings.Index(source, "func (s *Server) dlnaChildCountsContext")
	if start < 0 {
		t.Fatal("dlnaChildCountsContext not found")
	}
	end := strings.Index(source[start:], "\nfunc normalizeDLNABrowseWindow")
	if end < 0 {
		t.Fatal("dlnaChildCountsContext end marker not found")
	}
	body := source[start : start+end]
	if !strings.Contains(body, "GROUP BY parent_id") {
		t.Fatal("dlnaChildCountsContext should use one grouped child-count query")
	}
	if strings.Contains(body, "EXISTS") {
		t.Fatal("dlnaChildCountsContext should return exact counts, not has-children booleans")
	}
}

func TestDLNABrowseAdmissionIsBoundedPerClient(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	req := httptest.NewRequest(http.MethodPost, "/dlna/content-directory", nil)
	req.RemoteAddr = "192.0.2.10:54000"
	releases := make([]func(), 0, dlnaMaxBrowsePerClient)
	for index := 0; index < dlnaMaxBrowsePerClient; index++ {
		release, ok := server.tryAcquireDLNABrowseClient(req)
		if !ok {
			t.Fatalf("dlna browse admission rejected request %d before per-client limit", index)
		}
		releases = append(releases, release)
	}
	if release, ok := server.tryAcquireDLNABrowseClient(req); ok {
		release()
		t.Fatalf("dlna browse admission allowed request beyond per-client limit")
	}
	if rejected := server.dlnaBrowseRejected.Load(); rejected != 1 {
		t.Fatalf("dlna browse rejected count = %d, expected 1", rejected)
	}
	releases[0]()
	release, ok := server.tryAcquireDLNABrowseClient(req)
	if !ok {
		t.Fatalf("dlna browse admission did not recover after release")
	}
	release()
	for _, release := range releases[1:] {
		release()
	}
}

func TestDLNAAdmissionUsesSocketPeerAndEligibleInterface(t *testing.T) {
	privateV4 := netip.MustParsePrefix("192.168.10.0/24")
	privateV6 := netip.MustParsePrefix("fd12:3456::/64")
	if !dlnaPeerEligible(netip.MustParseAddr("192.168.10.20"), netip.MustParseAddr("192.168.10.2"), []netip.Prefix{privateV4}) {
		t.Fatal("same eligible LAN interface rejected")
	}
	if !dlnaPeerEligible(netip.MustParseAddr("fd12:3456::20"), netip.MustParseAddr("fd12:3456::2"), []netip.Prefix{privateV6}) {
		t.Fatal("same eligible IPv6 LAN interface rejected")
	}
	for _, peer := range []string{"10.8.0.20", "192.168.11.20", "203.0.113.20"} {
		if dlnaPeerEligible(netip.MustParseAddr(peer), netip.MustParseAddr("192.168.10.2"), []netip.Prefix{privateV4}) {
			t.Fatalf("unapproved or off-interface peer %s accepted", peer)
		}
	}

	for _, test := range []struct {
		name       string
		remoteAddr string
		forwarded  string
		tls        bool
		allowed    bool
	}{
		{name: "loopback", remoteAddr: "127.0.0.1:1900", allowed: true},
		{name: "public peer", remoteAddr: "203.0.113.20:1900"},
		{name: "forwarded private cannot bless public peer", remoteAddr: "203.0.113.20:1900", forwarded: "192.168.10.20"},
		{name: "reverse proxy", remoteAddr: "127.0.0.1:1900", forwarded: "192.168.10.20"},
		{name: "public TLS listener", remoteAddr: "127.0.0.1:1900", tls: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/dlna/device.xml", nil)
			request.RemoteAddr = test.remoteAddr
			if test.forwarded != "" {
				request.Header.Set("X-Forwarded-For", test.forwarded)
			}
			if test.tls {
				request.TLS = &tls.ConnectionState{}
			}
			if got := dlnaLANRequest(request); got != test.allowed {
				t.Fatalf("dlnaLANRequest=%v want=%v", got, test.allowed)
			}
		})
	}
}

func TestEveryUnauthenticatedDLNAHandlerRejectsPublicTLSAndLANDeviceDescriptionWorks(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	if _, err := db.Exec(`UPDATE settings SET value_json = ? WHERE key = 'dlna'`, `{"enabled":true,"friendlyName":"Portico LAN","exposedLibraries":["lib_movies"]}`); err != nil {
		t.Fatalf("enable DLNA: %v", err)
	}
	handlers := []struct {
		name, method, path string
		handler            http.HandlerFunc
	}{
		{"device", http.MethodGet, "/dlna/device.xml", server.handleDLNADeviceDescription},
		{"content description", http.MethodGet, "/dlna/content-directory.xml", server.handleDLNAContentDirectoryDescription},
		{"connection description", http.MethodGet, "/dlna/connection-manager.xml", server.handleDLNAConnectionManagerDescription},
		{"content control", http.MethodPost, "/dlna/content-directory", server.handleDLNAContentDirectoryControl},
		{"connection control", http.MethodPost, "/dlna/connection-manager", server.handleDLNAConnectionManagerControl},
		{"media", http.MethodGet, "/dlna/media/movie_meridian", server.handleDLNAMedia},
	}
	for _, test := range handlers {
		t.Run(test.name+" rejects TLS", func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.path, strings.NewReader(""))
			request.RemoteAddr = "127.0.0.1:49000"
			request.TLS = &tls.ConnectionState{}
			request.Header.Set("X-Forwarded-For", "192.168.1.44")
			recorder := httptest.NewRecorder()
			test.handler(recorder, request)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}

	request := httptest.NewRequest(http.MethodGet, "/dlna/device.xml", nil)
	request.RemoteAddr = "127.0.0.1:49001"
	request.Host = "public-attacker.example:443"
	recorder := httptest.NewRecorder()
	server.handleDLNADeviceDescription(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "Portico LAN") {
		t.Fatalf("LAN device description status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	if strings.Contains(recorder.Body.String(), "public-attacker.example") {
		t.Fatal("device description reflected an untrusted Host header")
	}
}

func TestDLNARendererProfilesAreVersionedAndProgressiveOnly(t *testing.T) {
	for _, test := range []struct {
		userAgent, id string
	}{
		{"SEC_HHP_[TV] Samsung", "samsung-progressive"},
		{"LG WebOS TV", "lg-progressive"},
		{"UnknownRenderer/1.0", "generic-progressive"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/dlna/device.xml", nil)
		request.Header.Set("User-Agent", test.userAgent)
		profile, err := resolveDLNARendererProfile(request)
		if err != nil {
			t.Fatalf("resolve %q: %v", test.userAgent, err)
		}
		if profile.Version != dlnaRendererProfileV1 || profile.ID != test.id {
			t.Fatalf("profile=%#v", profile)
		}
		if !profile.ReachableRoute["http"] || profile.ReachableRoute["hls"] {
			t.Fatalf("renderer routes=%#v", profile.ReachableRoute)
		}
		if profile.Client.ClientFamily != "dlna" || profile.Client.CapabilitySchemaVersion != playbackCapabilitySchemaV2 {
			t.Fatalf("W5 capability profile=%#v", profile.Client)
		}
	}
	if strings.Contains(dlnaDefaultProtocolInfo, "matroska") || strings.Contains(dlnaDefaultProtocolInfo, "audio/mp4") {
		t.Fatalf("protocol info advertises unsupported route facts: %s", dlnaDefaultProtocolInfo)
	}
}

func TestDLNARemoteOriginRejectsMetadataMixedAnswersAndRebinding(t *testing.T) {
	metadata, _ := url.Parse("http://169.254.169.254/latest/meta-data")
	if _, err := resolveDLNAOrigin(context.Background(), metadata, net.DefaultResolver.LookupIPAddr); err == nil {
		t.Fatal("metadata service literal was accepted")
	}
	origin, _ := url.Parse("https://media.example.test/movie.mp4")
	mixedLookup := func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}
	if _, err := resolveDLNAOrigin(context.Background(), origin, mixedLookup); err == nil {
		t.Fatal("mixed public/private DNS answer was accepted")
	}
	lookupCalls := 0
	rebindingLookup := func(context.Context, string) ([]net.IPAddr, error) {
		lookupCalls++
		if lookupCalls > 1 {
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("93.184.216.34")}}, nil
	}
	pinned, err := resolveDLNAOrigin(context.Background(), origin, rebindingLookup)
	if err != nil || pinned.String() != "93.184.216.34" || lookupCalls != 1 {
		t.Fatalf("origin was not resolved and pinned exactly once: pinned=%s calls=%d err=%v", pinned, lookupCalls, err)
	}
}

func TestDLNARemoteRedirectsStayOnExactAuthority(t *testing.T) {
	origin, _ := url.Parse("https://media.example.test:8443/library/movie.mp4")
	policy := dlnaRedirectPolicy(canonicalDLNAAuthority(origin))
	for _, test := range []struct {
		raw     string
		allowed bool
	}{
		{"https://media.example.test:8443/library/redirected.mp4", true},
		{"https://media.example.test/library/movie.mp4", false},
		{"http://media.example.test:8443/library/movie.mp4", false},
		{"https://cdn.example.test:8443/library/movie.mp4", false},
		{"https://user:secret@media.example.test:8443/library/movie.mp4", false},
	} {
		next, _ := http.NewRequest(http.MethodGet, test.raw, nil)
		next.Header.Set("Cookie", "secret=1")
		err := policy(next, []*http.Request{{URL: origin}})
		if (err == nil) != test.allowed {
			t.Fatalf("redirect %s err=%v allowed=%v", test.raw, err, test.allowed)
		}
		if test.allowed && next.Header.Get("Cookie") != "" {
			t.Fatal("redirect retained credential-bearing headers")
		}
	}
}

func TestDLNAResourcesRequireProbedRendererCompatibleContainerAndCodecs(t *testing.T) {
	profile, err := resolveDLNARendererProfile(nil)
	if err != nil {
		t.Fatalf("renderer profile: %v", err)
	}
	file := func(container, video, audio string) MediaItem {
		return MediaItem{Type: "movie", MediaFiles: []MediaFileVersion{{
			ID: "file", Path: "/library/media.bin", Available: true, Analysis: "analyzed",
			Container: container, VideoCodec: video, AudioCodec: audio, Width: 1920, Height: 1080, AudioChannels: 2,
		}}}
	}
	if resource, ok := dlnaResourceForRenderer(file("mp4", "h264", "aac"), profile); !ok || resource.ContentType != "video/mp4" || !strings.Contains(resource.Protocol, "video/mp4") {
		t.Fatalf("verified MP4 tuple rejected: resource=%#v ok=%v", resource, ok)
	}
	mp3 := file("mp3", "", "mp3")
	mp3.Type = "track"
	if resource, ok := dlnaResourceForRenderer(mp3, profile); !ok || resource.ContentType != "audio/mpeg" {
		t.Fatalf("verified MP3 tuple rejected: resource=%#v ok=%v", resource, ok)
	}
	for name, item := range map[string]MediaItem{
		"matroska bytes": file("mkv", "h264", "aac"),
		"HEVC in MP4":    file("mp4", "hevc", "aac"),
		"AC3 in MP4":     file("mp4", "h264", "ac3"),
		"oversized":      file("mp4", "h264", "aac"),
		"unprobed":       file("mp4", "h264", "aac"),
	} {
		if name == "oversized" {
			item.MediaFiles[0].Width = 3840
		}
		if name == "unprobed" {
			item.MediaFiles[0].Analysis = "unknown"
		}
		if resource, ok := dlnaResourceForRenderer(item, profile); ok {
			t.Fatalf("%s was falsely advertised as %#v", name, resource)
		}
	}
}
