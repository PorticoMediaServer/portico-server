package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const minAcoustIDScore = 0.80

type fpcalcResult struct {
	Duration    float64 `json:"duration"`
	Fingerprint string  `json:"fingerprint"`
}

type acoustIDLookupResponse struct {
	Status  string            `json:"status"`
	Results []acoustIDResult  `json:"results"`
	Error   *acoustIDAPIError `json:"error,omitempty"`
}

type acoustIDAPIError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type acoustIDResult struct {
	ID         string                    `json:"id"`
	Score      float64                   `json:"score"`
	Recordings []acoustIDRecordingResult `json:"recordings"`
}

type acoustIDRecordingResult struct {
	ID            string                    `json:"id"`
	Title         string                    `json:"title"`
	Duration      int                       `json:"duration"`
	ArtistCredit  []musicBrainzArtistCredit `json:"artists"`
	ReleaseGroups []musicBrainzReleaseGroup `json:"releasegroups"`
}

type acoustIDMatch struct {
	AcoustIDID  string
	RecordingID string
	Score       float64
}

func (s *Server) refreshTrackMetadataFromAcoustID(ctx context.Context, item MediaItem) (MediaItem, error) {
	if item.Type != "track" {
		return MediaItem{}, errors.New("AcoustID matching is only available for music tracks")
	}
	path, err := s.localSourcePathForTranscode(item)
	if err != nil {
		return MediaItem{}, err
	}
	if _, cachedMatch, ok := s.cachedAudioFingerprintContext(ctx, item.ID, path); ok && cachedMatch.RecordingID != "" && cachedMatch.Score >= minAcoustIDScore {
		item, err = s.applyAcoustIDIdentityEvidence(ctx, item, cachedMatch, "acoustid-cache")
		if err != nil {
			return MediaItem{}, err
		}
		return s.refreshTrackMetadataFromMusicBrainz(ctx, item)
	}
	fingerprint, err := s.audioFingerprint(ctx, path)
	if err != nil {
		return MediaItem{}, err
	}
	match, err := s.lookupAcoustID(ctx, fingerprint)
	if err != nil {
		return MediaItem{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if err := s.storeAudioFingerprintContext(ctx, item.ID, path, fingerprint, match, now); err != nil {
		return MediaItem{}, err
	}
	if match.RecordingID == "" || match.Score < minAcoustIDScore {
		return item, nil
	}
	item, err = s.applyAcoustIDIdentityEvidence(ctx, item, match, "acoustid")
	if err != nil {
		return MediaItem{}, err
	}
	return s.refreshTrackMetadataFromMusicBrainz(ctx, item)
}

func (s *Server) applyAcoustIDIdentityEvidence(ctx context.Context, item MediaItem, match acoustIDMatch, source string) (MediaItem, error) {
	result, err := s.applyMetadata(ctx, metadataApplyRequest{
		MediaID:          item.ID,
		ExpectedRevision: item.MetadataRevision,
		Origin:           metadataSourceProvider,
		Source:           source,
		Provider:         "acoustid",
		Identities: []metadataProviderIdentityProposal{
			{Provider: "acoustid", ExternalID: match.AcoustIDID, ExternalType: "fingerprint", Confidence: match.Score},
			{Provider: "musicbrainz", ExternalID: match.RecordingID, ExternalType: "recording", Confidence: match.Score},
		},
	})
	if err != nil {
		return MediaItem{}, err
	}
	item.MetadataRevision = result.Revision
	item.MetadataETag = result.ETag
	return s.getMediaContext(ctx, "", item.ID)
}

func (s *Server) audioFingerprint(ctx context.Context, path string) (fpcalcResult, error) {
	fpcalcPath := strings.TrimSpace(s.cfg.FPcalcPath)
	if fpcalcPath == "" {
		return fpcalcResult{}, errors.New("fpcalc is not configured")
	}
	if _, err := exec.LookPath(fpcalcPath); err != nil && filepath.Base(fpcalcPath) == fpcalcPath {
		return fpcalcResult{}, errors.New("fpcalc is not available on PATH")
	}
	cmd := exec.CommandContext(ctx, fpcalcPath, "-json", path)
	output, err := managedCommandOutput(ctx, cmd)
	if err != nil {
		return fpcalcResult{}, err
	}
	var result fpcalcResult
	if err := json.Unmarshal(output, &result); err != nil {
		return fpcalcResult{}, err
	}
	if strings.TrimSpace(result.Fingerprint) == "" {
		return fpcalcResult{}, errors.New("fpcalc did not return a fingerprint")
	}
	return result, nil
}

func (s *Server) lookupAcoustID(ctx context.Context, fingerprint fpcalcResult) (acoustIDMatch, error) {
	apiKey := strings.TrimSpace(s.cfg.AcoustIDAPIKey)
	if apiKey == "" {
		return acoustIDMatch{}, errors.New("AcoustID API key is not configured")
	}
	baseURL := strings.TrimRight(s.cfg.AcoustIDBaseURL, "/")
	if baseURL == "" {
		baseURL = "https://api.acoustid.org/v2"
	}
	values := url.Values{}
	values.Set("client", apiKey)
	values.Set("meta", "recordings releasegroups compress")
	values.Set("duration", strconv.Itoa(int(math.Round(fingerprint.Duration))))
	values.Set("fingerprint", fingerprint.Fingerprint)
	var payload acoustIDLookupResponse
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if err := waitForAcoustIDSlot(ctx); err != nil {
			return acoustIDMatch{}, err
		}
		requestCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
		req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, baseURL+"/lookup", strings.NewReader(values.Encode()))
		if err != nil {
			cancel()
			return acoustIDMatch{}, err
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "Portico/0.1 ( https://getportico.tv )")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			cancel()
			return acoustIDMatch{}, err
		}
		statusCode := resp.StatusCode
		retryAfter := retryAfterDuration(resp.Header.Get("Retry-After"))
		if statusCode >= 200 && statusCode < 300 {
			const maxAcoustIDResponseBytes = 2 << 20
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxAcoustIDResponseBytes+1))
			resp.Body.Close()
			cancel()
			if readErr != nil {
				return acoustIDMatch{}, readErr
			}
			if len(body) > maxAcoustIDResponseBytes {
				return acoustIDMatch{}, errors.New("AcoustID response exceeds the supported size")
			}
			if err := json.Unmarshal(body, &payload); err != nil {
				return acoustIDMatch{}, err
			}
			lastErr = nil
			break
		}
		resp.Body.Close()
		cancel()
		lastErr = fmt.Errorf("AcoustID lookup failed with HTTP %d", statusCode)
		if attempt == 2 || (statusCode != http.StatusTooManyRequests && statusCode != http.StatusServiceUnavailable) {
			return acoustIDMatch{}, lastErr
		}
		if retryAfter <= 0 {
			retryAfter = time.Duration(attempt+1) * time.Second
		}
		if retryAfter > 30*time.Second {
			return acoustIDMatch{}, lastErr
		}
		timer := time.NewTimer(retryAfter)
		select {
		case <-ctx.Done():
			timer.Stop()
			return acoustIDMatch{}, ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr != nil {
		return acoustIDMatch{}, lastErr
	}
	if payload.Status != "ok" {
		if payload.Error != nil && payload.Error.Message != "" {
			return acoustIDMatch{}, errors.New(payload.Error.Message)
		}
		return acoustIDMatch{}, errors.New("AcoustID lookup failed")
	}
	best := acoustIDMatch{}
	for _, result := range payload.Results {
		if result.Score < best.Score {
			continue
		}
		for _, recording := range result.Recordings {
			if recording.ID == "" {
				continue
			}
			best = acoustIDMatch{AcoustIDID: result.ID, RecordingID: recording.ID, Score: result.Score}
			break
		}
	}
	return best, nil
}

func waitForAcoustIDSlot(ctx context.Context) error {
	acoustIDThrottleMu.Lock()
	defer acoustIDThrottleMu.Unlock()
	wait := time.Second/3 - time.Since(acoustIDLastRequest)
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	acoustIDLastRequest = time.Now()
	return nil
}

func (s *Server) storeAudioFingerprint(mediaID, path string, fingerprint fpcalcResult, match acoustIDMatch, updatedAt string) error {
	return s.storeAudioFingerprintContext(context.Background(), mediaID, path, fingerprint, match, updatedAt)
}

func (s *Server) storeAudioFingerprintContext(ctx context.Context, mediaID, path string, fingerprint fpcalcResult, match acoustIDMatch, updatedAt string) error {
	_, err := s.execBackgroundWrite(ctx, `
		INSERT INTO audio_fingerprints (media_id, path, fingerprint, duration_seconds, acoustid_id, recording_id, score, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(media_id) DO UPDATE SET
			path = excluded.path,
			fingerprint = excluded.fingerprint,
			duration_seconds = excluded.duration_seconds,
			acoustid_id = excluded.acoustid_id,
			recording_id = excluded.recording_id,
			score = excluded.score,
			updated_at = excluded.updated_at`,
		mediaID, path, fingerprint.Fingerprint, int(math.Round(fingerprint.Duration)), match.AcoustIDID, match.RecordingID, match.Score, updatedAt)
	return err
}

func (s *Server) cachedAudioFingerprint(mediaID, path string) (fpcalcResult, acoustIDMatch, bool) {
	return s.cachedAudioFingerprintContext(context.Background(), mediaID, path)
}

func (s *Server) cachedAudioFingerprintContext(ctx context.Context, mediaID, path string) (fpcalcResult, acoustIDMatch, bool) {
	var fingerprint fpcalcResult
	var match acoustIDMatch
	var storedPath string
	var duration int
	err := s.queryBackgroundRow(ctx, `
		SELECT path, fingerprint, duration_seconds, acoustid_id, recording_id, score
		FROM audio_fingerprints
		WHERE media_id = ?`, mediaID).Scan(&storedPath, &fingerprint.Fingerprint, &duration, &match.AcoustIDID, &match.RecordingID, &match.Score)
	if err != nil {
		return fpcalcResult{}, acoustIDMatch{}, false
	}
	if storedPath != path || strings.TrimSpace(fingerprint.Fingerprint) == "" {
		return fpcalcResult{}, acoustIDMatch{}, false
	}
	fingerprint.Duration = float64(duration)
	return fingerprint, match, true
}
