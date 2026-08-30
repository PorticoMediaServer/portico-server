package app

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"sort"
	"strings"
	"time"
)

const (
	hostedDocumentAudience      = "portico-media-server"
	hostedSignatureAlgorithm    = "ed25519"
	hostedPolicyKind            = "policy-snapshot"
	hostedDocumentClockSkew     = time.Minute
	maximumPolicyLifetime       = 7 * 24 * time.Hour
	hostedPolicyRenewalInterval = 24 * time.Hour
	trustedTimeFloorInterval    = time.Hour
	maxRemotePolicyMembers      = 10000
	maxRemotePolicyTombstones   = 10000
	maxRemotePolicyEncoded      = 2 << 20
	maxRemotePolicyDecoded      = 4 << 20
	maxRemotePolicyAggregate    = 32 << 20
	hostedDocumentKeysetRefresh = 24 * time.Hour
	hostedDocumentKeysetSkew    = time.Minute
	hostedDocumentKeysetJitter  = 15 * time.Minute
)

const hostedDocumentKeySetStateKey = "hostedDocumentKeySetState"

type remotePolicyState struct {
	SnapshotID     string `json:"snapshotId"`
	SnapshotDigest string `json:"snapshotDigest,omitempty"`
	Generation     int64  `json:"generation,omitempty"`
	IssuedAt       string `json:"issuedAt"`
	ExpiresAt      string `json:"expiresAt,omitempty"`
	PolicyDigest   string `json:"policyDigest,omitempty"`
	PolicyRevision int64  `json:"policyRevision,omitempty"`
	PolicyRoot     string `json:"policyRoot,omitempty"`
	ContentRoot    string `json:"contentRoot,omitempty"`
	AckPending     bool   `json:"ackPending,omitempty"`
	// TrustedTimeFloor is advanced only after an authenticated Hosted exchange
	// or a verified signed checkpoint. It never extends ExpiresAt; it exists so
	// a host clock rollback across restart fails closed instead of manufacturing
	// additional policy lifetime.
	TrustedTimeFloor  string `json:"trustedTimeFloor,omitempty"`
	KeySetGeneration  int64  `json:"keySetGeneration,omitempty"`
	KeySetFingerprint string `json:"keySetFingerprint,omitempty"`
}

func normalizedSHA256Digest(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("SHA-256 digest is invalid")
	}
	return value, nil
}

func remotePolicyChunkDigest(raw json.RawMessage) (string, error) {
	var fields struct {
		Members    json.RawMessage `json:"members"`
		Tombstones json.RawMessage `json:"deletedAccountTombstones"`
	}
	if err := json.Unmarshal(raw, &fields); err != nil || len(fields.Members) == 0 || len(fields.Tombstones) == 0 {
		return "", errors.New("policy chunk payload is invalid")
	}
	payload, err := json.Marshal(fields)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}

func remotePolicyContentRoot(chunkHashes []string, itemCount int) (string, error) {
	payload, err := json.Marshal(struct {
		Version   int      `json:"version"`
		ItemCount int      `json:"itemCount"`
		Chunks    []string `json:"chunks"`
	}{1, itemCount, chunkHashes})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
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
	Generation    int64                            `json:"generation"`
	Fingerprint   string                           `json:"fingerprint"`
	IssuedAt      string                           `json:"issuedAt"`
	ExpiresAt     string                           `json:"expiresAt"`
	ActiveKeyID   string                           `json:"activeKeyId"`
	Keys          []hostedDocumentSigningPublicKey `json:"keys"`
}

type hostedDocumentSigningPublicKey struct {
	KeyID        string `json:"keyId"`
	Algorithm    string `json:"algorithm"`
	PublicKeyB64 string `json:"publicKeyB64"`
	State        string `json:"state"`
	ValidFrom    string `json:"validFrom,omitempty"`
	ValidUntil   string `json:"validUntil,omitempty"`
	RevokedAt    string `json:"revokedAt,omitempty"`
}

type hostedDocumentSigningKeySetState struct {
	SchemaVersion int                              `json:"schemaVersion"`
	Generation    int64                            `json:"generation"`
	Fingerprint   string                           `json:"fingerprint"`
	IssuedAt      string                           `json:"issuedAt"`
	ExpiresAt     string                           `json:"expiresAt"`
	ActiveKeyID   string                           `json:"activeKeyId"`
	Keys          []hostedDocumentSigningPublicKey `json:"keys"`
}

type hostedDocumentKeyRefreshCall struct {
	done chan struct{}
	err  error
}

func keySetStateFromDocumentKeySet(keySet hostedDocumentSigningKeySet) hostedDocumentSigningKeySetState {
	return hostedDocumentSigningKeySetState{SchemaVersion: keySet.SchemaVersion, Generation: keySet.Generation, Fingerprint: keySet.Fingerprint, IssuedAt: keySet.IssuedAt, ExpiresAt: keySet.ExpiresAt, ActiveKeyID: keySet.ActiveKeyID, Keys: append([]hostedDocumentSigningPublicKey(nil), keySet.Keys...)}
}

func documentKeySetAsWire(state hostedDocumentSigningKeySetState) hostedDocumentSigningKeySet {
	return hostedDocumentSigningKeySet{SchemaVersion: state.SchemaVersion, Generation: state.Generation, Fingerprint: state.Fingerprint, IssuedAt: state.IssuedAt, ExpiresAt: state.ExpiresAt, ActiveKeyID: state.ActiveKeyID, Keys: append([]hostedDocumentSigningPublicKey(nil), state.Keys...)}
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
	if s.hostedDocumentRevokedKeys[keyID] {
		s.hostedDocumentKeysMu.RUnlock()
		return ""
	}
	encoded := strings.TrimSpace(s.hostedDocumentPublicKeys[keyID])
	knownLifecycle := s.hostedDocumentKeySetState.Generation > 0
	s.hostedDocumentKeysMu.RUnlock()
	if encoded != "" {
		return encoded
	}
	if knownLifecycle {
		return ""
	}
	// Focused tests can construct Server values directly. The immutable config
	// remains a valid explicit trust seed even when no runtime cache was made.
	return strings.TrimSpace(s.cfg.HostedDocumentPublicKeys[keyID])
}

func (s *Server) trustedHostedDocumentKeys() map[string]string {
	result := copyHostedDocumentPublicKeys(s.cfg.HostedDocumentPublicKeys)
	s.hostedDocumentKeysMu.RLock()
	if s.hostedDocumentKeySetState.Generation > 0 {
		result = map[string]string{}
	}
	for keyID, encoded := range s.hostedDocumentPublicKeys {
		if s.hostedDocumentRevokedKeys[keyID] {
			delete(result, keyID)
			continue
		}
		result[keyID] = encoded
	}
	s.hostedDocumentKeysMu.RUnlock()
	return result
}

func (s *Server) loadHostedDocumentKeySetState() hostedDocumentSigningKeySetState {
	s.hostedDocumentKeysMu.RLock()
	state := s.hostedDocumentKeySetState
	s.hostedDocumentKeysMu.RUnlock()
	if state.Generation > 0 || state.Fingerprint != "" {
		s.installHostedDocumentKeySetState(state)
		return state
	}
	stored := strings.TrimSpace(s.secretSetting(hostedDocumentKeySetStateKey))
	if stored == "" {
		return hostedDocumentSigningKeySetState{}
	}
	var loaded hostedDocumentSigningKeySetState
	if json.Unmarshal([]byte(stored), &loaded) != nil {
		return hostedDocumentSigningKeySetState{}
	}
	s.hostedDocumentKeysMu.Lock()
	if s.hostedDocumentKeySetState.Generation == 0 && s.hostedDocumentKeySetState.Fingerprint == "" {
		s.hostedDocumentKeySetState = loaded
	}
	state = s.hostedDocumentKeySetState
	s.hostedDocumentKeysMu.Unlock()
	s.installHostedDocumentKeySetState(state)
	return state
}

func (s *Server) installHostedDocumentKeySetState(state hostedDocumentSigningKeySetState) {
	if state.Generation == 0 {
		return
	}
	s.hostedDocumentKeysMu.Lock()
	defer s.hostedDocumentKeysMu.Unlock()
	if s.hostedDocumentRevokedKeys == nil {
		s.hostedDocumentRevokedKeys = make(map[string]bool)
	}
	allowed := make(map[string]bool, len(state.Keys))
	for _, key := range state.Keys {
		if key.State != "revoked" && key.RevokedAt == "" {
			allowed[key.KeyID] = true
		}
	}
	for keyID := range s.hostedDocumentPublicKeys {
		if !allowed[keyID] {
			delete(s.hostedDocumentPublicKeys, keyID)
		}
	}
	for _, key := range state.Keys {
		if key.State == "revoked" || key.RevokedAt != "" {
			s.hostedDocumentRevokedKeys[key.KeyID] = true
			delete(s.hostedDocumentPublicKeys, key.KeyID)
			continue
		}
		if configured := strings.TrimSpace(s.cfg.HostedDocumentPublicKeys[key.KeyID]); configured != "" && configured != key.PublicKeyB64 {
			// A changed static seed for an already-known key ID is an
			// equivocation, not a replacement. Keep it unusable until a later
			// higher-generation keyset explicitly resolves the deployment.
			s.hostedDocumentRevokedKeys[key.KeyID] = true
			continue
		}
		if s.hostedDocumentPublicKeys == nil {
			s.hostedDocumentPublicKeys = copyHostedDocumentPublicKeys(s.cfg.HostedDocumentPublicKeys)
		}
		if s.hostedDocumentPublicKeys[key.KeyID] == "" {
			s.hostedDocumentPublicKeys[key.KeyID] = key.PublicKeyB64
		}
	}
}

func hostedDocumentKeySetRefreshDue(state hostedDocumentSigningKeySetState, now time.Time) bool {
	if state.Generation < 1 {
		return false
	}
	issued, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(state.IssuedAt))
	if err != nil {
		return true
	}
	hasher := fnv.New32a()
	_, _ = hasher.Write([]byte(state.Fingerprint))
	jitter := time.Duration(hasher.Sum32()%uint32(hostedDocumentKeysetJitter/time.Minute+1)) * time.Minute
	return !issued.Add(hostedDocumentKeysetRefresh + jitter).After(now)
}

func hostedDocumentKeyUsable(state hostedDocumentSigningKeySetState, keyID string, now time.Time) bool {
	for _, key := range state.Keys {
		if key.KeyID != keyID || key.State == "revoked" || strings.TrimSpace(key.RevokedAt) != "" {
			continue
		}
		if from, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(key.ValidFrom)); err == nil && now.Before(from.Add(-hostedDocumentKeysetSkew)) {
			return false
		}
		if until, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(key.ValidUntil)); err == nil && now.After(until.Add(hostedDocumentKeysetSkew)) {
			return false
		}
		return strings.TrimSpace(key.PublicKeyB64) != ""
	}
	return false
}

func validateHostedDocumentKeySet(keySet hostedDocumentSigningKeySet, requiredKeyID string, now time.Time, previous hostedDocumentSigningKeySetState) (map[string]string, hostedDocumentSigningKeySetState, error) {
	if keySet.SchemaVersion != 1 || keySet.Generation < 1 || !validHostedDocumentSigningKeyID(keySet.ActiveKeyID) || len(keySet.Keys) == 0 || len(keySet.Keys) > 16 {
		return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key set is invalid")
	}
	issuedAt, issuedErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(keySet.IssuedAt))
	expiresAt, expiresErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(keySet.ExpiresAt))
	if issuedErr != nil || expiresErr != nil || issuedAt.After(now.Add(hostedDocumentKeysetSkew)) || !expiresAt.After(issuedAt) || expiresAt.Sub(issuedAt) > maximumPolicyLifetime || now.After(expiresAt.Add(hostedDocumentKeysetSkew)) {
		return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key set validity window is invalid")
	}
	validated := make(map[string]string, len(keySet.Keys))
	activeSeen := false
	normalized := make([]hostedDocumentSigningPublicKey, 0, len(keySet.Keys))
	for _, candidate := range keySet.Keys {
		candidate.KeyID = strings.TrimSpace(candidate.KeyID)
		candidate.Algorithm = strings.ToLower(strings.TrimSpace(candidate.Algorithm))
		candidate.PublicKeyB64 = strings.TrimSpace(candidate.PublicKeyB64)
		candidate.State = strings.ToLower(strings.TrimSpace(candidate.State))
		candidate.ValidFrom = strings.TrimSpace(candidate.ValidFrom)
		candidate.ValidUntil = strings.TrimSpace(candidate.ValidUntil)
		candidate.RevokedAt = strings.TrimSpace(candidate.RevokedAt)
		if !validHostedDocumentSigningKeyID(candidate.KeyID) || candidate.Algorithm != hostedSignatureAlgorithm || (candidate.State != "active" && candidate.State != "verification" && candidate.State != "revoked") {
			return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key set contains invalid metadata")
		}
		if _, err := decodeHostedDocumentPublicKey(candidate.PublicKeyB64); err != nil {
			return nil, hostedDocumentSigningKeySetState{}, err
		}
		if candidate.ValidFrom != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, candidate.ValidFrom); err != nil || parsed.After(expiresAt.Add(hostedDocumentKeysetSkew)) {
				return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key validity start is invalid")
			}
		}
		if candidate.ValidUntil != "" {
			parsed, err := time.Parse(time.RFC3339Nano, candidate.ValidUntil)
			if err != nil || !parsed.After(issuedAt) {
				return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key validity end is invalid")
			}
		}
		if candidate.RevokedAt != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, candidate.RevokedAt); err != nil || parsed.Before(issuedAt) {
				return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key revocation time is invalid")
			}
		}
		if previousKey := findHostedDocumentSigningKey(previous.Keys, candidate.KeyID); previousKey != nil && previousKey.PublicKeyB64 != candidate.PublicKeyB64 {
			return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key ID conflicts with a previously trusted key")
		}
		if previousKey := findHostedDocumentSigningKey(previous.Keys, candidate.KeyID); previousKey != nil && previousKey.State == "revoked" && candidate.State != "revoked" {
			return nil, hostedDocumentSigningKeySetState{}, errors.New("revoked Hosted document signing key was reactivated")
		}
		if candidate.KeyID == keySet.ActiveKeyID && candidate.State == "active" && candidate.RevokedAt == "" {
			activeSeen = true
		}
		if candidate.State != "revoked" && candidate.RevokedAt == "" && hostedDocumentKeyUsable(hostedDocumentSigningKeySetState{Keys: []hostedDocumentSigningPublicKey{candidate}}, candidate.KeyID, now) {
			validated[candidate.KeyID] = candidate.PublicKeyB64
		}
		normalized = append(normalized, candidate)
	}
	if !activeSeen {
		return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key set does not contain a valid active key")
	}
	keySet.Keys = normalized
	computed, err := hostedDocumentKeySetFingerprint(keySet)
	if err != nil || !strings.EqualFold(strings.TrimSpace(keySet.Fingerprint), computed) {
		return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key set fingerprint is invalid")
	}
	if previous.Generation > 0 {
		if keySet.Generation < previous.Generation {
			return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key set generation rolled back")
		}
		if keySet.Generation == previous.Generation && !strings.EqualFold(keySet.Fingerprint, previous.Fingerprint) {
			return nil, hostedDocumentSigningKeySetState{}, errors.New("Hosted document signing key set generation equivocated")
		}
	}
	return validated, keySetStateFromDocumentKeySet(keySet), nil
}

func findHostedDocumentSigningKey(keys []hostedDocumentSigningPublicKey, keyID string) *hostedDocumentSigningPublicKey {
	for index := range keys {
		if keys[index].KeyID == keyID {
			return &keys[index]
		}
	}
	return nil
}

func hostedDocumentKeySetFingerprint(keySet hostedDocumentSigningKeySet) (string, error) {
	keys := append([]hostedDocumentSigningPublicKey(nil), keySet.Keys...)
	sort.Slice(keys, func(i, j int) bool { return keys[i].KeyID < keys[j].KeyID })
	payload, err := json.Marshal(struct {
		SchemaVersion int                              `json:"schemaVersion"`
		Generation    int64                            `json:"generation"`
		ActiveKeyID   string                           `json:"activeKeyId"`
		Keys          []hostedDocumentSigningPublicKey `json:"keys"`
	}{keySet.SchemaVersion, keySet.Generation, keySet.ActiveKeyID, keys})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
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
	now := time.Now().UTC()
	state := s.loadHostedDocumentKeySetState()
	if state.Generation > 0 && s.trustedHostedDocumentKey(requiredKeyID) != "" && !hostedDocumentKeySetRefreshDue(state, now) {
		return nil
	}
	// A configured key is a bootstrap verification seed, but it must not
	// suppress the first lifecycle acquisition. Once a keyset is persisted,
	// its validity and revocation state are authoritative across restarts.
	bootstrapKey := state.Generation == 0 && strings.TrimSpace(s.cfg.HostedDocumentPublicKeys[requiredKeyID]) != ""
	hostedBaseURL = strings.TrimRight(strings.TrimSpace(hostedBaseURL), "/")
	if hostedBaseURL == "" || !validHostedDocumentSigningKeyID(requiredKeyID) {
		return errors.New("Hosted document signing key is unavailable")
	}
	s.hostedDocumentKeyRefreshMu.Lock()
	if call := s.hostedDocumentKeyRefresh; call != nil {
		s.hostedDocumentKeyRefreshMu.Unlock()
		select {
		case <-call.done:
			return call.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	call := &hostedDocumentKeyRefreshCall{done: make(chan struct{})}
	s.hostedDocumentKeyRefresh = call
	s.hostedDocumentKeyRefreshMu.Unlock()
	defer func() {
		s.hostedDocumentKeyRefreshMu.Lock()
		close(call.done)
		s.hostedDocumentKeyRefresh = nil
		s.hostedDocumentKeyRefreshMu.Unlock()
	}()
	var keySet hostedDocumentSigningKeySet
	if err := s.hostedJSONWithTimeout(ctx, "GET", hostedBaseURL+"/api/signing-keys", "", nil, &keySet, 5*time.Second); err != nil {
		// Preserve the bounded outage behavior for a first-use server that still
		// has its explicit bootstrap key. This is only a network/availability
		// fallback: a malformed, conflicting, or revoked lifecycle response must
		// never fall back to static trust.
		if bootstrapKey && s.trustedHostedDocumentKey(requiredKeyID) != "" {
			return nil
		}
		call.err = fmt.Errorf("refresh Hosted document signing keys: %w", err)
		return call.err
	}
	validated, nextState, err := validateHostedDocumentKeySet(keySet, requiredKeyID, now, state)
	if err != nil {
		call.err = err
		return err
	}
	encodedState, err := json.Marshal(nextState)
	if err != nil {
		call.err = err
		return err
	}
	if err := s.saveSecretSetting(hostedDocumentKeySetStateKey, string(encodedState)); err != nil {
		call.err = fmt.Errorf("persist Hosted document signing key lifecycle: %w", err)
		return call.err
	}
	s.hostedDocumentKeysMu.Lock()
	if s.hostedDocumentPublicKeys == nil {
		s.hostedDocumentPublicKeys = copyHostedDocumentPublicKeys(s.cfg.HostedDocumentPublicKeys)
	}
	if s.hostedDocumentRevokedKeys == nil {
		s.hostedDocumentRevokedKeys = make(map[string]bool)
	}
	for keyID := range s.hostedDocumentPublicKeys {
		if _, retained := validated[keyID]; !retained {
			delete(s.hostedDocumentPublicKeys, keyID)
		}
	}
	for keyID, encoded := range validated {
		if existing := strings.TrimSpace(s.hostedDocumentPublicKeys[keyID]); existing != "" && existing != encoded {
			s.hostedDocumentKeysMu.Unlock()
			call.err = errors.New("Hosted document signing key ID conflicts with a previously trusted key")
			return call.err
		}
		if configured := strings.TrimSpace(s.cfg.HostedDocumentPublicKeys[keyID]); configured != "" && configured != encoded {
			s.hostedDocumentKeysMu.Unlock()
			call.err = errors.New("Hosted document signing key ID conflicts with the configured trust seed")
			return call.err
		}
	}
	for _, key := range nextState.Keys {
		if key.State == "revoked" || key.RevokedAt != "" {
			s.hostedDocumentRevokedKeys[key.KeyID] = true
			delete(s.hostedDocumentPublicKeys, key.KeyID)
		}
	}
	for keyID, encoded := range validated {
		s.hostedDocumentPublicKeys[keyID] = encoded
		delete(s.hostedDocumentRevokedKeys, keyID)
	}
	s.hostedDocumentKeySetState = nextState
	s.hostedDocumentKeysMu.Unlock()
	if validated[requiredKeyID] == "" {
		call.err = errors.New("Hosted document signing key set does not contain the required valid key")
		return call.err
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
	if len(raw) > maxRemotePolicyEncoded {
		return errors.New("policy snapshot exceeds the encoded byte limit")
	}
	if len(snapshot.Members) > maxRemotePolicyMembers || len(snapshot.DeletedAccountTombstones) > maxRemotePolicyTombstones {
		return errors.New("policy snapshot exceeds the authority item limit")
	}
	if snapshot.ChunkCount < 1 || snapshot.ChunkCount > 4096 || snapshot.ChunkIndex < 0 || snapshot.ChunkIndex >= snapshot.ChunkCount {
		return errors.New("policy snapshot chunk metadata is invalid")
	}
	if snapshot.ItemCount < len(snapshot.Members)+len(snapshot.DeletedAccountTombstones) || snapshot.ItemCount > maxRemotePolicyMembers+maxRemotePolicyTombstones {
		return errors.New("policy snapshot item count is invalid")
	}
	if snapshot.EncodedBytes > maxRemotePolicyEncoded || snapshot.DecodedBytes > maxRemotePolicyDecoded {
		return errors.New("policy snapshot byte metadata exceeds its bound")
	}
	if snapshot.ChunkCount > 1 {
		if len(snapshot.ManifestChunkHashes) != snapshot.ChunkCount || len(snapshot.ManifestChunkEncodedBytes) != snapshot.ChunkCount || len(snapshot.ManifestChunkDecodedBytes) != snapshot.ChunkCount {
			return errors.New("policy manifest chunk bounds are incomplete")
		}
		if _, err := normalizedSHA256Digest(snapshot.ContentRoot); err != nil {
			return errors.New("policy content root is invalid")
		}
		if _, err := normalizedSHA256Digest(snapshot.ChunkDigest); err != nil {
			return errors.New("policy chunk digest is invalid")
		}
		for i := range snapshot.ManifestChunkHashes {
			if _, err := normalizedSHA256Digest(snapshot.ManifestChunkHashes[i]); err != nil || snapshot.ManifestChunkEncodedBytes[i] <= 0 || snapshot.ManifestChunkEncodedBytes[i] > maxRemotePolicyEncoded || snapshot.ManifestChunkDecodedBytes[i] <= 0 || snapshot.ManifestChunkDecodedBytes[i] > maxRemotePolicyDecoded {
				return errors.New("policy manifest chunk bounds are invalid")
			}
		}
	}
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
	if len(snapshot.PolicyRoot) > 256 {
		return errors.New("policy root exceeds its bound")
	}
	if _, err := normalizedSHA256Digest(snapshot.Digest); err != nil {
		return errors.New("policy snapshot digest is invalid")
	}
	if snapshot.ManifestDigest != "" {
		if _, err := normalizedSHA256Digest(snapshot.ManifestDigest); err != nil {
			return errors.New("policy manifest digest is invalid")
		}
	}
	if _, err := normalizedSHA256Digest(snapshot.PolicyDigest); err != nil {
		return errors.New("policy state digest is invalid")
	}
	if snapshot.ContentRoot != "" {
		if _, err := normalizedSHA256Digest(snapshot.ContentRoot); err != nil {
			return errors.New("policy content root is invalid")
		}
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
		if previous.PolicyRevision > 0 && snapshot.PolicyRevision < previous.PolicyRevision {
			return errors.New("policy revision is older than the last accepted revision")
		}
		if previous.PolicyRevision > 0 && snapshot.PolicyRevision == previous.PolicyRevision {
			if !strings.EqualFold(strings.TrimSpace(snapshot.PolicyDigest), strings.TrimSpace(previous.PolicyDigest)) ||
				(previous.PolicyRoot != "" && snapshot.PolicyRoot != previous.PolicyRoot) ||
				(previous.ContentRoot != "" && !strings.EqualFold(strings.TrimSpace(snapshot.ContentRoot), strings.TrimSpace(previous.ContentRoot))) {
				return errors.New("policy revision collides with different signed authority")
			}
		}
		manifestDigest := snapshot.Digest
		if snapshot.ManifestDigest != "" {
			manifestDigest = snapshot.ManifestDigest
		}
		if snapshot.Generation == previous.Generation {
			if previous.PolicyRevision > 0 && snapshot.PolicyRevision != previous.PolicyRevision {
				return errors.New("policy generation collides with a different policy revision")
			}
			previousIssuedAt, parseErr := time.Parse(time.RFC3339Nano, previous.IssuedAt)
			if parseErr == nil {
				if issuedAt.Before(previousIssuedAt) {
					return errors.New("policy is older than the last accepted snapshot")
				}
				if issuedAt.Equal(previousIssuedAt) &&
					(snapshot.SnapshotID != previous.SnapshotID || !strings.EqualFold(strings.TrimSpace(manifestDigest), strings.TrimSpace(previous.SnapshotDigest))) {
					return errors.New("policy generation collides with different signed state")
				}
			} else if snapshot.SnapshotID != previous.SnapshotID || !strings.EqualFold(strings.TrimSpace(manifestDigest), strings.TrimSpace(previous.SnapshotDigest)) {
				return errors.New("policy generation collides with different signed state")
			}
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
	issuedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(state.IssuedAt))
	return err != nil || !issuedAt.Add(hostedPolicyRenewalInterval).After(now)
}

func remotePolicyContinuity(state remotePolicyState, now time.Time) string {
	expiresAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(state.ExpiresAt))
	if err != nil {
		return "unknown"
	}
	if floor, floorErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(state.TrustedTimeFloor)); floorErr == nil && now.Before(floor.Add(-hostedDocumentClockSkew)) {
		return "clock-invalid"
	}
	if now.Before(expiresAt) {
		return "valid"
	}
	return "hard-expired"
}

// advanceRemotePolicyTrustedTimeFloor records authenticated passage of time
// without changing the signed policy expiry. The coarse interval avoids a
// local database write on every heartbeat while still bounding how much a
// clock rollback can manufacture after a restart.
func (s *Server) advanceRemotePolicyTrustedTimeFloor(expected remotePolicyState, now time.Time) error {
	s.remotePolicySyncMu.Lock()
	defer s.remotePolicySyncMu.Unlock()
	state := s.loadRemotePolicyState()
	if strings.TrimSpace(state.SnapshotID) == "" {
		return nil
	}
	if state.AckPending || state.SnapshotID != expected.SnapshotID || state.Generation != expected.Generation ||
		!strings.EqualFold(strings.TrimSpace(state.PolicyDigest), strings.TrimSpace(expected.PolicyDigest)) {
		return nil
	}
	floor, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(state.TrustedTimeFloor))
	if err != nil {
		if strings.TrimSpace(state.TrustedTimeFloor) != "" {
			return errors.New("trusted policy time floor is invalid")
		}
		floor = time.Time{}
	}
	if !floor.IsZero() && now.Before(floor.Add(-hostedDocumentClockSkew)) {
		return errors.New("system clock moved behind the trusted policy time floor")
	}
	if !floor.IsZero() && !now.After(floor.Add(trustedTimeFloorInterval)) {
		return nil
	}
	state.TrustedTimeFloor = now.UTC().Format(time.RFC3339Nano)
	return s.saveRemotePolicyState(state)
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
	encrypted, err := s.encryptedRemotePolicyState(state)
	if err != nil {
		return err
	}
	_, err = s.execUserWrite(context.Background(), `INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, remoteAccessPolicyStateKey, string(encrypted), time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Server) encryptedRemotePolicyState(state remotePolicyState) ([]byte, error) {
	encoded, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	return s.encryptRemoteSecret(string(encoded))
}

func saveRemotePolicyStateTx(tx *sql.Tx, encrypted []byte, now time.Time) error {
	if tx == nil || len(encrypted) == 0 {
		return errors.New("encrypted remote policy state is unavailable")
	}
	_, err := tx.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, remoteAccessPolicyStateKey, string(encrypted), now.UTC().Format(time.RFC3339Nano))
	return err
}
