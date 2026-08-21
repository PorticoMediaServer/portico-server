// Package ffmpegprobe verifies FFmpeg pipelines by executing bounded, representative
// commands. Registry/listing output is deliberately not treated as capability proof.
package ffmpegprobe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
)

type Kind string

const (
	Decode             Kind = "decode"
	Encode             Kind = "encode"
	BitDepth8          Kind = "bit_depth_8"
	BitDepth10         Kind = "bit_depth_10"
	HDRToneMap         Kind = "hdr_tone_map"
	ScaleDeinterlace   Kind = "scale_deinterlace"
	BitmapSubtitleBurn Kind = "bitmap_subtitle_burn"
	DownloadReupload   Kind = "download_filter_reupload"
	OutputReprobe      Kind = "output_reprobe"
)

type Status string

const (
	Supported   Status = "supported"
	Unsupported Status = "unsupported"
	Unavailable Status = "unavailable"
	Error       Status = "error"
	Failed             = Error // retained as a readable classification alias
	TimedOut    Status = "timeout"
)

// Identity contains every external input which can change executable support.
// Callers should use an actual binary hash/build identity, not merely a pathname.
type Identity struct {
	BinaryFingerprint string `json:"binaryFingerprint"`
	FFmpegBuild       string `json:"ffmpegBuild"`
	FFmpegConfigure   string `json:"ffmpegConfigure"`
	Backend           string `json:"backend"`
	DeviceIdentity    string `json:"deviceIdentity"`
	DevicePath        string `json:"devicePath"`
	DriverIdentity    string `json:"driverIdentity"`
	DriverVersion     string `json:"driverVersion"`
	OS                string `json:"os"`
	Arch              string `json:"arch"`
	ConfigRevision    string `json:"configRevision"`
}

func (i Identity) normalized() Identity {
	if i.OS == "" {
		i.OS = runtime.GOOS
	}
	if i.Arch == "" {
		i.Arch = runtime.GOARCH
	}
	return i
}

func (i Identity) Validate() error {
	i = i.normalized()
	for name, value := range map[string]string{
		"binary fingerprint": i.BinaryFingerprint, "FFmpeg build": i.FFmpegBuild,
		"FFmpeg configure": i.FFmpegConfigure, "backend": i.Backend,
		"device identity": i.DeviceIdentity, "device path": i.DevicePath, "driver identity": i.DriverIdentity,
		"driver version": i.DriverVersion, "config revision": i.ConfigRevision,
	} {
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("ffmpegprobe: missing %s", name)
		}
	}
	return nil
}

// Probe is an executable capability assertion. Args should create a small synthetic
// input and write to a null or temporary output; an OutputReprobe should validate it.
type Probe struct {
	Name string   `json:"name"`
	Kind Kind     `json:"kind"`
	Args []string `json:"args"`
	// Unsupported is reported only when both an allowed exit code and one of the
	// probe-specific diagnostic markers match. FFmpeg reuses generic exit codes
	// for bad inputs, missing devices, and driver failures, so an exit code alone
	// is never capability evidence.
	UnsupportedExitCodes         []int    `json:"unsupportedExitCodes,omitempty"`
	UnsupportedDiagnosticMarkers []string `json:"unsupportedDiagnosticMarkers,omitempty"`
	// UnavailableDiagnosticMarkers identify exact, probe-specific device or
	// runtime availability failures that the executor cannot classify itself.
	UnavailableDiagnosticMarkers []string `json:"unavailableDiagnosticMarkers,omitempty"`
}

type Command struct {
	Binary string
	Args   []string
}
type Execution struct {
	ExitCode                         int
	Output                           string
	Err                              error
	TimedOut, Unavailable, Truncated bool
}
type Executor interface {
	Run(context.Context, Command, int64) Execution
}

type Result struct {
	Name            string `json:"name"`
	Kind            Kind   `json:"kind"`
	Status          Status `json:"status"`
	Diagnostic      string `json:"diagnostic,omitempty"`
	OutputTruncated bool   `json:"outputTruncated,omitempty"`
}
type Report struct {
	Identity  Identity  `json:"identity"`
	Results   []Result  `json:"results"`
	ProbedAt  time.Time `json:"probedAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	FromCache bool      `json:"fromCache"`
}

type Cache interface {
	Get(string, time.Time) (Report, bool)
	Put(string, Report)
}
type Clock interface{ Now() time.Time }
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

type Runner struct {
	Binary         string
	Executor       Executor
	Cache          Cache
	Clock          Clock
	Timeout        time.Duration
	TTL            time.Duration
	TransientTTL   time.Duration
	MaxOutputBytes int64
}

func (r Runner) Probe(ctx context.Context, id Identity, probes []Probe) (Report, error) {
	if err := id.Validate(); err != nil {
		return Report{}, err
	}
	if strings.TrimSpace(r.Binary) == "" {
		return Report{}, errors.New("ffmpegprobe: missing binary")
	}
	if r.Executor == nil {
		r.Executor = OSExecutor{}
	}
	if r.Clock == nil {
		r.Clock = systemClock{}
	}
	if r.Timeout <= 0 {
		r.Timeout = 15 * time.Second
	}
	if r.TTL <= 0 {
		r.TTL = 24 * time.Hour
	}
	if r.TransientTTL <= 0 {
		r.TransientTTL = 30 * time.Second
	}
	if r.MaxOutputBytes <= 0 {
		r.MaxOutputBytes = 64 << 10
	}
	id = id.normalized()
	key, err := cacheKey(id, probes)
	if err != nil {
		return Report{}, err
	}
	now := r.Clock.Now().UTC()
	if r.Cache != nil {
		if cached, ok := r.Cache.Get(key, now); ok {
			cached.FromCache = true
			return cached, nil
		}
	}
	report := Report{Identity: id, ProbedAt: now, Results: make([]Result, 0, len(probes))}
	transient := false
	for _, p := range probes {
		if strings.TrimSpace(p.Name) == "" || p.Kind == "" || len(p.Args) == 0 {
			return Report{}, errors.New("ffmpegprobe: invalid empty probe")
		}
		probeCtx, cancel := context.WithTimeout(ctx, r.Timeout)
		ex := r.Executor.Run(probeCtx, Command{Binary: r.Binary, Args: append([]string(nil), p.Args...)}, r.MaxOutputBytes)
		cancel()
		result := Result{Name: p.Name, Kind: p.Kind, OutputTruncated: ex.Truncated}
		diagnosticSource := diagnosticText(ex.Output, ex.Err)
		switch {
		case ex.TimedOut || errors.Is(ex.Err, context.DeadlineExceeded) || errors.Is(probeCtx.Err(), context.DeadlineExceeded):
			result.Status = TimedOut
		case ex.Unavailable || errors.Is(ex.Err, exec.ErrNotFound):
			result.Status = Unavailable
		case matchesDiagnosticMarker(diagnosticSource, p.UnavailableDiagnosticMarkers):
			result.Status = Unavailable
		case ex.Err == nil && ex.ExitCode == 0:
			result.Status = Supported
		case containsInt(p.UnsupportedExitCodes, ex.ExitCode) && matchesDiagnosticMarker(diagnosticSource, p.UnsupportedDiagnosticMarkers):
			result.Status = Unsupported
		default:
			result.Status = Failed
		}
		if result.Status == TimedOut || result.Status == Unavailable || result.Status == Error {
			transient = true
		}
		result.Diagnostic = sanitizeDiagnostic(ex.Output, ex.Err, r.Binary, id.DevicePath)
		report.Results = append(report.Results, result)
	}
	report.ExpiresAt = now.Add(r.TTL)
	if transient && r.TransientTTL < r.TTL {
		report.ExpiresAt = now.Add(r.TransientTTL)
	}
	if r.Cache != nil {
		r.Cache.Put(key, report)
	}
	return report, nil
}

func cacheKey(id Identity, probes []Probe) (string, error) {
	copyProbes := append([]Probe(nil), probes...)
	for n := range copyProbes {
		copyProbes[n].Args = append([]string(nil), copyProbes[n].Args...)
		copyProbes[n].UnsupportedExitCodes = append([]int(nil), copyProbes[n].UnsupportedExitCodes...)
		sort.Ints(copyProbes[n].UnsupportedExitCodes)
		copyProbes[n].UnsupportedDiagnosticMarkers = append([]string(nil), copyProbes[n].UnsupportedDiagnosticMarkers...)
		copyProbes[n].UnavailableDiagnosticMarkers = append([]string(nil), copyProbes[n].UnavailableDiagnosticMarkers...)
		sort.Strings(copyProbes[n].UnsupportedDiagnosticMarkers)
		sort.Strings(copyProbes[n].UnavailableDiagnosticMarkers)
	}
	b, err := json.Marshal(struct {
		Identity Identity `json:"identity"`
		Probes   []Probe  `json:"probes"`
	}{id, copyProbes})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func diagnosticText(output string, err error) string {
	if err == nil {
		return output
	}
	if output == "" {
		return err.Error()
	}
	return output + "\n" + err.Error()
}

func matchesDiagnosticMarker(diagnostic string, markers []string) bool {
	diagnostic = strings.ToLower(diagnostic)
	for _, marker := range markers {
		marker = strings.ToLower(strings.TrimSpace(marker))
		if marker != "" && strings.Contains(diagnostic, marker) {
			return true
		}
	}
	return false
}

func containsInt(v []int, target int) bool {
	for _, n := range v {
		if n == target {
			return true
		}
	}
	return false
}

func sanitizeDiagnostic(output string, err error, secrets ...string) string {
	text := output
	if err != nil {
		if text != "" {
			text += ": "
		}
		text += err.Error()
	}
	for _, secret := range secrets {
		if strings.TrimSpace(secret) != "" {
			text = strings.ReplaceAll(text, secret, "[redacted]")
		}
	}
	words := strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) })
	for n, word := range words {
		lower := strings.ToLower(word)
		keyValue := strings.IndexByte(word, '=') > 0
		if keyValue || strings.Contains(lower, "authorization:") || strings.Contains(word, "/") || strings.Contains(word, `\`) {
			words[n] = "[redacted]"
		}
	}
	if len(words) > 48 {
		words = append(words[:48], "…")
	}
	result := strings.Join(words, " ")
	if len(result) > 512 {
		result = result[:512] + "…"
	}
	return result
}

// OSExecutor runs without inheriting environment variables and captures bounded combined output.
type OSExecutor struct{}

func (OSExecutor) Run(ctx context.Context, command Command, max int64) Execution {
	cmd := exec.CommandContext(ctx, command.Binary, command.Args...)
	cmd.Env = []string{}
	w := &boundedWriter{remaining: max}
	cmd.Stdout, cmd.Stderr = w, w
	err := cmd.Run()
	result := Execution{Output: w.String(), Err: err, ExitCode: -1, Truncated: w.truncated}
	if ctx.Err() != nil {
		result.TimedOut = true
		result.Err = ctx.Err()
		return result
	}
	if errors.Is(err, exec.ErrNotFound) || os.IsNotExist(err) || errors.Is(err, os.ErrPermission) {
		result.Unavailable = true
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.ExitCode = exitErr.ExitCode()
	} else if err == nil {
		result.ExitCode = 0
	}
	return result
}

type boundedWriter struct {
	mu        sync.Mutex
	remaining int64
	b         strings.Builder
	truncated bool
}

func (w *boundedWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	original := len(p)
	if w.remaining <= 0 {
		w.truncated = w.truncated || original > 0
		return original, nil
	}
	if int64(len(p)) > w.remaining {
		p = p[:w.remaining]
		w.truncated = true
	}
	_, _ = io.WriteString(&w.b, string(p))
	w.remaining -= int64(len(p))
	return original, nil
}
func (w *boundedWriter) String() string { w.mu.Lock(); defer w.mu.Unlock(); return w.b.String() }
