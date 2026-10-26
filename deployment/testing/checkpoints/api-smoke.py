#!/usr/bin/env python3
"""Bounded server API smoke against one explicitly admitted loopback runtime.

The runner owns the real source, fixture library, ingestion and owner session.
This CLI reads that runtime by reference from a private 0600 context and covers
health/readiness, real-fixture Home/library/detail/search, the Reading
session/resource/progress lifecycle, playback options/queue/start admission/
current/stop, and read-only settings. It mutates only the exact reading
session, playback session and queue it created, and replays an existing
source-matched reading location when the fixture has one; that progress mark
stays in the run's disposable scratch database. It closes the owned handles in
a finally path.

Before any request, custody is verified against the live Job runtime: the
context names the run id, the runtime PID and its process start ticks, and the
CLI checks that this process is ours and owns the admitted loopback listener. A
context that only looks private is not enough.

The receipt publishes only closed operation ids, statuses, endpoint codes,
reasons and bounded boolean/count facts. It never reads or exports a server
runtime log, and never publishes a title, id, token, URL, body or exception.
Playback is reported as start admission and server session/preparing state only;
it is not playback-progress or M0 proof. No invented EPUB CFI, page index or
progression is sent.

Runner hook (owner-owned; this file does not edit runner.py). In Job.web_server,
inside the setup try block after the real-fixture /v1/home check and before the
device cleanup, write the context with the existing private_json() helper. The
runner's app.pid is the bwrap launcher; the owner must resolve the exact spawned
server process (or its exact original descendant) as server_pid and must not
substitute an arbitrary listener found by port. A missing marker is unavailable:

    # app.pid is the bwrap launcher; resolve the exact spawned server process.
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

The output name matches the existing flow-*.log evidence copy and receive
allowlists, so the hook needs no finish() change. A product failure is already
in the receipt, so the hook re-raises only when no receipt was written. The CLI
writes the receipt before its non-zero exit, as the browser hook already does.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import signal
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from api_smoke import ContextError, run
from api_smoke.context import admitted_origin

def write_output(target, document):
    if target == '-':
        sys.stdout.write(json.dumps(document, indent=2) + '\n')
        return
    path = Path(target)
    if not path.parent.is_dir():
        raise SystemExit('output directory unavailable')
    descriptor = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC | os.O_NOFOLLOW, 0o600)
    with os.fdopen(descriptor, 'w') as stream:
        json.dump(document, stream, indent=2)
        stream.write('\n')

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--context', required=True, type=Path)
    parser.add_argument('--origin', required=True)
    parser.add_argument('--run-id', required=True)
    parser.add_argument('--output', default='-')
    parser.add_argument('--deadline-seconds', type=int, default=300)
    args = parser.parse_args()
    os.umask(0o077)
    def stop(*_):
        raise SystemExit(143)
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGHUP, stop)
    try:
        admitted_origin(args.origin)
        if not re.fullmatch(r'[a-z0-9-]{1,96}', args.run_id):
            raise ContextError('run-id-not-admitted')
        document = run(args.context, args.origin, args.run_id, args.deadline_seconds)
    except ContextError as error:
        print(json.dumps({'error': str(error)}))
        return 2
    except SystemExit:
        raise
    except Exception:
        print(json.dumps({'error': 'unclassified'}))
        return 3
    try:
        write_output(args.output, document)
    except OSError:
        print(json.dumps({'error': 'output-unavailable'}))
        return 3
    failed = sum(1 for flow in document['flows'] if flow['status'] == 'fail')
    unavailable = sum(1 for flow in document['flows'] if flow['status'] == 'unavailable')
    if args.output != '-':
        print(json.dumps({'flows': len(document['flows']), 'failed': failed, 'unavailable': unavailable}))
    return 1 if failed else 0

if __name__ == '__main__':
    raise SystemExit(main())
