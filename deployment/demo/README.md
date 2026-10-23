# Demo host: decoder confinement

The demo is Debian 13 (kernel 6.12), unit `portico-demo-current`, user `portico`.

## What the Linux decoder sandbox needs

Every FFmpeg/ffprobe the server runs goes through one executor,
`portico-server/server/internal/mediaexec` (SEC-11, ARCH-MEDIA-09). It sandboxes the
process where the host allows it and otherwise runs it with the baseline
restrictions, and it never turns a feature off for that reason (D-MEDIA-6).
Server › Logs & diagnostics shows which applies and why ("Decoder sandbox").

- **`bwrap` (bubblewrap) on the server's `PATH`.** Without it, jobs run with the
  baseline: input as an open descriptor or a private loopback bridge, protocol
  whitelist, minimal environment, no core dumps, bounded output file size, own
  process group, parent-death signal, and the orphan ledger.
- **Unprivileged user namespaces.** bwrap runs with `--unshare-user --unshare-pid
  --unshare-net --unshare-ipc --unshare-uts`, and the server does not install
  bwrap setuid. So the kernel must allow user namespaces
  (`kernel.unprivileged_userns_clone=1`, `user.max_user_namespaces` > 0), and the
  unit must not block them. The server tries the sandbox for real at startup;
  a refusal is reported with bubblewrap's own message.
- **No helper install.** The server binary is its own helper
  (`--portico-media-exec`, `--portico-decoder-linux`, `--portico-decoder-inner`).
  FFmpeg's exact ELF dependencies are mounted read-only into the sandbox.
- **Owner choice:** `PORTICO_DECODER_SANDBOX=required` makes media jobs fail
  rather than run unsandboxed; `=off` skips the sandbox (baseline only).
- **Unit settings** that would break the sandbox, so leave them unset:
  - `RestrictNamespaces=` (anything other than `no`)
  - `PrivateUsers=yes`
  - a `SystemCallFilter=` that excludes `@mount`, or `unshare`/`clone3`/`pivot_root`

  `NoNewPrivileges=yes`, `ProtectSystem=strict`, `ProtectHome=yes` and
  `PrivateTmp=yes` are all fine.
- **Crash leftovers.** Each media process group is recorded under
  `<state>/run/media-processes/` (PID, start time, boot ID). At startup the
  server kills only groups a previous run recorded and that are provably the
  same processes (BE-MEDIA-06).

## What the demo host has (checked read-only, 23 Sep)

- `bubblewrap 0.12.0-1~deb13u1` at `/usr/bin/bwrap`
- `kernel.unprivileged_userns_clone = 1` and `user.max_user_namespaces = 3644`
- no AppArmor userns restriction
- the unit: `NoNewPrivileges`, `PrivateTmp`, `ProtectSystem=strict`,
  `ProtectHome`, `MemoryMax=1G`, `TasksMax=512`, and no namespace or syscall
  filters
- `PORTICO_FFMPEG` / `PORTICO_FFPROBE` point to the bundled
  `demo-20260919-2d8fa415/bin` builds, which link only libc, libm, libdl, librt,
  libpthread, libmvec and libgcc_s

The server's own probe (`CheckConfinement`: bwrap with the exact probe arguments,
running the static server binary as the inner helper) **exits 0 as `portico`**,
both on its own and inside a transient unit with the demo unit's sandbox
properties.

**So no unit, environment or package change is needed on the demo.** The host
is capable. The "This server platform does not provide the required decoder
confinement" message on live channels came from the server itself (below).

## Live TV on Linux (fixed on be/media)

Live TV and Library Channels no longer depend on the platform: the old
`GOOS != "darwin"` gate and the later sandbox-probe gate are gone
(BE-MEDIA-02). The Linux sandbox proxy forwards PUT for the reserved linear
output path only. Only an owner who sets `PORTICO_DECODER_SANDBOX=required` on a
host without a working sandbox sees `decoder_confinement_unavailable`.

## Current review cycles

Build an exact published server MAIN commit on Netcup; never compile on the demo:

```sh
python3 scripts/build-review.py FULL_MAIN_SHA --repo /path/to/server-checkout --runner /path/to/_program/runner-tools/run-remote.py --output /path/to/cycle-summary-builds --artifacts /home/justin/projects/Portico/.artifacts/release-r40/FULL_MAIN_SHA
scripts/deploy-demo.sh FULL_MAIN_SHA /path/to/artifacts --cycle review-CYCLE --stage-only
```

Staging uses a separate fresh structured data directory and loopback port32504. Complete normal public local-owner setup, create the second profile, and disable transcoding through the owner settings API before scanning. Keep the existing `/srv/portico-demo-20260919` media and synthetic Live TV services. Credentials and the cycle's arm64 embedded-JavaScript testing APK link belong only in `_program/private/REVIEW-SERVER.md`, mode0600; never in this repository or logs.

After setup verification, activate explicitly:

```sh
scripts/deploy-demo.sh FULL_MAIN_SHA /path/to/artifacts --cycle review-CYCLE --activate-only
```

The old service/configuration and old database remain available for rollback; a new binary never opens old state. The new unit uses `PORTICO_SERVICE=1` and explicitly permits its normal `/var/cache/portico` transcode path. Server transcoding stays disabled on this1CPU/1GB review box. A release APK uses the normal Welcome/local-server flow with `https://demo.getportico.tv` typed by the reviewer; no DEV proof entry, injected runtime or embedded credentials.

### Setup and catalogue guards

Complete fresh setup through `POST /v1/setup/browser`, `POST /v1/setup` with local auth, then `GET /v1/setup/state` and `POST /v1/setup/finish` with the returned revision. Create the second profile with `POST /v1/direct/profiles`; profile lists come back in that response. Disable transcoding with the revisioned owner console-settings API and verify `effective.transcodingEnabled` is false. Never carry an old session, server identity, database or installation into the fresh cycle.

Create Movies, TV Shows, Anime, Music and Audiobooks through `POST /v1/libraries`, then scan each with `POST /v1/libraries/{id}/scans`. Follow the returned job through `GET /v1/admin/ingestion/jobs/{jobId}`. Health readiness does not imply catalogue analysis is finished. The approved review view `/srv/portico-review-r40` preserves the original `/srv/portico-demo-20260919` files and contains the retained continuous Infected/Rick/Solo timelines plus four full-duration Rumours FLACs. Other retained clips keep their actual lengths; never substitute a title-runtime manifest for probe results.

Before personal progress or clock comparisons, require positive agreement between `GET /v1/items/{id}/detail` (`item.durationMs`), playback-options versions and parts, and file-info files (`durationMs`). Use the real personal revision for each separate personal-state operation. Current album songs come from `GET /v1/albums/{id}/songs`; their runtime sum and the genuine queue's `totalDurationMs` must agree. A controlled paused API occurrence/history fixture does not establish decoding or visible playback. Frame pairs remain the screen owner's/tester's work.

### Embedded arm64 APK

Use a clean exact native MAIN checkout. Omit `--seed` for the normal exact-lockfile npm install; an optional dependency seed must have its identical native lockfile:

```sh
python3 scripts/build-review-apk.py --repo /path/to/clean-native-main --seed /path/to/matching/apps/native/node_modules --engine /path/to/retained/media-engine/out/android --scratch /home/justin/projects/Portico/.scratch/release-r40 --artifacts /home/justin/projects/Portico/.artifacts/release-r40/native-COMMIT
```

The helper uses the Android slot, two workers/CPU affinity, current-source JNI, arm64 engine verification, notices and embedded-JS/ABI checks. It requires an unused owned dependency path; remove only that checkout's finished disposable dependency/build output after retaining the APK. Signatures are verified before review publication. The retained baseline engine is8.1.2; a source-only8.1.3 or different native MAIN is never an interchangeable payload. Publish the artifact's actual native cohort and retain the previous artifact until the next one is ready.
