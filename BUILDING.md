# Build Portico Media Server

This guide is for people who want to inspect or run the source. Most users
should install a package from the [latest release](https://github.com/PorticoMediaServer/portico-server/releases/latest).

## Requirements

- Go at the version declared in `go.mod`
- Node.js 24 and npm
- Make
- FFmpeg and FFprobe for playback and transcoding tests

Clone the repository, install the two JavaScript dependency trees, and run the
main verification commands:

```sh
npm --prefix packages/portico-client-core ci
npm --prefix web ci
make test
make build-server
```

The server binary is written to `dist/porticod` unless `SERVER_OUTPUT` is set.
During development, `make dev-api` starts the server and `make dev-web` starts
the browser interface.

The public application CI also cross-compiles Go for Linux x64/ARM64, Windows
x64/ARM64, and Apple Silicon macOS. Installer creation belongs to the release
workflow because it requires platform-specific tooling and an approved FFmpeg
component release.

## Private services are not required

The repository contains the public Hosted protocol documents needed to compile
and test clients. Portico's Hosted Services implementation, operator tooling,
credentials, and internal planning are deliberately not part of this source
tree and are not required to build Portico Media Server.
