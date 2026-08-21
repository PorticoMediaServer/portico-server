# Install Portico Media Server

Portico is prerelease software. Back up your Portico data and media metadata
before trying an early build, and read the notes on the release you install.

The permanent download page is:

<https://github.com/PorticoMediaServer/portico-server/releases/latest>

Each development release provides unsigned packages for Windows x64/ARM64,
Apple Silicon macOS, Linux x64/ARM64, and a multi-architecture Docker image.

- Windows: use the `Setup.exe` installer or portable ZIP.
- macOS: open the DMG and copy **Portico Media Server** to Applications.
- Debian/Ubuntu: install the matching `.deb` package.
- Fedora/RHEL-family systems: install the matching `.rpm` package.
- Other Linux systems: unpack the matching `.tar.gz` archive.
- Docker: pull `ghcr.io/porticomediaserver/portico-server:latest`.

Because packages are not yet signed, operating systems may show an unknown
publisher warning. Download only from the official GitHub organization and
compare the file with `checksums.sha256` on the release page. Do not enable
unattended installation of these preview builds.

Portico stores server data separately from the application. Linux packages use
`/var/lib/portico-media-server`, Windows service installs use
`%ProgramData%\Portico Media Server`, and the macOS application uses
`~/Library/Application Support/Portico Media Server`.
