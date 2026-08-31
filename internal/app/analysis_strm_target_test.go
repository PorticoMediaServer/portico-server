package app

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

type strmRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn strmRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return fn(request)
}

func TestSTRMTargetAnalysisTierGatesDescriptorReads(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]any
		allowed  bool
	}{
		{name: "file-list-only", settings: map[string]any{"analysisTier": analysisTierFileListOnly}},
		{name: "basic", settings: map[string]any{"analysisTier": analysisTierBasic}},
		{name: "custom-off", settings: map[string]any{"analysisTier": analysisTierCustom, "probeStreams": true}},
		{name: "custom-on", settings: map[string]any{"analysisTier": analysisTierCustom, "probeStreams": true, "analyzeSTRMTarget": true}, allowed: true},
		{name: "complete", settings: map[string]any{"analysisTier": analysisTierComplete}, allowed: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := newScannerTestServer(t)
			descriptor := filepath.Join(t.TempDir(), "private.strm")
			// A descriptor larger than the hard limit makes any attempted read
			// fail, proving disallowed profiles stopped at inventory metadata.
			if err := os.WriteFile(descriptor, []byte(strings.Repeat("x", int(strmDescriptorLimit+1))), 0o600); err != nil {
				t.Fatal(err)
			}
			item, source := seedSTRMAnalysisMedia(t, server, descriptor, test.settings)
			options := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
			if options.AnalyzeSTRMTarget != test.allowed {
				t.Fatalf("AnalyzeSTRMTarget=%t", options.AnalyzeSTRMTarget)
			}
			err := server.analyzeSTRMTarget(context.Background(), item, source, options)
			if !test.allowed && err != nil {
				t.Fatalf("disallowed tier opened descriptor: %v", err)
			}
			if test.allowed && err == nil {
				t.Fatal("authorized tier did not attempt bounded descriptor read")
			}
		})
	}
}

func TestSTRMTargetAnalysisQueueEligibilityRequiresExactCapability(t *testing.T) {
	file := scannerMediaFile{ID: "strm-media", FileID: "strm-file", SourcePath: "/library/movie.strm", SourceType: "strm"}
	if scannerAnalysisEligibleForProfile(file, map[string]bool{"probeStreams": true}) {
		t.Fatal("ordinary stream probing made STRM eligible")
	}
	if !scannerAnalysisEligibleForProfile(file, map[string]bool{"probeStreams": true, "analyzeSTRMTarget": true}) {
		t.Fatal("explicit STRM analysis capability did not make descriptor eligible")
	}
}

func TestSTRMTargetAnalysisPublishesOnlyRevisionBoundTechnicalFactsIdempotently(t *testing.T) {
	server := newScannerTestServer(t)
	directory := t.TempDir()
	descriptor := filepath.Join(directory, "private.strm")
	secret := "signed-query-token-should-never-persist"
	locator := "https://media.example.test/library/movie.mkv?token=" + secret
	if err := os.WriteFile(descriptor, []byte(locator+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, source := seedSTRMAnalysisMedia(t, server, descriptor, map[string]any{"analysisTier": analysisTierComplete})
	arguments := filepath.Join(directory, "arguments")
	payload := `{"format":{"format_name":"matroska,webm","duration":"120","bit_rate":"2500000","tags":{"comment":"` + secret + `"}},"streams":[{"index":0,"codec_type":"video","codec_name":"h264","profile":"Main","width":1280,"height":720,"avg_frame_rate":"24/1","pix_fmt":"yuv420p","tags":{"title":"` + secret + `"}},{"index":1,"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2,"channel_layout":"stereo","sample_rate":"48000","sample_fmt":"fltp"}]}`
	server.cfg.FFprobePath = writeSTRMFFprobeStub(t, directory, arguments, payload, "")
	client := &http.Client{Transport: strmRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("ffprobe stub should not issue a target request")
	})}
	options := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	options.ExpectedSourceRevision = source.SourceRevision
	options.ValidateSeekBehavior = false
	for attempt := 0; attempt < 2; attempt++ {
		if err := server.analyzeSTRMTargetResolved(context.Background(), item, source, options, locator, "matroska", client); err != nil {
			t.Fatalf("analysis attempt %d: %v", attempt+1, err)
		}
	}
	var count int
	var factsJSON, factsDigest string
	if err := server.db.QueryRow(`SELECT COUNT(*),MAX(facts_json),MAX(facts_digest) FROM media_analysis_facts WHERE media_id=? AND media_file_id=?`, item.ID, source.FileID).Scan(&count, &factsJSON, &factsDigest); err != nil {
		t.Fatal(err)
	}
	if count != 1 || factsDigest == "" || strings.Contains(factsJSON, secret) || strings.Contains(factsJSON, locator) {
		t.Fatalf("unsafe/idempotency facts count=%d digest=%q facts=%s", count, factsDigest, factsJSON)
	}
	var persistedStreamText string
	if err := server.db.QueryRow(`SELECT group_concat(source_identity || ' ' || storage_key || ' ' || display_title, ' ') FROM media_streams WHERE media_id=?`, item.ID).Scan(&persistedStreamText); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(persistedStreamText, secret) || strings.Contains(persistedStreamText, locator) {
		t.Fatalf("target leaked into stream facts: %q", persistedStreamText)
	}
	args, err := os.ReadFile(arguments)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(args), secret) || strings.Contains(string(args), locator) || !strings.Contains(string(args), "http://127.0.0.1:") {
		t.Fatalf("unsafe ffprobe arguments: %s", args)
	}
	if len(server.logEvents) != 0 {
		for _, event := range server.logEvents {
			if strings.Contains(event.Message, secret) || strings.Contains(event.Message, locator) {
				t.Fatalf("target leaked into log: %#v", event)
			}
		}
	}
}

func TestSTRMTargetAnalysisRejectsPrivateOriginsAndRedirects(t *testing.T) {
	if _, err := strmTargetHTTPClient(context.Background(), "http://127.0.0.1/private.mp4?token=secret"); err == nil {
		t.Fatal("private literal target was accepted")
	}
	redirecting := &http.Client{
		Transport: strmRoundTripFunc(func(request *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"http://127.0.0.1/private.mp4?token=secret"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    request,
			}, nil
		}),
		CheckRedirect: dlnaRedirectPolicy("https://media.example.test:443"),
	}
	proxyURL, closeProxy, err := startSTRMTargetProxy(context.Background(), "https://media.example.test/movie.mp4", redirecting)
	if err != nil {
		t.Fatal(err)
	}
	defer closeProxy()
	response, err := http.Get(proxyURL)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusBadGateway || strings.Contains(string(body), "127.0.0.1") || strings.Contains(string(body), "secret") {
		t.Fatalf("redirect result status=%d body=%q", response.StatusCode, body)
	}
}

func TestSTRMTargetAnalysisFencesDescriptorChangeCancellationAndPolicyDowngrade(t *testing.T) {
	server := newScannerTestServer(t)
	directory := t.TempDir()
	descriptor := filepath.Join(directory, "changing.strm")
	locator := "https://media.example.test/movie.mkv?token=private"
	if err := os.WriteFile(descriptor, []byte(locator+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	item, source := seedSTRMAnalysisMedia(t, server, descriptor, map[string]any{"analysisTier": analysisTierComplete})
	payload := `{"format":{"format_name":"matroska","duration":"10"},"streams":[{"index":0,"codec_type":"video","codec_name":"h264","profile":"Main","width":640,"height":360,"pix_fmt":"yuv420p"}]}`
	server.cfg.FFprobePath = writeSTRMFFprobeStub(t, directory, filepath.Join(directory, "args"), payload, "printf '\\n' >> "+strconv.Quote(descriptor))
	options := server.mediaAnalysisOptions(item, mediaAnalysisModeFull)
	options.ExpectedSourceRevision = source.SourceRevision
	options.ValidateSeekBehavior = false
	client := &http.Client{Transport: strmRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") })}
	if err := server.analyzeSTRMTargetResolved(context.Background(), item, source, options, locator, "matroska", client); !errors.Is(err, errSTRMDescriptorStale) {
		t.Fatalf("descriptor change error=%v", err)
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_analysis_facts WHERE media_id=?`, item.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("stale descriptor published count=%d err=%v", count, err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := server.analyzeSTRMTargetResolved(cancelled, item, source, options, locator, "matroska", client); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error=%v", err)
	}

	settings := `{"analysisTier":"custom","probeStreams":true,"analyzeSTRMTarget":false}`
	if _, err := server.db.Exec(`UPDATE libraries SET settings_json=? WHERE id=?`, settings, item.LibraryID); err != nil {
		t.Fatal(err)
	}
	err := server.withBackgroundTx(context.Background(), func(tx *sql.Tx) error {
		return assertSTRMTargetPublicationFenceTx(tx, item, source.Path, source.SourceRevision)
	})
	if !errors.Is(err, errSTRMTargetAnalysisDisabled) {
		t.Fatalf("policy downgrade fence=%v", err)
	}
}

func TestSTRMTargetLimitsAreExplicit(t *testing.T) {
	descriptor, requests, bytes, timeout := strmTargetLimits()
	if descriptor != 64<<10 || requests != 32 || bytes != 64<<20 || timeout != 45*time.Second {
		t.Fatalf("limits=%s", strmTargetBudgetDescription())
	}
}

func TestSTRMTargetResponseValidatorFailsClosedWithoutEvidenceAndOnLengthChange(t *testing.T) {
	bodyRequest, _ := http.NewRequest(http.MethodGet, "https://media.example.test/movie.mkv", nil)
	response := func(headers http.Header) *http.Response {
		return &http.Response{StatusCode: http.StatusPartialContent, Header: headers, Request: bodyRequest, ContentLength: -1}
	}
	unvalidated := &strmTargetProxyState{}
	if !unvalidated.acceptResponse(response(http.Header{})) || unvalidated.acceptResponse(response(http.Header{})) {
		t.Fatal("multiple body reads without a stable validator were accepted")
	}
	changed := &strmTargetProxyState{}
	if !changed.acceptResponse(response(http.Header{"ETag": []string{`"strong-v1"`}, "Content-Range": []string{"bytes 0-99/1000"}})) {
		t.Fatal("initial strong validator was rejected")
	}
	if changed.acceptResponse(response(http.Header{"ETag": []string{`"strong-v1"`}, "Content-Range": []string{"bytes 100-199/1001"}})) {
		t.Fatal("changed total length under the same ETag was accepted")
	}
	weak := &strmTargetProxyState{}
	if !weak.acceptResponse(response(http.Header{"ETag": []string{`W/"weak-v1"`}})) || weak.acceptResponse(response(http.Header{"ETag": []string{`W/"weak-v1"`}})) {
		t.Fatal("weak ETag was treated as a stable multi-read validator")
	}
}

func seedSTRMAnalysisMedia(t *testing.T, server *Server, descriptor string, settings map[string]any) (MediaItem, strmAnalysisSourceRecord) {
	t.Helper()
	library, err := server.createLibrary(CreateLibraryRequest{Name: "STRM Analysis", Type: "movie", Paths: []string{filepath.Dir(descriptor)}, Settings: settings})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(descriptor)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mediaID, fileID := randomID("strm-media"), randomID("strm-file")
	if _, err := server.db.Exec(`INSERT INTO media_items(id,library_id,type,title,sort_title,source_url,added_at) VALUES(?,?,'movie','STRM Movie','STRM Movie',?,?)`, mediaID, library.ID, descriptor, now); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_files(id,media_id,library_id,path,source_type,size_bytes,mod_time,content_fingerprint,identity_evidence,available,first_seen_at,last_seen_at)
		VALUES(?,?,?,?, 'strm', ?, ?, '', 'scanner:v2:descriptor-stat', 1, ?, ?)`, fileID, mediaID, library.ID, descriptor, info.Size(), fileModTime(info), now, now); err != nil {
		t.Fatal(err)
	}
	item, err := server.getMediaBackgroundSourceSeedContext(context.Background(), mediaID)
	if err != nil {
		t.Fatal(err)
	}
	item.MediaFiles = server.primaryMediaFileForPlaybackContext(context.Background(), item.ID, item.SourceURL)
	source, ok := server.strmAnalysisSource(context.Background(), item)
	if !ok {
		t.Fatal("seeded STRM source was not recognized")
	}
	return item, source
}

func writeSTRMFFprobeStub(t *testing.T, directory, arguments, payload, beforeOutput string) string {
	t.Helper()
	path := filepath.Join(directory, "ffprobe-strm-stub")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + strconv.Quote(arguments) + "\n"
	if strings.TrimSpace(beforeOutput) != "" {
		script += beforeOutput + "\n"
	}
	script += "printf '%s' " + strconv.Quote(payload) + "\n"
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}
