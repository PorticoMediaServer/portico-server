package librarychannels

import (
	"encoding/json"
	"time"
)

type LogoSource string

const (
	LogoNone    LogoSource = "none"
	LogoBuiltIn LogoSource = "built_in"
	LogoCustom  LogoSource = "custom"
)

type LogoTreatment string

const (
	LogoColor LogoTreatment = "color"
	LogoWhite LogoTreatment = "white"
	LogoBlack LogoTreatment = "black"
)

type LogoCorner string

const (
	LogoTopLeft     LogoCorner = "top_left"
	LogoTopRight    LogoCorner = "top_right"
	LogoBottomLeft  LogoCorner = "bottom_left"
	LogoBottomRight LogoCorner = "bottom_right"
)

type LogoConfig struct {
	Source     LogoSource
	Ref        string
	MIMEType   string
	BugEnabled bool
	// BugOverheadAccepted records the server owner's explicit acknowledgement
	// that burning a logo into video requires a transcoded channel rendition.
	BugOverheadAccepted bool
	BugCorner           LogoCorner
	BugWidthPct         float64
	BugInsetPct         float64
	BugTreatment        LogoTreatment
}

type Channel struct {
	ID                 string
	Name               string
	Description        string
	Enabled            bool
	SortOrder          int
	Timezone           string
	Seed               string
	DefaultRuleID      string
	QualityProfile     string
	Logo               LogoConfig
	TemplateKey        string
	TemplateVersion    int
	ConfigRevision     int64
	ActiveGenerationID string
	GeneratedThrough   time.Time
	HealthState        string
	HealthMessage      string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

type SelectionMode string

const (
	SelectionSequential     SelectionMode = "sequential"
	SelectionShuffleBag     SelectionMode = "shuffle_bag"
	SelectionWeightedRandom SelectionMode = "weighted_random"
)

type EpisodeMode string

const (
	EpisodeNone       EpisodeMode = "none"
	EpisodeInOrder    EpisodeMode = "in_order"
	EpisodeMarathon   EpisodeMode = "marathon"
	EpisodeRandomized EpisodeMode = "randomized"
)

type ExhaustionMode string

const (
	ExhaustionLoop  ExhaustionMode = "loop"
	ExhaustionSlate ExhaustionMode = "slate"
)

type Rule struct {
	ID              string
	ChannelID       string
	Name            string
	Enabled         bool
	SortOrder       int
	Query           json.RawMessage
	SelectionMode   SelectionMode
	EpisodeMode     EpisodeMode
	ExhaustionMode  ExhaustionMode
	DedupeWindow    int
	MaxConsecutive  int
	Config          json.RawMessage
	TemplateKey     string
	TemplateVersion int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// WeekdayMask uses time.Weekday as its bit index: Sunday is bit 0 and Saturday
// is bit 6. A value of 127 therefore means every day.
type WeekdayMask uint8

func (m WeekdayMask) Includes(day time.Weekday) bool {
	return m&(1<<uint(day)) != 0
}

type WeeklyBlock struct {
	ID              string
	ChannelID       string
	RuleID          string
	FallbackRuleID  string
	Name            string
	Enabled         bool
	Weekdays        WeekdayMask
	StartMinute     int
	EndMinute       int
	Priority        int
	Anchored        bool
	AllowOverrun    bool
	SortOrder       int
	TemplateKey     string
	TemplateVersion int
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

type Aggregate struct {
	Channel Channel
	Rules   []Rule
	Blocks  []WeeklyBlock
}

type Candidate struct {
	MediaID       string
	Title         string
	Subtitle      string
	Summary       string
	ContentRating string
	Artwork       json.RawMessage
	Duration      time.Duration
	Weight        float64
	Order         int64
	SeriesID      string
	SeasonNumber  int
	EpisodeNumber int
	Availability  string
}

type RuleCursor struct {
	Cycle         uint64   `json:"cycle"`
	Position      int      `json:"position"`
	SelectedGroup string   `json:"selectedGroup,omitempty"`
	RecentMedia   []string `json:"recentMedia,omitempty"`
	LastGroup     string   `json:"lastGroup,omitempty"`
	Consecutive   int      `json:"consecutive,omitempty"`
}

// ScheduleReason is a stable product-language key. Clients translate this key
// with the shared product-language catalog; the server never embeds English UI
// copy in a generated schedule.
type ScheduleReason string

const (
	ReasonNoScheduledMedia ScheduleReason = "library-channel.slate"
	ReasonMediaUnavailable ScheduleReason = "library-channel.program-unavailable"
	ReasonMediaRestricted  ScheduleReason = "library-channel.program-restricted"

	MessageNoPlayableCandidates   = "library-channel.warning-no-playable-candidates"
	MessageNoPlayableSchedule     = "library-channel.generation-no-playable-schedule"
	MessageGenerationFailed       = "library-channel.generation-failed"
	MessageGenerationLeaseExpired = "library-channel.generation-lease-expired"
	MessageRegenerationRequired   = "library-channel.health-regeneration-required"
	MessageScheduleWarnings       = "library-channel.health-schedule-warnings"
)

func isKnownLibraryChannelMessage(value string) bool {
	switch value {
	case string(ReasonNoScheduledMedia), string(ReasonMediaUnavailable), string(ReasonMediaRestricted),
		MessageNoPlayableCandidates, MessageNoPlayableSchedule, MessageGenerationFailed,
		MessageGenerationLeaseExpired, MessageRegenerationRequired, MessageScheduleWarnings:
		return true
	default:
		return false
	}
}

type PlayoutSourceKind string

const (
	PlayoutMedia          PlayoutSourceKind = "media"
	PlayoutGeneratedSlate PlayoutSourceKind = "generated_slate"
	PlayoutUnavailable    PlayoutSourceKind = "unavailable"
)

// PlayoutSource is the handoff contract between schedule generation and the
// playout layer. GeneratorID names a server-owned slate renderer, not a path.
type PlayoutSource struct {
	Kind            PlayoutSourceKind `json:"kind"`
	MediaID         string            `json:"mediaId,omitempty"`
	OffsetSeconds   int               `json:"offsetSeconds,omitempty"`
	DurationSeconds int               `json:"durationSeconds"`
	GeneratorID     string            `json:"generatorId,omitempty"`
}

type EntryKind string

const (
	EntryMedia       EntryKind = "media"
	EntrySlate       EntryKind = "slate"
	EntryUnavailable EntryKind = "unavailable"
)

type ScheduleEntry struct {
	ID                    string
	GenerationID          string
	ChannelID             string
	RuleID                string
	BlockID               string
	MediaID               string
	Kind                  EntryKind
	StartsAt              time.Time
	EndsAt                time.Time
	MediaOffsetSeconds    int
	SourceDurationSeconds int
	CycleNumber           uint64
	SelectionIndex        int
	Title                 string
	Subtitle              string
	Summary               string
	ContentRating         string
	Artwork               json.RawMessage
	Availability          string
	SelectionMetadata     json.RawMessage
	ReasonCode            ScheduleReason
	PlayoutSource         PlayoutSource
	CursorAfter           map[string]RuleCursor
	CreatedAt             time.Time
}

type GenerationStatus string

const (
	GenerationBuilding   GenerationStatus = "building"
	GenerationActive     GenerationStatus = "active"
	GenerationSuperseded GenerationStatus = "superseded"
	GenerationFailed     GenerationStatus = "failed"
)

type Generation struct {
	ID                string
	ChannelID         string
	ConfigRevision    int64
	Status            GenerationStatus
	HorizonStart      time.Time
	HorizonEnd        time.Time
	DeterministicSeed string
	CandidateHash     string
	InitialCursorHash string
	Cursors           map[string]RuleCursor
	Warnings          []string
	ErrorMessage      string
	LeaseExpiresAt    time.Time
	CreatedAt         time.Time
	CompletedAt       time.Time
}

// GenerationLease is returned only to the worker which acquired it. Only its
// SHA-256 digest is persisted, so database disclosure cannot be used to steal
// an in-flight generation.
type GenerationLease struct {
	GenerationID string
	Token        string
	ExpiresAt    time.Time
}

type GenerateRequest struct {
	GenerationID   string
	Channel        Channel
	Rules          []Rule
	Blocks         []WeeklyBlock
	Candidates     map[string][]Candidate
	InitialCursors map[string]RuleCursor
	// PreservedEntries and TailStart allow rolling extension without changing
	// the currently playing prefix. The last preserved entry must end exactly
	// at TailStart and carry the cursor checkpoint used for the tail.
	PreservedEntries []ScheduleEntry
	TailStart        time.Time
	Start            time.Time
	Days             int
	Now              time.Time
}

type GenerateResult struct {
	Generation  Generation
	Entries     []ScheduleEntry
	NextCursors map[string]RuleCursor
	Warnings    []string
}

type AccessDecision int

const (
	AccessAllowed AccessDecision = iota
	AccessRestricted
	AccessUnavailable
)
