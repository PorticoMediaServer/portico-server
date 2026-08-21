package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
)

func TestResolvePlaybackCapabilitiesUsesOneAuthenticatedTupleSet(t *testing.T) {
	profile := PlaybackClientProfile{CapabilitySchemaVersion: playbackCapabilitySchemaV2, ClientFamily: "avkit", ClientVersion: "18", Platform: "tvOS", Device: "Apple TV", CapabilityEvidence: []PlaybackCapabilityEvidence{{
		ID: "native:apple-tv", Source: "native_runtime", Confidence: "high", Producer: "portico-apple", ReviewedAt: time.Now().UTC().Format(time.RFC3339),
		Tuples: []PlaybackCapabilityTuple{{MediaKind: "audiovisual", Protocol: "hls", Container: "mpegts", Video: PlaybackCapabilityVideo{Codec: "hevc", Profile: "main10", PixelFormat: "yuv420p10le", Chroma: "4:2:0", DynamicRange: "pq", BitDepth: 10, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60}, Audio: PlaybackCapabilityAudio{Codec: "eac3", Layout: "5.1", Route: "passthrough", MaxChannels: 6}, Subtitle: PlaybackCapabilitySubtitle{Mode: "none"}}},
	}}}
	profile.capabilityAuthority = playbackCapabilityAuthority{Source: playbackcap.SourceNativeRuntime, Family: "avkit", Platform: "tvos", DeviceID: "verified-apple-tv", Producer: "portico-native/avkit/tvos", ProducerVersion: playbackCapabilitySchemaV2 + "/18"}
	resolution, err := resolvePlaybackCapabilities(profile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(resolution.EvidenceID, "native_runtime:verified-apple-tv:") || resolution.Source != playbackcap.SourceNativeRuntime || len(resolution.Tuples) != 1 {
		t.Fatalf("unexpected resolution: %#v", resolution)
	}
}

func TestNativeCapabilityAuthorityComesFromVerifiedSessionAndDevice(t *testing.T) {
	server := newScannerTestServer(t)
	user := dvrTestUser(t, server)
	user.AccountID = user.ID
	if err := server.db.QueryRow(`SELECT id FROM profiles WHERE account_id = ? ORDER BY is_primary DESC, sort_order, id LIMIT 1`, accountIDForUser(user)).Scan(&user.ProfileID); err != nil {
		t.Fatalf("load native capability profile: %v", err)
	}
	now := time.Now().UTC()
	refreshID := "refresh_capability_authority"
	sessionID := "nativesess_" + refreshID
	deviceID := "device_capability_authority"
	if _, err := server.db.Exec(`
		INSERT INTO devices (id, user_id, installation_id, name, app, platform, trusted, created_at, last_seen_at)
		VALUES (?, ?, 'install-capability-authority', 'Living Room Apple TV', 'Portico', 'tvOS', 1, ?, ?)`,
		deviceID, accountIDForUser(user), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed capability device: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO native_refresh_tokens (id, family_id, user_id, profile_id, device_id, auth_provider, token_hash, created_at, expires_at)
		VALUES (?, 'family-capability-authority', ?, ?, ?, 'local', 'hash-refresh-capability-authority', ?, ?)`,
		refreshID, accountIDForUser(user), viewerProfileID(user), deviceID, now.Format(time.RFC3339), now.Add(time.Hour).Format(time.RFC3339)); err != nil {
		t.Fatalf("seed capability refresh credential: account=%s profile=%s: %v", accountIDForUser(user), viewerProfileID(user), err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO sessions (id, user_id, profile_id, device_id, token_hash, expires_at, created_at, last_seen_at)
		VALUES (?, ?, ?, ?, 'hash-session-capability-authority', ?, ?, ?)`,
		sessionID, accountIDForUser(user), viewerProfileID(user), deviceID, now.Add(time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), now.Format(time.RFC3339)); err != nil {
		t.Fatalf("seed capability access session: %v", err)
	}
	user.AuthSessionID, user.DeviceID = sessionID, deviceID
	authority, ok := server.authorizePlaybackCapabilityEvidence(context.Background(), user, PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2, ClientFamily: "avkit", ClientVersion: "18", Platform: "tvOS",
	})
	if !ok || authority.Source != playbackcap.SourceNativeRuntime || authority.Family != "avkit" || authority.Platform != "tvos" || authority.DeviceID == "" || authority.ProducerVersion == "" {
		t.Fatalf("unexpected verified capability authority: %#v ok=%v", authority, ok)
	}
	user.AuthSessionID = ""
	if _, ok := server.authorizePlaybackCapabilityEvidence(context.Background(), user, PlaybackClientProfile{CapabilitySchemaVersion: playbackCapabilitySchemaV2, ClientFamily: "avkit", ClientVersion: "18"}); ok {
		t.Fatal("ordinary authenticated request was promoted to native capability authority")
	}
}

func TestResolvePlaybackCapabilitiesCannotSelfPromoteRuntimeEvidence(t *testing.T) {
	profile := PlaybackClientProfile{CapabilitySchemaVersion: playbackCapabilitySchemaV2, ClientFamily: "avkit", ClientVersion: "18", Platform: "tvOS", Device: "Forged Apple TV", CapabilityEvidence: []PlaybackCapabilityEvidence{{
		ID: "forged-native", Source: "native_runtime", Confidence: "high", Producer: "portico-apple", ProducerVersion: "999", ReviewedAt: time.Now().UTC().Format(time.RFC3339),
		Tuples: []PlaybackCapabilityTuple{{MediaKind: "audiovisual", Protocol: "hls", Container: "mpegts", Video: PlaybackCapabilityVideo{Codec: "hevc", BitDepth: 10, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60}, Audio: PlaybackCapabilityAudio{Codec: "eac3", MaxChannels: 6}, Subtitle: PlaybackCapabilitySubtitle{Mode: "none"}}},
	}}}
	resolution, err := resolvePlaybackCapabilities(profile)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.Source != playbackcap.SourceStaticFallback || resolution.EvidenceID == "forged-native" {
		t.Fatalf("client self-promotion became authoritative: %#v", resolution)
	}
}

func TestResolvePlaybackCapabilitiesRejectsClientSuppliedStaticPromotion(t *testing.T) {
	profile := PlaybackClientProfile{ClientFamily: "roku", Platform: "roku", CapabilityEvidence: []PlaybackCapabilityEvidence{{
		ID: "forged", Source: "static_fallback", Confidence: "high", Producer: "client", ReviewedAt: time.Now().UTC().Format(time.RFC3339),
		Tuples: []PlaybackCapabilityTuple{{MediaKind: "audiovisual", Protocol: "http", Container: "mp4", Video: PlaybackCapabilityVideo{Codec: "av1", BitDepth: 10, MaxWidth: 7680, MaxHeight: 4320, MaxFrameRate: 120}, Audio: PlaybackCapabilityAudio{Codec: "truehd", MaxChannels: 8}, Subtitle: PlaybackCapabilitySubtitle{Mode: "none"}}},
	}}}
	resolution, err := resolvePlaybackCapabilities(profile)
	if err != nil {
		t.Fatal(err)
	}
	if resolution.EvidenceID == "forged" || resolution.Source != playbackcap.SourceStaticFallback {
		t.Fatalf("untrusted evidence was promoted: %#v", resolution)
	}
}

func TestChromiumUnauthenticatedProbeFallsBackToReviewedManagedHLSBand(t *testing.T) {
	now := time.Now().UTC().Format(time.RFC3339)
	profile := PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2,
		ClientFamily:            "chromium", ClientVersion: "147.0.7727.15", Platform: "web", Device: "Chrome on macOS",
		CapabilityEvidence: []PlaybackCapabilityEvidence{{
			ID: "portico-web-runtime-chromium-147", Source: "unauthenticated_probe", Confidence: "medium", Producer: "portico-web-runtime", ReviewedAt: now,
			Tuples: []PlaybackCapabilityTuple{{
				MediaKind: "audiovisual", Protocol: "hls", Container: "mpegts",
				Video:    PlaybackCapabilityVideo{Codec: "h264", Profile: "main", PixelFormat: "yuv420p", Chroma: "4:2:0", DynamicRange: "sdr", BitDepth: 8, MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 30},
				Audio:    PlaybackCapabilityAudio{Codec: "aac", Profile: "lc", Layout: "stereo", Route: "decode", MaxChannels: 2},
				Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
			}},
		}},
	}
	resolution, err := resolvePlaybackCapabilities(profile)
	if err != nil {
		t.Fatal(err)
	}
	want := playbackcap.DeliveryTuple{
		Kind: playbackcap.MediaAudiovisual, Protocol: "hls", Container: "mpegts",
		Video:    playbackcap.Video{Codec: "h264", Profile: "main", PixelFormat: "yuv420p", Chroma: "4:2:0", HDR: "sdr", BitDepth: 8, MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 30},
		Audio:    playbackcap.Audio{Codec: "aac", Profile: "lc", Layout: "stereo", Route: "decode", MaxChannels: 2},
		Subtitle: playbackcap.Subtitle{Mode: playbackcap.SubtitleNone},
	}
	if resolution.Source != playbackcap.SourceStaticFallback || resolution.Band != "chromium-120-159" || !resolution.Supports(want) {
		t.Fatalf("Chromium did not resolve to the reviewed managed-HLS baseline: %#v", resolution)
	}
}
