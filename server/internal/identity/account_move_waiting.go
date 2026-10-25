package identity

import (
	"context"

	"portico.local/server/internal/dbwork"
)

// MoveWaiting is one account that still needs its link or code for the active
// move: the id the codes route accepts and the username the owner recognises. It
// never carries a code, hash or password material.
type MoveWaiting struct {
	AccountID string `json:"accountId"`
	Username  string `json:"username"`
}

// MoveWaitingList is bounded and ordered by username. Truncated says the waiting
// population is larger than this read lists; the list is never presented as
// complete when it is not.
type MoveWaitingList struct {
	Items     []MoveWaiting `json:"items"`
	Truncated bool          `json:"truncated"`
}

// MoveWaiting reads the accounts waiting on the active move, from the same
// owner-fenced snapshot and rows the codes writer validates against.
func (s *Service) MoveWaiting(ctx context.Context, bearer string) (out MoveWaitingList, err error) {
	out.Items = []MoveWaiting{}
	if s == nil || s.db == nil {
		return out, ErrDirectInput
	}
	snapshot, err := dbwork.BeginSnapshot(ctx, s.db)
	if err != nil {
		return out, err
	}
	defer snapshot.Rollback()
	if _, err = s.moveWebOwnerTx(ctx, snapshot.Tx(), bearer); err != nil {
		return out, err
	}
	rows, err := snapshot.Tx().QueryContext(ctx, `SELECT a.id,a.username FROM identity_move_waiting w JOIN accounts a ON a.id=w.account_id JOIN identity_moves m ON m.id=w.move_id WHERE w.state='waiting' AND m.stage IN('moving_to_local','moving_to_portico') ORDER BY a.username,a.id LIMIT 2001`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var item MoveWaiting
		if err = rows.Scan(&item.AccountID, &item.Username); err != nil {
			return out, err
		}
		out.Items = append(out.Items, item)
	}
	if err = rows.Err(); err != nil {
		return out, err
	}
	if len(out.Items) > 2000 {
		// One extra row proves more people are waiting than one sheet lists.
		out.Items = out.Items[:2000]
		out.Truncated = true
	}
	return out, nil
}
