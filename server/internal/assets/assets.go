package assets

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"portico.local/server/internal/mediaexec"
	"portico.local/server/internal/mediatools"
	"portico.local/server/internal/storage"
	"portico.local/server/internal/subtitles"
	"strconv"
	"strings"
	"time"
)

type Chapter struct {
	Title string  `json:"title"`
	Start float64 `json:"startSeconds"`
	End   float64 `json:"endSeconds"`
}
type Facts struct {
	SubtitleTextPending bool                 `json:"-"`
	VideoMetadata       []VideoNFO           `json:"-"`
	VideoMetadataStatus string               `json:"-"`
	AnalysisOperations  []string             `json:"-"`
	ObservedRevision    string               `json:"-"`
	InventoryOnly       bool                 `json:"-"`
	SubtitleInventory   *subtitles.Inventory `json:"-"`
	OriginUS            int64                `json:"-"`
	TimingKnown         bool                 `json:"-"`
	ChapterStatus       string               `json:"-"`
	Streams             []Stream             `json:"-"`
	TagSources          map[string]string    `json:"-"`
	Tags                map[string]string    `json:"-"`
	Chapters            []Chapter            `json:"-"`
	AttachedPicture     bool                 `json:"-"`
	ArtworkKey          string               `json:"-"`
	// ArtworkExtra carries sidecar cache keys (localmetadata.ReadSidecars:
	// "show/poster", "season/2/poster", "artist/portrait", "item/logo",
	// "album/cover") for the catalog commit to write as local_artwork rows.
	ArtworkExtra           map[string]string  `json:"-"`
	LocalMetadataIssue     string             `json:"-"`
	ObservedSize           int64              `json:"-"`
	ObservedModifiedNS     int64              `json:"-"`
	Container              string             `json:"container"`
	VideoCodec             string             `json:"videoCodec"`
	AudioCodec             string             `json:"audioCodec"`
	Width                  int                `json:"width"`
	Height                 int                `json:"height"`
	Duration               float64            `json:"duration"`
	BitRate                int64              `json:"-"`
	AudioEvidence          []AudioTagEvidence `json:"-"`
	AnalysisSourceEvidence string             `json:"-"`
}
type Source struct {
	ID string `json:"id"`
	Facts
}
type Probe struct {
	ReadGuard  func(context.Context, string) error
	Binary     string
	Supervisor *storage.Supervisor
}
type boundedBuffer struct {
	data  []byte
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(b.data)+len(p) > b.limit {
		return 0, errors.New("probe output exceeds limit")
	}
	b.data = append(b.data, p...)
	return len(p), nil
}
func (p Probe) Inspect(ctx context.Context, path string) (Facts, error) {
	return p.inspect(ctx, path, nil)
}

// InspectScan applies bounded local-media probing and rejects network protocols.
// File List Only never calls this method; ReadGuard is checked again at launch.
func (p Probe) InspectScan(ctx context.Context, path string) (Facts, error) {
	return p.inspect(ctx, path, []string{"-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,webm,avi,mpegts,mpeg,mpegvideo,mp3,flac,wav,ogg,aac,aiff,asf,ape,wv,tta,amr", "-probesize", "8388608", "-analyzeduration", "5000000"})
}
func (p Probe) InspectRemoteMP4(ctx context.Context, path string) (Facts, error) {
	facts, e := p.inspect(ctx, path, []string{"-f", "mov", "-enable_drefs", "0", "-use_absolute_path", "0", "-protocol_whitelist", "http,tcp", "-probesize", "8388608", "-analyzeduration", "5000000"})
	facts.Container = "mp4"
	return facts, e
}
func (p Probe) inspect(ctx context.Context, path string, options []string) (Facts, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	bin := p.Binary
	if bin == "" {
		bin = mediatools.Resolve("ffprobe")
	}
	args := append(options, []string{"-v", "error", "-show_format", "-show_streams", "-show_chapters", "-of", "json", path}...)
	out := &boundedBuffer{limit: 1 << 20}
	var runErr error
	if p.ReadGuard != nil {
		if e := p.ReadGuard(ctx, path); e != nil {
			return Facts{}, e
		}
	}
	if e := ctx.Err(); e != nil {
		return Facts{}, e
	}
	if handled, err := storage.RunScanCommand(ctx, bin, args, path, out); handled {
		runErr = err
	} else if p.Supervisor != nil {
		cmd, err := mediaexec.Command(probeJob(bin, args, path))
		if err != nil {
			return Facts{}, fmt.Errorf("media probe failed: %w", err)
		}
		key := path
		if len(options) > 0 && options[0] == "-f" {
			key = "playback:probe:" + path
		}
		runErr = p.Supervisor.Run(ctx, key, cmd, func(r io.Reader) error { _, e := io.Copy(out, r); return e })
	} else {
		cmd, err := mediaexec.CommandContext(ctx, probeJob(bin, args, path))
		if err != nil {
			return Facts{}, fmt.Errorf("media probe failed: %w", err)
		}
		cmd.Stdout = out
		cmd.Stderr = io.Discard
		cmd.WaitDelay = time.Second
		runErr = cmd.Run()
	}
	if runErr != nil {
		return Facts{}, fmt.Errorf("media probe failed: %w", runErr)
	}
	var raw struct {
		Format struct {
			Duration string            `json:"duration"`
			Start    string            `json:"start_time"`
			Name     string            `json:"format_name"`
			Tags     map[string]string `json:"tags"`
		} `json:"format"`
		Streams []struct {
			Index          *int              `json:"index"`
			Channels       int               `json:"channels"`
			ChannelLayout  string            `json:"channel_layout"`
			Type           string            `json:"codec_type"`
			Codec          string            `json:"codec_name"`
			Width          int               `json:"width"`
			Height         int               `json:"height"`
			ColorTransfer  string            `json:"color_transfer"`
			ColorPrimaries string            `json:"color_primaries"`
			Tags           map[string]string `json:"tags"`
			Disposition    struct {
				AttachedPic     int `json:"attached_pic"`
				Default         int `json:"default"`
				Forced          int `json:"forced"`
				HearingImpaired int `json:"hearing_impaired"`
				Comment         int `json:"comment"`
			} `json:"disposition"`
		} `json:"streams"`
		Chapters []struct {
			Start string            `json:"start_time"`
			End   string            `json:"end_time"`
			Tags  map[string]string `json:"tags"`
		} `json:"chapters"`
	}
	if e := json.Unmarshal(out.data, &raw); e != nil {
		return Facts{}, e
	}
	// Detail is read in a second pass over the same bytes so the two shapes stay
	// independent. A detail that fails to decode costs the detail, never the probe.
	var detail struct {
		Format struct {
			BitRate string `json:"bit_rate"`
		} `json:"format"`
		Streams []probeStreamDetail `json:"streams"`
	}
	if json.Unmarshal(out.data, &detail) != nil || len(detail.Streams) != len(raw.Streams) {
		detail.Streams = make([]probeStreamDetail, len(raw.Streams))
	}
	f := Facts{Container: strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")}
	f.Duration, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	if rate, e := strconv.ParseInt(detail.Format.BitRate, 10, 64); e == nil && rate > 0 && rate < 1<<40 {
		f.BitRate = rate
	}
	if start, e := strconv.ParseFloat(raw.Format.Start, 64); e == nil && !math.IsNaN(start) && !math.IsInf(start, 0) && math.Abs(start) <= 86400 {
		f.OriginUS = int64(math.Round(start * 1e6))
		f.TimingKnown = true
	}
	f.Tags = map[string]string{}
	f.TagSources = map[string]string{}
	copyTags := func(tags map[string]string) { CopyEmbeddedAudioTags(&f, tags) }
	copyTags(raw.Format.Tags)
	if len(raw.Streams) > 128 {
		return Facts{}, errors.New("source stream count exceeds 128")
	}
	f.Streams = []Stream{}
	knownStreams := true
	seenStreams := map[int]bool{}
	for position, s := range raw.Streams {
		if s.Disposition.AttachedPic != 0 {
			f.AttachedPicture = true
			continue
		}
		if s.Type == "video" || s.Type == "audio" || s.Type == "subtitle" {
			if s.Index == nil {
				knownStreams = false
			} else {
				if *s.Index < 0 || *s.Index > 65535 || s.Channels < 0 || s.Channels > 128 || seenStreams[*s.Index] {
					return Facts{}, errors.New("invalid probe stream index")
				}
				seenStreams[*s.Index] = true
				f.Streams = append(f.Streams, Stream{Index: *s.Index, Type: s.Type, Codec: streamText(s.Codec, 64), Language: streamText(s.Tags["language"], 32), Title: streamText(s.Tags["title"], 200), Channels: s.Channels, ChannelLayout: streamText(s.ChannelLayout, 64), Default: s.Disposition.Default != 0, Forced: s.Disposition.Forced != 0, ColorTransfer: streamText(s.ColorTransfer, 32), ColorPrimaries: streamText(s.ColorPrimaries, 32), HearingImpaired: s.Disposition.HearingImpaired != 0, Commentary: s.Disposition.Comment != 0, Detail: streamDetail(s.Type, s.Codec, s.Width, s.Height, detail.Streams[position])})
			}
		}
		if s.Type == "video" && f.VideoCodec == "" {
			f.VideoCodec = s.Codec
			f.Width = s.Width
			f.Height = s.Height
		}
		if s.Type == "audio" && f.AudioCodec == "" {
			f.AudioCodec = s.Codec
			copyTags(s.Tags)
		}
	}
	if !knownStreams {
		f.Streams = nil
	}
	f.ChapterStatus = "known"
	for _, c := range raw.Chapters {
		start, e1 := strconv.ParseFloat(c.Start, 64)
		end, e2 := strconv.ParseFloat(c.End, 64)
		if e1 != nil || e2 != nil || math.IsNaN(start) || math.IsNaN(end) || math.IsInf(start, 0) || math.IsInf(end, 0) || start < 0 || end <= start || end > f.Duration+0.1 || len(f.Chapters) >= 4096 || (len(f.Chapters) > 0 && start < f.Chapters[len(f.Chapters)-1].End) {
			f.ChapterStatus = "invalid"
			f.LocalMetadataIssue = "invalid_chapter_timeline"
			f.Chapters = nil
			break
		}
		if end > f.Duration {
			end = f.Duration
		}
		if end <= start {
			f.ChapterStatus = "invalid"
			f.Chapters = nil
			break
		}
		title := streamText(c.Tags["title"], 500)
		f.Chapters = append(f.Chapters, Chapter{title, start, end})
	}
	if (f.VideoCodec == "" && f.AudioCodec == "") || (f.Duration <= 0 || math.IsNaN(f.Duration) || math.IsInf(f.Duration, 0)) {
		return Facts{}, errors.New("file has no timed media stream")
	}
	return f, nil
}
func SupportedPath(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp4", ".m4v", ".mov", ".mkv", ".webm", ".avi", ".ts", ".m2ts", ".mts", ".vob", ".mpg", ".mpeg", ".strm", ".mp3", ".m4a", ".m4b", ".flac", ".ogg", ".opus", ".wav", ".aac", ".aiff":
		return true
	}
	return false
}
func Open(db *sql.DB, id string) (*os.File, error) {
	var path string
	var size, modified int64
	e := db.QueryRow(`SELECT path,size,modified_ns FROM catalog_assets WHERE token=? AND available=1`, id).Scan(&path, &size, &modified)
	if e != nil {
		return nil, e
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() != size || info.ModTime().UnixNano() != modified {
		f.Close()
		return nil, errors.New("source changed; rescan required")
	}
	return f, nil
}

func SupportedForKind(path, kind string) bool {
	extension := strings.ToLower(filepath.Ext(path))
	audio := false
	switch extension {
	case ".mp3", ".m4a", ".m4b", ".flac", ".ogg", ".opus", ".wav", ".aac", ".aiff":
		audio = true
	}
	if kind == "music" || kind == "audiobook" {
		return audio
	}
	return SupportedPath(path) && !audio
}

// OpenPlayback validates the opened descriptor against the creation-time pin,
// so a concurrent rescan cannot substitute a new file under an existing grant.
func OpenPlayback(db *sql.DB, session, grantHash string) (*os.File, error) {
	var path string
	var size, modified int64
	e := db.QueryRow(`SELECT a.path,COALESCE(pin.size,a.size),COALESCE(pin.modified_ns,a.modified_ns) FROM playback_sessions ps JOIN catalog_assets a ON a.token=ps.asset_id LEFT JOIN playback_source_pins pin ON pin.session_id=ps.id WHERE (ps.id=? OR ps.grant_hash=?) AND a.available=1 AND (pin.session_id IS NULL OR (pin.asset_id=a.token AND pin.size=a.size AND pin.modified_ns=a.modified_ns))`, session, grantHash).Scan(&path, &size, &modified)
	if e != nil {
		return nil, e
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() != size || info.ModTime().UnixNano() != modified {
		f.Close()
		return nil, errors.New("source changed; rescan required")
	}
	return f, nil
}
