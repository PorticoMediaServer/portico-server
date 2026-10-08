package playback

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

// ClientProfileVersion is the wire version of the capability document. A device
// publishes one document per sign-in; planning reads the stored copy, so every
// route that creates a session sees the same facts without the client resending
// them. A version this server does not know is refused rather than half-read.
const ClientProfileVersion = 1

// MaxClientProfileBytes bounds one published document.
const MaxClientProfileBytes = 32 << 10

// Evidence says how a fact was obtained. "declared" is the app's own table for
// its platform; "probed" was asked of the device at runtime (MediaCapabilities,
// MediaCodecList, AVFoundation, the HDMI sink). Planning trusts both equally; the
// distinction exists so a wrong decision can be traced to its source.
const (
	EvidenceDeclared = "declared"
	EvidenceProbed   = "probed"
	EvidenceMixed    = "mixed"
	EvidenceBuiltin  = "builtin"
)

// Transports a device can take. "direct" is the original file by byte range.
const (
	TransportDirect  = "direct"
	TransportHLSTS   = "hls_ts"
	TransportHLSFMP4 = "hls_fmp4"
)

type ClientIdentity struct {
	Family          string `json:"family"`
	Platform        string `json:"platform"`
	PlatformVersion string `json:"platformVersion"`
	OS              string `json:"os"`
	OSVersion       string `json:"osVersion"`
	Model           string `json:"model"`
	App             string `json:"app"`
	AppVersion      string `json:"appVersion"`
	Engine          string `json:"engine"`
	EngineVersion   string `json:"engineVersion"`
}

// ClientDisplay describes the panel or HDMI sink. It is diagnostic: what a
// device can present acceptably is carried per codec in DynamicRanges, because a
// device with an SDR panel may still tone map HDR well by itself.
type ClientDisplay struct {
	Width         int      `json:"width"`
	Height        int      `json:"height"`
	DynamicRanges []string `json:"dynamicRanges"`
	MaxFrameRate  float64  `json:"maxFrameRate"`
}

type ClientVideoCodec struct {
	Codec               string   `json:"codec"`
	Profiles            []string `json:"profiles"`
	MaxLevel            int      `json:"maxLevel"`
	BitDepths           []int    `json:"bitDepths"`
	MaxWidth            int      `json:"maxWidth"`
	MaxHeight           int      `json:"maxHeight"`
	MaxFrameRate        float64  `json:"maxFrameRate"`
	MaxBitrateBPS       int      `json:"maxBitrateBps"`
	DynamicRanges       []string `json:"dynamicRanges"`
	DolbyVisionProfiles []int    `json:"dolbyVisionProfiles"`
	Interlaced          bool     `json:"interlaced"`
	Evidence            string   `json:"evidence"`
}

type ClientAudioCodec struct {
	Codec         string   `json:"codec"`
	MaxChannels   int      `json:"maxChannels"`
	MaxSampleRate int      `json:"maxSampleRate"`
	ObjectAudio   []string `json:"objectAudio"`
	// Passthrough means the device hands the bitstream to a receiver instead of
	// decoding it. Such a codec is only usable while the output route can take it.
	Passthrough bool   `json:"passthrough"`
	Evidence    string `json:"evidence"`
}

type ClientTransport struct {
	Transport  string   `json:"transport"`
	Containers []string `json:"containers"`
	Video      []string `json:"video"`
	Audio      []string `json:"audio"`
}

type ClientAudioOutput struct {
	Route       string `json:"route"`
	MaxChannels int    `json:"maxChannels"`
	Spatial     bool   `json:"spatial"`
}

// ClientSubtitles lists what the device draws itself. Anything not listed is
// converted to text where that is lossless enough, and burned in otherwise.
type ClientSubtitles struct {
	Text   []string `json:"text"`
	Styled []string `json:"styled"`
	Bitmap []string `json:"bitmap"`
}

type ClientProfile struct {
	Version     int                `json:"version"`
	Client      ClientIdentity     `json:"client"`
	Evidence    string             `json:"evidence"`
	Display     ClientDisplay      `json:"display"`
	Video       []ClientVideoCodec `json:"video"`
	Audio       []ClientAudioCodec `json:"audio"`
	AudioOutput ClientAudioOutput  `json:"audioOutput"`
	Transports  []ClientTransport  `json:"transports"`
	Subtitles   ClientSubtitles    `json:"subtitles"`
	// MaxBitrateBPS is the device's own ceiling (a slow Wi-Fi chip, a measured
	// link). Zero means none; the network policy still applies on top of it.
	MaxBitrateBPS int `json:"maxBitrateBps"`
	// EmbeddedAudioSwitching is true when the engine can choose between audio
	// tracks inside an original file. Without it, only the first track of an
	// original is reachable, and any other choice needs an HLS route.
	EmbeddedAudioSwitching bool `json:"embeddedAudioSwitching"`
	// AudioDecode is what the client's own music engine decodes (spec §3/§18.1),
	// for a client that sends it here rather than in v1 capabilities (web).
	AudioDecode []ClientAudioDecode `json:"audioDecode,omitempty"`

	// Revision is a digest of the accepted document, assigned by the server.
	Revision string `json:"-"`
}

var (
	ErrClientProfile        = errors.New("The device capability profile is not valid.")
	ErrClientProfileVersion = errors.New("This server does not understand that capability profile version.")
	profileToken            = regexp.MustCompile(`^[a-z0-9][a-z0-9_.+-]{0,31}$`)
)

func profileText(s string, max int) bool {
	if len(s) > max {
		return false
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return false
		}
	}
	return true
}

func profileTokens(values []string, max int) bool {
	if len(values) > max {
		return false
	}
	for _, v := range values {
		if !profileToken.MatchString(v) {
			return false
		}
	}
	return true
}

var dynamicRangeDomain = map[string]bool{"sdr": true, "hdr10": true, "hdr10plus": true, "hlg": true, "dolby_vision": true}

func profileRanges(values []string) bool {
	if len(values) > 8 {
		return false
	}
	for _, v := range values {
		if !dynamicRangeDomain[v] {
			return false
		}
	}
	return true
}

func evidenceValue(v string) bool {
	return v == EvidenceDeclared || v == EvidenceProbed || v == EvidenceMixed
}

// ParseClientProfile validates one published document. It is strict about shape
// and bounds, and deliberately ignorant about content: an unknown codec name is
// kept (the planner simply never matches it), so a newer client is not refused
// by an older server for knowing about a codec the server has not met.
func ParseClientProfile(raw []byte) (ClientProfile, error) {
	var p ClientProfile
	if len(raw) == 0 || len(raw) > MaxClientProfileBytes {
		return p, ErrClientProfile
	}
	var head struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(raw, &head) != nil {
		return p, ErrClientProfile
	}
	if head.Version != ClientProfileVersion {
		return p, ErrClientProfileVersion
	}
	// Unknown fields are ignored on purpose: version 1 grows by addition, and a
	// client that knows a newer field must not be refused by an older server.
	if json.NewDecoder(bytes.NewReader(raw)).Decode(&p) != nil {
		return ClientProfile{}, ErrClientProfile
	}
	if err := p.validate(); err != nil {
		return ClientProfile{}, err
	}
	p.normalize()
	canonical, _ := json.Marshal(p)
	sum := sha256.Sum256(canonical)
	p.Revision = hex.EncodeToString(sum[:16])
	return p, nil
}

// ClientAudioDecode is one audioDecode entry (the v1 capability's shape).
type ClientAudioDecode struct {
	Codec         string   `json:"codec"`
	Containers    []string `json:"containers"`
	MaxSampleRate int      `json:"maxSampleRate,omitempty"`
	SampleRates   []int    `json:"sampleRates,omitempty"`
	MaxChannels   int      `json:"maxChannels,omitempty"`
	MaxBitDepth   int      `json:"maxBitDepth,omitempty"`
	Via           string   `json:"via,omitempty"`
}

// DecodeCaps are the profile's audioDecode entries in the planner's terms.
func (p ClientProfile) DecodeCaps() []AudioDecodeCap {
	out := make([]AudioDecodeCap, 0, len(p.AudioDecode))
	for _, a := range p.AudioDecode {
		out = append(out, AudioDecodeCap{Codec: strings.ToLower(a.Codec), Containers: lowerAll(a.Containers), MaxSampleRate: a.MaxSampleRate, SampleRates: a.SampleRates, MaxChannels: a.MaxChannels, MaxBitDepth: a.MaxBitDepth})
	}
	return out
}

func (p *ClientProfile) validate() error {
	c := p.Client
	if len(p.AudioDecode) > 32 {
		return ErrClientProfile
	}
	for _, a := range p.AudioDecode {
		if !profileToken.MatchString(strings.ToLower(a.Codec)) || len(a.Containers) == 0 || !profileTokens(lowerAll(a.Containers), 16) || len(a.SampleRates) > 32 || a.MaxSampleRate < 0 || a.MaxSampleRate > 1<<22 || a.MaxChannels < 0 || a.MaxChannels > 64 || a.MaxBitDepth < 0 || a.MaxBitDepth > 64 || !profileText(a.Via, 32) {
			return ErrClientProfile
		}
		for _, r := range a.SampleRates {
			if r <= 0 || r > 1<<22 {
				return ErrClientProfile
			}
		}
	}
	if !profileToken.MatchString(c.Family) || !evidenceValue(p.Evidence) {
		return ErrClientProfile
	}
	for _, v := range []string{c.Platform, c.PlatformVersion, c.OS, c.OSVersion, c.Model, c.App, c.AppVersion, c.Engine, c.EngineVersion} {
		if !profileText(v, 64) {
			return ErrClientProfile
		}
	}
	d := p.Display
	if d.Width < 0 || d.Width > 32768 || d.Height < 0 || d.Height > 32768 || d.MaxFrameRate < 0 || d.MaxFrameRate > 1000 || !profileRanges(d.DynamicRanges) {
		return ErrClientProfile
	}
	if len(p.Video) > 32 || len(p.Audio) > 48 || len(p.Transports) > 16 || p.MaxBitrateBPS < 0 || p.MaxBitrateBPS > 2_000_000_000 {
		return ErrClientProfile
	}
	for _, v := range p.Video {
		if !profileToken.MatchString(v.Codec) || !profileTokens(lowerAll(v.Profiles), 32) || v.MaxLevel < 0 || v.MaxLevel > 1000 || len(v.BitDepths) > 8 || v.MaxWidth < 0 || v.MaxWidth > 32768 || v.MaxHeight < 0 || v.MaxHeight > 32768 || v.MaxFrameRate < 0 || v.MaxFrameRate > 1000 || v.MaxBitrateBPS < 0 || v.MaxBitrateBPS > 2_000_000_000 || !profileRanges(v.DynamicRanges) || len(v.DolbyVisionProfiles) > 16 || (v.Evidence != "" && !evidenceValue(v.Evidence)) {
			return ErrClientProfile
		}
		for _, depth := range v.BitDepths {
			if depth < 1 || depth > 16 {
				return ErrClientProfile
			}
		}
		for _, dv := range v.DolbyVisionProfiles {
			if dv < 1 || dv > 31 {
				return ErrClientProfile
			}
		}
	}
	for _, a := range p.Audio {
		if !profileToken.MatchString(a.Codec) || a.MaxChannels < 0 || a.MaxChannels > 64 || a.MaxSampleRate < 0 || a.MaxSampleRate > 1<<22 || !profileTokens(a.ObjectAudio, 4) || (a.Evidence != "" && !evidenceValue(a.Evidence)) {
			return ErrClientProfile
		}
	}
	for _, t := range p.Transports {
		if t.Transport != TransportDirect && t.Transport != TransportHLSTS && t.Transport != TransportHLSFMP4 {
			return ErrClientProfile
		}
		if !profileTokens(t.Containers, 32) || !profileTokens(t.Video, 32) || !profileTokens(t.Audio, 48) {
			return ErrClientProfile
		}
	}
	o := p.AudioOutput
	if (o.Route != "" && !profileToken.MatchString(o.Route)) || o.MaxChannels < 0 || o.MaxChannels > 64 {
		return ErrClientProfile
	}
	if !profileTokens(p.Subtitles.Text, 16) || !profileTokens(p.Subtitles.Styled, 16) || !profileTokens(p.Subtitles.Bitmap, 16) {
		return ErrClientProfile
	}
	return nil
}

func lowerAll(values []string) []string {
	out := make([]string, len(values))
	for i, v := range values {
		out[i] = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(v), " ", "_"))
	}
	return out
}

// normalize makes equal documents byte-equal so the revision is stable: nil
// slices become empty, profile names fold to one spelling, lists are sorted.
func (p *ClientProfile) normalize() {
	fill := func(v *[]string) {
		if *v == nil {
			*v = []string{}
		}
		sort.Strings(*v)
	}
	fill(&p.Display.DynamicRanges)
	fill(&p.Subtitles.Text)
	fill(&p.Subtitles.Styled)
	fill(&p.Subtitles.Bitmap)
	if p.Video == nil {
		p.Video = []ClientVideoCodec{}
	}
	if p.Audio == nil {
		p.Audio = []ClientAudioCodec{}
	}
	if p.Transports == nil {
		p.Transports = []ClientTransport{}
	}
	for i := range p.Video {
		v := &p.Video[i]
		v.Profiles = lowerAll(v.Profiles)
		fill(&v.Profiles)
		fill(&v.DynamicRanges)
		if v.BitDepths == nil {
			v.BitDepths = []int{}
		}
		if v.DolbyVisionProfiles == nil {
			v.DolbyVisionProfiles = []int{}
		}
		sort.Ints(v.BitDepths)
		sort.Ints(v.DolbyVisionProfiles)
	}
	for i := range p.Audio {
		fill(&p.Audio[i].ObjectAudio)
	}
	for i := range p.Transports {
		fill(&p.Transports[i].Containers)
		fill(&p.Transports[i].Video)
		fill(&p.Transports[i].Audio)
	}
}

// BaselineClientProfile is what a device that has published nothing is planned
// against. It is exactly the rule the server applied to every client before
// profiles existed: MP4 with H.264 and AAC or MP3 plays as it is, MPEG-TS HLS
// carries H.264 with stereo AAC, and nothing else is assumed.
func BaselineClientProfile() ClientProfile {
	p := ClientProfile{
		Version:  ClientProfileVersion,
		Client:   ClientIdentity{Family: "unknown"},
		Evidence: EvidenceBuiltin,
		Display:  ClientDisplay{DynamicRanges: []string{"sdr"}},
		Video:    []ClientVideoCodec{{Codec: "h264", MaxFrameRate: 60, BitDepths: []int{8}, DynamicRanges: []string{"sdr"}, Evidence: EvidenceBuiltin}},
		Audio: []ClientAudioCodec{
			{Codec: "aac", MaxChannels: 2, Evidence: EvidenceBuiltin},
			{Codec: "mp3", MaxChannels: 2, Evidence: EvidenceBuiltin},
		},
		AudioOutput: ClientAudioOutput{MaxChannels: 2},
		Transports: []ClientTransport{
			{Transport: TransportDirect, Containers: []string{"m4v", "mov", "mp4"}, Video: []string{"h264"}, Audio: []string{"aac", "mp3"}},
			{Transport: TransportDirect, Containers: []string{"aac", "m4a", "m4b", "mp4"}, Video: []string{}, Audio: []string{"aac"}},
			{Transport: TransportDirect, Containers: []string{"mp3"}, Video: []string{}, Audio: []string{"mp3"}},
			{Transport: TransportHLSTS, Containers: []string{"mpegts"}, Video: []string{"h264"}, Audio: []string{"aac", "mp3"}},
		},
		Subtitles: ClientSubtitles{Text: []string{"srt", "vtt"}, Styled: []string{}, Bitmap: []string{}},
	}
	p.normalize()
	p.Revision = "baseline"
	return p
}

func (p ClientProfile) videoCodec(name string) *ClientVideoCodec {
	for i := range p.Video {
		if p.Video[i].Codec == name {
			return &p.Video[i]
		}
	}
	return nil
}

func (p ClientProfile) audioCodec(name string) *ClientAudioCodec {
	for i := range p.Audio {
		if p.Audio[i].Codec == name {
			return &p.Audio[i]
		}
	}
	return nil
}

// ClientSummary is the part of a profile kept with a decision: enough to say
// which device and which document the plan was made for, without the tables.
type ClientSummary struct {
	Family    string `json:"family"`
	Platform  string `json:"platform,omitempty"`
	Model     string `json:"model,omitempty"`
	Engine    string `json:"engine,omitempty"`
	Evidence  string `json:"evidence"`
	Revision  string `json:"revision"`
	Published bool   `json:"published"`
}

func (p ClientProfile) Summary() ClientSummary {
	return ClientSummary{Family: p.Client.Family, Platform: p.Client.Platform, Model: p.Client.Model, Engine: p.Client.Engine, Evidence: p.Evidence, Revision: p.Revision, Published: p.Evidence != EvidenceBuiltin}
}

// PublishClientProfile stores one device's document against its sign-in. The
// write is skipped when the stored revision already matches, so a client that
// republishes on every launch costs one indexed read.
func (s *Service) PublishClientProfile(ctx context.Context, p identity.Principal, raw []byte) (ClientProfile, error) {
	profile, err := ParseClientProfile(raw)
	if err != nil {
		return ClientProfile{}, err
	}
	if p.Hash == "" {
		return ClientProfile{}, identity.ErrUnauthorized
	}
	var stored string
	err = s.db.QueryRowContext(ctx, `SELECT revision FROM playback_client_profiles WHERE session_hash=?`, p.Hash).Scan(&stored)
	if err == nil && stored == profile.Revision {
		return profile, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ClientProfile{}, err
	}
	canonical, _ := json.Marshal(profile)
	_, err = dbwork.ExecWrite(ctx, s.db, dbwork.ClassPlaybackStart, `INSERT INTO playback_client_profiles(session_hash,account_id,profile_id,revision,family,document,updated_at_ms) VALUES(?,?,?,?,?,?,?) ON CONFLICT(session_hash) DO UPDATE SET revision=excluded.revision,family=excluded.family,document=excluded.document,updated_at_ms=excluded.updated_at_ms`, p.Hash, p.AccountID, p.ProfileID, profile.Revision, profile.Client.Family, string(canonical), time.Now().UnixMilli())
	return profile, err
}

type profileQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// clientProfileFor reads the stored document for this sign-in, or the baseline.
// A stored document that no longer parses (a server downgrade, a hand edit) is
// treated as absent: planning must never fail because of a capability document.
func clientProfileFor(ctx context.Context, q profileQuery, p identity.Principal) ClientProfile {
	if p.Hash == "" {
		return BaselineClientProfile()
	}
	var document string
	// A v1 device's profile belongs to the device (spec §3): it survives token
	// rotation and pruning, and every sign-in of that device plans from it.
	err := q.QueryRowContext(ctx, `SELECT c.planner_profile FROM authorization_family_tokens t JOIN identity_device_families f ON f.family_id=t.family_id JOIN playback_device_capabilities c ON c.device_id=f.device_id WHERE t.token_hash=? AND c.planner_profile<>''`, p.Hash).Scan(&document)
	if err == nil {
		if profile, parsed := ParseClientProfile([]byte(document)); parsed == nil {
			return profile
		}
	}
	err = q.QueryRowContext(ctx, `SELECT document FROM playback_client_profiles WHERE session_hash=?`, p.Hash).Scan(&document)
	if errors.Is(err, sql.ErrNoRows) {
		// A sign-in whose token has rotated is still the same device. Until it
		// publishes again, the newest document from its own token family stands.
		err = q.QueryRowContext(ctx, `SELECT c.document FROM playback_client_profiles c JOIN authorization_family_tokens original ON original.token_hash=c.session_hash JOIN authorization_family_tokens caller ON caller.family_id=original.family_id WHERE caller.token_hash=? ORDER BY c.updated_at_ms DESC LIMIT 1`, p.Hash).Scan(&document)
	}
	if err != nil {
		return BaselineClientProfile()
	}
	profile, err := ParseClientProfile([]byte(document))
	if err != nil {
		return BaselineClientProfile()
	}
	return profile
}

// ClientProfileFor is the read used by the HTTP edge to echo what is stored.
func (s *Service) ClientProfileFor(ctx context.Context, p identity.Principal) ClientProfile {
	return clientProfileFor(ctx, s.db, p)
}

// WithVideoBitrateCeiling returns p with every video codec's MaxBitrateBPS
// narrowed to ceiling (0 = no ceiling; a codec's 0 means unlimited, so it
// becomes ceiling). p's slices are copied; p itself is never modified.
func WithVideoBitrateCeiling(p ClientProfile, ceiling int) ClientProfile {
	if ceiling <= 0 {
		return p
	}
	narrowed := make([]ClientVideoCodec, len(p.Video))
	for i, v := range p.Video {
		v.MaxBitrateBPS = narrowCeiling(v.MaxBitrateBPS, ceiling)
		narrowed[i] = v
	}
	p.Video = narrowed
	return p
}
