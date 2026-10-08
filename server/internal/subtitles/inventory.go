package subtitles

import (
	"context"

	"errors"
	"os"
	"path/filepath"
	"portico.local/server/internal/storage"
	"sort"
	"strings"
)

const MaxTracks = 32
const MaxPreparedTracks = 8
const MaxPreparedBytes = 16 << 20
const ChunkMS int64 = 240000

type Track struct {
	Origin, Locator, Format, Language, Title, Reason string
	StreamIndex                                      int
	Default, Forced                                  bool
	Size, ModifiedNS                                 int64
	Digest                                           string
}
type Inventory struct {
	SourceEvidence string `json:",omitempty"`
	OriginUS       int64
	TimingKnown    bool
	Status         string
	Tracks         []Track
}

// Discover reads exact-basename associations only. The caller owns the media
// facts snapshot; this function never changes a catalog or follows a sidecar
// symlink. A same-basename second media version makes sidecar ownership ambiguous.
func Discover(ctx context.Context, c *storage.Client, key, path string, embedded []Track, originUS int64, known bool) (*Inventory, error) {
	return discover(ctx, c, key, path, embedded, originUS, known, func(visit func(storage.Snapshot) error) error {
		return c.Inventory(ctx, key, filepath.Dir(path), visit)
	})
}
func discover(ctx context.Context, c *storage.Client, key, path string, embedded []Track, originUS int64, known bool, enumerate func(func(storage.Snapshot) error) error) (*Inventory, error) {
	out := &Inventory{OriginUS: originUS, TimingKnown: known, Status: "known", Tracks: append([]Track{}, embedded...)}
	if len(out.Tracks) > MaxTracks {
		out.Status = "capacity"
		out.Tracks = nil
		return out, nil
	}
	if c == nil {
		return out, nil
	} // Production scanning installs the isolated storage client.
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var matches []storage.Snapshot
	ambiguous := false
	count := 0
	err := enumerate(func(v storage.Snapshot) error {
		count++
		if count > 4096 {
			return ErrCapacity
		}
		if v.Directory {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(v.Name))
		name := strings.TrimSuffix(v.Name, filepath.Ext(v.Name))
		if strings.EqualFold(name, stem) && filepath.Clean(v.Path) != filepath.Clean(path) && mediaExtension(ext) {
			ambiguous = true
		}
		if !(strings.EqualFold(name, stem) || strings.HasPrefix(strings.ToLower(name), strings.ToLower(stem)+".")) {
			return nil
		}
		switch ext {
		case ".srt", ".vtt", ".ass", ".ssa", ".sub", ".sup", ".idx":
			matches = append(matches, v)
		}
		if len(matches)+len(embedded) > MaxTracks {
			return ErrCapacity
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		out.Status = "unavailable"
		if errors.Is(err, ErrCapacity) {
			out.Status = "capacity"
		}
		return out, nil
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Name < matches[j].Name })
	for _, v := range matches {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ext := strings.ToLower(filepath.Ext(v.Name))
		if ext == ".sub" {
			continue
		} // .idx owns the paired packet file; no duplicate/false text track.
		t := Track{Origin: "sidecar", Locator: v.Path, Format: strings.TrimPrefix(ext, "."), StreamIndex: -1, Size: v.Size, ModifiedNS: v.ModifiedNS}
		suffix := strings.TrimPrefix(strings.ToLower(strings.TrimSuffix(v.Name, filepath.Ext(v.Name))), strings.ToLower(stem))
		parts := strings.Split(strings.TrimPrefix(suffix, "."), ".")
		for _, p := range parts {
			switch strings.ToLower(p) {
			case "forced":
				t.Forced = true
			case "sdh", "cc":
				t.Title = strings.ToUpper(p)
			case "":
			default:
				if t.Language == "" {
					t.Language = p
				} else {
					t.Reason = "ambiguous_association"
				}
			}
		}
		if ambiguous {
			t.Reason = "ambiguous_association"
		}
		if !validFormat(t.Format) {
			t.Reason = "unsupported_format"
		}
		if t.Reason == "" {
			before, e := c.Stat(ctx, key, v.Path)
			if e != nil {
				t.Reason = "source_unavailable"
			} else {
				limit := int64(MaxInputBytes)
				if ext == ".sup" {
					limit = MaxBinaryBytes
				}
				raw, e := c.ReadSmall(ctx, key, v.Path, limit)
				var companion []byte
				var pairedBefore storage.Snapshot
				if ext == ".idx" && e == nil {
					pairedPath := strings.TrimSuffix(v.Path, filepath.Ext(v.Path)) + ".sub"
					pairedBefore, e = c.Stat(ctx, key, pairedPath)
					if e == nil {
						companion, e = c.ReadSmall(ctx, key, pairedPath, MaxBinaryBytes)
					}
					if e == nil {
						after, x := c.Stat(ctx, key, pairedPath)
						if x != nil || after.Size != pairedBefore.Size || after.ModifiedNS != pairedBefore.ModifiedNS {
							e = ErrConflict
						}
					}
				}
				if e != nil {
					t.Reason = "source_unavailable"
				} else if _, e = CanonicalAsset(raw, companion, t.Format, 0); e != nil {
					t.Reason = Reason(e)
				} else {
					after, e := c.Stat(ctx, key, v.Path)
					if e != nil || before.Size != after.Size || before.ModifiedNS != after.ModifiedNS {
						t.Reason = "source_changed"
					} else {
						t.Digest = SidecarFingerprint(raw, companion)
						t.Size = after.Size
						t.ModifiedNS = after.ModifiedNS
					}
				}
			}
		}
		out.Tracks = append(out.Tracks, t)
	}
	return out, nil
}
func mediaExtension(s string) bool {
	switch s {
	case ".strm", ".mp4", ".m4v", ".mkv", ".avi", ".mov", ".webm", ".ts", ".m2ts":
		return true
	}
	return false
}
func Reason(e error) string {
	switch {
	case errors.Is(e, ErrCapacity):
		return "capacity"
	case errors.Is(e, ErrTiming):
		return "unsupported_timing"
	case errors.Is(e, os.ErrNotExist):
		return "source_unavailable"
	default:
		return "unsupported_text_features"
	}
}

// Codec mapping is shared by ingestion and STRM refresh. Unknown codecs remain
// inspectable but cannot be selected as another codec by a client.
func CodecFormat(codec string) string {
	switch codec {
	case "subrip", "text", "mov_text":
		return "srt"
	case "webvtt":
		return "vtt"
	case "ass":
		return "ass"
	case "ssa":
		return "ssa"
	case "hdmv_pgs_subtitle":
		return "pgs"
	case "dvd_subtitle":
		return "vobsub"
	case "dvb_subtitle":
		return "dvb"
	}
	return ""
}
