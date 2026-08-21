package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestRichMetadataBrowsePredicatesExecuteAgainstCurrentEvidence(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	seed, err := server.getMedia("", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	proposal := newProviderRichProposal("tmdb", map[string]any{"id": 42, "title": seed.Title})
	proposal.Values = append(proposal.Values,
		metadataProviderValueProposal{Field: "alternateTitle", Value: "Le Méridien", Locale: "fr-CA"},
		metadataProviderValueProposal{Field: "audienceRating", Value: "8.4"},
	)
	proposal.Relationships = append(proposal.Relationships,
		metadataRelationshipProposal{Kind: "spokenLanguage", Name: "Français", Attributes: map[string]string{"iso639-1": "fr"}},
		metadataRelationshipProposal{Kind: "keyword", Name: "harbour"},
		metadataRelationshipProposal{Kind: "company", Name: "Northlight", ExternalIDs: map[string]string{"tmdb": "7"}},
		metadataRelationshipProposal{Kind: "contentRating", Name: "14A", Attributes: map[string]string{"country": "CA"}},
	)
	proposal.normalize()
	if _, err := server.applyMetadata(context.Background(), metadataApplyRequest{
		MediaID: seed.ID, ExpectedRevision: seed.MetadataRevision, Origin: metadataSourceProvider,
		Source: "tmdb-fixture", Provider: "tmdb", ProviderRich: &proposal,
		Identities: []metadataProviderIdentityProposal{{Provider: "tmdb", ExternalID: "42", ExternalType: "movie", Confidence: 1}},
	}); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		field, operator string
		value           any
	}{
		{field: "alternateTitle", operator: "contains", value: "Mérid"},
		{field: "language", operator: "contains", value: "Français"},
		{field: "keyword", operator: "contains", value: "harbour"},
		{field: "company", operator: "equals", value: "Northlight"},
		{field: "regionalCertification", operator: "equals", value: "CA:14A"},
		{field: "audienceRating", operator: "at-least", value: 8},
		{field: "acceptedProviderIdentity", operator: "contains", value: "tmdb:movie:42"},
	}
	for _, test := range tests {
		t.Run(test.field, func(t *testing.T) {
			raw, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			predicate, args, issues := compileBrowsePredicate(BrowseExpression{Field: test.field, Operator: test.operator, Value: raw}, "query")
			if len(issues) != 0 {
				t.Fatalf("compile issues: %#v", issues)
			}
			queryArgs := append([]any{seed.ID}, args...)
			items, err := server.queryMediaListItemsContext(context.Background(), "", "WHERE m.id = ? AND ("+predicate+")", queryArgs)
			if err != nil {
				t.Fatalf("execute predicate %q: %v", predicate, err)
			}
			if len(items) != 1 || items[0].ID != seed.ID {
				t.Fatalf("predicate did not return current item: %#v", items)
			}
		})
	}
}
