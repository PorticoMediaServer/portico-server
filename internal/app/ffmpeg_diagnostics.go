package app

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FFmpeg diagnostics are deliberately bounded. FFmpeg can emit one warning
// per packet for a damaged or unavailable source, so retaining the complete
// stream would turn a media fault into a server memory fault.
const (
	ffmpegDiagnosticMaxBytes  int64 = 64 * 1024
	ffmpegDiagnosticHeadBytes int   = 8 * 1024
	ffmpegDiagnosticLineBytes int   = 4 * 1024
)

type ffmpegDiagnosticRecorder struct {
	mu            sync.Mutex
	command       string
	startedAt     time.Time
	head          []byte
	tail          []byte
	totalBytes    int64
	totalLines    int64
	errorLines    int64
	progressLines int64
	truncated     bool
	line          []byte
	lineError     bool
	lineProgress  bool
}

type ffmpegDiagnosticReport struct {
	CommandIdentity string
	Text            string
	Bytes           int64
	Lines           int64
	Truncated       bool
	ErrorLines      int64
	ProgressLines   int64
	ExitCode        int
	Signal          string
	Duration        time.Duration
}

func newFFmpegDiagnosticRecorder(executable string, args []string) *ffmpegDiagnosticRecorder {
	return &ffmpegDiagnosticRecorder{
		command:   ffmpegDiagnosticCommandIdentity(executable, args),
		startedAt: time.Now().UTC(),
		head:      make([]byte, 0, ffmpegDiagnosticHeadBytes),
		tail:      make([]byte, 0, int(ffmpegDiagnosticMaxBytes)-ffmpegDiagnosticHeadBytes),
		line:      make([]byte, 0, ffmpegDiagnosticLineBytes),
	}
}

func (r *ffmpegDiagnosticRecorder) Write(p []byte) (int, error) {
	if r == nil || len(p) == 0 {
		return len(p), nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.totalBytes += int64(len(p))
	if len(p) > 0 {
		r.totalLines += int64(countNewlines(p))
	}
	r.appendOutputLocked(p)
	r.classifyLocked(p)
	return len(p), nil
}

func (r *ffmpegDiagnosticRecorder) appendOutputLocked(p []byte) {
	if len(r.head) < ffmpegDiagnosticHeadBytes {
		count := minInt(ffmpegDiagnosticHeadBytes-len(r.head), len(p))
		r.head = append(r.head, p[:count]...)
		p = p[count:]
	}
	if len(p) == 0 {
		return
	}
	maxTail := int(ffmpegDiagnosticMaxBytes) - ffmpegDiagnosticHeadBytes
	if len(p) >= maxTail {
		r.tail = append(r.tail[:0], p[len(p)-maxTail:]...)
	} else {
		r.tail = append(r.tail, p...)
		if len(r.tail) > maxTail {
			r.tail = append(r.tail[:0], r.tail[len(r.tail)-maxTail:]...)
		}
	}
	if r.totalBytes > ffmpegDiagnosticMaxBytes {
		r.truncated = true
	}
}

func (r *ffmpegDiagnosticRecorder) classifyLocked(p []byte) {
	for len(p) > 0 {
		if newline := indexNewline(p); newline >= 0 {
			r.addLineFlagsLocked(p[:newline])
			r.finishLineLocked()
			p = p[newline+1:]
			continue
		}
		r.addLineFlagsLocked(p)
		return
	}
}

func (r *ffmpegDiagnosticRecorder) addLineFlagsLocked(p []byte) {
	if len(p) == 0 {
		return
	}
	text := strings.ToLower(string(p))
	if strings.Contains(text, "error") || strings.Contains(text, "fatal") || strings.Contains(text, "failed") || strings.Contains(text, "invalid") {
		r.lineError = true
	}
	if strings.Contains(text, "frame=") || strings.Contains(text, "fps=") || strings.Contains(text, "out_time") || strings.Contains(text, "speed=") {
		r.lineProgress = true
	}
	if len(r.line) < ffmpegDiagnosticLineBytes {
		count := minInt(ffmpegDiagnosticLineBytes-len(r.line), len(p))
		r.line = append(r.line, p[:count]...)
	}
}

func (r *ffmpegDiagnosticRecorder) finishLineLocked() {
	r.line = r.line[:0]
	if r.lineError {
		r.errorLines++
	}
	if r.lineProgress {
		r.progressLines++
	}
	r.lineError = false
	r.lineProgress = false
}

func (r *ffmpegDiagnosticRecorder) Report(err error) ffmpegDiagnosticReport {
	if r == nil {
		return ffmpegDiagnosticReport{ExitCode: -1}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.line) > 0 || r.lineError || r.lineProgress {
		r.totalLines++
		r.finishLineLocked()
	}
	output := make([]byte, 0, len(r.head)+len(r.tail)+32)
	output = append(output, r.head...)
	if r.truncated {
		output = append(output, []byte("\n... [stderr truncated] ...\n")...)
	}
	if len(r.tail) > 0 {
		output = append(output, r.tail...)
	}
	finished := time.Now().UTC()
	return ffmpegDiagnosticReport{
		CommandIdentity: r.command,
		Text:            sanitizeFFmpegDiagnosticText(string(output)),
		Bytes:           r.totalBytes,
		Lines:           r.totalLines,
		Truncated:       r.truncated,
		ErrorLines:      r.errorLines,
		ProgressLines:   r.progressLines,
		ExitCode:        ffmpegDiagnosticExitCode(err),
		Signal:          ffmpegDiagnosticSignal(err),
		Duration:        finished.Sub(r.startedAt),
	}
}

func (r ffmpegDiagnosticReport) API() *FFmpegDiagnostics {
	if r.CommandIdentity == "" && r.Text == "" && r.Bytes == 0 && r.Lines == 0 && r.ExitCode == -1 {
		return nil
	}
	exitCode := r.ExitCode
	return &FFmpegDiagnostics{
		CommandIdentity: r.CommandIdentity,
		Stderr:          r.Text,
		StderrBytes:     r.Bytes,
		StderrLines:     r.Lines,
		StderrTruncated: r.Truncated,
		ErrorLines:      r.ErrorLines,
		ProgressLines:   r.ProgressLines,
		ExitCode:        exitCode,
		Signal:          r.Signal,
		DurationMillis:  maxInt64(0, r.Duration.Milliseconds()),
	}
}

func ffmpegDiagnosticCommandIdentity(executable string, args []string) string {
	base := filepath.Base(strings.TrimSpace(executable))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "ffmpeg"
	}
	context := redactedFFmpegContext(args)
	if strings.HasPrefix(context, "ffmpeg") {
		context = base + strings.TrimPrefix(context, "ffmpeg")
	}
	return context
}

func sanitizeFFmpegDiagnosticText(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		line = redactDiagnosticCredentials(line)
		line = redactDiagnosticParameter(line, "username")
		line = redactDiagnosticParameter(line, "user")
		line = redactDiagnosticParameter(line, "password")
		line = redactDiagnosticParameter(line, "passwd")
		line = redactDiagnosticParameter(line, "token")
		line = redactDiagnosticParameter(line, "api_key")
		line = redactDiagnosticParameter(line, "apikey")
		line = redactDiagnosticParameter(line, "authorization")
		line = redactDiagnosticParameter(line, "cookie")
		lines[i] = line
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func redactDiagnosticCredentials(line string) string {
	for _, scheme := range []string{"http://", "https://", "rtsp://", "rtmp://"} {
		for {
			start := strings.Index(line, scheme)
			if start < 0 {
				break
			}
			authorityStart := start + len(scheme)
			authorityEnd := len(line)
			if slash := strings.IndexAny(line[authorityStart:], "/ \\t\r\n"); slash >= 0 {
				authorityEnd = authorityStart + slash
			}
			authority := line[authorityStart:authorityEnd]
			if at := strings.LastIndex(authority, "@"); at >= 0 {
				if strings.HasPrefix(authority, "<credentials>@") {
					break
				}
				line = line[:authorityStart] + "<credentials>@" + authority[at+1:] + line[authorityEnd:]
			} else {
				break
			}
		}
	}
	return line
}

func redactDiagnosticParameter(line, key string) string {
	lower := strings.ToLower(line)
	for _, separator := range []string{"?", "&", " ", "\t", "\"", "'"} {
		search := separator + key + "="
		for {
			index := strings.Index(lower, search)
			if index < 0 {
				break
			}
			valueStart := index + len(search)
			if strings.HasPrefix(strings.ToLower(line[valueStart:]), "<redacted>") {
				break
			}
			valueEnd := valueStart
			for valueEnd < len(line) && !strings.ContainsRune("& \t\r\n\"'", rune(line[valueEnd])) {
				valueEnd++
			}
			line = line[:valueStart] + "<redacted>" + line[valueEnd:]
			lower = strings.ToLower(line)
		}
	}
	return line
}

func ffmpegDiagnosticExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ProcessState != nil {
		return exitErr.ProcessState.ExitCode()
	}
	return -1
}

func countNewlines(value []byte) int {
	count := 0
	for _, b := range value {
		if b == '\n' {
			count++
		}
	}
	return count
}

func indexNewline(value []byte) int {
	for i, b := range value {
		if b == '\n' {
			return i
		}
	}
	return -1
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
