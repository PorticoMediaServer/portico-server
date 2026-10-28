package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/readprep"
)

var ErrFacetAuthorityUnavailable = errors.New("facet read authority is not installed")

type facetAuthorityContextKey struct{}

// WithFacetWindowAuthority installs a request's live authority checker. Runtime
// instances never capture a request principal or mutable viewer globally.
func WithFacetWindowAuthority(ctx context.Context, check FacetWindowCheck) context.Context {
	return context.WithValue(ctx, facetAuthorityContextKey{}, check)
}

// CheckFacetWindowAuthority is the process runtime's dispatcher. A missing
// request guard cannot authorize either preparing state or cached metadata.
func CheckFacetWindowAuthority(ctx context.Context, caller readprep.Caller, snapshot FacetBaseSnapshot) error {
	check, ok := ctx.Value(facetAuthorityContextKey{}).(FacetWindowCheck)
	if !ok || check == nil {
		return ErrFacetAuthorityUnavailable
	}
	return check(ctx, caller, snapshot)
}

// CaptureFacetAuthority is separate from the content source epoch. Ordinary
// metadata churn may retain a COMPLETE content basis; changes to facts used by
// an active restriction predicate cannot continue disclosing its old counts.
// Library grants and caller/session/profile authority are rechecked by HTTP.
func CaptureFacetAuthority(ctx context.Context, q compactcatalog.ReadQuery, scope FacetBaseScope) (string, error) {
	r, err := parseFacetBasePolicy(scope.Policy)
	if err != nil {
		return "", err
	}
	var library, kind string
	var policyEpoch, genreEpoch int64
	err = q.QueryRowContext(ctx, `SELECT c.library_id,l.kind,s.policy_epoch,s.genre_epoch
 FROM catalog_libraries c JOIN libraries l ON l.id=c.library_id
 CROSS JOIN catalog_facet_source_state s
 WHERE c.id=? AND c.library_id=? AND c.retired=0 AND s.singleton=1`, scope.LibraryID, scope.Library).Scan(&library, &kind, &policyEpoch, &genreEpoch)
	if err != nil {
		return "", err
	}
	document := struct {
		LibraryID   int64  `json:"libraryId"`
		Library     string `json:"library"`
		Kind        string `json:"kind"`
		Algorithm   int    `json:"algorithm"`
		PolicyEpoch *int64 `json:"policyEpoch,omitempty"`
		GenreEpoch  *int64 `json:"genreEpoch,omitempty"`
	}{LibraryID: scope.LibraryID, Library: library, Kind: kind, Algorithm: 1}
	genre := scope.Source == "search-video-genre-v1" || strings.HasPrefix(scope.Source, "search-video-genre-v1:")
	if r.Active() {
		document.PolicyEpoch = &policyEpoch
		// Only Search genres has the empty-title fallback that a first hidden
		// member can revoke. SIMPLE has no such witness; new-title materialization
		// must not revoke its existing COMPLETE content basis during a scan.
		if genre {
			document.GenreEpoch = &genreEpoch
		}
	}
	raw, err := json.Marshal(document)
	return string(raw), err
}

// IsSimpleFacetRequest uses the same dispatch order as Facets. Parsed nil
// queries stay nil, and Reading always wins over pivot/field dispatch.
func (s *Service) IsSimpleFacetRequest(request FacetRequest) (bool, error) {
	library, err := s.library(request.Library)
	if err != nil {
		return false, err
	}
	if isReadingLibrary(library.Kind) {
		return false, nil
	}
	return request.Query == nil && request.Pivot == "" && request.Field != "library" && request.Field != "actor" && request.Field != "director" && request.Field != "writer", nil
}
