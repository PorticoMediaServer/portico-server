package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"portico.local/server/internal/mediaexec"
	"regexp"
	"strings"
	"time"
)

// Lyric discovery is a foreground, read-only operation under the existing
// bounded storage helper. Neither the viewer nor a provider supplies a path.
type LocalLyric struct {
	Format   string `json:"format"`
	Language string `json:"language"`
	Origin   string `json:"origin"`
	Label    string `json:"label"`
	Data     []byte `json:"data"`
}
type LocalLyrics struct {
	Candidates []LocalLyric `json:"candidates"`
	Warnings   []string     `json:"warnings"`
}

func (c *Client) ReadLyrics(ctx context.Context, root, path string, size, modified int64, language, probe string) (LocalLyrics, error) {
	out := LocalLyrics{}
	if c.Guard != nil {
		if e := c.Guard(path); e != nil {
			return out, e
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req := request{Operation: "lyrics-local", Path: path, Size: size, ModifiedNS: modified, LyricsRoot: root, LyricsLanguage: language}
	probeUnavailable := false
	if probe != "" {
		// Built here, where the media executor is configured; the helper runs it
		// on the descriptor only it holds.
		argv, e := mediaexec.Argv(mediaexec.Job{Executable: probe, Args: []string{"-v", "error", "-protocol_whitelist", "file,pipe", "-show_entries", "format_tags:stream_tags", "-of", "json", descriptorInput}})
		if e != nil {
			probeUnavailable = true
		}
		req.LyricsProbeArgv = argv
	}
	if c.MountedRoot != nil {
		req.MountedRoot = c.MountedRoot(path)
	}
	raw, _ := json.Marshal(req)
	cmd := exec.Command(c.Binary, "--portico-storage-helper")
	cmd.Stdin = bytes.NewReader(raw)
	e := c.Supervisor.Run(ctx, "lyrics:"+root, cmd, func(r io.Reader) error {
		data, e := io.ReadAll(io.LimitReader(r, (4<<20)+1))
		if e != nil {
			return e
		}
		if len(data) > 4<<20 {
			return errors.New("lyric discovery exceeded limit")
		}
		return json.Unmarshal(data, &out)
	})
	if e == nil && probeUnavailable {
		out.Warnings = append(out.Warnings, "Embedded lyrics could not be read; check ffprobe and the source format.")
	}
	return out, e
}

var lyricLanguage = regexp.MustCompile(`^[A-Za-z]{2,8}(-[A-Za-z0-9]{1,8})*$`)

func lyricLocalHelper(r request, out io.Writer) error {
	result := LocalLyrics{Candidates: []LocalLyric{}, Warnings: []string{}}
	if !filepath.IsAbs(r.LyricsRoot) || !filepath.IsAbs(r.Path) || len(r.LyricsLanguage) > 63 || !lyricLanguage.MatchString(r.LyricsLanguage) {
		return errors.New("invalid lyric source")
	}
	rel, e := filepath.Rel(r.LyricsRoot, r.Path)
	if e != nil || !filepath.IsLocal(rel) {
		return errors.New("lyric source escaped library root")
	}
	root, e := os.OpenRoot(r.LyricsRoot)
	if e != nil {
		return e
	}
	defer root.Close()
	source, e := root.Open(rel)
	if e != nil {
		return e
	}
	defer source.Close()
	before, e := source.Stat()
	if e != nil {
		return e
	}
	if !before.Mode().IsRegular() || before.Size() != r.Size || before.ModTime().UnixNano() != r.ModifiedNS {
		return ErrPlaybackSource
	}
	stem := strings.TrimSuffix(rel, filepath.Ext(rel))
	type name struct{ path, format, language string }
	names := []name{{stem + ".lrc", "lrc", "und"}, {rel + ".lrc", "lrc", "und"}, {stem + ".lyrics", "text", "und"}, {stem + ".txt", "text", "und"}}
	if r.LyricsLanguage != "und" {
		names = append(names, name{stem + "." + r.LyricsLanguage + ".lrc", "lrc", r.LyricsLanguage}, name{stem + "." + r.LyricsLanguage + ".txt", "text", r.LyricsLanguage})
	}
	for _, n := range names {
		f, e := root.Open(n.path)
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			result.Warnings = append(result.Warnings, "A sidecar could not be read safely.")
			continue
		}
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
			f.Close()
			result.Warnings = append(result.Warnings, "A sidecar was not a bounded regular text file.")
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
		after, statErr := f.Stat()
		f.Close()
		current, pathErr := root.Stat(n.path)
		if err != nil || statErr != nil || pathErr != nil || !os.SameFile(info, current) || len(data) > 256<<10 || info.Size() != after.Size() || info.ModTime() != after.ModTime() {
			result.Warnings = append(result.Warnings, "A sidecar changed or failed during reading.")
			continue
		}
		result.Candidates = append(result.Candidates, LocalLyric{Format: n.format, Language: n.language, Origin: "sidecar", Label: filepath.Base(n.path), Data: data})
	}
	if len(r.LyricsProbeArgv) > 0 {
		tags, e := probeLyricTags(source, r.LyricsProbeArgv)
		if e != nil {
			result.Warnings = append(result.Warnings, "Embedded lyrics could not be read; check ffprobe and the source format.")
		} else {
			seen := map[string]bool{}
			for _, tags := range tags {
				for key, value := range tags {
					k := strings.ToLower(key)
					if k != "lyrics" && k != "unsyncedlyrics" && k != "unsynced lyrics" && k != "syncedlyrics" && !strings.HasPrefix(k, "lyrics-") {
						continue
					}
					if len(value) > 256<<10 {
						result.Warnings = append(result.Warnings, "An embedded lyric exceeded the text limit.")
						continue
					}
					if seen[value] || strings.TrimSpace(value) == "" {
						continue
					}
					seen[value] = true
					language := "und"
					if strings.HasPrefix(k, "lyrics-") && lyricLanguage.MatchString(strings.TrimPrefix(k, "lyrics-")) {
						language = strings.TrimPrefix(k, "lyrics-")
					}
					format := "text"
					if regexp.MustCompile(`(?m)^\s*\[[0-9]+:[0-9]{2}`).MatchString(value) {
						format = "lrc"
					}
					result.Candidates = append(result.Candidates, LocalLyric{Format: format, Language: language, Origin: "embedded", Label: "Embedded lyrics", Data: []byte(value)})
					if len(result.Candidates) >= 12 {
						break
					}
				}
				if len(result.Candidates) >= 12 {
					break
				}
			}
		}
	}
	after, e := source.Stat()
	if e != nil || after.Size() != before.Size() || after.ModTime() != before.ModTime() {
		return ErrPlaybackSource
	}
	// Check the path still denotes the descriptor that was probed.
	current, e := root.Stat(rel)
	if e != nil || !os.SameFile(before, current) {
		return ErrPlaybackSource
	}
	return json.NewEncoder(out).Encode(result)
}
