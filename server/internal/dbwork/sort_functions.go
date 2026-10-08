package dbwork

import (
	"database/sql/driver"
	"strings"

	"modernc.org/sqlite"
	"portico.local/server/internal/sorttext"
)

func init() {
	sqlite.MustRegisterDeterministicScalarFunction("portico_sort_title", 3, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		title, _ := args[0].(string)
		explicit, _ := args[1].(string)
		language, _ := args[2].(string)
		return sorttext.Key(title, explicit, language), nil
	})
	// SQLite's lower() folds ASCII only; an author's identity key folds as Go
	// does, so SQL that names an author uses this.
	sqlite.MustRegisterDeterministicScalarFunction("portico_author_key", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		name, _ := args[0].(string)
		return AuthorIdentityKey(name), nil
	})
	sqlite.MustRegisterDeterministicScalarFunction("portico_letter", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		title, _ := args[0].(string)
		return sorttext.Letter(title), nil
	})
}

// AuthorIdentityKey is an author's identity key (compactcatalog.AuthorKey).
func AuthorIdentityKey(name string) string { return "author:" + strings.ToLower(name) }
