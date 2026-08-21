package librarychannels

import "context"

// ScheduleCandidateResolver is the narrow adapter the canonical browse
// compiler must implement. The adapter receives a rule only after the shared
// canonical V1 AST validator and schedule-safe field policy have accepted it.
// It must return a profile-independent, deterministic snapshot containing only
// playable leaf media in canonical sort order. A query selecting show
// containers selects eligible series and MUST expand them to episode Candidates
// with SeriesID, SeasonNumber, and EpisodeNumber populated. A query selecting
// episodes resolves those leaves directly. Container IDs are never schedulable
// Candidates. EpisodeInOrder interleaves eligible series while preserving
// episode order; EpisodeMarathon deliberately holds one series until exhausted.
// Library Channels intentionally has no fallback query interpreter.
type ScheduleCandidateResolver interface {
	ResolveScheduleCandidates(context.Context, Rule) ([]Candidate, error)
}

func ResolveCandidateSnapshot(ctx context.Context, aggregate Aggregate, resolver ScheduleCandidateResolver) (map[string][]Candidate, error) {
	if err := ValidateAggregate(aggregate); err != nil {
		return nil, err
	}
	if resolver == nil {
		return nil, invalid("candidateResolver", "is required")
	}
	result := make(map[string][]Candidate)
	for _, rule := range aggregate.Rules {
		if !rule.Enabled {
			continue
		}
		candidates, err := resolver.ResolveScheduleCandidates(ctx, rule)
		if err != nil {
			return nil, err
		}
		result[rule.ID] = candidates
	}
	return result, nil
}
