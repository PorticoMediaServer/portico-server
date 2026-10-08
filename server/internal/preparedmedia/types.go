// Package preparedmedia owns owner-requested full-media derivatives. Catalog
// associations, playback occurrences and current permission remain elsewhere.
package preparedmedia

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"

	"portico.local/server/internal/identity"
)

var (
	ErrOwner         = errors.New("server owner required")
	ErrInput         = errors.New("invalid optimization request")
	ErrConflict      = errors.New("optimization revision changed")
	ErrSourceChanged = errors.New("optimization source changed")
	ErrUnavailable   = errors.New("optimization source unavailable")
	ErrUnsupported   = errors.New("optimization source or profile unsupported")
	ErrConfiguration = errors.New("optimization encoder or sandbox not configured")
	ErrOutput        = errors.New("prepared output validation failed")
	ErrCancelled     = errors.New("optimization cancelled")
)

const TargetID = "prepared-library"
const Kind = "media-optimization"

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
var validDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

type Authority func(context.Context, *sql.Tx, identity.Principal, string) (identity.Principal, error)

type Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	VideoKbps   int    `json:"videoKbps"`
	AudioKbps   int    `json:"audioKbps"`
	Description string `json:"description"`
}

// Immutable recipes: a recipe change gets a new ID, never different bytes under
// an old transformation identity. Clients cannot supply arbitrary encoder flags.
var profiles = []Profile{
	{"portable-360-v3", "360p", "video", 640, 360, 1000, 96, "H.264 SDR video and AAC stereo audio in MP4."},
	{"portable-480-v3", "480p", "video", 854, 480, 2000, 96, "H.264 SDR video and AAC stereo audio in MP4."},
	{"portable-1440-v3", "1440p", "video", 2560, 1440, 12000, 192, "H.264 SDR video and AAC stereo audio in MP4."},
	{"portable-2160-v3", "2160p", "video", 3840, 2160, 20000, 192, "H.264 SDR video and AAC stereo audio in MP4."},
	{"portable-720-v3", "Portable · 720p", "video", 1280, 720, 2500, 192, "H.264 SDR video and AAC stereo audio in MP4. Keeps the original."},
	{"portable-1080-v3", "Balanced · 1080p", "video", 1920, 1080, 5000, 192, "H.264 SDR video and AAC stereo audio in MP4. Keeps all audio tracks."},
	{"audio-aac-v3", "Audio · AAC", "audio", 0, 0, 0, 192, "AAC stereo in M4A for music and audiobook files. Keeps the original."},
}

// Legacy recipes remain resolvable for retained jobs/versions. New views offer
// v3 bounds streaming MP4 fragments; v2 omits unremapped chapter data.
var legacyProfiles = []Profile{
	{"portable-720-v2", "Portable · 720p", "video", 1280, 720, 2500, 192, "H.264 SDR video and AAC stereo audio in MP4. Keeps the original."},
	{"portable-1080-v2", "Balanced · 1080p", "video", 1920, 1080, 5000, 192, "H.264 SDR video and AAC stereo audio in MP4. Keeps all audio tracks."},
	{"audio-aac-v2", "Audio · AAC", "audio", 0, 0, 0, 192, "AAC stereo in M4A for music and audiobook files. Keeps the original."},

	{"portable-720-v1", "Portable · 720p", "video", 1280, 720, 2500, 192, "H.264 SDR video and AAC stereo audio in MP4. Keeps the original."},
	{"portable-1080-v1", "Balanced · 1080p", "video", 1920, 1080, 5000, 192, "H.264 SDR video and AAC stereo audio in MP4. Keeps all audio tracks."},
	{"audio-aac-v1", "Audio · AAC", "audio", 0, 0, 0, 192, "AAC stereo in M4A for music and audiobook files. Keeps the original."},
}

func Profiles() []Profile { return append([]Profile(nil), profiles...) }

// ProfileByID exposes the immutable recipe to size estimators.
func ProfileByID(id string) (Profile, error) { return profile(id) }

func profile(id string) (Profile, error) {
	for _, p := range profiles {
		if p.ID == id {
			return p, nil
		}
	}
	for _, p := range legacyProfiles {
		if p.ID == id {
			return p, nil
		}
	}
	return Profile{}, ErrInput
}
func hash(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type Request struct {
	IdempotencyKey         string `json:"idempotencyKey"`
	ItemID                 string `json:"itemId"`
	SourceID               string `json:"sourceId"`
	ExpectedSourceRevision string `json:"expectedSourceRevision"`
	ProfileID              string `json:"profileId"`
	TargetID               string `json:"targetId"`
}
type Command struct {
	IdempotencyKey   string `json:"idempotencyKey"`
	ExpectedRevision int64  `json:"expectedRevision"`
}
type SourceChoice struct {
	ID        string `json:"id"`
	Revision  string `json:"revision"`
	PartIndex int    `json:"partIndex"`
	Container string `json:"container"`
	Height    int    `json:"height"`
	Available bool   `json:"available"`
	Reason    string `json:"reason"`
}
type Job struct {
	ID         string `json:"id"`
	ItemID     string `json:"itemId"`
	SourceID   string `json:"sourceId"`
	ProfileID  string `json:"profileId"`
	TargetID   string `json:"targetId"`
	State      string `json:"state"`
	Phase      string `json:"phase"`
	Generation int64  `json:"generation"`
	Bytes      int64  `json:"bytes"`
	ErrorCode  string `json:"errorCode"`
	Revision   int64  `json:"revision"`
	CreatedMS  int64  `json:"createdMs"`
	UpdatedMS  int64  `json:"updatedMs"`
	VersionID  string `json:"versionId"`
}
type Facts struct {
	Container   string  `json:"container"`
	VideoCodec  string  `json:"videoCodec"`
	AudioCodec  string  `json:"audioCodec"`
	Width       int     `json:"width"`
	Height      int     `json:"height"`
	Duration    float64 `json:"duration"`
	AudioTracks int     `json:"audioTracks"`
}
type Version struct {
	ID             string  `json:"id"`
	ItemID         string  `json:"itemId"`
	SourceID       string  `json:"sourceId"`
	ProfileID      string  `json:"profileId"`
	TargetID       string  `json:"targetId"`
	SourceRevision string  `json:"sourceRevision"`
	PartIndex      int     `json:"partIndex"`
	EditionID      *string `json:"editionId"`
	Digest         string  `json:"digest"`
	Size           int64   `json:"size"`
	Facts          Facts   `json:"facts"`
	State          string  `json:"state"`
	Revision       int64   `json:"revision"`
	CreatedMS      int64   `json:"createdMs"`
	Selectable     bool    `json:"selectable"`
	Reason         string  `json:"reason"`
}
type View struct {
	ItemID                 string         `json:"itemId"`
	CanManage              bool           `json:"canManage"`
	PreparedOffersRevision string         `json:"preparedOffersRevision"`
	Configured             bool           `json:"configured"`
	ConfigurationReason    string         `json:"configurationReason"`
	Profiles               []Profile      `json:"profiles"`
	Targets                []Target       `json:"targets"`
	Sources                []SourceChoice `json:"sources"`
	Jobs                   []Job          `json:"jobs"`
	Versions               []Version      `json:"versions"`
	PollAfterMS            int            `json:"pollAfterMs"`
}
type Target struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Choice is an explicit server offer, not a library edition or an original
// source-version claim. Queue ownership and occurrence transitions stay in VOD.
type Choice struct {
	VersionID        string `json:"versionId"`
	ExpectedRevision int64  `json:"expectedRevision"`
	OffersRevision   string `json:"offersRevision"`
}

func ValidChoice(c *Choice) bool {
	return c == nil || validID.MatchString(c.VersionID) && c.ExpectedRevision > 0 && validDigest.MatchString(c.OffersRevision)
}
func OffersRevision(item string, versions []Version) string {
	return hash([]any{"prepared-offers-v1", item, versions})
}
