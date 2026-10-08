# Build Portico Media Server

This guide is for people who want to inspect or run the source. Most users
should install a package from the [latest release](https://github.com/PorticoMediaServer/portico-server/releases/latest).

## Requirements

- Go at the version declared in `server/go.mod`
- Node.js 24 and npm
- Make
- FFmpeg and FFprobe for playback and transcoding tests

Clone the repository, install the JavaScript dependencies, and run the main
verification commands:

```sh
npm ci
make test
make build-server
```

The server binary is written to `dist/portico-server` unless `SERVER_OUTPUT` is
set. During development, `make dev-api` starts the server and `make dev-web`
starts the browser interface.

## Repository layout

| Path | Contents |
| --- | --- |
| `server` | The server (Go module `portico.local/server`); API reference in `server/api` |
| `web` | The web application every server serves |
| `packages/client-core` | The headless client core shared by the web and native apps |
| `packages/design`, `packages/i18n` | Design tokens and component specification; strings and formatting |
| `packages/contracts` | TypeScript contract generated from the server's API registry |
| `apikit` | The Go API toolkit (routes, schemas, cursors, idempotency) |
| `fixtures` | Test fixtures |
| `third_party` | The qualified FFmpeg build recipe and the patched hls.js |
| `trust` | The Hosted root public key compiled into release builds |
| `packaging` | Linux, macOS and Windows package definitions used by releases |
| `scripts` | Build, release and test scripts |

The shared client packages here are the source of truth. The
[React Native clients](https://github.com/PorticoMediaServer/portico-react-native)
keep an identical copy, and the [Roku client](https://github.com/PorticoMediaServer/portico-roku)
reads them from a sibling checkout of this repository.

Release builds cover Linux x64/ARM64, Windows x64/ARM64, and Apple Silicon
macOS. Installer creation belongs to the release workflow because it requires
platform-specific tooling and an approved FFmpeg component release.

## Private services are not required

The repository contains the public Hosted protocol documents needed to compile
and test clients. Portico's Hosted Services implementation, operator tooling,
credentials, and internal planning are deliberately not part of this source
tree and are not required to build Portico Media Server.
