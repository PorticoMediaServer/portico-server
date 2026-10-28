package playbackv1

import (
 "context"
 "database/sql"
 "errors"
 "sync"
 "time"

 "portico.local/server/internal/dbwork"
 "portico.local/server/internal/usermutation"
)

// A completion retains only exact process handles/IDs until their committed
// fences finish. It contains no request or response, and a restart has no old
// processes to join. The durable grant fences and receipt are already atomic.
type stopCompletion struct {
 mu sync.Mutex
 finish func(context.Context) error
 done bool
}
func (s *Service) finishStop(ctx context.Context, key string, finish func(context.Context) error) error {
 if key == "" { return finish(ctx) }
 value, _ := s.stopCompletions.LoadOrStore(key, &stopCompletion{finish: finish})
 c := value.(*stopCompletion)
 c.mu.Lock()
 defer c.mu.Unlock()
 if c.done { return nil }
 cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
 defer cancel()
 if err := c.finish(cleanup); err != nil { return err }
 c.done = true
 s.stopCompletions.CompareAndDelete(key, c)
 return nil
}
func stopKey(ctx context.Context) string {
 _, e, ok := usermutation.RequestFrom(ctx)
 if ok { return e.Key }
 return ""
}
func (s *Service) replayStop(ctx context.Context, receipt *usermutation.Receipt) error {
 if value, ok := s.stopCompletions.Load(receipt.Key); ok {
  return s.finishStop(ctx, receipt.Key, value.(*stopCompletion).finish)
 }
 // All database fences committed with this receipt. On restart no process
 // handles survive; never rediscover media by a now mutable session ID.
 return nil
}
func checkStopReceipt(ctx context.Context, tx *sql.Tx, now int64) (*usermutation.Receipt, error) {
 if _, e, ok := usermutation.RequestFrom(ctx); ok && !e.Stop { return nil, usermutation.ErrConflict }
 return usermutation.CheckContext(ctx, tx, now)
}

func (s *Service) stopWithReceipt(ctx context.Context, c Caller, id string, position *int64, generation *int) error {
 if generation != nil && (*generation < 1 || int64(*generation) > 2147483647) { return &FieldError{Path:"generation"} }
 key := stopKey(ctx)
 // Preserve unkeyed ended/refused calls' no-writer behavior. Keyed requests
 // prove current authority and consult the receipt before any mutable target.
 if key == "" {
  r, err := s.session(ctx, c, id)
  if err != nil { return err }
  if generation != nil && r.generation != *generation { return ErrRevision }
  if r.ended != 0 { return nil }
 }
 var replay *usermutation.Receipt
 var finish func(context.Context) error
 err := dbwork.WithWriteTxContext(ctx, s.DB, dbwork.ClassEstablishedPlayback, func(ctx context.Context, tx *sql.Tx) error {
  var err error
  if replay, err = checkStopReceipt(ctx, tx, s.now().UnixMilli()); err != nil || replay != nil { return err }
  r, err := loadRow(ctx, tx, id)
  if err != nil { return err }
  if err = c.authorizeSession(ctx, r); err != nil { return err }
  if generation != nil && r.generation != *generation { return ErrRevision }
  result := usermutation.Result{Status:"already_stopped", TargetID:id, Revision:int64(r.revision), Generation:r.generation, PositionMs:r.position, PositionStatus:"already_stopped"}
  if r.ended == 0 {
   if err = s.drainObservationTx(ctx, tx, id); err != nil { return err }
   if r, err = loadRow(ctx, tx, id); err != nil { return err }
   disposition, err := s.stopPositionTx(ctx, tx, c, r, position)
   if err != nil { return err }
   if r, err = loadRow(ctx, tx, id); err != nil { return err }
   now := s.now()
   if err = s.endTxRaw(ctx, tx, r, now, "stopped", ""); err != nil { return err }
   // Cancel and fence prepared children in the same transaction. Postcommit
   // cleanup may retry only these captured presentations.
   media, err := cancelPreparedTx(ctx, tx, "session_id", id, "stopped")
   if err != nil { return err }
   if r.media != "" { media = append(media, r.media) }
   for _, m := range media {
    if _, err = tx.ExecContext(ctx, `UPDATE playback_sessions SET state='stopped' WHERE id=?`, m); err != nil { return err }
   }
   finish = func(ctx context.Context) error {
    for _, m := range media { if err := s.Playback.StopV1(ctx, m); err != nil { return err } }
    return nil
   }
   if key != "" {
    if err = dbwork.AfterCommit(tx, func() { s.stopCompletions.LoadOrStore(key, &stopCompletion{finish:finish}) }); err != nil { return err }
   }
   result.Status, result.Revision, result.PositionMs, result.PositionStatus = "applied", int64(r.revision+1), r.position, disposition
  }
  return usermutation.SaveContext(ctx, tx, result, s.now().UnixMilli())
 })
 if err != nil { return err }
 if replay != nil { return s.replayStop(ctx, replay) }
 if finish == nil { return nil }
 s.hub.wake()
 s.wakeChannels()
 s.sweepQueues()
 return s.finishStop(ctx, key, finish)
}

func (s *Service) stopPositionTx(ctx context.Context, tx *sql.Tx, c Caller, r row, position *int64) (string, error) {
 if position == nil || *position < 0 { return "unchanged", nil }
 if c.DeviceID != r.device { return "not_renderer", nil }
 if r.media == "" { return "unchanged", nil }
 if _, err := tx.ExecContext(ctx, `SAVEPOINT stop_position`); err != nil { return "", err }
 seq := r.lastSeq + 1
 err := s.Playback.ObservationEvidenceTx(ctx, tx, c.Principal, r.media, r.mediaGeneration, seq, float64(*position)/1000, "paused", 0, nil, false, s.now(), nil)
 // The legacy evidence port intentionally no-ops stale generations. Prove it
 // accepted this exact sequence before acknowledging the final position.
 if err == nil {
  var accepted bool
  err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM playback_sessions WHERE id=? AND generation=? AND sequence=? AND state='paused')`, r.media, r.mediaGeneration, seq).Scan(&accepted)
  if err == nil && !accepted { err = errStopEvidenceRefused }
 }
 if err == nil {
  _, err = tx.ExecContext(ctx, `UPDATE playback_v1_sessions SET position_ms=?,last_seq=? WHERE id=? AND ended_ms=0`, *position, seq, r.id)
 }
 if err != nil {
  if !refusedEvidence(err) && !errors.Is(err, errStopEvidenceRefused) { return "", err }
  if _, rollback := tx.ExecContext(ctx, `ROLLBACK TO stop_position`); rollback != nil { return "", rollback }
  if _, release := tx.ExecContext(ctx, `RELEASE stop_position`); release != nil { return "", release }
  return "policy_rejected", nil
 }
 _, err = tx.ExecContext(ctx, `RELEASE stop_position`)
 return "applied", err
}
var errStopEvidenceRefused = errors.New("final position evidence was not accepted")
