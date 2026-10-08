// Package receiver embeds the static web bundles the server serves directly.
//
// The Cast receiver application lives under cast/. It is plain HTML and
// JavaScript with no build step, compiled into the binary so every GOOS target
// and the Docker image serve identical bytes and no deployment has to publish
// the page separately.
package receiver

import (
	"embed"
	"io/fs"
)

//go:embed cast
var bundles embed.FS

// Files is the filesystem mounted at /receiver/.
func Files() fs.FS { return bundles }
