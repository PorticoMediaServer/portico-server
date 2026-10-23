package main

import (
	"context"
	"database/sql"
	"time"

	"portico.local/server/internal/metadata"
	"portico.local/server/internal/remotesources"
	"portico.local/server/internal/workpolicy"
)

// The source resolver owns the retained actor/job, inventory and read authority.
// These adapters supply its actual metadata and supported scheduling facts from
// the same caller transaction; neither adapter authorizes a decode.
type episodeMetadataState struct{ service *metadata.Service }

func (a episodeMetadataState) ReadEpisodeStateTx(ctx context.Context, tx *sql.Tx, target remotesources.EpisodeTarget) (remotesources.EpisodeState, error) {
	if a.service == nil || tx == nil || target.LibraryID == "" || target.EpisodeID == "" {
		return remotesources.EpisodeState{}, remotesources.ErrEpisodeAuthority
	}
	state, err := a.service.ReadEpisodeThumbnailStateTx(ctx, tx, target.LibraryID, target.EpisodeID)
	if err != nil {
		return remotesources.EpisodeState{}, err
	}
	return remotesources.EpisodeState{
		PolicyRevision: state.PolicyRevision, Mode: state.Mode,
		ArtworkRevision: state.ArtworkRevision, ProviderResolved: state.ProviderResolved,
		ProviderStill: state.ProviderStill, GenerationNeeded: state.GenerationNeeded,
	}, nil
}

type episodeAnalysisSchedule struct{ now func() time.Time }

func (a episodeAnalysisSchedule) ReadEpisodeAnalysisTx(ctx context.Context, tx *sql.Tx, _ remotesources.EpisodeTarget) (remotesources.EpisodeAnalysisFacts, error) {
	at := time.Now()
	if a.now != nil {
		at = a.now()
	}
	// Only automatic Episode work asks the source resolver for this reader.
	// episode_thumbnail is an operation, never a generic maintenance task.
	facts, err := workpolicy.ReadHeavyWorkTx(ctx, tx, "analysis", at)
	if err != nil {
		return remotesources.EpisodeAnalysisFacts{}, err
	}
	return remotesources.EpisodeAnalysisFacts{
		Revision: facts.Revision,
		Open:     facts.WindowOpen && facts.PausedUntil <= at.UnixMilli() && facts.NextAllowedAt == nil,
	}, nil
}

var _ remotesources.EpisodeStateReader = episodeMetadataState{}
var _ remotesources.EpisodeAnalysisReader = episodeAnalysisSchedule{}
