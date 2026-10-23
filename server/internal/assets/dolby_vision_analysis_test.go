package assets

import (
	"testing"
)

func TestDolbyVisionActualFrameMetadataSeparatesMELAndFEL(t *testing.T) {
	for _, test := range []struct{ residual, want string }{{"0", "FEL"}, {"1", "MEL"}} {
		t.Run(test.want, func(t *testing.T) {
			d := StreamDetail{DolbyVisionProfile: 7, EnhancementLayer: true}
			raw := []byte(`{"frames":[{"media_type":"video","width":160,"height":90,"side_data_list":[{"side_data_type":"Dolby Vision Metadata","rpu_type":2,"bl_bit_depth":10,"disable_residual_flag":` + test.residual + `}]}]}`)
			analyzeDolbyVisionFrames(raw, &d)
			if !d.DolbyVisionRPU || !d.DolbyVisionBaseLayer || !d.DolbyVisionMetadataKnown || d.DolbyVisionEnhancementType != test.want || d.DolbyVisionCMVersion != "" {
				t.Fatalf("actual RPU facts: %+v", d)
			}
		})
	}
	for _, raw := range []string{`{`, `{"frames":[]}`, `{"frames":[{"media_type":"video","width":160,"height":90,"side_data_list":[{"side_data_type":"DOVI configuration record","rpu_present_flag":1,"bl_present_flag":1}]}]}`, `{"frames":[{"media_type":"video","width":160,"height":90,"side_data_list":[{"side_data_type":"Dolby Vision RPU Data"}]}]}`, `{"frames":[{"media_type":"video","width":160,"height":90,"side_data_list":[{"side_data_type":"Dolby Vision Metadata","bl_bit_depth":10}]}]}`} {
		var d StreamDetail
		analyzeDolbyVisionFrames([]byte(raw), &d)
		if d.DolbyVisionMetadataKnown || d.DolbyVisionRPU || d.DolbyVisionBaseLayer || d.DolbyVisionEnhancementType != "" {
			t.Fatalf("unobserved packet facts: %+v", d)
		}
	}
}
