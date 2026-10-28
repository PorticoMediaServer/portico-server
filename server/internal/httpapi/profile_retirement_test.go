package httpapi

import (
	"context"
	"testing"

	"portico.local/server/internal/identity"
)

func retireProfileInHTTPTest(t *testing.T, s *identity.Service, ctx context.Context, bearer, id string, expected int64, deleteSaved bool) error {
	t.Helper()
	preview, err := s.PreviewDirectProfileDeletion(ctx, bearer, id)
	if err != nil {
		return err
	}
	if _, err = s.RetireDirectProfile(ctx, bearer, id, identity.ProfileRetirementRequest{ExpectedRevision: expected, Lifetime: preview.Lifetime, DeleteSavedResources: deleteSaved}); err != nil {
		return err
	}
	for i := 0; i < 20000; i++ {
		worked, err := s.PurgeProfileStep(ctx)
		if err != nil {
			return err
		}
		if !worked {
			return nil
		}
	}
	t.Fatal("profile purge failed to converge")
	return nil
}
