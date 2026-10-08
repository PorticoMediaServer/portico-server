package ingestion

import (
	"context"
	"errors"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/remotesources"
	"portico.local/server/internal/storage"
	"strings"
	"time"
)

func (s *Service) inspectRemote(ctx context.Context, path string, before storage.Snapshot) (assets.Facts, error) {
	fallback := assets.Facts{Container: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."), InventoryOnly: true}
	work, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	obj, err := s.storage.Remote.Acquire(work, path, "analysis")
	var facts assets.Facts
	if err == nil {
		defer obj.Close()
		if obj.Snapshot().Revision != before.Revision {
			return facts, storage.ErrRemoteChanged
		}
		var url string
		var closeBridge func()
		url, closeBridge, err = storage.RemoteProbeBridge(work, obj)
		if err == nil {
			defer closeBridge()
			facts, err = s.probe.InspectRemote(work, url, filepath.Ext(path))
		}
		if err == nil {
			_, err = obj.Observe(work)
		}
	}
	code := "complete"
	if err != nil {
		code = remotesources.ErrorCode(err)
	}
	_, _ = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `INSERT INTO remote_analysis_status VALUES(?,?,?) ON CONFLICT(path) DO UPDATE SET code=excluded.code,observed_at=excluded.observed_at`, path, code, time.Now().Unix())
	if errors.Is(err, storage.ErrRemoteChanged) {
		return facts, err
	}
	if ctx.Err() != nil {
		return facts, ctx.Err()
	}
	// Availability was established by native stat. An unsupported remote probe is
	// unknown technical metadata, not evidence of disappearance or a fake codec.
	if err != nil {
		return fallback, nil
	}
	return facts, nil
}

func (s *Service) recordDescriptorAnalysis(ctx context.Context, path string, err error) {
	code := "complete"
	if errors.Is(err, storage.ErrRemoteAnalysisDisabled) {
		code = "analysis_disabled"
	} else if err != nil {
		code = remotesources.ErrorCode(err)
	}
	// Diagnostic only: a target probe failure does not make its descriptor absent.
	_, _ = dbwork.ExecWrite(ctx, s.db, dbwork.ClassBackgroundMedia, `INSERT INTO remote_analysis_status VALUES(?,?,?) ON CONFLICT(path) DO UPDATE SET code=excluded.code,observed_at=excluded.observed_at`, path, code, time.Now().Unix())
}
