# Persistent walk server (Contabo)

Source-only coordinator for the walk server in `WALKS.md`. It tracks published
`portico-server` main, builds it on `portico-runner`, prepares a fresh
disposable state through the ordinary local setup APIs, runs one private
loopback listener under the normal service profile, keeps the previous
build/state for rollback, and reports a walk server as usable only after the
owner has verified the private entry.

This directory does not install, deploy, start a service, create a timer or
touch the running Contabo M0 runtime. The owner runs a one-shot `deploy` over
encrypted SSH with the local credential file on stdin; the server process
persists, and the owner's cadence re-runs `deploy` every 1-2 hours.

## Host and access plan

- Listener: `127.0.0.1:32510` (owned walk port; not the stable demo/stage port).
- Ubuntu browsers on mini-1: an encrypted SSH local forward from mini-1 to
  Contabo, `ssh -N -L 49300:127.0.0.1:32510 portico-runner`; the actual origin
  is `http://localhost:49300`.
- Android walkers: `adb reverse tcp:49300 tcp:49300` on mini-1 maps the exact
  device's loopback to the same tunnel endpoint. No auth/state injection.
- Apple/Mac: the owner's own SSH local forward to the same
  `127.0.0.1:32510`, using the same `http://localhost:49300` origin.
- The server never binds `0.0.0.0`, `::`, a public address or a DNS name. There
  is no proxy, public tunnel, DNS change, or HTTPS/Origin/proof-guard bypass.
  The tunnel preserves the browser Host/Origin, so the existing first-party
  origin checks remain in force.

## Credentials: stdin only

The sole credential file is the owner's local:

```
/home/justin/projects/Portico/.private/walks/credentials.json   (0600)
```

The owner delivers it over encrypted SSH stdin to a one-shot `deploy`:

```sh
ssh portico-runner \
  'python3 /home/runner/work/portico-internal/deployment/testing/walks/server.py deploy \
     --repo /home/runner/work/portico-server \
     --fixtures /home/runner/work/fixtures/portico-v3 \
     --work-root /home/runner/work/portico-v3-walks \
     --port 32510 --access-address http://localhost:49300' \
  < /home/justin/projects/Portico/.private/walks/credentials.json
```

The coordinator reads the inherited stdin FD, keeps the values in memory only
for that setup, and never creates, writes or retains a credential file on the
Contabo host. There is no credential CLI argument, no default credential
creation and no remote credential file. Transfer uses the encrypted SSH
channel; the local file stays 0600 and never enters Git or output.

```json
{
  "owner": {"username": "walkowner", "password": "..."},
  "members": [
    {"username": "walka", "password": "...", "name": "Walk A"},
    {"username": "walkb", "password": "...", "name": "Walk B"}
  ]
}
```

Owner and member accounts are created only through the published ordinary APIs:
`POST /v1/setup`, `POST /v1/setup/finish`, `POST /v1/direct/members` and
`POST /v1/direct/sign-in`. There is no SQL seeding, auth/session/state injection
or Root §56 pending-grant/approval revival. Passwords are sent only in those API
bodies over the loopback client; setup errors are closed codes.

Libraries are created first, then every listed member is created as an ordinary
full-library walk account with the three actual library IDs. At least one (in
fact each) member is signed in and verified against `GET /v1/libraries`, the
Movies content and the Books reading items before the state is accepted. No
restricted account is created here; if the owner wants one, it is a separate
account, never one of these walk members.

The service user on the host must own and be able to read the work root, the
fixture tree, the server checkout and the owner-written entry marker. The
credential file is not present on the host.

## Service profile and process confinement

The server uses the normal `PORTICO_SERVICE=1` profile, not a development-mode
substitute. It runs under bwrap with:

- the work root bound read-write;
- `/var/cache` as a private tmpfs and `/var/cache/portico` bound to the owned
  state `transcode/` directory, matching the M0 confinement;
- the fixture tree and the rest of the host read-only;
- explicit `XDG_CACHE_HOME`, `TMPDIR`, `PORTICO_DATA_DIR`, `PORTICO_WEB_DIR`,
  `PORTICO_FFMPEG` and `PORTICO_FFPROBE`;
- **no `HOME` override**;
- no public bind.

The launcher may be a bwrap parent. The coordinator resolves the actual server
child by executable path, start ticks and loopback listener ownership, records
that actual PID/start/executable/pgid plus the launcher PID/start/pgid in the
active receipt, and stops both owned process groups on rotation/rollback.
Missing or ambiguous identity is refused, never guessed.

## Fixtures and build

The coordinator accepts one fixture root (`--fixtures`, default
`/home/runner/work/fixtures/portico-v3`) containing the real `media/` tree and
the qualified `runtime/bundle/` with `bin/ffmpeg`, `bin/ffprobe` and
`toolchain-manifest.json`. The owner creates the verified manifest once:

```sh
python3 deployment/testing/walks/server.py fixtures manifest --fixtures /home/runner/work/fixtures/portico-v3
```

The manifest records the media tree hash, runtime tree hash and the pinned EPUB
hash. Each cycle rechecks the small runtime bundle, the EPUB and the manifest;
a missing or mismatched manifest is refused (`fixtures-unverified`), never
treated as verified.

`build.load` re-hashes the actual server binary, web tree and generation patch
against the stored manifest before any reused activation or rollback. A hash
mismatch refuses the build (`build-hash-mismatch` or
`generation-patch-mismatch`); existence alone is not trusted.

For each published `origin/main` commit, the coordinator extracts the exact
commit with `git archive`, runs the official API/schema generators, and builds
the release-tagged server with the official Hosted root and the exact commit
build ID, plus `portico-web-next`. Source declarations may legitimately precede
an official generated batch: a non-empty overlay is allowed **only** when it
comes from those official commands. The exact unified patch, its SHA256 and the
changed-file count are retained in the immutable build receipt
(`generatedClean`, `generatedFiles`, `generationPatchSHA256`,
`generationSource`); generator errors still refuse, and no hand edit is
accepted. The build uses that captured same-tree overlay.

Build limits are the existing runner limits (`GOMAXPROCS=2`,
`GOMEMLIMIT=1400MiB`, `go build -p=2`,
`NODE_OPTIONS=--max-old-space-size=1536`). Every build/npm/generation step runs
in its own process group and a timeout kills that group, not just the direct
child. Build output stays in a private log.

## Fresh state and rollback

A rotation first compares the active build and active state schema digest with
the new build's exact schema digest, and the active state's fixture/runtime
manifest with the current fixture. If both match, it reuses the active state:
no accounts, sessions or libraries are recreated, and walkers keep their
sessions. If the digest changed, the walk credentials changed, or the active
build/state is missing, it prepares a fresh state under `/home/runner/work/portico-v3-walks/states/` on a
staging loopback port: owner/member accounts, conversion mode, Movies/Music/
Books libraries, scans, Home/reading checks and member browse verification;
then it stops the staging server. The first deployment is always fresh and has
no retirement notice.

A fresh rotation explicitly reports identity/session retirement in
`SERVER.md`/`status.json` (`identityRetired`, `sessionsRetired`, `continuity`,
`notice`); walkers sign in again with the walk credentials. Identity is never
copied into an incompatible schema and pins are never bypassed. The prepared
state is activated on `127.0.0.1:32510`; the previous build/state is kept for
rollback. No active state is mutated in place.

`server.py rollback` restores the previous build and state. A failed activation
restarts the previous runtime. The last three builds and two states are
retained; older build directories are archived as non-usable handoff manifests
before deletion. A stopped runtime is always `usable: false`.

## Handoff and the private-entry marker

A running API server is not by itself a usable walk entry. The owner writes the
private marker only after the real tunnel/entry works:

```
/home/runner/work/portico-v3-walks/entry.json   (0600)
{"address": "http://localhost:49300", "verifiedAt": "2026-10-26T...Z", "verifiedBy": "Release"}
```

`SERVER.md`/`status.json` then reports `entryVerified: true` and `usable: true`.
Without a matching marker the status says `usable: false` with
`private entry not verified`. A stopped runtime is always `usable: false`; the
archive manifest says `stopped ephemeral walk checkpoint; not a usable server`.
The status records the listener, address, commit, state, continuity, identity/
session retirement, entry verification and the owner's local credential-file
location. The credential values are never in the status or archive.

## Owner cadence hook (proposal)

Run the one-shot deploy from mini-1 on a 1-2 hour owner timer, with the local
0600 file as SSH stdin. For example:

```ini
[Unit]
Description=Deploy the Portico walk server
After=network-online.target

[Service]
Type=oneshot
User=justin
ExecStart=/bin/bash -lc 'ssh -o BatchMode=yes portico-runner "python3 /home/runner/work/portico-internal/deployment/testing/walks/server.py deploy --repo /home/runner/work/portico-server --fixtures /home/runner/work/fixtures/portico-v3 --work-root /home/runner/work/portico-v3-walks --port 32510 --access-address http://localhost:49300 --server-md /home/runner/work/portico-v3-walks/SERVER.md" < /home/justin/projects/Portico/.private/walks/credentials.json'
```

A timer unit on mini-1 can run it every 90 minutes. The coordinator itself
creates no timer. The SSH tunnel is owner-managed separately; the deploy
command uses the same encrypted SSH access. Do not add a Contabo systemd unit
that stores the credential file: the design deliberately has no remote
persistent credentials.

Wait for the existing Contabo M0 runtime to be cleaned up before installing or
running any of this. The owner creates the fixture manifest, source checkout
and entry marker; the coordinator creates only its own 0700 work directories.

## Commands

```sh
server.py deploy     # one build/state rotation; credentials on stdin; server persists
server.py status     # closed status JSON, starts nothing
server.py rollback   # restore the previous build/state
server.py fixtures manifest|verify
server.py entry check
server.py archive list
```

`server.py` loads the `server/` helpers as the `walks_server` package, avoiding
the `server.py`/`server/` module-name collision.

## Current state

- Foundation source only; no install, deploy, build, service or timer was run.
- Existing Contabo M0 remains untouched and running until the owner cleans it up.
- Guard/deploy changes remain for the owner's read before any host action.
