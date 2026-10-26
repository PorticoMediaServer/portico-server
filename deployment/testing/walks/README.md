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


---

# Walk tooling: one frame, measured

Reserved tools for WALKS.md §1.3 (frame renderer) and §1.4 (web style extractor).
They read the approved mock-ups, never write to them, and never touch an app.

The mock-up HTML/CSS is the truth (WALKS §2b). These tools make its expected
values citable: file, frame label, element selector, and the computed values
next to the app's actual values.

## What is here

| Path | Purpose |
|---|---|
| `frame.mjs` | Render one exact frame to PNG plus the element/style/text/order JSON artifact. |
| `web-style.mjs` | Pair mock-up elements with app elements and report measurable differences. |
| `web/` | Helpers: label resolution, frame preparation, bounded extraction, comparison, paths. |

Nothing here is imported by the app or by the checkpoint runner. No design file,
font, screenshot or artwork is copied into a repository. Artifacts are written
by `walks/render-frame` into `v3/walks/frame-artifacts/` (outside every repo) and
mode 0600.

## Requirements

- Node 22 or newer.
- An existing Playwright install, by reference: `PORTICO_WALKS_PLAYWRIGHT=/path/to/node_modules/playwright/index.mjs`.
- Chromium: `PLAYWRIGHT_CHROMIUM_EXECUTABLE_PATH=/usr/bin/chromium` (or Playwright's own).
- Manrope: the product's variable face is used by default from
  `PORTICO_SERVER_REPO` (fallback `/home/justin/projects/Portico/apps/portico-server`)
  or `PORTICO_WALKS_MANROPE`. If no local face loads, the tool fails
  `manrope-unavailable`; it never measures fallback type.
- The design root defaults to
  `/home/justin/projects/Portico/.worktrees/design-ui-v1/design/ui-v1`
  (`PORTICO_WALKS_DESIGN` overrides it).

## Render one frame

```sh
walks/render-frame auth.html 4 --width 1440 --height 900 --surface web-desktop \
  --data '.rchip,.in,.dots' --identifier '.rchip=profile-chip'
```

writes `v3/walks/frame-artifacts/auth-4.png` and `auth-4.json`. The PNG is the
frame at its authored 1:1 design width (1440 web rows, 1920 TV screens, the
tablet stage's own width, a phone row's 390-wide phones as drawn). `--width` and
`--height` are the device the frame is compared against; they are recorded as
`target`. `device` is always the size actually rendered, so a comparer can refuse
a mismatch instead of guessing.

For a frame that draws several devices (a phone row of three, an iPad pair), pass
one exact device element and the renderer captures just it, with boxes relative
to it:

```sh
walks/render-frame phone-guide.html g1 --element '.col:nth-child(1) .phone' \
  --width 390 --height 844 --units points --surface ios-phone \
  --data '.card,.t,.s'
```

`--units` is `pixels` (web CSS px, Android) or `points` (iOS, tvOS); it defaults
to `pixels`. The artifact name gains a short element hash so several devices in
one frame do not overwrite each other.

Frame labels follow the mock-ups' own conventions, in this order:

1. the frame's `data-f` gallery value (`4`, `24b`, `1p`);
2. its own or its `.screen` id, with or without a short letter prefix (`f7`, `g1`, `7`);
3. its 1-based position in the page.

An unknown or ambiguous label fails with `label-unknown` or `label-ambiguous`
and the available labels; the tool never guesses.

## Shared measurement artifact

The renderer is the only writer of this format. Both the web extractor and the
native layout comparer read it; no second writer exists.

```json
{
  "tool": "render-frame",
  "surface": "web-desktop",
  "source": {"file": "auth.html", "label": "4"},
  "units": "pixels",
  "device": {"width": 1440, "height": 900},
  "target": {"width": 1440, "height": 900},
  "dataMapping": "explicit",
  "elements": [
    {
      "selector": ".au h1",
      "label": "Portico demo",
      "role": "heading",
      "box": [64, 218, 440, 63],
      "style": {
        "fontFamily": "Manrope", "fontSize": "64px", "fontWeight": "800",
        "lineHeight": "63px", "letterSpacing": "-2.24px",
        "color": "rgb(237, 245, 250)", "backgroundColor": "rgba(0, 0, 0, 0)",
        "textAlign": "start", "borderRadius": "0px",
        "padding": [0, 0, 0, 0], "margin": [0, 0, 0, 0], "gap": "normal",
        "opacity": 1, "display": "block",
        "normalizedRGB": {"color": [237, 245, 250, 1], "backgroundColor": [0, 0, 0, 0]},
        "backgroundImage": "none"
      },
      "textMode": "static"
    }
  ]
}
```

- `source.file` is relative to the design root; `source.label` is the label that
  selected the frame. Every element carries its exact `selector` (a unique short
  selector, or the nth-child path when no short selector is unique).
- `box` is `[x, y, width, height]`, relative to the captured frame or
  `--element`, rounded to whole design units.
- `style` holds `getComputedStyle` values, not authored shorthand. Colours stay
  as computed `rgb()`/`rgba()`; `normalizedRGB` gives the same values as numbers
  for comparers that sample screenshots, and `backgroundImage` classifies
  `none`/`gradient`/`image`/`other` without keeping the URL. Alpha, gradients and
  blur quality are out of scope (WALKS §2.5).
- `role` is a closed semantic name: `button`, `link`, `img`, `heading`,
  `textbox`, `searchbox`, `checkbox`, `radio`, `slider`, `combobox`,
  `navigation`, `main`, `list`, `listitem`, `table`, `row`, `cell`, `text`,
  `other`. An explicit short ARIA role wins; drawn action classes (`btn`, `chip`, `tab`, `card`, `icon-btn`, `ghost`, and the like) map to `button`; otherwise standard tag semantics apply.
- `textMode` is required on every element: `static` for copy drawn from the
  mock-up and `data` for content whose real value differs (title, poster, count,
  identity, date, a repeated list child). A `--data` selector marks the element
  and its subtree. Rendering refuses without an explicit mapping: pass `--data
  <selector>` or `--no-data` when the frame truly has no data content. The
  artifact therefore always says `dataMapping: "explicit"`.
- `identifier` is optional and appears on elements matched by `--identifier
  selector=identifier` (for example `--identifier '.btn.play=watch-title-play'`).
  It is the app's own test identifier, so repeated cards or controls pair by
  intent even when the drawn copy differs.
- `normalizedRGB` is the element's flat opaque background as `[r, g, b]`, and is
  omitted for translucent or gradient backgrounds. It wins over parsing
  `style.backgroundColor`; the full alpha values remain in
  `style.normalizedRGB`.

Text is the visible copy (bounded to 200 characters). A comparer pairs by role,
label, `identifier` and order, never by guessing real content.

The artifact is the only shared interface. `web-style.mjs` owns web app
comparisons. The native comparer (`deployment/testing/walks/native-layout.py` and
`native/`) reads the same artifact for iOS, tvOS, Android and TV surfaces; a
frame surface is either web or native, never both. Field spelling and units were
agreed directly with the native comparer owner; `identifier`, `textMode`,
`units`, the semantic roles and the colour precedence above are the settled
contract.

## Compare with the app

```sh
walks/web-style auth.html 4 \
  --app http://127.0.0.1:8080/ \
  --width 1440 --height 900 \
  --app-wait 'main' \
  --data '.rchip,.in,.dots' \
  --pair '.btn.play => main .actions button:first-child' \
  --pair '.tb .chip[aria-label="Filters"] => main [role="toolbar"] button:first-child' \
  --order '.tabs .tab, .card .t => main nav[aria-label="Library views"] button, main [data-card-kind] .t' \
  --out v3/walks/frame-artifacts
```

Each `--pair` is `frame selector => app selector`. Frame selectors are resolved
inside the selected frame; app selectors are ordinary page selectors. Each side
must resolve to exactly one element. Pairs are the walker's locators on purpose:
a raw whole-tree diff would drown in wrapper elements that are not fidelity
findings.

Pair stable chrome, controls and copy. A pair whose box or text changes with real
data (a title, a count, a synopsis) will be marked `data` by the `--data`
mapping; pair the stable ancestor or control instead, and let `DATA` rows stand
for the sample data. Text differences on `data` elements are never fidelity
findings. Like the renderer, the extractor needs an explicit mapping (`--data` or
`--no-data`); text and order labels are judged only against that mapping.

`--order` compares the visible label sequence of two selector lists, so order
changes are measured too.

Optional: `--storage-state <0600 file>` for a signed-in walk account, used by
reference only and never printed. `--strict-text` makes any copy difference a
fidelity difference; otherwise differing text is reported as `DATA` (real data
versus the frame's sample data), while missing expected text is always fidelity.

## Output and citation

Every difference row carries its citation in this shape:

```text
DIFF auth.html 4 .btn.play height: expected 48 actual 44 delta -4
DATA auth.html 4 .card .t text: expected "Severance" actual "Alien"
```

The JSON report adds `kind`, `field`, `fidelity`, `citation`, `appSelector` and
`expected`/`actual` for each row, plus the frame file, label, device viewport and
the counts of refused cross-origin requests. `frame.mjs`'s JSON gives every
mock-up element's box, computed type, colour, spacing, radius, order and a
selector path, so an OFF-FRAME verdict can quote the frame's exact expected
values (WALKS §2b). Work-list fix items can carry the same `citation` string.

Exit codes: `0` no fidelity differences, `1` measured fidelity differences,
`2` tool, label, font or navigation failure.

## Safety

- The design context allows only `file:`, `data:`, `blob:` and `about:` requests;
  the app context allows only the app's own origin. Every other request is
  refused and only counted, never recorded.
- No URL query, header, cookie, token, input value, credential, page error or
  stack is written to output.
- The tools are read-only for apps and mock-ups. No app code, guard, contract or
  shared renderer is changed here.

Device custody for walks: reuse `deployment/testing/checkpoints/avd_custody.py` — it holds `/tmp/portico-avd-<exact-name>.lock` for the whole window, proves the serial-to-AVD identity and requires exactly one agent-device session before anything is installed. `DEVICE_IN_USE` means skip, never force.
