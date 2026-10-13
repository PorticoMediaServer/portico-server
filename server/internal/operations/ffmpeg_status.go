package operations

import (
	"context"
	"sync"
	"time"

	"fmt"
	"os"
	"os/exec"
	"portico.local/server/internal/decoder"
)

type FFmpegStatus struct {
	Path            string   `json:"path"`
	State           string   `json:"state"`
	InUse           bool     `json:"inUse"`
	ReasonCode      string   `json:"reasonCode,omitempty"`
	MissingFeatures []string `json:"missingFeatures"`
	CheckedAt       string   `json:"checkedAt"`
}

// FFmpegValidator checks the saved build with the same qualification and
// sandbox as startup. It does not install it or disturb active conversions.
func FFmpegValidator(active string) func(context.Context, string) FFmpegStatus {
	var mu sync.Mutex
	var previous FFmpegStatus
	var identity string
	var checked time.Time
	return func(ctx context.Context, path string) FFmpegStatus {
		if path == "" {
			path = active
		}
		mu.Lock()
		defer mu.Unlock()
		resolved, err := exec.LookPath(path)
		var fileIdentity string
		if err == nil {
			var info os.FileInfo
			info, err = os.Stat(resolved)
			if err == nil {
				fileIdentity = fmt.Sprintf("%s:%d:%d", resolved, info.Size(), info.ModTime().UnixNano())
			}
		}
		if path == previous.Path && err == nil && fileIdentity == identity && time.Since(checked) < time.Minute {
			return previous
		}
		out := FFmpegStatus{Path: path, State: "refused", InUse: path == active, MissingFeatures: []string{}, CheckedAt: time.Now().UTC().Format(time.RFC3339)}
		if err != nil {
			out.ReasonCode = "ffmpeg.validation.unreadable"
		} else {
			bounded, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			facts, e := decoder.ProbeToolchain(bounded, path)
			if e != nil {
				out.ReasonCode = "ffmpeg.validation.cannotRun"
				if bounded.Err() != nil {
					out.State = "unchecked"
					out.ReasonCode = "ffmpeg.validation.timedOut"
				}
			} else {
				out.MissingFeatures = facts.MissingRequired()
				if len(out.MissingFeatures) > 0 {
					out.ReasonCode = "ffmpeg.validation.missingFeatures"
				} else {
					out.State = "accepted"
				}
			}
		}
		previous, identity, checked = out, fileIdentity, time.Now()
		return out
	}
}
