"""Synthetic transport/probe boundary regressions; no server, device or network."""
import copy
import io
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

HERE = Path(__file__).parent
sys.path.insert(0, str(HERE))
from api_smoke.client import Deadline
from transcode_check import media
from transcode_check.check import CheckFailure, retry_hint
from test_transcode_check import FakeClient, FakeDeadline, VIEW_BODY, routes, run


class Boundaries(unittest.TestCase):
    def test_missing_hint_on_owned_202_still_cleans_up(self):
        data = copy.deepcopy(VIEW_BODY)
        data.pop('retryAfterMs', None)
        doc = run(FakeClient(routes(start=[(202, data)])))
        self.assertEqual(doc['result'], 'failed')
        self.assertEqual(doc['steps'][-1]['reason'], 'retry-hint-invalid')
        self.assertTrue(doc['cleanup']['stopped'])

    def test_get_generation_drift_refuses_and_cleans_original(self):
        data = copy.deepcopy(VIEW_BODY)
        data['presentation']['generation'] += 1
        doc = run(FakeClient(routes(view=data)))
        self.assertEqual(doc['result'], 'failed')
        self.assertTrue(doc['cleanup']['stopped'])

    def test_each_playlist_hop_rechecks_original_runtime(self):
        calls = []
        context = SimpleNamespace(host='127.0.0.1', port=1, access_token='synthetic', installation_id='synthetic')
        check = media.LoopbackMedia(context, FakeDeadline(), verify=lambda ctx: calls.append(ctx))
        bodies = [b'#EXTM3U\n/v1/media/fixture-segment\n', b'G' + b'x' * 188 * 8]
        class Response:
            status = 200
            def __init__(self):
                self.body = io.BytesIO(bodies.pop(0))
                self.fp = None
            def getheader(self, name): return 'application/octet-stream'
            def read1(self, limit): return self.body.read(limit)
        class Connection:
            def __init__(self, *args, **kwargs): self.sock = self
            def settimeout(self, timeout): pass
            def request(self, *args, **kwargs): pass
            def getresponse(self): return Response()
            def close(self): pass
        with patch.object(media.http.client, 'HTTPConnection', Connection):
            result = check.fetch('/v1/media/fixture-list')
        self.assertEqual(result['via'], 'segment')
        self.assertEqual(len(calls), 6)
        self.assertTrue(all(x is context for x in calls))

    def test_probe_timeout_and_output_cap_reap_only_own_process(self):
        # These are tiny synthetic child scripts, never an installed media tool.
        with tempfile.TemporaryDirectory(prefix='.transcode-test-', dir=Path.home()) as tmp:
            root = Path(tmp)
            for name, program, reason in [
                ('timeout', 'import time;time.sleep(60)', 'probe-timeout'),
                ('overflow', 'import sys;sys.stdout.write("x"*1000000);sys.stdout.flush()', 'probe-failed'),
            ]:
                executable = root / name
                executable.write_text('#!' + sys.executable + '\n' + program + '\n')
                executable.chmod(0o700)
                children = []
                real_popen = subprocess.Popen
                def spawn(*args, **kwargs):
                    child = real_popen(*args, **kwargs)
                    children.append(child)
                    return child
                check = media.LoopbackMedia(None, Deadline(2), ffprobe=str(executable), scratch_root=str(root))
                with patch.object(media.subprocess, 'Popen', spawn), patch.object(media, '_PROBE_TIMEOUT', .2):
                    result = check.probe(b'synthetic sample')
                self.assertEqual(result['reason'], reason)
                self.assertEqual(len(children), 1)
                self.assertIsNotNone(children[0].returncode)
                self.assertEqual(sorted(x.name for x in root.iterdir()), sorted(['timeout'] if name == 'timeout' else ['timeout', 'overflow']))


if __name__ == '__main__':
    unittest.main()
