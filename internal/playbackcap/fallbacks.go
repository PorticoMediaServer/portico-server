package playbackcap

import (
	"errors"
	"fmt"
	"time"
)

// FallbackBand is a reviewed, bounded static assertion of potential
// compatibility. Static bands intentionally expose only Portico's H.264/AAC
// delivery baseline. Rich codec, HDR, DV, multichannel, passthrough and object
// audio assertions require authenticated runtime/native evidence.
type FallbackBand struct {
	Name     string
	Evidence Evidence
}

func (b FallbackBand) matches(client Client) bool { return matchesClient(b.Evidence, client) }

var fallbackReviewDate = time.Date(2026, time.August, 6, 0, 0, 0, 0, time.UTC)

func baselineTuple() DeliveryTuple {
	return DeliveryTuple{Kind: MediaAudiovisual, Protocol: "http", Container: "mp4", Video: Video{Codec: "h264", Profile: "main", PixelFormat: "yuv420p", Chroma: "4:2:0", HDR: "sdr", BitDepth: 8, MaxWidth: 1920, MaxHeight: 1080, MaxFrameRate: 30}, Audio: Audio{Codec: "aac", Profile: "lc", Layout: "stereo", Route: "decode", MaxChannels: 2}, Subtitle: Subtitle{Mode: SubtitleNone}}
}
func hlsTuple() DeliveryTuple {
	t := baselineTuple()
	t.Protocol = "hls"
	// Portico's H.264 HLS executor emits MPEG-TS segments. HEVC remuxes use
	// fragmented MP4 and must be declared by their exact runtime tuple.
	t.Container = "mpegts"
	return t
}
func withSubtitle(t DeliveryTuple, codec, kind string, mode SubtitleMode) DeliveryTuple {
	t.Subtitle = Subtitle{Codec: codec, Kind: kind, Mode: mode}
	return t
}
func textSubtitleTuple() DeliveryTuple {
	return withSubtitle(baselineTuple(), "webvtt", "text", SubtitleNative)
}
func convertedTextTuple() DeliveryTuple {
	return withSubtitle(baselineTuple(), "subrip", "text", SubtitleConvert)
}
func bitmapBurnTuple() DeliveryTuple {
	return withSubtitle(baselineTuple(), "hdmv_pgs_subtitle", "bitmap", SubtitleBurn)
}
func audioMP3Tuple() DeliveryTuple {
	return DeliveryTuple{Kind: MediaAudio, Protocol: "http", Container: "mp3", Audio: Audio{Codec: "mp3", Layout: "stereo", Route: "decode", MaxChannels: 2}, Subtitle: Subtitle{Mode: SubtitleNone}}
}
func audioAACTuple() DeliveryTuple {
	return DeliveryTuple{Kind: MediaAudio, Protocol: "http", Container: "m4a", Audio: Audio{Codec: "aac", Profile: "lc", Layout: "stereo", Route: "decode", MaxChannels: 2}, Subtitle: Subtitle{Mode: SubtitleNone}}
}

// Rich tuples are test/runtime-evidence builders, never static fallbacks.
func hevcTuple() DeliveryTuple {
	t := baselineTuple()
	t.Video = Video{Codec: "hevc", Profile: "main10", PixelFormat: "yuv420p10le", Chroma: "4:2:0", HDR: "sdr", BitDepth: 10, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60}
	return t
}
func av1Tuple() DeliveryTuple {
	t := baselineTuple()
	t.Video = Video{Codec: "av1", Profile: "main", PixelFormat: "yuv420p10le", Chroma: "4:2:0", HDR: "sdr", BitDepth: 10, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60}
	return t
}
func staticBand(name, family, platform, min, max string, tuples ...DeliveryTuple) FallbackBand {
	return FallbackBand{Name: name, Evidence: Evidence{ID: "static:" + name, Client: Client{Family: family, Platform: platform}, Tuples: tuples, Provenance: Provenance{Source: SourceStaticFallback, Confidence: ConfidenceLow, Producer: "portico-reviewed-fallbacks", MinVersion: min, MaxVersion: max, ReviewedAt: fallbackReviewDate}}}
}

func standardTuples() []DeliveryTuple {
	return []DeliveryTuple{baselineTuple(), textSubtitleTuple(), convertedTextTuple(), bitmapBurnTuple(), hlsTuple(), withSubtitle(hlsTuple(), "webvtt", "text", SubtitleNative), audioMP3Tuple(), audioAACTuple()}
}
func progressiveTuples() []DeliveryTuple {
	return []DeliveryTuple{baselineTuple(), textSubtitleTuple(), convertedTextTuple(), bitmapBurnTuple(), audioMP3Tuple(), audioAACTuple()}
}
func browserManagedHLSTuples() []DeliveryTuple {
	return append(progressiveTuples(), hlsTuple(), withSubtitle(hlsTuple(), "webvtt", "text", SubtitleNative))
}
func dlnaTuples() []DeliveryTuple {
	return []DeliveryTuple{baselineTuple(), audioMP3Tuple(), audioAACTuple()}
}
func unknownHLSTuples() []DeliveryTuple {
	return []DeliveryTuple{baselineTuple(), hlsTuple(), audioMP3Tuple(), audioAACTuple()}
}

// DefaultFallbackBands names every day-one family independently so version and
// model-year review can evolve without one family inheriting another's claims.
// Catch-all entries must remain last and baseline-only.
func DefaultFallbackBands() []FallbackBand {
	t := standardTuples()
	p := progressiveTuples()
	b := browserManagedHLSTuples()
	d := dlnaTuples()
	u := unknownHLSTuples()
	var out []FallbackBand
	add := func(n, f, p, min, max string, x []DeliveryTuple) {
		out = append(out, staticBand(n, f, p, min, max, x...))
	}
	// These bounded Portico Web bands include the reviewed H.264/AAC MPEG-TS
	// baseline consumed through the bundled managed HLS player. This does not
	// trust request-supplied browser probes or assert richer codec support.
	add("chromium-120-159", "chromium", "web", "120", "159.99", b)
	add("edge-120-159", "edge", "web", "120", "159.99", b)
	add("safari-17-19", "safari", "web", "17", "19.99", t)
	add("firefox-120-159", "firefox", "web", "120", "159.99", b)
	add("avkit-ios-17-19", "avkit", "ios", "17", "19.99", t)
	add("avkit-ipados-17-19", "avkit", "ipados", "17", "19.99", t)
	add("avkit-tvos-17-19", "avkit", "tvos", "17", "19.99", t)
	add("media3-android-10-16", "media3", "android", "10", "16.99", t)
	add("media3-android-tv-10-16", "media3", "android-tv", "10", "16.99", t)
	add("fire-tv-7-8", "fire-tv", "fireos", "7", "8.99", t)
	add("fire-tv-14", "fire-tv", "fireos", "14", "14.99", t)
	add("roku-os-11-14", "roku", "roku", "11", "14.99", t)
	add("tizen-2019-2021", "tizen", "tizen", "2019", "2021.99", t)
	add("tizen-2022-2026", "tizen", "tizen", "2022", "2026.99", t)
	add("webos-4-6", "webos", "webos", "4", "6.99", t)
	add("webos-22-26", "webos", "webos", "22", "26.99", t)
	add("dlna-best-effort", "dlna", "dlna", "", "", d)
	for _, v := range []struct {
		n, f, p string
		tuples  []DeliveryTuple
	}{{"chromium-unknown-conservative", "chromium", "web", p}, {"edge-unknown-conservative", "edge", "web", p}, {"safari-unknown-conservative", "safari", "web", u}, {"firefox-unknown-conservative", "firefox", "web", p}, {"avkit-ios-unknown-conservative", "avkit", "ios", u}, {"avkit-ipados-unknown-conservative", "avkit", "ipados", u}, {"avkit-tvos-unknown-conservative", "avkit", "tvos", u}, {"media3-android-unknown-conservative", "media3", "android", u}, {"media3-tv-unknown-conservative", "media3", "android-tv", u}, {"fire-tv-unknown-conservative", "fire-tv", "fireos", u}, {"roku-unknown-conservative", "roku", "roku", u}, {"cast-unknown-conservative", "cast", "cast", u}, {"tizen-unknown-conservative", "tizen", "tizen", u}, {"webos-unknown-conservative", "webos", "webos", u}} {
		add(v.n, v.f, v.p, "", "", v.tuples)
	}
	return out
}

// ValidateFallbackBands rejects ambiguous bounded bands and ensures broad
// unknown/future fallbacks cannot accidentally inherit rich tuples.
func ValidateFallbackBands(bands []FallbackBand, now time.Time) error {
	if len(bands) == 0 {
		return errors.New("fallback catalog is empty")
	}
	seen := map[string]bool{}
	catchall := map[string]bool{}
	for i, b := range bands {
		if seen[b.Name] {
			return fmt.Errorf("duplicate fallback band %q", b.Name)
		}
		seen[b.Name] = true
		if err := b.Evidence.Validate(now); err != nil {
			return fmt.Errorf("fallback %q: %w", b.Name, err)
		}
		key := normalize(b.Evidence.Client.Family) + "|" + normalize(b.Evidence.Client.Platform)
		p := b.Evidence.Provenance
		broad := p.MinVersion == "" && p.MaxVersion == ""
		if broad {
			if catchall[key] {
				return fmt.Errorf("multiple catch-all bands for %s", key)
			}
			catchall[key] = true
			allowed := unknownHLSTuples()
			if key == "dlna|dlna" {
				allowed = dlnaTuples()
			} else if key == "chromium|web" || key == "edge|web" || key == "firefox|web" {
				allowed = progressiveTuples()
			}
			if !sameTupleSet(b.Evidence.Tuples, allowed) {
				return fmt.Errorf("catch-all %q must use the conservative %s baseline", b.Name, catchallBaselineName(key))
			}
		} else if catchall[key] {
			return fmt.Errorf("bounded band %q appears after catch-all", b.Name)
		}
		for j := 0; j < i; j++ {
			a := bands[j]
			if normalize(a.Evidence.Client.Family) != normalize(b.Evidence.Client.Family) || normalize(a.Evidence.Client.Platform) != normalize(b.Evidence.Client.Platform) {
				continue
			}
			ap, bp := a.Evidence.Provenance, b.Evidence.Provenance
			if ap.MinVersion == "" && ap.MaxVersion == "" || broad {
				continue
			}
			if rangesOverlap(ap.MinVersion, ap.MaxVersion, bp.MinVersion, bp.MaxVersion) {
				return fmt.Errorf("bounded bands %q and %q overlap", a.Name, b.Name)
			}
		}
	}
	return nil
}
func catchallBaselineName(key string) string {
	if key == "dlna|dlna" {
		return "progressive-only"
	}
	if key == "chromium|web" || key == "edge|web" || key == "firefox|web" {
		return "browser-progressive-only"
	}
	return "progressive-and-HLS"
}
func rangesOverlap(amin, amax, bmin, bmax string) bool {
	c1, o1 := compareVersions(amin, bmax)
	c2, o2 := compareVersions(bmin, amax)
	return o1 && o2 && c1 <= 0 && c2 <= 0
}
func sameTupleSet(a, b []DeliveryTuple) bool {
	if len(a) != len(b) {
		return false
	}
	used := make([]bool, len(b))
	for _, x := range a {
		found := -1
		for j, y := range b {
			if !used[j] && fmt.Sprintf("%#v", x) == fmt.Sprintf("%#v", y) {
				found = j
				break
			}
		}
		if found < 0 {
			return false
		}
		used[found] = true
	}
	return true
}
