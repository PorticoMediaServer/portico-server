package decoder

import (
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/mediatools"
)

func CompanionProbe(ffmpeg, fallback string) string {
	if path, err := exec.LookPath(filepath.Join(filepath.Dir(ffmpeg), mediatools.Name("ffprobe"))); err == nil {
		return path
	}
	return fallback
}

// ConfigureResolvedTools is called only after the startup toolchain probe.
func ConfigureResolvedTools(ffmpeg, ffprobe string) error {
	return mediatools.ConfigurePair(ffmpeg, ffprobe)
}
