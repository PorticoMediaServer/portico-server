package app

import (
	"fmt"
	"strings"
)

type playbackCapabilityDiagnostic struct {
	ClientFamily  string
	Platform      string
	SchemaVersion string
	ClientVersion string
	TupleCount    int
	Tuples        []string
}

func playbackCapabilityDiagnosticFor(profile PlaybackClientProfile) playbackCapabilityDiagnostic {
	diagnostic := playbackCapabilityDiagnostic{
		ClientFamily:  playbackDiagnosticToken(profile.ClientFamily),
		Platform:      playbackDiagnosticToken(profile.Platform),
		SchemaVersion: playbackDiagnosticToken(profile.CapabilitySchemaVersion),
		ClientVersion: playbackDiagnosticToken(profile.ClientVersion),
	}
	for _, evidence := range profile.CapabilityEvidence {
		diagnostic.TupleCount += len(evidence.Tuples)
		for _, tuple := range evidence.Tuples {
			if len(diagnostic.Tuples) >= 48 {
				continue
			}
			diagnostic.Tuples = append(diagnostic.Tuples, fmt.Sprintf("kind=%s,protocol=%s,container=%s,video=%s:%s:%s:%s:%s:%d:%dx%d:%g,audio=%s:%s:%s:%d:%s,subtitle=%s:%s:%s",
				playbackDiagnosticToken(tuple.MediaKind), playbackDiagnosticToken(tuple.Protocol), playbackDiagnosticToken(tuple.Container),
				playbackDiagnosticToken(tuple.Video.Codec), playbackDiagnosticToken(tuple.Video.Profile), playbackDiagnosticToken(tuple.Video.PixelFormat),
				playbackDiagnosticToken(tuple.Video.Chroma), playbackDiagnosticToken(tuple.Video.DynamicRange), tuple.Video.BitDepth,
				tuple.Video.MaxWidth, tuple.Video.MaxHeight, tuple.Video.MaxFrameRate,
				playbackDiagnosticToken(tuple.Audio.Codec), playbackDiagnosticToken(tuple.Audio.Profile), playbackDiagnosticToken(tuple.Audio.Layout),
				tuple.Audio.MaxChannels, playbackDiagnosticToken(tuple.Audio.Route), playbackDiagnosticToken(tuple.Subtitle.Mode),
				playbackDiagnosticToken(tuple.Subtitle.Codec), playbackDiagnosticToken(tuple.Subtitle.Kind)))
		}
	}
	return diagnostic
}

func playbackDiagnosticToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "none"
	}
	if len(value) > 48 {
		return "invalid"
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || strings.ContainsRune("._+-", char) {
			continue
		}
		return "invalid"
	}
	return value
}
