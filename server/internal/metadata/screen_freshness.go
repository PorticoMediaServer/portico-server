package metadata

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/metadataprovider"
)

// The refresh path for an item whose identity is already accepted. Before this, every wake of
// such a row called the provider again — a scan, a sidecar appearing, a policy edit, a show
// fanning out to its episodes — and asked for a document it had fetched minutes earlier
// (screen_worker.go:344). Doc 46 named it: "A legitimately woken row always calls the provider
// even if it holds an accepted identity and an hour-old publication."
//
// Three things now stand between a wake and a request, in order: is the document still inside
// its freshness window, is there any allowance left for refreshing documents we already hold,
// and can the provider answer "not modified" instead of sending it again.

// screenConditionalProvider is a provider that can be asked conditionally. It is a separate,
// optional interface so ScreenProvider stays exactly as it was: a provider that does not offer
// validators, or a fake in a test, simply takes the unconditional path.
type screenConditionalProvider interface {
	ScreenDetailsConditional(ctx context.Context, kind, id, language, region string, in metadataprovider.Conditional) (metadataprovider.ScreenRecord, metadataprovider.Conditional, error)
}

// rememberedScreenRecord is the accepted identity's last stored provider document. It is the
// evidence a publication was built from, so republishing from it produces exactly what the
// provider would have produced, without asking.
func (s *Service) rememberedScreenRecord(ctx context.Context, b screenBase, id metadataprovider.ScreenID) (metadataprovider.ScreenRecord, bool, error) {
	var payload string
	e := dbwork.QueryRow(ctx, s.db, `SELECT payload FROM screen_metadata_candidates WHERE target_kind=? AND target_id=? AND provider=? AND provider_type=? AND provider_id=? LIMIT 1`,
		b.TargetKind, b.Entity, id.Provider, id.Type, id.ID).Scan(&payload)
	if errors.Is(e, sql.ErrNoRows) {
		return metadataprovider.ScreenRecord{}, false, nil
	}
	if e != nil {
		return metadataprovider.ScreenRecord{}, false, e
	}
	var r metadataprovider.ScreenRecord
	if json.Unmarshal([]byte(payload), &r) != nil || r.Identity != id {
		return metadataprovider.ScreenRecord{}, false, nil
	}
	if metadataprovider.ValidateScreenRecord(r) != nil {
		return metadataprovider.ScreenRecord{}, false, nil
	}
	return r, true, nil
}

// airingStatuses are the provider statuses that mean "another episode is coming". They are the
// one case where a fortnight is too long to wait: somebody watching a current series expects
// this week's episode to be there.
var airingStatuses = map[string]bool{"Returning Series": true, "In Production": true, "Planned": true, "Continuing": true, "Upcoming": true}

// recentlyAdded is how long after arriving in the library an item counts as the thing somebody
// is most likely about to open.
const recentlyAdded = 7 * 24 * time.Hour

// screenRefreshPriority decides how hard this row's freshness window is. Nothing here reaches
// a provider; it reads what the library already knows.
func (s *Service) screenRefreshPriority(ctx context.Context, b screenBase, remembered metadataprovider.ScreenRecord, now time.Time) RefreshPriority {
	if airingStatuses[remembered.Status] {
		return RefreshAiring
	}
	var added sql.NullString
	query := `SELECT added_text FROM catalog_item_details WHERE entity_id=?`
	if b.TargetKind == "show" {
		query = `SELECT min(d.added_text) FROM catalog_episodes e JOIN catalog_item_details d ON d.entity_id=e.entity_id WHERE e.show_id=?`
	}
	if e := dbwork.QueryRow(ctx, s.db, query, b.Entity).Scan(&added); e != nil || !added.Valid {
		return RefreshOrdinary
	}
	at, e := time.Parse(time.RFC3339, added.String)
	if e != nil || now.Sub(at) > recentlyAdded {
		return RefreshOrdinary
	}
	return RefreshRecentlyAdded
}

// screenRefreshRecord produces the accepted identity's current document, asking the provider
// only when the rules allow it. It reports the record and whether it came from the provider.
func (s *Service) screenRefreshRecord(ctx context.Context, p *screenClaim, id metadataprovider.ScreenID, provider ScreenProvider) (metadataprovider.ScreenRecord, error) {
	b := p.Base
	now := s.publicationTime().UTC()
	remembered, held, e := s.rememberedScreenRecord(ctx, b, id)
	if e != nil {
		return metadataprovider.ScreenRecord{}, e
	}
	kind := id.Type
	if held {
		priority := s.screenRefreshPriority(ctx, b, remembered, now)
		fresh, validators, err := s.documentFresh(ctx, id.Provider, kind, id.ID, priority, now)
		if err != nil {
			return metadataprovider.ScreenRecord{}, err
		}
		if fresh {
			// Inside the window the provider is not asked at all. The stored document is
			// republished, so a local change that woke this row still reaches the catalogue.
			return remembered, nil
		}
		// A refresh of something already held is what the budget is for. A first fetch never
		// reaches here, because there is nothing remembered to fall back to.
		admitted, err := s.admitRefresh(ctx, id.Provider, now)
		if err != nil {
			return metadataprovider.ScreenRecord{}, err
		}
		if !admitted {
			return remembered, nil
		}
		if conditional, ok := provider.(screenConditionalProvider); ok && !toConditional(validators).Empty() {
			r, out, err := conditional.ScreenDetailsConditional(ctx, kind, id.ID, b.Language, b.Region, toConditional(validators))
			if errors.Is(err, metadataprovider.ErrNotModified) {
				// The cheapest possible answer: no body, nothing to parse, and the window
				// starts again from now.
				if noteErr := s.noteDocumentChecked(ctx, id.Provider, kind, id.ID, fromConditional(out), false, now); noteErr != nil {
					return metadataprovider.ScreenRecord{}, noteErr
				}
				return remembered, nil
			}
			if err != nil {
				return metadataprovider.ScreenRecord{}, err
			}
			if noteErr := s.noteDocumentChecked(ctx, id.Provider, kind, id.ID, fromConditional(out), true, now); noteErr != nil {
				return metadataprovider.ScreenRecord{}, noteErr
			}
			return r, nil
		}
	}
	r, e := provider.ScreenDetails(ctx, kind, id.ID, b.Language, b.Region)
	if e != nil {
		return metadataprovider.ScreenRecord{}, e
	}
	if e = s.noteDocumentChecked(ctx, id.Provider, kind, id.ID, documentValidators{}, true, now); e != nil {
		return metadataprovider.ScreenRecord{}, e
	}
	return r, nil
}

func toConditional(v documentValidators) metadataprovider.Conditional {
	return metadataprovider.Conditional{ETag: v.ETag, LastModified: v.LastModified}
}

func fromConditional(c metadataprovider.Conditional) documentValidators {
	return documentValidators{ETag: c.ETag, LastModified: c.LastModified}
}
