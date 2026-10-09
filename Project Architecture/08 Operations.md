# Operations

Everything Portico runs is a systemd unit on a small VPS: static Go binaries, plus OS packages for Caddy, PostgreSQL, BIND and dnsdist. There's no Docker in production (the Docker image is for owners who want it), and no high availability yet; that gets revisited after release. There is no production user data yet.

The full host inventory, ports, units, firewall and directories are in `deployment/hosted/TOPOLOGY.md`. Operator credentials are in `Deployment Information/` (the secret parts are ignored by git).

## Hosts

| Host (ssh alias) | Role |
|---|---|
| `portico-core-us-1` (Netcup G12) | Hosted API, Hosted web host, PostgreSQL 17, Caddy for web., getportico.tv and cast.; also the second test runner (`portico-runner-g12`) |
| `portico-dns-primary-1` (ns1) and `portico-dns-de-1` (ns2) | dnsdist, portico-dns and BIND; WireGuard mesh with core-us-1 for ACME updates |
| `portico-demo` | The demo server, demo.getportico.tv |
| `portico-runner` (Contabo) | Build and test runner |

Use the aliases in `~/.ssh/config`. On the personal Ubuntu host, `portico-demo` resolves to the configured VPS and uses `~/.ssh/portico-vps-ed25519`; other host credentials remain as recorded in the topology and operator records. Cloudflare proxies the public web names with origin-pull mTLS.

## Releases

- **Server:** `portico-server/scripts/build-server-release.py` captures the source, runs the tests, builds with `-tags release` and the Hosted root compiled in, and signs a manifest. The FFmpeg bundle is built and verified separately (`build-ffmpeg.sh`, `verify-ffmpeg-bundle.sh`). The Docker image is built from `portico-server/Dockerfile`.
- **Hosted:** `portico-internal/scripts/build-hosted-release.py` captures Hosted and DNS from this repository, plus the web app and shared packages from portico-server, into one snapshot. From it, it builds `hosted`, `web-host`, `portico-dns` and the Hosted web build, and writes a release manifest listing both repositories' commits. `deploy-hosted-release.py` installs it on core-us-1; `deploy-dns-release.py` updates the DNS nodes.
- **Demo:** `portico-internal/scripts/deploy-demo.sh <portico-server commit> [--stage-only|--activate-only]` builds a committed server/web snapshot on the personal Ubuntu host, installs both as `/opt/portico/demo-releases/portico-<sha>`, switches `portico-demo-current`, and waits for readiness. It backs up the full stopped state and configuration first and restores them if activation fails. Preserve the populated demo library, media, old releases and backups. See [interactive testing](../deployment/testing/README.md) for the active release, credentials location and rollback record.

## Backups

- **Hosted:** the database is dumped by `portico-hosted-backup.service`, and signing keys separately by `portico-hosted-key-backup` (`deployment/hosted/BACKUPS.md`). `scripts/hosted-restore-test.sh` proves a restore.
- **DNS:** zones rebuild from Hosted (`scripts/rebuild-dns-after-hosted-restore.py`, `dns/deploy/RESTORE.md`).
- **Servers** back themselves up as plain SQLite ([Server](02%20Server.md)).

## Testing

- **Go tests and performance benchmarks run on the personal Ubuntu host.** The local media tools and full server suite were exercised during the October setup. `scripts/runner-test.sh` remains available for optional remote runner work, including per-slot PostgreSQL and the qualified FFmpeg bundle; verify its host access and topology before using it. It syncs uncommitted work into a shared per-checkout remote copy, so run one instance at a time per checkout. Useful flags: `--module`, `--tier default|media|all`, `--jobs`, `--stop`.
- **Tiers:** `default` is the everyday suite. `media` adds real-FFmpeg, Live TV, Library Channels and real-time tests. The performance tier (`PORTICO_PERFORMANCE_TIER`) runs scale proofs on synthetic libraries up to millions of items (`internal/catalogtest`, `catalog/rec_scale_test.go`).
- **Clients:** web/core checks and Android, Fire OS and Vega builds run on the personal Ubuntu host. `scripts/test-clients.sh` builds release-mode testing packages and starts/installs visible devices. The demo VPS serves the shared populated library. Apple `xcodebuild` and Apple simulator/device work remain on the Mac. See [the current runbook](../deployment/testing/README.md); emulator results do not establish visual parity or physical-device acceptance.
- **Shared copies:** `node scripts/shared-packages.mjs --check` in portico-react-native and `scripts/shared-apikit.sh` in portico-internal.

## Infrastructure names

Some identifiers on the live hosts still carry an earlier project name: on core-us-1, the units `portico-astra-hosted` and `portico-astra-web`, the user `portico-astra-hosted`, `/opt/portico-astra-hosted` and `/etc/portico-astra-hosted`; on the demo, an nginx log name. The deploy scripts and TOPOLOGY.md use the live names so they stay accurate. Renaming them is a planned migration ([Backlog](Backlog.md)).
