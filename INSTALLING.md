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
`/var/lib/portico-media-server`, Windows installs use
`%ProgramData%\Portico Media Server`, and the macOS application uses
`~/Library/Application Support/Portico Media Server`.

## Linux package paths and permissions

The packaged systemd service runs as `portico` and keeps the filesystem
read-only except for its state folder. When enabling media deletion or using a
separate recordings folder, grant access only to the directories you selected.
Create those directories first and give `portico` the required filesystem
permissions. Then run `sudo systemctl edit portico-media-server` and add, for
example:

```ini
[Service]
Environment=PORTICO_RECORDINGS_DIR=/srv/recordings
ReadWritePaths=/srv/media/Movies /srv/recordings
```

Replace the example paths with your actual library and recording directories;
quote paths containing spaces. The added paths also work beneath `/home`,
despite the default `ProtectHome=read-only`. This grants write access within the
service sandbox; it does not grant Unix ownership or ACL permissions. Keep
`ProtectSystem=strict`, `ProtectHome=read-only`, `NoNewPrivileges=true` and
`PrivateTmp=true`. Apply the drop-in with
`sudo systemctl daemon-reload && sudo systemctl restart portico-media-server`.

Optional environment settings can instead be stored in the root-owned
`/etc/portico-media-server/environment` file (one `NAME=value` per line).
A custom `PORTICO_STATE_DIR` also needs its own additive `ReadWritePaths` entry.
Selecting a path in the console or setting an environment variable alone does
not grant it sandbox write access.

Custom TLS certificate files only need read access. Copy the certificate and
private key into a private directory inside the state folder, or grant
`portico` read access to the selected files and traversal of their parent
directories using host ACLs. Keep the private key inaccessible to other users;
do not make the whole Let’s Encrypt directory writable. Configure the actual
certificate/key paths in the console. Certificate renewal must preserve that
read access.
