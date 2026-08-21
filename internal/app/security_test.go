package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestErrorsUseCorrelatedProblemDetails(t *testing.T) {
	serverURL := newAuthTestServer(t)
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set(requestIDHeader, "review-request-42")
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get(requestIDHeader) != "review-request-42" {
		t.Fatalf("request id header = %q", resp.Header.Get(requestIDHeader))
	}
	if contentType := resp.Header.Get("Content-Type"); !strings.HasPrefix(contentType, "application/problem+json") {
		t.Fatalf("content type = %q", contentType)
	}
	var problem struct {
		Type      string `json:"type"`
		Status    int    `json:"status"`
		Code      string `json:"code"`
		Detail    string `json:"detail"`
		RequestID string `json:"requestId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem.Status != http.StatusForbidden || problem.Code != "origin_not_allowed" || problem.Detail == "" || problem.RequestID != "review-request-42" {
		t.Fatalf("unexpected problem: %#v", problem)
	}
}

func TestInvalidRequestIDIsReplaced(t *testing.T) {
	serverURL := newAuthTestServer(t)
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set(requestIDHeader, "bad request id with spaces")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	requestID := resp.Header.Get(requestIDHeader)
	if requestID == "" || requestID == "bad request id with spaces" || !requestIDPattern.MatchString(requestID) {
		t.Fatalf("replacement request id = %q", requestID)
	}
}

func TestSecurityHeadersRejectDisallowedOrigin(t *testing.T) {
	serverURL := newAuthTestServer(t)
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, expected 403", resp.StatusCode)
	}
}

func TestSecurityHeadersDoNotTreatWildcardAsCredentialedOrigin(t *testing.T) {
	server := &Server{cfg: config.Config{AllowedOrigins: []string{"*"}}}
	req := httptest.NewRequest(http.MethodPost, "http://portico.local/api/auth/login", nil)
	req.Header.Set("Origin", "https://evil.example")
	if server.isAllowedOriginForRequest("https://evil.example", req) {
		t.Fatal("wildcard origin enabled credentialed cross-origin access")
	}
}

func TestSecurityHeadersAllowSameOrigin(t *testing.T) {
	serverURL := newAuthTestServer(t)
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Origin", serverURL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, expected 200", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != serverURL {
		t.Fatalf("allow origin = %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestSecurityHeadersAllowLoopbackWebDevOrigin(t *testing.T) {
	serverURL := newAuthTestServer(t)
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Origin", "http://127.0.0.1:19105")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, expected 200", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "http://127.0.0.1:19105" {
		t.Fatalf("allow origin = %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestSecurityHeadersRejectNonLoopbackWebDevOrigin(t *testing.T) {
	serverURL := newAuthTestServer(t)
	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Origin", "http://192.168.1.20:19105")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, expected 403", resp.StatusCode)
	}
}

func TestSecurityHeadersAllowHostedDirectRoutesInCSP(t *testing.T) {
	serverURL := newAuthTestServer(t)
	resp, err := http.Get(serverURL + "/")
	if err != nil {
		t.Fatalf("request app shell: %v", err)
	}
	defer resp.Body.Close()
	csp := resp.Header.Get("Content-Security-Policy")
	for _, directive := range []string{
		"connect-src 'self' https://api.getportico.tv https://*.direct.getportico.tv",
		"img-src 'self' data: https://image.tmdb.org https://*.direct.getportico.tv",
		"media-src 'self' blob: https://*.direct.getportico.tv",
	} {
		if !strings.Contains(csp, directive) {
			t.Fatalf("CSP missing %q in %q", directive, csp)
		}
	}
	if strings.Contains(csp, "http://[::1]:*") {
		t.Fatalf("CSP contains browser-invalid IPv6 localhost source: %q", csp)
	}
}

func TestSecurityHeadersRequireCSRFHeaderForBrowserMutations(t *testing.T) {
	serverURL := newAuthTestServer(t)

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/auth/login", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Origin", serverURL)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status without csrf = %d, expected 403", resp.StatusCode)
	}

	req, err = http.NewRequest(http.MethodPost, serverURL+"/api/auth/login", nil)
	if err != nil {
		t.Fatalf("create csrf request: %v", err)
	}
	req.Header.Set("Origin", serverURL)
	req.Header.Set(csrfHeaderName, "1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send csrf request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		t.Fatalf("csrf-protected same-origin mutation was rejected")
	}
}

func TestSecurityHeadersRequireCSRFForCookieMutationWithoutBrowserHeaders(t *testing.T) {
	serverURL := newAuthTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	loginUser(t, client, serverURL)

	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/media/movie_meridian/watched", strings.NewReader(`{"watched":true}`))
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cookie mutation without csrf status = %d, expected 403", resp.StatusCode)
	}
}

func TestBundledServerOverridesHostedRuntimeConfig(t *testing.T) {
	serverURL := newAuthTestServer(t)
	resp, err := http.Get(serverURL + "/portico-config.js")
	if err != nil {
		t.Fatalf("runtime config request: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read runtime config: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("runtime config status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), `mode: "bundled"`) || strings.Contains(string(body), `mode: "local"`) || strings.Contains(string(body), `mode: "hosted"`) {
		t.Fatalf("bundled runtime config should force bundled mode, got: %s", body)
	}
}

func TestRetiredLocalBridgeIsNotServedOrFrameable(t *testing.T) {
	serverURL := newAuthTestServer(t)
	resp, err := http.Get(serverURL + "/portico-local-bridge.html")
	if err != nil {
		t.Fatalf("retired bridge request: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("retired bridge status=%d body=%s", resp.StatusCode, body)
	}
	if strings.Contains(string(body), "portico.localBridge") {
		t.Fatalf("retired bridge implementation is still served: %s", body)
	}
	if frame := resp.Header.Get("X-Frame-Options"); frame != "DENY" {
		t.Fatalf("retired bridge response X-Frame-Options=%q, want DENY", frame)
	}
	if csp := resp.Header.Get("Content-Security-Policy"); !strings.Contains(csp, "frame-ancestors 'none'") || strings.Contains(csp, "web.getportico.tv") {
		t.Fatalf("retired bridge response CSP=%q", csp)
	}
}

func TestStaticHandlerDoesNotServeAppShellForMissingAssets(t *testing.T) {
	webDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(webDir, "index.html"), []byte("<!doctype html><title>Portico</title>"), 0o644); err != nil {
		t.Fatalf("write app shell: %v", err)
	}
	server := &Server{cfg: config.Config{WebDistDir: webDir}}

	assetRecorder := httptest.NewRecorder()
	server.handleStatic(assetRecorder, httptest.NewRequest(http.MethodGet, "/assets/SearchRoutes-missing.js", nil))
	if assetRecorder.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d, expected 404", assetRecorder.Code)
	}
	if strings.Contains(assetRecorder.Body.String(), "<!doctype html>") {
		t.Fatalf("missing asset should not receive the app shell")
	}

	routeRecorder := httptest.NewRecorder()
	server.handleStatic(routeRecorder, httptest.NewRequest(http.MethodGet, "/search?person=Robert+Foulk", nil))
	if routeRecorder.Code != http.StatusOK {
		t.Fatalf("app route status = %d, expected 200", routeRecorder.Code)
	}
	if !strings.Contains(routeRecorder.Body.String(), "<!doctype html>") {
		t.Fatalf("app route should receive the app shell")
	}
}

func TestSecurityHeadersRejectFetchMetadataCrossSiteMutation(t *testing.T) {
	serverURL := newAuthTestServer(t)
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/auth/login", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set(csrfHeaderName, "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, expected 403", resp.StatusCode)
	}
}

func TestSecurityHeadersAllowHostedOriginCrossSiteMutationWithCSRF(t *testing.T) {
	serverURL := newAuthTestServer(t)
	req, err := http.NewRequest(http.MethodPost, serverURL+"/api/auth/login", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Origin", "https://web.getportico.tv")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set(csrfHeaderName, "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusForbidden {
		t.Fatalf("hosted cross-site mutation with csrf was rejected")
	}
	if resp.Header.Get("Access-Control-Allow-Origin") != "https://web.getportico.tv" {
		t.Fatalf("allow origin = %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestHostedWebEnvironmentOriginMatrixIsExact(t *testing.T) {
	server := &Server{cfg: config.Config{AllowedOrigins: config.Load().AllowedOrigins}}
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/auth/login", nil)
	for _, origin := range []string{"https://web.getportico.tv", "https://beta-web.getportico.tv"} {
		if !server.isAllowedOriginForRequest(origin, req) {
			t.Fatalf("trusted hosted origin rejected: %s", origin)
		}
	}
	for _, origin := range []string{"https://hostile.example", "https://beta-web.getportico.tv.hostile.example", "http://beta-web.getportico.tv"} {
		if server.isAllowedOriginForRequest(origin, req) {
			t.Fatalf("hostile origin accepted: %s", origin)
		}
	}
}

func TestSecurityHeadersAllowHostedClientCoreRequestID(t *testing.T) {
	serverURL := newAuthTestServer(t)
	const hostedOrigin = "https://web.getportico.tv"

	preflight, err := http.NewRequest(http.MethodOptions, serverURL+"/api/system", nil)
	if err != nil {
		t.Fatalf("create preflight request: %v", err)
	}
	preflight.Header.Set("Origin", hostedOrigin)
	preflight.Header.Set("Access-Control-Request-Method", http.MethodGet)
	preflight.Header.Set("Access-Control-Request-Headers", "authorization, content-type, x-request-id, "+strings.ToLower(restoreStatusHeader))
	preflightResponse, err := http.DefaultClient.Do(preflight)
	if err != nil {
		t.Fatalf("send preflight request: %v", err)
	}
	defer preflightResponse.Body.Close()
	if preflightResponse.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, expected 204", preflightResponse.StatusCode)
	}
	if preflightResponse.Header.Get("Access-Control-Allow-Origin") != hostedOrigin {
		t.Fatalf("preflight allow origin = %q", preflightResponse.Header.Get("Access-Control-Allow-Origin"))
	}
	if !headerValueContainsToken(preflightResponse.Header.Get("Access-Control-Allow-Headers"), requestIDHeader) {
		t.Fatalf("preflight allow headers = %q, expected %s", preflightResponse.Header.Get("Access-Control-Allow-Headers"), requestIDHeader)
	}
	if !headerValueContainsToken(preflightResponse.Header.Get("Access-Control-Allow-Headers"), restoreStatusHeader) {
		t.Fatalf("preflight allow headers = %q, expected %s", preflightResponse.Header.Get("Access-Control-Allow-Headers"), restoreStatusHeader)
	}

	req, err := http.NewRequest(http.MethodGet, serverURL+"/api/auth/me", nil)
	if err != nil {
		t.Fatalf("create request: %v", err)
	}
	req.Header.Set("Origin", hostedOrigin)
	req.Header.Set(requestIDHeader, "hosted-client-core-42")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.Header.Get(requestIDHeader) != "hosted-client-core-42" {
		t.Fatalf("request id header = %q", resp.Header.Get(requestIDHeader))
	}
	if !headerValueContainsToken(resp.Header.Get("Access-Control-Expose-Headers"), requestIDHeader) {
		t.Fatalf("exposed headers = %q, expected %s", resp.Header.Get("Access-Control-Expose-Headers"), requestIDHeader)
	}
	if !headerValueContainsToken(resp.Header.Get("Access-Control-Expose-Headers"), csrfHeaderName) {
		t.Fatalf("exposed headers = %q, expected %s", resp.Header.Get("Access-Control-Expose-Headers"), csrfHeaderName)
	}
}

func headerValueContainsToken(value string, token string) bool {
	for _, candidate := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(candidate), token) {
			return true
		}
	}
	return false
}

func TestForwardedOriginHeadersRequireTrustedProxy(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://internal.example.test/api/auth/login", nil)
	req.Host = "internal.example.test"
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("X-Forwarded-Host", "proxy.example.test")

	untrusted := &Server{cfg: config.Config{}}
	if untrusted.isAllowedOriginForRequest("https://internal.example.test", req) {
		t.Fatal("untrusted forwarded proto was accepted for same-host origin")
	}
	if origin := untrusted.requestPublicOrigin(req); origin != "http://internal.example.test" {
		t.Fatalf("untrusted request origin = %q", origin)
	}

	trusted := &Server{cfg: config.Config{TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}}
	if !trusted.isAllowedOriginForRequest("https://internal.example.test", req) {
		t.Fatal("trusted forwarded proto was not accepted for same-host origin")
	}
	if origin := trusted.requestPublicOrigin(req); origin != "https://proxy.example.test" {
		t.Fatalf("trusted request origin = %q", origin)
	}
}

func TestRecordLogRedactsActiveSecrets(t *testing.T) {
	server := newScannerTestServer(t)
	event := server.recordLog("info", "Quick Connect approved", map[string]string{
		"code":        "ABCD-2345",
		"approvalURL": "https://media.example.test/#/settings/devices?quickConnectCode=ABCD-2345&token=abc123",
		"device":      "Living Room ABCD-2345",
	})
	if event.Fields["code"] != "[redacted]" {
		t.Fatalf("code field was not redacted: %#v", event.Fields)
	}
	for key, value := range event.Fields {
		if strings.Contains(value, "ABCD-2345") || strings.Contains(value, "abc123") {
			t.Fatalf("%s leaked a secret value: %q", key, value)
		}
	}
}
