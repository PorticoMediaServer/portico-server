package librarychannels

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestGenerateIsDeterministicAndGapFree(t *testing.T) {
	aggregate := testAggregate("America/Halifax")
	request := testGenerateRequest(aggregate, mustLocalTime(t, aggregate.Channel.Timezone, 2026, time.January, 10, 8, 15))
	first, err := Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("generate first schedule: %v", err)
	}
	second, err := Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("generate second schedule: %v", err)
	}
	if first.Generation.DeterministicSeed != second.Generation.DeterministicSeed {
		t.Fatalf("deterministic seeds differ: %q != %q", first.Generation.DeterministicSeed, second.Generation.DeterministicSeed)
	}
	if !reflect.DeepEqual(first.Entries, second.Entries) {
		t.Fatal("identical scheduling inputs produced different entries")
	}
	if !reflect.DeepEqual(first.NextCursors, second.NextCursors) {
		t.Fatal("identical scheduling inputs produced different cursors")
	}
	if err := ValidateScheduleEntries(aggregate.Channel.ID, request.GenerationID, first.Generation.HorizonStart, first.Generation.HorizonEnd, first.Entries); err != nil {
		t.Fatalf("validate schedule: %v", err)
	}
	if got := first.Generation.HorizonEnd.Sub(first.Generation.HorizonStart); got != 7*24*time.Hour {
		t.Fatalf("ordinary seven-day horizon = %v, want 168h", got)
	}
}

func TestGenerateResolvesOverlappingBlocksByPriorityWithoutOverlap(t *testing.T) {
	aggregate := testAggregate("America/Halifax")
	aggregate.Rules = append(aggregate.Rules, Rule{
		ID: "rule-special", ChannelID: aggregate.Channel.ID, Name: "Special", Enabled: true,
		Query:         json.RawMessage(`{"field":"genre","operator":"contains","value":"Comedy"}`),
		SelectionMode: SelectionSequential, EpisodeMode: EpisodeNone, ExhaustionMode: ExhaustionLoop,
		Config: json.RawMessage(`{}`),
	})
	aggregate.Blocks = []WeeklyBlock{
		{ID: "block-low", ChannelID: aggregate.Channel.ID, RuleID: "rule-default", Name: "Morning", Enabled: true, Weekdays: 127, StartMinute: 6 * 60, EndMinute: 12 * 60, Priority: 1},
		{ID: "block-high", ChannelID: aggregate.Channel.ID, RuleID: "rule-special", Name: "Anchored special", Enabled: true, Weekdays: 127, StartMinute: 8 * 60, EndMinute: 10 * 60, Priority: 10, Anchored: true},
	}
	request := testGenerateRequest(aggregate, mustLocalTime(t, aggregate.Channel.Timezone, 2026, time.February, 2, 6, 0))
	request.Days = 1
	request.Candidates["rule-special"] = []Candidate{
		{MediaID: "media-special", Title: "Special", Duration: 30 * time.Minute, Weight: 1, Artwork: json.RawMessage(`{}`)},
	}
	result, err := Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("generate schedule: %v", err)
	}
	if err := ValidateScheduleEntries(aggregate.Channel.ID, request.GenerationID, result.Generation.HorizonStart, result.Generation.HorizonEnd, result.Entries); err != nil {
		t.Fatalf("validate schedule: %v", err)
	}
	location, _ := time.LoadLocation(aggregate.Channel.Timezone)
	for _, entry := range result.Entries {
		local := entry.StartsAt.In(location)
		minute := local.Hour()*60 + local.Minute()
		if minute >= 8*60 && minute < 10*60 && entry.Kind == EntryMedia && entry.RuleID != "rule-special" {
			t.Fatalf("lower-priority rule won inside special block: %+v", entry)
		}
	}
}

func TestGenerateUsesCalendarDaysAcrossDST(t *testing.T) {
	aggregate := testAggregate("America/New_York")
	aggregate.Blocks = []WeeklyBlock{{
		ID: "block-dst", ChannelID: aggregate.Channel.ID, RuleID: "rule-default", Name: "DST block",
		Enabled: true, Weekdays: 127, StartMinute: 90, EndMinute: 210, Priority: 10, Anchored: true,
	}}

	t.Run("spring forward", func(t *testing.T) {
		start := mustLocalTime(t, aggregate.Channel.Timezone, 2026, time.March, 7, 12, 0)
		request := testGenerateRequest(aggregate, start)
		result, err := Generate(context.Background(), request)
		if err != nil {
			t.Fatalf("generate spring schedule: %v", err)
		}
		if got := result.Generation.HorizonEnd.Sub(result.Generation.HorizonStart); got != 167*time.Hour {
			t.Fatalf("spring horizon = %v, want 167h", got)
		}
		assertGapFree(t, result)
	})

	t.Run("fall back", func(t *testing.T) {
		start := mustLocalTime(t, aggregate.Channel.Timezone, 2026, time.October, 31, 12, 0)
		request := testGenerateRequest(aggregate, start)
		result, err := Generate(context.Background(), request)
		if err != nil {
			t.Fatalf("generate fall schedule: %v", err)
		}
		if got := result.Generation.HorizonEnd.Sub(result.Generation.HorizonStart); got != 169*time.Hour {
			t.Fatalf("fall horizon = %v, want 169h", got)
		}
		assertGapFree(t, result)
	})
}

func TestWallClockBoundariesHaveExplicitDSTPolicy(t *testing.T) {
	location, _ := time.LoadLocation("America/New_York")
	spring := localDate{Year: 2026, Month: time.March, Day: 8}
	missing, err := resolveWallMinute(spring, 2*60+30, location, false)
	if err != nil {
		t.Fatal(err)
	}
	if local := missing.In(location); local.Hour() != 3 || local.Minute() != 0 {
		t.Fatalf("nonexistent 02:30 resolved to %v, want first valid instant 03:00", local)
	}

	fall := localDate{Year: 2026, Month: time.November, Day: 1}
	first, err := resolveWallMinute(fall, 90, location, false)
	if err != nil {
		t.Fatal(err)
	}
	last, err := resolveWallMinute(fall, 90, location, true)
	if err != nil {
		t.Fatal(err)
	}
	if last.Sub(first) != time.Hour {
		t.Fatalf("repeated 01:30 boundaries differ by %v, want 1h", last.Sub(first))
	}
}

func TestValidateScheduleQueryRejectsViewerState(t *testing.T) {
	err := ValidateScheduleQuery(json.RawMessage(`{
		"all": [
			{"field": "genre", "operator": "equals", "value": "Drama"},
			{"field": "lastPlayedAt", "operator": "before", "value": "2026-01-01"}
		]
	}`))
	if err == nil {
		t.Errorf("expected viewer-state field to be rejected")
	}
	if err := ValidateScheduleQuery(json.RawMessage(`{"field":"contentRating","operator":"equals","value":"PG"}`)); err != nil {
		t.Fatalf("canonical global metadata query was rejected: %v", err)
	}
	if err := ValidateScheduleQuery(json.RawMessage(`{"field":"studio","operator":"equals","value":"A24"}`)); err == nil {
		t.Fatal("field outside the canonical browse contract was accepted")
	}
}

func TestScheduleQueryASTFailsClosed(t *testing.T) {
	invalidQueries := []json.RawMessage{
		json.RawMessage(`null`),
		json.RawMessage(`{"field":"profileId","operator":"equals","value":"p"}`),
		json.RawMessage(`{"field":"genre","operator":"execute","value":"Drama"}`),
		json.RawMessage(`{"field":"genre","operator":"equals","value":"Drama","extra":true}`),
		json.RawMessage(`{"all":[]}`),
		json.RawMessage(`{"unknown":[]}`),
	}
	for _, raw := range invalidQueries {
		if err := ValidateScheduleQuery(raw); err == nil {
			t.Errorf("query accepted: %s", raw)
		}
	}
}

func TestScheduleQueryUsesCanonicalFieldOperatorAndValueContract(t *testing.T) {
	valid := []json.RawMessage{
		json.RawMessage(`{"field":"genre","operator":"not-contains","value":"Horror"}`),
		json.RawMessage(`{"field":"year","operator":"at-least","value":1980}`),
		json.RawMessage(`{"field":"dateAdded","operator":"between","value":["2025-01-01","2026-01-01"]}`),
	}
	for _, raw := range valid {
		if err := ValidateScheduleQuery(raw); err != nil {
			t.Errorf("canonical query rejected (%s): %v", raw, err)
		}
	}
	invalid := []json.RawMessage{
		json.RawMessage(`{"field":"year","operator":"contains","value":"1980"}`),
		json.RawMessage(`{"field":"dateAdded","operator":"equals","value":"yesterday"}`),
		json.RawMessage(`{"field":"entityKind","operator":"equals","value":"server"}`),
		json.RawMessage(`{"field":"favorite","operator":"is","value":true}`),
	}
	for _, raw := range invalid {
		if err := ValidateScheduleQuery(raw); err == nil {
			t.Errorf("non-canonical or profile-specific query accepted: %s", raw)
		}
	}
}

func TestEmittedLibraryChannelLanguageKeysExist(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "api", "product-language", "en-US.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Messages map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	keys := []string{
		string(ReasonNoScheduledMedia), string(ReasonMediaUnavailable), string(ReasonMediaRestricted),
		MessageNoPlayableCandidates, MessageNoPlayableSchedule, MessageGenerationFailed, MessageGenerationLeaseExpired,
		MessageRegenerationRequired, MessageScheduleWarnings, "library-channel.logo-processing-overhead",
	}
	for _, key := range keys {
		if _, exists := catalog.Messages[key]; !exists {
			t.Errorf("emitted Library Channel language key %q is absent", key)
		}
	}
}

func TestValidateLogoRequiresSafeConfiguration(t *testing.T) {
	channel := testAggregate("UTC").Channel
	channel.Logo = LogoConfig{
		Source: LogoCustom, Ref: "logo.svg", MIMEType: "text/html", BugEnabled: true,
		BugCorner: LogoTopRight, BugWidthPct: 8, BugInsetPct: 2.5, BugTreatment: LogoColor,
	}
	if err := ValidateChannel(channel); err == nil {
		t.Errorf("expected unsafe custom logo MIME type to be rejected")
	}
	channel.Logo.MIMEType = "image/svg+xml"
	channel.Logo.BugOverheadAccepted = true
	if err := ValidateChannel(channel); err != nil {
		t.Fatalf("valid custom logo rejected: %v", err)
	}
	channel.Logo.Ref = "../../private/logo.svg"
	if err := ValidateChannel(channel); err == nil {
		t.Fatal("path-like logo reference was accepted")
	}
	channel.Logo.Ref = "asset:logo-1"
	channel.Logo.BugWidthPct = math.NaN()
	if err := ValidateChannel(channel); err == nil {
		t.Fatal("NaN logo percentage was accepted")
	}
}

func TestGenerateRejectsInvalidCandidate(t *testing.T) {
	aggregate := testAggregate("UTC")
	request := testGenerateRequest(aggregate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	request.Candidates["rule-default"][0].Duration = 0
	_, err := Generate(context.Background(), request)
	var validation *ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("error = %v, want ValidationError", err)
	}
}

func TestScheduleEntryLimitIsEnforcedBeforeAppend(t *testing.T) {
	entries := make([]ScheduleEntry, MaximumScheduleEntries)
	if err := appendScheduleEntry(&entries, ScheduleEntry{}); err == nil {
		t.Fatal("schedule entry amplification limit was not enforced")
	}
	if len(entries) != MaximumScheduleEntries {
		t.Fatalf("rejected append changed schedule length to %d", len(entries))
	}
}

func TestGenerateRejectsCorruptCursorAndDuplicateOrNonFiniteCandidates(t *testing.T) {
	aggregate := testAggregate("UTC")
	request := testGenerateRequest(aggregate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	request.InitialCursors["rule-default"] = RuleCursor{Position: -1}
	if _, err := Generate(context.Background(), request); err == nil {
		t.Fatal("negative cursor was accepted")
	}
	request.InitialCursors = map[string]RuleCursor{}
	request.Candidates["rule-default"] = append(request.Candidates["rule-default"], request.Candidates["rule-default"][0])
	if _, err := Generate(context.Background(), request); err == nil {
		t.Fatal("duplicate media was accepted")
	}
	request.Candidates["rule-default"] = request.Candidates["rule-default"][:1]
	request.Candidates["rule-default"][0].Weight = math.Inf(1)
	if _, err := Generate(context.Background(), request); err == nil {
		t.Fatal("infinite weight was accepted")
	}
}

func TestCandidateHashCoversScheduleSnapshotMetadata(t *testing.T) {
	aggregate := testAggregate("UTC")
	request := testGenerateRequest(aggregate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	first, err := Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	request.Candidates["rule-default"][0].Title = "Changed title"
	second, err := Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if first.Generation.CandidateHash == second.Generation.CandidateHash {
		t.Fatal("candidate metadata change did not change immutable snapshot hash")
	}
}

func TestGenerateUsesShorterCandidateBeforeBoundarySlate(t *testing.T) {
	aggregate := testAggregate("UTC")
	aggregate.Rules[0].SelectionMode = SelectionSequential
	aggregate.Blocks = []WeeklyBlock{{ID: "hour", ChannelID: aggregate.Channel.ID, RuleID: "rule-default", Name: "Hour", Enabled: true, Weekdays: 127, StartMinute: 0, EndMinute: 60, Anchored: true}}
	request := testGenerateRequest(aggregate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	request.Days = 1
	request.Candidates["rule-default"] = []Candidate{{MediaID: "long", Duration: 90 * time.Minute, Artwork: json.RawMessage(`{}`)}, {MediaID: "short", Duration: 30 * time.Minute, Artwork: json.RawMessage(`{}`)}}
	result, err := Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Entries[0].Kind != EntryMedia || result.Entries[0].MediaID != "short" {
		t.Fatalf("first entry = %+v", result.Entries[0])
	}
}

func TestWeightedSelectionCannotMissOnlyFittingCandidate(t *testing.T) {
	rule := Rule{ID: "weighted", SelectionMode: SelectionWeightedRandom, EpisodeMode: EpisodeNone, ExhaustionMode: ExhaustionLoop}
	candidates := []Candidate{{MediaID: "long", Duration: 2 * time.Hour, Weight: 1_000_000}, {MediaID: "fit", Duration: 20 * time.Minute, Weight: 0.000001}}
	candidate, _, _, _, ok := pickCandidateFitting(rule, candidates, RuleCursor{}, "stable", 30*time.Minute)
	if !ok || candidate.MediaID != "fit" {
		t.Fatalf("selection = %+v, %v", candidate, ok)
	}
}

func TestWeightedFittingFallbackPreservesDedupeAndConsecutiveRules(t *testing.T) {
	rule := Rule{ID: "weighted", SelectionMode: SelectionWeightedRandom, EpisodeMode: EpisodeNone, ExhaustionMode: ExhaustionLoop, DedupeWindow: 2, MaxConsecutive: 1}
	short := Candidate{MediaID: "short-a", SeriesID: "series-a", Duration: 20 * time.Minute, Weight: 0.000001}
	longAlternative := Candidate{MediaID: "long-b", SeriesID: "series-b", Duration: 2 * time.Hour, Weight: 1_000_000}
	cursor := RuleCursor{RecentMedia: []string{mediaCursorKey(short.MediaID)}, LastGroup: groupCursorKey(short.SeriesID), Consecutive: 1}
	if candidate, _, _, _, ok := pickCandidateFitting(rule, []Candidate{longAlternative, short}, cursor, "stable", 30*time.Minute); ok {
		t.Fatalf("constraint-bypassing fallback selected %+v", candidate)
	}
}

func TestUnavailableCandidatesDoNotDisplacePlayableMedia(t *testing.T) {
	rule := Rule{ID: "ordered", SelectionMode: SelectionSequential, EpisodeMode: EpisodeNone, ExhaustionMode: ExhaustionLoop}
	candidates := []Candidate{
		{MediaID: "missing", Duration: 20 * time.Minute, Availability: "missing"},
		{MediaID: "available", Duration: 20 * time.Minute, Availability: "available"},
	}
	selected, _, _, _, ok := pickCandidateFitting(rule, candidates, RuleCursor{}, "stable", time.Hour)
	if !ok || selected.MediaID != "available" {
		t.Fatalf("selected = %+v, %v", selected, ok)
	}
}

func TestEpisodeInOrderInterleavesSeriesWhileMarathonDoesNot(t *testing.T) {
	candidates := []Candidate{
		{MediaID: "a1", SeriesID: "a", SeasonNumber: 1, EpisodeNumber: 1, Duration: time.Minute},
		{MediaID: "a2", SeriesID: "a", SeasonNumber: 1, EpisodeNumber: 2, Duration: time.Minute},
		{MediaID: "b1", SeriesID: "b", SeasonNumber: 1, EpisodeNumber: 1, Duration: time.Minute},
		{MediaID: "b2", SeriesID: "b", SeasonNumber: 1, EpisodeNumber: 2, Duration: time.Minute},
	}
	ordered := prepareCandidates(Rule{ID: "ordered", EpisodeMode: EpisodeInOrder}, candidates, &RuleCursor{}, "seed")
	if got := []string{ordered[0].MediaID, ordered[1].MediaID, ordered[2].MediaID, ordered[3].MediaID}; !reflect.DeepEqual(got, []string{"a1", "b1", "a2", "b2"}) {
		t.Fatalf("ordered series sequence = %v", got)
	}
}

func TestGeneratedScheduleUsesSparseFinalCheckpoints(t *testing.T) {
	result, err := Generate(context.Background(), testGenerateRequest(testAggregate("UTC"), time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatal(err)
	}
	checkpoints := 0
	for _, entry := range result.Entries {
		if len(entry.CursorAfter) > 0 {
			checkpoints++
		}
	}
	maximum := len(result.Entries)/ScheduleCheckpointInterval + 1
	if checkpoints == 0 || checkpoints > maximum || len(result.Entries[len(result.Entries)-1].CursorAfter) == 0 {
		t.Fatalf("checkpoints=%d entries=%d maximum=%d", checkpoints, len(result.Entries), maximum)
	}
}

func TestGenerateRollingTailPreservesPrefixAndUsesCheckpoint(t *testing.T) {
	aggregate := testAggregate("UTC")
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first, err := Generate(context.Background(), testGenerateRequest(aggregate, start))
	if err != nil {
		t.Fatal(err)
	}
	newStart, tailStart := start.Add(24*time.Hour), first.Generation.HorizonEnd
	prefix := []ScheduleEntry{}
	for _, entry := range first.Entries {
		if !entry.StartsAt.Before(newStart) && !entry.EndsAt.After(tailStart) {
			prefix = append(prefix, entry)
		}
	}
	request := testGenerateRequest(aggregate, newStart)
	request.GenerationID = "generation-two"
	request.TailStart = tailStart
	request.PreservedEntries = prefix
	second, err := Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("extend: %v", err)
	}
	if !second.Generation.HorizonEnd.Equal(first.Generation.HorizonEnd.Add(24 * time.Hour)) {
		t.Fatalf("new horizon = %v", second.Generation.HorizonEnd)
	}
	if second.Generation.DeterministicSeed != first.Generation.DeterministicSeed {
		t.Fatal("rolling extension reshuffled the stable seed")
	}
	if len(second.Entries) <= len(prefix) {
		t.Fatal("tail was not extended")
	}
	for i := range prefix {
		if second.Entries[i].ID != prefix[i].ID || second.Entries[i].MediaID != prefix[i].MediaID || !second.Entries[i].StartsAt.Equal(prefix[i].StartsAt) {
			t.Fatalf("prefix changed at %d", i)
		}
	}
}

func TestMarathonKeepsEpisodesInOrder(t *testing.T) {
	aggregate := testAggregate("UTC")
	aggregate.Rules[0].EpisodeMode = EpisodeMarathon
	aggregate.Rules[0].SelectionMode = SelectionShuffleBag
	request := testGenerateRequest(aggregate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	request.Days = 1
	request.Candidates["rule-default"] = []Candidate{
		{MediaID: "episode-3", SeriesID: "show-one", SeasonNumber: 1, EpisodeNumber: 3, Title: "Three", Duration: 30 * time.Minute, Artwork: json.RawMessage(`{}`)},
		{MediaID: "episode-1", SeriesID: "show-one", SeasonNumber: 1, EpisodeNumber: 1, Title: "One", Duration: 30 * time.Minute, Artwork: json.RawMessage(`{}`)},
		{MediaID: "episode-2", SeriesID: "show-one", SeasonNumber: 1, EpisodeNumber: 2, Title: "Two", Duration: 30 * time.Minute, Artwork: json.RawMessage(`{}`)},
	}
	result, err := Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("generate marathon: %v", err)
	}
	got := make([]string, 0, 3)
	for _, entry := range result.Entries {
		if entry.Kind == EntryMedia {
			got = append(got, entry.MediaID)
			if len(got) == 3 {
				break
			}
		}
	}
	want := []string{"episode-1", "episode-2", "episode-3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("first marathon episodes = %v, want %v", got, want)
	}
}

func TestMarathonAdvancesToAnotherSeriesAfterCompletion(t *testing.T) {
	aggregate := testAggregate("UTC")
	aggregate.Rules[0].EpisodeMode = EpisodeMarathon
	aggregate.Rules[0].SelectionMode = SelectionShuffleBag
	aggregate.Rules[0].DedupeWindow = 0
	request := testGenerateRequest(aggregate, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	request.Days = 1
	request.Candidates["rule-default"] = []Candidate{
		{MediaID: "a-1", SeriesID: "show-a", SeasonNumber: 1, EpisodeNumber: 1, Duration: 30 * time.Minute, Artwork: json.RawMessage(`{}`)},
		{MediaID: "a-2", SeriesID: "show-a", SeasonNumber: 1, EpisodeNumber: 2, Duration: 30 * time.Minute, Artwork: json.RawMessage(`{}`)},
		{MediaID: "b-1", SeriesID: "show-b", SeasonNumber: 1, EpisodeNumber: 1, Duration: 30 * time.Minute, Artwork: json.RawMessage(`{}`)},
		{MediaID: "b-2", SeriesID: "show-b", SeasonNumber: 1, EpisodeNumber: 2, Duration: 30 * time.Minute, Artwork: json.RawMessage(`{}`)},
	}
	result, err := Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("generate marathon: %v", err)
	}
	groups := make([]string, 0, 3)
	for _, entry := range result.Entries {
		if entry.Kind != EntryMedia {
			continue
		}
		var metadata struct {
			SeriesKey string `json:"seriesKey"`
		}
		if err := json.Unmarshal(entry.SelectionMetadata, &metadata); err != nil {
			t.Fatalf("decode selection metadata: %v", err)
		}
		groups = append(groups, metadata.SeriesKey)
		if len(groups) == 3 {
			break
		}
	}
	if len(groups) != 3 || groups[0] != groups[1] || groups[2] == groups[1] {
		t.Fatalf("marathon group sequence = %v, want A,A,B", groups)
	}
}

func TestTuneResolutionReauthorizesAndComputesCurrentOffset(t *testing.T) {
	start := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	entry := ScheduleEntry{
		ID: "entry-one", GenerationID: "generation-one", ChannelID: "channel-one", MediaID: "media-one",
		Kind: EntryMedia, StartsAt: start, EndsAt: start.Add(time.Hour), SourceDurationSeconds: 3600,
		Availability: "available", Artwork: json.RawMessage(`{}`), SelectionMetadata: json.RawMessage(`{}`),
		PlayoutSource: PlayoutSource{Kind: PlayoutMedia, MediaID: "media-one", DurationSeconds: 3600},
	}
	if _, err := ResolvePlayoutSource(entry, start.Add(10*time.Minute), func(string) AccessDecision { return AccessRestricted }); !errors.Is(err, ErrProgramRestricted) {
		t.Fatalf("restricted tune = %v", err)
	}
	source, err := ResolvePlayoutSource(entry, start.Add(10*time.Minute), func(string) AccessDecision { return AccessAllowed })
	if err != nil || source.OffsetSeconds != 600 {
		t.Fatalf("source=%+v err=%v", source, err)
	}
}

func testAggregate(timezone string) Aggregate {
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	channel := Channel{
		ID: "channel-one", Name: "Movie Time", Enabled: true, Timezone: timezone,
		Seed: "stable-channel-seed", DefaultRuleID: "rule-default", QualityProfile: "automatic",
		Logo:           LogoConfig{Source: LogoNone, BugCorner: LogoTopRight, BugWidthPct: 8, BugInsetPct: 2.5, BugTreatment: LogoColor},
		ConfigRevision: 1, HealthState: "pending", CreatedAt: fixed, UpdatedAt: fixed,
	}
	rule := Rule{
		ID: "rule-default", ChannelID: channel.ID, Name: "Everything", Enabled: true,
		Query:         json.RawMessage(`{"field":"entityKind","operator":"equals","value":"movie"}`),
		SelectionMode: SelectionShuffleBag, EpisodeMode: EpisodeNone, ExhaustionMode: ExhaustionLoop,
		DedupeWindow: 2, Config: json.RawMessage(`{}`), CreatedAt: fixed, UpdatedAt: fixed,
	}
	return Aggregate{Channel: channel, Rules: []Rule{rule}}
}

func testGenerateRequest(aggregate Aggregate, start time.Time) GenerateRequest {
	return GenerateRequest{
		GenerationID: "generation-one", Channel: aggregate.Channel, Rules: aggregate.Rules,
		Blocks: aggregate.Blocks, Start: start, Days: 7,
		Now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Candidates: map[string][]Candidate{
			"rule-default": {
				{MediaID: "media-a", Title: "A", Duration: 30 * time.Minute, Weight: 1, Order: 1, Artwork: json.RawMessage(`{}`)},
				{MediaID: "media-b", Title: "B", Duration: 30 * time.Minute, Weight: 1, Order: 2, Artwork: json.RawMessage(`{}`)},
				{MediaID: "media-c", Title: "C", Duration: 30 * time.Minute, Weight: 1, Order: 3, Artwork: json.RawMessage(`{}`)},
			},
		},
		InitialCursors: map[string]RuleCursor{},
	}
}

func mustLocalTime(t *testing.T, timezone string, year int, month time.Month, day, hour, minute int) time.Time {
	t.Helper()
	location, err := time.LoadLocation(timezone)
	if err != nil {
		t.Fatalf("load timezone: %v", err)
	}
	return time.Date(year, month, day, hour, minute, 0, 0, location)
}

func assertGapFree(t *testing.T, result GenerateResult) {
	t.Helper()
	if err := ValidateScheduleEntries(result.Generation.ChannelID, result.Generation.ID, result.Generation.HorizonStart, result.Generation.HorizonEnd, result.Entries); err != nil {
		t.Fatalf("schedule is not gap-free: %v", err)
	}
}
