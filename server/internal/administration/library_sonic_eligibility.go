package administration

import (
	"context"
	"database/sql"
)

// LibrarySonicAuthority checks the current owner/main family in the same
// snapshot as the Library document. It grants no mutation or source access.
type LibrarySonicAuthority func(context.Context, *sql.Tx) (bool, error)

func (s *Service) librarySonicEligibility(ctx context.Context, tx *sql.Tx, doc *LibraryDocument, authority []LibrarySonicAuthority) error {
	controls := make([]string, 0, len(doc.EligibleControls)+2)
	configurable := false
	for _, id := range doc.EligibleControls {
		if id == "libraries.analysisSteps" {
			configurable = true
		}
		if id != "libraries.sonicAnalysis" && id != "libraries.sonicStatus" {
			controls = append(controls, id)
		}
	}
	doc.EligibleControls = controls
	if !configurable || doc.LibraryKind != "music" || len(authority) != 1 || authority[0] == nil || s.SonicRuntimeAvailable == nil || !s.SonicRuntimeAvailable() {
		return nil
	}
	allowed, err := authority[0](ctx, tx)
	if err != nil {
		return err
	}
	if allowed {
		doc.EligibleControls = append(doc.EligibleControls, "libraries.sonicAnalysis", "libraries.sonicStatus")
	}
	return nil
}
