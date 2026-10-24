package preparedmedia

import "portico.local/server/internal/audiotags"

// Keep WavPack's observed header mode with the very same retained source
// passed to the encoder. A codec name or a text tag cannot establish it.
func musicSourceFacts(original probed, tags audiotags.Tags) probed {
	if original.AudioCodec == "wavpack" {
		original.AudioLossless = tags.Lossless
	}
	return original
}
