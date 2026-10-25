package catalog

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ExpectedRemoteSource binds an explicit folder selection to the remote
// connection the owner saw. It adds no authority to read the path itself.
type ExpectedRemoteSource struct {
	SourceID   string `json:"sourceId"`
	Generation int64  `json:"generation"`
}

func checkExpectedRemoteSource(ctx context.Context, tx *sql.Tx, path string, expected *ExpectedRemoteSource) error {
	if expected == nil {
		return nil
	}
	if expected.SourceID == "" || len(expected.SourceID) > 256 || !utf8.ValidString(expected.SourceID) || strings.IndexFunc(expected.SourceID, unicode.IsControl) >= 0 || expected.Generation < 1 {
		return ErrAdminQuery
	}
	var id string
	var generation int64
	// Use the exact association expression the accepting INSERT/UPDATE uses,
	// in the same transaction. A new nearer prefix must not silently win.
	err := tx.QueryRowContext(ctx, `SELECT id,generation FROM remote_sources WHERE id=`+remoteSourceIDExpr, path, path).Scan(&id, &generation)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrSourceChanged
	}
	if err != nil {
		return err
	}
	if id != expected.SourceID || generation != expected.Generation {
		return ErrSourceChanged
	}
	return nil
}

// CreateFoldersExpectedSourceAuthorizedThen keeps the optional remote fence in
// the accepting library transaction, including its first and every later folder.
// Local callers retain the existing SourceGuard and nil-fence behavior.
func (s *Service) CreateFoldersExpectedSourceAuthorizedThen(ctx context.Context, name, kind string, folders []string, expected *ExpectedRemoteSource, authorize func(*sql.Tx) error, created func(*sql.Tx, Library) error) (Library, error) {
	if expected == nil {
		return s.CreateFoldersAuthorizedThen(ctx, name, kind, folders, authorize, created)
	}
	captured := *expected
	paths := append([]string(nil), folders...)
	return s.CreateFoldersAuthorizedThen(ctx, name, kind, paths, func(tx *sql.Tx) error {
		if authorize != nil {
			if err := authorize(tx); err != nil {
				return err
			}
		}
		for _, path := range paths {
			if err := checkExpectedRemoteSource(ctx, tx, path, &captured); err != nil {
				return err
			}
		}
		return nil
	}, created)
}
