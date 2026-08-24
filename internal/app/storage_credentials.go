package app

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// saveStorageSourceCredential persists WebDAV/rclone authority only through
// the platform secret envelope. Callers and API projections expose presence,
// never the stored envelope or recovered credential.
func (s *Server) saveStorageSourceCredential(ctx context.Context, sourceID, username, secret string) error {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" || secret == "" {
		return errors.New("storage source and credential are required")
	}
	envelope, err := s.encryptRemoteSecret(secret)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.execBackgroundWrite(ctx, `INSERT INTO storage_source_credentials(source_id,username,secret_envelope,updated_at) VALUES(?,?,?,?) ON CONFLICT(source_id) DO UPDATE SET username=excluded.username,secret_envelope=excluded.secret_envelope,updated_at=excluded.updated_at`, sourceID, strings.TrimSpace(username), string(envelope), now)
	return err
}

func (s *Server) storageSourceCredential(ctx context.Context, sourceID string) (username, secret string, err error) {
	var raw string
	err = s.queryBackgroundRow(ctx, `SELECT username,secret_envelope FROM storage_source_credentials WHERE source_id=?`, strings.TrimSpace(sourceID)).Scan(&username, &raw)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	if err != nil {
		return "", "", err
	}
	secret, err = s.decryptRemoteSecret([]byte(raw))
	if err != nil {
		return "", "", err
	}
	return username, secret, nil
}

func (s *Server) deleteStorageSourceCredential(ctx context.Context, sourceID string) error {
	_, err := s.execBackgroundWrite(ctx, `DELETE FROM storage_source_credentials WHERE source_id=?`, strings.TrimSpace(sourceID))
	return err
}
