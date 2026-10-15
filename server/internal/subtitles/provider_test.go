package subtitles

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type providerTransport func(*http.Request) (*http.Response, error)

func (f providerTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func providerResponse(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}
func TestProviderEpisodeSearchKeepsIdentityAndRegionalLanguage(t *testing.T) {
	p := &OpenSubtitles{key: "fixture", gate: make(chan struct{}, 1)}
	p.client = &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		q := r.URL.Query()
		if q.Get("parent_imdb_id") != "123" || q.Get("season_number") != "2" || q.Get("episode_number") != "3" || q.Get("languages") != "pt-br" || q.Get("query") != "" {
			t.Fatal(q)
		}
		return providerResponse(200, `{"data":[]}`), nil
	})}
	if _, e := p.SearchMedia(context.Background(), ProviderQuery{Title: "Pilot", Language: "pt-BR", ParentIMDbID: "123", Season: 2, Episode: 3, Page: 1}); e != nil {
		t.Fatal(e)
	}
}
func TestProviderRenewsExpiredTokenAndHonorsLoginHost(t *testing.T) {
	p := &OpenSubtitles{key: "fixture", token: "expired", username: "fixture-user", password: "fixture-password", gate: make(chan struct{}, 1)}
	requests, logins := 0, 0
	p.client = &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) {
		p.mu.Lock()
		p.next = time.Time{}
		p.mu.Unlock()
		if strings.HasSuffix(r.URL.Path, "/login") {
			logins++
			if r.Header.Get("Authorization") != "" {
				t.Fatal("expired bearer sent on login")
			}
			return providerResponse(200, `{"token":"renewed","base_url":"vip-api.opensubtitles.com"}`), nil
		}
		requests++
		if requests == 1 {
			return providerResponse(401, `{}`), nil
		}
		if r.URL.Host != "vip-api.opensubtitles.com" || r.Header.Get("Authorization") != "Bearer renewed" {
			t.Fatal("renewed credentials not used")
		}
		return providerResponse(200, `{"data":[]}`), nil
	})}
	if _, e := p.Search(context.Background(), "Title", "en", 1); e != nil || requests != 2 || logins != 1 {
		t.Fatal(e, requests, logins)
	}
}
func TestProviderRejectedLoginDoesNotRepeatCredentials(t *testing.T) {
	p := &OpenSubtitles{key: "fixture", username: "fixture-user", password: "fixture-password", gate: make(chan struct{}, 1)}
	logins := 0
	p.client = &http.Client{Transport: providerTransport(func(r *http.Request) (*http.Response, error) { logins++; return providerResponse(401, `{}`), nil })}
	for range 2 {
		if _, e := p.Search(context.Background(), "Title", "en", 1); e == nil {
			t.Fatal("bad credentials accepted")
		}
	}
	if logins != 1 || p.Status().Reason != "provider_sign_in_failed" {
		t.Fatal(logins, p.Status())
	}
}

func TestProviderTakesTheOwnersCredentialsAndFallsBackToTheEnvironment(t *testing.T) {
	p := &OpenSubtitles{key: "env-key", environment: openSubtitlesCredentials{key: "env-key"}, credentials: openSubtitlesCredentials{key: "env-key"}, gate: make(chan struct{}, 1)}
	if !p.Status().Enabled {
		t.Fatal("the environment's key should be in force")
	}
	p.SetCredentials("owner-key", "owner", "secret")
	p.adoptCredentials()
	if p.key != "owner-key" || p.username != "owner" || p.password != "secret" || p.token != "" {
		t.Fatalf("owner credentials not adopted: %q %q", p.key, p.username)
	}
	p.loginRejected.Store(true)
	p.SetCredentials("", "", "")
	p.adoptCredentials()
	if p.key != "env-key" || p.username != "" || p.loginRejected.Load() {
		t.Fatalf("clearing should fall back to the environment: %q %q", p.key, p.username)
	}
	empty := &OpenSubtitles{gate: make(chan struct{}, 1)}
	empty.SetCredentials("", "", "")
	if empty.Status().Reason != "provider_not_configured" {
		t.Fatalf("no key anywhere: %+v", empty.Status())
	}
	if _, err := empty.Search(context.Background(), "x", "en", 1); err != ErrUnavailable {
		t.Fatalf("an unconfigured provider must not call out: %v", err)
	}
}
