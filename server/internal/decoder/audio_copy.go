package decoder

import "errors"

// AudioCopyFilter is used by both the primary HLS stream and alternate audio.
// Core extraction changes the delivered variant, never the compressed core's
// samples. Presence in a requirements file is not runtime qualification.
func AudioCopyFilter(codec string, core bool, facts *ToolchainFacts) ([]string, error) {
	if !core {
		return nil, nil
	}
	if codec != "dts" || facts == nil || !facts.BitstreamFilters["dca_core"] {
		return nil, errors.New("dts_core_extraction_unavailable")
	}
	return []string{"-bsf:a", "dca_core"}, nil
}
