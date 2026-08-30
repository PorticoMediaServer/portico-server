package app

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestContractCursorIsEncryptedBoundAndExpiring(t *testing.T) {
	server := &Server{cfg: config.Config{AppDataDir: t.TempDir()}}
	now := time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)
	payload := struct {
		ID string `json:"id"`
	}{ID: "media_private_identifier"}

	token, err := server.encodeContractCursor("browse:lib_movies:query", "usr_owner", payload, now)
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	decodedToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode token transport: %v", err)
	}
	if strings.Contains(string(decodedToken), payload.ID) || strings.Contains(token, payload.ID) {
		t.Fatal("cursor exposed its payload")
	}

	var decoded struct {
		ID string `json:"id"`
	}
	if err := server.decodeContractCursor(token, "browse:lib_movies:query", "usr_owner", &decoded, now.Add(time.Minute)); err != nil {
		t.Fatalf("decode cursor: %v", err)
	}
	if decoded.ID != payload.ID {
		t.Fatalf("decoded payload = %#v", decoded)
	}
	if err := server.decodeContractCursor(token, "browse:lib_tv:query", "usr_owner", &decoded, now.Add(time.Minute)); !errors.Is(err, errInvalidCursor) {
		t.Fatalf("cross-scope cursor error = %v", err)
	}
	if err := server.decodeContractCursor(token, "browse:lib_movies:query", "usr_other", &decoded, now.Add(time.Minute)); !errors.Is(err, errInvalidCursor) {
		t.Fatalf("cross-principal cursor error = %v", err)
	}
	if err := server.decodeContractCursor(token, "browse:lib_movies:query", "usr_owner", &decoded, now.Add(cursorDefaultTTL)); !errors.Is(err, errCursorExpired) {
		t.Fatalf("expired cursor error = %v", err)
	}
}

func TestContractCursorRejectsTampering(t *testing.T) {
	server := &Server{cfg: config.Config{AppDataDir: t.TempDir()}}
	token, err := server.encodeContractCursor("search:movies", "usr_owner", map[string]any{"id": "m1"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode cursor transport: %v", err)
	}
	// Alter an authenticated byte rather than the final base64 character. The
	// latter can contain unused encoding bits and may still decode to the exact
	// same bytes, which is not a mutation of the protected cursor at all.
	raw[len(raw)-1] ^= 0x01
	tampered := base64.RawURLEncoding.EncodeToString(raw)
	if err := server.decodeContractCursor(tampered, "search:movies", "usr_owner", &map[string]any{}, time.Now().UTC()); !errors.Is(err, errInvalidCursor) {
		t.Fatalf("tampered cursor error = %v", err)
	}
}
