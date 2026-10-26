#!/usr/bin/env python3
"""One bounded transcode-environment check against an admitted loopback runtime.

Reads the server's published transcode capacity and delivery policy, starts one
ordinary authenticated playback session on a named fixture item with a low
quality ceiling so a conversion is requested, then fetches a small bounded
sample of the bytes the session actually delivers from its own same-origin
`/v1/media/` capability URL and probes that sample with an explicit absolute
`ffprobe` path. A 202 that already carries a real session view is owned and
stopped in a finally path; the id and generation must stay stable to the 201. A
conversion is only reported passed when the delivered bytes and probe prove a
converted stream within the requested height ceiling and the owned session was
stopped and confirmed ended.

Owner-only facts (capacity) are read only when the private context is the owner's;
a member context records them `unavailable — owner-required` and still runs the
device-scope steps. Nothing here changes server settings, guards, admission or
any production code.

Runner hook (owner-owned; this file does not edit runner.py). In Job.web_server,
after the real-fixture check and before the device cleanup, write the existing
private context with private_json() exactly as api-smoke documents it, then:

    result = self.out/'flow-transcode-check.log'
    try:
        self.run('transcode-check', ['python3', str(self.tools/'deployment/testing/checkpoints/transcode-check.py'),
            '--context', str(self.work/'api-context.json'), '--origin', base,
            '--run-id', self.args.run_id, '--item', fixture_movie_id,
            '--ffprobe', '/usr/bin/ffprobe', '--scratch', str(self.work),
            '--output', str(result)], timeout=180)
    except StepFailure:
        if not result.is_file():
            raise
    finally:
        if result.is_file():
            self.report['flows'] += json.loads(result.read_text())['steps']

The output name matches the existing flow-*.log evidence copy. A product failure
is already in the receipt, so the hook re-raises only when no receipt was
written. `fixture_movie_id` is the id the runner already holds; never a title.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import signal
import sys
import tempfile
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

from api_smoke import ContextError
from api_smoke.context import admitted_origin
from transcode_check import run


def write_output(target, document):
    if target == '-':
        sys.stdout.write(json.dumps(document, indent=2) + '\n')
        return
    path = Path(target)
    if not path.parent.is_dir():
        raise SystemExit('output directory unavailable')
    descriptor, temporary = tempfile.mkstemp(prefix='.transcode-receipt-', dir=path.parent)
    try:
        os.fchmod(descriptor, 0o600)
        with os.fdopen(descriptor, 'w') as stream:
            json.dump(document, stream, indent=2)
            stream.write('\n')
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--context', required=True, type=Path)
    parser.add_argument('--origin', required=True)
    parser.add_argument('--run-id', required=True)
    parser.add_argument('--item', required=True)
    parser.add_argument('--output', default='-')
    parser.add_argument('--budget-seconds', type=int, default=90)
    parser.add_argument('--ffprobe', help='absolute path to the qualified ffprobe; never searched on PATH')
    parser.add_argument('--scratch', help='job-private scratch directory for the probe sample')
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
        if not re.fullmatch(r'[A-Za-z0-9._~-]{1,256}', args.item):
            raise ContextError('item-not-admitted')
        document = run(args.context, args.origin, args.run_id, args.item, args.budget_seconds,
                       ffprobe=args.ffprobe, scratch_root=args.scratch)
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
    failed = sum(1 for step in document['steps'] if step['status'] == 'fail')
    if args.output != '-':
        print(json.dumps({'steps': len(document['steps']), 'failed': failed,
                          'result': document['result']}))
    return 1 if document['result'] == 'failed' else 0


if __name__ == '__main__':
    raise SystemExit(main())
