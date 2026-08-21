package app

import (
	"reflect"
	"testing"
)

func TestMetadataRecommendationProjectionUsesRichCurrentRevisionEvidence(t *testing.T) {
	input, ok := projectMetadataRecommendationInput(metadataRecommendationEvidence{
		MediaID: "movie-1", Kind: "movie", CurrentRevision: 4,
		Values: []metadataRecommendationValue{
			{Revision: 4, Field: "originalTitle", NormalizedValue: "Le Voyage", SourceKind: "provider", Decision: "accepted"},
			{Revision: 4, Field: "criticRating", NormalizedValue: "8.7", SourceKind: "manual", Decision: "locked"},
		},
		Relationships: []metadataRecommendationRelationship{
			{Revision: 4, Type: "studio", StableKey: "tmdb:44", NormalizedValue: "Provider Studio", SourceKind: "provider", Decision: "accepted"},
			{Revision: 4, Type: "studio", StableKey: "tmdb:44", NormalizedValue: "North Star", SourceKind: "manual", Decision: "locked"},
			{Revision: 4, Type: "person", StableKey: "tmdb:9", NormalizedValue: "Ari Vega", Role: "director", SourceKind: "manual", Decision: "locked"},
			{Revision: 4, Type: "keyword", StableKey: "tmdb:71", NormalizedValue: "found family", SourceKind: "provider", Decision: "accepted"},
		},
		Identities: []metadataRecommendationIdentity{{EvidenceRevision: 4, Provider: "TMDB", ExternalType: "movie", ExternalID: "817", Status: "accepted"}},
	})
	if !ok {
		t.Fatal("supported rich media evidence was excluded")
	}
	if !reflect.DeepEqual(input.Values["originaltitle"], []string{"le voyage"}) || !reflect.DeepEqual(input.Values["criticrating"], []string{"8.7"}) {
		t.Fatalf("rich values missing: %#v", input.Values)
	}
	if !reflect.DeepEqual(input.Relationships["studio"], []string{"north star"}) || !reflect.DeepEqual(input.Relationships["person"], []string{"ari vega|director"}) || !reflect.DeepEqual(input.Relationships["keyword"], []string{"found family"}) {
		t.Fatalf("rich relationships missing: %#v", input.Relationships)
	}
	if !reflect.DeepEqual(input.ProviderIDs, []string{"tmdb:movie:817"}) {
		t.Fatalf("stable provider identity missing: %#v", input.ProviderIDs)
	}
}

func TestMetadataRecommendationProjectionRejectsStaleAndNonWinningEvidence(t *testing.T) {
	input, ok := projectMetadataRecommendationInput(metadataRecommendationEvidence{
		MediaID: "show-1", Kind: "show", CurrentRevision: 8,
		Values: []metadataRecommendationValue{
			{Revision: 7, Field: "status", NormalizedValue: "cancelled", SourceKind: "provider", Decision: "accepted"},
			{Revision: 8, Field: "status", NormalizedValue: "returning", SourceKind: "manual", Decision: "locked"},
			{Revision: 8, Field: "status", NormalizedValue: "rumored", SourceKind: "provider", Decision: "candidate"},
		},
		Relationships: []metadataRecommendationRelationship{{Revision: 7, Type: "network", StableKey: "old", NormalizedValue: "Old Network", Decision: "accepted"}},
		Identities: []metadataRecommendationIdentity{
			{EvidenceRevision: 7, Provider: "tvdb", ExternalType: "series", ExternalID: "old", Status: "accepted"},
			{EvidenceRevision: 8, Provider: "tvdb", ExternalType: "series", ExternalID: "candidate", Status: "candidate"},
		},
	})
	if !ok {
		t.Fatal("supported media excluded")
	}
	if !reflect.DeepEqual(input.Values["status"], []string{"returning"}) || len(input.Relationships) != 0 || len(input.ProviderIDs) != 0 {
		t.Fatalf("stale or non-winning evidence leaked: %#v", input)
	}
	if _, ok := projectMetadataRecommendationInput(metadataRecommendationEvidence{MediaID: "live-1", Kind: "live-channel", CurrentRevision: 1}); ok {
		t.Fatal("unsupported kind was not neutrally excluded")
	}
}

func TestScoreRecommendationsUsesRichMetadataWithoutOverridingViewerSignals(t *testing.T) {
	seedMetadata, ok := projectMetadataRecommendationInput(metadataRecommendationEvidence{
		MediaID: "seed", Kind: "movie", CurrentRevision: 2,
		Relationships: []metadataRecommendationRelationship{{Revision: 2, Type: "keyword", StableKey: "theme:found-family", NormalizedValue: "found family", SourceKind: "provider", Decision: "accepted"}},
	})
	if !ok {
		t.Fatal("seed metadata projection failed")
	}
	matchingMetadata, ok := projectMetadataRecommendationInput(metadataRecommendationEvidence{
		MediaID: "matching", Kind: "movie", CurrentRevision: 3,
		Relationships: []metadataRecommendationRelationship{{Revision: 3, Type: "keyword", StableKey: "theme:found-family", NormalizedValue: "found family", SourceKind: "provider", Decision: "accepted"}},
	})
	if !ok {
		t.Fatal("candidate metadata projection failed")
	}
	scored := scoreRecommendations(
		[]localRecommendationSeed{{ID: "seed", Type: "movie", Watched: true, Metadata: seedMetadata}},
		[]localRecommendationSeed{
			{ID: "unrelated", Type: "movie"},
			{ID: "matching", Type: "movie", Metadata: matchingMetadata},
		},
	)
	if len(scored) != 2 || scored[0].ID != "matching" || scored[0].Source != "local_recommendations" {
		t.Fatalf("rich metadata did not refine ranking: %#v", scored)
	}
	if scored[0].Score-scored[1].Score <= 0 || scored[0].Score-scored[1].Score > 12.01 {
		t.Fatalf("rich metadata contribution escaped its cap: %#v", scored)
	}
}
