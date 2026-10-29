package catalog

import (
	"context"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/compactcatalog"
)

// AcceptInventoryAnalysis accepts immutable technical inputs before naming and
// queue acknowledgement. Accepted-pending is durable success, not an assertion
// that canonical rows or item summaries are already applied. The scan retries
// its exact observation while source41 performs the bounded publication.
func (s *Service) AcceptInventoryAnalysis(ctx context.Context, job, object, revision string, policyRevision int64, facts assets.Facts) (compactcatalog.InfoAcceptReceipt, error) {
	receipt, err := s.acceptInventoryInfo(ctx, job, object, revision, policyRevision, facts, false)
	if err != nil || receipt.State != compactcatalog.InfoInputApplied {
		return receipt, err
	}
	return receipt, s.commitInventoryAnalysisApplied(ctx, job, object, revision, policyRevision, facts)
}

// AcceptInventoryLocalRefresh has the same explicit receipt contract while
// retaining the refresh's original analysis/source/policy fences.
func (s *Service) AcceptInventoryLocalRefresh(ctx context.Context, job, object, revision string, policyRevision int64, facts assets.Facts) (compactcatalog.InfoAcceptReceipt, error) {
	receipt, err := s.acceptInventoryInfo(ctx, job, object, revision, policyRevision, facts, true)
	if err != nil || receipt.State != compactcatalog.InfoInputApplied {
		return receipt, err
	}
	return receipt, s.commitInventoryLocalRefreshApplied(ctx, job, object, revision, policyRevision, facts)
}
