package playback

import (
	"context"
	"time"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
)

// A library scanned before stream detail existed has codec names and nothing
// else: no bit depth, no profile, no Dolby Vision configuration. Planning against
// that would either refuse direct play it could have allowed or allow direct play
// that then fails on the device. Rather than ask the owner to rescan, the first
// play of such a title refreshes it: one bounded probe, outside any transaction,
// stored for every later play.

// detailRefreshTimeout bounds the probe. A title that cannot be probed in time
// plays anyway, planned on what is known.
const detailRefreshTimeout = 8 * time.Second

// ConfigureProbe wires the ffprobe used to refresh stream detail at first play.
// Without it, planning simply uses whatever the scanner recorded.
func (s *Service) ConfigureProbe(p assets.Probe) {
	s.probe = &p
	s.probeSlots = make(chan struct{}, 2)
}

// refreshStreamDetail brings the selected source up to the current detail
// version. Every failure is silent: this is an improvement to a decision, never a
// precondition for playing.
func (s *Service) refreshStreamDetail(ctx context.Context, selectedAsset string) {
	if s.probe == nil {
		return
	}
	var asset, path, container string
	var size, modified int64
	var version int
	err := dbwork.QueryRow(ctx, s.db, `SELECT a.token,a.path,a.container,a.size,a.modified_ns,COALESCE((SELECT f.detail_version FROM asset_stream_facts f WHERE f.asset_id=a.token AND f.size=a.size AND f.modified_ns=a.modified_ns),0) FROM catalog_assets a WHERE a.token=? AND a.available=1`, selectedAsset).Scan(&asset, &path, &container, &size, &modified, &version)
	if err != nil || version >= assets.StreamDetailVersion || container == "strm" {
		return
	}
	if s.StorageGuard != nil && s.StorageGuard(path) != nil {
		return
	}
	select {
	case s.probeSlots <- struct{}{}:
		defer func() { <-s.probeSlots }()
	default:
		// Two refreshes are already running. This play proceeds on known facts and
		// a later one refreshes the title.
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, detailRefreshTimeout)
	defer cancel()
	facts, err := s.probe.InspectScan(probeCtx, path)
	if err != nil || facts.Streams == nil {
		return
	}
	gated, err := dbwork.Begin(ctx, s.db, dbwork.ClassEstablishedPlayback)
	if err != nil {
		return
	}
	defer gated.Rollback()
	tx := gated.Tx()
	// Only for the exact file that was probed: a file replaced meanwhile belongs
	// to the scanner, not to this refresh.
	var current int
	if err = tx.QueryRowContext(ctx, `SELECT count(*) FROM catalog_assets WHERE token=? AND size=? AND modified_ns=? AND available=1`, asset, size, modified).Scan(&current); err != nil || current != 1 {
		return
	}
	if err = assets.PersistStreams(tx, asset, size, modified, facts.Streams); err != nil {
		return
	}
	_ = gated.Commit()
}
