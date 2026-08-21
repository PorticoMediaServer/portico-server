package librarychannels

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
)

type scheduleWindow struct {
	Start          time.Time
	End            time.Time
	RuleID         string
	FallbackRuleID string
	BlockID        string
	Priority       int
	Anchored       bool
	AllowOverrun   bool
	SortOrder      int
}

type localDate struct {
	Year  int
	Month time.Month
	Day   int
}

func Generate(ctx context.Context, request GenerateRequest) (GenerateResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	aggregate := Aggregate{Channel: request.Channel, Rules: request.Rules, Blocks: request.Blocks}
	if err := ValidateAggregate(aggregate); err != nil {
		return GenerateResult{}, err
	}
	if strings.TrimSpace(request.GenerationID) == "" {
		return GenerateResult{}, invalid("generationId", "is required")
	}
	if request.Start.IsZero() {
		return GenerateResult{}, invalid("start", "is required")
	}
	if err := ValidateCursorMap(request.InitialCursors); err != nil {
		return GenerateResult{}, err
	}
	days := request.Days
	if days == 0 {
		days = 7
	}
	if days < 1 || days > 7 {
		return GenerateResult{}, invalid("days", "must be between 1 and 7")
	}
	location, _ := time.LoadLocation(request.Channel.Timezone)
	horizonStart := request.Start.UTC().Truncate(time.Second)
	horizonEnd := horizonStart.In(location).AddDate(0, 0, days).UTC()
	if !horizonEnd.After(horizonStart) {
		return GenerateResult{}, invalid("start", "produced an invalid schedule horizon")
	}
	createdAt := request.Now.UTC().Truncate(time.Second)
	if request.Now.IsZero() {
		createdAt = time.Now().UTC().Truncate(time.Second)
	}

	rules := make(map[string]Rule, len(request.Rules))
	for _, rule := range request.Rules {
		if rule.Enabled {
			rules[rule.ID] = rule
		}
	}
	for ruleID, candidates := range request.Candidates {
		if _, exists := rules[ruleID]; !exists {
			continue
		}
		if len(candidates) > MaximumCandidatesPerRule {
			return GenerateResult{}, invalid("candidates."+ruleID, fmt.Sprintf("must not exceed %d items", MaximumCandidatesPerRule))
		}
		seen := make(map[string]struct{}, len(candidates))
		for index, candidate := range candidates {
			if err := validateCandidate(candidate); err != nil {
				return GenerateResult{}, fmt.Errorf("candidates[%s][%d]: %w", ruleID, index, err)
			}
			if _, duplicate := seen[candidate.MediaID]; duplicate {
				return GenerateResult{}, invalid(fmt.Sprintf("candidates[%s][%d].mediaId", ruleID, index), "must be unique within a rule")
			}
			seen[candidate.MediaID] = struct{}{}
		}
	}

	candidateHash := hashCandidates(request.Candidates, rules)
	deterministicSeed := stableHash(
		request.Channel.Seed,
		strconv.FormatInt(request.Channel.ConfigRevision, 10),
		candidateHash,
	)
	tailStart := request.TailStart.UTC().Truncate(time.Second)
	if request.TailStart.IsZero() {
		tailStart = horizonStart
	}
	if tailStart.Before(horizonStart) || !tailStart.Before(horizonEnd) {
		return GenerateResult{}, invalid("tailStart", "must fall within the requested horizon")
	}
	cursors := cloneCursors(request.InitialCursors)
	entries := make([]ScheduleEntry, 0, len(request.PreservedEntries)+days*48)
	if tailStart.After(horizonStart) {
		if len(request.PreservedEntries) == 0 {
			return GenerateResult{}, invalid("preservedEntries", "are required when extending a schedule tail")
		}
		previousEnd := horizonStart
		for index, source := range request.PreservedEntries {
			if !source.StartsAt.Equal(previousEnd) || source.EndsAt.After(tailStart) {
				return GenerateResult{}, invalid(fmt.Sprintf("preservedEntries[%d]", index), "must form a contiguous prefix ending at tailStart")
			}
			entry := source
			entry.GenerationID = request.GenerationID
			entry.ID = stableEntryID(request.Channel.ID, entry.StartsAt, entryIdentity(entry), index)
			entry.CursorAfter = cloneCursors(source.CursorAfter)
			if err := appendScheduleEntry(&entries, entry); err != nil {
				return GenerateResult{}, err
			}
			previousEnd = entry.EndsAt
		}
		if !previousEnd.Equal(tailStart) {
			return GenerateResult{}, invalid("preservedEntries", "must end exactly at tailStart")
		}
		checkpoint := entries[len(entries)-1].CursorAfter
		if len(checkpoint) == 0 {
			return GenerateResult{}, invalid("preservedEntries", "must include a cursor checkpoint at tailStart")
		}
		if err := ValidateCursorMap(checkpoint); err != nil {
			return GenerateResult{}, err
		}
		cursors = cloneCursors(checkpoint)
	} else if len(request.PreservedEntries) != 0 {
		return GenerateResult{}, invalid("preservedEntries", "must be empty when tailStart equals horizon start")
	}
	windows, err := materializeWindows(request.Channel, request.Blocks, tailStart, horizonEnd, location)
	if err != nil {
		return GenerateResult{}, err
	}
	warnings := make([]string, 0)
	warnedRules := make(map[string]struct{})
	position := tailStart

	for windowIndex, window := range windows {
		if err := ctx.Err(); err != nil {
			return GenerateResult{}, err
		}
		if !position.Before(window.End) {
			continue
		}
		if position.Before(window.Start) {
			position = window.Start
		}
		for position.Before(window.End) {
			ruleID := window.RuleID
			rule, hasRule := rules[ruleID]
			candidates := request.Candidates[ruleID]
			if (!hasRule || len(candidates) == 0) && window.FallbackRuleID != "" {
				ruleID = window.FallbackRuleID
				rule, hasRule = rules[ruleID]
				candidates = request.Candidates[ruleID]
			}
			if !hasRule || len(candidates) == 0 {
				if _, seen := warnedRules[ruleID]; !seen && ruleID != "" {
					if !contains(warnings, MessageNoPlayableCandidates) {
						warnings = append(warnings, MessageNoPlayableCandidates)
					}
					warnedRules[ruleID] = struct{}{}
				}
				if err := appendScheduleEntry(&entries, newSlateEntry(request.GenerationID, request.Channel.ID, window, position, window.End, len(entries), createdAt, cursors)); err != nil {
					return GenerateResult{}, err
				}
				position = window.End
				continue
			}

			cursorBefore := cursors[ruleID]
			fitEnd := window.End
			if window.AllowOverrun && !hasAnchoredBoundary(windows, windowIndex+1, horizonEnd) {
				fitEnd = horizonEnd
			}
			candidate, cursorAfter, selectionIndex, selectedCycle, ok := pickCandidateFitting(rule, candidates, cursorBefore, deterministicSeed+"|"+window.BlockID, fitEnd.Sub(position))
			if !ok && window.FallbackRuleID != "" && ruleID != window.FallbackRuleID {
				if fallback, exists := rules[window.FallbackRuleID]; exists {
					fallbackCandidates := request.Candidates[window.FallbackRuleID]
					fallbackCursor := cursors[window.FallbackRuleID]
					if selected, next, index, cycle, fits := pickCandidateFitting(fallback, fallbackCandidates, fallbackCursor, deterministicSeed+"|"+window.BlockID+"|fallback", fitEnd.Sub(position)); fits {
						ruleID, rule, candidate, cursorAfter, selectionIndex, selectedCycle, ok = window.FallbackRuleID, fallback, selected, next, index, cycle, true
					}
				}
			}
			if !ok {
				if err := appendScheduleEntry(&entries, newSlateEntry(request.GenerationID, request.Channel.ID, window, position, window.End, len(entries), createdAt, cursors)); err != nil {
					return GenerateResult{}, err
				}
				position = window.End
				continue
			}
			candidateEnd := position.Add(candidate.Duration)
			if candidateEnd.After(horizonEnd) {
				if err := appendScheduleEntry(&entries, newSlateEntry(request.GenerationID, request.Channel.ID, window, position, horizonEnd, len(entries), createdAt, cursors)); err != nil {
					return GenerateResult{}, err
				}
				position = horizonEnd
				break
			}
			if candidateEnd.After(window.End) {
				canOverrun := window.AllowOverrun && !hasAnchoredBoundary(windows, windowIndex+1, candidateEnd)
				if !canOverrun {
					if err := appendScheduleEntry(&entries, newSlateEntry(request.GenerationID, request.Channel.ID, window, position, window.End, len(entries), createdAt, cursors)); err != nil {
						return GenerateResult{}, err
					}
					position = window.End
					continue
				}
			}

			cursors[ruleID] = cursorAfter
			entry := newMediaEntry(
				request.GenerationID,
				request.Channel.ID,
				ruleID,
				window,
				candidate,
				position,
				candidateEnd,
				selectedCycle,
				selectionIndex,
				rule,
				len(entries),
				createdAt,
			)
			if (len(entries)+1)%ScheduleCheckpointInterval == 0 {
				entry.CursorAfter = cloneCursors(cursors)
			}
			if err := appendScheduleEntry(&entries, entry); err != nil {
				return GenerateResult{}, err
			}
			position = candidateEnd
		}
	}

	if position.Before(horizonEnd) {
		window := scheduleWindow{Start: position, End: horizonEnd}
		if err := appendScheduleEntry(&entries, newSlateEntry(request.GenerationID, request.Channel.ID, window, position, horizonEnd, len(entries), createdAt, cursors)); err != nil {
			return GenerateResult{}, err
		}
	}
	// The final boundary is always a rolling-generation checkpoint. Intermediate
	// checkpoints are sparse to avoid multiplying the complete cursor state into
	// every guide row.
	entries[len(entries)-1].CursorAfter = cloneCursors(cursors)
	if err := ValidateScheduleEntries(request.Channel.ID, request.GenerationID, horizonStart, horizonEnd, entries); err != nil {
		return GenerateResult{}, err
	}
	generation := Generation{
		ID:                request.GenerationID,
		ChannelID:         request.Channel.ID,
		ConfigRevision:    request.Channel.ConfigRevision,
		Status:            GenerationBuilding,
		HorizonStart:      horizonStart,
		HorizonEnd:        horizonEnd,
		DeterministicSeed: deterministicSeed,
		CandidateHash:     candidateHash,
		InitialCursorHash: hashCursors(effectiveInitialCursors(request, entries, tailStart)),
		Cursors:           cloneCursors(cursors),
		Warnings:          append([]string(nil), warnings...),
		CreatedAt:         createdAt,
	}
	return GenerateResult{Generation: generation, Entries: entries, NextCursors: cursors, Warnings: warnings}, nil
}

func appendScheduleEntry(entries *[]ScheduleEntry, entry ScheduleEntry) error {
	if len(*entries) >= MaximumScheduleEntries {
		return invalid("schedule.entries", fmt.Sprintf("must not exceed %d entries", MaximumScheduleEntries))
	}
	*entries = append(*entries, entry)
	return nil
}

func ValidateScheduleEntries(channelID, generationID string, horizonStart, horizonEnd time.Time, entries []ScheduleEntry) error {
	if !horizonEnd.After(horizonStart) {
		return invalid("schedule.horizon", "must have a positive duration")
	}
	if len(entries) == 0 {
		return invalid("schedule.entries", "must cover the requested horizon")
	}
	previousEnd := horizonStart
	ids := make(map[string]struct{}, len(entries))
	for index, entry := range entries {
		if entry.ChannelID != channelID || entry.GenerationID != generationID {
			return invalid(fmt.Sprintf("schedule.entries[%d]", index), "belongs to another channel or generation")
		}
		if _, exists := ids[entry.ID]; exists {
			return invalid(fmt.Sprintf("schedule.entries[%d].id", index), "must be unique")
		}
		ids[entry.ID] = struct{}{}
		if !entry.EndsAt.After(entry.StartsAt) {
			return invalid(fmt.Sprintf("schedule.entries[%d]", index), "must have a positive duration")
		}
		if entry.StartsAt.Before(previousEnd) {
			return invalid(fmt.Sprintf("schedule.entries[%d]", index), "overlaps the preceding entry")
		}
		if entry.StartsAt.After(previousEnd) {
			return invalid(fmt.Sprintf("schedule.entries[%d]", index), "leaves an unscheduled gap")
		}
		if err := validateEntryContract(entry, index); err != nil {
			return err
		}
		if entry.CycleNumber > math.MaxInt64 {
			return invalid(fmt.Sprintf("schedule.entries[%d].cycleNumber", index), "is too large")
		}
		if err := ValidateCursorMap(entry.CursorAfter); err != nil {
			return fmt.Errorf("schedule.entries[%d].cursorAfter: %w", index, err)
		}
		if err := validatePlayoutSource(entry); err != nil {
			return fmt.Errorf("schedule.entries[%d].playoutSource: %w", index, err)
		}
		previousEnd = entry.EndsAt
	}
	if !previousEnd.Equal(horizonEnd) {
		return invalid("schedule.entries", "must end at the requested horizon")
	}
	return nil
}

func validateEntryContract(entry ScheduleEntry, index int) error {
	path := fmt.Sprintf("schedule.entries[%d]", index)
	if !validOpaqueID(entry.ID) || !validOpaqueID(entry.ChannelID) || !validOpaqueID(entry.GenerationID) {
		return invalid(path, "contains an invalid identifier")
	}
	if entry.StartsAt.Nanosecond() != 0 || entry.EndsAt.Nanosecond() != 0 {
		return invalid(path, "timestamps must use whole seconds")
	}
	if entry.MediaOffsetSeconds < 0 || entry.SourceDurationSeconds < 0 || entry.SelectionIndex < 0 {
		return invalid(path, "contains a negative offset, duration, or selection index")
	}
	if len(entry.Title) > 500 || len(entry.Subtitle) > 500 || len(entry.Summary) > 10_000 || len(entry.ContentRating) > 100 {
		return invalid(path, "metadata exceeds schedule limits")
	}
	if len(entry.Artwork) > MaximumScheduleArtworkBytes || !validJSONObject(entry.Artwork) {
		return invalid(path+".artwork", "must be a JSON object within the schedule artwork limit")
	}
	if len(entry.SelectionMetadata) > 64*1024 || !validJSONObject(entry.SelectionMetadata) {
		return invalid(path+".selectionMetadata", "must be a JSON object no larger than 64 KiB")
	}
	durationSeconds := int(entry.EndsAt.Sub(entry.StartsAt) / time.Second)
	switch entry.Kind {
	case EntryMedia:
		if !validOpaqueID(entry.MediaID) || entry.Availability != "available" || entry.ReasonCode != "" || entry.SourceDurationSeconds <= 0 {
			return invalid(path, "media entries require available media and no failure reason")
		}
		if entry.MediaOffsetSeconds+durationSeconds > entry.SourceDurationSeconds {
			return invalid(path, "media entry exceeds its source duration")
		}
	case EntrySlate:
		if entry.MediaID != "" || entry.Availability != "available" || entry.ReasonCode != ReasonNoScheduledMedia || entry.SourceDurationSeconds != 0 || entry.MediaOffsetSeconds != 0 {
			return invalid(path, "slate entries must not identify media")
		}
	case EntryUnavailable:
		if entry.MediaID != "" || entry.MediaOffsetSeconds != 0 || (entry.Availability != "missing" && entry.Availability != "unavailable" && entry.Availability != "restricted") || (entry.ReasonCode != ReasonMediaUnavailable && entry.ReasonCode != ReasonMediaRestricted) {
			return invalid(path, "unavailable entries must be fully redacted and carry a canonical reason")
		}
	default:
		return invalid(path+".kind", "is not supported")
	}
	return nil
}

func validJSONObject(raw json.RawMessage) bool {
	if len(raw) == 0 || !json.Valid(raw) {
		return false
	}
	var object map[string]any
	return json.Unmarshal(raw, &object) == nil && object != nil
}

func materializeWindows(channel Channel, blocks []WeeklyBlock, horizonStart, horizonEnd time.Time, location *time.Location) ([]scheduleWindow, error) {
	raw := make([]scheduleWindow, 0, len(blocks)*8)
	startDate := dateOf(horizonStart.In(location))
	startDate = addDays(startDate, -1)
	endDate := dateOf(horizonEnd.In(location))
	for date := startDate; !dateAfter(date, addDays(endDate, 1)); date = addDays(date, 1) {
		weekday := time.Date(date.Year, date.Month, date.Day, 12, 0, 0, 0, location).Weekday()
		for _, block := range blocks {
			if !block.Enabled || !block.Weekdays.Includes(weekday) {
				continue
			}
			endDateForBlock := date
			if block.EndMinute <= block.StartMinute {
				endDateForBlock = addDays(date, 1)
			}
			start, err := resolveWallMinute(date, block.StartMinute, location, false)
			if err != nil {
				return nil, fmt.Errorf("block %s start: %w", block.ID, err)
			}
			end, err := resolveWallMinute(endDateForBlock, block.EndMinute, location, true)
			if err != nil {
				return nil, fmt.Errorf("block %s end: %w", block.ID, err)
			}
			start, end = maxTime(start, horizonStart), minTime(end, horizonEnd)
			if !end.After(start) {
				continue
			}
			raw = append(raw, scheduleWindow{
				Start: start, End: end, RuleID: block.RuleID, FallbackRuleID: block.FallbackRuleID,
				BlockID: block.ID, Priority: block.Priority, Anchored: block.Anchored,
				AllowOverrun: block.AllowOverrun, SortOrder: block.SortOrder,
			})
		}
	}

	boundaries := []time.Time{horizonStart, horizonEnd}
	for _, window := range raw {
		boundaries = append(boundaries, window.Start, window.End)
	}
	sort.Slice(boundaries, func(i, j int) bool { return boundaries[i].Before(boundaries[j]) })
	unique := boundaries[:0]
	for _, boundary := range boundaries {
		if len(unique) == 0 || !boundary.Equal(unique[len(unique)-1]) {
			unique = append(unique, boundary)
		}
	}

	resolved := make([]scheduleWindow, 0, len(unique))
	for index := 0; index+1 < len(unique); index++ {
		segmentStart, segmentEnd := unique[index], unique[index+1]
		if !segmentEnd.After(segmentStart) {
			continue
		}
		winner := scheduleWindow{
			Start: segmentStart, End: segmentEnd, RuleID: channel.DefaultRuleID,
			AllowOverrun: true, Priority: -1 << 30,
		}
		found := false
		for _, candidate := range raw {
			if candidate.Start.After(segmentStart) || candidate.End.Before(segmentEnd) {
				continue
			}
			if !found || windowPrecedes(candidate, winner) {
				winner = candidate
				winner.Start, winner.End = segmentStart, segmentEnd
				found = true
			}
		}
		if len(resolved) > 0 && sameWindowPolicy(resolved[len(resolved)-1], winner) && resolved[len(resolved)-1].End.Equal(segmentStart) {
			resolved[len(resolved)-1].End = segmentEnd
		} else {
			resolved = append(resolved, winner)
		}
	}
	return resolved, nil
}

func windowPrecedes(left, right scheduleWindow) bool {
	if left.Priority != right.Priority {
		return left.Priority > right.Priority
	}
	if left.Anchored != right.Anchored {
		return left.Anchored
	}
	if left.SortOrder != right.SortOrder {
		return left.SortOrder < right.SortOrder
	}
	return left.BlockID < right.BlockID
}

func sameWindowPolicy(left, right scheduleWindow) bool {
	return left.RuleID == right.RuleID && left.FallbackRuleID == right.FallbackRuleID &&
		left.BlockID == right.BlockID && left.Priority == right.Priority &&
		left.Anchored == right.Anchored && left.AllowOverrun == right.AllowOverrun
}

func hasAnchoredBoundary(windows []scheduleWindow, startIndex int, end time.Time) bool {
	for index := startIndex; index < len(windows); index++ {
		if !windows[index].Start.Before(end) {
			return false
		}
		if windows[index].Anchored {
			return true
		}
	}
	return false
}

func pickCandidate(rule Rule, source []Candidate, cursor RuleCursor, seed string) (Candidate, RuleCursor, int, uint64, bool) {
	candidates := prepareCandidates(rule, source, &cursor, seed)
	if len(candidates) == 0 {
		return Candidate{}, cursor, 0, 0, false
	}
	limit := len(candidates)*3 + 1
	for attempts := 0; attempts < limit; attempts++ {
		var candidate Candidate
		var selectionIndex int
		var selectedCycle uint64
		selectionMode := rule.SelectionMode
		if rule.EpisodeMode == EpisodeInOrder || rule.EpisodeMode == EpisodeMarathon {
			selectionMode = SelectionSequential
		} else if rule.EpisodeMode == EpisodeRandomized && selectionMode == SelectionSequential {
			selectionMode = SelectionShuffleBag
		}
		switch selectionMode {
		case SelectionWeightedRandom:
			selectedCycle = cursor.Cycle
			selectionIndex = weightedIndex(candidates, stableHash(seed, rule.ID, strconv.FormatUint(cursor.Cycle, 10)))
			candidate = candidates[selectionIndex]
			cursor.Cycle++
		case SelectionShuffleBag:
			if cursor.Position >= len(candidates) {
				if rule.ExhaustionMode != ExhaustionLoop {
					return Candidate{}, cursor, 0, 0, false
				}
				cursor.Cycle++
				cursor.Position = 0
			}
			order := deterministicPermutation(len(candidates), stableHash(seed, rule.ID, strconv.FormatUint(cursor.Cycle, 10)))
			selectedCycle = cursor.Cycle
			selectionIndex = order[cursor.Position]
			candidate = candidates[selectionIndex]
			cursor.Position++
		case SelectionSequential:
			if cursor.Position >= len(candidates) {
				if rule.ExhaustionMode != ExhaustionLoop {
					return Candidate{}, cursor, 0, 0, false
				}
				cursor.Cycle++
				cursor.Position = 0
				if rule.EpisodeMode == EpisodeMarathon {
					advanceMarathonGroup(source, &cursor, seed, rule.ID)
					candidates = prepareCandidates(rule, source, &cursor, seed)
					if len(candidates) == 0 {
						return Candidate{}, cursor, 0, 0, false
					}
				}
			}
			selectionIndex = cursor.Position
			selectedCycle = cursor.Cycle
			candidate = candidates[selectionIndex]
			cursor.Position++
		default:
			return Candidate{}, cursor, 0, 0, false
		}
		mediaKey := mediaCursorKey(candidate.MediaID)
		if contains(cursor.RecentMedia, mediaKey) && len(cursor.RecentMedia) < len(candidates) {
			continue
		}
		group := candidate.SeriesID
		if group == "" {
			group = candidate.MediaID
		}
		groupKey := groupCursorKey(group)
		if rule.MaxConsecutive > 0 && cursor.LastGroup == groupKey && cursor.Consecutive >= rule.MaxConsecutive && hasAlternativeGroup(candidates, groupKey) {
			continue
		}
		cursor.RecentMedia = appendRecent(cursor.RecentMedia, mediaKey, rule.DedupeWindow)
		if cursor.LastGroup == groupKey {
			cursor.Consecutive++
		} else {
			cursor.LastGroup = groupKey
			cursor.Consecutive = 1
		}
		return candidate, cursor, selectionIndex, selectedCycle, true
	}
	return Candidate{}, cursor, 0, 0, false
}

// pickCandidateFitting keeps deterministic selection order but considers
// shorter alternatives before emitting slate at an anchored boundary.
func pickCandidateFitting(rule Rule, source []Candidate, cursor RuleCursor, seed string, maximum time.Duration) (Candidate, RuleCursor, int, uint64, bool) {
	if maximum <= 0 {
		return Candidate{}, cursor, 0, 0, false
	}
	probe := cursor
	for attempts := 0; attempts < len(source); attempts++ {
		candidate, next, index, cycle, ok := pickCandidate(rule, source, probe, seed)
		if !ok {
			return Candidate{}, cursor, 0, 0, false
		}
		if candidate.Duration <= maximum {
			return candidate, next, index, cycle, true
		}
		probe = next
	}
	// Weighted sampling may legally repeat an item. A final deterministic scan
	// guarantees that a shorter program is not missed merely because the
	// weighted draws all landed on an oversized item.
	if rule.SelectionMode == SelectionWeightedRandom && rule.EpisodeMode != EpisodeInOrder && rule.EpisodeMode != EpisodeMarathon {
		preparedCursor := cursor
		prepared := prepareCandidates(rule, source, &preparedCursor, seed)
		fitting := make([]int, 0, len(prepared))
		for index, candidate := range prepared {
			mediaKey := mediaCursorKey(candidate.MediaID)
			groupKey := groupCursorKey(candidateGroup(candidate))
			deduped := contains(preparedCursor.RecentMedia, mediaKey) && len(preparedCursor.RecentMedia) < len(prepared)
			consecutive := rule.MaxConsecutive > 0 && preparedCursor.LastGroup == groupKey && preparedCursor.Consecutive >= rule.MaxConsecutive && hasAlternativeGroup(prepared, groupKey)
			if candidate.Duration <= maximum && !deduped && !consecutive {
				fitting = append(fitting, index)
			}
		}
		if len(fitting) > 0 {
			choice := fitting[deterministicIndex(stableHash(seed, "weighted-fit", strconv.FormatUint(cursor.Cycle, 10)), 0, len(fitting))]
			candidate := prepared[choice]
			next := preparedCursor
			cycle := next.Cycle
			next.Cycle++
			next.RecentMedia = appendRecent(next.RecentMedia, mediaCursorKey(candidate.MediaID), rule.DedupeWindow)
			group := groupCursorKey(candidateGroup(candidate))
			if next.LastGroup == group {
				next.Consecutive++
			} else {
				next.LastGroup, next.Consecutive = group, 1
			}
			return candidate, next, choice, cycle, true
		}
	}
	return Candidate{}, cursor, 0, 0, false
}

func prepareCandidates(rule Rule, source []Candidate, cursor *RuleCursor, seed string) []Candidate {
	candidates := make([]Candidate, 0, len(source))
	for _, candidate := range source {
		if candidate.Availability == "" || candidate.Availability == "available" {
			candidates = append(candidates, candidate)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if rule.EpisodeMode == EpisodeInOrder {
			// Ordinary ordered television interleaves series while preserving each
			// series' episode order. Only EpisodeMarathon holds one series.
			if candidates[i].SeasonNumber != candidates[j].SeasonNumber {
				return candidates[i].SeasonNumber < candidates[j].SeasonNumber
			}
			if candidates[i].EpisodeNumber != candidates[j].EpisodeNumber {
				return candidates[i].EpisodeNumber < candidates[j].EpisodeNumber
			}
			if candidates[i].SeriesID != candidates[j].SeriesID {
				return candidates[i].SeriesID < candidates[j].SeriesID
			}
		} else if rule.EpisodeMode == EpisodeMarathon {
			if candidates[i].SeriesID != candidates[j].SeriesID {
				return candidates[i].SeriesID < candidates[j].SeriesID
			}
			if candidates[i].SeasonNumber != candidates[j].SeasonNumber {
				return candidates[i].SeasonNumber < candidates[j].SeasonNumber
			}
			if candidates[i].EpisodeNumber != candidates[j].EpisodeNumber {
				return candidates[i].EpisodeNumber < candidates[j].EpisodeNumber
			}
		}
		if candidates[i].Order != candidates[j].Order {
			return candidates[i].Order < candidates[j].Order
		}
		return candidates[i].MediaID < candidates[j].MediaID
	})
	if rule.EpisodeMode != EpisodeMarathon {
		return candidates
	}
	groups := make([]string, 0)
	seen := make(map[string]struct{})
	for _, candidate := range candidates {
		if candidate.SeriesID == "" {
			continue
		}
		if _, ok := seen[candidate.SeriesID]; !ok {
			seen[candidate.SeriesID] = struct{}{}
			groups = append(groups, candidate.SeriesID)
		}
	}
	if len(groups) == 0 {
		return candidates
	}
	if cursor.SelectedGroup == "" || !containsGroupKey(groups, cursor.SelectedGroup) {
		cursor.SelectedGroup = groupCursorKey(groups[deterministicIndex(stableHash(seed, rule.ID, "marathon"), 0, len(groups))])
		cursor.Position = 0
		cursor.Cycle = 0
	}
	filtered := candidates[:0]
	for _, candidate := range candidates {
		if groupCursorKey(candidate.SeriesID) == cursor.SelectedGroup {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func advanceMarathonGroup(source []Candidate, cursor *RuleCursor, seed, ruleID string) {
	groups := make([]string, 0)
	seen := make(map[string]struct{})
	for _, candidate := range source {
		if candidate.SeriesID == "" {
			continue
		}
		if _, exists := seen[candidate.SeriesID]; exists {
			continue
		}
		seen[candidate.SeriesID] = struct{}{}
		groups = append(groups, candidate.SeriesID)
	}
	if len(groups) < 2 {
		return
	}
	sort.Strings(groups)
	order := deterministicPermutation(len(groups), stableHash(seed, ruleID, "marathon-groups"))
	current := -1
	for index, groupIndex := range order {
		if groupCursorKey(groups[groupIndex]) == cursor.SelectedGroup {
			current = index
			break
		}
	}
	if current < 0 {
		cursor.SelectedGroup = groupCursorKey(groups[order[0]])
		return
	}
	cursor.SelectedGroup = groupCursorKey(groups[order[(current+1)%len(order)]])
}

func validateCandidate(candidate Candidate) error {
	if !validOpaqueID(candidate.MediaID) {
		return invalid("mediaId", "must be an opaque identifier")
	}
	if candidate.Duration < MinimumCandidateDuration || candidate.Duration > MaximumCandidateDuration || candidate.Duration%time.Second != 0 {
		return invalid("duration", fmt.Sprintf("must be a whole number of seconds between %s and %s", MinimumCandidateDuration, MaximumCandidateDuration))
	}
	if math.IsNaN(candidate.Weight) || math.IsInf(candidate.Weight, 0) || candidate.Weight < 0 || candidate.Weight > 1_000_000 {
		return invalid("weight", "must be a finite number between 0 and 1000000")
	}
	if len(candidate.Title) > 500 || len(candidate.Subtitle) > 500 || len(candidate.Summary) > 10_000 || len(candidate.ContentRating) > 100 || (candidate.SeriesID != "" && !validOpaqueID(candidate.SeriesID)) {
		return invalid("metadata", "exceeds schedule metadata limits")
	}
	if candidate.SeasonNumber < 0 || candidate.SeasonNumber > 100_000 || candidate.EpisodeNumber < 0 || candidate.EpisodeNumber > 1_000_000 {
		return invalid("episode", "contains an invalid season or episode number")
	}
	if len(candidate.Artwork) > MaximumScheduleArtworkBytes {
		return invalid("artwork", fmt.Sprintf("must not exceed %d KiB", MaximumScheduleArtworkBytes/1024))
	}
	if len(candidate.Artwork) > 0 && !validJSONObject(candidate.Artwork) {
		return invalid("artwork", "must be a JSON object")
	}
	switch candidate.Availability {
	case "", "available", "missing", "unavailable":
	default:
		return invalid("availability", "is not supported")
	}
	return nil
}

func newMediaEntry(generationID, channelID, ruleID string, window scheduleWindow, candidate Candidate, start, end time.Time, cycle uint64, selectionIndex int, rule Rule, entryIndex int, now time.Time) ScheduleEntry {
	kind := EntryMedia
	availability := candidate.Availability
	if availability == "" {
		availability = "available"
	}
	if availability != "available" {
		kind = EntryUnavailable
	}
	artwork := candidate.Artwork
	if len(artwork) == 0 {
		artwork = json.RawMessage(`{}`)
	}
	metadata, _ := json.Marshal(map[string]any{
		"selectionMode": string(rule.SelectionMode),
		"episodeMode":   string(rule.EpisodeMode),
		"seriesKey":     groupCursorKey(candidateGroup(candidate)),
		"seasonNumber":  candidate.SeasonNumber,
		"episodeNumber": candidate.EpisodeNumber,
	})
	reason := ScheduleReason("")
	playout := PlayoutSource{Kind: PlayoutMedia, MediaID: candidate.MediaID, DurationSeconds: int(candidate.Duration / time.Second)}
	if kind == EntryUnavailable {
		reason = ReasonMediaUnavailable
		playout = PlayoutSource{Kind: PlayoutUnavailable, DurationSeconds: int(end.Sub(start) / time.Second)}
	}
	return ScheduleEntry{
		ID: stableEntryID(channelID, start, candidate.MediaID, entryIndex), GenerationID: generationID,
		ChannelID: channelID, RuleID: ruleID, BlockID: window.BlockID, MediaID: candidate.MediaID,
		Kind: kind, StartsAt: start, EndsAt: end, SourceDurationSeconds: int(candidate.Duration / time.Second),
		CycleNumber: cycle, SelectionIndex: selectionIndex, Title: candidate.Title, Subtitle: candidate.Subtitle,
		Summary: candidate.Summary, ContentRating: candidate.ContentRating, Artwork: artwork,
		Availability: availability, SelectionMetadata: metadata, ReasonCode: reason, PlayoutSource: playout,
		CreatedAt: now,
	}
}

func hasAlternativeGroup(candidates []Candidate, current string) bool {
	for _, candidate := range candidates {
		group := groupCursorKey(candidateGroup(candidate))
		if group != current {
			return true
		}
	}
	return false
}

func newSlateEntry(generationID, channelID string, window scheduleWindow, start, end time.Time, entryIndex int, now time.Time, cursors map[string]RuleCursor) ScheduleEntry {
	duration := int(end.Sub(start) / time.Second)
	entry := ScheduleEntry{
		ID: stableEntryID(channelID, start, "slate", entryIndex), GenerationID: generationID, ChannelID: channelID,
		RuleID: window.RuleID, BlockID: window.BlockID, Kind: EntrySlate, StartsAt: start, EndsAt: end,
		Artwork: json.RawMessage(`{}`), SelectionMetadata: json.RawMessage(`{}`), Availability: "available", ReasonCode: ReasonNoScheduledMedia,
		PlayoutSource: PlayoutSource{Kind: PlayoutGeneratedSlate, DurationSeconds: duration, GeneratorID: "portico-default-slate-v1"},
		CreatedAt:     now,
	}
	if (entryIndex+1)%ScheduleCheckpointInterval == 0 {
		entry.CursorAfter = cloneCursors(cursors)
	}
	return entry
}

func candidateGroup(candidate Candidate) string {
	if candidate.SeriesID != "" {
		return candidate.SeriesID
	}
	return candidate.MediaID
}

func mediaCursorKey(mediaID string) string { return stableHash("media", mediaID) }
func groupCursorKey(groupID string) string { return stableHash("group", groupID) }

func containsGroupKey(groups []string, key string) bool {
	for _, group := range groups {
		if groupCursorKey(group) == key {
			return true
		}
	}
	return false
}

func hashCandidates(candidateMap map[string][]Candidate, enabledRules map[string]Rule) string {
	ruleIDs := make([]string, 0, len(enabledRules))
	for ruleID := range enabledRules {
		ruleIDs = append(ruleIDs, ruleID)
	}
	sort.Strings(ruleIDs)
	parts := make([]string, 0)
	for _, ruleID := range ruleIDs {
		candidates := append([]Candidate(nil), candidateMap[ruleID]...)
		sort.Slice(candidates, func(i, j int) bool {
			if candidates[i].MediaID != candidates[j].MediaID {
				return candidates[i].MediaID < candidates[j].MediaID
			}
			return candidates[i].Order < candidates[j].Order
		})
		parts = append(parts, ruleID)
		for _, candidate := range candidates {
			parts = append(parts, candidate.MediaID, candidate.Title, candidate.Subtitle, candidate.Summary,
				candidate.ContentRating, string(candidate.Artwork), strconv.FormatInt(int64(candidate.Duration), 10),
				strconv.FormatFloat(candidate.Weight, 'g', -1, 64), strconv.FormatInt(candidate.Order, 10),
				candidate.SeriesID, strconv.Itoa(candidate.SeasonNumber), strconv.Itoa(candidate.EpisodeNumber), candidate.Availability)
		}
	}
	return stableHash(parts...)
}

func hashCursors(cursors map[string]RuleCursor) string {
	encoded, _ := json.Marshal(cursors)
	return stableHash(string(encoded))
}

func effectiveInitialCursors(request GenerateRequest, entries []ScheduleEntry, tailStart time.Time) map[string]RuleCursor {
	if tailStart.Equal(request.Start.UTC().Truncate(time.Second)) || len(entries) == 0 {
		return request.InitialCursors
	}
	for index := len(entries) - 1; index >= 0; index-- {
		if entries[index].EndsAt.Equal(tailStart) {
			return entries[index].CursorAfter
		}
	}
	return request.InitialCursors
}

func validatePlayoutSource(entry ScheduleEntry) error {
	source := entry.PlayoutSource
	if source.DurationSeconds <= 0 || source.OffsetSeconds < 0 {
		return invalid("", "must have a positive duration and non-negative offset")
	}
	switch source.Kind {
	case PlayoutMedia:
		if entry.Kind != EntryMedia || source.MediaID == "" || source.MediaID != entry.MediaID || source.GeneratorID != "" || source.OffsetSeconds != entry.MediaOffsetSeconds || source.DurationSeconds != entry.SourceDurationSeconds {
			return invalid("", "media source must identify the entry media")
		}
	case PlayoutGeneratedSlate:
		if entry.Kind != EntrySlate || source.MediaID != "" || !validAssetID(source.GeneratorID) {
			return invalid("", "generated slate must identify an opaque renderer")
		}
	case PlayoutUnavailable:
		if entry.Kind != EntryUnavailable || source.MediaID != "" || source.GeneratorID != "" {
			return invalid("", "unavailable source must not identify playable media")
		}
	default:
		return invalid("", "kind is not supported")
	}
	return nil
}

func entryIdentity(entry ScheduleEntry) string {
	if entry.MediaID != "" {
		return entry.MediaID
	}
	return string(entry.Kind) + ":" + string(entry.ReasonCode)
}

// ProjectSchedule preserves the global timeline while redacting all private
// media metadata for a profile which cannot access the underlying item.
func ProjectSchedule(entries []ScheduleEntry, decide func(mediaID string) AccessDecision) []ScheduleEntry {
	projected := make([]ScheduleEntry, len(entries))
	for i, source := range entries {
		entry := source
		entry.Artwork = append(json.RawMessage(nil), source.Artwork...)
		entry.SelectionMetadata = append(json.RawMessage(nil), source.SelectionMetadata...)
		// Cursor checkpoints are scheduler state and are never part of a profile
		// projection, even when every program is authorized.
		entry.CursorAfter = nil
		if entry.Kind == EntryUnavailable || entry.MediaID != "" {
			decision := AccessUnavailable
			if entry.Kind == EntryUnavailable && entry.Availability == "restricted" {
				decision = AccessRestricted
			}
			if entry.MediaID != "" && decide != nil {
				decision = decide(entry.MediaID)
			}
			if decision != AccessAllowed {
				entry.MediaID, entry.Title, entry.Subtitle, entry.Summary, entry.ContentRating = "", "", "", "", ""
				entry.Artwork, entry.SelectionMetadata = json.RawMessage(`{}`), json.RawMessage(`{}`)
				entry.Kind, entry.Availability = EntryUnavailable, "restricted"
				entry.ReasonCode = ReasonMediaRestricted
				if decision == AccessUnavailable {
					entry.Availability, entry.ReasonCode = "unavailable", ReasonMediaUnavailable
				}
				entry.PlayoutSource = PlayoutSource{Kind: PlayoutUnavailable, DurationSeconds: int(entry.EndsAt.Sub(entry.StartsAt) / time.Second)}
			}
		}
		projected[i] = entry
	}
	return projected
}

func stableHash(parts ...string) string {
	hash := sha256.New()
	for _, part := range parts {
		_, _ = hash.Write([]byte(strconv.Itoa(len(part))))
		_, _ = hash.Write([]byte{':'})
		_, _ = hash.Write([]byte(part))
		_, _ = hash.Write([]byte{'|'})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func stableEntryID(channelID string, start time.Time, identity string, _ int) string {
	return "lce_" + stableHash(channelID, start.UTC().Format(time.RFC3339Nano), identity)[:24]
}

func deterministicPermutation(length int, seed string) []int {
	order := make([]int, length)
	for index := range order {
		order[index] = index
	}
	for index := length - 1; index > 0; index-- {
		other := deterministicIndex(seed, length-index, index+1)
		order[index], order[other] = order[other], order[index]
	}
	return order
}

func deterministicIndex(seed string, counter, length int) int {
	if length <= 1 {
		return 0
	}
	sum := sha256.Sum256([]byte(seed + "|" + strconv.Itoa(counter)))
	return int(binary.BigEndian.Uint64(sum[:8]) % uint64(length))
}

func weightedIndex(candidates []Candidate, seed string) int {
	total := 0.0
	for _, candidate := range candidates {
		weight := candidate.Weight
		if weight <= 0 {
			weight = 1
		}
		total += weight
	}
	sum := sha256.Sum256([]byte(seed))
	unit := float64(binary.BigEndian.Uint64(sum[:8])) / float64(^uint64(0))
	target := unit * total
	for index, candidate := range candidates {
		weight := candidate.Weight
		if weight <= 0 {
			weight = 1
		}
		if target < weight {
			return index
		}
		target -= weight
	}
	return len(candidates) - 1
}

func appendRecent(recent []string, mediaID string, limit int) []string {
	if limit <= 0 {
		return nil
	}
	result := make([]string, 0, len(recent)+1)
	for _, existing := range recent {
		if existing != mediaID {
			result = append(result, existing)
		}
	}
	recent = append(result, mediaID)
	if len(recent) > limit {
		recent = recent[len(recent)-limit:]
	}
	return recent
}

func cloneCursors(source map[string]RuleCursor) map[string]RuleCursor {
	result := make(map[string]RuleCursor, len(source))
	for id, cursor := range source {
		cursor.RecentMedia = append([]string(nil), cursor.RecentMedia...)
		result[id] = cursor
	}
	return result
}

func resolveWallMinute(date localDate, minute int, location *time.Location, endBoundary bool) (time.Time, error) {
	approximate := time.Date(date.Year, date.Month, date.Day, minute/60, minute%60, 0, 0, location).UTC()
	scanStart := approximate.Add(-4 * time.Hour).Truncate(time.Minute)
	scanEnd := approximate.Add(5 * time.Hour)
	matches := make([]time.Time, 0, 2)
	var firstAfter time.Time
	for instant := scanStart; !instant.After(scanEnd); instant = instant.Add(time.Minute) {
		local := instant.In(location)
		if local.Year() != date.Year || local.Month() != date.Month || local.Day() != date.Day {
			continue
		}
		wallMinute := local.Hour()*60 + local.Minute()
		if wallMinute == minute {
			matches = append(matches, instant)
		}
		if wallMinute > minute && firstAfter.IsZero() {
			firstAfter = instant
		}
	}
	if len(matches) > 0 {
		if endBoundary {
			return matches[len(matches)-1], nil
		}
		return matches[0], nil
	}
	if !firstAfter.IsZero() {
		return firstAfter, nil
	}
	return time.Time{}, fmt.Errorf("could not resolve %04d-%02d-%02d %02d:%02d in %s", date.Year, date.Month, date.Day, minute/60, minute%60, location)
}

func dateOf(value time.Time) localDate {
	return localDate{Year: value.Year(), Month: value.Month(), Day: value.Day()}
}

func addDays(date localDate, count int) localDate {
	value := time.Date(date.Year, date.Month, date.Day+count, 12, 0, 0, 0, time.UTC)
	return dateOf(value)
}

func dateAfter(left, right localDate) bool {
	return time.Date(left.Year, left.Month, left.Day, 0, 0, 0, 0, time.UTC).After(time.Date(right.Year, right.Month, right.Day, 0, 0, 0, 0, time.UTC))
}

func minTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func maxTime(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}

func contains[T comparable](values []T, target T) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
