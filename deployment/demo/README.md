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
