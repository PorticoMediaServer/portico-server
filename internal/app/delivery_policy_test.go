package app

import (
	"context"
	"net/http/httptest"
	"net/netip"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/playbackplan"
)

func TestPlaybackPolicyUsesMediaAppropriateNetworkDefaults(t *testing.T) {
	cellularVideo := defaultResolvedPlaybackPolicy("movie", "cellular")
	if cellularVideo.QualityProfile != "data_saver" || cellularVideo.MaxVideoBitrateMbps != 0 || cellularVideo.MaxVideoHeight != 0 || cellularVideo.AllowHDR {
		t.Fatalf("unexpected cellular video defaults: %+v", cellularVideo)
	}
	remoteAudio := defaultResolvedPlaybackPolicy("track", "remote")
	if remoteAudio.QualityProfile != "standard" || remoteAudio.MaxAudioBitrateKbps != 0 || remoteAudio.MaxVideoBitrateMbps != 0 {
		t.Fatalf("unexpected remote audio defaults: %+v", remoteAudio)
	}
	localVideo := defaultResolvedPlaybackPolicy("movie", "lan")
	if localVideo.NetworkClass != "local" || localVideo.QualityProfile != "original" || localVideo.MaxVideoBitrateMbps != 0 {
		t.Fatalf("unexpected LAN defaults: %+v", localVideo)
	}
}

func TestLiveHLSIsAnAdditiveResolvedPlaybackPolicyExtension(t *testing.T) {
	policy := defaultResolvedPlaybackPolicy("live_channel", playbackNetworkUnknown)
	if policy.DirectPlayPolicy == "" || policy.DirectStreamPolicy == "" || policy.TranscodePolicy == "" || policy.QualityProfile == "" || policy.NetworkClass == "" {
		t.Fatalf("generic resolved policy fields must remain authoritative: %+v", policy)
	}
	if policy.LiveHLS == nil {
		t.Fatal("expected additive Live HLS policy")
	}
	if policy.LiveHLS.CredentialQueryAllowed || policy.LiveHLS.AuthorizationTransport != "header_or_secure_http_only_cookie" || policy.LiveHLS.PlaylistScope != "playback_session" || policy.LiveHLS.SegmentScope != "playback_session" {
		t.Fatalf("unexpected Live HLS security extension: %+v", policy.LiveHLS)
	}
	if movie := defaultResolvedPlaybackPolicy("movie", playbackNetworkUnknown); movie.LiveHLS != nil {
		t.Fatalf("non-Live policy must not carry the protocol extension: %+v", movie.LiveHLS)
	}
}

func TestLiveTVQualityResourcesCannotExceedResolvedPolicy(t *testing.T) {
	policy := defaultResolvedPlaybackPolicy("live_channel", playbackNetworkRemote)
	policy.DeliveryProfile = resolvedDeliveryProfile("live_channel", policy)
	if selected := liveTVQualityForResolvedPolicy(policy); selected != "1080p-high" {
		t.Fatalf("remote standard policy selected %q", selected)
	}
	policy.MaxVideoBitrateMbps = 8
	policy.MaxVideoHeight = 1080
	authority, err := issueContinuousPlaybackQualityOffers("channel", "version", "source", policy, []string{"1080p-high", "720p-high", "480p"}, true)
	if err != nil {
		t.Fatalf("issue quality offers: %v", err)
	}
	for _, offer := range authority.set.Offers {
		if offer.Kind == playbackQualityKindOriginal {
			t.Fatalf("unbounded Original escaped the remote policy: %#v", authority.set.Offers)
		}
	}
	policy.MaxVideoBitrateMbps = 2
	policy.MaxVideoHeight = 480
	policy.DeliveryProfile = resolvedDeliveryProfile("live_channel", policy)
	if selected := liveTVQualityForResolvedPolicy(policy); selected != "480p" {
		t.Fatalf("explicit Live TV ceilings selected %q", selected)
	}
}

func TestTrustedProxyUsesForwardedPublicClientForRemotePolicy(t *testing.T) {
	server := &Server{cfg: config.Config{TrustedProxyCIDRs: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}}
	request := httptest.NewRequest("POST", "https://portico.example/api/playback-sessions", nil)
	request.RemoteAddr = "127.0.0.1:443"
	request.Header.Set("X-Forwarded-For", "203.0.113.42")
	if got := server.playbackNetworkClassForRequest(request); got != playbackNetworkRemote {
		t.Fatalf("trusted proxy public client classified as %q", got)
	}
	request.Header.Del("X-Forwarded-For")
	if got := server.playbackNetworkClassForRequest(request); got != playbackNetworkRemote {
		t.Fatalf("trusted proxy without client provenance classified as %q", got)
	}
}

func TestPublicHostnameDoesNotOverrideLANLocality(t *testing.T) {
	server := &Server{}
	request := httptest.NewRequest("POST", "https://media.direct.getportico.tv/api/playback-sessions", nil)
	request.Host = "media.direct.getportico.tv"
	request.RemoteAddr = "192.168.1.44:51822"
	if got := server.playbackNetworkClassForRequest(request); got != playbackNetworkLocal {
		t.Fatalf("public-direct request from LAN classified as %q", got)
	}

}

func TestEffectivePlaybackNetworkClassSeparatesTransportFromLocality(t *testing.T) {
	cases := []struct {
		name      string
		transport string
		locality  string
		want      string
	}{
		{name: "wifi local", transport: "wifi", locality: "local", want: "local"},
		{name: "cellular remote", transport: "cellular", locality: "remote", want: "cellular"},
		{name: "wifi remote", transport: "wifi", locality: "remote", want: "wifi"},
		{name: "unknown local", transport: "unknown", locality: "local", want: "local"},
		{name: "unknown remote", transport: "unknown", locality: "remote", want: "remote"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := effectivePlaybackNetworkClass(test.transport, test.locality); got != test.want {
				t.Fatalf("effective network class = %q, want %q", got, test.want)
			}
		})
	}
}

func TestObservedPublicIPHairpinIsLocalitySignal(t *testing.T) {
	server := newRemoteAccessUnitServer(t)
	settings, err := server.remoteAccessSettings()
	if err != nil {
		t.Fatalf("load remote access defaults: %v", err)
	}
	settings.Enabled = true
	settings.LastPublicIPAddress = "8.8.8.8"
	if err := server.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save observed public IP: %v", err)
	}
	settings, err = server.remoteAccessSettings()
	if err != nil || settings.LastPublicIPAddress != "8.8.8.8" {
		t.Fatalf("load observed public IP: settings=%+v err=%v", settings, err)
	}
	request := httptest.NewRequest("POST", "https://media.direct.getportico.tv/api/playback-sessions", nil)
	request.RemoteAddr = "8.8.8.8:51822"
	if got := server.playbackNetworkClassForRequest(request); got != playbackNetworkLocal {
		t.Fatalf("same-public-IP hairpin classified as %q", got)
	}
	request.RemoteAddr = "9.9.9.9:51822"
	if got := server.playbackNetworkClassForRequest(request); got != playbackNetworkRemote {
		t.Fatalf("different public viewer classified as %q", got)
	}
}

func TestResolvedPlaybackPolicyKeepsHairpinLocalityWhenClientReportsWiFi(t *testing.T) {
	server := newRemoteAccessUnitServer(t)
	settings, err := server.remoteAccessSettings()
	if err != nil {
		t.Fatalf("load remote access defaults: %v", err)
	}
	settings.Enabled = true
	settings.LastPublicIPAddress = "8.8.8.8"
	if err := server.saveRemoteAccessSettings(settings); err != nil {
		t.Fatalf("save observed public IP: %v", err)
	}
	request := httptest.NewRequest("POST", "https://media.direct.getportico.tv/api/playback-sessions", nil)
	request.RemoteAddr = "8.8.8.8:51822"
	policy, _ := server.resolvePlaybackPolicyForRequest(context.Background(), request, User{}, MediaItem{Type: "movie"}, PlaybackIntent{
		TransportClass: playbackNetworkWiFi,
	}, PlaybackClientProfile{})
	if policy.ServerLocality != playbackNetworkLocal || policy.NetworkClass != playbackNetworkLocal || policy.QualityProfile != "original" {
		t.Fatalf("Wi-Fi intent overwrote server-observed hairpin locality: %+v", policy)
	}
}

func TestDeliveryPreferenceProducesExplainableModes(t *testing.T) {
	item := MediaItem{Type: "movie", Streams: []Stream{
		{ID: "video", Kind: "video", Codec: "h264", Height: 1080, Bitrate: 4_000_000},
		{ID: "audio", Kind: "audio", Codec: "aac", Bitrate: 160_000},
	}}
	direct := PlaybackDecision{Mode: "direct_play", Reason: "source compatible", ReasonCodes: []string{"source_compatible"}}
	remux := applyResolvedDeliveryMode(direct, item, ResolvedPlaybackPolicy{DirectPlayPolicy: "allow", DirectStreamPolicy: "prefer", TranscodePolicy: "allow"}, true, true)
	if remux.Mode != "direct_stream" || !remux.RequiresRemux || remux.RequiresTranscode || !containsString(remux.ReasonCodes, "delivery_policy_direct_stream") {
		t.Fatalf("direct stream preference was not explainable: %+v", remux)
	}
	transcode := applyResolvedDeliveryMode(direct, item, ResolvedPlaybackPolicy{DirectPlayPolicy: "allow", DirectStreamPolicy: "allow", TranscodePolicy: "require"}, true, true)
	if transcode.Mode != "transcode_required" || !transcode.RequiresTranscode || !containsString(transcode.ReasonCodes, "delivery_policy_transcode_required") {
		t.Fatalf("transcode preference was not explainable: %+v", transcode)
	}
}

func TestResolvedDeliveryProfileMapsToEnforcedMediaPreset(t *testing.T) {
	video := defaultResolvedPlaybackPolicy("movie", "cellular")
	video.DeliveryProfile = resolvedDeliveryProfile("movie", video)
	if video.DeliveryProfile != "video-data-saver" || transcodeQualityForResolvedPolicy("movie", video) != "video-data-saver" {
		t.Fatalf("cellular video did not map to its enforced preset: %+v", video)
	}
	audio := defaultResolvedPlaybackPolicy("audiobook", "remote")
	audio.DeliveryProfile = resolvedDeliveryProfile("audiobook", audio)
	if audio.DeliveryProfile != "audio-standard" || transcodePresets[transcodeQualityForResolvedPolicy("audiobook", audio)].audioK != 192 {
		t.Fatalf("remote audiobook did not map to the 192 kbps preset: %+v", audio)
	}
}

func TestDirectStreamFailsClosedWithoutExactSeekEvidence(t *testing.T) {
	item := MediaItem{Type: "movie", Streams: []Stream{{ID: "video", Kind: "video", Codec: "h264", Width: 1920, Height: 1080, Bitrate: 4_000_000}, {ID: "audio", Kind: "audio", Codec: "aac", Bitrate: 160_000}}}
	decision := applyResolvedDeliveryMode(
		PlaybackDecision{Mode: "direct_stream", RequiresRemux: true}, item,
		ResolvedPlaybackPolicy{QualityProfile: "original", DeliveryProfile: "video-original", DirectPlayPolicy: "allow", DirectStreamPolicy: "allow", TranscodePolicy: "allow"}, false, true,
	)
	if decision.Mode != "transcode_required" || !decision.VideoTranscode || !containsString(decision.ReasonCodes, "direct_stream_exact_seek_evidence_missing") {
		t.Fatalf("copy remux was allowed without exact-seek evidence: %+v", decision)
	}
}

func TestDirectStreamUsesPositiveExactSeekEvidence(t *testing.T) {
	item := MediaItem{Type: "movie", Streams: []Stream{{ID: "video", Kind: "video", Codec: "h264", Width: 1920, Height: 1080, Bitrate: 4_000_000}, {ID: "audio", Kind: "audio", Codec: "aac", Bitrate: 160_000}}}
	decision := applyResolvedDeliveryMode(
		PlaybackDecision{Mode: "direct_stream", RequiresRemux: true}, item,
		ResolvedPlaybackPolicy{QualityProfile: "original", DeliveryProfile: "video-original", DirectPlayPolicy: "allow", DirectStreamPolicy: "allow", TranscodePolicy: "allow"}, true, true,
	)
	if decision.Mode != "direct_stream" || !decision.RequiresRemux || decision.RequiresTranscode {
		t.Fatalf("positive exact-seek remux evidence was not honored: %+v", decision)
	}
}

func TestResolvedDeliveryFailsClosedForUnknownSourceLimits(t *testing.T) {
	item := MediaItem{Type: "movie", Streams: []Stream{{ID: "video", Kind: "video", Codec: "h264"}, {ID: "audio", Kind: "audio", Codec: "aac"}}}
	decision := PlaybackDecision{Mode: "direct_play"}
	policy := ResolvedPlaybackPolicy{MaxVideoBitrateMbps: 8, MaxVideoHeight: 1080, MaxAudioBitrateKbps: 192, TranscodePolicy: "allow"}
	resolved := applyResolvedDeliveryMode(decision, item, policy, false, true)
	if resolved.Mode != "transcode_required" || !resolved.RequiresTranscode || !containsString(resolved.ReasonCodes, "delivery_policy_source_metadata_unknown") {
		t.Fatalf("unknown capped source failed open: %+v", resolved)
	}
}

func TestDirectStreamPreservesExactSeekReasonWhenSourceLimitsAreUnknown(t *testing.T) {
	item := MediaItem{Type: "movie", Streams: []Stream{{ID: "video", Kind: "video", Codec: "h264"}, {ID: "audio", Kind: "audio", Codec: "aac"}}}
	policy := ResolvedPlaybackPolicy{MaxVideoBitrateMbps: 8, MaxVideoHeight: 1080, MaxAudioBitrateKbps: 192, TranscodePolicy: "allow"}
	resolved := applyResolvedDeliveryMode(PlaybackDecision{Mode: "direct_stream", RequiresRemux: true}, item, policy, false, true)
	if resolved.Mode != "transcode_required" || !containsString(resolved.ReasonCodes, "direct_stream_exact_seek_evidence_missing") || !containsString(resolved.ReasonCodes, "delivery_policy_source_metadata_unknown") {
		t.Fatalf("conservative fallback lost an actionable reason: %+v", resolved)
	}
}

func TestResolvedDeliveryFailsClosedWithoutPrimaryStreamMetadata(t *testing.T) {
	policy := ResolvedPlaybackPolicy{MaxVideoBitrateMbps: 8, MaxVideoHeight: 1080, TranscodePolicy: "allow"}
	for _, item := range []MediaItem{{Type: "movie"}, {Type: "episode", Streams: []Stream{{Kind: "video", Codec: "h264"}}}} {
		resolved := applyResolvedDeliveryMode(PlaybackDecision{Mode: "direct_play"}, item, policy, false, true)
		if resolved.Mode != "transcode_required" || !containsString(resolved.ReasonCodes, "delivery_policy_source_metadata_unknown") {
			t.Fatalf("missing primary metadata failed open for %+v: %+v", item, resolved)
		}
	}
}

func TestResolvedVideoProfileSatisfiesMixedAudioAndVideoCaps(t *testing.T) {
	policy := ResolvedPlaybackPolicy{MaxVideoBitrateMbps: 20, MaxVideoHeight: 2160, MaxAudioBitrateKbps: 128}
	if profile := resolvedDeliveryProfile("movie", policy); profile != "video-data-saver" {
		t.Fatalf("mixed cap selected %q, expected video-data-saver", profile)
	}
	policy = ResolvedPlaybackPolicy{MaxAudioBitrateKbps: 192}
	if profile := resolvedDeliveryProfile("episode", policy); profile != "video-standard" {
		t.Fatalf("audio-only video cap selected %q, expected video-standard", profile)
	}
}

func TestResolvedDeliveryContradictoryPoliciesNeverTranscode(t *testing.T) {
	item := MediaItem{Type: "movie", Streams: []Stream{{ID: "video", Kind: "video", Codec: "h264", Bitrate: 4_000_000, Height: 1080}}}
	for _, mode := range []string{"direct_play", "optimized_version", "direct_stream"} {
		decision := PlaybackDecision{Mode: mode}
		policy := ResolvedPlaybackPolicy{DirectPlayPolicy: "never", DirectStreamPolicy: "never", TranscodePolicy: "never"}
		resolved := applyResolvedDeliveryMode(decision, item, policy, false, true)
		if resolved.Mode != "unavailable" || !containsString(resolved.ReasonCodes, "delivery_policy_transcode_disabled") {
			t.Fatalf("mode %s bypassed transcode prohibition: %+v", mode, resolved)
		}
	}
}

func TestTextSubtitleChoiceRequiresRenegotiationUnderTranscodeNeverPolicy(t *testing.T) {
	plan := testPlaybackExecutionPlan(t, func(plan *playbackExecutionPlan) {
		plan.Plan.Mode, plan.Plan.Protocol, plan.Plan.Container = playbackplan.DirectPlay, "http", "mp4"
	})
	resources := playbackResourcesForResponse("/api/media/movie-text-subtitle/stream", plan)
	if len(resources) != 1 || !resources[0].Default || resources[0].SubtitleMode != "off" {
		t.Fatalf("response published an unsealed subtitle resource: %#v", resources)
	}
}

func TestQualityOffersDoNotPublishAlternateDirectResources(t *testing.T) {
	plan := testPlaybackExecutionPlan(t, func(plan *playbackExecutionPlan) {
		plan.Plan.Mode, plan.Plan.Protocol, plan.Plan.Container = playbackplan.DirectPlay, "http", "mp4"
	})
	resources := playbackResourcesForResponse("/api/media/movie-direct-auto/stream", plan)

	if len(resources) != 1 || !resources[0].Default || resources[0].SourceURL != "/api/media/movie-direct-auto/stream" || resources[0].StreamFormat != "http" {
		t.Fatalf("response published resources outside the active sealed plan: %#v", resources)
	}
}
