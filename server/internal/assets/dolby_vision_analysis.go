package assets

import "encoding/json"

// analyzeDolbyVisionFrames consumes actual decoded-frame side data. A container
// configuration's rpu_present_flag describes its declaration, not an observed
// RPU; only the decoder's parsed metadata establishes these packet facts.
func analyzeDolbyVisionFrames(raw []byte, d *StreamDetail) {
	var doc struct {
		Frames []struct {
			MediaType string `json:"media_type"`
			Width     int    `json:"width"`
			Height    int    `json:"height"`
			Side      []struct {
				Type            string `json:"side_data_type"`
				RPUType         *int   `json:"rpu_type"`
				BaseDepth       *int   `json:"bl_bit_depth"`
				DisableResidual *int   `json:"disable_residual_flag"`
			} `json:"side_data_list"`
		} `json:"frames"`
	}
	if json.Unmarshal(raw, &doc) != nil || len(doc.Frames) == 0 || len(doc.Frames) > 240 {
		return
	}
	for _, frame := range doc.Frames {
		if frame.MediaType != "video" || frame.Width <= 0 || frame.Width > 32768 || frame.Height <= 0 || frame.Height > 32768 {
			continue
		}
		for _, side := range frame.Side {
			if side.Type != "Dolby Vision Metadata" || side.RPUType == nil || *side.RPUType != 2 || side.BaseDepth == nil || *side.BaseDepth < 8 || *side.BaseDepth > 16 {
				continue
			}
			d.DolbyVisionMetadataKnown = true
			d.DolbyVisionRPU, d.DolbyVisionBaseLayer = true, true
			// The residual flag is parsed from the actual RPU. It is not
			// inferred from profile 7, a neighbouring PID, or a filename.
			if d.DolbyVisionProfile == 7 && d.EnhancementLayer && side.DisableResidual != nil {
				switch *side.DisableResidual {
				case 0:
					d.DolbyVisionEnhancementType = "FEL"
				case 1:
					d.DolbyVisionEnhancementType = "MEL"
				}
			}
			// ffprobe's frame metadata does not expose the complete CM
			// extension/version fact. Leave it unknown rather than infer it.
		}
	}
}
