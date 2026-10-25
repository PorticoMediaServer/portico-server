//go:build !linux

package telemetry

// memoryTranscodeDirectory offers no memory conversion directory on this
// platform: it has no mounted RAM file system to name, so a client leaves the
// choice out instead of drawing one the server cannot honour.
func memoryTranscodeDirectory() string { return "" }
