package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
)

const playbackCapabilitySchemaV2 = playbackcap.SchemaVersion

// playbackCapabilityAuthority is server-derived request state. It is
// deliberately unexported and stored in a non-JSON field on the client
// profile, so a request body cannot manufacture native/runtime provenance.
type playbackCapabilityAuthority struct {
	Source          playbackcap.EvidenceSource
	Family          string
	Platform        string
	Device          string
	DeviceID        string
	Producer        string
	ProducerVersion string
}

func resolvePlaybackCapabilities(profile PlaybackClientProfile) (playbackcap.Resolution, error) {
	client := playbackcap.Client{
		Family:   playbackClientFamily(profile),
		Version:  strings.TrimSpace(profile.ClientVersion),
		Platform: playbackClientPlatform(profile),
		Device:   strings.TrimSpace(profile.Device),
	}
	evidence := make([]playbackcap.Evidence, 0, len(profile.CapabilityEvidence))
	for _, candidate := range profile.CapabilityEvidence {
		if normalized, ok := normalizePlaybackCapabilityEvidence(client, profile.CapabilitySchemaVersion, profile.capabilityAuthority, candidate); ok {
			evidence = append(evidence, normalized)
		}
	}
	return playbackcap.DefaultResolver().Resolve(client, evidence)
}

func normalizePlaybackCapabilityEvidence(client playbackcap.Client, schemaVersion string, authority playbackCapabilityAuthority, raw PlaybackCapabilityEvidence) (playbackcap.Evidence, bool) {
	reviewedAt, err := time.Parse(time.RFC3339, strings.TrimSpace(raw.ReviewedAt))
	if err != nil {
		return playbackcap.Evidence{}, false
	}
	source := playbackcap.EvidenceSource(strings.ToLower(strings.TrimSpace(raw.Source)))
	switch source {
	case playbackcap.SourceNativeRuntime, playbackcap.SourceAuthenticatedRuntime:
		if authority.Source != source || strings.TrimSpace(schemaVersion) != playbackCapabilitySchemaV2 ||
			client.Normalized().Family != strings.ToLower(strings.TrimSpace(authority.Family)) ||
			client.Normalized().Platform != strings.ToLower(strings.TrimSpace(authority.Platform)) {
			return playbackcap.Evidence{}, false
		}
	case playbackcap.SourceUnauthenticatedProbe:
	default:
		// Request bodies cannot promote unauthenticated or static claims. Static
		// evidence comes only from Portico's reviewed built-in registry.
		return playbackcap.Evidence{}, false
	}
	tuples := make([]playbackcap.DeliveryTuple, 0, len(raw.Tuples))
	for _, tuple := range raw.Tuples {
		tuples = append(tuples, playbackcap.DeliveryTuple{
			Kind:      playbackcap.MediaKind(strings.ToLower(strings.TrimSpace(tuple.MediaKind))),
			Protocol:  tuple.Protocol,
			Container: tuple.Container,
			Video: playbackcap.Video{
				Codec: tuple.Video.Codec, Profile: tuple.Video.Profile, Level: tuple.Video.Level,
				Tag: tuple.Video.Tag, PixelFormat: tuple.Video.PixelFormat, Chroma: tuple.Video.Chroma,
				HDR: tuple.Video.DynamicRange, BitDepth: tuple.Video.BitDepth,
				DolbyVisionProfile: tuple.Video.DolbyVisionProfile, MaxWidth: tuple.Video.MaxWidth,
				MaxHeight: tuple.Video.MaxHeight, MaxFrameRate: tuple.Video.MaxFrameRate,
			},
			Audio: playbackcap.Audio{
				Codec: tuple.Audio.Codec, Profile: tuple.Audio.Profile, Layout: tuple.Audio.Layout,
				Route: tuple.Audio.Route, MaxChannels: tuple.Audio.MaxChannels,
				ObjectPassthrough: tuple.Audio.ObjectPassthrough,
			},
			Subtitle: playbackcap.Subtitle{
				Codec: tuple.Subtitle.Codec, Kind: tuple.Subtitle.Kind,
				Mode: playbackcap.SubtitleMode(strings.ToLower(strings.TrimSpace(tuple.Subtitle.Mode))),
			},
		})
	}
	evidenceID := strings.TrimSpace(raw.ID)
	producer := strings.TrimSpace(raw.Producer)
	producerVersion := strings.TrimSpace(raw.ProducerVersion)
	authenticated := false
	if source == playbackcap.SourceNativeRuntime || source == playbackcap.SourceAuthenticatedRuntime {
		encoded, _ := json.Marshal(tuples)
		digest := sha256.Sum256(encoded)
		evidenceID = fmt.Sprintf("%s:%s:%x", source, authority.DeviceID, digest[:8])
		producer = authority.Producer
		producerVersion = authority.ProducerVersion
		authenticated = true
	}
	evidence := playbackcap.Evidence{
		ID:     evidenceID,
		Client: client,
		Provenance: playbackcap.Provenance{
			Source: source, Confidence: playbackcap.Confidence(strings.ToLower(strings.TrimSpace(raw.Confidence))),
			Producer: producer, ProducerVersion: producerVersion, SchemaVersion: strings.TrimSpace(schemaVersion),
			MinVersion: strings.TrimSpace(raw.MinVersion), MaxVersion: strings.TrimSpace(raw.MaxVersion),
			ReviewedAt: reviewedAt.UTC(), Authenticated: authenticated,
		},
		Tuples: tuples,
	}
	if evidence.Validate(time.Now().UTC()) != nil {
		return playbackcap.Evidence{}, false
	}
	return evidence, true
}

func (s *Server) authorizePlaybackCapabilityEvidence(ctx context.Context, user User, profile PlaybackClientProfile) (playbackCapabilityAuthority, bool) {
	if s == nil || s.db == nil || !strings.HasPrefix(user.AuthSessionID, "nativesess_") || strings.TrimSpace(user.DeviceID) == "" || strings.TrimSpace(profile.CapabilitySchemaVersion) != playbackCapabilitySchemaV2 {
		return playbackCapabilityAuthority{}, false
	}
	refreshID := strings.TrimPrefix(user.AuthSessionID, "nativesess_")
	var installationID, deviceName, appName, platform, deviceRevoked, refreshCreated, refreshExpires, refreshRevoked string
	var trusted int
	err := s.queryUserRow(ctx, `
		SELECT COALESCE(d.installation_id, ''), COALESCE(NULLIF(d.display_name, ''), d.name), d.app, d.platform,
			d.trusted, COALESCE(d.revoked_at, ''), n.created_at, n.expires_at, COALESCE(n.revoked_at, '')
		FROM sessions s
		JOIN native_refresh_tokens n ON n.id = ? AND n.user_id = s.user_id AND n.device_id = s.device_id
			AND n.profile_id = COALESCE(NULLIF(s.profile_id, ''), s.user_id)
		JOIN devices d ON d.id = s.device_id AND d.user_id = s.user_id
		WHERE s.id = ? AND s.user_id = ? AND COALESCE(NULLIF(s.profile_id, ''), s.user_id) = ? AND s.device_id = ?`,
		refreshID, user.AuthSessionID, accountIDForUser(user), viewerProfileID(user), user.DeviceID).
		Scan(&installationID, &deviceName, &appName, &platform, &trusted, &deviceRevoked, &refreshCreated, &refreshExpires, &refreshRevoked)
	if err != nil || trusted != 1 || deviceRevoked != "" || refreshRevoked != "" {
		return playbackCapabilityAuthority{}, false
	}
	expiresAt, err := parseCredentialTime(refreshExpires)
	if err != nil || !expiresAt.After(time.Now().UTC()) {
		return playbackCapabilityAuthority{}, false
	}
	family, canonicalPlatform := nativeCapabilityFamily(platform)
	if family == "" || strings.TrimSpace(appName) == "" || (strings.TrimSpace(profile.ClientFamily) != "" && !strings.EqualFold(strings.TrimSpace(profile.ClientFamily), family)) || strings.TrimSpace(profile.ClientVersion) == "" {
		return playbackCapabilityAuthority{}, false
	}
	producer := "portico-native/" + family + "/" + canonicalPlatform
	producerVersion := playbackCapabilitySchemaV2 + "/" + strings.ToLower(strings.TrimSpace(appName)) + "/" + refreshCreated
	deviceIdentity := strings.TrimSpace(user.DeviceID)
	if installationID != "" {
		deviceIdentity += "/" + installationID
	}
	return playbackCapabilityAuthority{
		Source: playbackcap.SourceNativeRuntime, Family: family, Platform: canonicalPlatform,
		Device: strings.TrimSpace(deviceName), DeviceID: deviceIdentity,
		Producer: producer, ProducerVersion: producerVersion,
	}, true
}

func nativeCapabilityFamily(platform string) (family, canonicalPlatform string) {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "ios":
		return "avkit", "ios"
	case "ipados":
		return "avkit", "ipados"
	case "tvos":
		return "avkit", "tvos"
	case "android":
		return "media3", "android"
	case "android-tv", "android tv":
		return "media3", "android-tv"
	case "fireos", "fire-tv", "fire tv":
		return "fire-tv", "fireos"
	case "roku":
		return "roku", "roku"
	case "tizen":
		return "tizen", "tizen"
	case "webos":
		return "webos", "webos"
	case "cast", "chromecast":
		return "cast", "cast"
	default:
		return "", ""
	}
}

func playbackClientFamily(profile PlaybackClientProfile) string {
	if family := strings.ToLower(strings.TrimSpace(profile.ClientFamily)); family != "" {
		return family
	}
	identity := strings.ToLower(strings.Join([]string{profile.Device, profile.Platform}, " "))
	for _, candidate := range []struct{ needle, family string }{
		{"fire tv", "fire-tv"}, {"android tv", "media3"}, {"chromecast", "cast"},
		{"google cast", "cast"}, {"microsoft edge", "edge"}, {"edge", "edge"},
		{"firefox", "firefox"}, {"safari", "safari"}, {"chrome", "chromium"},
		{"chromium", "chromium"}, {"roku", "roku"}, {"tizen", "tizen"},
		{"webos", "webos"}, {"ios", "avkit"}, {"ipados", "avkit"},
		{"tvos", "avkit"}, {"android", "media3"}, {"dlna", "dlna"},
	} {
		if strings.Contains(identity, candidate.needle) {
			return candidate.family
		}
	}
	return "chromium"
}

func playbackClientPlatform(profile PlaybackClientProfile) string {
	planned := strings.ToLower(strings.TrimSpace(profile.Platform))
	switch {
	case strings.Contains(planned, "android tv"):
		return "android-tv"
	case strings.Contains(planned, "android"):
		return "android"
	case strings.Contains(planned, "ipados"):
		return "ipados"
	case strings.Contains(planned, "tvos"):
		return "tvos"
	case strings.Contains(planned, "ios"):
		return "ios"
	case strings.Contains(planned, "fire"):
		return "fireos"
	case strings.Contains(planned, "roku"):
		return "roku"
	case strings.Contains(planned, "tizen"):
		return "tizen"
	case strings.Contains(planned, "webos"):
		return "webos"
	case strings.Contains(planned, "cast"):
		return "cast"
	case strings.Contains(planned, "dlna"):
		return "dlna"
	default:
		return "web"
	}
}
