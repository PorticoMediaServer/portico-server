package playback

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"portico.local/server/internal/assets"
	"regexp"
	"strconv"
	"strings"
	"time"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/decoder"
	"portico.local/server/internal/mediaexec"
	"portico.local/server/internal/supervise"
)

// AudioRenditionPlan describes independently produced audio on the source clock.
// A published but unrequested rendition owns no process or generated segments.
type AudioRenditionPlan struct {
	Failure     string `json:"failure,omitempty"`
	Ordinal     int    `json:"ordinal"`
	StreamIndex int    `json:"streamIndex"`
	Action      string `json:"action"`
	Codec       string `json:"codec"`
	Channels    int    `json:"channels"`
	BitrateBPS  int    `json:"bitrateBps"`
	Downmix     string `json:"downmix,omitempty"`
	Language    string `json:"language"`
	Label       string `json:"label"`
	Default     bool   `json:"default"`
	Commentary  bool   `json:"commentary,omitempty"`
}

func planAudioRenditions(p DeliveryPlan, source DeliverySource, client ClientProfile, target QualityTarget, convert bool) []AudioRenditionPlan {
	if p.Mode != "hls" || p.VideoAction == "none" || len(source.Audio) < 2 {
		return nil
	}
	transport := transportNamed(client.Transports, TransportHLSTS)
	if transport == nil {
		transport = &ClientTransport{Transport: TransportHLSTS, Audio: []string{"aac"}}
	}
	out := []AudioRenditionPlan{}
	// Keep the chosen track even when it falls beyond the bounded offer window.
	tracks := source.Audio[:min(16, len(source.Audio))]
	if p.AudioStream >= 0 {
		found := false
		for _, a := range tracks {
			found = found || a.Index == p.AudioStream
		}
		if !found {
			tracks = append([]SourceAudio{}, tracks...)
			for _, a := range source.Audio {
				if a.Index == p.AudioStream {
					tracks[len(tracks)-1] = a
					break
				}
			}
		}
	}
	for _, a := range tracks {
		r := AudioRenditionPlan{Ordinal: len(out), StreamIndex: a.Index, Action: "copy", Codec: a.Codec, Channels: a.Channels, BitrateBPS: int(a.BitRate), Language: manifestAudioLanguage(a.Language), Default: a.Index == p.AudioStream, Commentary: a.Commentary}
		if len(audioRejections(&a, client, *transport)) > 0 || !serverCopyAudio(TransportHLSTS, a.Codec) || bitrateOver(a.BitRate, target.MaxAudioBitrateBPS) {
			if !convert {
				continue
			}
			t := audioConversionTarget(&a, client, target)
			r.Action, r.Codec, r.Channels, r.BitrateBPS = "convert", t.Codec, t.Channels, t.Bitrate
			if t.Downmix {
				r.Downmix = DownmixITULimited
			}
		}
		r.Label = fmt.Sprintf("%s · %s · track %d", r.Language, strings.ToUpper(r.Codec), a.Ordinal+1)
		if a.Title != "" {
			r.Label = a.Title + " · " + r.Label
		}
		if a.Commentary {
			r.Label += " · Commentary"
		}
		// The API label is also the HLS NAME identity used for track selection.
		r.Label = manifestQuoted(r.Label)
		out = append(out, r)
	}
	if len(out) < 2 {
		return nil
	}
	return out
}

func audioCodecString(codec string) string {
	switch codec {
	case "aac":
		return "mp4a.40.2"
	case "ac3":
		return "ac-3"
	case "eac3":
		return "ec-3"
	case "mp3":
		return "mp4a.40.34"
	case "opus":
		return "opus"
	}
	return ""
}

func videoCodecString(p *DeliveryPlan) string {
	switch p.OutputVideoCodec {
	case "h264":
		profile, level := 100, 40
		if p.VideoAction == "copy" && p.Trace != nil && p.Trace.Video != nil {
			v := p.Trace.Video
			if v.Level > 0 {
				level = v.Level
			}
			switch strings.ToLower(v.Profile) {
			case "baseline", "constrained baseline":
				profile = 66
			case "main":
				profile = 77
			case "high 10":
				profile = 110
			}
		} else if p.TargetHeight > 1080 || (p.TargetHeight == 0 && p.Trace != nil && p.Trace.Video != nil && p.Trace.Video.Height > 1080) {
			level = 51
		}
		return fmt.Sprintf("avc1.%02x00%02x", profile, level)
	case "hevc":
		if hevcSampleEntry(p) == "dvh1" {
			return fmt.Sprintf("dvh1.%02d.%02d", p.Trace.Video.DolbyVisionProfile, max(1, p.Trace.Video.DolbyVisionLevel))
		}
		level := 120
		main10 := false
		if p.Trace != nil && p.Trace.Video != nil {
			v := p.Trace.Video
			main10 = v.BitDepth > 8
			if v.Level > 0 {
				level = v.Level
			}
		}
		if main10 {
			return fmt.Sprintf("hvc1.2.4.L%d.B0", level)
		}
		return fmt.Sprintf("hvc1.1.6.L%d.B0", level)
	case "av1":
		level, depth, profile := 8, 8, 0
		if p.Trace != nil && p.Trace.Video != nil {
			v := p.Trace.Video
			if v.Level >= 0 && v.Level <= 31 {
				level = v.Level
			}
			if v.BitDepth == 10 || v.BitDepth == 12 {
				depth = v.BitDepth
			}
			switch strings.ToLower(v.Profile) {
			case "high":
				profile = 1
			case "professional":
				profile = 2
			}
		}
		return fmt.Sprintf("av01.%d.%02dM.%02d", profile, level, depth)
	case "vp9":
		return "vp09.00.41.08"
	}
	return ""
}

func manifestQuoted(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '"' || r == '\\' || r < 32 || r == 127 {
			return -1
		}
		return r
	}, s)
}

func hlsMasterManifest(p *DeliveryPlan) []byte {
	var b strings.Builder
	version := 4
	if p.OutputContainer == "fmp4_hls" {
		version = 7
	}
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:%d\n#EXT-X-INDEPENDENT-SEGMENTS\n", version)
	for i, r := range p.TextRenditions {
		fmt.Fprintf(&b, "#EXT-X-MEDIA:TYPE=SUBTITLES,GROUP-ID=\"subtitles\",NAME=\"%s\",LANGUAGE=\"%s\",DEFAULT=NO,AUTOSELECT=NO,URI=\"text-%d.m3u8\"\n", manifestQuoted(r.Label), manifestAudioLanguage(r.Language), i)
	}
	codecs := []string{videoCodecString(p)}
	audioRate := p.AudioBitrateBPS
	for _, r := range p.Renditions {
		def, auto := "NO", "YES"
		if r.Default {
			def = "YES"
		}
		if r.Commentary {
			auto = "NO"
		}
		fmt.Fprintf(&b, "#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"audio\",NAME=\"%s\",LANGUAGE=\"%s\",DEFAULT=%s,AUTOSELECT=%s,CHANNELS=\"%d\",URI=\"audio-%d.m3u8\"\n", manifestQuoted(r.Label), r.Language, def, auto, max(1, r.Channels), r.Ordinal)
		if c := audioCodecString(r.Codec); c != "" && !containsString(codecs, c) {
			codecs = append(codecs, c)
		}
		audioRate = max(audioRate, r.BitrateBPS)
	}
	if len(p.Renditions) == 0 && p.AudioAction != "none" {
		if c := audioCodecString(p.OutputAudioCodec); c != "" {
			codecs = append(codecs, c)
		}
	}
	videoRate := p.VideoBitrateBPS
	if videoRate == 0 && p.Trace != nil {
		videoRate = int(p.Trace.BitRate)
	}
	if videoRate <= 0 {
		videoRate = 8_000_000
	}
	fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d,CODECS=\"%s\"", videoRate+max(audioRate, 192000), strings.Join(codecs, ","))
	if hevcSampleEntry(p) == "dvh1" && p.Trace.Video.DolbyVisionProfile == 8 {
		brand := ""
		switch p.Trace.Video.DolbyVisionCompatibility {
		case 1:
			brand = "db1p"
		case 4:
			brand = "db4h"
		}
		if brand != "" {
			fmt.Fprintf(&b, ",SUPPLEMENTAL-CODECS=\"%s/%s\"", videoCodecString(p), brand)
		}
	}
	if len(p.Renditions) > 0 {
		b.WriteString(",AUDIO=\"audio\"")
	}
	if len(p.TextRenditions) > 0 {
		b.WriteString(",SUBTITLES=\"subtitles\"")
	}
	if p.Trace != nil && p.Trace.Video != nil {
		v := p.Trace.Video
		w, h := v.Width, v.Height
		if p.TargetHeight > 0 && h > p.TargetHeight {
			w = (w * p.TargetHeight / h) / 2 * 2
			h = p.TargetHeight
		}
		if w > 0 && h > 0 {
			fmt.Fprintf(&b, ",RESOLUTION=%dx%d", w, h)
		}
		rate := v.FrameRate
		if p.MaxFrameRate > 0 {
			rate = min(rate, p.MaxFrameRate)
		}
		if rate > 0 {
			fmt.Fprintf(&b, ",FRAME-RATE=%.3f", rate)
		}
		dr := "SDR"
		if !p.ToneMap && p.VideoAction == "copy" {
			dynamicRange := v.OutputRange
			if dynamicRange == "" {
				dynamicRange = v.DynamicRange
			}
			switch dynamicRange {
			case assets.RangeHDR10, "hdr10_plus", assets.RangeDolbyVision:
				dr = "PQ"
			case assets.RangeHLG:
				dr = "HLG"
			}
		}
		fmt.Fprintf(&b, ",VIDEO-RANGE=%s", dr)
	}
	b.WriteString("\nvideo.m3u8\n")
	return []byte(b.String())
}

func writeNamedManifest(dir, name string, data []byte) error {
	path := filepath.Join(dir, name)
	if existing, err := os.ReadFile(path); err == nil && string(existing) == string(data) {
		return nil
	}
	if err := os.WriteFile(path+".tmp", data, 0600); err != nil {
		return err
	}
	return os.Rename(path+".tmp", path)
}

func publishRenditionManifests(dir string, p *DeliveryPlan, video []byte) error {
	if p == nil || (len(p.Renditions) == 0 && len(p.TextRenditions) == 0 && p.OutputContainer != "fmp4_hls") {
		return writeManifest(dir, video)
	}
	if err := writeNamedManifest(dir, "video.m3u8", video); err != nil {
		return err
	}
	for _, r := range p.Renditions {
		media := strings.ReplaceAll(string(hlsTimelineManifest(p.Duration)), "segment-", fmt.Sprintf("audio-%d-", r.Ordinal))
		if err := writeNamedManifest(dir, fmt.Sprintf("audio-%d.m3u8", r.Ordinal), []byte(media)); err != nil {
			return err
		}
	}
	if err := publishTextManifests(dir, p); err != nil {
		return err
	}
	return writeManifest(dir, hlsMasterManifest(p))
}

func renditionKey(id string, ordinal int) string { return id + "#a" + strconv.Itoa(ordinal) }
func producerSession(key string) string          { id, _, _ := strings.Cut(key, "#a"); return id }
func (h *HLS) sessionActiveLocked(id string) bool {
	for key := range h.active {
		if producerSession(key) == id {
			return true
		}
	}
	return false
}

var renditionSegmentName = regexp.MustCompile(`^audio-([0-9]{1,2})-([0-9]{6})\.ts$`)

// startRendition uses the same process and restart budgets as video. Only an
// explicitly requested track is admitted; all windows stop with their session.
func (h *HLS) startRendition(ctx context.Context, id string, r AudioRenditionPlan, from int) error {
	if r.Failure != "" {
		return ErrAudioRenditionUnavailable
	}
	key := renditionKey(id, r.Ordinal)
	h.mu.Lock()
	defer h.mu.Unlock()
	var valid bool
	if err := dbwork.QueryRow(ctx, h.db, `SELECT EXISTS(SELECT 1 FROM playback_sessions WHERE id=? AND state NOT IN('stopped','ended','failed') AND expires_at>?)`, id, time.Now().UTC().Format(time.RFC3339)).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return errors.New("playback stopped")
	}
	if w := h.windows[key]; w != nil {
		if w.coversDemand(from) {
			return nil
		}
		if w.relocating {
			h.mu.Unlock()
			select {
			case <-w.done:
			case <-ctx.Done():
				h.mu.Lock()
				return ctx.Err()
			}
			err := h.startRendition(ctx, id, r, from)
			h.mu.Lock()
			return err
		}
		if !h.restartAllowedLocked(id) {
			return nil
		}
		w.relocating = true
		w.cancel()
		h.mu.Unlock()
		select {
		case <-w.done:
		case <-ctx.Done():
			h.mu.Lock()
			return ctx.Err()
		}
		h.mu.Lock()
	}
	if h.windows[key] != nil {
		return nil
	}
	if !h.roomToProduce() {
		return ErrConversionSpace
	}
	run, cancel := context.WithCancel(h.ctx)
	w := &hlsWindow{start: from, served: -1, produced: from - 1, cancel: cancel, done: make(chan struct{})}
	h.active[key] = cancel
	h.windows[key] = w
	h.noteSessionServedLocked(id)
	h.producers.Add(1)
	supervise.Go("playback.hls.audio-rendition", func() {
		defer h.producers.Done()
		defer func() { h.mu.Lock(); delete(h.active, key); delete(h.windows, key); close(w.done); h.mu.Unlock() }()
		h.produceRendition(run, id, r, from, w)
	})
	return nil
}

func (h *HLS) produceRendition(ctx context.Context, id string, r AudioRenditionPlan, from int, w *hlsWindow) {
	var generation int
	if dbwork.QueryRow(ctx, h.db, `SELECT generation FROM playback_sessions WHERE id=?`, id).Scan(&generation) != nil {
		return
	}
	ctx = withProducerGeneration(ctx, generation)
	failed := func(code string) {
		if r.Default {
			h.failedWith(ctx, id, code, "")
		} else {
			h.renditionFailed(ctx, id, generation, r, code)
		}
	}
	plan, err := loadDeliveryPlan(h.db, id)
	if err != nil {
		failed(FailureSourceUnreadable)
		return
	}
	input, remoteInput, err := h.openWindowInput(ctx, id, plan)
	if err != nil {
		if ctx.Err() == nil {
			failed(FailureSourceUnreadable)
		}
		return
	}
	defer input.Close()
	if remoteInput != nil {
		defer remoteInput.Close()
	}
	dir := filepath.Join(h.root, id)
	if err = os.MkdirAll(dir, 0700); err != nil {
		failed(FailureOutOfSpace)
		return
	}
	offset := strconv.Itoa(from * HLSSegmentSeconds)
	args := []string{"-nostdin", "-hide_banner", "-loglevel", "error", "-y", "-protocol_whitelist", "file,pipe", "-ss", offset, "-i", input.Argument, "-ss", "0", "-map", "0:" + strconv.Itoa(r.StreamIndex), "-vn", "-sn", "-dn"}
	graph, err := decoder.BuildConversion(decoder.ConversionRequest{CopyAudio: r.Action == "copy", ConvertAudio: r.Action == "convert", AudioCodec: r.Codec, AudioChannels: r.Channels, AudioBitrateBPS: r.BitrateBPS, Downmix: r.Downmix == DownmixITULimited})
	if err != nil {
		failed(FailureConverter)
		return
	}
	args = append(args, graph.Audio...)
	args = append(args, "-max_muxing_queue_size", "1024", "-f", "hls", "-hls_time", "6", "-hls_list_size", "0", "-hls_flags", "independent_segments+temp_file", "-output_ts_offset", offset, "-muxdelay", "0", "-muxpreload", "0", "-start_number", strconv.Itoa(from), "-hls_segment_filename", filepath.Join(dir, fmt.Sprintf("audio-%d-%%06d.ts", r.Ordinal)), filepath.Join(dir, fmt.Sprintf("audio-%d-window.m3u8", r.Ordinal)))
	run, cancel := context.WithTimeout(ctx, 6*time.Hour)
	defer cancel()
	watchDone := make(chan struct{})
	supervise.Go("playback.hls.watch-audio", func() {
		defer close(watchDone)
		tick := time.NewTicker(250 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-tick.C:
			}
			h.mu.Lock()
			produced := w.produced
			h.mu.Unlock()
			for produced+1 < hlsSegmentCount(plan.Duration) && segmentReady(filepath.Join(dir, fmt.Sprintf("audio-%d-%06d.ts", r.Ordinal, produced+1))) {
				produced++
			}
			h.mu.Lock()
			w.produced = produced
			served := max(w.served, w.start-1)
			h.mu.Unlock()
			limit := max(12, h.configuration().ThrottleBufferSeconds)
			edge := served + (limit+5)/6
			if segmentReady(filepath.Join(dir, fmt.Sprintf("audio-%d-%06d.ts", r.Ordinal, edge))) {
				h.mu.Lock()
				w.throttled = true
				h.mu.Unlock()
				cancel()
				return
			}
		}
	})
	if input.File != nil {
		_, _ = input.File.Seek(0, io.SeekStart)
	}
	tail := &tailWriter{limit: 4096}
	if remoteInput != nil {
		err = remoteInput.Run(run, dir, func(url string) ([]string, error) {
			copied := append([]string{}, args...)
			for i := 0; i+1 < len(copied); i++ {
				if copied[i] == "-i" {
					copied[i+1] = url
					break
				}
				if copied[i] == "-protocol_whitelist" {
					copied[i+1] = "http,tcp"
				}
			}
			return copied, nil
		})
	} else {
		var cmd *exec.Cmd
		if cmd, err = mediaexec.CommandContext(run, mediaexec.Job{Executable: h.binary, Args: args, Files: input.ExtraFiles(), ReadPaths: input.ReadPaths(), WriteDirs: []string{dir}}); err == nil {
			cmd.WaitDelay = 2 * time.Second
			cmd.Stderr = tail
			err = cmd.Run()
		}
	}
	cancel()
	<-watchDone
	h.mu.Lock()
	throttled := w.throttled
	h.mu.Unlock()
	if err != nil && !throttled && ctx.Err() == nil {
		failed(classifyConverterFailure(tail.String()))
	}
}

func (h *HLS) renditionFile(ctx context.Context, id, name string, p *DeliveryPlan) (string, error) {
	path := filepath.Join(h.root, id, name)
	parts := renditionSegmentName.FindStringSubmatch(name)
	if parts == nil {
		return path, nil
	}
	ordinal, _ := strconv.Atoi(parts[1])
	index, _ := strconv.Atoi(parts[2])
	if ordinal >= len(p.Renditions) || index >= hlsSegmentCount(p.Duration) {
		return "", errors.New("unknown stream artifact")
	}
	key := renditionKey(id, ordinal)
	if segmentReady(path) {
		h.noteRenditionServed(id, key, index)
		return path, nil
	}
	if err := h.startRendition(ctx, id, p.Renditions[ordinal], index); err != nil {
		return "", err
	}
	return h.awaitSegment(ctx, key, index, func(ctx context.Context) (string, error) {
		wait, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		tick := time.NewTicker(100 * time.Millisecond)
		defer tick.Stop()
		for {
			var failure string
			if e := dbwork.QueryRow(ctx, h.db, `SELECT COALESCE(json_extract(renditions_json,?), '') FROM playback_delivery_plans WHERE session_id=?`, fmt.Sprintf("$[%d].failure", ordinal), id).Scan(&failure); e != nil {
				return "", e
			}
			if failure != "" {
				return "", ErrAudioRenditionUnavailable
			}
			if segmentReady(path) {
				h.noteRenditionServed(id, key, index)
				return path, nil
			}
			select {
			case <-wait.Done():
				if ctx.Err() != nil {
					return "", ctx.Err()
				}
				return "", ErrSegmentPreparing
			case <-tick.C:
			}
		}
	})
}

func hevcSampleEntry(p *DeliveryPlan) string {
	if p != nil && p.OutputVideoCodec == "hevc" && p.VideoAction == "copy" && p.Trace != nil && p.Trace.Video != nil && p.Trace.Video.OutputRange == assets.RangeDolbyVision && (p.Trace.Video.DolbyVisionProfile == 5 || p.Trace.Video.DolbyVisionProfile == 8) {
		return "dvh1"
	}
	return "hvc1"
}

var ErrAudioRenditionUnavailable = errors.New("This audio track is unavailable for this playback. Choose another audio track.")

func (h *HLS) renditionFailed(ctx context.Context, id string, generation int, r AudioRenditionPlan, code string) {
	write, cancel := persist(ctx)
	defer cancel()
	_, _ = dbwork.ExecWrite(write, h.db, dbwork.ClassEstablishedPlayback, `UPDATE playback_delivery_plans SET renditions_json=json_set(renditions_json,?,?) WHERE session_id=? AND json_extract(renditions_json,?)=? AND EXISTS(SELECT 1 FROM playback_sessions WHERE id=? AND generation=?)`, fmt.Sprintf("$[%d].failure", r.Ordinal), code, id, fmt.Sprintf("$[%d].streamIndex", r.Ordinal), r.StreamIndex, id, generation)
}

func (h *HLS) noteRenditionServed(id, key string, index int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.noteSessionServedLocked(id)
	if w := h.windows[key]; w != nil {
		w.served = max(w.served, index)
	}
}
