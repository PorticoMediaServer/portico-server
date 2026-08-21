package app

import (
	"errors"
	"strings"
	"testing"
)

func TestFFmpegDiagnosticRecorderBoundsAndClassifiesOutput(t *testing.T) {
	recorder := newFFmpegDiagnosticRecorder("/opt/portico/bin/ffmpeg", []string{
		"-i", "https://user:password@example.test/live.m3u8?token=secret",
		"-headers", "Authorization: Bearer secret",
	})
	if _, err := recorder.Write([]byte("first error line\nframe=1 fps=30\n")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2000; i++ {
		if _, err := recorder.Write([]byte("repeated decoder warning: invalid packet\n")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := recorder.Write([]byte("last progress frame=9999 speed=1x")); err != nil {
		t.Fatal(err)
	}
	report := recorder.Report(errors.New("ffmpeg exited"))
	if report.Bytes <= ffmpegDiagnosticMaxBytes || !report.Truncated {
		t.Fatalf("report did not record bounded truncation: %+v", report)
	}
	if len(report.Text) > int(ffmpegDiagnosticMaxBytes)+64 {
		t.Fatalf("bounded report text length=%d", len(report.Text))
	}
	if report.Lines < 2000 || report.ErrorLines == 0 || report.ProgressLines == 0 {
		t.Fatalf("report counters=%+v", report)
	}
	if !strings.Contains(report.Text, "first error line") || !strings.Contains(report.Text, "last progress") || !strings.Contains(report.Text, "stderr truncated") {
		t.Fatalf("head/tail context missing: %q", report.Text)
	}
	if strings.Contains(report.Text, "password") || strings.Contains(report.Text, "secret") || strings.Contains(report.CommandIdentity, "secret") {
		t.Fatalf("credential leaked in diagnostics: report=%+v", report)
	}
	if !strings.Contains(report.CommandIdentity, "<media-source>") || !strings.Contains(report.CommandIdentity, "<provider-transport-redacted>") {
		t.Fatalf("command identity was not redacted: %q", report.CommandIdentity)
	}
}

func TestFFmpegDiagnosticRecorderCountsSplitLines(t *testing.T) {
	recorder := newFFmpegDiagnosticRecorder("ffmpeg", nil)
	_, _ = recorder.Write([]byte("frame=1 fps=1"))
	_, _ = recorder.Write([]byte(" speed=1x\nerror: failed\npartial"))
	report := recorder.Report(nil)
	if report.Lines != 3 || report.ProgressLines != 1 || report.ErrorLines != 1 {
		t.Fatalf("split-line counters=%+v", report)
	}
	if report.ExitCode != 0 {
		t.Fatalf("successful exit code=%d", report.ExitCode)
	}
}
