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

## Network access

By default the server listens on port 32500 for IPv4 and IPv6 where the host
supports dual-stack sockets. `PORTICO_BIND=:32500` selects this default explicitly;
use an IP and port to restrict the listener. HTTP and HTTPS share the port.
Plain HTTP works from any address, including overlay networks and carrier-grade
NAT. HTTPS is recommended when traffic crosses an untrusted network.

The owner's **Home networks** (`lanNetworks`) list is the only definition of
local access. Defaults include loopback, private IPv4, link-local, unique-local
IPv6 and directly attached subnets. Replace the list to add or remove ranges;
`[]` makes every request remote, including loopback. This controls recovery
access, remote sign-in and streaming limits. No overlay or proxy is detected.

For a reverse proxy on its own hostname, list its socket address or range in
**Trusted proxies** (`trustedProxies`). Preserve `Host`, or set
`X-Forwarded-Host`; set `X-Forwarded-Proto` and append the real source to
`X-Forwarded-For`. RFC `Forwarded` is also supported. Configure the proxy to
replace client-supplied forwarding headers. Portico ignores these headers from
other peers and walks trusted proxy chains from right to left. The bundled web
app accepts writes whose origin matches the external request origin. No origin
allow-list variable is needed. Hosting below a URL path is unsupported.

In Docker bridge, rootless port publishing or a TCP tunnel, the source address
may be the gateway for every client. If forwarding headers are available, trust
the HTTP proxy's exact address and pass the client address. Otherwise remove the
hidden gateway's range from Home networks, or use an empty list to treat everyone
as remote. The connectivity status report shows the client address Portico used;
it cannot recover an address that the transport did not send.

**External address** (`advertiseUrl`) adds an HTTP or HTTPS origin clients can
use, such as `https://media.example.com`. No path, query or credentials are
accepted. `accessUrls` adds further origins. Local clients read them at
`GET /v1/networking/connection`; only HTTPS origins are published to Hosted.
Every client still verifies the server's pinned identity before sending secrets.

For the server's own TLS listener, set `customCertificatePath`,
`customCertificateKeyPath` and `customCertificateDomain` together. Use a PEM
certificate chain and unencrypted PEM private key readable by the server's user.
Hosted is optional. Changes reload within 30 seconds of the next handshake or
status request; a broken replacement retains the last valid certificate until
it expires. Status appears in `GET /v1/admin/connectivity/status`.

**Connection limit** (`connectionLimit`) accepts 0 for the memory-based default,
or a number up to 65536. Restart after changing it. The console and sign-in have
32 reserved connections beyond the ordinary limit. Request lanes, header
deadlines and HTTP/2 stream limits still apply. Capacity is not a tested viewer
count; increase it according to the machine and workload.

First-run setup needs no local code. Browser setup writes require JSON and the
same request origin; cross-site forms and cross-site preflights cannot claim the
server. Native API clients may omit Origin but must send `application/json`.
