<div align="center">
  <a href="https://getportico.tv">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/PorticoMediaServer/getportico-tv/main/public/brand/portico-wordmark-white.svg">
      <img src="https://raw.githubusercontent.com/PorticoMediaServer/getportico-tv/main/public/brand/portico-wordmark-black.svg" alt="Portico" width="420">
    </picture>
  </a>

  <h1>Portico Media Server</h1>

  <p><strong>Portico is a free personal media server that turns your media collection into a refined streaming experience across all of your devices.</strong></p>

  <p>
    <a href="https://getportico.tv">Website</a> ·
    <a href="https://github.com/PorticoMediaServer/portico-server/releases">Releases</a> ·
    <a href="https://github.com/PorticoMediaServer/portico-server/issues">Report an issue</a>
  </p>

  <p>
    <a href="LICENSE"><img alt="GPL-3.0-or-later" src="https://img.shields.io/badge/license-GPL--3.0--or--later-59636e?style=flat-square"></a>
    <img alt="Prerelease software" src="https://img.shields.io/badge/status-prerelease-e09f3e?style=flat-square">
  </p>
</div>

---

Portico organizes and streams media you keep on your own storage. The server is
written in Go, uses SQLite, and serves its React web app on the same address.
It is prerelease software: routes and database layouts can change between builds.
Read the release notes and keep a backup before upgrading.

- [Install, run, upgrade and recover](INSTALLING.md)
- [Library layout, naming and matching](docs/libraries.md)
- [API developer guide](server/api/guide/README.md)
- [Build from source](BUILDING.md) and [release packaging](RELEASING.md)
- [Security policy and trade-offs](SECURITY.md)
- [Third-party notices](THIRD-PARTY-NOTICES.md)

## Installation options

The release packaging covers Windows x64/ARM64 (service installer or portable
ZIP), Apple Silicon macOS (PKG, also inside a DMG), Linux x64/ARM64 (DEB, RPM or
portable tar archive), and Linux amd64/arm64 Docker images. Check the
[release page](https://github.com/PorticoMediaServer/portico-server/releases)
for the artifacts available for the build you want. Preview packages are unsigned
at the operating-system level; release manifests have separate signatures.

Open `http://127.0.0.1:32500/` on the server after installation. Complete setup,
choose media folders the service account can read, and add libraries. A Portico
Account is optional for local accounts and direct server sign-in.

## Integrations

Outside apps use the same local API as Portico's own clients. Owners can create
API keys; members can create personal tokens where allowed. The server provides
a bounded title query, reports of changed paths, outbound webhooks, an event feed,
and an MCP endpoint for assistants. These integrations do not require Hosted.
There is no plugin, theme or custom CSS system.

## Source and feedback

Go and JavaScript dependencies and tool versions are described in [BUILDING.md](BUILDING.md).
The headless client core, contracts, design tokens and strings in `packages/`
are shared with the [native clients](https://github.com/PorticoMediaServer/portico-react-native).

Report problems through [GitHub Issues](https://github.com/PorticoMediaServer/portico-server/issues).
External code contributions are not accepted; see [CONTRIBUTING.md](CONTRIBUTING.md).
Report suspected vulnerabilities privately using [SECURITY.md](SECURITY.md).

Portico Media Server is GPL-3.0-or-later; see [LICENSE](LICENSE). Third-party
components retain their own licences. [TRADEMARKS.md](TRADEMARKS.md) explains
use of the Portico name. Developed by [Justin Ehler](https://ehler.ca).
Copyright © 2026 Justin Ehler.
