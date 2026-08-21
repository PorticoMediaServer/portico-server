package librarychannels

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/PorticoMediaServer/portico-server/internal/browsecontract"
)

var (
	ErrNotFound             = errors.New("library channel not found")
	ErrRevisionConflict     = errors.New("library channel configuration revision conflict")
	ErrTemplateExists       = errors.New("library channel template already restored")
	ErrGenerationStale      = errors.New("library channel generation is stale")
	ErrGenerationInProgress = errors.New("library channel generation is already in progress")
	ErrNoPlayableSchedule   = errors.New("library channel generation contains no playable media")
	ErrProgramRestricted    = errors.New("library channel program is restricted")
	ErrProgramUnavailable   = errors.New("library channel program is unavailable")
)

const (
	MaximumRulesPerChannel      = 64
	MaximumBlocksPerChannel     = 256
	MaximumCandidatesPerRule    = 20_000
	MaximumScheduleEntries      = 20_000
	MaximumRecentMediaPerRule   = 250
	ScheduleCheckpointInterval  = 64
	MinimumCandidateDuration    = time.Second
	MaximumCandidateDuration    = 48 * time.Hour
	MaximumScheduleArtworkBytes = 64 * 1024
)

type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string {
	if e.Field == "" {
		return e.Message
	}
	return e.Field + ": " + e.Message
}

func invalid(field, message string) error { return &ValidationError{Field: field, Message: message} }

func ValidateAggregate(aggregate Aggregate) error {
	if err := ValidateChannel(aggregate.Channel); err != nil {
		return err
	}
	if len(aggregate.Rules) == 0 || len(aggregate.Rules) > MaximumRulesPerChannel {
		return invalid("rules", fmt.Sprintf("must contain between 1 and %d rules", MaximumRulesPerChannel))
	}
	if len(aggregate.Blocks) > MaximumBlocksPerChannel {
		return invalid("blocks", fmt.Sprintf("must not exceed %d blocks", MaximumBlocksPerChannel))
	}
	rules := make(map[string]Rule, len(aggregate.Rules))
	for i, rule := range aggregate.Rules {
		if err := ValidateRule(rule, aggregate.Channel.ID); err != nil {
			return fmt.Errorf("rules[%d]: %w", i, err)
		}
		if _, exists := rules[rule.ID]; exists {
			return invalid(fmt.Sprintf("rules[%d].id", i), "must be unique")
		}
		rules[rule.ID] = rule
	}
	if aggregate.Channel.DefaultRuleID != "" {
		if _, ok := rules[aggregate.Channel.DefaultRuleID]; !ok {
			return invalid("channel.defaultRuleId", "must identify a rule in this channel")
		}
	}
	blocks := make(map[string]struct{}, len(aggregate.Blocks))
	for i, block := range aggregate.Blocks {
		if err := ValidateBlock(block, aggregate.Channel.ID, rules); err != nil {
			return fmt.Errorf("blocks[%d]: %w", i, err)
		}
		if _, exists := blocks[block.ID]; exists {
			return invalid(fmt.Sprintf("blocks[%d].id", i), "must be unique")
		}
		blocks[block.ID] = struct{}{}
	}
	return nil
}

func ValidateChannel(channel Channel) error {
	if !validOpaqueID(channel.ID) {
		return invalid("channel.id", "must be an opaque identifier no longer than 128 characters")
	}
	if n := strings.TrimSpace(channel.Name); n == "" || utf8.RuneCountInString(n) > 120 {
		return invalid("channel.name", "must contain between 1 and 120 characters")
	}
	if utf8.RuneCountInString(channel.Description) > 1000 {
		return invalid("channel.description", "must not exceed 1000 characters")
	}
	if strings.TrimSpace(channel.Seed) == "" || utf8.RuneCountInString(channel.Seed) > 256 {
		return invalid("channel.seed", "must contain between 1 and 256 characters")
	}
	if channel.ConfigRevision < 1 {
		return invalid("channel.configRevision", "must be positive")
	}
	if channel.TemplateVersion < 0 {
		return invalid("channel.templateVersion", "must not be negative")
	}
	if channel.QualityProfile == "" {
		return invalid("channel.qualityProfile", "is required")
	}
	if channel.Timezone == "" || channel.Timezone == "Local" {
		return invalid("channel.timezone", "must be an explicit IANA time zone")
	}
	if _, err := time.LoadLocation(channel.Timezone); err != nil {
		return invalid("channel.timezone", "must be a recognized IANA time zone")
	}
	switch channel.HealthState {
	case "", "pending", "healthy", "warning", "error", "disabled":
	default:
		return invalid("channel.healthState", "is not supported")
	}
	if channel.HealthMessage != "" && !isKnownLibraryChannelMessage(channel.HealthMessage) {
		return invalid("channel.healthMessage", "must be a known Library Channel product-language key")
	}
	return ValidateLogo(channel.Logo)
}

var assetIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)

func validAssetID(value string) bool {
	return assetIDPattern.MatchString(value) && !strings.Contains(value, "..")
}

func ValidateLogo(logo LogoConfig) error {
	switch logo.Source {
	case LogoNone:
		if logo.Ref != "" || logo.MIMEType != "" {
			return invalid("channel.logo", "a logo reference requires a built-in or custom source")
		}
	case LogoBuiltIn:
		if !validAssetID(logo.Ref) {
			return invalid("channel.logo.ref", "must be an opaque asset identifier")
		}
	case LogoCustom:
		if !validAssetID(logo.Ref) {
			return invalid("channel.logo.ref", "must be an opaque asset identifier")
		}
		switch logo.MIMEType {
		case "image/svg+xml", "image/png", "image/webp":
		default:
			return invalid("channel.logo.mimeType", "must be SVG, PNG, or WebP")
		}
	default:
		return invalid("channel.logo.source", "is not supported")
	}
	if logo.BugEnabled && logo.Source == LogoNone {
		return invalid("channel.logo.bugEnabled", "requires a channel logo")
	}
	if logo.BugEnabled && !logo.BugOverheadAccepted {
		return invalid("channel.logo.bugOverheadAccepted", "must acknowledge the transcoding overhead before enabling an on-screen bug")
	}
	switch logo.BugCorner {
	case LogoTopLeft, LogoTopRight, LogoBottomLeft, LogoBottomRight:
	default:
		return invalid("channel.logo.bugCorner", "is not supported")
	}
	if math.IsNaN(logo.BugWidthPct) || math.IsInf(logo.BugWidthPct, 0) || logo.BugWidthPct < 2 || logo.BugWidthPct > 20 {
		return invalid("channel.logo.bugWidthPercent", "must be a finite number between 2 and 20")
	}
	if math.IsNaN(logo.BugInsetPct) || math.IsInf(logo.BugInsetPct, 0) || logo.BugInsetPct < 0 || logo.BugInsetPct > 10 {
		return invalid("channel.logo.bugInsetPercent", "must be a finite number between 0 and 10")
	}
	switch logo.BugTreatment {
	case LogoColor, LogoWhite, LogoBlack:
	default:
		return invalid("channel.logo.bugTreatment", "is not supported")
	}
	return nil
}

func ValidateRule(rule Rule, channelID string) error {
	if !validOpaqueID(rule.ID) {
		return invalid("id", "must be an opaque identifier no longer than 128 characters")
	}
	if rule.ChannelID != channelID {
		return invalid("channelId", "must match the containing channel")
	}
	if n := strings.TrimSpace(rule.Name); n == "" || utf8.RuneCountInString(n) > 120 {
		return invalid("name", "must contain between 1 and 120 characters")
	}
	switch rule.SelectionMode {
	case SelectionSequential, SelectionShuffleBag, SelectionWeightedRandom:
	default:
		return invalid("selectionMode", "is not supported")
	}
	switch rule.EpisodeMode {
	case EpisodeNone, EpisodeInOrder, EpisodeMarathon, EpisodeRandomized:
	default:
		return invalid("episodeMode", "is not supported")
	}
	switch rule.ExhaustionMode {
	case ExhaustionLoop, ExhaustionSlate:
	default:
		return invalid("exhaustionMode", "is not supported")
	}
	if rule.DedupeWindow < 0 || rule.DedupeWindow > 1000 {
		return invalid("dedupeWindow", "must be between 0 and 1000")
	}
	if rule.MaxConsecutive < 0 || rule.MaxConsecutive > 1000 {
		return invalid("maxConsecutive", "must be between 0 and 1000")
	}
	if rule.TemplateVersion < 0 {
		return invalid("templateVersion", "must not be negative")
	}
	if err := ValidateScheduleQuery(rule.Query); err != nil {
		return fmt.Errorf("query: %w", err)
	}
	config := rule.Config
	if len(config) == 0 {
		config = json.RawMessage(`{}`)
	}
	if len(config) > 64*1024 || !json.Valid(config) {
		return invalid("config", "must be valid JSON no larger than 64 KiB")
	}
	var configValue map[string]any
	if err := json.Unmarshal(config, &configValue); err != nil || configValue == nil {
		return invalid("config", "must be a JSON object")
	}
	return nil
}

func ValidateBlock(block WeeklyBlock, channelID string, rules map[string]Rule) error {
	if !validOpaqueID(block.ID) {
		return invalid("id", "must be an opaque identifier no longer than 128 characters")
	}
	if block.ChannelID != channelID {
		return invalid("channelId", "must match the containing channel")
	}
	if n := strings.TrimSpace(block.Name); n == "" || utf8.RuneCountInString(n) > 120 {
		return invalid("name", "must contain between 1 and 120 characters")
	}
	if _, ok := rules[block.RuleID]; !ok {
		return invalid("ruleId", "must identify a rule in this channel")
	}
	if block.FallbackRuleID != "" {
		if _, ok := rules[block.FallbackRuleID]; !ok {
			return invalid("fallbackRuleId", "must identify a rule in this channel")
		}
	}
	if block.Weekdays == 0 || block.Weekdays > 127 {
		return invalid("weekdayMask", "must include at least one valid weekday")
	}
	if block.StartMinute < 0 || block.StartMinute >= 1440 {
		return invalid("startMinute", "must be between 0 and 1439")
	}
	if block.EndMinute < 0 || block.EndMinute >= 1440 {
		return invalid("endMinute", "must be between 0 and 1439")
	}
	if block.TemplateVersion < 0 {
		return invalid("templateVersion", "must not be negative")
	}
	return nil
}

func ValidateScheduleQuery(raw json.RawMessage) error {
	err := browsecontract.ValidateQuery(raw, browsecontract.ValidationOptions{
		AllowedFields: scheduleSafeFields,
		AllowEmpty:    true,
	})
	if err == nil {
		return nil
	}
	var contractError *browsecontract.ValidationError
	if errors.As(err, &contractError) {
		return invalid(contractError.Path, contractError.Message)
	}
	return err
}

var scheduleSafeFields = map[string]struct{}{
	"entityKind": {}, "title": {}, "year": {}, "decade": {}, "dateAdded": {},
	"genre": {}, "tag": {}, "author": {}, "series": {}, "contentRating": {},
	"availability": {}, "durationSeconds": {},
}

func ValidateCursorMap(cursors map[string]RuleCursor) error {
	if len(cursors) > MaximumRulesPerChannel {
		return invalid("cursors", fmt.Sprintf("must not exceed %d rules", MaximumRulesPerChannel))
	}
	for ruleID, cursor := range cursors {
		if strings.TrimSpace(ruleID) == "" {
			return invalid("cursors", "rule identifiers must not be empty")
		}
		if cursor.Cycle > math.MaxInt64 || cursor.Position < 0 || cursor.Position > 1_000_000 || cursor.Consecutive < 0 || cursor.Consecutive > 1000 {
			return invalid("cursors."+ruleID, "contains an invalid position or consecutive count")
		}
		if len(cursor.RecentMedia) > MaximumRecentMediaPerRule || len(cursor.SelectedGroup) > 64 || len(cursor.LastGroup) > 64 {
			return invalid("cursors."+ruleID, "exceeds cursor limits")
		}
		if (cursor.SelectedGroup != "" && !validCursorToken(cursor.SelectedGroup)) || (cursor.LastGroup != "" && !validCursorToken(cursor.LastGroup)) {
			return invalid("cursors."+ruleID, "contains a non-canonical group token")
		}
		seen := make(map[string]struct{}, len(cursor.RecentMedia))
		for _, mediaID := range cursor.RecentMedia {
			if !validCursorToken(mediaID) {
				return invalid("cursors."+ruleID, "contains a non-canonical recent-media token")
			}
			if _, duplicate := seen[mediaID]; duplicate {
				return invalid("cursors."+ruleID, "contains a duplicate recent media id")
			}
			seen[mediaID] = struct{}{}
		}
	}
	return nil
}

func validOpaqueID(value string) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= 128 && assetIDPattern.MatchString(value) && !strings.Contains(value, "..")
}

func validCursorToken(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}
