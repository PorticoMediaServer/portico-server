package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

type liveTVProviderRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn liveTVProviderRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestLiveTVLogoMissIsNegativeCached(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	var upstreamRequests int
	providerClient := &http.Client{Transport: liveTVProviderRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		upstreamRequests++
		return &http.Response{
			StatusCode: http.StatusNotFound,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("missing")),
			Request:    request,
		}, nil
	})}

	for attempt := 0; attempt < 2; attempt++ {
		request := httptest.NewRequest(http.MethodGet, "/api/live-tv/logos/test", nil)
		request = request.WithContext(context.WithValue(request.Context(), liveTVHTTPClientContextKey{}, providerClient))
		response := httptest.NewRecorder()
		server.proxyLiveTVLogo(response, request, "https://provider.example.test/missing-logo.png", "")
		if response.Code != http.StatusNoContent {
			t.Fatalf("attempt %d status=%d body=%s", attempt+1, response.Code, response.Body.String())
		}
		if response.Header().Get("Cache-Control") != "private, max-age=300" {
			t.Fatalf("attempt %d cache control=%q", attempt+1, response.Header().Get("Cache-Control"))
		}
	}
	if upstreamRequests != 1 {
		t.Fatalf("upstream requests=%d want=1", upstreamRequests)
	}
}

func TestLiveTVDockerProviderHLSNestedPlaylistAndSegmentThroughHandler(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})

	const (
		sourceID  = "src_provider_hls"
		channelID = "chan_provider_hls"
	)
	providerPrefix := "/" + randomID("gb-news-fixture")
	masterURL := "http://threadfin:34400" + providerPrefix + "/master.m3u8"
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`
		INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at)
		VALUES (?, 'Provider fixture', 'm3u', 1, ?, ?)`, sourceID, now, now); err != nil {
		t.Fatalf("insert Live TV source: %v", err)
	}
	if _, err := db.Exec(`
		INSERT INTO live_tv_channels (
			id, source_id, number, name, stream_url, logo_url, group_title, enabled,
			last_seen_at, created_at, updated_at
		) VALUES (?, ?, '101', 'GB News fixture', ?, 'https://provider.example.test/gb-news-logo.png', 'News', 1, ?, ?, ?)`,
		channelID, sourceID, masterURL, now, now, now); err != nil {
		t.Fatalf("insert Live TV channel: %v", err)
	}

	providerBodies := map[string]struct {
		contentType string
		body        string
	}{
		providerPrefix + "/master.m3u8": {
			contentType: "application/vnd.apple.mpegurl",
			body:        "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=2400000,RESOLUTION=1280x720\n720/index.m3u8\n",
		},
		providerPrefix + "/720/index.m3u8": {
			contentType: "application/vnd.apple.mpegurl",
			body:        "#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXT-X-MEDIA-SEQUENCE:9\n#EXTINF:6.0,\nsegment-9.ts\n",
		},
		providerPrefix + "/720/segment-9.ts": {
			contentType: "video/mp2t",
			body:        "provider-transport-stream-fixture",
		},
	}
	var providerMu sync.Mutex
	providerRequests := []string{}
	providerClient := &http.Client{Transport: liveTVProviderRoundTripFunc(func(request *http.Request) (*http.Response, error) {
		providerMu.Lock()
		providerRequests = append(providerRequests, request.URL.Path)
		providerMu.Unlock()
		fixture, ok := providerBodies[request.URL.Path]
		if !ok {
			return &http.Response{StatusCode: http.StatusNotFound, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("missing fixture")), Request: request}, nil
		}
		header := http.Header{"Content-Type": []string{fixture.contentType}}
		return &http.Response{StatusCode: http.StatusOK, Header: header, Body: io.NopCloser(strings.NewReader(fixture.body)), ContentLength: int64(len(fixture.body)), Request: request}, nil
	})}

	porticoHandler := server.Handler()
	testServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		ctx := context.WithValue(request.Context(), liveTVHTTPClientContextKey{}, providerClient)
		porticoHandler.ServeHTTP(writer, request.WithContext(ctx))
	}))
	defer testServer.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("create cookie jar: %v", err)
	}
	authenticated := &http.Client{Jar: jar}
	loginUser(t, authenticated, testServer.URL)

	var playback PlaybackResponse
	status, body := doJSON(t, authenticated, http.MethodPost, testServer.URL+"/api/live-tv/play", map[string]any{
		"channelId":        channelID,
		"clientInstanceId": "provider-hls-integration",
		"clientProfile":    map[string]any{"supportsHls": true, "supportedContainers": []string{"hls"}},
	}, &playback)
	if status != http.StatusOK {
		t.Fatalf("start Live TV status=%d body=%s", status, body)
	}
	if playback.SessionID == "" || playback.MediaGrant.Token == "" || !playback.IsLive {
		t.Fatalf("incomplete Live TV playback response: %#v", playback)
	}
	if playback.Policy.NetworkClass != playbackNetworkLocal || playback.Policy.LiveHLS == nil || playback.Policy.LiveHLS.CredentialQueryAllowed {
		t.Fatalf("Live TV did not return the resolved generic policy and additive HLS security policy: %#v", playback.Policy)
	}
	if playback.SelectedQualityID != "source" || !strings.Contains(playback.SourceURL, "quality=source") {
		t.Fatalf("Live TV source did not bind its resolved quality: selected=%q source=%q", playback.SelectedQualityID, playback.SourceURL)
	}
	wantLogo := "/api/live-tv/logos/" + channelID
	if playback.Media.Images.Thumb != wantLogo {
		t.Fatalf("Live TV playback logo=%q want=%q", playback.Media.Images.Thumb, wantLogo)
	}

	var queue PlaybackSessionQueueResponse
	status, body = doJSON(t, authenticated, http.MethodGet, testServer.URL+"/api/playback-sessions/"+url.PathEscape(playback.SessionID)+"/queue", nil, &queue)
	if status != http.StatusOK {
		t.Fatalf("read Live TV queue status=%d body=%s", status, body)
	}
	if queue.Current.ID != channelID || queue.CanMutate || queue.Total != 0 || len(queue.Items) != 0 {
		t.Fatalf("unexpected read-only Live TV queue: %#v", queue)
	}
	if queue.Current.Images.Thumb != wantLogo {
		t.Fatalf("Live TV queue logo=%q want=%q", queue.Current.Images.Thumb, wantLogo)
	}

	var restored PlaybackRestoreResponse
	status, body = doJSON(t, authenticated, http.MethodPost, testServer.URL+"/api/playback/active", map[string]any{
		"clientInstanceId": "provider-hls-integration",
		"clientProfile":    map[string]any{"supportsHls": true, "supportedContainers": []string{"hls"}},
	}, &restored)
	if status != http.StatusOK || !restored.Active || restored.Playback == nil {
		t.Fatalf("restore Live TV status=%d body=%s response=%#v", status, body, restored)
	}
	if restored.Playback.Media.Images.Thumb != wantLogo {
		t.Fatalf("restored Live TV logo=%q want=%q", restored.Playback.Media.Images.Thumb, wantLogo)
	}
	// Restore rotates the session's short-lived media grant. Continue the HLS
	// chain with the authoritative restored descriptor.
	playback = *restored.Playback
	status, body = doJSON(t, authenticated, http.MethodPatch, testServer.URL+"/api/playback-sessions/"+url.PathEscape(playback.SessionID)+"/queue", map[string]any{
		"expectedRevision": 0, "action": "clear",
	}, nil)
	if status != http.StatusBadRequest || !strings.Contains(body, "queue_not_supported") {
		t.Fatalf("mutate Live TV queue status=%d body=%s", status, body)
	}

	mediaClient := authenticated
	masterBody := getLiveTVFixtureResource(t, mediaClient, absoluteFixtureURL(testServer.URL, playback.SourceURL), http.StatusOK)
	variantPath := firstHLSResourceLine(masterBody)
	if variantPath == "" || !strings.Contains(variantPath, "/api/live-tv/hls/"+channelID+"/item") || strings.Contains(variantPath, "media_grant=") {
		t.Fatalf("master playlist did not expose a credential-free provider item route:\n%s", masterBody)
	}
	if strings.Contains(variantPath, "uri=") || strings.Contains(variantPath, "threadfin") || strings.Contains(variantPath, encodeRawURLForLeakCheck(masterURL)) {
		t.Fatalf("master playlist exposed the provider URI directly or reversibly:\n%s", masterBody)
	}
	if !strings.Contains(variantPath, "quality=source") {
		t.Fatalf("master playlist did not propagate its grant-bound quality:\n%s", masterBody)
	}

	getLiveTVFixtureResource(t, testServer.Client(), absoluteFixtureURL(testServer.URL, variantPath), http.StatusUnauthorized)

	mediaPlaylistBody := getLiveTVFixtureResource(t, mediaClient, absoluteFixtureURL(testServer.URL, variantPath), http.StatusOK)
	segmentPath := firstHLSResourceLine(mediaPlaylistBody)
	if segmentPath == "" || !strings.Contains(segmentPath, "/api/live-tv/hls/"+channelID+"/item") || strings.Contains(segmentPath, "media_grant=") {
		t.Fatalf("media playlist did not expose a credential-free provider segment route:\n%s", mediaPlaylistBody)
	}
	if strings.Contains(segmentPath, "uri=") || strings.Contains(segmentPath, "threadfin") {
		t.Fatalf("media playlist exposed the provider segment URI:\n%s", mediaPlaylistBody)
	}
	if !strings.Contains(segmentPath, "quality=source") {
		t.Fatalf("media playlist did not propagate its grant-bound quality:\n%s", mediaPlaylistBody)
	}
	segmentBody := getLiveTVFixtureResource(t, mediaClient, absoluteFixtureURL(testServer.URL, segmentPath), http.StatusOK)
	if segmentBody != providerBodies[providerPrefix+"/720/segment-9.ts"].body {
		t.Fatalf("proxied segment body=%q", segmentBody)
	}

	providerMu.Lock()
	requested := strings.Join(providerRequests, ",")
	providerMu.Unlock()
	for _, expected := range []string{providerPrefix + "/master.m3u8", providerPrefix + "/720/index.m3u8", providerPrefix + "/720/segment-9.ts"} {
		if !strings.Contains(requested, expected) {
			t.Fatalf("provider request chain %q omitted %q", requested, expected)
		}
	}
}

func encodeRawURLForLeakCheck(value string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(value))
}

func getLiveTVFixtureResource(t *testing.T, client *http.Client, endpoint string, wantStatus int) string {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read %s: %v", endpoint, err)
	}
	if response.StatusCode != wantStatus {
		t.Fatalf("GET %s status=%d want=%d body=%s", endpoint, response.StatusCode, wantStatus, body)
	}
	return string(body)
}

func firstHLSResourceLine(playlist string) string {
	for _, line := range strings.Split(playlist, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

func absoluteFixtureURL(serverURL, resource string) string {
	if strings.HasPrefix(resource, "http://") || strings.HasPrefix(resource, "https://") {
		return resource
	}
	return strings.TrimRight(serverURL, "/") + "/" + strings.TrimLeft(resource, "/")
}

func stripQueryParameter(t *testing.T, endpoint, key string) string {
	t.Helper()
	parsed, err := url.Parse(endpoint)
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	query := parsed.Query()
	query.Del(key)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func TestLiveTVPlaybackProgressAcceptsOrderedHeartbeat(t *testing.T) {
	_, db, server := newDiscoveryTestServer(t, config.Config{})
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := db.Exec(`INSERT INTO live_tv_sources (id, name, type, enabled, created_at, updated_at) VALUES ('src_live_progress', 'Live progress', 'm3u', 1, ?, ?)`, now, now); err != nil {
		t.Fatalf("insert source: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO live_tv_channels (id, source_id, name, stream_url, enabled, last_seen_at, created_at, updated_at) VALUES ('chan_live_progress', 'src_live_progress', 'Bloomberg fixture', 'https://provider.example.test/bloomberg/master.m3u8', 1, ?, ?, ?)`, now, now, now); err != nil {
		t.Fatalf("insert channel: %v", err)
	}

	httpServer := httptest.NewServer(server.Handler())
	defer httpServer.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, httpServer.URL)
	var playback PlaybackResponse
	status, body := doJSON(t, client, http.MethodPost, httpServer.URL+"/api/live-tv/play", map[string]any{"channelId": "chan_live_progress", "clientProfile": map[string]any{"supportsHls": true, "supportedContainers": []string{"hls"}}}, &playback)
	if status != http.StatusOK {
		t.Fatalf("start Live TV status=%d body=%s", status, body)
	}
	var acknowledgement PlaybackProgressAcknowledgement
	status, body = doJSON(t, client, http.MethodPatch, httpServer.URL+"/api/playback-sessions/"+url.PathEscape(playback.SessionID), map[string]any{
		"eventSequence":   1,
		"recordedAt":      time.Now().UTC().Format(time.RFC3339Nano),
		"state":           "playing",
		"positionSeconds": 0,
		"isPlaying":       true,
	}, &acknowledgement)
	if status != http.StatusOK || !acknowledgement.Accepted {
		t.Fatalf("Live TV heartbeat status=%d accepted=%v body=%s", status, acknowledgement.Accepted, body)
	}
	var state string
	if err := db.QueryRow(`SELECT state FROM playback_sessions WHERE id = ?`, playback.SessionID).Scan(&state); err != nil {
		t.Fatalf("load playback session: %v", err)
	}
	if state != "playing" {
		t.Fatalf("Live TV heartbeat state=%q", state)
	}

	encoded, _ := json.Marshal(acknowledgement)
	if !strings.Contains(string(encoded), `"sessionState":"playing"`) {
		t.Fatalf("unexpected acknowledgement: %s", encoded)
	}
}
