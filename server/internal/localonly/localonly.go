// Package localonly answers whether a library uses local metadata only (its
// metadata agent is "local"), for every package that can reach the network on
// a library's behalf. It only reads.
package localonly

import (
	"context"
	"database/sql"
	"errors"
)

// Err answers an online lookup requested for a local-only library.
var Err = errors.New("This library uses local metadata only. Change its metadata source to look things up online.")

type querier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// Library reports whether the library must never go online.
func Library(ctx context.Context, q querier, library string) (bool, error) {
	var local bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM library_metadata_agents WHERE library_id=? AND agent='local')`, library).Scan(&local)
	return local, err
}

// Item reports whether the library holding the item must never go online.
func Item(ctx context.Context, q querier, item string) (bool, error) {
	var local bool
	err := q.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id JOIN library_metadata_agents a ON a.library_id=cl.library_id WHERE e.public_id=pid_blob(?) AND a.agent='local')`, item).Scan(&local)
	return local, err
}
