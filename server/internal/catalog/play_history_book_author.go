package catalog

import (
	"database/sql"
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"

	"portico.local/server/internal/compactcatalog"
)

// capturePlayHistoryBookAuthor reads the canonical book author for an accepted
// audiobook file in insertPlay's existing transaction. The caller retains the
// nullable value once; history reads must never fill it from current metadata.
// ParentTitle remains the book title, including its complete stored suffix.
func capturePlayHistoryBookAuthor(tx *sql.Tx, entityID int64) (sql.NullString, error) {
	var author sql.NullString
	err := tx.QueryRow(`SELECT b.author FROM catalog_book_files f
 JOIN catalog_entities file ON file.id=f.entity_id AND file.kind=? AND file.retired=0
 JOIN catalog_books b ON b.entity_id=f.book_id
 JOIN catalog_entities book ON book.id=b.entity_id AND book.kind=? AND book.retired=0
 AND book.library_id=file.library_id AND b.library_id=book.library_id
 WHERE f.entity_id=?`, compactcatalog.Part, compactcatalog.Book, entityID).Scan(&author)
	if errors.Is(err, sql.ErrNoRows) {
		return sql.NullString{}, nil
	}
	if err != nil {
		return sql.NullString{}, err
	}
	// Keep the actual canonical text. Neither an absent author nor unreadable
	// metadata can become a title, narrator, filename or private locator.
	if !author.Valid || strings.TrimSpace(author.String) == "" || len(author.String) > 1024 ||
		!utf8.ValidString(author.String) || strings.ContainsFunc(author.String, unicode.IsControl) ||
		strings.Contains(author.String, "://") || strings.HasPrefix(author.String, "/") || strings.Contains(author.String, "\\") {
		return sql.NullString{}, nil
	}
	return author, nil
}
