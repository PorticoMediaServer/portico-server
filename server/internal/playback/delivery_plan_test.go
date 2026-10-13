package playback

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
)

// openPolicy is the resolution a viewer gets with no stored preferences on an
// unclamped server: everything allowed, nothing narrowed.
func openPolicy() ResolvedDeliveryPolicy {
	return ResolveDeliveryPolicy(nil, NetworkLocal, LocalityLocal, "ethernet", DefaultDeliveryConfiguration())
}

func autoTarget(policy ResolvedDeliveryPolicy, height int) QualityTarget {
	return QualityRung{ID: "auto", Kind: QualityAutomatic}.Target(policy, height)
}

func planFor(t *testing.T, in DeliveryInput) DeliveryPlan {
	t.Helper()
	p, err := planDelivery(in)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDeliveryRouteSelectionAndExecutableCodecPolicy(t *testing.T) {
	policy := openPolicy()
	cases := []struct{ container, video, audio, mode, strategy, videoAction, audioAction string }{
		{"mp4", "h264", "aac", "direct", "original", "original", "original"},
		{"mov", "h264", "mp3", "direct", "original", "original", "original"},
		{"m4b", "", "aac", "direct", "original", "original", "original"},
		{"mp3", "", "mp3", "direct", "original", "original", "original"},
		// The container was the only obstacle: both streams are copied.
		{"mkv", "h264", "aac", "hls", "copy_remux", "copy", "copy"},
		{"avi", "h264", "mp3", "hls", "copy_remux", "copy", "copy"},
		{"mp4", "h264", "ac3", "hls", "audio_conversion", "copy", "convert"},
		{"mkv", "vp9", "opus", "hls", "video_conversion", "convert", "convert"},
		{"mp4", "hevc", "aac", "hls", "video_conversion", "convert", "convert"},
		{"flac", "", "flac", "hls", "audio_conversion", "none", "convert"},
	}
	for _, tt := range cases {
		t.Run(tt.container+tt.video+tt.audio, func(t *testing.T) {
			p := planFor(t, DeliveryInput{SourceID: "source", Container: tt.container, VideoCodec: tt.video, AudioCodec: tt.audio, Duration: 100, Policy: policy, Target: autoTarget(policy, 0), Config: DefaultDeliveryConfiguration(), TranscodingEnabled: true})
			if p.Mode != tt.mode || p.Strategy != DeliveryStrategy(tt.strategy) || p.VideoAction != tt.videoAction || p.AudioAction != tt.audioAction || p.Profile != DeliveryProfileLegacy {
				t.Fatal(p)
			}
			if len(p.ReasonCodes) == 0 {
				t.Fatal("plan published no reason codes")
			}
			_, args, stages, err := p.CodecArgs()
			if tt.mode != "hls" {
				if err == nil {
					t.Fatal("direct accepted as conversion")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(args, " ")
			if tt.audioAction == "convert" && !strings.Contains(joined, "-c:a aac -b:a 192k -ac 2") {
				t.Fatal(args)
			}
			if tt.strategy == "copy_remux" && !reflect.DeepEqual(args, []string{"-c:v", "copy", "-c:a", "copy"}) {
				t.Fatal(args)
			}
			// These sources have no observed sample rate, so the conversion names one.
			if tt.videoAction == "copy" && tt.audioAction == "convert" && !reflect.DeepEqual(args, []string{"-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-ac", "2", "-ar", "48000"}) {
				t.Fatal(args)
			}
			if tt.videoAction == "none" && strings.Contains(joined, "-c:v") {
				t.Fatal(args)
			}
			if tt.videoAction == "convert" {
				if !strings.Contains(joined, "-c:v libx264") || !strings.Contains(joined, "-preset veryfast") {
					t.Fatal(args)
				}
				var encode bool
				for _, s := range stages {
					if s.Operation == decoder.StageEncode && s.Execution == decoder.ExecutionSoftware {
						encode = true
					}
				}
				if !encode {
					t.Fatal("conversion published no software encode stage", stages)
				}
			}
		})
	}
	remote := planFor(t, DeliveryInput{SourceID: "source", Container: "strm", VideoCodec: "h264", AudioCodec: "aac", Duration: 10, Remote: true, Policy: policy, Target: autoTarget(policy, 0), Config: DefaultDeliveryConfiguration(), TranscodingEnabled: true})
	if remote.Mode != "remote" || remote.Strategy != DeliveryOriginal || remote.ReasonCodes[0] != ReasonRemotePreflightCompatible {
		t.Fatal(remote)
	}

}

// Remux is chosen only when the container is genuinely the only obstacle, and
// only when the owner and the viewer both permit it.
func TestDeliveryCopyRemuxSelection(t *testing.T) {
	base := DeliveryInput{SourceID: "source", Container: "mkv", VideoCodec: "h264", AudioCodec: "aac", Duration: 100, Height: 1080, TranscodingEnabled: true}

	open := openPolicy()
	base.Policy, base.Target, base.Config = open, autoTarget(open, 1080), DefaultDeliveryConfiguration()
	if p := planFor(t, base); p.Strategy != DeliveryCopyRemux {
		t.Fatal("container-only obstacle did not remux", p.Strategy)
	}

	// The owner turned remux off: the same source must convert instead.
	cfg := DefaultDeliveryConfiguration()
	cfg.RemuxEnabled = false
	off := ResolveDeliveryPolicy(nil, NetworkLocal, LocalityLocal, "ethernet", cfg)
	disabled := base
	disabled.Policy, disabled.Config, disabled.Target = off, cfg, autoTarget(off, 1080)
	// With the wrapper change refused, the cheapest admissible route re-encodes
	// only the audio and still copies the video.
	p := planFor(t, disabled)
	if p.Strategy != DeliveryAudioConversion || p.VideoAction != "copy" {
		t.Fatal("remux disabled but still selected", p.Strategy, p.VideoAction)
	}
	if !contains(p.ReasonCodes, ReasonDirectStreamRefused) {
		t.Fatal("refusal not published", p.ReasonCodes)
	}

	// A rung that constrains the picture is not a container problem.
	rung := base
	rung.Target = QualityRung{ID: "480p", Kind: QualityFixed, TargetDisplayHeight: 480, MaxVideoBitrateBPS: 2_000_000, MaxAudioBitrateBPS: 96_000}.Target(open, 1080)
	if p := planFor(t, rung); p.Strategy != DeliveryVideoConversion || p.TargetHeight != 480 || p.VideoBitrateBPS != 2_000_000 {
		t.Fatal("fixed rung did not force conversion", p)
	}

	// Direct play is still preferred over remux when the source already fits.
	direct := base
	direct.Container = "mp4"
	if p := planFor(t, direct); p.Strategy != DeliveryOriginal || p.Mode != "direct" {
		t.Fatal("direct-playable source remuxed", p)
	}
}

func contains(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// The policy matrix: network class crossed with preference and server clamp.
func TestDeliveryToneMapArguments(t *testing.T) {
	cfg := DefaultDeliveryConfiguration()
	cfg.ToneMapAlgorithm = "mobius"
	sdrLane := ResolveDeliveryPolicy(stubPreferences{"quality.cellular.allowHDR": false, "quality.cellular.maxVideoHeight": 2160}, NetworkCellular, LocalityRemote, "cellular", cfg)
	in := DeliveryInput{SourceID: "s", Container: "mkv", VideoCodec: "hevc", AudioCodec: "aac", Duration: 10, Height: 2160, ColorTransfer: "smpte2084", ColorPrimaries: "bt2020", Policy: sdrLane, Target: autoTarget(sdrLane, 2160), Config: cfg, TranscodingEnabled: true}
	p := planFor(t, in)
	if !p.ToneMap || p.ToneMapAlgorithm != "mobius" {
		t.Fatal("HDR source on an SDR lane did not tone map", p)
	}
	if !contains(p.ReasonCodes, ReasonHDRNotAllowed) {
		t.Fatal("tone-map reason not published", p.ReasonCodes)
	}
	_, args, stages, err := p.CodecArgs()
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "zscale=t=linear:npl=100") || !strings.Contains(joined, "tonemap=tonemap=mobius:desat=0") || !strings.Contains(joined, "zscale=t=bt709:m=bt709:r=tv") {
		t.Fatal("software tone-map graph not generated", args)
	}
	var toneStage bool
	for _, s := range stages {
		if s.Operation == decoder.StageToneMap && s.Execution == decoder.ExecutionSoftware {
			toneStage = true
		}
	}
	if !toneStage {
		t.Fatal("tone-map stage not published", stages)
	}

	// A lane that allows HDR changes nothing for a device that cannot take the
	// source: the picture is still converted to 8-bit BT.709, and an HDR picture
	// converted without tone mapping is grey and washed out. The old planner read
	// "HDR allowed" as "do not tone map" and produced exactly that.
	hdrLane := openPolicy()
	in.Policy, in.Target = hdrLane, autoTarget(hdrLane, 2160)
	if p := planFor(t, in); !p.ToneMap || p.VideoAction != "convert" {
		t.Fatal("HDR source converted to SDR without tone mapping", p)
	}

	// Owner tone mapping off leaves the picture alone even on an SDR lane.
	cfg.ToneMapping = false
	in.Config, in.Policy, in.Target = cfg, sdrLane, autoTarget(sdrLane, 2160)
	p = planFor(t, in)
	if p.ToneMap {
		t.Fatal("owner disabled tone mapping but it was applied")
	}
	if p.Trace == nil || !contains(p.Trace.Notes, NoteToneMapOwnerOff) {
		t.Fatal("the trace does not say why an HDR picture was left unmapped", p.Trace)
	}
}

// stubPreferences is the registry accessor shape, with only the keys a test sets.
type stubPreferences map[string]any

func (s stubPreferences) Bool(key string) bool {
	v, ok := s[key].(bool)
	return ok && v
}
func (s stubPreferences) Int(key string) int {
	v, _ := s[key].(int)
	return v
}
func (s stubPreferences) Text(key string) string {
	v, _ := s[key].(string)
	return v
}

func TestDeliverySessionSnapshotRestartPinAndLegacy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	db, err := persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, item, token := catalogFixture(t, db, "lib", "/test", compactcatalog.Movie, "item", "Film", compactcatalog.Asset{Path: "/test/file.mp4", Size: 20, ModifiedNS: 30, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080, Duration: 100})
	principal := identity.Principal{Hash: "session", Viewer: identity.Viewer{AccountID: "owner", ProfileID: "profile", Authority: "local", Role: "owner"}}
	session, err := New(db).Create(principal, item, "auto", "request")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := loadDeliveryPlan(db, session.ID)
	if err != nil || plan == nil {
		t.Fatal(plan, err)
	}
	if plan.SourceSize != 20 || plan.SourceModifiedNS != 30 || plan.SourceID != token || plan.FactsRevision != 0 || plan.Strategy != DeliveryOriginal {
		t.Fatal(plan)
	}
	if plan.QualityID != "auto" || plan.Policy == nil || plan.Policy.NetworkClass != NetworkUnknown {
		t.Fatal("resolved policy not persisted with the plan", plan.QualityID, plan.Policy)
	}
	// Later descriptive reprobe cannot rewrite the decision for an existing session.
	updateCatalogAsset(t, db, token, func(a *compactcatalog.Asset) { a.VideoCodec = "hevc" })
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = persistence.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	after, err := loadDeliveryPlan(db, session.ID)
	if err != nil || !reflect.DeepEqual(plan, after) {
		t.Fatal(after, err)
	}
	repeated, err := New(db).Create(principal, item, "auto", "request")
	if err != nil || repeated.ID != session.ID || repeated.Mode != "direct" {
		t.Fatal(repeated, err)
	}
	updateCatalogAsset(t, db, token, func(a *compactcatalog.Asset) { a.ModifiedNS = 31 })
	if _, err = loadDeliveryPlan(db, session.ID); err != ErrStaleOffer {
		t.Fatal("changed fingerprint accepted", err)
	}
	if _, err = db.Exec(`DELETE FROM playback_delivery_plans WHERE session_id=?`, session.ID); err != nil {
		t.Fatal(err)
	}
	if legacy, err := loadDeliveryPlan(db, session.ID); err != nil || legacy != nil {
		t.Fatal("legacy fallback not explicit", legacy, err)
	}
}
