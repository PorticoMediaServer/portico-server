package migrations

import (
	"embed"
	"io/fs"
)

// files is the one canonical release migration bundle. Keeping the embed
// declaration beside the SQL prevents a packaged binary from depending on a
// process working directory or a separately copied migration directory.
//
//go:embed *.sql migration-manifest.json
var files embed.FS

// FS returns the read-only migration bundle embedded in the server binary.
func FS() fs.FS {
	return files
}
