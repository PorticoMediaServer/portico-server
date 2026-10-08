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

This is the primary repository for Portico. Visit [getportico.tv](https://getportico.tv) for downloads, documentation, and project news.

Portico Media Server organizes and streams the movies, television, music, and other media you already own. The project is being developed in the open and is not yet considered ready for everyday production use.

## Project status

Portico is prerelease software. Builds may be incomplete, upgrades may require extra care, and some features are still changing. Please read the release notes before installing or updating.

## Downloads

The newest stable release will always be available from the unchanging [latest release](https://github.com/PorticoMediaServer/portico-server/releases/latest) page. Prerelease and development builds appear on the [complete releases page](https://github.com/PorticoMediaServer/portico-server/releases).

Planned packages include:

| Platform | Architectures | Packages |
| --- | --- | --- |
| Windows | x64, ARM64 | Installer (`.exe`) and portable archive |
| macOS | Apple Silicon | Application disk image (`.dmg`) |
| Debian and Ubuntu | x64, ARM64 | `.deb` and distribution-neutral `.tar.gz` archive |
| Fedora and related systems | x64, ARM64 | `.rpm` and distribution-neutral `.tar.gz` archive |
| Docker | amd64, arm64 | Container image from GitHub Container Registry |

Unsigned packages are suitable for early testing. Platform signing and notarization will be introduced before unattended updates are enabled.

## Technology

| Area | Technology |
| --- | --- |
| Server | Go |
| Database | SQLite |
| Web application | React, TypeScript, Vite |
| Media processing | FFmpeg and FFprobe |
| API contract | OpenAPI |
| Containers | Docker / OCI |
| Automation | GitHub Actions |

## Building locally

You need Go, Node.js, npm, and Make. FFmpeg is optional for compilation but required for real playback and transcoding.

```sh
make test
make build-server
```

See [BUILDING.md](BUILDING.md) for supported tool versions and complete instructions. Installation and release packaging are described in [INSTALLING.md](INSTALLING.md) and [RELEASING.md](RELEASING.md).

## Related repositories

- [Portico React Native clients](https://github.com/PorticoMediaServer/portico-react-native)
- [Portico for Roku](https://github.com/PorticoMediaServer/portico-roku)
- [getportico.tv website](https://github.com/PorticoMediaServer/getportico-tv)

## Feedback and contributions

Bug reports, feature requests, and thoughtful feedback are welcome through [GitHub Issues](https://github.com/PorticoMediaServer/portico-server/issues).

Portico does not accept external code contributions. Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening an issue.

Security problems should be reported privately as described in [SECURITY.md](SECURITY.md).

## License and trademarks

Portico Media Server is licensed under `GPL-3.0-or-later`. See [LICENSE](LICENSE). Third-party components remain under their respective licenses.

The GPL does not grant permission to present modified or redistributed builds as official Portico releases. See [TRADEMARKS.md](TRADEMARKS.md).

---

Made with ❤️ in Nova Scotia, Canada | Developed by [Justin Ehler](https://ehler.ca)  
Copyright © 2026 Justin Ehler  
Portico Media Server is free software licensed under the GNU General  
Public License, version 3 or, at your option, any later version.
