//go:build linux

package telemetry

// memoryTranscodeDirectory names the RAM file system a conversion directory can
// use when the host has one this server can write to. Only a real mounted
// RAM-backed file system counts: /dev/shm is the standard one, and a host that
// does not mount it gets no memory choice rather than one that would fill RAM.
func memoryTranscodeDirectory() string {
	const sharedMemory = "/dev/shm"
	if !writable(sharedMemory) {
		return ""
	}
	return sharedMemory
}
