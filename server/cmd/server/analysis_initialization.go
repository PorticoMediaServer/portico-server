package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"portico.local/server/internal/mediaanalysis"
	"portico.local/server/internal/subtitlevideo"
)

// ffmpeg is the shared, already resolved and verified startup tool.
func initializeAnalysis(db *sql.DB, state string, inputs *subtitlevideo.Runtime, ffmpeg string) (*mediaanalysis.Service, error) {
	o := mediaanalysis.Options{DB: db, Directory: filepath.Join(state, "analysis-artifacts"), FFmpeg: ffmpeg, Open: func(ctx context.Context, item, asset string) (mediaanalysis.Input, error) {
		return inputs.OpenSubtitleInput(ctx, item, asset, "")
	}, Decoder: func(input mediaanalysis.Input) mediaanalysis.Decode { return inputs.AnalysisDecoder(input) }}
	// These are real operator budgets. No capability is suppressed because a
	// developer lacked a decoder, browser, simulator, or provider credentials.
	for name, dest := range map[string]*int64{"PORTICO_ANALYSIS_MAX_INPUT_BYTES": &o.MaxInputBytes, "PORTICO_ANALYSIS_MAX_GENERATED_BYTES": &o.MaxGeneratedBytes, "PORTICO_ANALYSIS_MAX_CACHE_BYTES": &o.MaxCacheBytes} {
		if value := os.Getenv(name); value != "" {
			n, e := strconv.ParseInt(value, 10, 64)
			if e != nil || n <= 0 {
				return nil, fmt.Errorf("invalid %s", name)
			}
			*dest = n
		}
	}
	for name, dest := range map[string]*int{"PORTICO_ANALYSIS_MAX_PREVIEW_FRAMES": &o.MaxPreviewFrames, "PORTICO_ANALYSIS_RETAIN_DAYS": &o.RetainDays} {
		if value := os.Getenv(name); value != "" {
			n, e := strconv.Atoi(value)
			if e != nil || n <= 0 {
				return nil, fmt.Errorf("invalid %s", name)
			}
			*dest = n
		}
	}
	return mediaanalysis.New(o)
}
