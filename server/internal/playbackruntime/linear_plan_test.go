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
