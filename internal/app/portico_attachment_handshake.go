package app

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	porticoAttachmentHandshakeVersion = 1
	porticoAttachmentHandshakeTTL     = 60 * time.Second
	porticoAttachmentHandshakeLimit   = 256
	porticoAttachmentPerIPLimit       = 16
	porticoAttachmentMaxCiphertext    = 96 * 1024
	porticoAttachmentMaxRequestBody   = 132 * 1024
	porticoAttachmentMaxHandshakeBody = 8 * 1024
)

type PorticoAttachmentHandshakeRequest struct {
	Version         int    `json:"version"`
	ClientPublicKey string `json:"clientPublicKey"`
	ClientNonce     string `json:"clientNonce"`
}

type PorticoAttachmentHandshakeResponse struct {
	Version                    int    `json:"version"`
	HandshakeID                string `json:"handshakeId"`
	ServerID                   string `json:"serverId"`
	ServerPublicKey            string `json:"serverPublicKey"`
	ServerPublicKeyFingerprint string `json:"serverPublicKeyFingerprint"`
	ClientPublicKey            string `json:"clientPublicKey"`
	ClientNonce                string `json:"clientNonce"`
	ServerEphemeralPublicKey   string `json:"serverEphemeralPublicKey"`
	Audience                   string `json:"audience"`
	IssuedAt                   string `json:"issuedAt"`
	ExpiresAt                  string `json:"expiresAt"`
	SignatureAlgorithm         string `json:"signatureAlgorithm"`
	Signature                  string `json:"signature"`
}

type PorticoAttachmentEncryptedRequest struct {
	Version     int    `json:"version"`
	HandshakeID string `json:"handshakeId"`
	Ciphertext  string `json:"ciphertext"`
}

type PorticoAttachmentEncryptedResponse struct {
	Version     int    `json:"version"`
	HandshakeID string `json:"handshakeId"`
	Ciphertext  string `json:"ciphertext"`
}

type porticoAttachmentProtectedResponse struct {
	Status     int             `json:"status"`
	RetryAfter string          `json:"retryAfter,omitempty"`
	Body       json.RawMessage `json:"body"`
}

type porticoAttachmentHandshakeState struct {
	clientIP      string
	key           []byte
	requestNonce  []byte
	responseNonce []byte
	requestAAD    []byte
	responseAAD   []byte
	expiresAt     time.Time
	status        string
	requestDigest [sha256.Size]byte
	responseBody  []byte
}

type porticoAttachmentCapture struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (capture *porticoAttachmentCapture) Header() http.Header {
	return capture.header
}

func (capture *porticoAttachmentCapture) WriteHeader(status int) {
	if capture.status == 0 {
		capture.status = status
	}
}

func (capture *porticoAttachmentCapture) Write(body []byte) (int, error) {
	if capture.status == 0 {
		capture.status = http.StatusOK
	}
	return capture.body.Write(body)
}

func (s *Server) handlePorticoAttachmentHandshake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	if !s.porticoAccountMode() {
		writeError(w, http.StatusConflict, "portico_auth_unavailable", "This server is not configured for Portico Account sign-in.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, porticoAttachmentMaxHandshakeBody)
	var req PorticoAttachmentHandshakeRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Version != porticoAttachmentHandshakeVersion {
		writeError(w, http.StatusBadRequest, "attachment_handshake_invalid", "The attachment handshake version is not supported.")
		return
	}
	clientPublicBytes, err := decodePorticoAttachmentValue(req.ClientPublicKey, 65)
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_handshake_invalid", "The client attachment key is invalid.")
		return
	}
	clientNonce, err := decodePorticoAttachmentValue(req.ClientNonce, 32)
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_handshake_invalid", "The client attachment nonce is invalid.")
		return
	}
	clientPublic, err := ecdh.P256().NewPublicKey(clientPublicBytes)
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_handshake_invalid", "The client attachment key is invalid.")
		return
	}
	settings, err := s.remoteAccessSettings()
	if err != nil || strings.TrimSpace(settings.ServerID) == "" {
		writeError(w, http.StatusConflict, "portico_auth_unavailable", "This server is not connected to Portico Hosted Services.")
		return
	}
	identity, err := s.loadOrCreateServerIdentity()
	if err != nil || len(identity.PrivateKey) != ed25519.PrivateKeySize {
		writeError(w, http.StatusInternalServerError, "attachment_handshake_failed", "The server identity is unavailable.")
		return
	}
	serverEphemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "attachment_handshake_failed", "The server could not create an attachment key.")
		return
	}
	sharedSecret, err := serverEphemeral.ECDH(clientPublic)
	if err != nil {
		writeError(w, http.StatusBadRequest, "attachment_handshake_invalid", "The attachment key agreement failed.")
		return
	}
	audience := strings.TrimRight(s.requestPublicOrigin(r), "/")
	if audience == "" {
		writeError(w, http.StatusBadRequest, "attachment_handshake_invalid", "The attachment route origin is unavailable.")
		return
	}
	now := time.Now().UTC()
	handshakeID := randomID("attach")
	response := PorticoAttachmentHandshakeResponse{
		Version:                    porticoAttachmentHandshakeVersion,
		HandshakeID:                handshakeID,
		ServerID:                   strings.TrimSpace(settings.ServerID),
		ServerPublicKey:            base64.RawURLEncoding.EncodeToString(identity.PublicKey),
		ServerPublicKeyFingerprint: identity.Fingerprint,
		ClientPublicKey:            base64.RawURLEncoding.EncodeToString(clientPublicBytes),
		ClientNonce:                base64.RawURLEncoding.EncodeToString(clientNonce),
		ServerEphemeralPublicKey:   base64.RawURLEncoding.EncodeToString(serverEphemeral.PublicKey().Bytes()),
		Audience:                   audience,
		IssuedAt:                   now.Format(time.RFC3339Nano),
		ExpiresAt:                  now.Add(porticoAttachmentHandshakeTTL).Format(time.RFC3339Nano),
		SignatureAlgorithm:         "ed25519",
	}
	transcript := porticoAttachmentTranscript(response)
	response.Signature = base64.RawURLEncoding.EncodeToString(ed25519.Sign(identity.PrivateKey, transcript))
	key := porticoAttachmentKey(sharedSecret, transcript)
	state := &porticoAttachmentHandshakeState{
		clientIP:      clientIPFromRequest(r),
		key:           key,
		requestNonce:  porticoAttachmentDerivedBytes("request-nonce", handshakeID, response.ServerID, identity.Fingerprint, 12),
		responseNonce: porticoAttachmentDerivedBytes("response-nonce", handshakeID, response.ServerID, identity.Fingerprint, 12),
		requestAAD:    porticoAttachmentAAD("request", handshakeID, response.ServerID, identity.Fingerprint),
		responseAAD:   porticoAttachmentAAD("response", handshakeID, response.ServerID, identity.Fingerprint),
		expiresAt:     now.Add(porticoAttachmentHandshakeTTL),
		status:        "pending",
	}
	if !s.storePorticoAttachmentHandshake(handshakeID, state, now) {
		writeError(w, http.StatusTooManyRequests, "attachment_handshake_busy", "Too many attachment handshakes are active. Try again shortly.")
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

func (s *Server) handlePorticoSessionAttach(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Use POST for this endpoint.")
		return
	}
	var encrypted PorticoAttachmentEncryptedRequest
	r.Body = http.MaxBytesReader(w, r.Body, porticoAttachmentMaxRequestBody)
	if !decodeJSON(w, r, &encrypted) {
		return
	}
	if encrypted.Version != porticoAttachmentHandshakeVersion || strings.TrimSpace(encrypted.HandshakeID) == "" || len(encrypted.Ciphertext) > porticoAttachmentMaxCiphertext*2 {
		writeError(w, http.StatusBadRequest, "attachment_handshake_invalid", "The encrypted attachment request is invalid.")
		return
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(encrypted.Ciphertext)
	if err != nil || len(ciphertext) == 0 || len(ciphertext) > porticoAttachmentMaxCiphertext {
		writeError(w, http.StatusBadRequest, "attachment_handshake_invalid", "The encrypted attachment request is invalid.")
		return
	}
	state, cached, ok := s.claimPorticoAttachmentHandshake(encrypted.HandshakeID, ciphertext, clientIPFromRequest(r), time.Now().UTC())
	if !ok {
		writeError(w, http.StatusUnauthorized, "attachment_handshake_invalid", "The attachment handshake is invalid, expired, or already used.")
		return
	}
	if cached {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(state.responseBody)
		return
	}
	plaintext, err := openPorticoAttachment(state.key, state.requestNonce, state.requestAAD, ciphertext)
	if err != nil || len(plaintext) == 0 || len(plaintext) > porticoAttachmentMaxCiphertext {
		s.discardPorticoAttachmentHandshake(encrypted.HandshakeID)
		writeError(w, http.StatusUnauthorized, "attachment_handshake_invalid", "The encrypted attachment request could not be authenticated.")
		return
	}
	var request PorticoSessionAttachRequest
	decoder := json.NewDecoder(bytes.NewReader(plaintext))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		s.discardPorticoAttachmentHandshake(encrypted.HandshakeID)
		writeError(w, http.StatusBadRequest, "attachment_handshake_invalid", "The encrypted attachment payload is invalid.")
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		s.discardPorticoAttachmentHandshake(encrypted.HandshakeID)
		writeError(w, http.StatusBadRequest, "attachment_handshake_invalid", "The encrypted attachment payload is invalid.")
		return
	}
	requestBody, _ := json.Marshal(request)
	inner := r.Clone(r.Context())
	inner.Body = io.NopCloser(bytes.NewReader(requestBody))
	inner.ContentLength = int64(len(requestBody))
	capture := &porticoAttachmentCapture{header: make(http.Header)}
	s.handlePorticoSessionAttachDecrypted(capture, inner)
	status := capture.status
	if status == 0 {
		status = http.StatusInternalServerError
	}
	protected, err := json.Marshal(porticoAttachmentProtectedResponse{
		Status: status, RetryAfter: capture.header.Get("Retry-After"), Body: append(json.RawMessage(nil), capture.body.Bytes()...),
	})
	if err != nil {
		s.discardPorticoAttachmentHandshake(encrypted.HandshakeID)
		writeError(w, http.StatusInternalServerError, "attachment_handshake_failed", "The attachment response could not be protected.")
		return
	}
	sealed, err := sealPorticoAttachment(state.key, state.responseNonce, state.responseAAD, protected)
	if err != nil {
		s.discardPorticoAttachmentHandshake(encrypted.HandshakeID)
		writeError(w, http.StatusInternalServerError, "attachment_handshake_failed", "The attachment response could not be protected.")
		return
	}
	outer, _ := json.Marshal(PorticoAttachmentEncryptedResponse{
		Version: porticoAttachmentHandshakeVersion, HandshakeID: encrypted.HandshakeID,
		Ciphertext: base64.RawURLEncoding.EncodeToString(sealed),
	})
	s.completePorticoAttachmentHandshake(encrypted.HandshakeID, outer)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(outer)
}

func decodePorticoAttachmentValue(value string, length int) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil || len(decoded) != length {
		return nil, errors.New("invalid attachment value")
	}
	return decoded, nil
}

func porticoAttachmentTranscript(response PorticoAttachmentHandshakeResponse) []byte {
	return []byte(strings.Join([]string{
		"portico-attachment-handshake-v1",
		"handshakeId=" + response.HandshakeID,
		"serverId=" + response.ServerID,
		"serverPublicKey=" + response.ServerPublicKey,
		"serverPublicKeyFingerprint=" + response.ServerPublicKeyFingerprint,
		"clientPublicKey=" + response.ClientPublicKey,
		"clientNonce=" + response.ClientNonce,
		"serverEphemeralPublicKey=" + response.ServerEphemeralPublicKey,
		"audience=" + response.Audience,
		"issuedAt=" + response.IssuedAt,
		"expiresAt=" + response.ExpiresAt,
	}, "\n"))
}

func porticoAttachmentKey(sharedSecret, transcript []byte) []byte {
	salt := sha256.Sum256(transcript)
	extract := hmac.New(sha256.New, salt[:])
	_, _ = extract.Write(sharedSecret)
	prk := extract.Sum(nil)
	expand := hmac.New(sha256.New, prk)
	_, _ = expand.Write([]byte("portico-attachment-aead-v1"))
	_, _ = expand.Write([]byte{1})
	return expand.Sum(nil)
}

func porticoAttachmentDerivedBytes(label, handshakeID, serverID, fingerprint string, length int) []byte {
	digest := sha256.Sum256([]byte(strings.Join([]string{"portico-attachment-v1", label, handshakeID, serverID, fingerprint}, "\n")))
	return append([]byte(nil), digest[:length]...)
}

func porticoAttachmentAAD(direction, handshakeID, serverID, fingerprint string) []byte {
	return []byte(strings.Join([]string{"portico-attachment-aead-v1", direction, handshakeID, serverID, fingerprint, "/api/auth/portico/sessions"}, "\n"))
}

func sealPorticoAttachment(key, nonce, aad, plaintext []byte) ([]byte, error) {
	aead, err := porticoAttachmentAEAD(key)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nonce, plaintext, aad), nil
}

func openPorticoAttachment(key, nonce, aad, ciphertext []byte) ([]byte, error) {
	aead, err := porticoAttachmentAEAD(key)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, aad)
}

func porticoAttachmentAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func (s *Server) storePorticoAttachmentHandshake(id string, state *porticoAttachmentHandshakeState, now time.Time) bool {
	s.porticoAttachmentMu.Lock()
	defer s.porticoAttachmentMu.Unlock()
	if s.porticoAttachmentHandshakes == nil {
		s.porticoAttachmentHandshakes = make(map[string]*porticoAttachmentHandshakeState)
	}
	for existingID, existing := range s.porticoAttachmentHandshakes {
		if !existing.expiresAt.After(now) {
			delete(s.porticoAttachmentHandshakes, existingID)
		}
	}
	if len(s.porticoAttachmentHandshakes) >= porticoAttachmentHandshakeLimit {
		return false
	}
	perIP := 0
	for _, existing := range s.porticoAttachmentHandshakes {
		if existing.clientIP == state.clientIP {
			perIP++
		}
	}
	if perIP >= porticoAttachmentPerIPLimit {
		return false
	}
	s.porticoAttachmentHandshakes[id] = state
	return true
}

func (s *Server) claimPorticoAttachmentHandshake(id string, ciphertext []byte, clientIP string, now time.Time) (*porticoAttachmentHandshakeState, bool, bool) {
	s.porticoAttachmentMu.Lock()
	defer s.porticoAttachmentMu.Unlock()
	state := s.porticoAttachmentHandshakes[id]
	if state == nil || !state.expiresAt.After(now) {
		delete(s.porticoAttachmentHandshakes, id)
		return nil, false, false
	}
	if !hmac.Equal([]byte(state.clientIP), []byte(clientIP)) {
		return nil, false, false
	}
	digest := sha256.Sum256(ciphertext)
	if state.status == "complete" && hmac.Equal(state.requestDigest[:], digest[:]) {
		return state, true, true
	}
	if state.status != "pending" {
		return nil, false, false
	}
	state.status = "processing"
	state.requestDigest = digest
	return state, false, true
}

func (s *Server) completePorticoAttachmentHandshake(id string, body []byte) {
	s.porticoAttachmentMu.Lock()
	defer s.porticoAttachmentMu.Unlock()
	if state := s.porticoAttachmentHandshakes[id]; state != nil && state.status == "processing" {
		state.status = "complete"
		state.responseBody = append([]byte(nil), body...)
		clear(state.key)
		clear(state.requestNonce)
		clear(state.responseNonce)
		clear(state.requestAAD)
		clear(state.responseAAD)
		state.key = nil
		state.requestNonce = nil
		state.responseNonce = nil
		state.requestAAD = nil
		state.responseAAD = nil
	}
}

func (s *Server) discardPorticoAttachmentHandshake(id string) {
	s.porticoAttachmentMu.Lock()
	defer s.porticoAttachmentMu.Unlock()
	delete(s.porticoAttachmentHandshakes, id)
}

func (s *Server) porticoAttachmentHandshakeCount() int {
	s.porticoAttachmentMu.Lock()
	defer s.porticoAttachmentMu.Unlock()
	return len(s.porticoAttachmentHandshakes)
}
