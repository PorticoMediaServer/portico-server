package decoder

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestMusicUntouchedPCMBitExactR29(t *testing.T) {
	ff, e := exec.LookPath("ffmpeg")
	if e != nil {
		t.Skip("ffmpeg unavailable")
	}
	const rate, frames = 88200, 4096
	pcm := make([]byte, frames*2*3)
	seed := uint32(7)
	for n := range pcm {
		seed = seed*1664525 + 1013904223
		pcm[n] = byte(seed >> 24)
	}
	var wave bytes.Buffer
	wave.WriteString("RIFF")
	binary.Write(&wave, binary.LittleEndian, uint32(len(pcm)+36))
	wave.WriteString("WAVEfmt ")
	binary.Write(&wave, binary.LittleEndian, uint32(16))
	for _, n := range []uint16{1, 2} {
		binary.Write(&wave, binary.LittleEndian, n)
	}
	binary.Write(&wave, binary.LittleEndian, uint32(rate))
	binary.Write(&wave, binary.LittleEndian, uint32(rate*6))
	binary.Write(&wave, binary.LittleEndian, uint16(6))
	binary.Write(&wave, binary.LittleEndian, uint16(24))
	wave.WriteString("data")
	binary.Write(&wave, binary.LittleEndian, uint32(len(pcm)))
	wave.Write(pcm)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "tone.wav", time.Time{}, bytes.NewReader(wave.Bytes()))
	}))
	defer server.Close()
	spec := AnalysisSpec{Kind: "audio_flac", MaxDurationUS: 1000000, AudioFrames: frames, AudioOut: frames, AudioRate: rate, AudioSourceRate: rate, AudioChannels: 2, AudioSourceDepth: 24, AudioBitDepth: 24}
	args, e := AnalysisArgs(server.URL, spec)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(strings.Join(args, " "), "aresample") {
		t.Fatal("untouched source resampled", args)
	}
	encoded, e := exec.Command(ff, args...).Output()
	if e != nil {
		t.Fatal(e)
	}
	decode := exec.Command(ff, "-v", "error", "-i", "pipe:0", "-c:a", "pcm_s24le", "-f", "s24le", "pipe:1")
	decode.Stdin = bytes.NewReader(encoded)
	raw, e := decode.Output()
	if e != nil || !bytes.Equal(raw, pcm) {
		t.Fatal("untouched PCM changed", e, len(raw), len(pcm))
	}
	spec.AudioBitDepth = 16
	args, e = AnalysisArgs(server.URL, spec)
	if e != nil || !strings.Contains(strings.Join(args, " "), "resampler=soxr:precision=28:osf=s16:dither_method=triangular") {
		t.Fatal(args, e)
	}
}
