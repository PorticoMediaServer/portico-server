package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	hostedDocumentAudience    = "portico-media-server"
	hostedSignatureAlgorithm  = "ed25519"
	hostedPolicyKind          = "policy-snapshot"
	hostedDocumentClockSkew   = time.Minute
	maximumPolicyLifetime     = 7 * 24 * time.Hour
	hostedPolicyRenewalWindow = 24 * time.Hour
	hostedPolicyGracePeriod   = 24 * time.Hour
	hostedPolicyPlaybackDrain = 4 * time.Hour
)

type remotePolicyState struct {
	SnapshotID     string `json:"snapshotId"`
	SnapshotDigest string `json:"snapshotDigest,omitempty"`
	Generation     int64  `json:"generation,omitempty"`
	IssuedAt       string `json:"issuedAt"`
	ExpiresAt      string `json:"expiresAt,omitempty"`
	PolicyDigest   string `json:"policyDigest,omitempty"`
	AckPending     bool   `json:"ackPending,omitempty"`
}

func normalizedSHA256Digest(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("SHA-256 digest is invalid")
	}
	return value, nil
}

func (s *Server) loadRemotePolicyState() remotePolicyState {
	stored := strings.TrimSpace(s.secretSetting(remoteAccessPolicyStateKey))
	if stored == "" {
		return remotePolicyState{}
	}
	var state remotePolicyState
	if json.Unmarshal([]byte(stored), &state) != nil {
		return remotePolicyState{}
	}
	return state
}

type hostedDocumentSigningKeySet struct {
	SchemaVersion int                              `json:"schemaVersion"`
	ActiveKeyID   string                           `json:"activeKeyId"`
	Keys          []hostedDocumentSigningPublicKey `json:"keys"`
}

type hostedDocumentSigningPublicKey struct {
	KeyID        string `json:"keyId"`
	Algorithm    string `json:"algorithm"`
	PublicKeyB64 string `json:"publicKeyB64"`
	State        string `json:"state"`
}

func copyHostedDocumentPublicKeys(source map[string]string) map[string]string {
	result := make(map[string]string, len(source))
	for keyID, encoded := range source {
		keyID = strings.TrimSpace(keyID)
		encoded = strings.TrimSpace(encoded)
		if keyID != "" && encoded != "" {
			result[keyID] = encoded
		}
	}
	return result
}

func (s *Server) trustedHostedDocumentKey(keyID string) string {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return ""
	}
	s.hostedDocumentKeysMu.RLock()
	encoded := strings.TrimSpace(s.hostedDocumentPublicKeys[keyID])
	s.hostedDocumentKeysMu.RUnlock()
	if encoded != "" {
		return encoded
	}
	// Focused tests can construct Server values directly. The immutable config
	// remains a valid explicit trust seed even when no runtime cache was made.
	return strings.TrimSpace(s.cfg.HostedDocumentPublicKeys[keyID])
}

func (s *Server) trustedHostedDocumentKeys() map[string]string {
	result := copyHostedDocumentPublicKeys(s.cfg.HostedDocumentPublicKeys)
	s.hostedDocumentKeysMu.RLock()
	for keyID, encoded := range s.hostedDocumentPublicKeys {
		result[keyID] = encoded
	}
	s.hostedDocumentKeysMu.RUnlock()
	return result
}

func validHostedDocumentSigningKeyID(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 3 || len(value) > 80 {
		return false
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._:-", r) {
			continue
		}
		return false
	}
	return true
}

// ensureHostedDocumentKey refreshes the public verification set over the same
// authenticated-TLS Hosted origin already required for first attachment and
// policy synchronization. Static configuration is an optional trust seed, not
// a deployment prerequisite. Reusing a key ID with different material fails
// closed, so ordinary key rotation must publish a new ID.
func (s *Server) ensureHostedDocumentKey(ctx context.Context, hostedBaseURL, requiredKeyID string) error {
	requiredKeyID = strings.TrimSpace(requiredKeyID)
	if s.trustedHostedDocumentKey(requiredKeyID) != "" {
		return nil
	}
	hostedBaseURL = strings.TrimRight(strings.TrimSpace(hostedBaseURL), "/")
	if hostedBaseURL == "" || !validHostedDocumentSigningKeyID(requiredKeyID) {
		return errors.New("Hosted document signing key is unavailable")
	}
	var keySet hostedDocumentSigningKeySet
	if err := s.hostedJSONWithTimeout(ctx, "GET", hostedBaseURL+"/api/signing-keys", "", nil, &keySet, 5*time.Second); err != nil {
		return fmt.Errorf("refresh Hosted document signing keys: %w", err)
	}
	if keySet.SchemaVersion != 1 || !validHostedDocumentSigningKeyID(keySet.ActiveKeyID) || len(keySet.Keys) == 0 || len(keySet.Keys) > 16 {
		return errors.New("Hosted document signing key set is invalid")
	}
	validated := make(map[string]string, len(keySet.Keys))
	activeSeen := false
	for _, candidate := range keySet.Keys {
		candidate.KeyID = strings.TrimSpace(candidate.KeyID)
		candidate.Algorithm = strings.ToLower(strings.TrimSpace(candidate.Algorithm))
		candidate.PublicKeyB64 = strings.TrimSpace(candidate.PublicKeyB64)
		candidate.State = strings.ToLower(strings.TrimSpace(candidate.State))
		if !validHostedDocumentSigningKeyID(candidate.KeyID) || candidate.Algorithm != hostedSignatureAlgorithm ||
			(candidate.State != "active" && candidate.State != "verification") {
			return errors.New("Hosted document signing key set contains invalid metadata")
		}
		if _, err := decodeHostedDocumentPublicKey(candidate.PublicKeyB64); err != nil {
			return err
		}
		if previous, exists := validated[candidate.KeyID]; exists && previous != candidate.PublicKeyB64 {
			return errors.New("Hosted document signing key set contains a conflicting key ID")
		}
		validated[candidate.KeyID] = candidate.PublicKeyB64
		if candidate.KeyID == keySet.ActiveKeyID && candidate.State == "active" {
			activeSeen = true
		}
	}
	if !activeSeen || validated[requiredKeyID] == "" {
		return errors.New("Hosted document signing key set does not contain the required key")
	}
	s.hostedDocumentKeysMu.Lock()
	defer s.hostedDocumentKeysMu.Unlock()
	if s.hostedDocumentPublicKeys == nil {
		s.hostedDocumentPublicKeys = copyHostedDocumentPublicKeys(s.cfg.HostedDocumentPublicKeys)
	}
	for keyID, encoded := range validated {
		if existing := strings.TrimSpace(s.hostedDocumentPublicKeys[keyID]); existing != "" && existing != encoded {
			return errors.New("Hosted document signing key ID conflicts with a previously trusted key")
		}
		if configured := strings.TrimSpace(s.cfg.HostedDocumentPublicKeys[keyID]); configured != "" && configured != encoded {
			return errors.New("Hosted document signing key ID conflicts with the configured trust seed")
		}
	}
	for keyID, encoded := range validated {
		s.hostedDocumentPublicKeys[keyID] = encoded
	}
	return nil
}

func (s *Server) verifyHostedPolicySnapshot(raw json.RawMessage, snapshot RemotePolicySnapshot, expectedServerID string, now time.Time) error {
	previous := remotePolicyState{}
	if stored := s.secretSetting(remoteAccessPolicyStateKey); stored != "" {
		_ = json.Unmarshal([]byte(stored), &previous)
	}
	return verifyHostedPolicySnapshot(raw, snapshot, expectedServerID, now, s.trustedHostedDocumentKeys(), previous)
}

func verifyHostedPolicySnapshot(raw json.RawMessage, snapshot RemotePolicySnapshot, expectedServerID string, now time.Time, encodedKeys map[string]string, previous remotePolicyState) error {
	if strings.TrimSpace(snapshot.SnapshotID) == "" || strings.TrimSpace(snapshot.Signature) == "" {
		return errors.New("snapshot ID and signature are required")
	}
	if snapshot.Kind != hostedPolicyKind {
		return errors.New("policy document kind is invalid")
	}
	if snapshot.Version != 1 {
		return fmt.Errorf("unsupported policy document version %d", snapshot.Version)
	}
	if snapshot.Audience != hostedDocumentAudience {
		return errors.New("policy audience does not match this server product")
	}
	if snapshot.SignatureAlgorithm != hostedSignatureAlgorithm {
		return errors.New("unsupported policy signature algorithm")
	}
	if snapshot.ServerID == "" || snapshot.ServerID != expectedServerID {
		return errors.New("policy subject does not match the claimed server")
	}
	if snapshot.Generation < 1 {
		return errors.New("policy generation is invalid")
	}
	if _, err := normalizedSHA256Digest(snapshot.Digest); err != nil {
		return errors.New("policy snapshot digest is invalid")
	}
	if _, err := normalizedSHA256Digest(snapshot.PolicyDigest); err != nil {
		return errors.New("policy state digest is invalid")
	}
	issuedAt, err := time.Parse(time.RFC3339Nano, snapshot.IssuedAt)
	if err != nil {
		return errors.New("policy issuedAt is invalid")
	}
	expiresAt, err := time.Parse(time.RFC3339Nano, snapshot.ExpiresAt)
	if err != nil {
		return errors.New("policy expiresAt is invalid")
	}
	if issuedAt.After(now.Add(hostedDocumentClockSkew)) {
		return errors.New("policy is not valid yet")
	}
	if now.After(expiresAt.Add(hostedDocumentClockSkew)) {
		return errors.New("policy has expired")
	}
	if !expiresAt.After(issuedAt) || expiresAt.Sub(issuedAt) > maximumPolicyLifetime {
		return errors.New("policy validity window is not allowed")
	}
	if previous.Generation > 0 {
		if snapshot.Generation < previous.Generation {
			return errors.New("policy generation is older than the last accepted snapshot")
		}
		if snapshot.Generation == previous.Generation &&
			(snapshot.SnapshotID != previous.SnapshotID || !strings.EqualFold(strings.TrimSpace(snapshot.Digest), strings.TrimSpace(previous.SnapshotDigest))) {
			return errors.New("policy generation collides with different signed state")
		}
	} else if previous.IssuedAt != "" {
		previousIssuedAt, parseErr := time.Parse(time.RFC3339Nano, previous.IssuedAt)
		if parseErr == nil && (issuedAt.Before(previousIssuedAt) || issuedAt.Equal(previousIssuedAt) && snapshot.SnapshotID != previous.SnapshotID) {
			return errors.New("policy is older than the last accepted snapshot")
		}
	}
	encodedKey := strings.TrimSpace(encodedKeys[snapshot.SignatureKeyID])
	if encodedKey == "" {
		return errors.New("policy signing key is not trusted")
	}
	publicKey, err := decodeHostedDocumentPublicKey(encodedKey)
	if err != nil {
		return err
	}
	payload, err := canonicalHostedDocument("policy-snapshot", raw)
	if err != nil {
		return fmt.Errorf("canonicalize policy: %w", err)
	}
	signature, err := base64.RawURLEncoding.DecodeString(snapshot.Signature)
	if err != nil || len(signature) != ed25519.SignatureSize {
		return errors.New("policy signature encoding is invalid")
	}
	if !ed25519.Verify(publicKey, payload, signature) {
		return errors.New("policy signature is invalid")
	}
	return nil
}

func remotePolicyRenewalDue(state remotePolicyState, now time.Time) bool {
	expiresAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(state.ExpiresAt))
	return err != nil || !expiresAt.After(now.Add(hostedPolicyRenewalWindow))
}

func remotePolicyContinuity(state remotePolicyState, now time.Time) string {
	expiresAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(state.ExpiresAt))
	if err != nil {
		return "unknown"
	}
	if now.Before(expiresAt) {
		return "valid"
	}
	if now.Before(expiresAt.Add(hostedPolicyGracePeriod)) {
		return "grace"
	}
	if now.Before(expiresAt.Add(hostedPolicyGracePeriod + hostedPolicyPlaybackDrain)) {
		return "hard-expired-draining"
	}
	return "hard-expired"
}

func canonicalHostedDocument(kind string, raw json.RawMessage) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var object map[string]any
	if err := decoder.Decode(&object); err != nil {
		return nil, err
	}
	delete(object, "signature")
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(object); err != nil {
		return nil, err
	}
	payload := bytes.TrimSuffix(canonical.Bytes(), []byte{'\n'})
	return append([]byte("portico-signed-document:"+kind+":v1\n"), payload...), nil
}

func decodeHostedDocumentPublicKey(encoded string) (ed25519.PublicKey, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(encoded)
	}
	if err != nil || len(decoded) != ed25519.PublicKeySize {
		return nil, errors.New("trusted Hosted document key is not a valid Ed25519 public key")
	}
	return ed25519.PublicKey(decoded), nil
}

func (s *Server) saveRemotePolicyState(state remotePolicyState) error {
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.saveSecretSetting(remoteAccessPolicyStateKey, string(encoded))
}
