package metadata

import (
	"context"
	"database/sql"
	"errors"

	"portico.local/server/internal/dbwork"
)

// QueueRefresh asks the screen worker to look at one movie again. The screen
// worker owns film matching and details; the retired legacy queues are not
// written.
var ErrRefreshKind = errors.New("direct movie metadata refresh is not available for this item kind")

func (s *Service) QueueRefresh(item string) error {
	gated, e := dbwork.Begin(context.Background(), s.db, dbwork.ClassBackgroundMedia)
	if e != nil {
		return e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	if e = s.QueueRefreshTx(context.Background(), tx, item); e != nil {
		return e
	}
	return gated.Commit()
}

// QueueRefreshTx participates in the durable job's progress transaction.
func (s *Service) QueueRefreshTx(ctx context.Context, tx *sql.Tx, item string) error {
	var e error
	entity, e := resolveEntity(ctx, tx, item)
	if e != nil {
		return e
	}
	var kind int
	if e = tx.QueryRow(`SELECT kind FROM catalog_entities WHERE id=?`, entity).Scan(&kind); e != nil {
		return e
	}
	if kind != 1 {
		return ErrRefreshKind
	}
	if _, e = tx.Exec(`UPDATE screen_metadata_work SET status='pending',generation=generation+1,revision=revision+1,requested_provider='',query_override='',lease='',lease_until='',attempts=0,next_attempt='',error='' WHERE target_kind='item' AND target_id=?`, entity); e != nil {
		return e
	}
	// A person asking for a refresh is the one thing the freshness rule must never stand in
	// the way of: the rule exists to stop this server asking on its own. Forgetting what this
	// item's accepted providers were last told makes the next pass go and look.
	if _, e = tx.Exec(`DELETE FROM metadata_document_freshness WHERE (provider,kind,resource) IN
  (SELECT provider,provider_type,provider_id FROM screen_metadata_publications WHERE target_kind='item' AND target_id=? AND decision='accepted')`, entity); e != nil {
		return e
	}
	return nil
}
