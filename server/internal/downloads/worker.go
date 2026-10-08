package downloads

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"portico.local/server/internal/contentaccess"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/entityid"
	"portico.local/server/internal/mediasource"
	"portico.local/server/internal/supervise"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/preparedmedia"
	"portico.local/server/internal/worker"
)

// Start runs the preparation worker until Close. Everything it does is also
// reachable by calling Advance directly, which is how the tests drive it: the
// loop adds scheduling, never behaviour.
func (s *Service) Start() {
	if s.done != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel, s.done = cancel, make(chan struct{})
	shared := worker.NewSignal()
	dbwork.WakeOnCommit(shared)
	supervise.Go("downloads.worker", func() {
		defer close(s.done)
		// A preparation exists because somebody asked for one, so the loop is
		// driven by that request and by any commit, not by a five-second tick
		// that asked an empty queue the same question all night.
		worker.RunWith(ctx, "downloads.worker", s.wake, shared, func(ctx context.Context) time.Duration {
			// A failing pass is not fatal: the next wake retries, and every
			// preparation carries its own terminal state if it truly cannot run.
			_ = s.Advance(ctx)
			return 0
		})
	})
}

func (s *Service) Close() error {
	if s.cancel != nil {
		s.cancel()
		<-s.done
		s.cancel, s.done = nil, nil
	}
	return nil
}

// Advance runs one bounded pass: retention first, then a page of live work.
// It is safe to call concurrently with itself; every write is fenced on the
// revision the pass read.
func (s *Service) Advance(ctx context.Context) error {
	s.advanceMu.Lock()
	defer s.advanceMu.Unlock()
	if e := s.advanceRequests(ctx); e != nil {
		return e
	}
	if e := s.expire(ctx); e != nil {
		return e
	}
	if e := s.collectSnapshots(ctx); e != nil {
		return e
	}
	rows, e := s.db.QueryContext(ctx, `SELECT id FROM download_preparations WHERE state='running' OR (state='queued' AND (SELECT count(*) FROM download_preparations active WHERE active.profile_key=download_preparations.profile_key AND active.state='running')<?) ORDER BY updated_ms,id LIMIT 8`, MaxLivePerViewer)
	if e != nil {
		return e
	}
	ids := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		ids = append(ids, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	moved := false
	for _, id := range ids {
		progressed, err := s.step(ctx, id)
		if errors.Is(err, compactcatalog.ErrBuilding) {
			// This item's catalogue change isn't published yet: its
			// preparation waits for a later pass, and the others still run.
			continue
		}
		if err != nil {
			return err
		}
		moved = moved || progressed
	}
	// A large original is hashed one bounded window per pass. Waking the loop
	// immediately after a window that moved bytes keeps a long file running at
	// disk speed instead of one window every tick, while a pass that moved
	// nothing still waits, so watching a conversion someone else is running
	// costs one poll per tick rather than a spin.
	if moved {
		s.signal()
	}
	return nil
}

// expire retires unused ready claims and sweeps spent grants. A claim the
// viewer never transferred expires retentionDays after it became ready; one
// they did transfer expires retentionDays after the last transfer, so an actively
// used download is not deleted out from under a client that keeps refreshing it.
func (s *Service) expire(ctx context.Context) error {
	gated, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return e
	}
	tx := gated.Tx()
	defer gated.Rollback()
	settings, e := readSettings(tx)
	if e != nil {
		return e
	}
	now := s.millis()
	deadline := now - int64(settings.RetentionDays)*24*60*60*1000
	rows, e := tx.QueryContext(ctx, `SELECT id FROM download_preparations WHERE state='ready' AND (CASE WHEN used_ms>0 THEN used_ms ELSE ready_ms END)<? ORDER BY updated_ms LIMIT 64`, deadline)
	if e != nil {
		return e
	}
	expired := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		expired = append(expired, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range expired {
		if e = revokeForPreparation(tx, id, ReasonRetention, now); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='expired',reason=?,hash_state=NULL,revision=revision+1,updated_ms=? WHERE id=? AND state='ready'`, ReasonRetention, now, id); e != nil {
			return e
		}
	}
	// A ready claim whose item left the library is no longer a download anybody
	// can use, and leaving it committed would hold storage against the owner's
	// ceiling forever. Retiring it here is also what stops its receipts.
	rows, e = tx.QueryContext(ctx, `SELECT id FROM download_preparations WHERE state='ready' AND NOT EXISTS(SELECT 1 FROM catalog_entities WHERE id=download_preparations.item_id) ORDER BY updated_ms LIMIT 64`)
	if e != nil {
		return e
	}
	orphaned := []string{}
	for rows.Next() {
		var id string
		if e = rows.Scan(&id); e != nil {
			rows.Close()
			return e
		}
		orphaned = append(orphaned, id)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	for _, id := range orphaned {
		if e = revokeForPreparation(tx, id, ReasonItemDeleted, now); e != nil {
			return e
		}
		if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='unavailable',reason=?,hash_state=NULL,revision=revision+1,updated_ms=? WHERE id=? AND state='ready'`, ReasonItemDeleted, now, id); e != nil {
			return e
		}
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM download_grants WHERE expires_ms<?`, now-int64(GrantReplayWindow/time.Millisecond)); e != nil {
		return e
	}
	return gated.Commit()
}

// plan is what one pass decided to do outside the database transaction.
type optimization struct {
	p                               identity.Principal
	item, asset, profile, operation string
}
type plan struct {
	optimization     *optimization
	hashBytes        bool
	path             string
	size, modifiedNS int64
	offset           int64
	state            []byte
	revision         int64
	snapshotVersion  mediasource.Version
}

func (s *Service) step(ctx context.Context, id string) (bool, error) {
	p, e := s.prepare(ctx, id)
	if e == nil && p.optimization != nil {
		o := p.optimization
		if e = s.optimizer.RequestOptimization(ctx, o.p, o.item, o.asset, o.profile, o.operation); e != nil {
			if ctx.Err() != nil || dbwork.Retryable(e) {
				return false, e
			}
			return false, s.fail(ctx, id, p.revision, ReasonOptimizeFailed)
		}
		_, e = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `UPDATE download_preparation_authority SET optimization_requested=1 WHERE preparation_id=?`, id)
		return true, e
	}
	if e != nil || !p.hashBytes {
		return false, e
	}
	if p.snapshotVersion.ID() == "" {
		p, e = s.snapshotOriginal(ctx, id, p)
		if e != nil {
			if ctx.Err() != nil {
				return false, ctx.Err()
			}
			reason := ReasonSourceChanged
			if errors.Is(e, ErrStorageFull) {
				reason = ReasonStorageFull
			}
			return false, s.fail(ctx, id, p.revision, reason)
		}
		if !p.hashBytes {
			return false, nil
		}
	}
	digestState, read, e := s.hashChunk(ctx, p)
	if e != nil {
		return false, s.fail(ctx, id, p.revision, ReasonSourceMissing)
	}
	if e = s.record(ctx, id, p, digestState, read); e != nil {
		return false, e
	}
	return read > 0 && p.offset+read < p.size, nil
}

// prepare decides, inside one transaction, what a preparation needs next. It
// finalizes everything it can decide from the database alone, and only hands
// back work when bytes have to be read.
func (s *Service) prepare(ctx context.Context, id string) (plan, error) {
	var out plan
	gated2, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return out, e
	}
	tx := gated2.Tx()
	defer gated2.Rollback()
	now := s.millis()
	var state, quality, item, authority, account, profile, artifactRef, snapshotJSON string
	var bytesDone, bytesTotal, revision int64
	var hashState []byte
	e = tx.QueryRowContext(ctx, `SELECT state,quality,COALESCE((SELECT pid(public_id) FROM catalog_entities WHERE id=download_preparations.item_id),''),authority,account_id,profile_id,artifact_ref,bytes_done,bytes_total,revision,hash_state,source_version_json FROM download_preparations WHERE id=?`, id).Scan(&state, &quality, &item, &authority, &account, &profile, &artifactRef, &bytesDone, &bytesTotal, &revision, &hashState, &snapshotJSON)
	if errors.Is(e, sql.ErrNoRows) {
		return out, nil
	}
	if e != nil {
		return out, e
	}
	if state != StateQueued && state != StateRunning {
		return out, nil
	}
	if state == StateQueued {
		var running int
		if e = tx.QueryRowContext(ctx, `SELECT count(*) FROM download_preparations WHERE profile_key=(SELECT profile_key FROM download_preparations WHERE id=?) AND state='running'`, id).Scan(&running); e != nil {
			return out, e
		}
		if running >= MaxLivePerViewer {
			return out, nil
		}
	}
	viewer := identity.Viewer{AccountID: account, ProfileID: profile, Authority: authority}
	allowed, e := ProfileAllowsDownloads(tx, viewer)
	if e != nil {
		return out, e
	}
	if !allowed {
		return out, terminal(ctx, gated2, id, StateUnavailable, ReasonNotAllowed, now)
	}
	var stored, device, optimizationOperation string
	var requested bool
	e = tx.QueryRowContext(ctx, `SELECT principal,device_id,optimization_requested,optimization_operation FROM download_preparation_authority WHERE preparation_id=?`, id).Scan(&stored, &device, &requested, &optimizationOperation)
	if e != nil && !errors.Is(e, sql.ErrNoRows) {
		return out, e
	}
	principal := identity.Principal{Viewer: viewer}
	if stored != "" {
		if e = json.Unmarshal([]byte(stored), &principal); e != nil {
			return out, e
		}
	}
	if device != "" {
		visible, args, err := s.requestAccess(ctx, tx, principal, device)
		if errors.Is(err, errRequestUnavailable) {
			return out, err
		}
		if err != nil {
			return out, terminal(ctx, gated2, id, StateUnavailable, ReasonNotAllowed, now)
		}
		var visibleNow bool
		if e = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM catalog_entities i WHERE i.public_id=pid_blob(?) AND (`+visible+`))`, append([]any{item}, args...)...).Scan(&visibleNow); e != nil {
			return out, e
		}
		if !visibleNow {
			// Facts are synchronous, so a present-but-invisible item is a
			// refusal — except while the derived data behind the visibility
			// check rebuilds, which answers retry, never the end of the
			// download.
			if e = compactcatalog.CheckReadiness(ctx, tx, compactcatalog.DomainBrowseRows, compactcatalog.DomainBrowseEdges, compactcatalog.DomainAvailability); e != nil {
				return out, e
			}
			return out, terminal(ctx, gated2, id, StateUnavailable, ReasonItemDeleted, now)
		}
	} else if err := contentaccess.VisibleKnownItemTx(ctx, tx, principal, item); err != nil {
		if !errors.Is(err, identity.ErrContentRestricted) {
			return out, err
		}
		return out, terminal(ctx, gated2, id, StateUnavailable, ReasonItemDeleted, now)
	}
	// Checkpoints refer exclusively to a published immutable snapshot. A later
	// rescan or different selected asset cannot change the bytes being hashed.
	if quality == QualityOriginal && snapshotJSON != "" {
		version, err := sourceSnapshotVersion(snapshotJSON, id, bytesTotal)
		if err != nil {
			return out, terminal(ctx, gated2, id, StateUnavailable, ReasonSourceChanged, now)
		}
		if state == StateQueued {
			if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='running',revision=revision+1,updated_ms=? WHERE id=?`, now, id); e != nil {
				return out, e
			}
			revision++
		}
		out = plan{hashBytes: true, path: filepath.Join(s.snapshotRoot, id+".source"), size: bytesTotal, offset: bytesDone, state: hashState, revision: revision, snapshotVersion: version}
		return out, gated2.Commit()
	}
	found, err := resolveSource(ctx, tx, item)
	if reason := sourceReason(found, err); reason != "" {
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return out, err
		}
		return out, terminal(ctx, gated2, id, StateUnavailable, reason, now)
	}
	if quality != QualityOriginal {
		if s.optimizer != nil && principal.Role == "owner" && stored != "" && !requested {
			rung, ok := ladderRung(quality)
			if !ok {
				return out, ErrInput
			}
			if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='running',revision=revision+1,updated_ms=? WHERE id=?`, now, id); e != nil {
				return out, e
			}
			if optimizationOperation == "" {
				optimizationOperation = identity.Token()
				if _, e = tx.ExecContext(ctx, `UPDATE download_preparation_authority SET optimization_operation=? WHERE preparation_id=?`, optimizationOperation, id); e != nil {
					return out, e
				}
			}
			out.optimization = &optimization{principal, item, found.Asset, preparedProfile(found.Kind, rung), optimizationOperation}
			out.revision = revision + 1
			return out, gated2.Commit()
		}
		return out, s.advanceOptimized(ctx, gated2, id, quality, found, now)
	}
	// Capture this original into one immutable snapshot before checkpointing.
	// Legacy mutable-source checkpoints are discarded, even at identical size.
	if snapshotJSON == "" || artifactRef != found.Asset || bytesTotal != found.Size {
		if found.Size > bytesTotal {
			if e := storageRoom(tx, found.Size-bytesTotal); e != nil {
				return out, e
			}
		}
		bytesDone, hashState = 0, nil
		if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET artifact_kind='source',artifact_ref=?,artifact_container=?,bytes_total=?,bytes_done=0,estimated=0,hash_state=NULL,state='running',started_ms=?,revision=revision+1,updated_ms=? WHERE id=?`, found.Asset, found.Container, found.Size, now, now, id); e != nil {
			return out, e
		}
		revision++
	} else if state == StateQueued {
		if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='running',started_ms=CASE WHEN started_ms=0 THEN ? ELSE started_ms END,revision=revision+1,updated_ms=? WHERE id=?`, now, now, id); e != nil {
			return out, e
		}
		revision++
	}
	out = plan{hashBytes: true, path: found.Path, size: found.Size, modifiedNS: found.ModifiedNS, offset: bytesDone, state: hashState, revision: revision}
	return out, gated2.Commit()
}

// advanceOptimized resolves a ladder rung against prepared media. The bytes are
// produced by internal/preparedmedia; downloads only observes them.
func (s *Service) advanceOptimized(ctx context.Context, gatedArg *dbwork.Write, id, quality string, found source, now int64) error {
	tx := gatedArg.Tx()
	rung, ok := ladderRung(quality)
	if !ok {
		return terminal(ctx, gatedArg, id, StateFailed, ReasonFailed, now)
	}
	profileID := preparedProfile(found.Kind, rung)
	versions, e := preparedmedia.VersionsTx(ctx, tx, found.Item)
	if e != nil {
		return e
	}
	for _, v := range versions {
		if v.ProfileID != profileID || !v.Selectable || !validDigest.MatchString(v.Digest) {
			continue
		}
		if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='ready',reason='',artifact_kind='prepared',artifact_ref=?,artifact_digest=?,artifact_container=?,bytes_total=?,bytes_done=?,estimated=0,ready_ms=?,revision=revision+1,updated_ms=? WHERE id=?`,
			v.ID, v.Digest, v.Facts.Container, v.Size, v.Size, now, now, id); e != nil {
			return e
		}
		if e = s.announce(ctx, tx, id, now, true, ""); e != nil {
			return e
		}
		return gatedArg.Commit()
	}
	// No published version yet. An optimization already in flight is progress;
	// a failed one is a failure; nothing at all means no owner has prepared this
	// rung, and a viewer cannot order a conversion for themselves.
	var state string
	var bytes int64
	itemEntity, e := entityid.Resolve(ctx, tx, found.Item)
	if e != nil {
		return terminal(ctx, gatedArg, id, StateUnavailable, ReasonItemDeleted, now)
	}
	e = tx.QueryRowContext(ctx, `SELECT state,bytes FROM prepared_media_jobs WHERE item_id=? AND profile_id=? ORDER BY created_ms DESC LIMIT 1`, itemEntity, profileID).Scan(&state, &bytes)
	if errors.Is(e, sql.ErrNoRows) {
		return terminal(ctx, gatedArg, id, StateUnavailable, ReasonNotOptimized, now)
	}
	if e != nil {
		return e
	}
	switch state {
	case "queued", "running", "cancelling":
		if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='running',reason='',bytes_done=?,started_ms=CASE WHEN started_ms=0 THEN ? ELSE started_ms END,revision=revision+1,updated_ms=? WHERE id=?`, bytes, now, now, id); e != nil {
			return e
		}
		return gatedArg.Commit()
	case "failed":
		return terminal(ctx, gatedArg, id, StateFailed, ReasonOptimizeFailed, now)
	default:
		return terminal(ctx, gatedArg, id, StateUnavailable, ReasonNotOptimized, now)
	}
}

// terminal writes a final state and commits. It also withdraws any offline
// authorization the claim had, because a preparation that can no longer be
// transferred must not keep vouching for bytes offline.
func terminal(ctx context.Context, gatedArg *dbwork.Write, id, state, reason string, now int64) error {
	tx := gatedArg.Tx()
	if state != StateReady {
		if e := revokeForPreparation(tx, id, reason, now); e != nil {
			return e
		}
	}
	if _, e := tx.ExecContext(ctx, `UPDATE download_preparations SET state=?,reason=?,hash_state=NULL,revision=revision+1,updated_ms=? WHERE id=?`, state, reason, now, id); e != nil {
		return e
	}
	return gatedArg.Commit()
}

// announce calls the notifications seam inside the same transaction that made
// the state true, so a notification can never describe a state that rolled back.
func (s *Service) announce(ctx context.Context, tx *sql.Tx, id string, now int64, ready bool, reason string) error {
	var authority, account, profile, item string
	if e := tx.QueryRowContext(ctx, `SELECT authority,account_id,profile_id,COALESCE((SELECT pid(public_id) FROM catalog_entities WHERE id=download_preparations.item_id),'') FROM download_preparations WHERE id=?`, id).Scan(&authority, &account, &profile, &item); e != nil {
		return e
	}
	viewer := identity.Viewer{AccountID: account, ProfileID: profile, Authority: authority}
	if ready {
		return s.notifier.DownloadReady(tx, now, viewer, id, item)
	}
	return s.notifier.DownloadFailed(tx, now, viewer, id, item, reason)
}

// hashChunk reads one bounded window of the source and folds it into the
// running SHA-256. The hash state is checkpointed by the caller, so a pause, a
// restart or a crash costs one window rather than the whole file.
func (s *Service) hashChunk(ctx context.Context, p plan) ([]byte, int64, error) {
	digest := sha256.New()
	if len(p.state) > 0 {
		if e := digest.(encoding.BinaryUnmarshaler).UnmarshalBinary(p.state); e != nil {
			return nil, 0, e
		}
	}
	f, e := s.openSnapshot(p.snapshotVersion)
	if e != nil {
		return nil, 0, e
	}
	defer f.Close()
	if p.offset > 0 {
		if _, e = f.Seek(p.offset, io.SeekStart); e != nil {
			return nil, 0, e
		}
	}
	remaining := p.size - p.offset
	if remaining > hashChunk {
		remaining = hashChunk
	}
	if remaining < 0 {
		return nil, 0, ErrInput
	}
	read, e := io.CopyN(digest, f, remaining)
	if e != nil {
		return nil, read, e
	}
	state, e := digest.(encoding.BinaryMarshaler).MarshalBinary()
	return state, read, e
}

// record folds one window's work back into the row, refusing to write if the
// viewer paused, cancelled or retried while the bytes were being read.
func (s *Service) record(ctx context.Context, id string, p plan, state []byte, read int64) error {
	gated3, e := dbwork.Begin(ctx, s.db, dbwork.ClassForegroundTransfer)
	if e != nil {
		return e
	}
	tx := gated3.Tx()
	defer gated3.Rollback()
	now := s.millis()
	var revision, total int64
	var current string
	if e = tx.QueryRowContext(ctx, `SELECT revision,bytes_total,state FROM download_preparations WHERE id=?`, id).Scan(&revision, &total, &current); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		return e
	}
	if revision != p.revision || current != StateRunning {
		return nil
	}
	done := p.offset + read
	if done < total {
		_, e = tx.ExecContext(ctx, `UPDATE download_preparations SET bytes_done=?,hash_state=?,revision=revision+1,updated_ms=? WHERE id=? AND revision=?`, done, state, now, id, revision)
		if e != nil {
			return e
		}
		return gated3.Commit()
	}
	digest, e := finishHash(state)
	if e != nil {
		return e
	}
	if digest != p.snapshotVersion.Evidence().Revision {
		return terminal(ctx, gated3, id, StateUnavailable, ReasonSourceChanged, now)
	}
	if _, e = tx.ExecContext(ctx, `UPDATE download_preparations SET state='ready',reason='',artifact_digest=?,bytes_done=?,hash_state=NULL,ready_ms=?,revision=revision+1,updated_ms=? WHERE id=? AND revision=?`, digest, done, now, now, id, revision); e != nil {
		return e
	}
	if e = s.announce(ctx, tx, id, now, true, ""); e != nil {
		return e
	}
	return gated3.Commit()
}

// fail records a terminal failure discovered outside a transaction.
func (s *Service) fail(ctx context.Context, id string, revision int64, reason string) error {
	gated4, e := dbwork.Begin(ctx, s.db, dbwork.ClassBackgroundMedia)
	if e != nil {
		return e
	}
	tx := gated4.Tx()
	defer gated4.Rollback()
	now := s.millis()
	var current int64
	if e = tx.QueryRowContext(ctx, `SELECT revision FROM download_preparations WHERE id=?`, id).Scan(&current); e != nil {
		if errors.Is(e, sql.ErrNoRows) {
			return nil
		}
		return e
	}
	if current != revision {
		return nil
	}
	if e = s.announce(ctx, tx, id, now, false, reason); e != nil {
		return e
	}
	return terminal(ctx, gated4, id, StateFailed, reason, now)
}

// directSource is the fallback reader used when no isolated media process is
// configured. The path comes from the server's own asset table, never from a
// request, and it is opened read-only.
func directSource(_ context.Context, name string, _, _ int64) (io.ReadSeekCloser, error) {
	return os.Open(name)
}

// finishHash reads back the checkpointed state and renders the digest in the
// lowercase hex the rest of the server publishes for content digests.
func finishHash(state []byte) (string, error) {
	digest := sha256.New()
	if e := digest.(encoding.BinaryUnmarshaler).UnmarshalBinary(state); e != nil {
		return "", e
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}
