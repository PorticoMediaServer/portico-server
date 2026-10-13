package playbackruntime

import (
	"portico.local/server/internal/playback"
	"testing"
)

func TestLinearUsesPublishedProfileAndBoundsConvertedPicture(t *testing.T) {
	w := playback.LinearWork{ClientProfile: playback.BaselineClientProfile(), TranscodingEnabled: true}
	w.Selection.Quality.AllowLossy = true
	w.Selection.Quality.AllowHDRToSDR = true
	w.ClientProfile.Video[0].MaxFrameRate = 30
	w.ClientProfile.Video[0].MaxLevel = 41
	w.ClientProfile.Video[0].MaxHeight = 1080
	p, e := linearPlan([]byte(`{"streams":[{"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p","width":3840,"height":2160,"level":52,"avg_frame_rate":"60/1","field_order":"tt"},{"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2,"sample_rate":"48000"}]}`), w)
	if e != nil {
		t.Fatal(e)
	}
	if !p.ConvertVideo || !p.Deinterlace || p.MaxHeight != 1080 || p.MaxFrameRate > 30 || p.Level != 41 {
		t.Fatalf("%+v", p)
	}
	w.ClientProfile.Transports = nil
	if _, e = linearPlan([]byte(`{"streams":[{"codec_type":"audio","codec_name":"aac","channels":2}]}`), w); e == nil {
		t.Fatal("admitted unavailable output transport")
	}
}

// A remote bitrate cap reaches the linear plan through the session's stored
// profile: narrowed to 3 Mbit/s, a 20 Mbit/s 1080p H.264 source converts at
// the ceiling; the un-narrowed baseline copies it, as today.
func TestLinearPlanHonorsNarrowedProfile(t *testing.T) {
	probe := []byte(`{"streams":[{"codec_type":"video","codec_name":"h264","profile":"High","pix_fmt":"yuv420p","width":1920,"height":1080,"level":40,"avg_frame_rate":"30/1","field_order":"progressive","bit_rate":"20000000"},{"codec_type":"audio","codec_name":"aac","profile":"LC","channels":2,"sample_rate":"48000"}]}`)
	work := func(p playback.ClientProfile) playback.LinearWork {
		w := playback.LinearWork{ClientProfile: p, TranscodingEnabled: true}
		w.Selection.Quality.AllowLossy = true
		return w
	}
	base, err := linearPlan(probe, work(playback.BaselineClientProfile()))
	if err != nil {
		t.Fatal(err)
	}
	if base.ConvertVideo || base.VideoBitrate != 0 {
		t.Fatalf("baseline plan changed: %+v", base)
	}
	capped, err := linearPlan(probe, work(playback.WithVideoBitrateCeiling(playback.BaselineClientProfile(), 3000000)))
	if err != nil {
		t.Fatal(err)
	}
	if !capped.ConvertVideo || capped.VideoBitrate != 3000000 {
		t.Fatalf("capped plan: %+v", capped)
	}
}

func TestLinearCopiesCompatibleCodecsAndUnknownLiveRates(t *testing.T) {
	for _, codec := range []string{"h264", "hevc", "mpeg2video"} {
		p := playback.BaselineClientProfile()
		p.Video = []playback.ClientVideoCodec{{Codec: codec, MaxFrameRate: 60, MaxBitrateBPS: 400000000, BitDepths: []int{8}, DynamicRanges: []string{"sdr"}, Evidence: playback.EvidenceBuiltin}}
		p.Audio = []playback.ClientAudioCodec{{Codec: "ac3", MaxChannels: 6, Evidence: playback.EvidenceBuiltin}}
		p.AudioOutput.MaxChannels = 6
		for i := range p.Transports {
			if p.Transports[i].Transport == playback.TransportHLSTS {
				p.Transports[i].Video = []string{codec}
				p.Transports[i].Audio = []string{"ac3"}
			}
		}
		w := playback.LinearWork{ClientProfile: p, TranscodingEnabled: false}
		w.Selection.Quality.Mode = "original"
		w.Selection.Quality.AllowLossy = true
		raw := []byte(`{"streams":[{"codec_type":"video","codec_name":"` + codec + `","pix_fmt":"yuv420p","width":1920,"height":1080,"avg_frame_rate":"0/0","field_order":"progressive"},{"codec_type":"audio","codec_name":"ac3","channels":6,"sample_rate":"48000"}]}`)
		plan, e := linearPlan(raw, w)
		if e != nil || plan.ConvertVideo || plan.ConvertAudio {
			t.Fatalf("%s: %+v %v", codec, plan, e)
		}
	}
}

func TestLinearAudioConversionKeepsCopiedHDR(t *testing.T) {
	p := playback.BaselineClientProfile()
	p.Video = append(p.Video, playback.ClientVideoCodec{Codec: "hevc", BitDepths: []int{10}, DynamicRanges: []string{"hdr10"}, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60, Evidence: playback.EvidenceBuiltin})
	for i := range p.Transports {
		if p.Transports[i].Transport == playback.TransportHLSTS {
			p.Transports[i].Video = append(p.Transports[i].Video, "hevc")
		}
	}
	w := playback.LinearWork{ClientProfile: p, TranscodingEnabled: true}
	w.Selection.Quality.AllowLossy = true
	raw := []byte(`{"streams":[{"codec_type":"video","codec_name":"hevc","pix_fmt":"yuv420p10le","color_transfer":"smpte2084","width":1920,"height":1080,"avg_frame_rate":"30/1","field_order":"progressive"},{"codec_type":"audio","codec_name":"ac3","channels":6,"sample_rate":"48000"}]}`)
	plan, e := linearPlan(raw, w)
	if e != nil || plan.ConvertVideo || !plan.ConvertAudio || plan.HDRToSDR {
		t.Fatalf("%+v %v", plan, e)
	}
}
