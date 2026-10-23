package postgres

import (
	"context"
	"portico.local/hosted/internal/directory"
	"strings"
)

type ServerEvent struct {
	Kind string `json:"kind"`
	Name string `json:"name,omitempty"`
}

func (s *Store) ServerEvent(ctx context.Context, a directory.ServerAuthority, q ServerEvent) error {
	if q.Kind != "rename" && q.Kind != "shutdown" {
		return directory.ErrInvalid
	}
	if q.Kind == "rename" && (strings.TrimSpace(q.Name) == "" || len(q.Name) > 120 || strings.ContainsAny(q.Name, "\x00\r\n")) {
		return directory.ErrInvalid
	}
	tx, e := s.beginServerOperation(ctx, a)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if q.Kind == "rename" {
		_, e = tx.ExecContext(ctx, `UPDATE servers SET name=$2 WHERE id=$1`, a.ServerID, q.Name)
		if e == nil {
			e = touchServerListTx(ctx, tx, a.ServerID)
		}
	} else {
		if e = withdrawClaimAddressesTx(ctx, tx, a.ServerID, a.OperationID); e != nil {
			return e
		}
		_, e = tx.ExecContext(ctx, `UPDATE servers SET presence_expires_at=clock_timestamp(),endpoint_verified=false,base_url=NULL WHERE id=$1`, a.ServerID)
		if e == nil {
			_, e = tx.ExecContext(ctx, `DELETE FROM current_server_endpoints WHERE server_id=$1`, a.ServerID)
		}
		if e == nil {
			// Shutdown clears the address and presence the list shows.
			e = touchServerListTx(ctx, tx, a.ServerID)
		}
	}
	if e != nil {
		return e
	}
	return tx.Commit()
}
