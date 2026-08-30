package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func collectionCursorScope(parts ...string) string {
	data, _ := json.Marshal(parts)
	digest := sha256.Sum256(data)
	return "collection:" + base64.RawURLEncoding.EncodeToString(digest[:])
}

func (s *Server) decodeCollectionCursor(r *http.Request, scope, principal string, now time.Time, target any) error {
	token := strings.TrimSpace(r.URL.Query().Get("cursor"))
	if token == "" {
		return nil
	}
	var raw json.RawMessage
	if err := s.decodeContractCursor(token, scope, principal, &raw, now); err != nil {
		return err
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return fmt.Errorf("%w: malformed collection payload", errInvalidCursor)
	}
	return nil
}

func (s *Server) collectionPageInfo(scope, principal string, after any, hasMore bool, total *int, now time.Time) (CursorPageInfo, error) {
	pageInfo := CursorPageInfo{HasMore: hasMore, Total: total}
	if !hasMore {
		return pageInfo, nil
	}
	if after == nil {
		return CursorPageInfo{}, fmt.Errorf("%w: keyset boundary is required", errInvalidCursor)
	}
	cursor, err := s.encodeContractCursor(scope, principal, after, now)
	if err != nil {
		return CursorPageInfo{}, err
	}
	pageInfo.NextCursor = &cursor
	return pageInfo, nil
}

func writeCollectionCursorError(w http.ResponseWriter, err error, resource string) {
	if errors.Is(err, errCursorExpired) {
		writeError(w, http.StatusBadRequest, "cursor_expired", "The "+resource+" cursor expired. Restart from the first page.")
		return
	}
	writeError(w, http.StatusBadRequest, "invalid_cursor", "The "+resource+" cursor is invalid for this account and result set.")
}

const (
	cursorEnvelopeRevision = 1
	cursorDefaultTTL       = time.Hour
	cursorKeySize          = 32
)

var (
	errInvalidCursor = errors.New("invalid cursor")
	errCursorExpired = errors.New("cursor expired")
)

type cursorEnvelope struct {
	Revision  int             `json:"v"`
	Scope     string          `json:"s"`
	Principal string          `json:"p"`
	ExpiresAt int64           `json:"exp"`
	Payload   json.RawMessage `json:"d"`
}

// encodeContractCursor returns an encrypted, authenticated cursor. The scope
// binds it to the normalized query and the principal binds it to the current
// visibility boundary. Cursor payloads are intentionally not readable by a
// client, even when their contents are not independently secret.
func (s *Server) encodeContractCursor(scope, principal string, payload any, now time.Time) (string, error) {
	scope = strings.TrimSpace(scope)
	principal = strings.TrimSpace(principal)
	if scope == "" || principal == "" {
		return "", fmt.Errorf("%w: scope and principal are required", errInvalidCursor)
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal cursor payload: %w", err)
	}
	envelope, err := json.Marshal(cursorEnvelope{
		Revision:  cursorEnvelopeRevision,
		Scope:     scope,
		Principal: principal,
		ExpiresAt: now.Add(cursorDefaultTTL).Unix(),
		Payload:   data,
	})
	if err != nil {
		return "", fmt.Errorf("marshal cursor envelope: %w", err)
	}
	aead, err := s.cursorAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("create cursor nonce: %w", err)
	}
	sealed := aead.Seal(nil, nonce, envelope, []byte("portico-cursor-v1"))
	token := append(nonce, sealed...)
	return base64.RawURLEncoding.EncodeToString(token), nil
}

func (s *Server) decodeContractCursor(token, scope, principal string, target any, now time.Time) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return fmt.Errorf("%w: malformed token", errInvalidCursor)
	}
	aead, err := s.cursorAEAD()
	if err != nil {
		return err
	}
	if len(raw) <= aead.NonceSize() {
		return fmt.Errorf("%w: malformed token", errInvalidCursor)
	}
	nonce, ciphertext := raw[:aead.NonceSize()], raw[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, ciphertext, []byte("portico-cursor-v1"))
	if err != nil {
		return fmt.Errorf("%w: authentication failed", errInvalidCursor)
	}
	var envelope cursorEnvelope
	if err := json.Unmarshal(plain, &envelope); err != nil {
		return fmt.Errorf("%w: malformed envelope", errInvalidCursor)
	}
	if envelope.Revision != cursorEnvelopeRevision || envelope.Scope != strings.TrimSpace(scope) || envelope.Principal != strings.TrimSpace(principal) {
		return fmt.Errorf("%w: cursor does not belong to this result set", errInvalidCursor)
	}
	if envelope.ExpiresAt <= 0 || !now.Before(time.Unix(envelope.ExpiresAt, 0)) {
		return errCursorExpired
	}
	if target == nil {
		return nil
	}
	if err := json.Unmarshal(envelope.Payload, target); err != nil {
		return fmt.Errorf("%w: malformed payload", errInvalidCursor)
	}
	return nil
}

func (s *Server) cursorAEAD() (cipher.AEAD, error) {
	key, err := s.cursorSigningKey()
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("initialize cursor encryption: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize cursor authentication: %w", err)
	}
	return aead, nil
}

func (s *Server) cursorSigningKey() ([]byte, error) {
	path := filepath.Join(s.cfg.AppDataDir, "secrets", "cursor-aead.key")
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != cursorKeySize {
			return nil, errors.New("cursor key has an invalid length")
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read cursor key: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create cursor key directory: %w", err)
	}
	key = make([]byte, cursorKeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("generate cursor key: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if os.IsExist(err) {
			key, readErr := os.ReadFile(path)
			if readErr != nil {
				return nil, fmt.Errorf("read concurrently-created cursor key: %w", readErr)
			}
			if len(key) != cursorKeySize {
				return nil, errors.New("cursor key has an invalid length")
			}
			return key, nil
		}
		return nil, fmt.Errorf("create cursor key: %w", err)
	}
	if _, err := file.Write(key); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("write cursor key: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return nil, fmt.Errorf("close cursor key: %w", err)
	}
	return key, nil
}
