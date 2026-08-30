package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"strings"
)

const liveTVSecretPrefix = "ltv1:"

func (s *Server) sealLiveTVSourceSecret(sourceID, plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	aead, err := s.liveTVSourceAEAD()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.New("generate source-secret nonce")
	}
	sealed := aead.Seal(nonce, nonce, []byte(plaintext), []byte(sourceID))
	return liveTVSecretPrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (s *Server) openLiveTVSourceSecret(sourceID, envelope string) (string, error) {
	if envelope == "" {
		return "", nil
	}
	if !strings.HasPrefix(envelope, liveTVSecretPrefix) {
		return "", errors.New("invalid Live TV source credential")
	}
	encoded := strings.TrimPrefix(envelope, liveTVSecretPrefix)
	sealed, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("invalid Live TV source credential")
	}
	aead, err := s.liveTVSourceAEAD()
	if err != nil {
		return "", err
	}
	if len(sealed) < aead.NonceSize() {
		return "", errors.New("invalid Live TV source credential")
	}
	plaintext, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(sourceID))
	if err != nil {
		return "", errors.New("Live TV source credential could not be authenticated")
	}
	return string(plaintext), nil
}

func (s *Server) liveTVSourceAEAD() (cipher.AEAD, error) {
	root, err := s.nativeCredentialHMACKey()
	if err != nil {
		return nil, err
	}
	mac := hmac.New(sha256.New, root)
	_, _ = mac.Write([]byte("portico/live-tv/source-secret/v1"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
