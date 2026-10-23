package decoder

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

func TestDecodeProbeRequiresHardwareFrames(t *testing.T) {
	seen := 0
	d := &HardwareDetector{Run: func(_ context.Context, cmd *exec.Cmd) error {
		args := confinedArguments(t, cmd)
		if strings.Contains(strings.Join(args, " "), "-hwaccel vaapi") {
			seen++
			if !strings.Contains(strings.Join(args, " "), "hwdownload") {
				t.Fatal("decoder fallback could pass without hardware frames", args)
			}
		}
		return nil
	}}
	d.probeDecodeCodecs(context.Background(), fakeFFmpeg(t), BackendVAAPI, "/dev/dri/renderD128", defaultProbeTimeout)
	if seen == 0 {
		t.Fatal("no decode commands exercised")
	}
}
