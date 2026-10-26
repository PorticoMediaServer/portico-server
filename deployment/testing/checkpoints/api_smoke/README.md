# Server API smoke (isolated loopback)

One bounded API pass over the checkpoint runner's real source and fixture
runtime, after its library setup and before its disposable device is removed.
It does not deploy, scan, build, drive a client or read a runtime log.

## Entry point

```sh
python3 deployment/testing/checkpoints/api-smoke.py \
  --context /private/api-context.json \
  --origin http://127.0.0.1:PORT \
  --run-id m0-... \
  --output /evidence/flow-api-smoke.log
```

`--origin` and `--run-id` are required and must equal the context values. Only
an explicit numeric loopback HTTP origin is admitted
(`http://127.0.0.1:<1024-65535>`); the client uses `http.client` directly, so
environment proxies, redirects and TLS egress cannot apply. The default output
is stdout; a file is written 0600 with `O_NOFOLLOW`.

## Private runtime context

The runner writes this file by reference; it is never copied to evidence.

```json
{
  "origin": "http://127.0.0.1:PORT",
  "accessToken": "<owner session token>",
  "installationId": "<setup installation id>",
  "owner": true,
  "scope": "checkpoint-disposable",
  "runId": "m0-...",
  "runtime": {"pid": 12345, "startTimeTicks": 123456789}
}
```

The file must be an owned regular file with mode exactly 0600 and no symlink.
Unknown keys are refused. `scope` must be exactly `checkpoint-disposable`.
`owner` only enables the read-only owner settings paths; it is never inferred
from a response.

## Runtime custody

A private-looking context is not proof of a disposable runtime. Before any
request, `api_smoke/custody.py` checks that:

- `/proc/<pid>` exists and is owned by the current user;
- the process start ticks in `/proc/<pid>/stat` equal `startTimeTicks`;
- the process owns the LISTEN socket on `127.0.0.1:<port>` (matched through
  `/proc/net/tcp` inode and `/proc/<pid>/fd`).

A restart, a reused PID or a different listener is refused with a closed
`runtime-*` reason before the first request or mutation. The marker is issued by
the runner owner under the Job lifetime; it is not a production credential or
endpoint.

`app.pid` in the runner is the bwrap launcher, which may not own the actual
server listener. The owner integration must supply the verified exact spawned
server process (or its exact original descendant) in `runtime.pid` and
`startTimeTicks`. Never substitute an arbitrary listener found by port; a
missing or ambiguous marker is `unavailable` for the owner's integration, not an
admitted runtime.

## Receipt

The output is a JSON document with `flows`, each carrying a closed `id`,
`status` (`pass`, `fail`, `unavailable`), `kind`, an optional `http` status, a
closed endpoint `code`, a closed `reason`, a closed `label` and bounded
boolean/count `facts`. Titles, ids, tokens, URLs, bodies, raw headers and
exception text are never published.

The flows are:

- `health-live`, `health-ready` (`GET /health/live`, `GET /health/ready`, both
  source contracts at status 200; readiness may answer 503 before setup, so the
  flow runs only after the runner's readiness wait);
- `home-real-rows`, `library-real-content`, `detail-real-item`,
  `search-real-title`;
- `reading-list`, `reading-session-open`, `reading-resource`,
  `reading-progress-read`, `reading-progress-save`, `reading-progress-confirm`,
  `reading-progress-stale-guard`, `reading-session-close`;
- `playback-options`, `playback-queue-create`, `playback-queue-read`,
  `playback-start-admission`, `playback-preparing`, `playback-current`,
  `playback-controls-read`, `playback-stop`, `playback-queue-cleanup`;
- `settings-me`, `settings-downloads`, `settings-console`,
  `settings-configuration`, and `cleanup-owned-handles`.

A missing prerequisite marks dependent flows `unavailable`; a real fixture or
contract failure is `fail`, never a pass. Book absence does not fail or stop
movie/library coverage; the movie and book prerequisites are resolved
independently and each dependent flow reports its own unavailable reason.

## Playback is admission, not progress

`playback-start-admission` reports only that the server accepted the start and
created a session (`admitted`, `sessionCreated`, `preparing`). A `202` is a
successful start in preparation, not playback-progress proof.
`playback-preparing` and `playback-current` report the server session state
(`preparing`, `serverStatePlaying`, `serverStatePaused`, `serverStatePreparing`,
`observedPlayback`). No renderer timeline is reported, so none of these flows
claims actual playback, progress or M0 acceptance. No capability document is
written and no default/fake capability profile is supplied.

## Reading progress scope

`reading-progress-read` always validates the current progress document. It does
not invent a location: no hard-coded CFI, page index or progression is written.

`reading-progress-save`, `reading-progress-confirm` and
`reading-progress-stale-guard` run only when the read found an existing
progress location that is source-matched to the session publication and valid
for the current format. That exact location is replayed with the current
expected revision, then with the stale revision; the stale write must answer
`412 revision_mismatch` (verified against `apikit/apierror`). If no such
location exists, those flows are `unavailable` with a closed
`no-existing-reading-location`, `reading-source-mismatch` or
`reading-location-unusable` reason.

When it does run, the save writes the current profile's reading position for the
exact fixture item. That is a persistent profile mark in the run's disposable
scratch database, not a session-only mutation. It is permitted only under
`scope: checkpoint-disposable` and the verified Job runtime; it never touches a
retained or production profile. The cleanup path closes the owned reading
session, playback session and queue; any progress row remains only in the
scratch database the runner deletes.

## Integration hook

Published `runner.py` has the setup inline in `Job.web_server`; the existing
static `api()` helper is module-level, so there is no `Job.api` method. After
the real-fixture `/v1/home` check and before the device cleanup, the owner can
add:

```python
# app.pid is the bwrap launcher; resolve the exact spawned server process
# (or its exact original descendant) as server_pid. Never use an arbitrary
# listener found by port.
server_pid = ...
stat = Path('/proc/%d/stat' % server_pid).read_text()
private_json(self.work/'api-context.json', {
    'origin': base, 'accessToken': token, 'installationId': installation,
    'owner': True, 'scope': 'checkpoint-disposable', 'runId': self.args.run_id,
    'runtime': {'pid': server_pid,
                'startTimeTicks': int(stat.rpartition(')')[2].split()[19])}})
result = self.out/'flow-api-smoke.log'
try:
    self.run('api-smoke', ['python3', str(self.tools/'deployment/testing/checkpoints/api-smoke.py'),
        '--context', str(self.work/'api-context.json'), '--origin', base,
        '--run-id', self.args.run_id, '--output', str(result)], timeout=420)
except StepFailure:
    if not result.is_file():
        raise
finally:
    if result.is_file():
        self.report['flows'] += json.loads(result.read_text())['flows']
```

The output name matches the existing `flow-*.log` evidence copy and receive
allowlists, so `finish()` needs no change. The CLI writes the receipt before its
non-zero product-failure exit; the hook re-raises only when no receipt exists.

Mutations are limited to the reading session, playback session and queue this
smoke created, plus a replayed existing fixture progress location in the
disposable scratch. Its `finally` path closes the exact handles; there are no
library scans, broad deletes, public demo writes or Hosted writes. No
device-approval, pending-grant, poll, outcome, notice or decline path is
touched; Root §56's excluded feature remains excluded.
