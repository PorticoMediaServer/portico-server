package decoder

// PreparedProducible reports whether the conversion code can build this recipe
// today. It asks the same builder a real encode uses, so the answer cannot
// drift from what an encode would do: a recipe the builder learns to make is
// offered from then on, and one it refuses is never offered.
func PreparedProducible(r PreparedRecipe) bool {
	_, err := preparedArgs("http://127.0.0.1/input", r, false)
	if err != nil {
		return false
	}
	video, depth, channels := "h264", 8, 2
	if r.AudioOnly {
		video = ""
	}
	if r.VideoCodec == "hevc" {
		video, depth, channels = "hevc", 10, 6
	}
	return AvailableConversionOutputs(CurrentToolchain()).SupportsConversion(video, depth, "aac", channels)
}

// PreparedToneMapAvailable reports whether an H.264 recipe can tone-map an HDR
// source on the processor. Without it such a source is refused rather than
// written as a flat, grey standard picture.
func PreparedToneMapAvailable() bool {
	f := CurrentToolchain()
	return f != nil && f.Filters["zscale"] && f.Filters["tonemap"]
}

// PreparedDolbyVisionBaseAvailable reports whether the units a Dolby Vision
// source carries beside its base layer can be removed from a prepared HEVC
// stream.
func PreparedDolbyVisionBaseAvailable() bool {
	f := CurrentToolchain()
	return f != nil && f.BitstreamFilters["filter_units"]
}
