package compactcatalog

import (
	"context"
	"errors"
)

// ErrFacetProducerIncomplete keeps normalized SIMPLE outputs disabled until
// exact source/policy closure is established. Quiescent legacy facet buckets do
// not prove the restricted-show/member oracle or coherent retained raw inputs.
var ErrFacetProducerIncomplete = errors.New("normalized facet producer closure is incomplete")

// CheckNormalizedFacetReadiness is the opt-in barrier for the new normalized
// base reader. Call it in the same read transaction as the base snapshot. It
// deliberately does not change the legacy CheckReadiness/serving behaviour.
//
// The family must have no pending, rebuilding or failed work, including source
// fanouts and upstream browse/availability/container work. Even then this slice
// cannot certify the exact policy oracle, unbounded holders or raw-source
// retention; fail closed rather than blessing idle but incomplete output.
func CheckNormalizedFacetReadiness(ctx context.Context, q ReadQuery) error {
	var pending bool
	err := q.QueryRowContext(ctx, `SELECT
 NOT EXISTS(SELECT 1 FROM catalog_derivations WHERE domain=33 AND version>=6)
 OR EXISTS(SELECT 1 FROM catalog_derivations WHERE domain IN(18,19,20,25,26,28,31,33,35,36,37) AND rebuilding=1)
 OR EXISTS(SELECT 1 FROM catalog_dirty WHERE domain IN(18,19,20,25,26,28,31,33,35,36,37))
 OR EXISTS(SELECT 1 FROM catalog_derivation_failures WHERE domain IN(18,19,20,25,26,28,31,33,35,36,37))
 OR EXISTS(SELECT 1 FROM catalog_facet_source_fanouts)
 OR EXISTS(SELECT 1 FROM catalog_generations WHERE retaining=1)`).Scan(&pending)
	if err != nil {
		return err
	}
	if pending {
		return ErrBuilding
	}
	return ErrFacetProducerIncomplete
}
