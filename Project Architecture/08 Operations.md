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

All five use one ssh key, `~/.ssh/portico_vps_ed25519`, through the aliases in `~/.ssh/config`. Cloudflare proxies the public web names with origin-pull mTLS.

## Releases

- **Server:** `portico-server/scripts/build-server-release.py` captures the source, runs the tests, builds with `-tags release` and the Hosted root compiled in, and signs a manifest. The FFmpeg bundle is built and verified separately (`build-ffmpeg.sh`, `verify-ffmpeg-bundle.sh`). The Docker image is built from `portico-server/Dockerfile`.
- **Hosted:** `portico-internal/scripts/build-hosted-release.py` captures Hosted and DNS from this repository, plus the web app and shared packages from portico-server, into one snapshot. From it, it builds `hosted`, `web-host`, `portico-dns` and the Hosted web build, and writes a release manifest listing both repositories' commits. `deploy-hosted-release.py` installs it on core-us-1; `deploy-dns-release.py` updates the DNS nodes.
- **Demo:** `portico-internal/scripts/deploy-demo.sh <portico-server commit> [--stage-only|--fresh]` builds the server on the G12 and the web app on the Mac, installs both as `/opt/portico/demo-releases/portico-<sha>`, switches `portico-demo-current`, and waits for readiness. It copies the state aside first, keeps the previous release for rollback, and with `--fresh` starts from empty state. Demo data is disposable.

## Backups

- **Hosted:** the database is dumped by `portico-hosted-backup.service`, and signing keys separately by `portico-hosted-key-backup` (`deployment/hosted/BACKUPS.md`). `scripts/hosted-restore-test.sh` proves a restore.
- **DNS:** zones rebuild from Hosted (`scripts/rebuild-dns-after-hosted-restore.py`, `dns/deploy/RESTORE.md`).
- **Servers** back themselves up as plain SQLite ([Server](02%20Server.md)).

## Testing

- **Go tests run on the runners, not the Mac.** `scripts/runner-test.sh` (in portico-server and portico-internal) syncs the checkout, including uncommitted work, to `~/work/<name>-<branch>` on the first runner with a free slot, and runs every Go module in the repository with a per-slot PostgreSQL database and the production FFmpeg bundle. Run one at a time per checkout, since concurrent runs share the remote copy. Useful flags: `--module`, `--tier default|media|all`, `--jobs`, `--stop`.
- **Tiers:** `default` is the everyday suite. `media` adds real-FFmpeg, Live TV, Library Channels and real-time tests. The performance tier (`PORTICO_PERFORMANCE_TIER`) runs scale proofs on synthetic libraries up to millions of items (`internal/catalogtest`, `catalog/rec_scale_test.go`).
- **Clients** test on the Mac: `npm run check:web` and `npm run test:core` in portico-server; `npx tsc --noEmit` and `npm test` in the Apple app, and `xcodebuild` for the simulator SDKs.
- **Shared copies:** `node scripts/shared-packages.mjs --check` in portico-react-native and `scripts/shared-apikit.sh` in portico-internal.

## Infrastructure names

Some identifiers on the live hosts still carry an earlier project name: on core-us-1, the units `portico-astra-hosted` and `portico-astra-web`, the user `portico-astra-hosted`, `/opt/portico-astra-hosted` and `/etc/portico-astra-hosted`; on the demo, an nginx log name. The deploy scripts and TOPOLOGY.md use the live names so they stay accurate. Renaming them is a planned migration ([Backlog](Backlog.md)).
