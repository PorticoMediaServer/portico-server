package app

import (
	"net/url"
	"strings"
	"time"
)

const (
	liveTVHLSReferenceTTL     = 10 * time.Minute
	liveTVHLSReferenceMaximum = 16_384
)

type liveTVHLSReference struct {
	channelID string
	sourceURL string
	itemURL   string
	grantHash string
	expiresAt time.Time
}

// issueLiveTVHLSReference keeps provider locations on the server. The public
// playlist receives only an opaque, short-lived locator; the media grant and
// current channel source remain the authorization and revision fences.
func (s *Server) issueLiveTVHLSReference(channelID string, sourceURL string, itemURL string, qualityID string, mediaGrant string) string {
	now := time.Now().UTC()
	token := randomID("hlsitem")
	s.liveTVHLSReferenceMu.Lock()
	defer s.liveTVHLSReferenceMu.Unlock()
	// A small number of tests and embedding callers construct Server values
	// directly instead of using NewServer. Keep this security boundary safe in
	// every valid construction path: a nil reference table must never turn a
	// provider playlist request into a process panic.
	if s.liveTVHLSReferences == nil {
		s.liveTVHLSReferences = make(map[string]liveTVHLSReference)
	}
	if len(s.liveTVHLSReferences) >= liveTVHLSReferenceMaximum {
		var oldestToken string
		var oldestExpiry time.Time
		for candidate, reference := range s.liveTVHLSReferences {
			if !now.Before(reference.expiresAt) {
				delete(s.liveTVHLSReferences, candidate)
				continue
			}
			if oldestToken == "" || reference.expiresAt.Before(oldestExpiry) {
				oldestToken, oldestExpiry = candidate, reference.expiresAt
			}
		}
		if len(s.liveTVHLSReferences) >= liveTVHLSReferenceMaximum && oldestToken != "" {
			delete(s.liveTVHLSReferences, oldestToken)
		}
	}
	s.liveTVHLSReferences[token] = liveTVHLSReference{
		channelID: strings.TrimSpace(channelID),
		sourceURL: strings.TrimSpace(sourceURL),
		itemURL:   strings.TrimSpace(itemURL),
		grantHash: hashToken(strings.TrimSpace(mediaGrant)),
		expiresAt: now.Add(liveTVHLSReferenceTTL),
	}
	out := "/api/live-tv/hls/" + url.PathEscape(channelID) + "/item?ref=" + url.QueryEscape(token)
	if normalized := normalizeLiveTVQualityID(qualityID); normalized != "" {
		out += "&quality=" + url.QueryEscape(normalized)
	}
	return out
}

func (s *Server) resolveLiveTVHLSReference(channelID string, sourceURL string, token string, mediaGrant string) (string, bool) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > 256 {
		return "", false
	}
	now := time.Now().UTC()
	s.liveTVHLSReferenceMu.Lock()
	defer s.liveTVHLSReferenceMu.Unlock()
	reference, ok := s.liveTVHLSReferences[token]
	if !ok {
		return "", false
	}
	if !now.Before(reference.expiresAt) {
		delete(s.liveTVHLSReferences, token)
		return "", false
	}
	if reference.channelID != strings.TrimSpace(channelID) || reference.sourceURL != strings.TrimSpace(sourceURL) || reference.grantHash != hashToken(strings.TrimSpace(mediaGrant)) {
		return "", false
	}
	return reference.itemURL, true
}
