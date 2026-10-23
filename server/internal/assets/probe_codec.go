package assets

// canonicalProbeCodec translates actual FFmpeg decoder names to the codec keys
// used by the planner. It does not infer a codec from a suffix or container.
func canonicalProbeCodec(codec string) string {
	switch codec {
	case "musepack7":
		return "mpc7"
	case "musepack8":
		return "mpc8"
	default:
		return codec
	}
}
