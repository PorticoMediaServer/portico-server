package localmetadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/assets"
	"portico.local/server/internal/mediaexec"
	"portico.local/server/internal/storage"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Service struct {
	root, binary string
	storage      *storage.Client
	mu           sync.Mutex
	// sidecars caches sidecar-file reads by full path (positive hits only,
	// bounded): one show's hundred episodes share a single poster read.
	sidecars map[string]string
	// readSmall reads one small file; storage in production, a fake in tests.
	readSmall func(ctx context.Context, library, path string, limit int64) ([]byte, error)
}

func New(root, binary string, store *storage.Client) (*Service, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	s := &Service{root: root, binary: binary, storage: store}
	if store != nil {
		s.readSmall = store.ReadSmall
	}
	return s, nil
}

func (s *Service) Prepare(ctx context.Context, library, path string, f assets.Facts) assets.Facts {
	return s.PrepareKind(ctx, library, "music", path, f)
}
func (s *Service) PrepareKind(ctx context.Context, library, kind, path string, f assets.Facts) assets.Facts {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	f = s.prepareAudioEvidence(ctx, library, kind, path, f)
	for _, name := range []string{"cover.jpg", "folder.jpg", "cover.png", "folder.png"} {
		raw, err := s.storage.ReadSmall(ctx, library, filepath.Join(filepath.Dir(path), name), 4<<20)
		if err != nil {
			continue
		}
		if key, err := s.store(raw); err == nil {
			f.ArtworkKey = key
			return f
		}
		f.LocalMetadataIssue = "invalid_local_cover"
	}
	if f.AttachedPicture && s.binary != "" {
		if s.storage.IsRemote(path) {
			f.LocalMetadataIssue = "remote_embedded_artwork_not_prepared"
			return f
		}
		if err := storage.CheckScanRead(ctx, path); err != nil {
			return f
		}
		// A malformed file that makes ffmpeg hang used to block this goroutine
		// forever: no context, no WaitDelay. Both now, matching the probe path.
		extract, cancelExtract := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelExtract()
		args := []string{"-nostdin", "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,mp3,flac,ogg,wav,aac,asf,aiff,ape,wv", "-i", path, "-map", "0:v:0", "-frames:v", "1", "-vf", "scale=1024:1024:force_original_aspect_ratio=decrease", "-c:v", "mjpeg", "-f", "image2pipe", "pipe:1"}
		var raw []byte
		var buffer bytes.Buffer
		handled, err := storage.RunScanCommand(extract, s.binary, args, path, &buffer)
		if handled {
			raw = buffer.Bytes()
		} else {
			// Outside a scan: the one library file, read-only, through the media
			// executor (mediaexec), at background priority.
			var cmd *exec.Cmd
			if cmd, err = mediaexec.Command(mediaexec.Job{Executable: s.binary, Args: args, ReadPaths: []string{path}, Background: true}); err == nil {
				cmd.WaitDelay = 2 * time.Second
				err = s.storage.Supervisor.Run(extract, library+":art", cmd, func(r io.Reader) error {
					var err error
					raw, err = io.ReadAll(io.LimitReader(r, (4<<20)+1))
					if len(raw) > 4<<20 {
						return errors.New("artwork exceeds limit")
					}
					return err
				})
			}
		}
		if err == nil {
			if key, err := s.store(raw); err == nil {
				f.ArtworkKey = key
			} else {
				f.LocalMetadataIssue = "invalid_embedded_cover"
			}
		}
	}
	return f
}
func (s *Service) store(raw []byte) (string, error) {
	if len(raw) == 0 || len(raw) > 4<<20 {
		return "", errors.New("artwork exceeds limit")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "jpeg" && format != "png") || config.Width <= 0 || config.Height <= 0 || config.Width > 8192 || config.Height > 8192 || int64(config.Width)*int64(config.Height) > 16_000_000 {
		return "", errors.New("invalid local artwork")
	}
	digest := sha256.Sum256(raw)
	key := hex.EncodeToString(digest[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	target := filepath.Join(s.root, key+".img")
	if _, err = os.Stat(target); err == nil {
		return key, nil
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return "", err
	}
	var total int64
	for _, entry := range entries {
		if info, e := entry.Info(); e == nil {
			total += info.Size()
		}
	}
	if total+int64(len(raw)) > 128<<20 {
		return "", errors.New("local artwork cache full")
	}
	file, err := os.CreateTemp(s.root, "art-")
	if err != nil {
		return "", err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(raw); err == nil {
		err = file.Sync()
	}
	file.Close()
	if err == nil {
		err = os.Rename(file.Name(), target)
	}
	return key, err
}

var keyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func (s *Service) Open(key string) (*os.File, string, error) {
	if !keyPattern.MatchString(key) {
		return nil, "", errors.New("invalid artwork key")
	}
	file, err := os.Open(filepath.Join(s.root, key+".img"))
	if err != nil {
		return nil, "", err
	}
	var header [512]byte
	n, err := file.Read(header[:])
	if err != nil && err != io.EOF {
		file.Close()
		return nil, "", err
	}
	_, _ = file.Seek(0, 0)
	return file, http.DetectContentType(header[:n]), nil
}

func (s *Service) MovieArtwork(ctx context.Context, library, path string, f assets.Facts) assets.Facts {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stem := strings.TrimSuffix(path, filepath.Ext(path))
	for _, candidate := range []string{stem + "-poster.jpg", stem + "-poster.png", filepath.Join(filepath.Dir(path), "poster.jpg"), filepath.Join(filepath.Dir(path), "poster.png")} {
		raw, err := s.storage.ReadSmall(ctx, library, candidate, 4<<20)
		if err != nil {
			continue
		}
		if key, err := s.store(raw); err == nil {
			f.ArtworkKey = key
			return f
		}
	}
	return f
}
