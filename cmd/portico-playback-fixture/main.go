// portico-playback-fixture starts an intentionally isolated, loopback-only
// Portico server for downstream client conformance tests. It is not a mode of
// porticod and cannot be enabled in a production server.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/app"
	"github.com/PorticoMediaServer/portico-server/internal/config"
	"github.com/PorticoMediaServer/portico-server/internal/database"
	"github.com/PorticoMediaServer/portico-server/internal/mediafacts"
)

const fixtureSchema = "portico.playback-fixture/v1"

const fixtureLifetime = 2 * time.Hour

type manifest struct {
	Schema     string            `json:"schema"`
	BaseURL    string            `json:"baseUrl"`
	ExpiresAt  string            `json:"expiresAt"`
	Owner      credential        `json:"owner"`
	User       credential        `json:"user"`
	ProfileIDs map[string]string `json:"profileIds"`
	Media      map[string]string `json:"media"`
	Lifecycle  []string          `json:"lifecycleScenarios"`
	Control    controlManifest   `json:"control"`
}

type controlManifest struct {
	Path   string `json:"path"`
	Secret string `json:"secret"`
}

type faultSpec struct {
	Path    string `json:"path"`
	Status  int    `json:"status,omitempty"`
	DelayMS int    `json:"delayMs,omitempty"`
}

type faultGate struct {
	mu     sync.Mutex
	secret string
	next   *faultSpec
}

const fixtureControlPath = "/__portico_playback_fixture/control"

func (g *faultGate) handler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == fixtureControlPath {
			g.control(w, r)
			return
		}
		g.mu.Lock()
		var fault *faultSpec
		if g.next != nil && r.URL.Path == g.next.Path {
			copy := *g.next
			fault, g.next = &copy, nil
		}
		g.mu.Unlock()
		if fault != nil {
			if fault.DelayMS > 0 {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(time.Duration(fault.DelayMS) * time.Millisecond):
				}
			}
			if fault.Status != 0 {
				// A deterministic one-shot must reach the fixture exactly once.
				// Browsers otherwise cache a bare 404/410 and can replay it after
				// the gate has cleared, falsely turning recovery into a second fault.
				w.Header().Set("Cache-Control", "no-store")
				http.Error(w, http.StatusText(fault.Status), fault.Status)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (g *faultGate) control(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+g.secret || !isLoopback(r.RemoteAddr) {
		http.NotFound(w, r)
		return
	}
	var spec faultSpec
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&spec); err != nil || !validFault(spec) {
		http.Error(w, "invalid fault", http.StatusBadRequest)
		return
	}
	g.mu.Lock()
	g.next = &spec
	g.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"armed":true}`)
}

func validFault(spec faultSpec) bool {
	return strings.HasPrefix(spec.Path, "/api/") && !strings.Contains(spec.Path, "?") &&
		(spec.Status == 0 || spec.Status == http.StatusNotFound || spec.Status == http.StatusGone) &&
		spec.DelayMS >= 0 && spec.DelayMS <= 10_000 && (spec.Status != 0 || spec.DelayMS > 0)
}

func isLoopback(remote string) bool {
	host, _, err := net.SplitHostPort(remote)
	return err == nil && net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

type credential struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func main() {
	var manifestPath, appData, ffmpeg, ffprobe string
	flag.StringVar(&manifestPath, "manifest", "", "required path for the private connection manifest")
	flag.StringVar(&appData, "app-data", "", "optional private working directory (retained when supplied)")
	flag.StringVar(&ffmpeg, "ffmpeg", "ffmpeg", "ffmpeg executable")
	flag.StringVar(&ffprobe, "ffprobe", "ffprobe", "ffprobe executable")
	flag.Parse()
	if manifestPath == "" {
		fatal(errors.New("--manifest is required"))
	}
	ownedTemp := appData == ""
	if ownedTemp {
		var err error
		appData, err = os.MkdirTemp("", "portico-playback-fixture-")
		if err != nil {
			fatal(err)
		}
		defer os.RemoveAll(appData)
	}
	if err := os.MkdirAll(appData, 0o700); err != nil {
		fatal(err)
	}
	if err := os.Chmod(appData, 0o700); err != nil {
		fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, appData, manifestPath, ffmpeg, ffprobe); err != nil {
		fatal(err)
	}
}

func run(ctx context.Context, appData, manifestPath, ffmpeg, ffprobe string) error {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	web := filepath.Join(appData, "web")
	for _, dir := range []string{web, filepath.Join(appData, "media"), filepath.Join(appData, "transcodes")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(web, "index.html"), []byte("<!doctype html><title>Portico playback fixture</title>"), 0o600); err != nil {
		return err
	}
	cfg := config.Config{Addr: listener.Addr().String(), AppDataDir: appData, DatabasePath: filepath.Join(appData, "portico.db"), BackupDir: filepath.Join(appData, "backups"), WebDistDir: web, TranscodeDir: filepath.Join(appData, "transcodes"), FFmpegPath: ffmpeg, FFprobePath: ffprobe}
	db, err := database.Open(cfg)
	if err != nil {
		return fmt.Errorf("open fixture database: %w", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO settings(key,value_json,updated_at) VALUES('transcoder','{"enabled":true,"maxConcurrentSessions":2,"maxSoftwareSessions":2}',?) ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json,updated_at=excluded.updated_at`, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	server := app.NewServer(cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	controlSecret, err := randomPassword()
	if err != nil {
		return err
	}
	gate := &faultGate{secret: controlSecret}
	httpServer := &http.Server{Handler: gate.handler(server.Handler()), ReadHeaderTimeout: 5 * time.Second}
	serveDone := make(chan error, 1)
	go func() { serveDone <- httpServer.Serve(listener) }()
	baseURL := "http://" + listener.Addr().String()
	password, err := randomPassword()
	if err != nil {
		return err
	}
	viewerPassword, err := randomPassword()
	if err != nil {
		return err
	}
	client := newClient()
	if err := postJSON(client, baseURL+"/api/auth/setup", map[string]any{"serverName": "Playback Fixture Server", "username": "fixture-owner", "email": "fixture-owner@example.invalid", "displayName": "Fixture Owner", "password": password, "setupMode": "local_only", "localOnlyAcknowledged": true}, nil); err != nil {
		return err
	}
	media, err := createMedia(ctx, filepath.Join(appData, "media"), ffmpeg)
	if err != nil {
		return err
	}
	if err := seedMedia(db, media); err != nil {
		return err
	}
	if err := seedMissingMedia(db, filepath.Join(appData, "media", "fixture-missing.mp4")); err != nil {
		return err
	}
	var user struct {
		ID string `json:"id"`
	}
	if err := postJSON(client, baseURL+"/api/users", map[string]any{"username": "fixture-viewer", "email": "fixture-viewer@example.invalid", "displayName": "Fixture Viewer", "password": viewerPassword, "permissions": map[string]bool{"playMedia": true, "transcode": true}, "libraryIds": []string{"fixture-library"}}, &user); err != nil {
		return err
	}
	profiles := map[string]string{}
	for key, login := range map[string]string{"owner": "fixture-owner", "user": "fixture-viewer"} {
		var id string
		if err := db.QueryRow(`SELECT p.id FROM profiles p JOIN users u ON u.id=p.account_id WHERE u.username=? ORDER BY p.is_primary DESC, p.created_at LIMIT 1`, login).Scan(&id); err != nil {
			return err
		}
		profiles[key] = id
	}
	expiresAt := time.Now().UTC().Add(fixtureLifetime)
	m := manifest{Schema: fixtureSchema, BaseURL: baseURL, ExpiresAt: expiresAt.Format(time.RFC3339), Owner: credential{"fixture-owner", password}, User: credential{"fixture-viewer", viewerPassword}, ProfileIDs: profiles, Media: map[string]string{"direct": "fixture-direct", "remux": "fixture-remux", "transcode": "fixture-transcode", "multiTrack": "fixture-multitrack", "music": "fixture-music", "missing": "fixture-missing"}, Lifecycle: []string{"create-session", "renew-media-grant", "reject-expired-or-revoked-grant", "renegotiate-generation", "one-shot-resource-fault"}, Control: controlManifest{Path: fixtureControlPath, Secret: controlSecret}}
	if err := writeManifest(manifestPath, m); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "\n*** PORTICO DEVELOPER PLAYBACK FIXTURE — LOOPBACK ONLY — NOT PRODUCTION ***\nmanifest: %s\nbase URL: %s\nexpires: %s\n\n", manifestPath, baseURL, m.ExpiresAt)
	lifetime := time.NewTimer(time.Until(expiresAt))
	defer lifetime.Stop()
	select {
	case <-ctx.Done():
	case <-lifetime.C:
	case err := <-serveDone:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdown)
	return server.Shutdown(shutdown)
}

func seedMissingMedia(db *sql.DB, path string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := db.Exec(`INSERT INTO media_items(id,library_id,type,title,sort_title,source_url,duration_seconds,added_at) VALUES('fixture-missing','fixture-library','movie','Missing source','Missing source',?,2,?)`, path, now)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO media_files(id,media_id,library_id,path,container,source_type,size_bytes,mod_time,available,first_seen_at,last_seen_at) VALUES('fixture-missing-file','fixture-missing','fixture-library',?,'mp4','local',1,?,1,?,?)`, path, now, now, now)
	return err
}

func newClient() *http.Client {
	jar, _ := cookiejar.New(nil)
	return &http.Client{Jar: jar, Timeout: 10 * time.Second}
}

func postJSON(client *http.Client, target string, body, out any) error {
	raw, _ := json.Marshal(body)
	req, err := http.NewRequest(http.MethodPost, target, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Portico-CSRF", "1")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s returned %d: %s", target, resp.StatusCode, payload)
	}
	if out != nil {
		return json.Unmarshal(payload, out)
	}
	return nil
}

func randomPassword() (string, error) {
	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "Fx!" + base64.RawURLEncoding.EncodeToString(raw), nil
}

func createMedia(ctx context.Context, dir, ffmpeg string) (map[string]string, error) {
	specs := []struct{ id, ext, vcodec, acodec string }{{"fixture-direct", "mp4", "libx264", "aac"}, {"fixture-remux", "mkv", "libx264", "aac"}, {"fixture-transcode", "avi", "mpeg4", "libmp3lame"}}
	result := map[string]string{}
	for _, s := range specs {
		path := filepath.Join(dir, s.id+"."+s.ext)
		cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "color=c=navy:s=320x180:r=24:d=2", "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=2", "-c:v", s.vcodec, "-pix_fmt", "yuv420p", "-c:a", s.acodec, "-ac", "2", "-y", path)
		if output, err := cmd.CombinedOutput(); err != nil {
			return nil, fmt.Errorf("generate %s: %w: %s", s.id, err, output)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, err
		}
		result[s.id] = path
	}
	subtitlePath := filepath.Join(dir, "fixture-multitrack.srt")
	if err := os.WriteFile(subtitlePath, []byte("1\n00:00:00,000 --> 00:00:01,500\nPortico playback fixture\n"), 0o600); err != nil {
		return nil, err
	}
	multiPath := filepath.Join(dir, "fixture-multitrack.mkv")
	multi := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=purple:s=320x180:r=24:d=2",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=2",
		"-f", "lavfi", "-i", "sine=frequency=880:sample_rate=48000:duration=2",
		"-i", subtitlePath, "-map", "0:v:0", "-map", "1:a:0", "-map", "2:a:0", "-map", "3:s:0",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-ac", "2", "-c:s", "webvtt",
		"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=fra", "-metadata:s:s:0", "language=eng", "-y", multiPath)
	if output, err := multi.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("generate fixture-multitrack: %w: %s", err, output)
	}
	musicPath := filepath.Join(dir, "fixture-music.mp3")
	music := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "sine=frequency=523:sample_rate=48000:duration=2", "-c:a", "libmp3lame", "-ac", "2", "-y", musicPath)
	if output, err := music.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("generate fixture-music: %w: %s", err, output)
	}
	for id, path := range map[string]string{"fixture-multitrack": multiPath, "fixture-music": musicPath} {
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, err
		}
		result[id] = path
	}
	return result, nil
}

func seedMedia(db *sql.DB, media map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	mediaRoot := filepath.Dir(media["fixture-direct"])
	if _, err = tx.Exec(`INSERT INTO libraries(id,name,type,path,created_at) VALUES('fixture-library','Playback Conformance','movie',?,?)`, mediaRoot, now); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO library_paths(id,library_id,path,sort_order,created_at) VALUES('fixture-library-path','fixture-library',?,0,?)`, mediaRoot, now); err != nil {
		return err
	}
	types := map[string]struct{ container, video, audio string }{"fixture-direct": {"mp4", "h264", "aac"}, "fixture-remux": {"mkv", "h264", "aac"}, "fixture-transcode": {"avi", "mpeg4", "mp3"}, "fixture-multitrack": {"mkv", "h264", "aac"}, "fixture-music": {"mp3", "", "mp3"}}
	for id, path := range media {
		t := types[id]
		stat, err := os.Stat(path)
		if err != nil {
			return err
		}
		mediaType := "movie"
		if id == "fixture-music" {
			mediaType = "track"
		}
		if _, err = tx.Exec(`INSERT INTO media_items(id,library_id,type,title,sort_title,source_url,duration_seconds,added_at) VALUES(?, 'fixture-library',?,?,?,?,2,?)`, id, mediaType, id, id, path, now); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO media_files(id,media_id,library_id,path,container,source_type,size_bytes,mod_time,available,first_seen_at,last_seen_at) VALUES(?,?,?,?,?,'local',?,?,1,?,?)`, id+"-file", id, "fixture-library", path, t.container, stat.Size(), stat.ModTime().UTC().Format(time.RFC3339Nano), now, now); err != nil {
			return err
		}
		if t.video != "" {
			if _, err = tx.Exec(`INSERT INTO media_streams(id,media_id,file_id,stream_index,kind,codec,bitrate,width,height,profile,pixel_format,bit_depth,frame_rate,display_title) VALUES(?,?,?,0,'video',?,500000,320,180,'main','yuv420p',8,24,'Fixture video')`, id+"-video", id, id+"-file", t.video); err != nil {
				return err
			}
		}
		audioProfile := ""
		if t.audio == "aac" {
			audioProfile = "lc"
		}
		if _, err = tx.Exec(`INSERT INTO media_streams(id,media_id,file_id,stream_index,kind,codec,profile,language,channels,bitrate,channel_layout,sample_rate,display_title) VALUES(?,?,?,1,'audio',?,?,'und',2,128000,'stereo',48000,'Fixture audio')`, id+"-audio", id, id+"-file", t.audio, audioProfile); err != nil {
			return err
		}
		if id == "fixture-multitrack" {
			if _, err = tx.Exec(`UPDATE media_streams SET language='eng', display_title='English stereo' WHERE id=?`, id+"-audio"); err != nil {
				return err
			}
			if _, err = tx.Exec(`INSERT INTO media_streams(id,media_id,file_id,stream_index,kind,codec,profile,language,channels,bitrate,channel_layout,sample_rate,display_title) VALUES(?,?,?,2,'audio','aac','lc','fra',2,128000,'stereo',48000,'French stereo')`, id+"-audio-fra", id, id+"-file"); err != nil {
				return err
			}
			if _, err = tx.Exec(`INSERT INTO media_streams(id,media_id,file_id,stream_index,kind,codec,language,display_title) VALUES(?,?,?,3,'subtitle','webvtt','eng','English subtitles')`, id+"-subtitle-eng", id, id+"-file"); err != nil {
				return err
			}
		}
		if id == "fixture-remux" || id == "fixture-transcode" || id == "fixture-multitrack" || id == "fixture-music" {
			exact := true
			revision := "fixture-source-v1:" + id
			container := t.container
			if container == "mkv" {
				container = "matroska"
			}
			facts := mediafacts.Facts{Version: mediafacts.SchemaVersion, Source: mediafacts.Source{Fingerprint: revision, Revision: revision, SizeBytes: stat.Size()}, Container: container, DurationUS: 2_000_000, DurationConfidence: mediafacts.ConfidenceExact, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown,
				Video: []mediafacts.Video{{Index: 0, Codec: "h264", Profile: "main", CodedWidth: 320, CodedHeight: 180, SampleAspectRatio: mediafacts.Rational{Num: 1, Den: 1}, DisplayAspectRatio: mediafacts.Rational{Num: 16, Den: 9}, PixelFormat: "yuv420p", BitDepth: 8, ChromaSubsampling: "4:2:0", FrameRate: mediafacts.Rational{Num: 24, Den: 1}, VariableFrameRateConfidence: mediafacts.ConfidenceUnknown, ExactSeekSafe: &exact, KeyframeEvidenceAt: now, KeyframeEvidenceRevision: revision, Timing: mediafacts.Timing{Duration: &mediafacts.Rational{Num: 2, Den: 1}, DurationConfidence: mediafacts.ConfidenceExact}}},
				Audio: []mediafacts.Audio{{Index: 1, Codec: "aac", Profile: "lc", Layout: "stereo", Channels: 2, SampleRate: 48000, Language: "und", Timing: mediafacts.Timing{Duration: &mediafacts.Rational{Num: 2, Den: 1}, DurationConfidence: mediafacts.ConfidenceExact}}}}
			if id == "fixture-transcode" {
				facts.Video[0].Codec, facts.Video[0].Profile = "mpeg4", ""
				facts.Audio[0].Codec, facts.Audio[0].Profile = "mp3", ""
			}
			if id == "fixture-music" {
				facts.Video = nil
				facts.Audio[0].Codec, facts.Audio[0].Profile = "mp3", ""
			}
			if id == "fixture-multitrack" {
				facts.Audio[0].Language = "eng"
				facts.Audio = append(facts.Audio, mediafacts.Audio{Index: 2, Codec: "aac", Profile: "lc", Layout: "stereo", Channels: 2, SampleRate: 48000, Language: "fra", Timing: mediafacts.Timing{Duration: &mediafacts.Rational{Num: 2, Den: 1}, DurationConfidence: mediafacts.ConfidenceExact}})
				facts.Subtitles = append(facts.Subtitles, mediafacts.Subtitle{Index: 3, Codec: "webvtt", Kind: "text", Language: "eng", Timing: mediafacts.Timing{Duration: &mediafacts.Rational{Num: 2, Den: 1}, DurationConfidence: mediafacts.ConfidenceExact}})
			}
			canonical, canonicalErr := facts.Canonical()
			if canonicalErr != nil {
				return canonicalErr
			}
			body, bodyErr := canonical.StableJSON()
			if bodyErr != nil {
				return bodyErr
			}
			digest, digestErr := canonical.Digest()
			if digestErr != nil {
				return digestErr
			}
			if _, err = tx.Exec(`INSERT INTO media_analysis_facts(media_id,media_file_id,schema_version,source_revision,source_fingerprint,facts_digest,facts_json,analyzed_at) VALUES(?,?,?,?,?,?,?,?)`, id, id+"-file", mediafacts.SchemaVersion, revision, revision, digest, string(body), now); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func writeManifest(path string, value manifest) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func fatal(err error) { fmt.Fprintln(os.Stderr, "portico-playback-fixture:", err); os.Exit(1) }
