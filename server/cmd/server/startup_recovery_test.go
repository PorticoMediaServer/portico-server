package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStartupFailurePageStaysReachable(t *testing.T) {
	reporter := newStartupReporter()
	done := reporter.step("database", "Open the database")
	problem := errors.New("permission denied: /private/library?token=secret")
	done(problem)
	reporter.fail(problem)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stopped := make(chan struct{})
	go func() { serveStartupRecovery(ctx, listener, reporter); close(stopped) }()
	request, _ := http.NewRequest("GET", "http://"+listener.Addr().String()+"/", nil)
	request.Header.Set("Accept", "text/html")
	response, err := (&http.Client{Timeout: time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 503 || !strings.Contains(string(body), "permission") || !strings.Contains(string(body), "restart") || strings.Contains(string(body), "secret") {
		t.Fatalf("recovery=%d %s", response.StatusCode, body)
	}
	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("recovery failed to stop")
	}
}

func TestRecoveryCopyMatchesStringCatalogue(t *testing.T) {
	source, err := os.ReadFile("../../../packages/i18n/src/catalog/en-US/settings.ts")
	if err != nil {
		t.Fatal(err)
	}
	for id, message := range startupCopy {
		key, _ := json.Marshal(id)
		value, _ := json.Marshal(message)
		if !strings.Contains(string(source), string(key)+": "+string(value)) {
			t.Errorf("recovery copy %s is missing or differs from the catalogue", id)
		}
	}
}

func TestRecoveryTranslationsMatchTheTranslatedCatalogues(t *testing.T) {
	for lang, copy := range translatedStartupCopy {
		raw, err := os.ReadFile("../../../packages/i18n/translations/" + lang + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var record struct {
			Messages map[string]struct{ Text string } `json:"messages"`
		}
		if err := json.Unmarshal(raw, &record); err != nil {
			t.Fatal(err)
		}
		for id := range startupCopy {
			if copy[id] == "" || copy[id] != record.Messages[id].Text {
				t.Errorf("%s recovery copy %s is missing or differs from translations/%s.json", lang, id, lang)
			}
		}
	}
}

func TestRecoveryPageFollowsAcceptLanguage(t *testing.T) {
	recovery := &startupRecovery{Phase: startupCopy["server.startup.phase"], PhaseID: "server.startup.phase", Advice: startupCopy["server.startup.space"], AdviceID: "server.startup.space"}
	for header, want := range map[string]string{"": "en", "fr-CA,fr;q=0.9,en;q=0.8": "fr", "de-AT": "de", "es-419": "es", "ru-RU,ru": "ru", "ja-JP": "en", "en-GB": "en", "not a header;;": "en"} {
		if got := startupLanguage(header); got != want {
			t.Errorf("Accept-Language %q: got %s, want %s", header, got, want)
		}
	}
	request, _ := http.NewRequest("GET", "/", nil)
	request.Header.Set("Accept", "text/html")
	request.Header.Set("Accept-Language", "ru")
	response := httptest.NewRecorder()
	recovery.ServeHTTP(response, request)
	body := response.Body.String()
	if !strings.Contains(body, `lang="ru"`) || !strings.Contains(body, translatedStartupCopy["ru"]["server.startup.space"]) || !strings.Contains(body, translatedStartupCopy["ru"]["server.startup.phase"]) || response.Header().Get("Vary") != "Accept-Language" {
		t.Fatalf("russian recovery page: %s", body)
	}
	// A failed phase's own label is a diagnostic and stays as written; the API reply stays English.
	labelled := &startupRecovery{Phase: "Open and install the database", AdviceID: "server.startup.space"}
	response = httptest.NewRecorder()
	labelled.ServeHTTP(response, request)
	if !strings.Contains(response.Body.String(), "Open and install the database") {
		t.Fatalf("phase label: %s", response.Body.String())
	}
	request.Header.Set("Accept", "application/json")
	response = httptest.NewRecorder()
	recovery.ServeHTTP(response, request)
	if !strings.Contains(response.Body.String(), "Free space on the drive") {
		t.Fatalf("json reply: %s", response.Body.String())
	}
}
