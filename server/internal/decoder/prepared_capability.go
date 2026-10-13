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
	video := "h264"
	if r.AudioOnly {
		video = ""
	}
	return AvailableConversionOutputs(CurrentToolchain()).SupportsConversion(video, 8, "aac", 2)
}
