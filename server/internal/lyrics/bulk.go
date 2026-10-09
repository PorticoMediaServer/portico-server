package lyrics

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/localonly"
	"portico.local/server/internal/worker"

	"portico.local/server/internal/identity"
)

// Bulk is the durable library lyric acquisition run. It owns no scheduler of
// its own: the console operation admits it, this domain records progress, and
// a single worker quantum advances it. Every acquisition goes through the same
// provider client, document parser and timing validation as the per-item path.
type Bulk struct {
	Service *Service
	Now     func() time.Time
	// Batch bounds one worker quantum. Each song costs one provider request.
	Batch int
	// provider substitutes the acquisition client in tests. Production always
	// uses the configured LRCLIB client, with its own origin and dial policy.
	provider func(context.Context, string) ([]acquired, error)
}

func (b *Bulk) search(ctx context.Context, query string) ([]acquired, error) {
	if b.provider != nil {
		return b.provider(ctx, query)
	}
	if b.Service.Provider == nil {
		return nil, ErrUnavailable
	}
	return b.Service.Provider.Search(ctx, query)
}

type BulkRun struct {
	ID         string `json:"id"`
	LibraryID  string `json:"libraryId"`
	State      string `json:"state"`
	Phase      string `json:"phase"`
	ErrorCode  string `json:"errorCode"`
	Candidates int64  `json:"candidates"`
	Processed  int64  `json:"processed"`
	Published  int64  `json:"published"`
	Skipped    int64  `json:"skipped"`
	Missing    int64  `json:"missing"`
}

const bulkColumns = `id,library_id,state,phase,error_code,candidates,processed,published,skipped,missing`

func scanBulk(row interface{ Scan(...any) error }) (BulkRun, error) {
	var v BulkRun
	e := row.Scan(&v.ID, &v.LibraryID, &v.State, &v.Phase, &v.ErrorCode, &v.Candidates, &v.Processed, &v.Published, &v.Skipped, &v.Missing)
	return v, e
}
func (b *Bulk) now() int64 {
	if b.Now != nil {
		return b.Now().UnixMilli()
	}
	return time.Now().UnixMilli()
}
func (b *Bulk) batch() int {
	if b.Batch > 0 {
		return min(b.Batch, 64)
	}
	return 4
}

// missingLyricsQuery selects songs with no library-scope lyric. A viewer's own
// private lyric is never a reason to withhold a library acquisition, and a
// tombstoned resource counts as missing.
const missingLyricsQuery = `SELECT pid(e.public_id) FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE cl.library_id=? AND e.kind=7 AND (?='' OR e.public_id>pid_blob(?))
 AND NOT EXISTS(SELECT 1 FROM lyric_resources r WHERE r.item_id=e.id AND r.scope='library' AND r.deleted=0)
 ORDER BY e.public_id LIMIT ?`

// QueueTx admits one run inside the console operation's own transaction. The
// provider must be configured and enabled at admission; a disabled provider is
// refused rather than queued to fail later.
func (b *Bulk) QueueTx(ctx context.Context, tx *sql.Tx, operation, library string) (string, error) {
	if b == nil || b.Service == nil || operation == "" || library == "" {
		return "", ErrInput
	}
	settings, e := b.Service.settings(ctx, tx)
	if e != nil {
		return "", e
	}
	if !settings.LRCLIB {
		return "", ErrUnavailable
	}
	var known bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_libraries WHERE library_id=? AND retired=0)`, library).Scan(&known); e != nil {
		return "", e
	}
	if !known {
		return "", ErrInput
	}
	if local, err := localonly.Library(ctx, tx, library); err != nil || local {
		if err != nil {
			return "", err
		}
		return "", localonly.Err
	}
	var candidates int64
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM catalog_entities e JOIN catalog_libraries cl ON cl.id=e.library_id WHERE cl.library_id=? AND e.kind=7 AND NOT EXISTS(SELECT 1 FROM lyric_resources r WHERE r.item_id=e.id AND r.scope='library' AND r.deleted=0)`, library).Scan(&candidates); e != nil {
		return "", e
	}
	var actor string
	if e = tx.QueryRowContext(ctx, `SELECT actor FROM console_operations WHERE id=?`, operation).Scan(&actor); e != nil && !errors.Is(e, sql.ErrNoRows) {
		return "", e
	}
	now := b.now()
	_, e = tx.ExecContext(ctx, `INSERT INTO lyric_fetch_runs(id,library_id,actor,state,phase,provider_revision,candidates,created_ms,updated_ms) VALUES(?,?,?,'queued',?,?,?,?,?)
 ON CONFLICT(id) DO NOTHING`, operation, library, actor, fmt.Sprintf("%d songs without lyrics", candidates), settings.Revision, candidates, now, now)
	if e != nil {
		return "", e
	}
	return operation, nil
}
func (b *Bulk) Observe(ctx context.Context, id string) (BulkRun, error) {
	if b == nil || b.Service == nil {
		return BulkRun{}, ErrUnavailable
	}
	return scanBulk(b.Service.DB.QueryRowContext(ctx, `SELECT `+bulkColumns+` FROM lyric_fetch_runs WHERE id=?`, id))
}

// CancelTx fences the run inside the console command's transaction, so the
// acknowledgement cannot precede the fence.
func (b *Bulk) CancelTx(ctx context.Context, tx *sql.Tx, id string) error {
	if b == nil || b.Service == nil {
		return ErrUnavailable
	}
	_, e := tx.ExecContext(ctx, `UPDATE lyric_fetch_runs SET cancel_requested=1,updated_ms=? WHERE id=? AND state IN('queued','running')`, b.now(), id)
	return e
}
func (b *Bulk) Cancel(ctx context.Context, id string) error {
	if b == nil || b.Service == nil {
		return ErrUnavailable
	}
	_, e := dbwork.ExecWrite(ctx, b.Service.DB, dbwork.ClassBackgroundMedia, `UPDATE lyric_fetch_runs SET state='cancelled',phase='cancelled',cancel_requested=1,updated_ms=? WHERE id=? AND state IN('queued','running')`, b.now(), id)
	return e
}

// Run advances admitted work. One quantum handles a bounded batch of songs for
// a single run so provider pressure stays predictable.
func (b *Bulk) Run(ctx context.Context) {
	ctx = dbwork.WithClass(ctx, dbwork.ClassBackgroundMedia)
	wake := worker.NewSignal()
	unregister := dbwork.WakeOnTables(wake, "lyric_fetch_runs", "lyric_provider_settings", "lyric_resources", "catalog_entities", "catalog_asset_links", "console_documents", "maintenance_*")
	defer unregister()
	worker.Run(ctx, "lyrics.bulk", wake, func(ctx context.Context) time.Duration {
		for dbwork.Yield(ctx) {
			batchStarted := time.Now()
			more, e := b.Step(ctx)
			if e != nil {
				return 2 * time.Second
			}
			if !more || ctx.Err() != nil {
				return 0
			}
			if !dbwork.PaceBackground(ctx, batchStarted) {
				return 0
			}
		}
		return 0
	})
}

// Step advances at most one run by one batch. It reports whether more work of
// the same kind is immediately available.
func (b *Bulk) Step(ctx context.Context) (bool, error) {
	if b == nil || b.Service == nil {
		return false, nil
	}
	run, e := scanBulk(b.Service.DB.QueryRowContext(ctx, `SELECT `+bulkColumns+` FROM lyric_fetch_runs WHERE state IN('queued','running') ORDER BY created_ms,id LIMIT 1`))
	if errors.Is(e, sql.ErrNoRows) {
		return false, nil
	}
	if e != nil {
		return false, e
	}
	var cancelled bool
	var cursor string
	var providerRevision int64
	if e = b.Service.DB.QueryRowContext(ctx, `SELECT cancel_requested,cursor,provider_revision FROM lyric_fetch_runs WHERE id=?`, run.ID).Scan(&cancelled, &cursor, &providerRevision); e != nil {
		return false, e
	}
	if cancelled {
		return true, b.finish(ctx, run.ID, "cancelled", "cancelled", "")
	}
	settings, e := b.Service.Settings(ctx)
	if e != nil {
		return false, e
	}
	if !settings.LRCLIB || settings.Revision != providerRevision {
		return true, b.finish(ctx, run.ID, "failed", "provider disabled or reconfigured", "lyrics_provider_unavailable")
	}
	items, e := b.pending(ctx, run.LibraryID, cursor, b.batch())
	if e != nil {
		return false, e
	}
	if len(items) == 0 {
		return true, b.finish(ctx, run.ID, "succeeded", fmt.Sprintf("%d published, %d without a provider match, %d skipped", run.Published, run.Missing, run.Skipped), "")
	}
	if _, e = dbwork.ExecWrite(ctx, b.Service.DB, dbwork.ClassBackgroundMedia, `UPDATE lyric_fetch_runs SET state='running',phase=?,updated_ms=? WHERE id=? AND state IN('queued','running')`, fmt.Sprintf("fetching %d of %d", run.Processed+1, run.Candidates), b.now(), run.ID); e != nil {
		return false, e
	}
	for _, item := range items {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		outcome, e := b.fetchOne(ctx, run.ID, item, settings.Revision)
		if e != nil {
			return false, e
		}
		// The counter name is chosen from a closed set, never composed from data.
		var counter string
		switch outcome {
		case "published":
			counter = `published=published+1`
		case "missing":
			counter = `missing=missing+1`
		case "skipped":
			counter = `skipped=skipped+1`
		default:
			return false, ErrInput
		}
		if _, e = dbwork.ExecWrite(ctx, b.Service.DB, dbwork.ClassBackgroundMedia, `UPDATE lyric_fetch_runs SET cursor=?,processed=processed+1,`+counter+`,updated_ms=? WHERE id=?`, item, b.now(), run.ID); e != nil {
			return false, e
		}
	}
	return true, nil
}
func (b *Bulk) finish(ctx context.Context, id, state, phase, code string) error {
	_, e := dbwork.ExecWrite(ctx, b.Service.DB, dbwork.ClassBackgroundMedia, `UPDATE lyric_fetch_runs SET state=?,phase=?,error_code=?,updated_ms=? WHERE id=? AND state IN('queued','running')`, state, phase, code, b.now(), id)
	return e
}
func (b *Bulk) pending(ctx context.Context, library, cursor string, limit int) ([]string, error) {
	rows, e := b.Service.DB.QueryContext(ctx, missingLyricsQuery, library, cursor, cursor, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// fetchOne acquires one song through the provider client and publishes the
// first candidate whose timing fits the selected source. It returns the run
// counter to advance: published, missing or skipped.
func (b *Bulk) fetchOne(ctx context.Context, run, item string, providerRevision int64) (string, error) {
	gated, e := dbwork.BeginSnapshot(ctx, b.Service.DB)
	if e != nil {
		return "", e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	var entity int64
	var kind int64
	if e = tx.QueryRowContext(ctx, `SELECT id,kind FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&entity, &kind); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return "skipped", nil
		}
		return "", e
	}
	if kind != 7 {
		return "skipped", nil
	}
	if local, err := localonly.Item(ctx, tx, item); err != nil || local {
		if err != nil {
			return "", err
		}
		return "skipped", nil
	}
	all, e := sources(ctx, tx, Access{ItemID: item}, entity)
	if e != nil || len(all) == 0 {
		gated.Rollback()
		if e != nil && !errors.Is(e, ErrSource) && !errors.Is(e, ErrCapacity) {
			return "", e
		}
		return "skipped", nil
	}
	src := all[0]
	var query string
	if e = tx.QueryRowContext(ctx, `SELECT i.title||' '||COALESCE((SELECT ar.title FROM catalog_song_artists sa JOIN catalog_entities ar ON ar.id=sa.artist_id WHERE sa.song_id=i.id ORDER BY ar.id LIMIT 1),'') FROM catalog_entities i WHERE i.public_id=pid_blob(?)`, item).Scan(&query); e != nil {
		return "", e
	}
	gated.Rollback()
	query = strings.TrimSpace(query)
	if query == "" {
		return "skipped", nil
	}
	found, e := b.search(ctx, query)
	if e != nil {
		// A provider outage is not a publication failure for this song.
		return "missing", nil
	}
	var chosen *acquired
	for i := range found {
		if validateTiming(found[i].doc, found[i].doc.EmbeddedOffsetMS, src.SourceDuration) != nil {
			continue
		}
		if chosen == nil || chosen.doc.Format != "lrc" && found[i].doc.Format == "lrc" {
			chosen = &found[i]
		}
		if chosen.doc.Format == "lrc" {
			break
		}
	}
	if chosen == nil {
		return "missing", nil
	}
	return b.publish(ctx, run, item, src, *chosen, providerRevision)
}
func (b *Bulk) publish(ctx context.Context, run, item string, src Source, v acquired, providerRevision int64) (string, error) {
	gated2, e := dbwork.Begin(ctx, b.Service.DB, dbwork.ClassBackgroundMedia)
	if e != nil {
		return "", e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	// The provider switch and the source binding are both rechecked after the
	// external call. Neither the run nor the candidate may outlive them.
	settings, e := b.Service.settings(ctx, tx)
	if e != nil {
		return "", e
	}
	if !settings.LRCLIB || settings.Revision != providerRevision {
		return "skipped", nil
	}
	entity, e := entityid.Resolve(ctx, tx, item)
	if e != nil {
		return "skipped", nil
	}
	all, e := sources(ctx, tx, Access{ItemID: item}, entity)
	if e != nil || len(all) == 0 || all[0].ID != src.ID || all[0].Version != src.Version {
		if e != nil && !errors.Is(e, ErrSource) && !errors.Is(e, ErrCapacity) {
			return "", e
		}
		return "skipped", nil
	}
	var present bool
	if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM lyric_resources WHERE item_id=? AND scope='library' AND deleted=0)`, entity).Scan(&present); e != nil {
		return "", e
	}
	if present {
		return "skipped", nil
	}
	var count int
	if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM lyric_resources WHERE item_id=? AND scope='library'`, entity).Scan(&count); e != nil {
		return "", e
	}
	if count >= 16 {
		return "skipped", nil
	}
	var actor string
	if e = tx.QueryRowContext(ctx, `SELECT actor FROM lyric_fetch_runs WHERE id=?`, run).Scan(&actor); e != nil {
		return "", e
	}
	offset := v.doc.EmbeddedOffsetMS
	if e = validateTiming(v.doc, offset, src.SourceDuration); e != nil {
		return "skipped", nil
	}
	id := identity.Token()
	if _, e = tx.ExecContext(ctx, `INSERT INTO lyric_resources(id,item_id,asset_id,source_version,scope,authority,account_id,profile_id,revision) VALUES(?,?,?,?,'library','local','','',1)`, id, entity, src.ID, src.Version); e != nil {
		return "", e
	}
	raw, _ := json.Marshal(v.doc)
	prov, _ := json.Marshal(v.provenance)
	if _, e = tx.ExecContext(ctx, `INSERT INTO lyric_revisions(resource_id,revision,document,digest,language,offset_ms,provenance,actor,created_at,deleted) VALUES(?,1,?,?,?,?,?,?,?,0)`, id, string(raw), digest(v.doc), v.language, offset, string(prov), actor, time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
		return "", e
	}
	if e = gated2.Commit(); e != nil {
		return "", e
	}
	return "published", nil
}
