package app

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/librarychannels"
)

const (
	libraryChannelMaintenanceInterval = 30 * time.Second
	libraryChannelRenewalWindow       = 48 * time.Hour
	libraryChannelWorkerBudget        = 2 * time.Minute
	libraryChannelMaintenanceBatch    = 4
)

// runLibraryChannelMaintenance owns durable schedule work. The database queue
// coalesces create, update, restore, deletion-triggered repair, and rolling
// renewal into one request per channel. This worker is deliberately bounded:
// one process, a small batch, and one channel at a time.
func (s *Server) runLibraryChannelMaintenance(ctx context.Context) {
	s.runLibraryChannelMaintenancePass(ctx)
	ticker := time.NewTicker(libraryChannelMaintenanceInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runLibraryChannelMaintenancePass(ctx)
		}
	}
}

func (s *Server) runLibraryChannelMaintenancePass(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}
	store := librarychannels.NewStore(s.dbHandle())
	if recovered, err := store.RecoverExpiredGenerations(ctx); err != nil {
		s.log.Warn("Library Channel lease recovery failed", "error", err)
	} else if recovered > 0 {
		s.log.Warn("recovered expired Library Channel generation leases", "count", recovered)
	}
	if _, err := store.EnqueueExpiringChannels(ctx, time.Now().UTC().Add(libraryChannelRenewalWindow)); err != nil {
		s.log.Warn("Library Channel renewal enqueue failed", "error", err)
		return
	}
	requests, err := store.ListDueGenerationRequests(ctx, libraryChannelMaintenanceBatch)
	if err != nil {
		s.log.Warn("Library Channel generation queue read failed", "error", err)
		return
	}
	for _, request := range requests {
		if err := ctx.Err(); err != nil {
			return
		}
		generationCtx, cancel := context.WithTimeout(ctx, libraryChannelWorkerBudget)
		_, generationErr := s.generateLibraryChannelSchedule(generationCtx, request.ChannelID)
		cancel()
		if generationErr == nil {
			continue // CommitGeneration atomically clears the satisfied queue row.
		}
		if errors.Is(generationErr, librarychannels.ErrGenerationInProgress) {
			_ = store.DeferGenerationRequest(ctx, request.ChannelID, request.RequestedRevision, 10*time.Second, librarychannels.MessageGenerationFailed)
			continue
		}
		retry := libraryChannelGenerationRetry(request.Attempts)
		if err := store.DeferGenerationRequest(ctx, request.ChannelID, request.RequestedRevision, retry, librarychannels.MessageGenerationFailed); err != nil {
			s.log.Warn("Library Channel generation retry could not be persisted", "channelId", request.ChannelID, "error", err)
		}
		s.log.Warn("Library Channel background generation failed", "channelId", request.ChannelID, "retryAfter", retry.String(), "error", generationErr)
	}
	_, _ = store.PruneCompletedGenerations(ctx, time.Now().UTC().Add(-30*24*time.Hour))
}

func libraryChannelGenerationRetry(attempts int) time.Duration {
	if attempts < 0 {
		attempts = 0
	}
	exponent := math.Min(float64(attempts), 6)
	retry := time.Duration(30*math.Pow(2, exponent)) * time.Second
	if retry > 30*time.Minute {
		return 30 * time.Minute
	}
	return retry
}

// renewLibraryChannelGenerationLease keeps a commit lease alive until the
// caller finishes. CommitGeneration still validates the lease against the
// database clock, so cancellation or renewal failure can never authorize a
// stale replacement.
func renewLibraryChannelGenerationLease(ctx context.Context, store *librarychannels.Store, generationID, token string) func() {
	renewCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(libraryChannelGenerationLease / 3)
		defer ticker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-ticker.C:
				if _, err := store.RenewGenerationLease(renewCtx, generationID, token, libraryChannelGenerationLease); err != nil {
					return
				}
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}
