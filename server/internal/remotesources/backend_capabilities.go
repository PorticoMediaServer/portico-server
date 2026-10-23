package remotesources

import (
	"net/netip"
	"strings"

	"portico.local/server/internal/storage"
)

// backendCapabilities records what the configured V1 transport can establish.
// These are internal source facts, never an operation grant, UI promise or
// immutable Version. Observed file identity still comes from fresh metadata.
type backendCapabilities struct {
	Backend                        string
	AdmittedOperationsBounded      bool
	HTTPProxyConfigured            bool
	HTTPSOpaque                    bool
	NativeDirect                   bool
	NativeHostnameCheckConnectGap  bool
	SFTPConnectionPoolBounded      bool
	SMBIdlePoolMayExceedOperations bool
	HiddenRetriesWithinOperation   bool
	ThrottleReportsAsynchronous    bool
	MetadataEqualityProvesBytes    bool
}

func sourceBackendCapabilities(backend, host string) (backendCapabilities, error) {
	facts := backendCapabilities{Backend: backend, AdmittedOperationsBounded: true, HiddenRetriesWithinOperation: true, ThrottleReportsAsynchronous: true}
	switch backend {
	case "sftp", "smb":
		facts.NativeDirect = true
		_, err := netip.ParseAddr(strings.Trim(host, "[]"))
		facts.NativeHostnameCheckConnectGap = err != nil
		facts.SFTPConnectionPoolBounded = backend == "sftp"
		facts.SMBIdlePoolMayExceedOperations = backend == "smb"
	case "s3", "b2", "drive", "onedrive", "dropbox", "webdav", "http":
		facts.HTTPProxyConfigured, facts.HTTPSOpaque = true, true
	default:
		return backendCapabilities{}, storage.ErrRemoteConfig
	}
	return facts, nil
}
