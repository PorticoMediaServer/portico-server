package app

import (
	"strings"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/playbackcap"
)

// authenticatedPlaybackRuntimeProfile is deterministic evidence from a
// hypothetical authenticated test runtime. Tests using it exercise playback
// lifecycle behavior, not capability discovery or incompatibility handling.
func authenticatedPlaybackRuntimeProfile() PlaybackClientProfile {
	profile := PlaybackClientProfile{
		CapabilitySchemaVersion: playbackCapabilitySchemaV2,
		ClientFamily:            "avkit",
		ClientVersion:           "18",
		Platform:                "tvos",
		Device:                  "Authenticated Test Runtime",
		CapabilityEvidence: []PlaybackCapabilityEvidence{{
			ID:         "authenticated:portico-test-runtime-v1",
			Source:     "authenticated_runtime",
			Confidence: "high",
			Producer:   "portico-server-tests",
			ReviewedAt: time.Now().UTC().Format(time.RFC3339),
			Tuples: []PlaybackCapabilityTuple{
				{
					MediaKind: "audiovisual", Protocol: "http", Container: "mp4",
					Video:    PlaybackCapabilityVideo{Codec: "h264", DynamicRange: "sdr", BitDepth: 8, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60},
					Audio:    PlaybackCapabilityAudio{Codec: "aac", Layout: "stereo", Route: "decode", MaxChannels: 2},
					Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
				},
				{
					MediaKind: "audiovisual", Protocol: "http", Container: "matroska",
					Video:    PlaybackCapabilityVideo{Codec: "h264", DynamicRange: "sdr", BitDepth: 8, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60},
					Audio:    PlaybackCapabilityAudio{Codec: "aac", Layout: "stereo", Route: "decode", MaxChannels: 2},
					Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
				},
				{
					MediaKind: "audiovisual", Protocol: "hls", Container: "mpegts",
					Video:    PlaybackCapabilityVideo{Codec: "h264", DynamicRange: "sdr", BitDepth: 8, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60},
					Audio:    PlaybackCapabilityAudio{Codec: "aac", Layout: "stereo", Route: "decode", MaxChannels: 2},
					Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
				},
				{
					MediaKind: "audiovisual", Protocol: "hls", Container: "mp4",
					Video:    PlaybackCapabilityVideo{Codec: "h264", DynamicRange: "sdr", BitDepth: 8, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60},
					Audio:    PlaybackCapabilityAudio{Codec: "aac", Layout: "stereo", Route: "decode", MaxChannels: 2},
					Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
				},
				{
					MediaKind: "audio", Protocol: "http", Container: "mp4",
					Audio:    PlaybackCapabilityAudio{Codec: "aac", Layout: "stereo", Route: "decode", MaxChannels: 2},
					Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
				},
				{
					MediaKind: "audio", Protocol: "http", Container: "mp3",
					Audio:    PlaybackCapabilityAudio{Codec: "mp3", Layout: "stereo", Route: "decode", MaxChannels: 2},
					Subtitle: PlaybackCapabilitySubtitle{Mode: "none"},
				},
			},
		}},
	}
	profile.capabilityAuthority = playbackCapabilityAuthority{
		Source: playbackcap.SourceAuthenticatedRuntime, Family: profile.ClientFamily, Platform: profile.Platform,
		Device: profile.Device, DeviceID: "verified-test-runtime", Producer: "portico-server-tests",
		ProducerVersion: playbackCapabilitySchemaV2 + "/18",
	}
	return profile
}

func attachAuthenticatedPlaybackRuntime(profile PlaybackClientProfile) PlaybackClientProfile {
	fixture := authenticatedPlaybackRuntimeProfile()
	identity := strings.ToLower(strings.TrimSpace(profile.Device + " " + profile.Platform))
	switch {
	case strings.Contains(identity, "firefox"):
		fixture.ClientFamily, fixture.ClientVersion, fixture.Platform = "firefox", "130", "web"
	case strings.Contains(identity, "edge"):
		fixture.ClientFamily, fixture.ClientVersion, fixture.Platform = "edge", "125", "web"
	case strings.Contains(identity, "chrome"), strings.Contains(identity, "chromium"):
		fixture.ClientFamily, fixture.ClientVersion, fixture.Platform = "chromium", "125", "web"
	case strings.Contains(identity, "safari"):
		fixture.ClientFamily, fixture.ClientVersion, fixture.Platform = "safari", "18", "web"
	case strings.Contains(identity, "ipad"):
		fixture.ClientFamily, fixture.ClientVersion, fixture.Platform = "avkit", "18", "ipados"
	case strings.Contains(identity, "iphone"), strings.Contains(identity, "ios"):
		fixture.ClientFamily, fixture.ClientVersion, fixture.Platform = "avkit", "18", "ios"
	}
	profile.CapabilitySchemaVersion = fixture.CapabilitySchemaVersion
	profile.ClientFamily = fixture.ClientFamily
	profile.ClientVersion = fixture.ClientVersion
	profile.Platform = fixture.Platform
	if profile.Device == "" {
		profile.Device = fixture.Device
	}
	profile.CapabilityEvidence = fixture.CapabilityEvidence
	profile.capabilityAuthority = fixture.capabilityAuthority
	profile.capabilityAuthority.Family = playbackClientFamily(profile)
	profile.capabilityAuthority.Platform = playbackClientPlatform(profile)
	profile.capabilityAuthority.Device = profile.Device
	return profile
}

func seedExactPlaybackFactsForFixture(t testingT, server *Server, mediaID string) {
	t.Helper()
	if _, err := server.db.Exec(`DELETE FROM media_analysis_facts WHERE media_id = ?`, mediaID); err != nil {
		t.Fatalf("clear stale playback analysis fixture: %v", err)
	}
	if mediaID == "movie_meridian" {
		if _, err := server.db.Exec(`UPDATE media_items SET source_url = 'https://media.example.com/portico-test-runtime.mp4' WHERE id = ?; DELETE FROM media_files WHERE media_id = ?`, mediaID, mediaID); err != nil {
			t.Fatalf("seed deterministic playback source: %v", err)
		}
	}
	var mediaType string
	if err := server.db.QueryRow(`SELECT type FROM media_items WHERE id = ?`, mediaID).Scan(&mediaType); err != nil {
		return
	}
	if mediaType == "album" {
		rows, err := server.db.Query(`SELECT id FROM media_items WHERE parent_id = ? AND type = 'track' ORDER BY index_number, id`, mediaID)
		if err != nil {
			t.Fatalf("load album playback fixture tracks: %v", err)
		}
		var childIDs []string
		for rows.Next() {
			var childID string
			if err := rows.Scan(&childID); err != nil {
				rows.Close()
				t.Fatalf("scan album playback fixture track: %v", err)
			}
			childIDs = append(childIDs, childID)
		}
		rows.Close()
		for _, childID := range childIDs {
			seedExactPlaybackFactsForFixture(t, server, childID)
		}
		return
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_streams WHERE media_id = ?`, mediaID).Scan(&count); err != nil {
		t.Fatalf("count playback fixture streams: %v", err)
	}
	if count == 0 {
		if mediaType == "track" || mediaType == "audiobook" {
			_, err := server.db.Exec(`INSERT INTO media_streams (id, media_id, stream_index, kind, codec, channels, bitrate, channel_layout, sample_rate, display_title) VALUES (?, ?, 0, 'audio', 'mp3', 2, 160000, 'stereo', 48000, 'MP3 Stereo')`, "fixture_audio_"+mediaID, mediaID)
			if err != nil {
				t.Fatalf("seed audio playback facts: %v", err)
			}
		} else {
			_, err := server.db.Exec(`INSERT INTO media_streams (id, media_id, stream_index, kind, codec, profile, bitrate, width, height, pixel_format, bit_depth, frame_rate, dynamic_range, display_title) VALUES (?, ?, 0, 'video', 'h264', 'main', 4000000, 1920, 1080, 'yuv420p', 8, 24, 'sdr', 'H264 Main 1080p'), (?, ?, 1, 'audio', 'aac', 'lc', 160000, 0, 0, '', 0, 0, '', 'AAC LC Stereo')`, "fixture_video_"+mediaID, mediaID, "fixture_audio_"+mediaID, mediaID)
			if err != nil {
				t.Fatalf("seed audiovisual playback facts: %v", err)
			}
			_, err = server.db.Exec(`UPDATE media_streams SET channels = 2, channel_layout = 'stereo', sample_rate = 48000 WHERE id = ?`, "fixture_audio_"+mediaID)
			if err != nil {
				t.Fatalf("complete audio playback facts: %v", err)
			}
		}
	}
	if _, err := server.db.Exec(`UPDATE media_streams SET codec = 'h264', profile = 'main', pixel_format = 'yuv420p', bit_depth = 8, frame_rate = 24, dynamic_range = 'sdr', bitrate = CASE WHEN bitrate > 0 THEN bitrate ELSE 4000000 END, width = CASE WHEN width > 0 THEN width ELSE 1920 END, height = CASE WHEN height > 0 THEN height ELSE 1080 END WHERE media_id = ? AND kind = 'video'`, mediaID); err != nil {
		t.Fatalf("complete H264 playback facts: %v", err)
	}
	if _, err := server.db.Exec(`UPDATE media_streams SET codec = 'aac', profile = 'lc', channels = 2, channel_layout = 'stereo', sample_rate = CASE WHEN sample_rate > 0 THEN sample_rate ELSE 48000 END, bitrate = CASE WHEN bitrate > 0 THEN bitrate ELSE 160000 END WHERE media_id = ? AND kind = 'audio'`, mediaID); err != nil {
		t.Fatalf("complete AAC playback facts: %v", err)
	}
	if mediaType == "track" || mediaType == "audiobook" {
		if _, err := server.db.Exec(`UPDATE media_streams SET codec = 'mp3', profile = '' WHERE media_id = ? AND kind = 'audio'`, mediaID); err != nil {
			t.Fatalf("complete MP3 audio-only playback facts: %v", err)
		}
	}
}

type testingT interface {
	Helper()
	Fatalf(string, ...any)
}

func authenticatedPlaybackRuntimeRequest(mediaID string) map[string]any {
	return map[string]any{
		"mediaId":       mediaID,
		"skipPreroll":   true,
		"clientProfile": authenticatedPlaybackRuntimeProfile(),
	}
}
